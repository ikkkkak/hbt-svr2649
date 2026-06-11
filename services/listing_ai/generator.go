package listing_ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"apartments-clone-server/services"
)

// Generator builds listing drafts using OpenRouter + location catalog.
type Generator struct {
	AI *services.AIService
}

// NewGenerator creates a listing AI generator.
func NewGenerator(ai *services.AIService) *Generator {
	return &Generator{AI: ai}
}

type llmDraft struct {
	Title                   string   `json:"title"`
	Description             string   `json:"description"`
	CityName                string   `json:"city_name"`
	ZoneName                string   `json:"zone_name"`
	QuartierName            string   `json:"quartier_name"`
	NeighborhoodDescription string   `json:"neighborhood_description"`
	IndoorFeatures          []string `json:"indoor_features"`
	OutdoorFeatures         []string `json:"outdoor_features"`
	PlotNumber              string   `json:"plot_number"`
}

// Generate runs the full pipeline: match location → LLM copy → merge draft.
func (g *Generator) Generate(in GenerateInput) (*Draft, error) {
	catalog, err := GetLocationCatalog()
	if err != nil {
		return nil, fmt.Errorf("location catalog: %w", err)
	}
	filtered := FilterCatalogForInput(catalog, in)
	maxLines := 50
	if in.CityHint != "" || in.ZoneHint != "" || in.QuartierHint != "" {
		maxLines = 32
	}

	llm, err := g.callLLM(in, BuildCatalogSummary(filtered, maxLines))
	if err != nil {
		llm = fallbackLLMDraft(in)
	}

	// Match using form fields + description + LLM-extracted names (quartier-first).
	loc, conf := MatchLocationFromInput(filtered, in, llm.CityName, llm.ZoneName, llm.QuartierName)

	draft := &Draft{
		Title:                   strings.TrimSpace(llm.Title),
		Description:             strings.TrimSpace(llm.Description),
		NeighborhoodDescription: strings.TrimSpace(llm.NeighborhoodDescription),
		IndoorFeatures:          llm.IndoorFeatures,
		OutdoorFeatures:         llm.OutdoorFeatures,
		ImageURLs:               append([]string(nil), in.ImageURLs...),
		VideoURLs:               append([]string(nil), in.VideoURLs...),
		LocationMatchConfidence: conf,
		Area:                    in.Area,
		AreaUnit:                in.AreaUnit,
		Price:                   in.Price,
		LandType:                in.LandType,
	}

	if in.Kind == KindRent {
		draft.PropertyType = "entire_place"
		if in.PropertyCategoryID > 0 {
			draft.PropertyCategoryID = in.PropertyCategoryID
		}
	} else {
		draft.PropertyType = in.PropertyType
	}

	if loc.CityID > 0 {
		draft.CityID = loc.CityID
		draft.CityName = loc.CityName
	} else if llm.CityName != "" {
		draft.CityName = llm.CityName
	}
	if loc.ZoneID > 0 {
		draft.ZoneID = loc.ZoneID
		draft.ZoneName = loc.ZoneName
	} else if llm.ZoneName != "" {
		draft.ZoneName = llm.ZoneName
	}
	if loc.QuartierID > 0 {
		draft.QuartierID = loc.QuartierID
		draft.QuartierName = loc.QuartierName
	} else if llm.QuartierName != "" {
		draft.QuartierName = llm.QuartierName
	}

	if in.Bedrooms != nil {
		draft.Bedrooms = *in.Bedrooms
	}
	if in.Bathrooms != nil {
		draft.Bathrooms = *in.Bathrooms
	}

	switch in.Kind {
	case KindRent:
		draft.NightlyPrice = in.Price
		draft.PropertyType = "entire_place"
	default:
		draft.Price = in.Price
	}

	if in.Latitude != nil {
		draft.Latitude = *in.Latitude
	}
	if in.Longitude != nil {
		draft.Longitude = *in.Longitude
	}

	if draft.Title == "" {
		draft.Title = fallbackTitle(in)
	}
	if draft.Description == "" {
		draft.Description = fallbackDescription(in, draft)
	}
	draft.Description = sanitizeListingDescription(draft.Description)

	if len(in.AmenityIDs) > 0 {
		draft.AmenityIDs = append([]uint(nil), in.AmenityIDs...)
	}

	draft.PlotNumber = resolvePlotNumber(in, llm)

	return draft, nil
}

func (g *Generator) callLLM(in GenerateInput, catalogSummary string) (llmDraft, error) {
	lang := resolveListingLanguage(in)

	kindLabel := string(in.Kind)
	schema := `{"title":"","description":"","city_name":"","zone_name":"","quartier_name":"","neighborhood_description":"","indoor_features":[],"outdoor_features":[]}`
	plotRules := ""
	if in.Kind == KindLand {
		schema = `{"title":"","description":"","city_name":"","zone_name":"","quartier_name":"","neighborhood_description":"","indoor_features":[],"outdoor_features":[],"plot_number":""}`
		plotRules = `
- plot_number: cadastre parcel number ONLY (digits/alphanumeric as on title deed). Copy exactly from user text when mentioned. Leave empty if not stated — never invent.`
	}
	system := `You are Meskeny Listing Agent — a professional real-estate copywriter for Mauritania (Meskeny app).
Return ONLY valid JSON (no markdown fences) matching this schema:
` + schema + `

Rules:
- title: max 90 chars, clear, no ALL CAPS
- description: 2-4 short paragraphs, professional, honest, no fake claims
- NEVER mention numeric amenity IDs, internal codes, or "amenity IDs selected" in description
- If amenities are provided, weave them naturally by name (e.g. Wi‑Fi, parking) — do not list raw IDs
- city_name, zone_name, quartier_name: MUST copy EXACT catalog spellings from the list below when the user mentioned that place (including quartier in their description). Never invent a quartier name.
- neighborhood_description: 1-2 sentences about the area vibe
- indoor_features / outdoor_features: only for sale listings, short tags` + plotRules + `

` + languagePromptBlock(lang)

	amenityLine := "not specified"
	if len(in.AmenityNames) > 0 {
		amenityLine = strings.Join(in.AmenityNames, ", ")
	} else if len(in.AmenityIDs) > 0 {
		amenityLine = "selected (use generic phrasing only — never numeric IDs in copy)"
	}

	user := fmt.Sprintf(`OUTPUT_LANGUAGE_CODE: %s (derived from the user's own words — NOT the app settings)

Listing type: %s
User details (match this language for title/description): %s
Price: %.0f %s
Bedrooms: %v
Bathrooms: %v
Area: %.0f %s
Property type: %s
Land type: %s
Amenities (mention by name in description if relevant, never as IDs): %s
Plot number hint (from form): %q
Location hints — city: %q, zone: %q, quartier: %q
Media: %d photos, %d videos
Catalog (city > zone > quartier):
%s`,
		lang,
		kindLabel,
		in.Details,
		in.Price, in.Currency,
		ptrInt(in.Bedrooms),
		ptrInt(in.Bathrooms),
		in.Area, in.AreaUnit,
		in.PropertyType,
		in.LandType,
		amenityLine,
		strings.TrimSpace(in.PlotNumber),
		in.CityHint, in.ZoneHint, in.QuartierHint,
		len(in.ImageURLs), len(in.VideoURLs),
		catalogSummary,
	)

	raw, err := g.AI.CompleteListingJSON(system, user)
	if err != nil {
		return llmDraft{}, err
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var out llmDraft
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return llmDraft{}, fmt.Errorf("parse llm json: %w", err)
	}
	return out, nil
}

func ptrInt(p *int) any {
	if p == nil {
		return "not specified"
	}
	return *p
}

func fallbackLLMDraft(in GenerateInput) llmDraft {
	return llmDraft{
		Title:        fallbackTitle(in),
		Description:  fallbackDescription(in, nil),
		CityName:     in.CityHint,
		ZoneName:     in.ZoneHint,
		QuartierName: in.QuartierHint,
	}
}

func fallbackTitle(in GenerateInput) string {
	lang := resolveListingLanguage(in)
	place := strings.TrimSpace(strings.Join([]string{in.QuartierHint, in.ZoneHint, in.CityHint}, ", "))
	if d := strings.TrimSpace(in.Details); d != "" {
		first := d
		if idx := strings.IndexAny(first, "\n"); idx > 0 {
			first = first[:idx]
		}
		runes := []rune(first)
		if len(runes) > 90 {
			first = string(runes[:90])
		}
		return first
	}
	switch lang {
	case "ar":
		switch in.Kind {
		case KindLand:
			return "أرض للبيع — " + place
		case KindSale:
			return "عقار للبيع — " + place
		default:
			return "إيجار — " + place
		}
	case "en":
		switch in.Kind {
		case KindLand:
			return "Land for sale — " + place
		case KindSale:
			return "Property for sale — " + place
		default:
			return "Rental — " + place
		}
	default:
		switch in.Kind {
		case KindLand:
			if in.Area > 0 {
				return fmt.Sprintf("Terrain %.0f %s — %s", in.Area, in.AreaUnit, place)
			}
			return "Terrain à vendre — " + place
		case KindSale:
			return fmt.Sprintf("Bien à vendre — %s", place)
		default:
			return fmt.Sprintf("Location — %s", place)
		}
	}
}

func fallbackDescription(in GenerateInput, d *Draft) string {
	// Preserve user wording as the description body when LLM fails.
	body := strings.TrimSpace(in.Details)
	if body != "" {
		return body
	}
	lang := resolveListingLanguage(in)
	var b strings.Builder
	if in.Price > 0 {
		switch lang {
		case "ar":
			b.WriteString(fmt.Sprintf("السعر: %.0f %s.\n", in.Price, in.Currency))
		case "en":
			b.WriteString(fmt.Sprintf("Price: %.0f %s.\n", in.Price, in.Currency))
		default:
			b.WriteString(fmt.Sprintf("Prix: %.0f %s.\n", in.Price, in.Currency))
		}
	}
	return strings.TrimSpace(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// resolveListingLanguage uses only user-written text — never app UI locale.
func resolveListingLanguage(in GenerateInput) string {
	return DetectOutputLanguage(in.Details, in.CityHint, in.ZoneHint, in.QuartierHint)
}

var plotNumberPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:plot|parcelle|parcel|lot|قطعة|رقم\s*القطعة|parcelle\s*n[°o]?)\s*[:#]?\s*([A-Za-z0-9\-/]+)`),
	regexp.MustCompile(`(?i)(?:plot|parcelle|parcel|lot)\s+([A-Za-z0-9\-/]+)`),
}

func extractPlotNumberFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, re := range plotNumberPatterns {
		if m := re.FindStringSubmatch(text); len(m) > 1 {
			candidate := strings.TrimSpace(m[1])
			if candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func normalizePlotNumber(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	return strings.TrimSpace(s)
}

var amenityIDLinePattern = regexp.MustCompile(`(?im)^\s*amenity\s*ids?\s*selected\s*:\s*[\d,\s]+\.?\s*$`)

func sanitizeListingDescription(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if amenityIDLinePattern.MatchString(trimmed) {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "amenity ids selected") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func resolvePlotNumber(in GenerateInput, llm llmDraft) string {
	if in.Kind != KindLand {
		return ""
	}
	if pn := normalizePlotNumber(in.PlotNumber); pn != "" {
		return pn
	}
	if pn := normalizePlotNumber(llm.PlotNumber); pn != "" {
		return pn
	}
	return normalizePlotNumber(extractPlotNumberFromText(in.Details))
}
