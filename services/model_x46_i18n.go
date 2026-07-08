package services

import (
	"fmt"
	"strings"
)

// ModelX46EscalationUserCopy returns localized push/in-app copy when a chat escalates to a human.
func ModelX46EscalationUserCopy(lang, urgency string) (title, body string) {
	lang = NormalizeNotificationLang(lang)
	if lang == "" {
		lang = "en"
	}
	urgency = strings.ToLower(strings.TrimSpace(urgency))

	switch lang {
	case "ar":
		title = "جاري ربطك بمختص"
		switch urgency {
		case "urgent":
			body = "طلبك عاجل. يربطك أحد المختصين الآن — الرد المتوقع خلال 5 دقائق."
		case "high":
			body = "أربطك بمختص يمكنه مساعدتك بشكل أفضل. الرد المتوقع خلال 15 دقيقة."
		default:
			body = "أحوّلك إلى أحد خبرائنا للمتابعة خلال 30 دقيقة. سجل المحادثة مرفق."
		}
	case "fr":
		title = "Connexion à un spécialiste"
		switch urgency {
		case "urgent":
			body = "Votre demande est urgente. Un spécialiste vous rejoint — réponse sous 5 minutes."
		case "high":
			body = "Je vous mets en relation avec un spécialiste. Réponse attendue sous 15 minutes."
		default:
			body = "Un expert vous recontactera sous 30 minutes. L'historique du chat est inclus."
		}
	default:
		title = "Connecting you with a specialist"
		switch urgency {
		case "urgent":
			body = "Your request is urgent. A specialist is connecting now — expected response within 5 minutes."
		case "high":
			body = "I'm connecting you with a specialist who can better assist. Expected response within 15 minutes."
		default:
			body = "I'm passing you to one of our experts who will follow up within 30 minutes. Your conversation history is included."
		}
	}
	return title, body
}

// ModelX46EscalationAgentCopy returns localized copy for agent-facing escalation alerts.
func ModelX46EscalationAgentCopy(lang, urgency, triggerType, reason string) (title, body string) {
	lang = NormalizeNotificationLang(lang)
	if lang == "" {
		lang = "en"
	}
	switch lang {
	case "ar":
		title = fmt.Sprintf("[%s] تصعيد جديد: %s", urgency, triggerType)
		body = reason
	case "fr":
		title = fmt.Sprintf("[%s] Nouvelle escalade : %s", urgency, triggerType)
		body = reason
	default:
		title = fmt.Sprintf("[%s] New escalation: %s", urgency, triggerType)
		body = reason
	}
	if strings.TrimSpace(body) == "" {
		body = DefaultNotificationBody(lang, "escalation_update", NotificationTypeLabel(lang, "escalation_update"))
	}
	return title, body
}

// ModelX46PropertyMatchFallback returns template push copy when AI personalization is unavailable.
func ModelX46PropertyMatchFallback(lang, propertyTitle, location string) (title, body string) {
	lang = NormalizeNotificationLang(lang)
	if lang == "" {
		lang = "en"
	}
	title = strings.TrimSpace(propertyTitle)
	if title == "" {
		title = NotificationTypeLabel(lang, "property_match")
	}
	loc := strings.TrimSpace(location)
	switch lang {
	case "ar":
		if loc != "" {
			body = fmt.Sprintf("عقار يطابق اهتماماتك في %s. اضغط للاطلاع.", loc)
		} else {
			body = "عقار جديد يطابق تفضيلاتك. اضغط للاطلاع."
		}
	case "fr":
		if loc != "" {
			body = fmt.Sprintf("Un bien correspond à vos critères à %s. Touchez pour voir.", loc)
		} else {
			body = "Un bien correspond à vos critères. Touchez pour voir."
		}
	default:
		if loc != "" {
			body = fmt.Sprintf("A property matches your preferences in %s. Tap to view.", loc)
		} else {
			body = "A property matches your preferences. Tap to view."
		}
	}
	return title, body
}

// ModelX46MarketAlertFallback returns template market-alert push copy.
func ModelX46MarketAlertFallback(lang string) (title, body string) {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return "تنبيه السوق", "تحديث جديد قد يهم بحثك العقاري. اضغط للاطلاع."
	case "fr":
		return "Alerte marché", "Une mise à jour pourrait intéresser votre recherche. Touchez pour voir."
	default:
		return "Market alert", "A new update may matter for your property search. Tap to view."
	}
}
