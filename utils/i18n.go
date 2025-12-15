package utils

import (
	"encoding/json"

	"gorm.io/datatypes"
)

// ResolveLocalizedText picks the best localized version from a JSONB map
// of translations of the form { "en": "...", "fr": "...", "ar": "..." }.
// If the requested language is missing, it falls back to English, then to raw.
func ResolveLocalizedText(raw string, translations datatypes.JSON, lang string) string {
	if raw == "" && len(translations) == 0 {
		return ""
	}

	m := map[string]string{}
	if len(translations) > 0 {
		_ = json.Unmarshal(translations, &m)
	}

	switch lang {
	case "en", "fr", "ar":
	default:
		lang = "en"
	}

	if v, ok := m[lang]; ok && v != "" {
		return v
	}
	if v, ok := m["en"]; ok && v != "" {
		return v
	}
	return raw
}


