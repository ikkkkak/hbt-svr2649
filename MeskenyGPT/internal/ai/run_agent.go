package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/capture"
	"apartments-clone-server/MeskenyGPT/internal/ai/agent"
	"apartments-clone-server/MeskenyGPT/internal/ai/client"
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/property"
	"apartments-clone-server/MeskenyGPT/internal/ai/response"
	"apartments-clone-server/MeskenyGPT/internal/ai/rules"
)

func (s *service) HandleAgentRun(
	ctx context.Context,
	in AgentRunInput,
	emit func(AgentEvent),
) (ChatOutput, error) {
	runStart := time.Now()
	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	persona := strings.TrimSpace(in.Persona)
	if persona == "" {
		persona = "buyer"
	}
	tier := strings.TrimSpace(in.Tier)
	if tier == "" {
		tier = "free"
	}

	emit(AgentEvent{
		Type:    AgentEventRunStarted,
		RunID:   runID,
		Persona: persona,
		Tier:    tier,
	})

	if strings.TrimSpace(in.Text) == "" {
		emit(AgentEvent{Type: AgentEventStepError, RunID: runID, StepID: "understand", Error: "empty message"})
		return ChatOutput{}, fmt.Errorf("empty message")
	}

	// ── Step: understand ─────────────────────────────────────────────────────
	t0 := time.Now()
	emit(AgentEvent{
		Type:   AgentEventStepStart,
		RunID:  runID,
		StepID: "understand",
		Label:  stepLabel(lang.LangFR, "understand"),
	})

	if blocked, reason := s.guard.Check(in.Text); blocked {
		blk := response.BlockedOutput(reason)
		emit(AgentEvent{
			Type:    AgentEventBlocked,
			RunID:   runID,
			Message: blk.Message,
			Blocked: true,
		})
		return ChatOutput{Message: blk.Message}, nil
	}

	msgCtx := lang.AnalyzeMessage(in.Text)
	msgCtx.RawText = strings.TrimSpace(in.Text)
	histTurns := make([]lang.HistoryTurn, 0, len(in.History))
	for _, h := range in.History {
		histTurns = append(histTurns, lang.HistoryTurn{Role: h.Role, Content: h.Content})
	}
	msgCtx = lang.EnrichContextFromHistory(msgCtx, histTurns, in.Text)
	msgCtx = s.hydrateSessionFilters(ctx, in.SessionID, msgCtx)
	if strings.Contains(in.Text, "[MESKENY_PICKER]") {
		if histLang, ok := lastUserLangFromHistory(in.History); ok {
			msgCtx.Lang = histLang
		}
	}
	sharedPropertyMode := isSharedPropertyRequest(in.Text)
	valuationMode := sharedPropertyMode && isValuationIntentText(in.Text)
	agentRole := agent.RouteRole(msgCtx, in.Text)
	s.persistSessionFilters(ctx, in.SessionID, msgCtx)

	clarifyGate := lang.ShouldClarifyBeforeSearch(msgCtx)
	runPath := agent.ResolvePath(agentRole, msgCtx, clarifyGate)
	stepPlan := agent.BuildStepPlan(agentRole, msgCtx, runPath)

	emit(AgentEvent{
		Type:   AgentEventStepDone,
		RunID:  runID,
		StepID: "understand",
		MS:     time.Since(t0).Milliseconds(),
		Label:  planLabel(stepPlan, "understand", msgCtx.Lang),
		Detail: map[string]any{
			"lang":   langCode(msgCtx.Lang),
			"intent": intentCode(msgCtx.Intent),
			"type":   msgCtx.Type,
			"city":   msgCtx.City,
			"zone":   msgCtx.Zone,
			"role":   agentRole,
		},
	})
	emit(AgentEvent{
		Type:    AgentEventStreamStart,
		RunID:   runID,
		Role:    agentRole,
		Lang:    langCode(msgCtx.Lang),
		RTL:     msgCtx.Lang == lang.LangAR,
		Persona: persona,
		Tier:    tier,
	})

	emitStepPlan(emit, runID, stepPlan)

	// ── Step: plan ───────────────────────────────────────────────────────────
	tPlan := time.Now()
	emit(AgentEvent{
		Type:   AgentEventStepStart,
		RunID:  runID,
		StepID: "plan",
		Label:  planLabel(stepPlan, "plan", msgCtx.Lang),
	})
	planDetail := map[string]any{"persona": persona, "role": agentRole}
	if persona == "broker" && tier == "free" {
		planDetail["broker_note"] = "portfolio_tools_require_pro"
	}
	emit(AgentEvent{
		Type:   AgentEventStepDone,
		RunID:  runID,
		StepID: "plan",
		MS:     time.Since(tPlan).Milliseconds(),
		Label:  planLabel(stepPlan, "plan", msgCtx.Lang),
		Detail: planDetail,
	})

	if persona == "broker" {
		if isPortfolioRequest(in.Text) {
			return s.agentRunBrokerPortfolio(ctx, in, emit, runID, runStart, msgCtx, tier)
		}
		if isMarketingPackRequest(in.Text) {
			return s.agentRunBrokerMarketing(ctx, in, emit, runID, runStart, msgCtx, tier)
		}
		if !isBrokerProTier(tier) {
			emit(AgentEvent{
				Type: AgentEventToolCall,
				RunID: runID, Tool: "broker_pro",
				Args: map[string]any{"status": "locked", "upgrade": "pro"},
			})
		}
	}

	// Greeting / help — instant reply (no 30s LLM wait); skip for shared property / valuation
	if (msgCtx.Intent == lang.IntentGreeting || msgCtx.Intent == lang.IntentHelp) && !sharedPropertyMode {
		msg := response.GreetingMessage(msgCtx.Lang)
		out := ChatOutput{
			Message:      msg,
			QuickReplies: response.GreetingQuickReplies(msgCtx.Lang),
			SessionID:    in.SessionID,
		}
		out.InteractionID = s.recordTurn(ctx, capture.TurnPathGreeting, in.SessionID, in.UserID,
			len(in.History)/2, msgCtx, in.Text, msg.Content, -1, time.Since(runStart).Milliseconds())
		emitVerificationWithPlan(emit, runID, msgCtx, in.Text, true, 0.96,
			[]string{"Instant greeting — no listing search"}, nil, stepPlan)
		emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
		return out, nil
	}

	if sharedPropertyMode && valuationMode {
		return s.agentRunPropertySearch(ctx, in, emit, runID, runStart, msgCtx, stepPlan, valuationMode)
	}

	// Clarify before any empty geo search (enterprise: no city= zone= → 0 rows)
	if lang.ShouldClarifyBeforeSearch(msgCtx) {
		cl := response.ProactiveClarificationOutput(msgCtx)
		out := ChatOutput{
			Message:      cl.Message,
			QuickReplies: cl.QuickReplies,
			SessionID:    in.SessionID,
		}
		r := lang.EvaluateSearchReadiness(msgCtx)
		gaps := []string{}
		if r.MissingLocation {
			gaps = append(gaps, "city_or_zone")
		}
		if r.MissingPurpose {
			gaps = append(gaps, "rent_or_buy")
		}
		out.InteractionID = s.recordTurn(ctx, capture.TurnPathClarify, in.SessionID, in.UserID,
			len(in.History)/2, msgCtx, in.Text, cl.Message.Content, -1, time.Since(runStart).Milliseconds())
		emitVerificationWithPlan(emit, runID, msgCtx, in.Text, true, 0.88,
			[]string{"Search blocked until location is known", "Only real Meskeny listings after filters are set"},
			gaps, stepPlan)
		emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
		return out, nil
	}

	// ── Property search (DB-backed) ──────────────────────────────────────────
	if lang.IsPropertySearchIntent(msgCtx.Intent) {
		return s.agentRunPropertySearch(ctx, in, emit, runID, runStart, msgCtx, stepPlan, valuationMode)
	}

	// Follow-up on cached cards
	if msgCtx.Intent == lang.IntentUnknown && looksLikeSummariseLastResultsQuery(msgCtx.Lang, in.Text) {
		if last := s.loadLastCards(ctx, in.SessionID); len(last) > 0 {
			tG := time.Now()
			emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "gather", Label: planLabel(stepPlan, "gather", msgCtx.Lang)})
			emit(AgentEvent{
				Type: AgentEventToolCall, RunID: runID, Tool: "session_cards",
				Args: map[string]any{"count": len(last)},
			})
			emit(AgentEvent{
				Type: AgentEventStepDone, RunID: runID, StepID: "gather",
				MS: time.Since(tG).Milliseconds(), Detail: map[string]any{"count": len(last)},
			})
			msg := response.Message{ID: fmt.Sprintf("msg_%d", time.Now().UnixNano()), Role: "assistant"}
			msg.Content = s.summarisePropertiesWithModel(ctx, msgCtx, in.Text, last)
			out := ChatOutput{
				Message:                 msg,
				PropertyRecommendations: last,
				QuickReplies:            response.SearchResultsFollowUpQuickReplies(msgCtx),
				SessionID:               in.SessionID,
			}
			out.InteractionID = s.recordTurn(ctx, capture.TurnPathFollowUp, in.SessionID, in.UserID,
				len(in.History)/2, msgCtx, in.Text, msg.Content, len(last), time.Since(runStart).Milliseconds())
			emitVerificationWithPlan(emit, runID, msgCtx, in.Text, true, 0.88, nil, nil, stepPlan)
			emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
			return out, nil
		}
	}

	// ── Conversational + RAG ─────────────────────────────────────────────────
	return s.agentRunConversational(ctx, in, emit, runID, runStart, msgCtx, stepPlan)
}

func (s *service) agentRunPropertySearch(
	ctx context.Context,
	in AgentRunInput,
	emit func(AgentEvent),
	runID string,
	runStart time.Time,
	msgCtx lang.MessageContext,
	stepPlan []agent.Step,
	valuationMode bool,
) (ChatOutput, error) {
	tG := time.Now()
	emit(AgentEvent{
		Type:   AgentEventStepStart,
		RunID:  runID,
		StepID: "gather",
		Label:  planLabel(stepPlan, "gather", msgCtx.Lang),
	})

	rules.ApplyAdminSearchRules(s.gdb, &msgCtx)
	f := property.FiltersFromContext(msgCtx)
	f.Query = in.Text
	property.EnrichFiltersFromCatalog(s.gdb, &f)
	toolName := "search_properties"
	if msgCtx.Intent == lang.IntentSearchLand {
		toolName = "search_landmarks"
	}
	emit(AgentEvent{
		Type: AgentEventToolCall,
		RunID: runID, Tool: toolName,
		Args: map[string]any{
			"city": msgCtx.City, "zone": msgCtx.Zone, "type": msgCtx.Type,
			"budget_min": f.BudgetMin, "budget_max": f.BudgetMax,
			"purpose": f.Purpose,
		},
	})

	var props []property.Property
	var err error
	if msgCtx.Intent == lang.IntentSearchLand {
		props, err = s.props.FindLandmarks(ctx, f)
	} else {
		props, err = s.props.Find(ctx, f)
	}
	if err != nil {
		emit(AgentEvent{
			Type: AgentEventStepError, RunID: runID, StepID: "gather",
			Error: err.Error(),
		})
	}
	cards := property.ToCards(props)
	s.cacheLastCards(ctx, in.SessionID, cards)

	emit(AgentEvent{
		Type:   AgentEventStepDone,
		RunID:  runID,
		StepID: "gather",
		MS:     time.Since(tG).Milliseconds(),
		Detail: map[string]any{"count": len(cards)},
	})

	if len(cards) == 0 {
		if valuationMode {
			msg := response.Message{ID: fmt.Sprintf("msg_%d", time.Now().UnixNano()), Role: "assistant"}
			switch msgCtx.Lang {
			case lang.LangAR:
				msg.Content = "لا أستطيع تقديم تقييم سوقي موثوق لهذا العقار الآن لأن المقارنات غير كافية في البيانات الحالية."
			case lang.LangEN:
				msg.Content = "I can’t produce a reliable market valuation yet because comparables in this segment are insufficient."
			default:
				msg.Content = "Je ne peux pas fournir d'estimation fiable pour le moment : comparables insuffisants."
			}
			out := ChatOutput{Message: msg, SessionID: in.SessionID}
			out.InteractionID = s.recordTurn(ctx, capture.TurnPathNoResults, in.SessionID, in.UserID,
				len(in.History)/2, msgCtx, in.Text, msg.Content, 0, time.Since(runStart).Milliseconds())
			emitVerificationWithPlan(emit, runID, msgCtx, in.Text, true, 0.7,
				[]string{"No comparables in database"}, []string{"Add city or budget"}, stepPlan)
			emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
			return out, nil
		}
		no := response.NoResultsOutput(msgCtx)
		out := ChatOutput{Message: no.Message, QuickReplies: no.QuickReplies, SessionID: in.SessionID}
		out.InteractionID = s.recordTurn(ctx, capture.TurnPathNoResults, in.SessionID, in.UserID,
			len(in.History)/2, msgCtx, in.Text, no.Message.Content, 0, time.Since(runStart).Milliseconds())
		emitVerificationWithPlan(emit, runID, msgCtx, in.Text, true, 0.82,
			[]string{"Zero listings matched filters — suggestions are data-grounded"},
			[]string{"Try quick replies to adjust budget or zone"}, stepPlan)
		emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
		return out, nil
	}

	// analyze
	tA := time.Now()
	emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "analyze", Label: planLabel(stepPlan, "analyze", msgCtx.Lang)})
	with := response.WithCardsOutput(msgCtx, cards)
	msg := with.Message
	if valuationMode {
		msg.Content = s.summarisePropertiesWithModel(ctx, msgCtx, in.Text, cards)
	}
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	emit(AgentEvent{
		Type: AgentEventStepDone, RunID: runID, StepID: "analyze",
		MS: time.Since(tA).Milliseconds(),
		Detail: map[string]any{"listings": len(cards), "valuation": valuationMode},
	})

	emitVerificationWithPlan(emit, runID, msgCtx, in.Text, true, 0.92,
		[]string{"Results from Meskeny database only"}, nil, stepPlan)

	elapsed := time.Since(runStart).Milliseconds()
	out := ChatOutput{
		Message:                 msg,
		PropertyRecommendations: with.PropertyRecommendations,
		QuickReplies:            response.SearchResultsFollowUpQuickReplies(msgCtx),
		SessionID:               in.SessionID,
	}
	out.InteractionID = s.recordTurn(ctx, capture.TurnPathDBSearch, in.SessionID, in.UserID,
		len(in.History)/2, msgCtx, in.Text, msg.Content, len(cards), elapsed)
	emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
	return out, nil
}

func (s *service) agentRunConversational(
	ctx context.Context,
	in AgentRunInput,
	emit func(AgentEvent),
	runID string,
	runStart time.Time,
	msgCtx lang.MessageContext,
	stepPlan []agent.Step,
) (ChatOutput, error) {
	tG := time.Now()
	emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "gather", Label: planLabel(stepPlan, "gather", msgCtx.Lang)})
	ragCtx, _ := s.retriever.Retrieve(ctx, msgCtx)
	emit(AgentEvent{
		Type: AgentEventToolCall, RunID: runID, Tool: "rag_retrieve",
		Args: map[string]any{"has_market": ragCtx.MarketSummary != "", "faq_snippets": len(ragCtx.FAQSnippets)},
	})
	emit(AgentEvent{
		Type: AgentEventStepDone, RunID: runID, StepID: "gather",
		MS: time.Since(tG).Milliseconds(),
	})

	tA := time.Now()
	emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "analyze", Label: planLabel(stepPlan, "analyze", msgCtx.Lang)})

	sys := buildAgentConversationalSystemPrompt(msgCtx, len(in.History) > 0)
	if ragCtx.MarketSummary != "" {
		sys += "\n\n=== MAURITANIA CONTEXT ===\n" + ragCtx.MarketSummary
	}
	for _, n := range ragCtx.Notes {
		sys += "\n- " + n
	}
	if len(ragCtx.FAQSnippets) > 0 {
		sys += "\n\n=== ADMIN PLAYBOOK ===\n"
		for _, snip := range ragCtx.FAQSnippets {
			sys += strings.TrimSpace(snip) + "\n---\n"
		}
	}
	// Scraped ministry/cadastre/market knowledge — lets the agent ANSWER
	// procedure questions (ownership transfer, titles, documents, fees) from
	// real government pages and cite them, instead of guessing.
	sys += retrieveScrapedMarketBlock(ctx, s.gdb, msgCtx)

	msgs := []client.Message{{Role: "system", Content: sys}}
	msgs = append(msgs, sanitizeHistoryForLLM(in.History)...)
	msgs = append(msgs, client.Message{Role: "user", Content: in.Text})

	ctx2, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	streamDeltas := !in.DeepThink
	resp, err := s.or.ChatStream(ctx2, msgs, func(delta string) error {
		if streamDeltas {
			emit(AgentEvent{Type: AgentEventTextDelta, RunID: runID, Delta: delta})
		}
		return nil
	})
	if err != nil {
		emit(AgentEvent{Type: AgentEventStepError, RunID: runID, StepID: "analyze", Error: err.Error()})
		api := response.APIErrorOutput(msgCtx)
		out := ChatOutput{Message: api.Message, SessionID: in.SessionID}
		emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
		return out, nil
	}
	draft := resp

	emit(AgentEvent{
		Type: AgentEventStepDone, RunID: runID, StepID: "analyze",
		MS: time.Since(tA).Milliseconds(),
		Detail: map[string]any{"chars": len(draft)},
	})

	finalContent := draft
	if in.DeepThink {
		tDeep := time.Now()
		deepLabel := deepThinkStepLabel(msgCtx.Lang)
		emit(AgentEvent{
			Type:   AgentEventStepStart,
			RunID:  runID,
			StepID: "deep_review",
			Label:  deepLabel,
		})
		if refined := s.refineWithSelfReview(ctx2, msgCtx, msgs, draft); strings.TrimSpace(refined) != "" {
			finalContent = refined
		}
		emit(AgentEvent{
			Type:   AgentEventStepDone,
			RunID:  runID,
			StepID: "deep_review",
			MS:     time.Since(tDeep).Milliseconds(),
		})
	}

	finalContent = enforceNoCardsResponseIntegrity(msgCtx, finalContent)
	finalContent = enforceMeskenyIdentity(msgCtx, finalContent)

	matches, conf, assumptions, gaps := verifyConversational(msgCtx, in.Text, finalContent)
	emitVerificationWithPlan(emit, runID, msgCtx, in.Text, matches, conf, assumptions, gaps, stepPlan)

	msg := response.Message{
		ID: fmt.Sprintf("msg_%d", time.Now().UnixNano()), Role: "assistant", Content: finalContent,
	}
	qr := response.GenerateQuickReplies(msgCtx, msg.Content)
	elapsed := time.Since(runStart).Milliseconds()

	interactionID := s.recordTurn(ctx, capture.TurnPathLLM, in.SessionID, in.UserID,
		len(in.History)/2, msgCtx, in.Text, msg.Content, -1, elapsed)

	out := ChatOutput{
		Message: msg, QuickReplies: qr, SessionID: in.SessionID, InteractionID: interactionID,
	}
	emitFinal(emit, runID, out, runStart, stepPlan, msgCtx.Lang)
	return out, nil
}

func verifyConversational(msgCtx lang.MessageContext, userText, answer string) (matches bool, confidence float64, assumptions, gaps []string) {
	matches = true
	confidence = 0.82
	assumptions = []string{"Answer based on Meskeny knowledge, not live listings unless search was run"}
	if lang.IsPropertySearchIntent(msgCtx.Intent) {
		assumptions = append(assumptions, "User may expect listing cards — suggest using search filters")
	}
	if looksLikeFoundResultsClaim(answer) && !lang.IsPropertySearchIntent(msgCtx.Intent) {
		matches = false
		confidence = 0.45
		gaps = append(gaps, "Response claims listings without a database search")
	}
	if containsHan(userText) || containsHan(answer) {
		assumptions = append(assumptions, "Chinese script detected — full zh playbook is P1")
	}
	if strings.Contains(strings.ToLower(userText), "registry") ||
		strings.Contains(strings.ToLower(userText), "ministry") ||
		strings.Contains(userText, "تيتر") {
		assumptions = append(assumptions, "Land registry API not connected — use paper_types on listings only")
		gaps = append(gaps, "Cannot verify title with government database in this version")
	}
	return matches, confidence, assumptions, gaps
}

func emitVerification(emit func(AgentEvent), runID string, msgCtx lang.MessageContext, userText string, matches bool, conf float64, assumptions, gaps []string) {
	emitVerificationWithPlan(emit, runID, msgCtx, userText, matches, conf, assumptions, gaps, nil)
}

func emitVerificationWithPlan(emit func(AgentEvent), runID string, msgCtx lang.MessageContext, userText string, matches bool, conf float64, assumptions, gaps []string, stepPlan []agent.Step) {
	tV := time.Now()
	verifyLabel := stepLabel(msgCtx.Lang, "verify")
	if len(stepPlan) > 0 {
		verifyLabel = planLabel(stepPlan, "verify", msgCtx.Lang)
	}
	emit(AgentEvent{
		Type:   AgentEventStepStart,
		RunID:  runID,
		StepID: "verify",
		Label:  verifyLabel,
	})
	emit(AgentEvent{
		Type:          AgentEventVerification,
		RunID:         runID,
		MatchesIntent: matches,
		Confidence:    conf,
		Assumptions:   assumptions,
		Gaps:          gaps,
	})
	emit(AgentEvent{
		Type:   AgentEventStepDone,
		RunID:  runID,
		StepID: "verify",
		MS:     time.Since(tV).Milliseconds(),
	})
}

func emitStepPlan(emit func(AgentEvent), runID string, plan []agent.Step) {
	items := make([]StepPlanItem, len(plan))
	for i, s := range plan {
		items[i] = StepPlanItem{ID: s.ID, Label: s.Label}
	}
	emit(AgentEvent{Type: AgentEventStepPlan, RunID: runID, Steps: items})
}

func planLabel(plan []agent.Step, stepID string, l lang.Lang) string {
	for _, s := range plan {
		if s.ID == stepID {
			return s.Label
		}
	}
	return stepLabel(l, stepID)
}

func emitFollowUps(emit func(AgentEvent), runID string, qr []response.QuickReply) {
	if len(qr) == 0 {
		return
	}
	questions := make([]string, 0, len(qr))
	for _, q := range qr {
		action := strings.TrimSpace(q.Action)
		if action == "" || strings.HasPrefix(action, "picker_") {
			continue
		}
		questions = append(questions, action)
	}
	if len(questions) == 0 {
		return
	}
	emit(AgentEvent{
		Type:      AgentEventFollowUps,
		RunID:     runID,
		FollowUps: questions,
	})
}

func emitFinal(emit func(AgentEvent), runID string, out ChatOutput, runStart time.Time, stepPlan []agent.Step, l lang.Lang) {
	deliverLabel := planLabel(stepPlan, "deliver", l)
	emit(AgentEvent{
		Type:   AgentEventStepStart,
		RunID:  runID,
		StepID: "deliver",
		Label:  deliverLabel,
	})
	emit(AgentEvent{
		Type:                    AgentEventFinal,
		RunID:                   runID,
		Message:                 out.Message,
		PropertyRecommendations: cardsToAny(out.PropertyRecommendations),
		QuickReplies:            out.QuickReplies,
		SessionID:               out.SessionID,
		InteractionID:           out.InteractionID,
	})
	emitFollowUps(emit, runID, out.QuickReplies)
	emit(AgentEvent{
		Type:    AgentEventStepDone,
		RunID:   runID,
		StepID:  "deliver",
		TotalMS: time.Since(runStart).Milliseconds(),
	})
	emit(AgentEvent{
		Type:    AgentEventRunComplete,
		RunID:   runID,
		TotalMS: time.Since(runStart).Milliseconds(),
	})
}

func cardsToAny(cards []property.Card) []any {
	if len(cards) == 0 {
		return nil
	}
	out := make([]any, len(cards))
	for i, c := range cards {
		out[i] = c
	}
	return out
}

func containsHan(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func langCode(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "ar"
	case lang.LangEN:
		return "en"
	case lang.LangZH:
		return "zh"
	default:
		return "fr"
	}
}

func intentCode(i lang.Intent) string {
	switch i {
	case lang.IntentSearchRent:
		return "search_rent"
	case lang.IntentSearchBuy:
		return "search_buy"
	case lang.IntentSearchLand:
		return "search_land"
	case lang.IntentGreeting:
		return "greeting"
	case lang.IntentHelp:
		return "help"
	case lang.IntentInfoProcedure:
		return "info_procedure"
	default:
		return "unknown"
	}
}

func deepThinkStepLabel(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "مراجعة عميقة وتحسين الإجابة"
	case lang.LangEN:
		return "Deep review and refinement"
	case lang.LangZH:
		return "深度审阅与优化"
	default:
		return "Relecture approfondie"
	}
}

func stepLabel(l lang.Lang, step string) string {
	labels := map[string]map[lang.Lang]string{
		"understand": {
			lang.LangAR: "فهم طلبك",
			lang.LangEN: "Understanding your request",
			lang.LangFR: "Comprendre votre demande",
			lang.LangZH: "理解您的需求",
		},
		"plan": {
			lang.LangAR: "تخطيط الخطوات",
			lang.LangEN: "Planning next steps",
			lang.LangFR: "Planifier les étapes",
			lang.LangZH: "规划处理步骤",
		},
		"gather": {
			lang.LangAR: "جمع البيانات من Meskeny",
			lang.LangEN: "Gathering Meskeny data",
			lang.LangFR: "Collecte des données Meskeny",
			lang.LangZH: "从 Meskeny 获取数据",
		},
		"analyze": {
			lang.LangAR: "تحليل النتائج",
			lang.LangEN: "Analyzing results",
			lang.LangFR: "Analyse des résultats",
			lang.LangZH: "分析结果",
		},
		"verify": {
			lang.LangAR: "التحقق من ملاءمة الإجابة",
			lang.LangEN: "Checking answer matches your question",
			lang.LangFR: "Vérifier la pertinence de la réponse",
			lang.LangZH: "核对回答是否符合您的问题",
		},
		"deliver": {
			lang.LangAR: "تقديم النتيجة",
			lang.LangEN: "Delivering answer",
			lang.LangFR: "Présentation de la réponse",
			lang.LangZH: "呈现结果",
		},
	}
	if m, ok := labels[step]; ok {
		if s, ok2 := m[l]; ok2 {
			return s
		}
		return m[lang.LangFR]
	}
	return step
}
