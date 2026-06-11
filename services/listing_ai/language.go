package listing_ai

import (
	"strings"
	"unicode"

	"apartments-clone-server/services"
)

// DetectOutputLanguage infers listing copy language ONLY from user-written text
// (description + location hints). App UI locale is intentionally ignored.
func DetectOutputLanguage(parts ...string) string {
	blob := strings.TrimSpace(strings.Join(parts, " "))
	if blob == "" {
		return "fr"
	}

	// Any Arabic script in the user's text → Arabic output (strict).
	for _, r := range blob {
		if unicode.In(r, unicode.Arabic) {
			return "ar"
		}
	}

	switch services.DetectLang(blob) {
	case services.LangAR, services.LangHASSANIYA:
		return "ar"
	case services.LangEN:
		return "en"
	default:
		return "fr"
	}
}

func languagePromptBlock(lang string) string {
	switch lang {
	case "ar":
		return `LANGUAGE (mandatory): Arabic only (العربية).
- title, description, neighborhood_description MUST be in Arabic.
- Do NOT use French, English, or Latin letters in those fields (digits and MRU are OK).
- indoor_features / outdoor_features: Arabic short tags only.`
	case "en":
		return `LANGUAGE (mandatory): English only.
- title, description, neighborhood_description MUST be in English.
- Do NOT use French or Arabic in those fields.`
	default:
		return `LANGUAGE (mandatory): French only (français).
- title, description, neighborhood_description MUST be in French.
- Do NOT use English or Arabic in those fields.`
	}
}
