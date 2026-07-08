package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"strings"
)

// ResolveUserNotificationLang returns en | fr | ar for push/in-app copy.
func ResolveUserNotificationLang(userID uint) string {
	if userID == 0 {
		return "en"
	}
	var pref models.NotificationPreference
	if err := storage.DB.Where("user_id = ?", userID).
		Order("updated_at DESC").
		First(&pref).Error; err == nil {
		if lang := NormalizeNotificationLang(pref.Language); lang != "" {
			return lang
		}
	}
	return "en"
}

// NormalizeNotificationLang maps device / preference locale to supported push languages.
func NormalizeNotificationLang(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(s, "ar"):
		return "ar"
	case strings.HasPrefix(s, "fr"):
		return "fr"
	case strings.HasPrefix(s, "en"):
		return "en"
	default:
		return ""
	}
}

func normalizeNotificationTypeKey(typ string) string {
	t := strings.TrimSpace(strings.ToLower(typ))
	t = strings.TrimPrefix(t, "smart_")
	return t
}

func looksLikeRawNotificationKey(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.Contains(s, " ") || strings.Contains(s, "•") || strings.Contains(s, "—") {
		return false
	}
	// snake_case internal keys, e.g. property_status_changed
	if strings.Contains(s, "_") {
		return true
	}
	lower := strings.ToLower(s)
	return s == lower && len(s) > 3
}

// NotificationTypeLabel returns a short human label for a notification type key.
func NotificationTypeLabel(lang, notificationType string) string {
	key := normalizeNotificationTypeKey(notificationType)
	labels := notificationTypeLabels(lang)
	if label, ok := labels[key]; ok && label != "" {
		return label
	}
	// Fallback: replace underscores with spaces and title-case lightly
	human := strings.ReplaceAll(key, "_", " ")
	if human == "" {
		switch lang {
		case "ar":
			return "إشعار"
		case "fr":
			return "Notification"
		default:
			return "Notification"
		}
	}
	return human
}

func notificationTypeLabels(lang string) map[string]string {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return map[string]string{
			"property_status_changed":         "تحديث حالة العقار",
			"host_mode_reminder":              "تذكير وضع المضيف",
			"reservation_request":             "طلب حجز جديد",
			"message_received":                "رسالة جديدة",
			"reservation_reminder":            "تذكير الحجز",
			"experience_booked":               "حجز تجربة",
			"reservation_accepted":            "تم قبول الحجز",
			"reservation_rejected":            "تم رفض الحجز",
			"welcome":                         "مرحبًا بك",
			"property_offer":                  "عرض شراء",
			"property_tour":                   "طلب زيارة",
			"new_property":                    "عقار جديد",
			"price_drop":                      "انخفاض السعر",
			"existing_property":               "عقار متاح",
			"new_video_for_property":          "فيديو جديد",
			"property_view_milestone":           "إنجاز المشاهدات",
			"property_popularity_alert":       "عقار شائع",
			"rent_suggestion":                 "اقتراح إيجار",
			"continue_browsing":               "متابعة التصفح",
			"trending_properties":             "عقارات رائجة",
			"weekly_digest":                   "ملخص أسبوعي",
			"nearby_property":                 "عقار قريب",
			"viewed_property_reminder":        "ما زال متاحًا",
			"still_available":                 "ما زال متاحًا",
			"similar_properties":              "عقارات مشابهة",
			"reengage_digest":                 "عقارات جديدة",
			"investment_opportunity":          "فرصة استثمار",
			"landmark_investment_opportunity": "فرصة أرض",
			"discovery_spotlight":             "اكتشافات لك",
			"discovery_landmark":              "أرض مميزة",
			"location_discovery":              "اكتشف العقارات",
			"meskeny_guide":                   "دليل Meskeny",
			"property_match":                  "عقار يناسبك",
			"escalation_update":               "تصعيد إلى مختص",
			"market_alert":                    "تنبيه السوق",
			"ai_property_match":               "اقتراح ذكي",
			"group_join_request":              "طلب انضمام",
			"group_join_accepted":             "تم قبول الانضمام",
			"group_join_declined":             "تم رفض الانضمام",
			"notification_digest":             "تحديثات متعددة",
			"suggestion_digest":               "تحديثات متعددة",
			"host_contact":                    "رسالة من المضيف",
			"property_inquiry":                "استفسار عن عقار",
			"experience_booking_confirmed":    "تأكيد الحجز",
		}
	case "fr":
		return map[string]string{
			"property_status_changed":         "Mise à jour du bien",
			"host_mode_reminder":              "Rappel mode hôte",
			"reservation_request":             "Nouvelle demande de réservation",
			"message_received":                "Nouveau message",
			"reservation_reminder":            "Rappel de réservation",
			"experience_booked":               "Expérience réservée",
			"reservation_accepted":            "Réservation acceptée",
			"reservation_rejected":            "Réservation refusée",
			"welcome":                         "Bienvenue",
			"property_offer":                  "Offre d'achat",
			"property_tour":                   "Demande de visite",
			"new_property":                    "Nouveau bien",
			"price_drop":                      "Baisse de prix",
			"existing_property":               "Bien disponible",
			"new_video_for_property":          "Nouvelle vidéo",
			"property_view_milestone":         "Palier de vues",
			"property_popularity_alert":       "Bien populaire",
			"rent_suggestion":                 "Location suggérée",
			"continue_browsing":               "Reprendre votre recherche",
			"trending_properties":             "Tendances locales",
			"weekly_digest":                   "Sélection de la semaine",
			"nearby_property":                 "Bien à proximité",
			"viewed_property_reminder":        "Toujours disponible",
			"still_available":                 "Toujours disponible",
			"similar_properties":              "Biens similaires",
			"reengage_digest":                 "Nouveautés pour vous",
			"investment_opportunity":          "Opportunité d'investissement",
			"landmark_investment_opportunity": "Opportunité foncière",
			"discovery_spotlight":             "Coups de cœur",
			"discovery_landmark":              "Terrain remarquable",
			"location_discovery":              "Découvrir des biens",
			"meskeny_guide":                   "Guide Meskeny",
			"property_match":                  "Bien pour vous",
			"escalation_update":               "Escalade vers un spécialiste",
			"market_alert":                    "Alerte marché",
			"ai_property_match":               "Suggestion intelligente",
			"group_join_request":              "Demande d'adhésion",
			"group_join_accepted":             "Adhésion acceptée",
			"group_join_declined":             "Adhésion refusée",
			"notification_digest":             "Plusieurs mises à jour",
			"suggestion_digest":               "Plusieurs mises à jour",
			"host_contact":                    "Message de l'hôte",
			"property_inquiry":                "Demande sur un bien",
			"experience_booking_confirmed":    "Réservation confirmée",
		}
	default:
		return map[string]string{
			"property_status_changed":         "Property status update",
			"host_mode_reminder":              "Host mode reminder",
			"reservation_request":             "New reservation request",
			"message_received":                "New message",
			"reservation_reminder":            "Reservation reminder",
			"experience_booked":               "Experience booked",
			"reservation_accepted":            "Reservation accepted",
			"reservation_rejected":            "Reservation declined",
			"welcome":                         "Welcome",
			"property_offer":                  "Purchase offer",
			"property_tour":                   "Tour request",
			"new_property":                    "New property",
			"price_drop":                      "Price drop",
			"existing_property":               "Property available",
			"new_video_for_property":          "New video",
			"property_view_milestone":         "Views milestone",
			"property_popularity_alert":       "Popular property",
			"rent_suggestion":                 "Rental suggestion",
			"continue_browsing":               "Continue browsing",
			"trending_properties":             "Trending near you",
			"weekly_digest":                   "Weekly picks",
			"nearby_property":                 "Property nearby",
			"viewed_property_reminder":        "Still available",
			"still_available":                 "Still available",
			"similar_properties":              "Similar homes",
			"reengage_digest":                 "New for you",
			"investment_opportunity":          "Investment opportunity",
			"landmark_investment_opportunity": "Land opportunity",
			"discovery_spotlight":             "Spotlight picks",
			"discovery_landmark":              "Featured land",
			"location_discovery":              "Discover properties",
			"meskeny_guide":                   "Meskeny Guide",
			"property_match":                  "Property match",
			"escalation_update":               "Specialist handoff",
			"market_alert":                    "Market alert",
			"ai_property_match":               "Smart suggestion",
			"group_join_request":              "Join request",
			"group_join_accepted":             "Join accepted",
			"group_join_declined":             "Join declined",
			"notification_digest":             "Multiple updates",
			"suggestion_digest":               "Multiple updates",
			"host_contact":                    "Message from host",
			"property_inquiry":                "Property inquiry",
			"experience_booking_confirmed":    "Booking confirmed",
		}
	}
}

// EnsureNotificationCopy replaces raw internal keys with localized human copy.
func EnsureNotificationCopy(lang, notificationType, title, body string) (string, string) {
	lang = NormalizeNotificationLang(lang)
	if lang == "" {
		lang = "en"
	}
	typ := strings.TrimSpace(notificationType)
	label := NotificationTypeLabel(lang, typ)

	outTitle := strings.TrimSpace(title)
	outBody := strings.TrimSpace(body)

	if looksLikeRawNotificationKey(outTitle) || outTitle == typ {
		outTitle = label
	}
	if outBody == "" || looksLikeRawNotificationKey(outBody) || outBody == typ {
		outBody = DefaultNotificationBody(lang, typ, label)
	}
	return sanitizePostgreSQLText(outTitle), sanitizePostgreSQLText(outBody)
}

// sanitizePostgreSQLText strips invalid UTF-8 sequences (e.g. from byte-truncated Arabic).
func sanitizePostgreSQLText(s string) string {
	return strings.ToValidUTF8(strings.TrimSpace(s), "")
}

func DefaultNotificationBody(lang, typ, label string) string {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return fmt.Sprintf("لديك تحديث: %s. اضغط للاطلاع.", label)
	case "fr":
		return fmt.Sprintf("Vous avez une mise à jour : %s. Touchez pour voir.", label)
	default:
		return fmt.Sprintf("You have an update: %s. Tap to view.", label)
	}
}

// NotificationDigestCopy builds localized batch digest push text.
func NotificationDigestCopy(lang string, count int, notificationTypes []string) (string, string) {
	lang = NormalizeNotificationLang(lang)
	if lang == "" {
		lang = "en"
	}
	labels := make([]string, 0, len(notificationTypes))
	for _, typ := range notificationTypes {
		labels = append(labels, NotificationTypeLabel(lang, typ))
	}
	joined := strings.Join(labels, ", ")

	switch lang {
	case "ar":
		return "Meskeny — تحديثات جديدة", fmt.Sprintf("%d إشعارات: %s", count, joined)
	case "fr":
		return "Meskeny — plusieurs mises à jour", fmt.Sprintf("%d notifications : %s", count, joined)
	default:
		return "Meskeny — new updates", fmt.Sprintf("%d notifications: %s", count, joined)
	}
}

func PropertyStatusChangedCopy(lang, status, propertyTitle string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		switch NormalizeNotificationLang(lang) {
		case "ar":
			return "✅ تمت الموافقة على العقار", fmt.Sprintf("تهانينا! تمت الموافقة على \"%s\" وهو الآن مرئي.", propertyTitle)
		case "fr":
			return "✅ Bien approuvé", fmt.Sprintf("Félicitations ! Votre bien \"%s\" est approuvé et visible.", propertyTitle)
		default:
			return "✅ Property approved", fmt.Sprintf("Congratulations! Your property \"%s\" is approved and live.", propertyTitle)
		}
	case "rejected":
		switch NormalizeNotificationLang(lang) {
		case "ar":
			return "❌ تم رفض العقار", fmt.Sprintf("تم رفض \"%s\". راجع التفاصيل وأعد الإرسال.", propertyTitle)
		case "fr":
			return "❌ Bien refusé", fmt.Sprintf("Votre bien \"%s\" a été refusé. Vérifiez les détails et soumettez à nouveau.", propertyTitle)
		default:
			return "❌ Property rejected", fmt.Sprintf("Your property \"%s\" was rejected. Review details and resubmit.", propertyTitle)
		}
	case "under_review":
		switch NormalizeNotificationLang(lang) {
		case "ar":
			return "🔍 العقار قيد المراجعة", fmt.Sprintf("\"%s\" قيد المراجعة من فريقنا.", propertyTitle)
		case "fr":
			return "🔍 Bien en cours de révision", fmt.Sprintf("Votre bien \"%s\" est en cours de révision.", propertyTitle)
		default:
			return "🔍 Property under review", fmt.Sprintf("Your property \"%s\" is being reviewed by our team.", propertyTitle)
		}
	default:
		switch NormalizeNotificationLang(lang) {
		case "ar":
			return "🏠 تحديث العقار", fmt.Sprintf("تم تحديث حالة \"%s\" إلى %s.", propertyTitle, status)
		case "fr":
			return "🏠 Mise à jour du bien", fmt.Sprintf("Le statut de \"%s\" a été mis à jour : %s.", propertyTitle, status)
		default:
			return "🏠 Property update", fmt.Sprintf("The status of \"%s\" was updated to %s.", propertyTitle, status)
		}
	}
}

func HostModeReminderCopy(lang, userName string) (string, string) {
	name := strings.TrimSpace(userName)
	switch NormalizeNotificationLang(lang) {
	case "ar":
		if name == "" {
			name = "عزيزي المستخدم"
		}
		return "🏠 ابدأ رحلتك كمالك عقار!", fmt.Sprintf("مرحباً %s! أضف عقارك الأول في أقل من دقيقتين وابدأ في استقبال الحجوزات.", name)
	case "fr":
		if name == "" {
			name = "there"
		}
		return "🏠 Publiez votre premier bien", fmt.Sprintf("Bonjour %s ! Ajoutez votre bien en moins de 2 minutes et recevez des réservations.", name)
	default:
		if name == "" {
			name = "there"
		}
		return "🏠 List your first property", fmt.Sprintf("Hi %s! Add your property in under 2 minutes and start getting bookings.", name)
	}
}

func SmartContinueBrowsingCopy(lang string) (string, string) {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return "تابع من حيث توقفت", "عقارات جديدة بانتظارك في منطقتك."
	case "fr":
		return "Reprenez votre recherche", "De nouveaux biens vous attendent dans votre zone."
	default:
		return "Continue where you left off", "New properties are waiting for you in your area."
	}
}

func SmartTrendingCopy(lang string) (string, string) {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return "الأكثر رواجًا في منطقتك", "هذه العقارات تحظى باهتمام كبير. اطلع عليها الآن."
	case "fr":
		return "Tendances dans votre zone", "Ces biens attirent beaucoup d'attention. Découvrez-les."
	default:
		return "Trending in your area", "These properties are getting a lot of attention. Check them now."
	}
}

func SmartWeeklyDigestCopy(lang, propertyTitle string) (string, string) {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return "أفضل العقارات هذا الأسبوع", fmt.Sprintf("اختيارات لك: %s", propertyTitle)
	case "fr":
		return "Meilleurs biens de la semaine", fmt.Sprintf("Notre sélection pour vous : %s", propertyTitle)
	default:
		return "Top properties this week", fmt.Sprintf("Best picks for you: %s", propertyTitle)
	}
}

func SmartNearbyCopy(lang, propertyTitle string, distKm float64) (string, string) {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return "عقار جديد بالقرب منك", fmt.Sprintf("%s على بعد %.1f كم", propertyTitle, distKm)
	case "fr":
		return "Nouveau bien près de vous", fmt.Sprintf("%s à %.1f km", propertyTitle, distKm)
	default:
		return "New property near you", fmt.Sprintf("%s is %.1f km away", propertyTitle, distKm)
	}
}

func SmartRentSuggestionCopy(lang, propertyTitle string) (string, string) {
	switch NormalizeNotificationLang(lang) {
	case "ar":
		return "🏠 عقار للإيجار", fmt.Sprintf("شاهد هذا: %s", propertyTitle)
	case "fr":
		return "🏠 Location disponible", fmt.Sprintf("À découvrir : %s", propertyTitle)
	default:
		return "🏠 Property for rent", fmt.Sprintf("Check this out: %s", propertyTitle)
	}
}
