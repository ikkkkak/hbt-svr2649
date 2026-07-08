// Model X46 — branded Meskeny AI identity (user-facing: never expose OpenRouter/vendor names).
package prompt

import (
	"fmt"
	"strings"
)

const (
	ModelX46Name    = "Meskeny Model X46"
	ModelX46Creator = "Meskeny Technology"
	ModelX46Version = "1.0.0"
)

// UserContext carries personalization injected into the system prompt.
type UserContext struct {
	Name                 string
	BudgetRange          string
	PreferredLocations   []string
	PropertyType         string
	FamilySize           string
	ActiveSearches       int
	SavedProperties      int
	PreviousInteractions int
	PreferredLanguage    string
}

// SystemPromptModelX46 builds the full branded system prompt for chat/search/escalation.
func SystemPromptModelX46(user *UserContext) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(`You are %s, the proprietary real-estate intelligence developed exclusively by %s (version %s).

IDENTITY RULES:
- Always present yourself as %s only. Never mention OpenAI, Anthropic, OpenRouter, or other vendors.
- Speak Arabic, French, or English to match the user's language.
- Never invent listings, prices, or registry claims — only use Meskeny database data when properties are provided.
- Be professional, warm, concise, and honest when uncertain.

CAPABILITIES:
- Semantic property matching with clear match reasons
- Mauritania market context (Nouakchott zones, MRU pricing)
- Transaction guidance in plain language
- Escalate to human agents for legal disputes, negotiation, financing, explicit requests, or strong frustration
- Smart notifications only when highly relevant — never generic spam

`, ModelX46Name, ModelX46Creator, ModelX46Version, ModelX46Name))

	if user != nil {
		b.WriteString(fmt.Sprintf(`USER CONTEXT:
- Name: %s
- Budget: %s
- Locations: %s
- Property type: %s
- Family size: %s
- Active searches: %d
- Saved properties: %d
- Prior chats: %d
- Language: %s

`, user.Name, user.BudgetRange, strings.Join(user.PreferredLocations, ", "),
			user.PropertyType, user.FamilySize, user.ActiveSearches, user.SavedProperties,
			user.PreviousInteractions, user.PreferredLanguage))
	}

	b.WriteString(MauritaniaContext())
	return b.String()
}

// WelcomeMessageModelX46 returns a localized welcome string (server-side fallback).
func WelcomeMessageModelX46(lang, userName string) string {
	name := strings.TrimSpace(userName)
	switch normalizePromptLang(lang) {
	case "ar":
		if name != "" {
			return fmt.Sprintf("مرحباً %s! أنا %s، مساعدك العقاري الذكي. كيف يمكنني مساعدتك في إيجاد العقار المناسب؟", name, ModelX46Name)
		}
		return fmt.Sprintf("مرحباً! أنا %s، الذكاء الاصطناعي الخاص بـ Meskeny. صِف ما تبحث عنه — «منزل هادئ قرب مدارس جيدة» أو «استثمار عائد مرتفع» — وسأجد لك الأنسب.", ModelX46Name)
	case "fr":
		if name != "" {
			return fmt.Sprintf("Bon retour, %s ! Je suis %s, votre assistant immobilier intelligent. Comment puis-je vous aider aujourd'hui ?", name, ModelX46Name)
		}
		return fmt.Sprintf("Bonjour ! Je suis %s, l'IA propriétaire de Meskeny. Décrivez votre recherche — « maison calme près des écoles » ou « investissement rentable » — et je trouverai votre match.", ModelX46Name)
	default:
		if name != "" {
			return fmt.Sprintf("Welcome back, %s! I'm %s, your personal real estate intelligence. How can I help you find your perfect property today?", name, ModelX46Name)
		}
		return fmt.Sprintf("Hello! I'm %s, Meskeny's proprietary AI. Tell me what you're looking for — a quiet family home near good schools or a high-yield investment — and I'll find your match.", ModelX46Name)
	}
}

func normalizePromptLang(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(s, "ar"):
		return "ar"
	case strings.HasPrefix(s, "fr"):
		return "fr"
	default:
		return "en"
	}
}
