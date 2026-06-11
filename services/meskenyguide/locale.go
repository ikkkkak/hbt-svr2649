package meskenyguide

import (
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// ResolveHostLocale returns en | fr | ar for guide copy generation.
func ResolveHostLocale(hostID uint) string {
	var pref models.NotificationPreference
	if err := storage.DB.Where("user_id = ?", hostID).
		Order("updated_at DESC").
		First(&pref).Error; err == nil {
		if lang := normalizeLocale(pref.Language); lang != "" {
			return lang
		}
	}
	return "en"
}

// NormalizeLocalePublic is used by HTTP handlers (X-App-Locale header).
func NormalizeLocalePublic(raw string) string {
	return normalizeLocale(raw)
}

func normalizeLocale(raw string) string {
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

func disclaimerForLocale(lang string) string {
	switch normalizeLocale(lang) {
	case "ar":
		return "\n\nهذا توجيه خوارزمي مبني على بيانات المنصة، وليس نصيحة مالية أو قانونية."
	case "fr":
		return "\n\nIl s'agit d'un conseil algorithmique basé sur les données de la plateforme, pas d'un avis financier ou juridique."
	default:
		return "\n\nThis is algorithmic guidance based on platform data, not financial or legal advice."
	}
}

func llmLanguageInstruction(lang string) string {
	switch normalizeLocale(lang) {
	case "ar":
		return "Write ALL JSON string values in Modern Standard Arabic. Keep numbers and percentages as digits."
	case "fr":
		return "Write ALL JSON string values in French."
	default:
		return "Write ALL JSON string values in English."
	}
}
