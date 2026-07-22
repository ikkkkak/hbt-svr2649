// package services

// import (
// 	"bytes"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"net/http"
// 	"os"
// 	"regexp"
// 	"strings"
// 	"time"
// )

// // OpenRouter chat message
// type ORMessage struct {
// 	Role    string `json:"role"`
// 	Content string `json:"content"`
// }

// // OpenRouter chat request payload
// type ORChatRequest struct {
// 	Model       string      `json:"model"`
// 	Messages    []ORMessage `json:"messages"`
// 	MaxTokens   int         `json:"max_tokens,omitempty"`
// 	Temperature float64     `json:"temperature,omitempty"`
// 	Stream      bool        `json:"stream"`
// }

// // OpenRouter chat response payload
// type ORChatResponse struct {
// 	Choices []struct {
// 		Message struct {
// 			Role     string `json:"role"`
// 			Content  string `json:"content"`
// 			Thinking string `json:"thinking,omitempty"`
// 		} `json:"message"`
// 	} `json:"choices"`
// }

// // AIService handles AI-related operations using OpenRouter models
// type AIService struct {
// 	client  *http.Client
// 	apiKey  string
// 	model   string
// 	badWords []string
// }

// // NewAIService creates a new AI service instance backed by OpenRouter
// func NewAIService() *AIService {
// 	apiKey := ""
// 	if apiKey == "" {
// 		fmt.Println("⚠️ OPENROUTER_API_KEY not set – Meskeny AI will not be able to call OpenRouter")
// 	}

// 	model := os.Getenv("OPENROUTER_MODEL")
// 	if model == "" {
// 		// Default to a capable free model; can be overridden via env.
// 		model = "openai/gpt-oss-120b"
// 	}

// 	return &AIService{
// 		client: &http.Client{
// 			Timeout: 90 * time.Second,
// 		},
// 		apiKey: apiKey,
// 		model:  model,
// 		badWords: []string{
// 			// English
// 			"fuck", "shit", "damn", "bitch", "ass", "bastard", "crap", "dick", "pussy",
// 			"whore", "slut", "nigger", "faggot", "retard",
// 			// French
// 			"merde", "putain", "connard", "salope", "encule", "bordel", "nique",
// 			// Arabic transliterated
// 			"kos", "sharmouta", "ibn el", "ya kalb", "ya hmar",
// 		},
// 	}
// }

// // GetSystemPrompt returns the system prompt for Meskeny AI
// func (s *AIService) GetSystemPrompt() string {
// 	return `You are Meskeny AI, a professional and friendly AI assistant for the Meskeny real estate app in Mauritania.

// Your identity:
// - You are Meskeny AI, NOT ChatGPT, Claude, Gemini, or any other AI provider
// - You help users find properties for rent or sale in Mauritania
// - You are knowledgeable about Mauritanian cities and neighborhoods
// - You speak French, Arabic, and English fluently
// - You are warm, helpful, and professional

// Your capabilities:
// - Help users find properties based on their preferences (location, price, size, type)
// - Provide information about cities and zones in Mauritania
// - Answer questions about the rental/buying process
// - Give tips for property searching
// - Explain property features and amenities

// Important rules:
// - Always be respectful and professional
// - If asked who you are, say you are "Meskeny AI", the intelligent assistant for Meskeny app
// - Never claim to be from another AI company
// - Focus on helping users find their ideal property
// - For each user message, FIRST give a clear, helpful answer or set of property suggestions. Only THEN, if it is really necessary, ask at most ONE short follow‑up question.
// - NEVER ask again for information that the user already provided in the last messages (city, neighborhood, budget, bedrooms, etc.). Use what they said and move forward.
// - If the user writes something like "regardless of price" or "no budget limit", you MUST treat the budget as open and you MUST NOT ask again about budget.
// - If the user clearly specifies a city and/or neighborhood (for example: "Nouakchott Tevragh Zeina"), you MUST NOT ask again "in which city?" or "where?" for that query.
// - Do not repeat the same question multiple times in a row. If the user does not answer, move on and give them your best suggestions with the information you already have.
// - If you don't know something specific about a property, encourage users to contact the owner
// - When recommending properties, ask about: budget, location preference, number of bedrooms, property type

// Examples of correct behavior:
// - User: "Find me a house for sale regardless of price and bedrooms in Nouakchott Tevragh Zeina."
//   Assistant: You immediately suggest 2–5 matching houses in that area (with prices, bedrooms, key features) and then optionally ask ONE refinement like "Do you prefer a quiet street or something closer to shops?". You DO NOT ask which city, neighborhood, budget, or bedrooms again.

// Available cities in Mauritania:
// - Nouakchott (capital, largest city)
// - Nouadhibou (second largest economical city, coastal)
// - Kaédi, Zouérate, Rosso, Atar, Kiffa, Néma, Sélibaby, Aioun el Atrouss

// Property types available:
// - Apartments (Appartement)
// - Houses (Maison)
// - Villas
// - Land (Terrain)
// - Commercial spaces (Boutique, Local commercial)
// - Offices

// Always respond in the same language the user writes in. Be concise but helpful.`
// }

// // ContainsBadWords checks if a message contains inappropriate language
// func (s *AIService) ContainsBadWords(message string) bool {
// 	lowerMessage := strings.ToLower(message)
// 	for _, word := range s.badWords {
// 		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(word))
// 		matched, _ := regexp.MatchString(pattern, lowerMessage)
// 		if matched {
// 			return true
// 		}
// 	}
// 	return false
// }

// // SendMessage sends a message to the OpenRouter chat completions API and returns the response
// func (s *AIService) SendMessage(userMessage string, conversationHistory []map[string]string, deepThink bool) (string, error) {
// 	// Build conversation for OpenRouter:
// 	// - system prompt (Meskeny AI instructions)
// 	// - prior messages (user/assistant)
// 	// - current user message
// 	messages := []ORMessage{
// 		{
// 			Role:    "system",
// 			Content: s.GetSystemPrompt(),
// 		},
// 	}

// 	for _, msg := range conversationHistory {
// 		role := "user"
// 		if msg["role"] == "assistant" {
// 			role = "assistant"
// 		}
// 		messages = append(messages, ORMessage{
// 			Role:    role,
// 			Content: msg["content"],
// 		})
// 	}

// 	messages = append(messages, ORMessage{
// 		Role:    "user",
// 		Content: userMessage,
// 	})

// 	maxTokens := 256
// 	if deepThink {
// 		maxTokens = 768
// 	}

// 	reqPayload := ORChatRequest{
// 		Model:       s.model,
// 		Messages:    messages,
// 		MaxTokens:   maxTokens,
// 		Temperature: 0.5,
// 		Stream:      false,
// 	}

// 	requestBody, err := json.Marshal(reqPayload)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to marshal request: %w", err)
// 	}

// 	url := "https://openrouter.ai/api/v1/chat/completions"
// 	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
// 	if err != nil {
// 		return "", fmt.Errorf("failed to create request: %w", err)
// 	}
// 	httpReq.Header.Set("Content-Type", "application/json")
// 	if s.apiKey != "" {
// 		httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
// 	}
// 	// Optional: tell OpenRouter this is Meskeny AI; safe to omit referer.
// 	httpReq.Header.Set("X-Title", "Meskeny AI")

// 	if deepThink {
// 		// Enable thinking mode where supported, but never return raw chain-of-thought.
// 		prefs := map[string]any{
// 			"thinking": map[string]any{
// 				"enabled":       true,
// 				"budget_tokens": 256,
// 			},
// 		}
// 		if b, err := json.Marshal(prefs); err == nil {
// 			httpReq.Header.Set("X-OpenRouter-Preferences", string(b))
// 		}
// 	}

// 	resp, err := s.client.Do(httpReq)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to send request to OpenRouter: %w", err)
// 	}
// 	defer resp.Body.Close()

// 	// Read full body so we can log details and handle edge cases.
// 	bodyBytes, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to read OpenRouter response body: %w", err)
// 	}

// 	// Helper: detect if user wrote in Arabic to localize fallback messages.
// 	// We avoid regex here to keep it simple and fast.
// 	isArabic := false
// 	for _, r := range userMessage {
// 		if r >= 0x0600 && r <= 0x06FF {
// 			isArabic = true
// 			break
// 		}
// 	}

// 	if resp.StatusCode != http.StatusOK {
// 		fmt.Printf("❌ OpenRouter API Error: Status %d, Body: %s\n", resp.StatusCode, string(bodyBytes))
// 		if isArabic {
// 			return "عذراً، واجهنا مشكلة أثناء الاتصال بنموذج الذكاء الاصطناعي. جرّب أن تعيد صياغة سؤالك أو أرسله بشكل أبسط، وسأحاول مساعدتك من جديد.", nil
// 		}
// 		return "Je suis désolé, un problème est survenu en contactant le modèle. Peux-tu reformuler ta demande plus simplement et réessayer ?", nil
// 	}

// 	if len(bodyBytes) == 0 {
// 		fmt.Println("⚠️ OpenRouter: Empty response body")
// 		if isArabic {
// 			return "عذراً، لم أستطع معالجة طلبك لأن النموذج أعاد استجابة فارغة. لخص لي باختصار (في ٢–٣ جمل) ما الذي تريد من Meskeny AI أن يفعله الآن وسأساعدك خطوة بخطوة.", nil
// 		}
// 		return "Je suis désolé, je n'ai pas pu traiter ta demande (réponse vide du modèle). Peux-tu la résumer en 2‑3 phrases simples, par exemple : ce que tu veux que Meskeny AI fasse pour toi maintenant ?", nil
// 	}

// 	var orResp ORChatResponse
// 	if err := json.Unmarshal(bodyBytes, &orResp); err != nil {
// 		fmt.Printf("❌ OpenRouter: Failed to decode JSON response: %v\nRaw body: %s\n", err, string(bodyBytes))
// 		if isArabic {
// 			return "عذراً، لم أستطع فهم استجابة النموذج بشكل صحيح. جرّب أن تعيد صياغة طلبك ببعض الجمل البسيطة والواضحة.", nil
// 		}
// 		return "Je suis désolé, je n'ai pas pu bien comprendre la réponse du modèle. Peux-tu reformuler ta demande en quelques phrases plus simples ?", nil
// 	}

// 	if len(orResp.Choices) > 0 {
// 		msg := orResp.Choices[0].Message
// 		if msg.Content != "" {
// 			// We intentionally ignore msg.Thinking to avoid exposing chain-of-thought.
// 			fmt.Printf("✅ OpenRouter: Got response (%d chars)\n", len(msg.Content))
// 			return msg.Content, nil
// 		}
// 	}

// 	fmt.Printf("⚠️ OpenRouter: Empty response content field. Raw body: %s\n", string(bodyBytes))

// 	if isArabic {
// 		return "عذراً، لم أستطع معالجة طلبك كما هو (النموذج أعاد استجابة فارغة). لخص لي ببساطة ما الذي تريد من Meskeny AI أن يقدمه لك الآن وسأساعدك خطوة بخطوة.", nil
// 	}
// 	return "Je suis désolé, je n'ai pas pu traiter ta demande telle quelle (le modèle a renvoyé une réponse vide). Résume simplement ce que tu attends de Meskeny AI maintenant et je vais t'aider étape par étape.", nil
// }

// // GenerateQuickReplies generates contextual quick replies based on conversation
// func (s *AIService) GenerateQuickReplies(userMessage, aiResponse string) []map[string]string {
// 	lowerMessage := strings.ToLower(userMessage)
// 	lowerResponse := strings.ToLower(aiResponse)

// 	// Property search context
// 	if strings.Contains(lowerMessage, "cherche") || strings.Contains(lowerMessage, "looking") || strings.Contains(lowerMessage, "find") {
// 		return []map[string]string{
// 			{"id": "1", "text": "🏠 Appartement", "action": "search_apartment"},
// 			{"id": "2", "text": "🏡 Maison", "action": "search_house"},
// 			{"id": "3", "text": "🏢 Boutique", "action": "search_commercial"},
// 			{"id": "4", "text": "📍 Terrain", "action": "search_land"},
// 		}
// 	}

// 	// Location context
// 	if strings.Contains(lowerMessage, "où") || strings.Contains(lowerMessage, "where") || strings.Contains(lowerMessage, "location") {
// 		return []map[string]string{
// 			{"id": "1", "text": "📍 Nouakchott", "action": "location_nouakchott"},
// 			{"id": "2", "text": "📍 Nouadhibou", "action": "location_nouadhibou"},
// 			{"id": "3", "text": "📍 Autre ville", "action": "location_other"},
// 		}
// 	}

// 	// Budget context
// 	if strings.Contains(lowerMessage, "prix") || strings.Contains(lowerMessage, "budget") || strings.Contains(lowerMessage, "price") {
// 		return []map[string]string{
// 			{"id": "1", "text": "💰 < 100,000 MRU", "action": "budget_low"},
// 			{"id": "2", "text": "💰 100k - 300k MRU", "action": "budget_medium"},
// 			{"id": "3", "text": "💰 > 300,000 MRU", "action": "budget_high"},
// 		}
// 	}

// 	// Purpose context
// 	if strings.Contains(lowerResponse, "louer") || strings.Contains(lowerResponse, "acheter") {
// 		return []map[string]string{
// 			{"id": "1", "text": "🔑 À louer", "action": "purpose_rent"},
// 			{"id": "2", "text": "🏷️ À acheter", "action": "purpose_buy"},
// 		}
// 	}

// 	// Default helpful actions
// 	return []map[string]string{
// 		{"id": "1", "text": "🔍 Chercher propriété", "action": "search_property"},
// 		{"id": "2", "text": "📞 Contacter support", "action": "contact_support"},
// 		{"id": "3", "text": "❓ Aide", "action": "help"},
// 	}
// }

// // GetInitialGreeting returns the initial greeting message
// func (s *AIService) GetInitialGreeting() (string, []map[string]string) {
// 	greeting := `Bonjour! 👋 Je suis **Meskeny AI**, votre assistant immobilier intelligent.

// Je peux vous aider à:
// • 🏠 Trouver un appartement ou une maison
// • 🏢 Chercher un local commercial
// • 📍 Explorer les quartiers
// • 💰 Comparer les prix

// Comment puis-je vous aider aujourd'hui?`

// 	quickReplies := []map[string]string{
// 		{"id": "1", "text": "🏠 Chercher à louer", "action": "search_rent"},
// 		{"id": "2", "text": "🏷️ Chercher à acheter", "action": "search_buy"},
// 		{"id": "3", "text": "📍 Explorer Nouakchott", "action": "explore_nouakchott"},
// 		{"id": "4", "text": "❓ Comment ça marche?", "action": "how_it_works"},
// 	}

// 	return greeting, quickReplies
// }

// // GenerateSessionTitle generates a title from the first user message
// func (s *AIService) GenerateSessionTitle(message string) string {
// 	if len(message) > 40 {
// 		return message[:40] + "..."
// 	}
// 	return message
// }

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ─────────────────────────────────────────────────────────────────────────────
// Wire types (OpenRouter)
// ─────────────────────────────────────────────────────────────────────────────

type ORMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ORChatRequest struct {
	Model       string      `json:"model"`
	Messages    []ORMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	Stream      bool        `json:"stream"`
}

type ORChatResponse struct {
	Choices []struct {
		Message struct {
			Role     string `json:"role"`
			Content  string `json:"content"`
			Thinking string `json:"thinking,omitempty"`
		} `json:"message"`
	} `json:"choices"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Language enum
// ─────────────────────────────────────────────────────────────────────────────

type Lang int

const (
	LangFR Lang = iota // French   (default)
	LangAR             // Arabic
	LangEN             // English
	LangHASSANIYA      // Hassaniya (Arabic dialect)
)

// ─────────────────────────────────────────────────────────────────────────────
// Intent enum — every distinct user intent we handle
// ─────────────────────────────────────────────────────────────────────────────

type Intent int

const (
	// Property search
	IntentSearchRent          Intent = iota // user wants to rent
	IntentSearchBuy                         // user wants to buy
	IntentSearchAny                         // rent or buy not yet specified
	IntentSearchLand                        // looking for terrain/land
	IntentSearchCommercial                  // boutique / local commercial / office
	IntentSearchByBudget                    // user leads with price
	IntentSearchByLocation                  // user leads with city/neighbourhood
	IntentSearchByRooms                     // user leads with bedroom count
	IntentSearchByType                      // user specifies property type first

	// Information requests
	IntentInfoCity          // ask about a city / neighbourhood
	IntentInfoProcess       // how does renting/buying work
	IntentInfoPrices        // general price levels
	IntentInfoNeighbourhood // which area is best

	// Contact & support
	IntentContactOwner   // wants to reach a property owner
	IntentContactSupport // needs human support

	// Conversational
	IntentGreeting    // hello, hi, bonjour…
	IntentThanks      // thank you
	IntentComplaint   // frustration or negative feedback
	IntentRepeat      // "say that again" / "repeat"
	IntentMore        // "show me more" / "autres résultats"
	IntentRefine      // user adds a filter to a previous search
	IntentReset       // "start over" / "recommencer"
	IntentHelp        // what can you do
	IntentHowItWorks  // how does the app work
	IntentOffTopic    // totally unrelated to real estate
	IntentInappropriate // bad words / abuse
	IntentUnknown     // fallback
)

// ─────────────────────────────────────────────────────────────────────────────
// Detected context extracted from a single message
// ─────────────────────────────────────────────────────────────────────────────

type MessageContext struct {
	Lang        Lang
	Intent      Intent
	Cities      []string
	Zones       []string  // neighbourhood / quartier
	Budget      string    // raw budget string found in text
	Rooms       string    // "3" / "2 chambres"
	PropertyType string   // normalized type label
	Purpose     string    // "rent" | "buy" | ""
	NoBudgetCap bool      // user said "no limit" / "sans limite"
	IsQuestion  bool      // ends with ? or interrogative
}

// ─────────────────────────────────────────────────────────────────────────────
// Localised strings
// ─────────────────────────────────────────────────────────────────────────────

type l10n struct {
	// Fallback messages
	APIError     string
	EmptyBody    string
	DecodeError  string
	EmptyContent string

	// Quick reply labels
	QRApartment  string
	QRHouse      string
	QRVilla      string
	QRLand       string
	QRCommercial string
	QRRent       string
	QRBuy        string
	QRNouakchott string
	QRNouadhibou string
	QROtherCity  string
	QRBudgetLow  string
	QRBudgetMid  string
	QRBudgetHigh string
	QRNoLimit    string
	QRMore       string
	QRReset      string
	QRHelp       string
	QRContact    string
}

var strings_FR = l10n{
	APIError:     "Je suis désolé, un problème est survenu en contactant le modèle. Peux-tu reformuler ta demande simplement et réessayer ?",
	EmptyBody:    "Je n'ai pas pu traiter ta demande (réponse vide). Résume en 2–3 phrases ce que tu attends de Meskeny AI et je t'aiderai étape par étape.",
	DecodeError:  "Je n'ai pas pu comprendre la réponse du modèle. Reformule ta demande en quelques phrases simples.",
	EmptyContent: "Je n'ai pas pu traiter ta demande telle quelle. Résume simplement ce que tu attends de Meskeny AI.",
	QRApartment:  "🏠 Appartement",
	QRHouse:      "🏡 Maison",
	QRVilla:      "🏛️ Villa",
	QRLand:       "📍 Terrain",
	QRCommercial: "🏢 Local commercial",
	QRRent:       "🔑 À louer",
	QRBuy:        "🏷️ À acheter",
	QRNouakchott: "📍 Nouakchott",
	QRNouadhibou: "📍 Nouadhibou",
	QROtherCity:  "🗺️ Autre ville",
	QRBudgetLow:  "💰 < 50k MRU",
	QRBudgetMid:  "💰 50k–200k MRU",
	QRBudgetHigh: "💰 > 200k MRU",
	QRNoLimit:    "♾️ Sans limite",
	QRMore:       "📋 Voir plus",
	QRReset:      "🔄 Recommencer",
	QRHelp:       "❓ Aide",
	QRContact:    "📞 Contacter le support",
}

var strings_AR = l10n{
	APIError:     "عذراً، واجهنا مشكلة أثناء الاتصال بالنموذج. جرّب أن تعيد صياغة سؤالك بشكل أبسط.",
	EmptyBody:    "عذراً، لم أستطع معالجة طلبك (استجابة فارغة). لخّص لي ما تريده في ٢–٣ جمل وسأساعدك.",
	DecodeError:  "لم أستطع فهم استجابة النموذج. أعد صياغة طلبك ببعض الجمل البسيطة.",
	EmptyContent: "لم أستطع معالجة طلبك. لخّص ببساطة ما تريد من Meskeny AI.",
	QRApartment:  "🏠 شقة",
	QRHouse:      "🏡 منزل",
	QRVilla:      "🏛️ فيلا",
	QRLand:       "📍 أرض",
	QRCommercial: "🏢 محل تجاري",
	QRRent:       "🔑 للإيجار",
	QRBuy:        "🏷️ للبيع",
	QRNouakchott: "📍 نواكشوط",
	QRNouadhibou: "📍 نواذيبو",
	QROtherCity:  "🗺️ مدينة أخرى",
	QRBudgetLow:  "💰 أقل من 50k MRU",
	QRBudgetMid:  "💰 50k–200k MRU",
	QRBudgetHigh: "💰 أكثر من 200k MRU",
	QRNoLimit:    "♾️ بدون حد",
	QRMore:       "📋 المزيد",
	QRReset:      "🔄 ابدأ من جديد",
	QRHelp:       "❓ مساعدة",
	QRContact:    "📞 التواصل مع الدعم",
}

var strings_EN = l10n{
	APIError:     "Sorry, there was a problem reaching the AI model. Please rephrase your request and try again.",
	EmptyBody:    "Sorry, I couldn't process your request (empty response). Summarize what you need in 2–3 sentences and I'll help you step by step.",
	DecodeError:  "I couldn't understand the model's response. Please rephrase your request in a few simple sentences.",
	EmptyContent: "I couldn't process your request. Summarize simply what you expect from Meskeny AI.",
	QRApartment:  "🏠 Apartment",
	QRHouse:      "🏡 House",
	QRVilla:      "🏛️ Villa",
	QRLand:       "📍 Land",
	QRCommercial: "🏢 Commercial space",
	QRRent:       "🔑 To rent",
	QRBuy:        "🏷️ To buy",
	QRNouakchott: "📍 Nouakchott",
	QRNouadhibou: "📍 Nouadhibou",
	QROtherCity:  "🗺️ Other city",
	QRBudgetLow:  "💰 < 50k MRU",
	QRBudgetMid:  "💰 50k–200k MRU",
	QRBudgetHigh: "💰 > 200k MRU",
	QRNoLimit:    "♾️ No limit",
	QRMore:       "📋 Show more",
	QRReset:      "🔄 Start over",
	QRHelp:       "❓ Help",
	QRContact:    "📞 Contact support",
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyword maps — all lowercased, normalised (no diacritics needed for matching
// because we normalise the input before matching)
// ─────────────────────────────────────────────────────────────────────────────

// Mauritanian cities (canonical lowercase)
var mauritaniaCities = []string{
	"nouakchott", "nouadhibou", "kaedi", "zouerate", "rosso",
	"atar", "kiffa", "nema", "selibaby", "aioun", "tidjikja",
	"akjoujt", "boutilimit", "guerou", "aleg", "boghe",
}

// Nouakchott neighbourhoods
var nouakchottZones = []string{
	"tevragh zeina", "ksar", "el mina", "toujounine", "dar naim",
	"riyad", "sebkha", "arafat", "teyarett", "capital",
	"pk", "pk5", "pk6", "pk10", "pk12",
	"ile maurice", "socogim",
}

// Property types — value is the normalised label we forward to the model
var propertyTypeKeywords = map[string]string{
	// FR
	"appartement": "Appartement", "appart": "Appartement",
	"studio": "Studio",
	"maison": "Maison", "villa": "Villa",
	"terrain": "Terrain", "parcelle": "Terrain",
	"boutique": "Boutique", "local": "Local commercial",
	"bureau": "Bureau", "office": "Bureau",
	"chambre": "Chambre",
	// AR
	"شقة": "Appartement", "شقه": "Appartement",
	"فيلا": "Villa", "منزل": "Maison", "بيت": "Maison",
	"ارض": "Terrain", "أرض": "Terrain",
	"محل": "Boutique", "مكتب": "Bureau",
	// EN
	"apartment": "Appartement", "flat": "Appartement",
	"house": "Maison", "land": "Terrain",
	"commercial": "Local commercial", "shop": "Boutique",
}

// Rent intent keywords
var rentKW = []string{
	// FR
	"louer", "location", "à louer", "en location", "mensuel", "mois",
	"bail",
	// AR
	"إيجار", "ايجار", "للإيجار", "للايجار", "أجار",
	// EN
	"rent", "rental", "lease", "monthly",
}

// Buy intent keywords
var buyKW = []string{
	// FR
	"acheter", "achat", "acquérir", "acquisition", "vente", "à vendre",
	"propriétaire", "investissement",
	// AR
	"شراء", "للبيع", "بيع", "تملك", "امتلاك",
	// EN
	"buy", "purchase", "sale", "invest", "ownership",
}

// No-budget-cap phrases
var noBudgetCapKW = []string{
	// FR
	"sans limite", "peu importe le prix", "peu importe", "n'importe quel prix",
	"pas de budget", "indépendamment du prix", "quel que soit le prix",
	"regardless of price", "regardless of budget", "no budget", "no limit",
	// AR
	"بغض النظر عن السعر", "بغض النظر", "مهما كان السعر", "لا يهم السعر",
	"بدون حد", "بدون سقف",
}

// Greeting keywords
var greetingKW = []string{
	"bonjour", "bonsoir", "salut", "coucou", "hello", "hi", "hey",
	"salam", "ahlan", "مرحبا", "السلام عليكم", "أهلا",
}

// Thanks keywords
var thanksKW = []string{
	"merci", "thank", "شكرا", "شكراً", "thanks",
}

// Reset keywords
var resetKW = []string{
	"recommencer", "reset", "nouveau", "début", "start over",
	"من البداية", "ابدأ", "إعادة",
}

// More results keywords
var moreKW = []string{
	"plus", "encore", "autres", "suite", "more", "show more", "suivant",
	"المزيد", "أكثر", "التالي",
}

// Help keywords
var helpKW = []string{
	"aide", "help", "comment", "quoi", "que peux", "que pouvez",
	"what can", "مساعدة", "كيف", "ماذا",
}

// Off-topic topics — things clearly outside real estate
var offTopicKW = []string{
	"météo", "weather", "recette", "cuisine", "football", "sport",
	"politique", "politics", "film", "musique", "music", "médecin",
	"docteur", "doctor", "visa", "passeport", "passport",
}

// firstNonEmpty returns the first non-empty string from the list.
// Used to try multiple env-var names for the API key.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// AIService
// ─────────────────────────────────────────────────────────────────────────────

type AIService struct {
	client   *http.Client
	apiKey   string
	model    string
	badWords []string
}

func NewAIService() *AIService {
	// Try multiple common env-var names so the service works regardless of
	// how the key was exported on the host (Render, Railway, bare VPS, etc.)
	apiKey := firstNonEmpty(
		os.Getenv("OPENROUTER_API_KEY"),
		os.Getenv("OPENROUTER_KEY"),
		os.Getenv("OR_API_KEY"),
	)
	if apiKey == "" {
		fmt.Println("🚨 CRITICAL: No OpenRouter API key found.")
		fmt.Println("   Set one of: OPENROUTER_API_KEY | OPENROUTER_KEY | OR_API_KEY")
		fmt.Println("   Example:  export OPENROUTER_API_KEY=<your-key>")
	} else {
		// Log only the prefix so we can confirm the right key loaded without
		// exposing the full secret in logs.
		visible := apiKey
		if len(apiKey) > 16 {
			visible = apiKey[:12] + "…" + apiKey[len(apiKey)-4:]
		}
		fmt.Printf("✅ OpenRouter API key loaded: %s\n", visible)
	}
	
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "openai/gpt-4o-mini" // fast, cheap, multilingual
	}

	return &AIService{
		client: &http.Client{Timeout: 90 * time.Second},
		apiKey: apiKey,
		model:  model,
		badWords: []string{
			// EN
			"fuck", "shit", "bitch", "ass", "bastard", "crap", "dick",
			"pussy", "whore", "slut", "nigger", "faggot", "retard",
			// FR
			"merde", "putain", "connard", "salope", "encule", "nique",
			// AR transliterated
			"kos", "sharmouta", "ya kalb", "ya hmar",
			// AR script
			"كس", "عاهرة", "كلب",
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Language detection
// ─────────────────────────────────────────────────────────────────────────────

// DetectLang infers the dominant language from a message.
// Priority: Arabic script → French keywords → English keywords → French (default).
func DetectLang(msg string) Lang {
	// Arabic Unicode block: U+0600–U+06FF
	arabicCount := 0
	latinCount := 0
	for _, r := range msg {
		if r >= 0x0600 && r <= 0x06FF {
			arabicCount++
		} else if unicode.IsLetter(r) && r < 0x0250 {
			latinCount++
		}
	}
	if arabicCount > 0 && arabicCount >= latinCount/2 {
		return LangAR
	}

	lower := strings.ToLower(msg)
	frWords := []string{"je", "tu", "il", "nous", "vous", "le", "la", "les",
		"un", "une", "des", "est", "cherche", "veux", "louer", "acheter"}
	enWords := []string{"i ", "you", "the", "is", "are", "want", "looking", "find", "need"}

	frScore, enScore := 0, 0
	for _, w := range frWords {
		if strings.Contains(lower, " "+w+" ") || strings.HasPrefix(lower, w+" ") {
			frScore++
		}
	}
	for _, w := range enWords {
		if strings.Contains(lower, " "+w+" ") || strings.HasPrefix(lower, w) {
			enScore++
		}
	}
	if enScore > frScore {
		return LangEN
	}
	return LangFR
}

// l returns the localised string bundle for a given language.
func (s *AIService) l(lang Lang) l10n {
	switch lang {
	case LangAR, LangHASSANIYA:
		return strings_AR
	case LangEN:
		return strings_EN
	default:
		return strings_FR
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Text normalisation helpers
// ─────────────────────────────────────────────────────────────────────────────

// normalize lowercases and strips common diacritics so keyword matching
// works regardless of accent usage (é→e, è→e, à→a, etc.)
func normalize(s string) string {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer(
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"à", "a", "â", "a", "ä", "a",
		"ù", "u", "û", "u", "ü", "u",
		"î", "i", "ï", "i",
		"ô", "o", "ö", "o",
		"ç", "c",
	)
	return replacer.Replace(s)
}

// containsAny returns true if text contains any of the given keywords.
func containsAny(text string, keywords []string) bool {
	norm := normalize(text)
	for _, kw := range keywords {
		if strings.Contains(norm, normalize(kw)) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Intent & context detection — the brain of the service
// ─────────────────────────────────────────────────────────────────────────────

// AnalyzeMessage parses a user message and returns a rich MessageContext.
// This is the single source of truth for everything downstream: quick replies,
// token budget, model temperature, and prompt injection.
func AnalyzeMessage(msg string) MessageContext {
	ctx := MessageContext{
		Lang:   DetectLang(msg),
		Intent: IntentUnknown,
	}
	norm := normalize(msg)

	// ── Bad words ──────────────────────────────────────────────────────────
	// (checked separately in ContainsBadWords, but flag here too)

	// ── No-budget-cap ──────────────────────────────────────────────────────
	if containsAny(msg, noBudgetCapKW) {
		ctx.NoBudgetCap = true
	}

	// ── Cities ─────────────────────────────────────────────────────────────
	for _, city := range mauritaniaCities {
		if strings.Contains(norm, city) {
			ctx.Cities = append(ctx.Cities, city)
		}
	}
	for _, zone := range nouakchottZones {
		if strings.Contains(norm, zone) {
			ctx.Zones = append(ctx.Zones, zone)
		}
	}

	// ── Property type ──────────────────────────────────────────────────────
	for kw, label := range propertyTypeKeywords {
		if strings.Contains(msg, kw) || strings.Contains(norm, normalize(kw)) {
			ctx.PropertyType = label
			break
		}
	}

	// ── Purpose ────────────────────────────────────────────────────────────
	isRent := containsAny(msg, rentKW)
	isBuy := containsAny(msg, buyKW)
	if isRent && !isBuy {
		ctx.Purpose = "rent"
	} else if isBuy && !isRent {
		ctx.Purpose = "buy"
	}

	// ── Budget extraction (simple: find a number followed by k/MRU/UM) ────
	budgetRe := regexp.MustCompile(`(?i)(\d[\d\s,.]*)(\s*)(k|mru|um|000)`)
	if m := budgetRe.FindString(msg); m != "" {
		ctx.Budget = strings.TrimSpace(m)
	}

	// ── Room count extraction ───────────────────────────────────────────────
	roomRe := regexp.MustCompile(`(\d+)\s*(?:chambres?|pièces?|bedrooms?|غرف|غرفة|rooms?)`)
	if m := roomRe.FindStringSubmatch(msg); len(m) > 1 {
		ctx.Rooms = m[1]
	}

	// ── Question detection ─────────────────────────────────────────────────
	ctx.IsQuestion = strings.Contains(msg, "?") ||
		containsAny(msg, []string{"comment", "combien", "where", "how", "what", "quel",
			"quelle", "كيف", "أين", "متى", "ما هو"})

	// ── Intent classification (ordered by specificity) ─────────────────────

	switch {
	// Conversational first
	case containsAny(msg, greetingKW) && len(strings.Fields(msg)) < 6:
		ctx.Intent = IntentGreeting

	case containsAny(msg, thanksKW):
		ctx.Intent = IntentThanks

	case containsAny(msg, resetKW):
		ctx.Intent = IntentReset

	case containsAny(msg, moreKW) &&
		!containsAny(msg, append(rentKW, buyKW...)):
		ctx.Intent = IntentMore

	case containsAny(msg, helpKW) && len(strings.Fields(msg)) < 8:
		ctx.Intent = IntentHelp

	case containsAny(msg, offTopicKW) &&
		!containsAny(msg, rentKW) && !containsAny(msg, buyKW):
		ctx.Intent = IntentOffTopic

	// Contact
	case containsAny(msg, []string{"contact", "appel", "call", "email", "whatsapp", "téléphone"}):
		ctx.Intent = IntentContactOwner

	case containsAny(msg, []string{"support", "aide humain", "agent", "conseiller"}):
		ctx.Intent = IntentContactSupport

	// Information
	case containsAny(msg, []string{"info", "renseign", "comment fonctionne", "how does", "كيف يعمل",
		"process", "procédure", "étapes", "steps"}):
		ctx.Intent = IntentInfoProcess

	case len(ctx.Cities) > 0 && !isRent && !isBuy && ctx.PropertyType == "" && ctx.Budget == "":
		ctx.Intent = IntentInfoCity

	case containsAny(msg, []string{"prix", "price", "tarif", "combien coûte", "how much", "سعر", "تكلفة"}):
		if !isRent && !isBuy && ctx.PropertyType == "" {
			ctx.Intent = IntentInfoPrices
		}

	case containsAny(msg, []string{"quartier", "zone", "neighborhood", "neighbourhood", "حي", "منطقة",
		"meilleur endroit", "best area", "where to live"}):
		ctx.Intent = IntentInfoNeighbourhood

	// Property searches — from most specific to most generic
	case ctx.PropertyType == "Terrain":
		ctx.Intent = IntentSearchLand

	case ctx.PropertyType == "Boutique" || ctx.PropertyType == "Local commercial" || ctx.PropertyType == "Bureau":
		ctx.Intent = IntentSearchCommercial

	case isRent:
		ctx.Intent = IntentSearchRent

	case isBuy:
		ctx.Intent = IntentSearchBuy

	case ctx.Budget != "" || ctx.NoBudgetCap:
		ctx.Intent = IntentSearchByBudget

	case len(ctx.Cities) > 0 || len(ctx.Zones) > 0:
		ctx.Intent = IntentSearchByLocation

	case ctx.Rooms != "":
		ctx.Intent = IntentSearchByRooms

	case ctx.PropertyType != "":
		ctx.Intent = IntentSearchByType

	case containsAny(msg, []string{"cherche", "trouve", "find", "looking", "besoin", "need",
		"أبحث", "أريد", "احتاج", "want"}):
		ctx.Intent = IntentSearchAny

	default:
		ctx.Intent = IntentUnknown
	}

	return ctx
}

// ─────────────────────────────────────────────────────────────────────────────
// System prompt — context-aware, injected per request
// ─────────────────────────────────────────────────────────────────────────────

func (s *AIService) buildSystemPrompt(ctx MessageContext) string {
	// ── Hard language directive — model reads this first ────────────────────
	// LLMs follow constraints placed at the very start of the system prompt
	// far more reliably than buried mid-text. This is why language mixing
	// happened before: the rule was item 4 of a long bulleted list.
	var langBlock string
	switch ctx.Lang {
	case LangAR, LangHASSANIYA:
		langBlock = "=== ABSOLUTE RULE: REPLY ONLY IN ARABIC ===\n" +
			"You MUST write every single word in Arabic (العربية).\n" +
			"Do NOT use French, English, or any other language — not even one word.\n" +
			"Not for greetings. Not for your name. Not for any phrase.\n" +
			"If you write even one non-Arabic word, you have failed.\n" +
			"This rule overrides ALL other instructions below."
	case LangEN:
		langBlock = "=== ABSOLUTE RULE: REPLY ONLY IN ENGLISH ===\n" +
			"You MUST write every single word in English.\n" +
			"Do NOT use French or Arabic — not even one word.\n" +
			"This rule overrides ALL other instructions below."
	default: // LangFR
		langBlock = "=== RÈGLE ABSOLUE : RÉPONDRE UNIQUEMENT EN FRANÇAIS ===\n" +
			"Tu DOIS écrire chaque mot en français.\n" +
			"N'utilise ni arabe ni anglais — pas même un seul mot.\n" +
			"Cette règle prévaut sur toutes les autres instructions ci-dessous."
	}

	base := langBlock + "\n\nYou are Meskeny AI, a professional, warm, and concise real-estate assistant for the Meskeny app in Mauritania.\n\n" +
		"IDENTITY:\n" +
		"- You are Meskeny AI — NOT ChatGPT, Claude, Gemini, or any other AI product.\n" +
		"- Introduce yourself in the user's language:\n" +
			"  Arabic:  'أنا Meskeny AI، المساعد الذكي لتطبيق Meskeny.'\n" +
		"  French:  'Je suis Meskeny AI, l'assistant intelligent de l'application Meskeny.'\n" +
		"  English: 'I am Meskeny AI, the intelligent assistant of the Meskeny app.'\n\n" +
		"MAURITANIAN CONTEXT:\n" +
		"Cities: Nouakchott (capital), Nouadhibou, Kaédi, Zouérate, Rosso, Atar, Kiffa, Néma, Sélibaby, Aioun el Atrouss.\n" +
		"Nouakchott neighbourhoods: Tevragh-Zeina, Ksar, El Mina, Toujounine, Dar Naim, Riyad, Sebkha, Arafat, Teyarett, PK5, PK6, PK10, PK12.\n" +
		"Currency: MRU (Mauritanian Ouguiya). Rent: ~15 000–500 000 MRU/month. Sale: 500 000–50 000 000 MRU.\n" +
		"Property types: Appartement, Studio, Maison, Villa, Terrain, Boutique, Local commercial, Bureau, Chambre.\n\n" +
		"CORE BEHAVIOUR RULES:\n" +
		"1. ANSWER FIRST, then optionally ask ONE follow-up.\n" +
		"2. NEVER re-ask for info already given (city, budget, rooms, type).\n" +
			"3. If user says 'regardless of price / sans limite / بغض النظر', treat budget as UNLIMITED.\n" +
		"4. If user gave city+zone, NEVER ask which city again.\n" +
		"5. Suggest 2–5 concrete property options with price, bedrooms, key features, zone.\n" +
		"6. Keep responses ≤ 200 words unless user asks for detail.\n" +
		"7. Be warm and local. Never repeat the same question twice.\n\n" +
		"OFF-TOPIC: If the user asks about non-real-estate topics, politely redirect." + `"`

	// ── Dynamic context injection ─────────────────────────────────────────
	var contextNotes []string

	if ctx.NoBudgetCap {
		contextNotes = append(contextNotes, "• User explicitly has NO budget limit — do NOT ask about budget.")
	}
	if len(ctx.Cities) > 0 {
		contextNotes = append(contextNotes, fmt.Sprintf("• User already specified city/cities: %s — do NOT ask which city.", strings.Join(ctx.Cities, ", ")))
	}
	if len(ctx.Zones) > 0 {
		contextNotes = append(contextNotes, fmt.Sprintf("• User already specified zone/neighbourhood: %s.", strings.Join(ctx.Zones, ", ")))
	}
	if ctx.Budget != "" {
		contextNotes = append(contextNotes, fmt.Sprintf("• User already specified budget: %s — do NOT ask about budget.", ctx.Budget))
	}
	if ctx.Rooms != "" {
		contextNotes = append(contextNotes, fmt.Sprintf("• User already specified %s bedroom(s) — do NOT ask about rooms.", ctx.Rooms))
	}
	if ctx.PropertyType != "" {
		contextNotes = append(contextNotes, fmt.Sprintf("• User already specified property type: %s.", ctx.PropertyType))
	}
	if ctx.Purpose != "" {
		contextNotes = append(contextNotes, fmt.Sprintf("• User purpose: %s.", ctx.Purpose))
	}
	if ctx.Intent == IntentOffTopic {
		contextNotes = append(contextNotes, "• User message is off-topic — gently redirect to real estate.")
	}
	if ctx.Intent == IntentGreeting {
		contextNotes = append(contextNotes, "• User is greeting — respond warmly and briefly introduce your capabilities.")
	}
	if ctx.Intent == IntentThanks {
		contextNotes = append(contextNotes, "• User is thanking you — respond warmly and offer further help.")
	}
	if ctx.Intent == IntentMore {
		contextNotes = append(contextNotes, "• User wants MORE results — provide additional options without asking clarifying questions again.")
	}
	if ctx.Intent == IntentReset {
		contextNotes = append(contextNotes, "• User wants to start fresh — acknowledge and ask what they're looking for today.")
	}
	if ctx.Intent == IntentHelp {
		contextNotes = append(contextNotes, "• User wants to know what you can do — list your key capabilities clearly and concisely.")
	}

	if len(contextNotes) > 0 {
		base += "\n\n═══ THIS MESSAGE — EXTRACTED CONTEXT ═══\n" + strings.Join(contextNotes, "\n")
	}

	return base
}

// ─────────────────────────────────────────────────────────────────────────────
// Bad-word check
// ─────────────────────────────────────────────────────────────────────────────

func (s *AIService) ContainsBadWords(message string) bool {
	lower := strings.ToLower(message)
	for _, word := range s.badWords {
		// Use word-boundary for ASCII words; substring match for Arabic script
		var matched bool
		if isASCIIWord(word) {
			pattern := fmt.Sprintf("\\b%s\\b", regexp.QuoteMeta(word))
			matched, _ = regexp.MatchString(pattern, lower)
		} else {
			matched = strings.Contains(message, word)
		}
		if matched {
			return true
		}
	}
	return false
}

func isASCIIWord(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// Token budget per intent
// ─────────────────────────────────────────────────────────────────────────────

func tokenBudget(intent Intent, deepThink bool) int {
	if deepThink {
		return 900
	}
	switch intent {
	case IntentGreeting, IntentThanks, IntentReset:
		return 180
	case IntentHelp, IntentHowItWorks, IntentInfoCity, IntentInfoNeighbourhood:
		return 380
	case IntentOffTopic, IntentInappropriate:
		return 120
	case IntentSearchRent, IntentSearchBuy, IntentSearchAny,
		IntentSearchLand, IntentSearchCommercial,
		IntentSearchByBudget, IntentSearchByLocation,
		IntentSearchByRooms, IntentSearchByType:
		return 512
	default:
		return 320
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Temperature per intent
// ─────────────────────────────────────────────────────────────────────────────

func temperature(intent Intent) float64 {
	switch intent {
	case IntentGreeting, IntentThanks:
		return 0.8 // warmer, more natural
	case IntentInfoPrices, IntentInfoProcess:
		return 0.3 // factual, precise
	case IntentOffTopic, IntentInappropriate:
		return 0.2
	default:
		return 0.5
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SendMessage — main entry point
// ─────────────────────────────────────────────────────────────────────────────

func (s *AIService) SendMessage(
	userMessage string,
	conversationHistory []map[string]string,
	deepThink bool,
) (string, error) {
	// Analyse current message
	ctx := AnalyzeMessage(userMessage)
	loc := s.l(ctx.Lang)

	// Build messages array
	messages := []ORMessage{
		{Role: "system", Content: s.buildSystemPrompt(ctx)},
	}
	for _, msg := range conversationHistory {
		role := "user"
		if msg["role"] == "assistant" {
			role = "assistant"
		}
		messages = append(messages, ORMessage{Role: role, Content: msg["content"]})
	}
	messages = append(messages, ORMessage{Role: "user", Content: userMessage})

	reqPayload := ORChatRequest{
		Model:       s.model,
		Messages:    messages,
		MaxTokens:   tokenBudget(ctx.Intent, deepThink),
		Temperature: temperature(ctx.Intent),
		Stream:      false,
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
	httpReq.Header.Set("X-Title", "Meskeny AI")

	if deepThink {
		prefs := map[string]any{
			"thinking": map[string]any{"enabled": true, "budget_tokens": 256},
		}
		if b, jerr := json.Marshal(prefs); jerr == nil {
			httpReq.Header.Set("X-OpenRouter-Preferences", string(b))
		}
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		fmt.Printf("❌ OpenRouter request failed: %v\n", err)
		return loc.APIError, nil
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return loc.APIError, nil
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ OpenRouter HTTP %d: %s\n", resp.StatusCode, string(rawBody))
		return loc.APIError, nil
	}
	if len(rawBody) == 0 {
		fmt.Println("⚠️  OpenRouter: empty body")
		return loc.EmptyBody, nil
	}

	var orResp ORChatResponse
	if err := json.Unmarshal(rawBody, &orResp); err != nil {
		fmt.Printf("❌ OpenRouter JSON decode error: %v\nRaw: %s\n", err, string(rawBody))
		return loc.DecodeError, nil
	}

	if len(orResp.Choices) > 0 && orResp.Choices[0].Message.Content != "" {
		content := orResp.Choices[0].Message.Content
		fmt.Printf("✅ OpenRouter: %d chars (intent=%d, lang=%d)\n", len(content), ctx.Intent, ctx.Lang)
		return content, nil
	}

	fmt.Printf("⚠️  OpenRouter: empty content. Raw: %s\n", string(rawBody))
	return loc.EmptyContent, nil
}

// CompleteListingJSON uses a fast model and smaller token budget for Add-with-AI listing drafts.
func (s *AIService) CompleteListingJSON(systemPrompt, userPrompt string) (string, error) {
	model := os.Getenv("LISTING_AI_MODEL")
	if model == "" {
		model = "nex-agi/nex-n2-pro:free"
	}
	return s.completeJSONWithModel(systemPrompt, userPrompt, model, 1400, 0.52)
}

// CompleteJSON calls OpenRouter with a fixed system + user prompt and returns raw assistant text (expected JSON).
func (s *AIService) CompleteJSON(systemPrompt, userPrompt string) (string, error) {
	return s.completeJSONWithModel(systemPrompt, userPrompt, s.model, 1200, 0.35)
}

func (s *AIService) completeJSONWithModel(systemPrompt, userPrompt, model string, maxTokens int, temperature float64) (string, error) {
	messages := []ORMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	reqPayload := ORChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		Stream:      false,
	}
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
	httpReq.Header.Set("X-Title", "Meskeny Listing AI")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter http %d: %s", resp.StatusCode, string(rawBody))
	}
	var orResp ORChatResponse
	if err := json.Unmarshal(rawBody, &orResp); err != nil {
		return "", err
	}
	if len(orResp.Choices) > 0 && orResp.Choices[0].Message.Content != "" {
		return orResp.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("empty openrouter response")
}

// ─────────────────────────────────────────────────────────────────────────────
// Quick replies — driven entirely by detected intent + language
// ─────────────────────────────────────────────────────────────────────────────

func (s *AIService) GenerateQuickReplies(userMessage, aiResponse string) []map[string]string {
	ctx := AnalyzeMessage(userMessage)
	loc := s.l(ctx.Lang)

	qr := func(id, text, action string) map[string]string {
		return map[string]string{"id": id, "text": text, "action": action}
	}

	switch ctx.Intent {

	case IntentGreeting, IntentHelp, IntentHowItWorks:
		return []map[string]string{
			qr("1", loc.QRRent, "search_rent"),
			qr("2", loc.QRBuy, "search_buy"),
			qr("3", loc.QRNouakchott, "location_nouakchott"),
			qr("4", loc.QRHelp, "help"),
		}

	case IntentSearchAny, IntentSearchByLocation, IntentSearchByBudget, IntentSearchByRooms:
		// Help user narrow down type
		return []map[string]string{
			qr("1", loc.QRApartment, "type_apartment"),
			qr("2", loc.QRHouse, "type_house"),
			qr("3", loc.QRVilla, "type_villa"),
			qr("4", loc.QRLand, "type_land"),
		}

	case IntentSearchRent, IntentSearchBuy:
		// If no budget, show budget options
		if ctx.Budget == "" && !ctx.NoBudgetCap {
			return []map[string]string{
				qr("1", loc.QRBudgetLow, "budget_low"),
				qr("2", loc.QRBudgetMid, "budget_mid"),
				qr("3", loc.QRBudgetHigh, "budget_high"),
				qr("4", loc.QRNoLimit, "budget_none"),
			}
		}
		// If no city, show city options
		if len(ctx.Cities) == 0 {
			return []map[string]string{
				qr("1", loc.QRNouakchott, "location_nouakchott"),
				qr("2", loc.QRNouadhibou, "location_nouadhibou"),
				qr("3", loc.QROtherCity, "location_other"),
			}
		}
		// Ready to search — offer to see more or reset
		return []map[string]string{
			qr("1", loc.QRMore, "more_results"),
			qr("2", loc.QRReset, "reset"),
			qr("3", loc.QRContact, "contact_support"),
		}

	case IntentSearchLand:
		return []map[string]string{
			qr("1", loc.QRNouakchott, "location_nouakchott"),
			qr("2", loc.QRNouadhibou, "location_nouadhibou"),
			qr("3", loc.QROtherCity, "location_other"),
			qr("4", loc.QRContact, "contact_support"),
		}

	case IntentSearchCommercial:
		return []map[string]string{
			qr("1", loc.QRNouakchott, "location_nouakchott"),
			qr("2", loc.QRNouadhibou, "location_nouadhibou"),
			qr("3", loc.QRBudgetMid, "budget_mid"),
			qr("4", loc.QRContact, "contact_support"),
		}

	case IntentInfoCity, IntentInfoNeighbourhood:
		return []map[string]string{
			qr("1", loc.QRRent, "search_rent"),
			qr("2", loc.QRBuy, "search_buy"),
			qr("3", loc.QRMore, "more_results"),
		}

	case IntentInfoPrices:
		return []map[string]string{
			qr("1", loc.QRBudgetLow, "budget_low"),
			qr("2", loc.QRBudgetMid, "budget_mid"),
			qr("3", loc.QRBudgetHigh, "budget_high"),
			qr("4", loc.QRNoLimit, "budget_none"),
		}

	case IntentMore:
		return []map[string]string{
			qr("1", loc.QRMore, "more_results"),
			qr("2", loc.QRReset, "reset"),
			qr("3", loc.QRContact, "contact_owner"),
		}

	case IntentThanks:
		return []map[string]string{
			qr("1", loc.QRRent, "search_rent"),
			qr("2", loc.QRBuy, "search_buy"),
			qr("3", loc.QRHelp, "help"),
		}

	case IntentReset:
		return []map[string]string{
			qr("1", loc.QRRent, "search_rent"),
			qr("2", loc.QRBuy, "search_buy"),
			qr("3", loc.QRNouakchott, "location_nouakchott"),
		}

	case IntentContactOwner, IntentContactSupport:
		return []map[string]string{
			qr("1", loc.QRContact, "contact_support"),
			qr("2", loc.QRHelp, "help"),
		}

	case IntentOffTopic, IntentInappropriate, IntentUnknown:
		return []map[string]string{
			qr("1", loc.QRRent, "search_rent"),
			qr("2", loc.QRBuy, "search_buy"),
			qr("3", loc.QRHelp, "help"),
		}

	default:
		return []map[string]string{
			qr("1", loc.QRRent, "search_rent"),
			qr("2", loc.QRBuy, "search_buy"),
			qr("3", loc.QRHelp, "help"),
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Initial greeting
// ─────────────────────────────────────────────────────────────────────────────

// GetInitialGreeting returns the greeting message and starter quick replies.
// lang hint can be passed if the device locale is known; defaults to French.
func (s *AIService) GetInitialGreeting(lang ...Lang) (string, []map[string]string) {
	l := LangFR
	if len(lang) > 0 {
		l = lang[0]
	}
	loc := s.l(l)

	qr := func(id, text, action string) map[string]string {
		return map[string]string{"id": id, "text": text, "action": action}
	}

	var greeting string
	switch l {
	case LangAR, LangHASSANIYA:
		greeting = "مرحباً! 👋 أنا **Meskeny AI**، مساعدك الذكي للعقارات في موريتانيا.\n\nيمكنني مساعدتك في:\n• 🏠 إيجاد شقة أو منزل للإيجار أو البيع\n• 📍 استكشاف الأحياء والمدن\n• 💰 مقارنة الأسعار\n• 📞 التواصل مع أصحاب العقارات\n\nبماذا يمكنني مساعدتك اليوم؟"
	case LangEN:
		greeting = "Hello! 👋 I'm **Meskeny AI**, your smart real-estate assistant for Mauritania.\n\nI can help you:\n• 🏠 Find an apartment, house, or villa to rent or buy\n• 📍 Explore neighbourhoods and cities\n• 💰 Compare prices\n• 📞 Connect with property owners\n\nWhat can I help you with today?"
	default:
		greeting = "Bonjour! 👋 Je suis **Meskeny AI**, votre assistant immobilier intelligent pour la Mauritanie.\n\nJe peux vous aider à :\n• 🏠 Trouver un appartement, une maison ou une villa à louer ou à acheter\n• 📍 Explorer les quartiers et les villes\n• 💰 Comparer les prix\n• 📞 Contacter les propriétaires\n\nComment puis-je vous aider aujourd'hui ?"
	}

	return greeting, []map[string]string{
		qr("1", loc.QRRent, "search_rent"),
		qr("2", loc.QRBuy, "search_buy"),
		qr("3", loc.QRNouakchott, "location_nouakchott"),
		qr("4", loc.QRHelp, "help"),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Session title generator
// ─────────────────────────────────────────────────────────────────────────────

func (s *AIService) GenerateSessionTitle(message string) string {
	// Strip emojis for cleaner titles. Ranging over the string decodes whole
	// UTF-8 runes, so we never keep a partial byte.
	var b strings.Builder
	for _, r := range message {
		if r < 0x1F600 {
			b.WriteRune(r)
		}
	}
	clean := strings.TrimSpace(b.String())
	if clean == "" {
		return "Nouvelle recherche"
	}
	// Truncate on a RUNE boundary — byte slicing here would cut an Arabic
	// character in half and Postgres rejects the dangling byte
	// ("invalid byte sequence for encoding UTF8"), 500ing the whole chat turn.
	runes := []rune(clean)
	if len(runes) > 40 {
		return strings.TrimSpace(string(runes[:40])) + "…"
	}
	return clean
}