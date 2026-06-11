package response

import (
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/property"
)

// BlockedOutput returns a simple blocked message payload.
func BlockedOutput(reason string) (out struct {
	Message Message `json:"message"`
}) {
	out.Message = Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: reason,
	}
	return
}

// NoResultsOutput is used when the DB has no matching properties.
// Uses BuildNoResultsPayload for contextual message and follow-up chips.
func NoResultsOutput(ctx lang.MessageContext) (out struct {
	Message      Message     `json:"message"`
	QuickReplies []QuickReply `json:"quick_replies,omitempty"`
}) {
	searchCtx := SearchContext{
		Lang:         ctx.Lang,
		City:         ctx.City,
		Zone:         ctx.Zone,
		Quartier:     ctx.Quartier,
		PropertyType: ctx.Type,
		BudgetMRU:    ctx.BudgetMRU,
		BudgetMin:    ctx.BudgetMin,
		BudgetMax:    ctx.BudgetMax,
	}
	if ctx.Intent == lang.IntentSearchRent {
		searchCtx.Purpose = "rent"
	} else {
		searchCtx.Purpose = "sale"
	}
	txt, chips := BuildNoResultsPayload(searchCtx)
	out.Message = Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: txt,
	}
	out.QuickReplies = chips
	return
}

// WithCardsOutput builds a message that tells the user to check the cards.
func WithCardsOutput(ctx lang.MessageContext, cards []property.Card) (out struct {
	Message                 Message           `json:"message"`
	PropertyRecommendations []property.Card   `json:"propertyRecommendations"`
}) {
	txt := buildSearchSummaryText(ctx, len(cards))
	out.Message = Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: txt,
	}
	out.PropertyRecommendations = cards
	return
}

// APIErrorOutput is used when the model API fails.
func APIErrorOutput(ctx lang.MessageContext) (out struct {
	Message Message `json:"message"`
}) {
	txt := "Je suis désolé, un problème est survenu avec MeskenyGPT. Réessaie dans un instant."
	if ctx.Lang == lang.LangAR {
		txt = "عذراً، حدثت مشكلة مؤقتة في MeskenyGPT. حاول مرة أخرى بعد لحظات."
	} else if ctx.Lang == lang.LangEN {
		txt = "Sorry, MeskenyGPT hit a temporary issue. Please try again in a moment."
	}
	out.Message = Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: txt,
	}
	return
}

// GreetingMessage returns the first greeting for a language.
func GreetingMessage(l lang.Lang) Message {
	content := "Bonjour ! Je suis MeskenyGPT, مساعدك العقاري الذكي."
	if l == lang.LangAR {
		content = "مرحباً! أنا MeskenyGPT، المساعد الذكي لمساعدتك في العثور على عقار مناسب في موريتانيا."
	} else if l == lang.LangEN {
		content = "Hi! I'm MeskenyGPT, your smart real-estate assistant for Mauritania."
	}
	return Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: content,
	}
}

// GreetingQuickReplies returns starter chips.
func GreetingQuickReplies(l lang.Lang) []QuickReply {
	switch l {
	case lang.LangAR:
		return []QuickReply{
			{ID: "1", Text: "🏠 أريد شقة للإيجار", Action: "أريد شقة للإيجار في نواكشوط"},
			{ID: "2", Text: "🏡 أبحث عن منزل للبيع", Action: "أبحث عن منزل للبيع"},
			{ID: "3", Text: "📍 ابحث حسب الحي", Action: "أريد البحث حسب الحي والميزانية"},
		}
	case lang.LangEN:
		return []QuickReply{
			{ID: "1", Text: "🏠 Find me a rental", Action: "I need an apartment for rent"},
			{ID: "2", Text: "🏡 Buy a house", Action: "I am looking for a house for sale"},
			{ID: "3", Text: "📍 Search by area", Action: "Help me search by area and budget"},
		}
	default:
		return []QuickReply{
			{ID: "1", Text: "🏠 Je cherche une location", Action: "Je cherche un appartement à louer"},
			{ID: "2", Text: "🏡 Je veux acheter", Action: "Je cherche une maison à vendre"},
			{ID: "3", Text: "📍 Recherche par quartier", Action: "Aide-moi à chercher par quartier et budget"},
		}
	}
}

// SearchResultsFollowUpQuickReplies returns three follow-up chips after DB-backed listing cards.
func SearchResultsFollowUpQuickReplies(ctx lang.MessageContext) []QuickReply {
	isLand := ctx.Intent == lang.IntentSearchLand || strings.EqualFold(strings.TrimSpace(ctx.Type), "land")
	switch ctx.Lang {
	case lang.LangAR:
		if isLand {
			return []QuickReply{
				{ID: "sf1", Text: "📐 أرض بمساحة مختلفة", Action: "أبحث عن أرض بنفس المنطقة لكن بمساحة أخرى"},
				{ID: "sf2", Text: "💰 حدد الميزانية", Action: "أريد أرضاً ضمن ميزانية محددة"},
				{ID: "sf3", Text: "📍 منطقة أخرى", Action: "اعرض أراضي في منطقة قريبة"},
			}
		}
		return []QuickReply{
			{ID: "sf1", Text: "💰 الميزانية", Action: "حدد ميزانيتي بدقة لنضيق البحث"},
			{ID: "sf2", Text: "🛏️ الغرف والمساحة", Action: "أريد عدد غرف أو مساحة محددة"},
			{ID: "sf3", Text: "📍 حي آخر", Action: "ابحث لي في حي مجاور"},
		}
	case lang.LangEN:
		if isLand {
			return []QuickReply{
				{ID: "sf1", Text: "📐 Different plot size", Action: "Same area but a different land size"},
				{ID: "sf2", Text: "💰 Set budget", Action: "Search land within a clearer budget"},
				{ID: "sf3", Text: "📍 Nearby area", Action: "Show land in a nearby area"},
			}
		}
		return []QuickReply{
			{ID: "sf1", Text: "💰 Budget", Action: "Narrow down by budget"},
			{ID: "sf2", Text: "🛏️ Rooms / size", Action: "I need a specific number of rooms or m²"},
			{ID: "sf3", Text: "📍 Another area", Action: "Search in a nearby neighborhood"},
		}
	default:
		if isLand {
			return []QuickReply{
				{ID: "sf1", Text: "📐 Autre surface", Action: "Je cherche un terrain de taille différente dans la même zone"},
				{ID: "sf2", Text: "💰 Budget", Action: "Préciser mon budget pour les terrains"},
				{ID: "sf3", Text: "📍 Zone proche", Action: "Montrer des terrains dans une zone voisine"},
			}
		}
		return []QuickReply{
			{ID: "sf1", Text: "💰 Budget", Action: "Préciser mon budget"},
			{ID: "sf2", Text: "🛏️ Pièces / surface", Action: "Je veux un nombre de pièces ou une surface précise"},
			{ID: "sf3", Text: "📍 Autre quartier", Action: "Chercher dans un quartier voisin"},
		}
	}
}

// GenerateQuickReplies is a very small placeholder.
func GenerateQuickReplies(ctx lang.MessageContext, content string) []QuickReply {
	// Picker-driven flow:
	// The frontend will render a structured picker (city -> zone -> quartier -> budget)
	// when it sees the `picker_city` action.
	pickCityText := "📍 Choose city"
	switch ctx.Lang {
	case lang.LangAR:
		pickCityText = "📍 اختر المدينة"
	case lang.LangFR:
		pickCityText = "📍 Choisir la ville"
	}

	// If the assistant is explicitly asking for city/zone/district and/or budget,
	// we attach a picker quick-reply even when intent is "unknown".
	// This makes the picker robust to LLM wording changes.
	lower := strings.ToLower(content)
	needsCity := strings.Contains(lower, "city") ||
		strings.Contains(lower, "ville") ||
		strings.Contains(lower, "مدينة")
	needsZone := strings.Contains(lower, "zone") ||
		strings.Contains(lower, "quartier") ||
		strings.Contains(lower, "neighborhood") ||
		strings.Contains(lower, "neighbourhood") ||
		strings.Contains(lower, "منطقة")
	needsBudget := strings.Contains(lower, "budget") ||
		strings.Contains(lower, "prix") ||
		strings.Contains(lower, "tarif") ||
		strings.Contains(lower, "ميزانيت") ||
		strings.Contains(lower, "الميزان") ||
		strings.Contains(lower, "ميزانيتك")

	if (needsCity || needsZone || needsBudget) && ctx.Intent == lang.IntentUnknown {
		switch ctx.Lang {
		case lang.LangAR:
			return []QuickReply{
				{ID: "picker_city", Text: pickCityText, Action: "picker_city"},
				{ID: "q2", Text: "💰 حدد الميزانية", Action: "ميزانيتي 20 مليون أوقية"},
				{ID: "q3", Text: "🛏️ عدد الغرف", Action: "أحتاج 3 غرف نوم"},
			}
		case lang.LangEN:
			return []QuickReply{
				{ID: "picker_city", Text: pickCityText, Action: "picker_city"},
				{ID: "q2", Text: "💰 Set budget", Action: "My budget is around 2 million MRU"},
				{ID: "q3", Text: "🛏️ Bedrooms", Action: "I need 3 bedrooms"},
			}
		default:
			return []QuickReply{
				{ID: "picker_city", Text: pickCityText, Action: "picker_city"},
				{ID: "q2", Text: "💰 Définir budget", Action: "Mon budget est autour de 2 millions MRU"},
				{ID: "q3", Text: "🛏️ Chambres", Action: "Je veux 3 chambres"},
			}
		}
	}

	switch ctx.Intent {
	case lang.IntentGreeting, lang.IntentHelp,
		lang.IntentSearchRent, lang.IntentSearchBuy, lang.IntentSearchAny:
		switch ctx.Lang {
		case lang.LangAR:
			return []QuickReply{
				{ID: "picker_city", Text: pickCityText, Action: "picker_city"},
				{ID: "q2", Text: "🏠 إيجار", Action: "أريد عقار للإيجار"},
				{ID: "q3", Text: "🏡 شراء", Action: "أريد عقار للبيع"},
			}
		case lang.LangEN:
			return []QuickReply{
				{ID: "picker_city", Text: pickCityText, Action: "picker_city"},
				{ID: "q2", Text: "🏠 Rentals", Action: "Show me rentals"},
				{ID: "q3", Text: "🏡 For sale", Action: "Show me properties for sale"},
			}
		default:
			return []QuickReply{
				{ID: "picker_city", Text: pickCityText, Action: "picker_city"},
				{ID: "q2", Text: "🏠 Location", Action: "Montre-moi des biens à louer"},
				{ID: "q3", Text: "🏡 Vente", Action: "Montre-moi des biens à vendre"},
			}
		}
	default:
		return []QuickReply{}
	}
}

func buildSearchSummaryText(ctx lang.MessageContext, count int) string {
	if count <= 0 {
		return buildNoResultsText(ctx)
	}
	switch ctx.Lang {
	case lang.LangAR:
		return fmt.Sprintf("✅ عثرت على %d عقارًا حقيقيًا في قاعدة بيانات Meskeny يطابق طلبك تقريبًا.\nاستعرض الخيارات في البطاقات أسفل هذه الرسالة واضغط على أي عقار لعرض التفاصيل الكاملة.", count)
	case lang.LangEN:
		return fmt.Sprintf("✅ I found %d real properties in Meskeny that roughly match your request.\nBrowse them in the cards below and tap any property to open full details.", count)
	default:
		return fmt.Sprintf("✅ J'ai trouvé %d biens réels dans Meskeny qui correspondent à ta demande.\nParcours-les dans les cartes ci-dessous et appuie sur un bien pour voir la fiche complète.", count)
	}
}

func buildNoResultsText(ctx lang.MessageContext) string {
	switch ctx.Lang {
	case lang.LangAR:
		return "لم أجد حالياً أي عقار في قاعدة بيانات Meskeny يطابق هذا الطلب بدقة.\nجرّب تعديل السعر أو الحي أو نوع العقار وسأبحث لك من جديد في العقارات الحقيقية الموجودة لدينا."
	case lang.LangEN:
		return "I couldn't find any live properties in the Meskeny database that match this request.\nTry adjusting the budget, area, or property type and I'll search again in the real listings."
	default:
		return "Je n'ai trouvé aucun bien en ligne dans la base Meskeny qui corresponde exactement à cette demande.\nEssaie d'ajuster le budget, le quartier ou le type de bien et je relancerai une recherche dans les annonces réelles."
	}
}

