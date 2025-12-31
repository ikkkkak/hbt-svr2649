package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// GeminiRequest represents the request to Gemini API
type GeminiRequest struct {
	Contents         []GeminiContent  `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig"`
	SafetySettings   []SafetySetting  `json:"safetySettings"`
}

type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	TopK            int     `json:"topK"`
	TopP            float64 `json:"topP"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiResponse represents the response from Gemini API
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// AIService handles AI-related operations
type AIService struct {
	apiKey   string
	apiURL   string
	badWords []string
}

// NewAIService creates a new AI service instance
func NewAIService() *AIService {
	apiKey := "AIzaSyALepfjs6wIRvtbKF47L9ZPmybHUbdnKqE"
	if apiKey == "" {
		fmt.Println("⚠️ GEMINI_API_KEY not set, AI features will be limited")
	}

	return &AIService{
		apiKey: apiKey,
		apiURL: "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent",
		badWords: []string{
			// English
			"fuck", "shit", "damn", "bitch", "ass", "bastard", "crap", "dick", "pussy",
			"whore", "slut", "nigger", "faggot", "retard",
			// French
			"merde", "putain", "connard", "salope", "encule", "bordel", "nique",
			// Arabic transliterated
			"kos", "sharmouta", "ibn el", "ya kalb", "ya hmar",
		},
	}
}

// GetSystemPrompt returns the system prompt for Meskeny AI
func (s *AIService) GetSystemPrompt() string {
	return `You are Meskeny AI, a professional and friendly AI assistant for the Meskeny real estate app in Mauritania.

Your identity:
- You are Meskeny AI, NOT ChatGPT, Claude, Gemini, or any other AI provider
- You help users find properties for rent or sale in Mauritania
- You are knowledgeable about Mauritanian cities and neighborhoods
- You speak French, Arabic, and English fluently
- You are warm, helpful, and professional

Your capabilities:
- Help users find properties based on their preferences (location, price, size, type)
- Provide information about cities and zones in Mauritania
- Answer questions about the rental/buying process
- Give tips for property searching
- Explain property features and amenities

Important rules:
- Always be respectful and professional
- If asked who you are, say you are "Meskeny AI", the intelligent assistant for Meskeny app
- Never claim to be from another AI company
- Focus on helping users find their ideal property
- If you don't know something specific about a property, encourage users to contact the owner
- When recommending properties, ask about: budget, location preference, number of bedrooms, property type

Available cities in Mauritania:
- Nouakchott (capital, largest city)
- Nouadhibou (second largest, coastal)
- Kaédi, Zouérate, Rosso, Atar, Kiffa, Néma, Sélibaby, Aioun el Atrouss

Property types available:
- Apartments (Appartement)
- Houses (Maison)
- Villas
- Land (Terrain)
- Commercial spaces (Boutique, Local commercial)
- Offices

Always respond in the same language the user writes in. Be concise but helpful.`
}

// ContainsBadWords checks if a message contains inappropriate language
func (s *AIService) ContainsBadWords(message string) bool {
	lowerMessage := strings.ToLower(message)
	for _, word := range s.badWords {
		pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(word))
		matched, _ := regexp.MatchString(pattern, lowerMessage)
		if matched {
			return true
		}
	}
	return false
}

// SendMessage sends a message to Gemini and returns the response
func (s *AIService) SendMessage(userMessage string, conversationHistory []map[string]string) (string, error) {
	if s.apiKey == "" {
		return "Je suis désolé, le service AI n'est pas disponible pour le moment. Veuillez réessayer plus tard.", nil
	}

	// Build conversation contents
	contents := []GeminiContent{
		{
			Role:  "user",
			Parts: []GeminiPart{{Text: s.GetSystemPrompt()}},
		},
		{
			Role:  "model",
			Parts: []GeminiPart{{Text: "Bonjour! Je suis Meskeny AI, votre assistant immobilier intelligent. Comment puis-je vous aider à trouver votre propriété idéale aujourd'hui?"}},
		},
	}

	// Add conversation history
	for _, msg := range conversationHistory {
		role := "user"
		if msg["role"] == "assistant" {
			role = "model"
		}
		contents = append(contents, GeminiContent{
			Role:  role,
			Parts: []GeminiPart{{Text: msg["content"]}},
		})
	}

	// Add current user message
	contents = append(contents, GeminiContent{
		Role:  "user",
		Parts: []GeminiPart{{Text: userMessage}},
	})

	request := GeminiRequest{
		Contents: contents,
		GenerationConfig: GenerationConfig{
			Temperature:     0.7,
			TopK:            40,
			TopP:            0.95,
			MaxOutputTokens: 1024,
		},
		SafetySettings: []SafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_MEDIUM_AND_ABOVE"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_MEDIUM_AND_ABOVE"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_MEDIUM_AND_ABOVE"},
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_MEDIUM_AND_ABOVE"},
		},
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", s.apiURL, s.apiKey)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error body for debugging
		var errorBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errorBody)
		fmt.Printf("❌ Gemini API Error: Status %d, Body: %+v\n", resp.StatusCode, errorBody)
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		response := geminiResp.Candidates[0].Content.Parts[0].Text
		fmt.Printf("✅ Gemini API: Got response (%d chars)\n", len(response))
		return response, nil
	}

	fmt.Println("⚠️ Gemini API: No candidates in response")

	return "Je suis désolé, je n'ai pas pu traiter votre demande. Pouvez-vous reformuler?", nil
}

// GenerateQuickReplies generates contextual quick replies based on conversation
func (s *AIService) GenerateQuickReplies(userMessage, aiResponse string) []map[string]string {
	lowerMessage := strings.ToLower(userMessage)
	lowerResponse := strings.ToLower(aiResponse)

	// Property search context
	if strings.Contains(lowerMessage, "cherche") || strings.Contains(lowerMessage, "looking") || strings.Contains(lowerMessage, "find") {
		return []map[string]string{
			{"id": "1", "text": "🏠 Appartement", "action": "search_apartment"},
			{"id": "2", "text": "🏡 Maison", "action": "search_house"},
			{"id": "3", "text": "🏢 Boutique", "action": "search_commercial"},
			{"id": "4", "text": "📍 Terrain", "action": "search_land"},
		}
	}

	// Location context
	if strings.Contains(lowerMessage, "où") || strings.Contains(lowerMessage, "where") || strings.Contains(lowerMessage, "location") {
		return []map[string]string{
			{"id": "1", "text": "📍 Nouakchott", "action": "location_nouakchott"},
			{"id": "2", "text": "📍 Nouadhibou", "action": "location_nouadhibou"},
			{"id": "3", "text": "📍 Autre ville", "action": "location_other"},
		}
	}

	// Budget context
	if strings.Contains(lowerMessage, "prix") || strings.Contains(lowerMessage, "budget") || strings.Contains(lowerMessage, "price") {
		return []map[string]string{
			{"id": "1", "text": "💰 < 100,000 MRU", "action": "budget_low"},
			{"id": "2", "text": "💰 100k - 300k MRU", "action": "budget_medium"},
			{"id": "3", "text": "💰 > 300,000 MRU", "action": "budget_high"},
		}
	}

	// Purpose context
	if strings.Contains(lowerResponse, "louer") || strings.Contains(lowerResponse, "acheter") {
		return []map[string]string{
			{"id": "1", "text": "🔑 À louer", "action": "purpose_rent"},
			{"id": "2", "text": "🏷️ À acheter", "action": "purpose_buy"},
		}
	}

	// Default helpful actions
	return []map[string]string{
		{"id": "1", "text": "🔍 Chercher propriété", "action": "search_property"},
		{"id": "2", "text": "📞 Contacter support", "action": "contact_support"},
		{"id": "3", "text": "❓ Aide", "action": "help"},
	}
}

// GetInitialGreeting returns the initial greeting message
func (s *AIService) GetInitialGreeting() (string, []map[string]string) {
	greeting := `Bonjour! 👋 Je suis **Meskeny AI**, votre assistant immobilier intelligent.

Je peux vous aider à:
• 🏠 Trouver un appartement ou une maison
• 🏢 Chercher un local commercial
• 📍 Explorer les quartiers
• 💰 Comparer les prix

Comment puis-je vous aider aujourd'hui?`

	quickReplies := []map[string]string{
		{"id": "1", "text": "🏠 Chercher à louer", "action": "search_rent"},
		{"id": "2", "text": "🏷️ Chercher à acheter", "action": "search_buy"},
		{"id": "3", "text": "📍 Explorer Nouakchott", "action": "explore_nouakchott"},
		{"id": "4", "text": "❓ Comment ça marche?", "action": "how_it_works"},
	}

	return greeting, quickReplies
}

// GenerateSessionTitle generates a title from the first user message
func (s *AIService) GenerateSessionTitle(message string) string {
	if len(message) > 40 {
		return message[:40] + "..."
	}
	return message
}
