package listing_ai

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var structuredDetailLine = regexp.MustCompile(`(?m)^(?:نوع العقار|Type de bien|Property type|سنة البناء|Année de construction|Year built|غرف النوم|Chambres|Bedrooms|الحمامات|Salles de bain|Bathrooms|المساحة|Surface|Area|Prix|Price|السعر|Numéro de parcelle|Plot number|رقم القطعة)\s*:`)

// extractUserStory strips auto-appended structured lines; keeps only the user's free-text notes.
func extractUserStory(details string) string {
	details = strings.TrimSpace(details)
	if details == "" {
		return ""
	}
	idx := structuredDetailLine.FindStringIndex(details)
	if idx != nil && idx[0] > 0 {
		return strings.TrimSpace(details[:idx[0]])
	}
	return details
}

func normalizeForCompare(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isEchoOfUserInput detects when the model pasted the user's notes instead of rewriting.
func isEchoOfUserInput(llm llmDraft, userStory string) bool {
	story := normalizeForCompare(userStory)
	if story == "" {
		return false
	}
	titleN := normalizeForCompare(llm.Title)
	descN := normalizeForCompare(llm.Description)

	if titleN != "" && (titleN == story || strings.Contains(story, titleN) && len(titleN) > len(story)*7/10) {
		return true
	}
	if descN == "" {
		return false
	}
	if descN == story {
		return true
	}
	// Description contains nearly all of the user story unchanged.
	if len(story) >= 20 && strings.Contains(descN, story) {
		return true
	}
	if len(story) >= 40 {
		prefix := story
		if len(prefix) > 80 {
			prefix = prefix[:80]
		}
		if strings.Contains(descN, prefix) {
			return true
		}
	}
	// Title is literally the first line of user story.
	firstLine := userStory
	if i := strings.IndexAny(userStory, "\n"); i > 0 {
		firstLine = userStory[:i]
	}
	if normalizeForCompare(firstLine) == titleN && len(titleN) > 15 {
		return true
	}
	return false
}

func buildTemplateDraft(in GenerateInput) llmDraft {
	lang := ResolveListingLanguage(in)
	place := locationLabel(in)
	ptype := strings.TrimSpace(in.PropertyType)
	if ptype == "" && in.Kind == KindLand {
		ptype = landTypeLabel(lang)
	}

	title := templateTitle(lang, in.Kind, ptype, place, in)
	desc := templateDescription(lang, in, place)
	hood := templateNeighborhood(lang, place)

	indoor, outdoor := templateFeatures(lang, in)

	return llmDraft{
		Title:                   title,
		Description:             desc,
		CityName:                in.CityHint,
		ZoneName:                in.ZoneHint,
		QuartierName:            in.QuartierHint,
		NeighborhoodDescription: hood,
		IndoorFeatures:          indoor,
		OutdoorFeatures:         outdoor,
		PlotNumber:              strings.TrimSpace(in.PlotNumber),
	}
}

func locationLabel(in GenerateInput) string {
	parts := []string{
		strings.TrimSpace(in.QuartierHint),
		strings.TrimSpace(in.ZoneHint),
		strings.TrimSpace(in.CityHint),
	}
	out := make([]string, 0, 3)
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func landTypeLabel(lang string) string {
	switch lang {
	case "ar":
		return "أرض"
	case "en":
		return "Land"
	default:
		return "Terrain"
	}
}

func templateTitle(lang string, kind Kind, ptype, place string, in GenerateInput) string {
	area := ""
	if in.Area > 0 {
		area = formatArea(in.Area, in.AreaUnit)
	}
	beds := ""
	if in.Bedrooms != nil && *in.Bedrooms > 0 {
		beds = intToStr(*in.Bedrooms)
	}

	switch lang {
	case "ar":
		switch kind {
		case KindLand:
			if area != "" && place != "" {
				return trimRunes("أرض للبيع — "+area+" في "+place, 90)
			}
			return trimRunes("أرض للبيع — "+place, 90)
		case KindRent:
			if beds != "" && place != "" {
				return trimRunes("للإيجار: "+ptype+" "+beds+" غرف — "+place, 90)
			}
			return trimRunes("للإيجار — "+place, 90)
		default:
			if ptype != "" && place != "" {
				return trimRunes(ptype+" للبيع — "+place, 90)
			}
			return trimRunes("عقار للبيع — "+place, 90)
		}
	case "en":
		switch kind {
		case KindLand:
			return trimRunes("Land for sale — "+joinNonEmpty(area, place), 90)
		case KindRent:
			return trimRunes("For rent — "+joinNonEmpty(ptype, place), 90)
		default:
			return trimRunes(joinNonEmpty(ptype, "for sale")+" — "+place, 90)
		}
	default:
		switch kind {
		case KindLand:
			return trimRunes("Terrain à vendre — "+joinNonEmpty(area, place), 90)
		case KindRent:
			return trimRunes("Location — "+joinNonEmpty(ptype, place), 90)
		default:
			return trimRunes(joinNonEmpty(ptype, "à vendre")+" — "+place, 90)
		}
	}
}

func templateDescription(lang string, in GenerateInput, place string) string {
	facts := collectFactLines(lang, in, place)
	var paras []string

	switch lang {
	case "ar":
		paras = append(paras, openingLineAR(in.Kind, place))
		if len(facts) > 0 {
			paras = append(paras, "يتميز هذا العقار بـ "+strings.Join(facts, "، ")+".")
		}
		paras = append(paras, "للمزيد من المعلومات أو لترتيب زيارة، يرجى التواصل معنا.")
	case "en":
		paras = append(paras, openingLineEN(in.Kind, place))
		if len(facts) > 0 {
			paras = append(paras, "Key details: "+strings.Join(facts, ", ")+".")
		}
		paras = append(paras, "Contact us for more information or to schedule a viewing.")
	default:
		paras = append(paras, openingLineFR(in.Kind, place))
		if len(facts) > 0 {
			paras = append(paras, "Points clés : "+strings.Join(facts, ", ")+".")
		}
		paras = append(paras, "Contactez-nous pour plus d'informations ou organiser une visite.")
	}
	return strings.Join(paras, "\n\n")
}

func openingLineAR(kind Kind, place string) string {
	switch kind {
	case KindLand:
		if place != "" {
			return "فرصة لاقتناء أرض في " + place + "."
		}
		return "فرصة لاقتناء أرض في موقع مميز."
	case KindRent:
		if place != "" {
			return "إذا كنت تبحث عن سكن مريح في " + place + "، فهذا العقار قد يناسب احتياجاتك."
		}
		return "إذا كنت تبحث عن سكن مريح، فهذا العقار قد يناسب احتياجاتك."
	default:
		if place != "" {
			return "فرصة مميزة لامتلاك عقار في " + place + "."
		}
		return "فرصة مميزة لامتلاك عقار في موقع جيد."
	}
}

func openingLineEN(kind Kind, place string) string {
	switch kind {
	case KindLand:
		return "An opportunity to acquire land" + suffixPlaceEN(place) + "."
	case KindRent:
		return "A rental opportunity" + suffixPlaceEN(place) + " for comfortable living."
	default:
		return "A property for sale" + suffixPlaceEN(place) + " worth exploring."
	}
}

func openingLineFR(kind Kind, place string) string {
	switch kind {
	case KindLand:
		return "Occasion d'acquérir un terrain" + suffixPlaceFR(place) + "."
	case KindRent:
		return "Bien en location" + suffixPlaceFR(place) + ", adapté à un séjour confortable."
	default:
		return "Bien à vendre" + suffixPlaceFR(place) + ", à découvrir."
	}
}

func suffixPlaceEN(place string) string {
	if place == "" {
		return ""
	}
	return " in " + place
}

func suffixPlaceFR(place string) string {
	if place == "" {
		return ""
	}
	return " à " + place
}

func collectFactLines(lang string, in GenerateInput, place string) []string {
	var lines []string
	if in.Area > 0 {
		switch lang {
		case "ar":
			lines = append(lines, "مساحة "+formatArea(in.Area, in.AreaUnit))
		case "en":
			lines = append(lines, formatArea(in.Area, in.AreaUnit))
		default:
			lines = append(lines, "surface "+formatArea(in.Area, in.AreaUnit))
		}
	}
	if in.Bedrooms != nil && *in.Bedrooms > 0 {
		switch lang {
		case "ar":
			lines = append(lines, intToStr(*in.Bedrooms)+" غرف نوم")
		case "en":
			lines = append(lines, intToStr(*in.Bedrooms)+" bedrooms")
		default:
			lines = append(lines, intToStr(*in.Bedrooms)+" chambres")
		}
	}
	if in.Bathrooms != nil && *in.Bathrooms > 0 {
		switch lang {
		case "ar":
			lines = append(lines, intToStr(*in.Bathrooms)+" حمام")
		case "en":
			lines = append(lines, intToStr(*in.Bathrooms)+" bathrooms")
		default:
			lines = append(lines, intToStr(*in.Bathrooms)+" salles de bain")
		}
	}
	if in.Price > 0 {
		switch lang {
		case "ar":
			lines = append(lines, "سعر "+formatPrice(in.Price, in.Currency))
		case "en":
			lines = append(lines, "price "+formatPrice(in.Price, in.Currency))
		default:
			lines = append(lines, "prix "+formatPrice(in.Price, in.Currency))
		}
	}
	if strings.TrimSpace(in.PropertyType) != "" {
		lines = append(lines, strings.TrimSpace(in.PropertyType))
	}
	if len(in.AmenityNames) > 0 {
		lines = append(lines, strings.Join(in.AmenityNames, ", "))
	}
	_ = place
	return lines
}

func templateNeighborhood(lang, place string) string {
	if place == "" {
		return ""
	}
	switch lang {
	case "ar":
		return "يقع العقار في " + place + "، منطقة معروفة في نواكشوط."
	case "en":
		return "Located in " + place + ", a well-known area in Nouakchott."
	default:
		return "Situé à " + place + ", un quartier connu de Nouakchott."
	}
}

func templateFeatures(lang string, in GenerateInput) (indoor, outdoor []string) {
	if in.Kind != KindSale {
		return nil, nil
	}
	for _, n := range in.AmenityNames {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		l := strings.ToLower(n)
		if strings.Contains(l, "pool") || strings.Contains(l, "piscine") || strings.Contains(l, "garden") || strings.Contains(l, "jardin") {
			outdoor = append(outdoor, n)
		} else {
			indoor = append(indoor, n)
		}
	}
	return indoor, outdoor
}

func joinNonEmpty(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func intToStr(n int) string {
	return fmt.Sprintf("%d", n)
}

func trimRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max])
}
