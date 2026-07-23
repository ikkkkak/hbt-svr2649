package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/capture"
	"apartments-clone-server/MeskenyGPT/internal/ai/client"
	"apartments-clone-server/MeskenyGPT/internal/ai/escalation"
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/property"
	"apartments-clone-server/MeskenyGPT/internal/ai/rag"
	"apartments-clone-server/MeskenyGPT/internal/ai/response"
	"apartments-clone-server/MeskenyGPT/internal/ai/rules"
	"apartments-clone-server/MeskenyGPT/internal/ai/safety"
	"apartments-clone-server/MeskenyGPT/internal/ai/vector"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// ChatInput is the data needed for a single AI turn.
type ChatInput struct {
	UserID    uint
	SessionID string
	Text      string
	DeepThink bool
	// History: prior turns (user + assistant only), chronological order.
	// Used for conversational mode so the agent remembers context.
	History []ChatMessage
}

// ChatOutput is what the frontend expects from a single AI turn.
type ChatOutput struct {
	Message                 response.Message  `json:"message"`
	PropertyRecommendations []property.Card   `json:"propertyRecommendations,omitempty"`
	QuickReplies            []response.QuickReply `json:"quick_replies,omitempty"`
	SessionID               string            `json:"session_id,omitempty"`
	InteractionID           uint              `json:"interaction_id,omitempty"`
	Escalation              *EscalationInfo   `json:"escalation,omitempty"`
}

type EscalationInfo struct {
	ID      uint   `json:"id"`
	Status  string `json:"status"`
	Urgency string `json:"urgency"`
	Reason  string `json:"reason"`
}

// Service is the public AI interface used by HTTP handlers.
type Service interface {
	HandleChatTurn(ctx context.Context, in ChatInput) (ChatOutput, error)
	HandleAgentRun(ctx context.Context, in AgentRunInput, emit func(AgentEvent)) (ChatOutput, error)
	GetGreeting(ctx context.Context, l lang.Lang) (ChatOutput, error)
	UpdateSessionFilters(ctx context.Context, sessionID string, patch SessionFilterPatch) (SessionFilterContext, error)
}

type service struct {
	cfg      Config
	or       client.OpenRouterClient
	props    property.Store
	guard    safety.Guard
	logger   capture.Logger
	retriever rag.Retriever
	gdb      *gorm.DB
	rdb      *redis.Client
	vector     *vector.Engine
	escalation *escalation.Engine
}

const lastCardsKeyPrefix = "meskenygpt:last_cards:"
const lastCardsTTL = 20 * time.Minute

func looksLikeSummariseLastResultsQuery(l lang.Lang, msg string) bool {
	s := strings.ToLower(strings.TrimSpace(msg))
	if s == "" {
		return false
	}
	switch l {
	case lang.LangAR:
		return strings.Contains(msg, "افضل") || strings.Contains(msg, "أفضل") ||
			strings.Contains(msg, "العروض") || strings.Contains(msg, "من هذه") ||
			strings.Contains(msg, "اللي") || strings.Contains(msg, "ارسلت") ||
			strings.Contains(msg, "أرسلت") || strings.Contains(msg, "اختر") || strings.Contains(msg, "اختار")
	case lang.LangFR:
		return strings.Contains(s, "meille") || strings.Contains(s, "parmi") || strings.Contains(s, "ces") || strings.Contains(s, "que tu as envoyé")
	default:
		return strings.Contains(s, "best") || strings.Contains(s, "which one") || strings.Contains(s, "among these") || strings.Contains(s, "from these") || strings.Contains(s, "you sent")
	}
}

func (s *service) cacheLastCards(ctx context.Context, sessionID string, cards []property.Card) {
	if s.rdb == nil || strings.TrimSpace(sessionID) == "" || len(cards) == 0 {
		return
	}
	b, err := json.Marshal(cards)
	if err != nil {
		return
	}
	_ = s.rdb.Set(ctx, lastCardsKeyPrefix+sessionID, string(b), lastCardsTTL).Err()
}

func (s *service) loadLastCards(ctx context.Context, sessionID string) []property.Card {
	if s.rdb == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	raw, err := s.rdb.Get(ctx, lastCardsKeyPrefix+sessionID).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var cards []property.Card
	if err := json.Unmarshal([]byte(raw), &cards); err != nil {
		return nil
	}
	return cards
}

func isSharedPropertyRequest(text string) bool {
	return strings.Contains(text, "Property context (shared from app card):")
}

func isValuationIntentText(text string) bool {
	l := strings.ToLower(text)
	return strings.Contains(l, "market value") ||
		strings.Contains(l, "valuation") ||
		strings.Contains(l, "estimated value") ||
		strings.Contains(l, "estimate value") ||
		strings.Contains(l, "how much is it worth") ||
		strings.Contains(l, "worth") ||
		strings.Contains(l, "valeur du marché") ||
		strings.Contains(l, "estimation") ||
		strings.Contains(l, "prix du marché") ||
		strings.Contains(l, "تقييم") ||
		strings.Contains(l, "القيمة السوقية") ||
		strings.Contains(l, "قيمة سوقية")
}

func noCardsClarificationText(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "باش نضمن لك نتائج دقيقة من الإعلانات الحقيقية، حدد لي من فضلك: كراء أو بيع، والمنطقة/السكتور، وميزانية تقريبية."
	case lang.LangEN:
		return "To return accurate real listings, please specify: rent or sale, area/secteur, and an approximate budget."
	default:
		return "Pour te donner des annonces réelles et précises, indique: location ou achat, zone/secteur, et un budget approximatif."
	}
}

func looksLikeFoundResultsClaim(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	claims := []string{
		"i found", "found", "i found these", "here are",
		"j'ai trouvé", "je trouve", "voici",
		"وجدت", "عثرت", "لقيت", "هاذ", "هذه العروض", "هذه النتائج",
	}
	for _, c := range claims {
		if strings.Contains(t, c) {
			return true
		}
	}
	return false
}

func enforceNoCardsResponseIntegrity(msgCtx lang.MessageContext, text string) string {
	trimmed := strings.TrimSpace(text)
	// This guard exists ONLY for property-search answers — it stops the model
	// claiming it "found listings" when there are no cards. It must NOT touch
	// informational/procedure answers: a procedure answer legitimately says
	// "here are the steps / إليك الخطوات / هذه الوثائق", which is not a false
	// listings claim. Applying it there replaced good, grounded procedure
	// answers with the wrong "rent or buy? budget?" property-search prompt.
	if msgCtx.Intent == lang.IntentInfoProcedure {
		return trimmed
	}
	if trimmed == "" {
		return noCardsClarificationText(msgCtx.Lang)
	}
	if looksLikeFoundResultsClaim(trimmed) {
		return noCardsClarificationText(msgCtx.Lang)
	}
	return trimmed
}

func shouldOverrideIdentityReply(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	banned := []string{
		"openai", "chatgpt", "claude", "anthropic", "google", "gemini",
		"mistral", "llama", "meta ai", "copilot",
	}
	for _, b := range banned {
		if strings.Contains(t, b) {
			return true
		}
	}
	return false
}

func enforceMeskenyIdentity(msgCtx lang.MessageContext, text string) string {
	if !shouldOverrideIdentityReply(text) {
		return strings.TrimSpace(text)
	}
	switch msgCtx.Lang {
	case lang.LangAR:
		return "أنا MeskenyGPT، المساعد الذكي داخل تطبيق Meskeny للعقار في موريتانيا. أقدر نعاونك في البحث عن العقارات، المقارنة بين الخيارات، وفهم السوق المحلي بشكل عملي."
	case lang.LangFR:
		return "Je suis MeskenyGPT, l'assistant intelligent intégré à l'application Meskeny pour l'immobilier en Mauritanie. Je peux t'aider à chercher des biens, comparer des options et comprendre le marché local."
	default:
		return "I am MeskenyGPT, the in-app smart assistant for Meskeny real estate in Mauritania. I can help you search listings, compare options, and understand the local market."
	}
}

// NewService wires all AI dependencies together.
func NewService(cfg Config, db any, cache any) Service {
	var retr rag.Retriever
	var gdbRef *gorm.DB
	var rdbRef *redis.Client
	if gdb, ok := db.(*gorm.DB); ok && gdb != nil {
		gdbRef = gdb
		retr = rag.NewCompositeRetriever(gdb)
	} else {
		retr = rag.NewMauritaniaRetriever()
	}
	if rdb, ok := cache.(*redis.Client); ok && rdb != nil {
		rdbRef = rdb
	}
	orClient := client.NewOpenRouterClient(cfg.APIKey, cfg.Model, cfg.TimeoutSeconds)
	return &service{
		cfg:       cfg,
		or:        orClient,
		props:     property.NewStore(db),
		guard:     safety.NewGuard(),
		logger:    capture.NewDBLogger(),
		retriever: retr,
		gdb:       gdbRef,
		rdb:       rdbRef,
		vector:    vector.NewEngine(gdbRef),
		escalation: escalation.NewEngine(gdbRef),
	}
}

func (s *service) HandleChatTurn(ctx context.Context, in ChatInput) (ChatOutput, error) {
	start := time.Now()

	if strings.TrimSpace(in.Text) == "" {
		return ChatOutput{}, fmt.Errorf("empty message")
	}

	// 1) Safety checks
	if blocked, reason := s.guard.Check(in.Text); blocked {
		blk := response.BlockedOutput(reason)
		msgCtx := lang.AnalyzeMessage(in.Text)
		s.recordTurn(ctx, capture.TurnPathBlocked, in.SessionID, in.UserID, len(in.History)/2, msgCtx,
			in.Text, blk.Message.Content, -1, time.Since(start).Milliseconds())
		return ChatOutput{Message: blk.Message}, nil
	}

	// 2) Lang + intent
	msgCtx := lang.AnalyzeMessage(in.Text)
	msgCtx.RawText = strings.TrimSpace(in.Text)
	histTurns := make([]lang.HistoryTurn, 0, len(in.History))
	for _, h := range in.History {
		histTurns = append(histTurns, lang.HistoryTurn{Role: h.Role, Content: h.Content})
	}
	msgCtx = lang.EnrichContextFromHistory(msgCtx, histTurns, in.Text)
	msgCtx = s.hydrateSessionFilters(ctx, in.SessionID, msgCtx)
	if strings.Contains(in.Text, "[MESKENY_PICKER]") {
		// Picker submissions may append an English helper sentence from mobile
		// ("Use the filters exactly..."), which can wrongly force EN.
		// For picker turns, prefer the most recent *real user* language from history.
		if histLang, ok := lastUserLangFromHistory(in.History); ok {
			msgCtx.Lang = histLang
		}
	}
	sharedPropertyMode := isSharedPropertyRequest(in.Text)
	valuationMode := sharedPropertyMode && isValuationIntentText(in.Text)
	s.persistSessionFilters(ctx, in.SessionID, msgCtx)

	// Never run empty geo DB queries (city= zone= budget=0) — proactive clarification first.
	if lang.ShouldClarifyBeforeSearch(msgCtx) {
		cl := response.ProactiveClarificationOutput(msgCtx)
		s.recordTurn(ctx, capture.TurnPathClarify, in.SessionID, in.UserID, len(in.History)/2, msgCtx,
			in.Text, cl.Message.Content, -1, time.Since(start).Milliseconds())
		return ChatOutput{
			Message:      cl.Message,
			QuickReplies: cl.QuickReplies,
			SessionID:    in.SessionID,
		}, nil
	}

	// 3) Property search: pure DB mode
	if lang.IsPropertySearchIntent(msgCtx.Intent) {
		// Apply admin deterministic search rules (no tokens, DB-backed).
		rules.ApplyAdminSearchRules(s.gdb, &msgCtx)

		f := property.FiltersFromContext(msgCtx)
		f.Query = in.Text
		var props []property.Property
		var err error
		usedSemantic := false
		if s.vector != nil && s.vector.Enabled() && msgCtx.Intent != lang.IntentSearchLand {
			semProps, _, semErr := s.vector.Search(ctx, in.Text, f, 12)
			if semErr == nil && len(semProps) > 0 {
				props = semProps
				usedSemantic = true
			}
		}
		if !usedSemantic {
			if msgCtx.Intent == lang.IntentSearchLand {
				props, err = s.props.FindLandmarks(ctx, f)
			} else {
				props, err = s.props.Find(ctx, f)
			}
		}
		if err != nil {
			fmt.Printf("❌ MeskenyGPT property/landmark search error: %v\n", err)
		}
		cards := property.ToCards(props)
		s.cacheLastCards(ctx, in.SessionID, cards)

		if len(cards) == 0 {
			if valuationMode {
				msg := response.Message{
					ID:   fmt.Sprintf("msg_%d", time.Now().UnixNano()),
					Role: "assistant",
				}
				switch msgCtx.Lang {
				case lang.LangAR:
					msg.Content = "لا أستطيع تقديم تقييم سوقي موثوق لهذا العقار الآن لأن المقارنات المباشرة في نفس المنطقة/الفئة غير كافية في البيانات الحالية. أرسل المدينة أو الميزانية أو النوع وسأعطيك تقديراً عملياً مع نطاق سعري واضح."
				case lang.LangEN:
					msg.Content = "I can’t produce a reliable market valuation for this exact listing yet because direct comparable listings in the same area/segment are not sufficient right now. Share city, budget, or type and I’ll return a practical price range with confidence notes."
				default:
					msg.Content = "Je ne peux pas donner une estimation de marché fiable pour ce bien précis pour le moment, car les comparables directs dans la même zone/catégorie sont insuffisants. Donne la ville, le budget ou le type et je te renverrai une fourchette de prix claire avec un niveau de confiance."
				}
				s.recordTurn(ctx, capture.TurnPathNoResults, in.SessionID, in.UserID, len(in.History)/2, msgCtx,
					in.Text, msg.Content, 0, time.Since(start).Milliseconds())
				return ChatOutput{
					Message:   msg,
					SessionID: in.SessionID,
				}, nil
			}
			no := response.NoResultsOutput(msgCtx)
			s.recordTurn(ctx, capture.TurnPathNoResults, in.SessionID, in.UserID, len(in.History)/2, msgCtx,
				in.Text, no.Message.Content, 0, time.Since(start).Milliseconds())
			return ChatOutput{
				Message:      no.Message,
				QuickReplies: no.QuickReplies,
				SessionID:    in.SessionID,
			}, nil
		}

		// Deterministic DB-backed messaging:
		// We intentionally avoid asking the LLM to "list options" in free-text,
		// because it can hallucinate extra listings/prices.
		with := response.WithCardsOutput(msgCtx, cards)
		msg := with.Message
		if valuationMode {
			msg.Content = s.summarisePropertiesWithModel(ctx, msgCtx, in.Text, cards)
		}
		if msg.ID == "" {
			msg.ID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
		}

		elapsed := time.Since(start).Milliseconds()

		out := ChatOutput{
			Message:                 msg,
			PropertyRecommendations: with.PropertyRecommendations,
			QuickReplies:            response.SearchResultsFollowUpQuickReplies(msgCtx),
			SessionID:               in.SessionID,
		}

		out.InteractionID = s.recordTurn(ctx, capture.TurnPathDBSearch, in.SessionID, in.UserID,
			len(in.History)/2, msgCtx, in.Text, msg.Content, len(cards), elapsed)
		return s.withEscalation(ctx, in, out), nil
	}

	// Follow-up: user asks to pick best among previous DB cards (needs session cache).
	if msgCtx.Intent == lang.IntentUnknown && looksLikeSummariseLastResultsQuery(msgCtx.Lang, in.Text) {
		if last := s.loadLastCards(ctx, in.SessionID); len(last) > 0 {
			msg := response.Message{
				ID:   fmt.Sprintf("msg_%d", time.Now().UnixNano()),
				Role: "assistant",
			}
			msg.Content = s.summarisePropertiesWithModel(ctx, msgCtx, in.Text, last)
			out := ChatOutput{
				Message:                 msg,
				PropertyRecommendations: last,
				QuickReplies:            response.SearchResultsFollowUpQuickReplies(msgCtx),
				SessionID:               in.SessionID,
			}
			out.InteractionID = s.recordTurn(ctx, capture.TurnPathFollowUp, in.SessionID, in.UserID,
				len(in.History)/2, msgCtx, in.Text, msg.Content, len(last), time.Since(start).Milliseconds())
			return out, nil
		}
	}

	// 4) Conversational mode: retrieve RAG context and call OpenRouter
	ragCtx, _ := s.retriever.Retrieve(ctx, msgCtx)

	sys := buildAgentConversationalSystemPrompt(msgCtx, len(in.History) > 0)
	if ragCtx.MarketSummary != "" {
		sys += "\n\n=== MAURITANIA CONTEXT (use this for all answers) ===\n" + ragCtx.MarketSummary
	}
	for _, n := range ragCtx.Notes {
		sys += "\n- " + n
	}
	if len(ragCtx.FAQSnippets) > 0 {
		var kb strings.Builder
		kb.WriteString("\n\n=== ADMIN PLAYBOOK (retrieved facts — align answers; do not contradict; stay concise) ===\n")
		for _, s := range ragCtx.FAQSnippets {
			kb.WriteString(strings.TrimSpace(s))
			kb.WriteString("\n---\n")
		}
		sys += kb.String()
	}
	// Live market listings (admin-scraped) — lets the AI reason over real
	// current prices/comparables and cite the source URL.
	sys += retrieveScrapedMarketBlock(ctx, s.gdb, msgCtx)
	// Administrative-procedure questions: strict grounding so the AI answers
	// from real scraped official data (or admits it lacks it) — never invents
	// steps/fees/URLs, which would read as a scam to users.
	pgBlock, pgGrounded, pgURLs, pgVerified := procedureGroundingBlock(ctx, s.gdb, msgCtx)
	sys += pgBlock

	msgs := []client.Message{{Role: "system", Content: sys}}
	msgs = append(msgs, sanitizeHistoryForLLM(in.History)...)
	msgs = append(msgs, client.Message{Role: "user", Content: in.Text})

	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	resp, err := s.or.Chat(ctx, msgs)
	if err != nil {
		fmt.Printf("❌ MeskenyGPT OpenRouter error: %v\n", err)
		api := response.APIErrorOutput(msgCtx)
		return ChatOutput{Message: api.Message, SessionID: in.SessionID}, nil
	}
	finalContent := resp.Content
	if in.DeepThink {
		if refined := s.refineWithSelfReview(ctx, msgCtx, msgs, resp.Content); strings.TrimSpace(refined) != "" {
			finalContent = refined
		}
	}
	// Guarantee the trust markers in code (the LLM can't be trusted to add the
	// ⚠️ disclaimer / source citation reliably).
	finalContent = EnforceProcedureHonesty(msgCtx, finalContent, pgGrounded, pgURLs, pgVerified)

	elapsed := time.Since(start).Milliseconds()
	msg := response.Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: finalContent,
	}
	msg.Content = enforceNoCardsResponseIntegrity(msgCtx, msg.Content)
	msg.Content = enforceMeskenyIdentity(msgCtx, msg.Content)
	qr := response.GenerateQuickReplies(msgCtx, msg.Content)

	interactionID := s.recordTurn(ctx, capture.TurnPathLLM, in.SessionID, in.UserID,
		len(in.History)/2, msgCtx, in.Text, msg.Content, -1, elapsed)

	return s.withEscalation(ctx, in, ChatOutput{
		Message:       msg,
		QuickReplies:  qr,
		SessionID:     in.SessionID,
		InteractionID: interactionID,
	}), nil
}

func (s *service) GetGreeting(ctx context.Context, l lang.Lang) (ChatOutput, error) {
	msg := response.GreetingMessage(l)
	qr := response.GreetingQuickReplies(l)
	return ChatOutput{
		Message:      msg,
		QuickReplies: qr,
	}, nil
}

// summarisePropertiesWithModel sends the DB-backed cards to OpenRouter with
// a strict prompt: the model MUST only describe these properties and MUST NOT
// invent new listings. If the call fails, it falls back to a safe fixed text.
func (s *service) summarisePropertiesWithModel(
	ctx context.Context,
	msgCtx lang.MessageContext,
	userText string,
	cards []property.Card,
) string {
	isSharedPropertyMode := strings.Contains(userText, "Property context (shared from app card):")

	// Build a plain-text list of properties for the model to reason on.
	var b strings.Builder
	for i, c := range cards {
		fmt.Fprintf(&b, "%d) ID=%d; العنوان=%s; السعر=%.0f %s; المدينة=%s; الغرف=%d;\n",
			i+1, c.ID, c.Title, c.Price, c.Currency, c.City, c.Bedrooms)
	}

	// Language-specific instructions, but always the same hard rules:
	// - Talk ONLY about these properties
	// - Do NOT invent new listings or prices
	// - Be concise and helpful
	var sys string
	switch msgCtx.Lang {
	case lang.LangAR:
		sys = "أنت MeskenyGPT، مساعد عقاري ذكي لتطبيق Meskeny في موريتانيا.\n" +
			"يُسمح لك فقط بالحديث عن العقارات الموجودة في القائمة أدناه، ولا يحق لك اختراع أي عقار جديد أو سعر جديد.\n" +
			"لا تبدأ الجواب بتحيات مثل: مرحباً، أهلاً، مرحباً بكم.\n" +
			"اشرح للمستخدم بإيجاز (من 2 إلى 4 جمل) ما هي أفضل الخيارات له ولماذا، باستخدام اللغة العربية فقط.\n" +
			"لا تذكر أي معرّفات داخلية مثل رقم ID للعقار.\n" +
			"لا تستخدم جداول Markdown بعلامة |، استخدم نقاط واضحة وعناوين قصيرة.\n" +
			"ذكِّره أنه يمكنه الضغط على البطاقات في التطبيق لعرض التفاصيل الكاملة والصور."
		if isSharedPropertyMode {
			sys += "\nالوضع الحالي: المستخدم شارك عقاراً محدداً ويريد تقييمه، وليس بحثاً عاماً.\n" +
				"ركّز أولاً على تقييم هذا العقار بالذات، ثم استخدم العقارات الأخرى فقط كمقارنات.\n" +
				"لا تبدأ الجواب بعبارة مثل: وجدت X عقارات.\n" +
				"لا تقل أن العقار على الشاطئ إلا إذا كان ذلك مذكوراً صراحةً في نفس العقار المستهدف."
		}
	case lang.LangEN:
		sys = "You are MeskenyGPT, a real-estate assistant for Meskeny in Mauritania.\n" +
			"You MUST only talk about the properties listed below and MUST NOT invent any new listings, prices, or locations.\n" +
			"Do NOT start with greetings like 'Hello', 'Hi', 'Bonjour', or 'Salut'.\n" +
			"Give a short summary (2–4 sentences) of the best options and why they might fit the user. Reply only in English.\n" +
			"Do not mention internal IDs (like property ID). Use title/location wording only.\n" +
			"Do not output markdown pipe tables. Use concise bullets and labeled sections instead.\n" +
			"Remind the user they can tap the cards in the app to open full details and photos."
		if isSharedPropertyMode {
			sys += "\nCurrent mode: user shared one specific listing and wants valuation/comments for THAT listing.\n" +
				"Prioritize valuation of the shared subject listing first; use other listings only as comparables.\n" +
				"Do NOT start with wording like 'I found X properties'.\n" +
				"Do NOT claim 'beachfront' unless that is explicitly stated for the subject listing itself."
		}
	default: // French
		sys = "Tu es MeskenyGPT, assistant immobilier pour l'application Meskeny en Mauritanie.\n" +
			"Tu DOIS parler uniquement des biens listés ci-dessous et tu n'as PAS le droit d'inventer d'autres annonces, prix ou lieux.\n" +
			"Ne commence PAS par des salutations comme 'Bonjour', 'Salut', ou 'Hi'.\n" +
			"Donne un court résumé (2–4 phrases) des meilleures options et pourquoi elles peuvent convenir. Réponds uniquement en français.\n" +
			"Ne mentionne jamais les identifiants internes (ex: ID du bien).\n" +
			"N'utilise pas de tableau Markdown avec | ; préfère des puces claires avec des sections courtes.\n" +
			"Rappelle à l'utilisateur qu'il peut appuyer sur les cartes dans l'application pour voir les détails complets et les photos."
		if isSharedPropertyMode {
			sys += "\nMode actuel : l'utilisateur a partagé un bien précis et veut son estimation/commentaire, pas une recherche générale.\n" +
				"Commence par analyser la valeur du bien cible ; utilise les autres biens uniquement comme comparables.\n" +
				"Ne commence PAS par 'J'ai trouvé X biens'.\n" +
				"N'affirme pas 'bord de mer' pour le bien cible sauf mention explicite dans ses données."
		}
	}

	propsText := "PROPRIÉTÉS DISPONIBLES / AVAILABLE PROPERTIES / العقارات المتاحة:\n" + b.String()

	msgs := []client.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: userText},
		{Role: "system", Content: propsText},
	}

	ctx2, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	resp, err := s.or.Chat(ctx2, msgs)
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		// Fallback to fixed summary if model fails.
		if isSharedPropertyMode && len(cards) > 0 {
			subject := cards[0]
			switch msgCtx.Lang {
			case lang.LangAR:
				return fmt.Sprintf("تقييم أولي للعقار \"%s\": السعر الحالي %.0f %s. " +
					"تمت مقارنة العقار بعقارات قريبة في نفس الفئة لدعم التقدير، ولم تُسجَّل عروض مؤكدة لهذا العقار حالياً. " +
					"هذا يعطي تقديراً سوقياً مبدئياً قريباً من سعر العرض مع هامش تفاوض محدود.", subject.Title, subject.Price, subject.Currency)
			case lang.LangEN:
				return fmt.Sprintf("Initial valuation for \"%s\": current asking price is %.0f %s. "+
					"The estimate is based on nearby comparable listings in the same segment, and there are currently no confirmed offers recorded for this listing. "+
					"This supports a market value close to the asking price with a limited negotiation margin.", subject.Title, subject.Price, subject.Currency)
			default:
				return fmt.Sprintf("Estimation initiale du bien \"%s\" : prix affiché %.0f %s. "+
					"L'évaluation s'appuie sur des comparables proches du même segment, et aucune offre confirmée n'est enregistrée pour ce bien à ce stade. "+
					"Cela suggère une valeur de marché proche du prix demandé avec une marge de négociation limitée.", subject.Title, subject.Price, subject.Currency)
			}
		}
		with := response.WithCardsOutput(msgCtx, cards)
		return with.Message.Content
	}

	return resp.Content
}

func (s *service) refineWithSelfReview(
	ctx context.Context,
	msgCtx lang.MessageContext,
	baseMessages []client.Message,
	draft string,
) string {
	if strings.TrimSpace(draft) == "" {
		return ""
	}
	reviewInstr := "Review the previous assistant draft. Identify weak points, missing constraints, and any unclear wording. Return only concise improvement notes."
	finalInstr := "Rewrite the answer using those notes. Keep the same language as the user, be concrete and practical, avoid fluff, and do not mention this review process."
	if msgCtx.Lang == lang.LangAR {
		reviewInstr = "راجع مسودة المساعد السابقة. استخرج نقاط الضعف، القيود الناقصة، وأي غموض. أعد فقط ملاحظات تحسين مختصرة."
		finalInstr = "أعد صياغة الجواب اعتماداً على تلك الملاحظات. التزم بلغة المستخدم، كن عملياً وواضحاً، وتجنب الحشو، ولا تذكر عملية المراجعة."
	} else if msgCtx.Lang == lang.LangFR {
		reviewInstr = "Relis le brouillon précédent. Identifie les points faibles, contraintes manquantes et passages flous. Retourne uniquement des notes d'amélioration concises."
		finalInstr = "Réécris la réponse avec ces notes. Garde la langue de l'utilisateur, reste concret et pratique, évite le bla-bla, et ne mentionne pas cette relecture."
	}

	reviewMsgs := append([]client.Message{}, baseMessages...)
	reviewMsgs = append(reviewMsgs,
		client.Message{Role: "assistant", Content: draft},
		client.Message{Role: "user", Content: reviewInstr},
	)
	reviewResp, err := s.or.Chat(ctx, reviewMsgs)
	if err != nil || strings.TrimSpace(reviewResp.Content) == "" {
		return ""
	}

	finalMsgs := append([]client.Message{}, baseMessages...)
	finalMsgs = append(finalMsgs,
		client.Message{Role: "assistant", Content: draft},
		client.Message{Role: "system", Content: "Improvement notes:\n" + strings.TrimSpace(reviewResp.Content)},
		client.Message{Role: "user", Content: finalInstr},
	)
	finalResp, err := s.or.Chat(ctx, finalMsgs)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(finalResp.Content)
}

// buildAgentConversationalSystemPrompt configures Muskini / Meskeny AI for natural multi-turn dialogue.
func buildAgentConversationalSystemPrompt(msgCtx lang.MessageContext, hasHistory bool) string {
	// Administrative-procedure questions get a DEDICATED advisor persona — not
	// the property-search persona below, which would (wrongly) ask the user for
	// rent-vs-buy / city / budget on a national procedure.
	if msgCtx.Intent == lang.IntentInfoProcedure {
		return buildProcedureAdvisorSystemPrompt()
	}

	var b strings.Builder
	b.WriteString("You are Muskini (Meskeny AI), a conversational real-estate expert for the Meskeny app in Mauritania. ")
	b.WriteString("Behave like a smart local agent, not a scripted bot: be warm, concise, and proactive. ")
	b.WriteString("Match the user's language—Arabic (including Hassaniya-style phrasing when natural), French, English, Simplified Chinese when the user writes in Chinese, or mixed code-switching—mirror their style briefly when they mix languages. ")
	b.WriteString("Extract intent from messy messages; ask at most one or two focused follow-ups when needed (e.g. city, rent vs buy, budget, property type). ")
	b.WriteString("Never invent specific listings, prices, or availability. If they need homes, say you can search real Meskeny listings once you know city/budget/type. ")
	b.WriteString("Identity rule: you are MeskenyGPT in the Meskeny app. Never claim to be OpenAI, ChatGPT, Claude, Gemini, or any external assistant/provider, even if the user asks. ")
	b.WriteString("Do not give legal advice or negotiate on behalf of sellers; suggest viewing the listing or contacting an agent for contracts. ")
	b.WriteString("Never expose internal database or property IDs; refer to listings by title or neighborhood only. ")
	b.WriteString("Avoid markdown pipe tables; use short bullets if needed. ")
	if hasHistory {
		b.WriteString("You have prior messages in this session: stay consistent, build on what was said, and do not repeat the same canned greeting every time. ")
	} else {
		b.WriteString("If the user only says hi/hello/salam, reply with one short warm line and ask how you can help with housing in Mauritania. ")
	}
	// Light intent hint for the model (does not replace user text).
	switch msgCtx.Intent {
	case lang.IntentGreeting:
		b.WriteString("(User message looks like a greeting.) ")
	case lang.IntentHelp:
		b.WriteString("(User may want general help navigating Meskeny.) ")
	default:
	}
	return b.String()
}

// buildProcedureAdvisorSystemPrompt is the persona for real-estate
// ADMINISTRATIVE-PROCEDURE questions (ownership transfer, title registration,
// documents, fees, cadastre, permits). It must be genuinely helpful and never
// behave like the property-search assistant.
func buildProcedureAdvisorSystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are Muskini (Meskeny AI), an expert guide to real-estate ADMINISTRATIVE PROCEDURES in Mauritania — ownership transfer, title registration (تحفيظ), required documents, fees, cadastre, and permits. ")
	b.WriteString("The user is asking HOW to complete an official procedure. This is NOT a property search. ")
	b.WriteString("ABSOLUTELY DO NOT ask about rent vs buy, budget, city, sector, or property type — those are irrelevant to a national procedure, and asking them makes you look broken. Never ask for search filters. Never say you need to know the city/budget/type. ")
	b.WriteString("Answer DIRECTLY, confidently and completely in the user's language, as clear ordered steps. You MAY use one compact markdown table for the documents/fees and short numbered steps. ")
	b.WriteString("If OFFICIAL PROCEDURE DATA is provided below, base the answer on it and cite its exact source URLs. ")
	b.WriteString("If NO official data is provided, still give the standard general steps that apply in Mauritania — typically: (1) gather identity documents (CNI) for both parties plus the existing ownership title/deed; (2) draft the transfer/sale/gift contract before a notary «كاتب العدل»; (3) pay the registration/mutation duties; (4) file the transfer at the land-registry office «المحافظة العقارية / مصلحة التسجيل العقاري»; (5) collect the updated title in the new owner's name. ")
	b.WriteString("When you rely on general steps (no official data), say once, briefly, that exact fees/timelines should be confirmed with the authority — but STILL give the full helpful steps; do NOT deflect, do NOT reply that 'the sources don't contain it'. ")
	b.WriteString("NEVER invent specific fee percentages, exact office addresses, article numbers, or URLs. Never fabricate a procedures.gov.mr link. ")
	b.WriteString("End by offering to help further — e.g. connect them with a Meskeny specialist or a notary, or help once they have their documents ready. ")
	b.WriteString("Identity rule: you are MeskenyGPT in the Meskeny app. Never claim to be OpenAI, ChatGPT, Claude, Gemini, or any external assistant/provider. ")
	return b.String()
}

// sanitizeHistoryForLLM keeps only user/assistant turns for the chat API (max 20 messages).
func sanitizeHistoryForLLM(h []ChatMessage) []client.Message {
	if len(h) == 0 {
		return nil
	}
	const maxMessages = 20
	start := 0
	if len(h) > maxMessages {
		start = len(h) - maxMessages
	}
	out := make([]client.Message, 0, maxMessages)
	for _, m := range h[start:] {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		out = append(out, client.Message{Role: role, Content: content})
	}
	return out
}

func lastUserLangFromHistory(h []ChatMessage) (lang.Lang, bool) {
	for i := len(h) - 1; i >= 0; i-- {
		if strings.ToLower(strings.TrimSpace(h[i].Role)) != "user" {
			continue
		}
		content := strings.TrimSpace(h[i].Content)
		if content == "" || strings.Contains(content, "[MESKENY_PICKER]") {
			continue
		}
		return lang.DetectLang(content), true
	}
	return 0, false
}

func (s *service) withEscalation(ctx context.Context, in ChatInput, out ChatOutput) ChatOutput {
	if s.escalation == nil {
		return out
	}
	msgs := make([]escalation.Message, 0, len(in.History)+1)
	for _, h := range in.History {
		msgs = append(msgs, escalation.Message{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, escalation.Message{Role: "user", Content: in.Text})
	trig := s.escalation.Evaluate(ctx, msgs)
	if trig == nil {
		return out
	}
	var uid *uint
	if in.UserID > 0 {
		uid = &in.UserID
	}
	row, err := s.escalation.Execute(ctx, trig, in.SessionID, uid)
	if err != nil {
		return out
	}
	out.Escalation = &EscalationInfo{
		ID:      row.ID,
		Status:  row.Status,
		Urgency: row.Urgency,
		Reason:  row.Reason,
	}
	return out
}
