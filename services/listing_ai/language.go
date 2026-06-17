package listing_ai

import (
	"strings"
	"unicode"

	"apartments-clone-server/services"
)

// DetectOutputLanguage infers listing copy language from user-written text
// (description + location hints). App UI locale is not used here.
func DetectOutputLanguage(parts ...string) string {
	blob := strings.TrimSpace(strings.Join(parts, " "))
	if blob == "" {
		return "fr"
	}

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

func normalizeLangCode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ar", "arabic", "ar-ma":
		return "ar"
	case "en", "english", "en-us", "en-gb":
		return "en"
	case "fr", "french", "fr-fr":
		return "fr"
	default:
		return ""
	}
}

// ResolveListingLanguage picks output language: user story wins, then explicit hint, then detection.
func ResolveListingLanguage(in GenerateInput) string {
	details := strings.TrimSpace(in.Details)
	detected := DetectOutputLanguage(in.Details, in.CityHint, in.ZoneHint, in.QuartierHint)
	explicit := normalizeLangCode(in.Language)

	if len([]rune(details)) >= 12 {
		return detected
	}
	if explicit != "" {
		return explicit
	}
	return detected
}

func languagePromptBlock(lang string) string {
	switch lang {
	case "ar":
		return `LANGUAGE (strict — mandatory):
- title, description, neighborhood_description, indoor_features, outdoor_features: Arabic only (العربية).
- Do NOT mix French or English words in narrative fields (MRU, m², and digits are OK).
- Match the tone of the user's Arabic notes — formal but warm.
- Example title style: «شقة 3 غرف في تفرغ زينة — مطبخ حديث وموقف سيارات»`
	case "en":
		return `LANGUAGE (strict — mandatory):
- title, description, neighborhood_description, indoor_features, outdoor_features: English only.
- Do NOT mix French or Arabic in narrative fields.
- Match the user's English tone — clear and professional.
- Example title style: "Modern 3-bed apartment in Tevragh Zeina — renovated kitchen"`
	default:
		return `LANGUAGE (strict — mandatory):
- title, description, neighborhood_description, indoor_features, outdoor_features: French only (français).
- Do NOT mix English or Arabic in narrative fields.
- Match the user's French tone — professional and inviting.
- Example title style: «Appartement 3 chambres à Tevragh Zeina — cuisine rénovée»`
	}
}
