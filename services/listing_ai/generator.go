package listing_ai

import (
	"encoding/json"
	"fmt"
	"log"
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

	userStory := extractUserStory(in.Details)

	llm, err := g.callLLM(in, userStory, BuildCatalogSummary(filtered, maxLines), false)
	if err != nil {
		log.Printf("listing_ai: LLM error (%v) — using template draft", err)
		llm = buildTemplateDraft(in)
	} else if isEchoOfUserInput(llm, userStory) {
		log.Printf("listing_ai: LLM echoed user input — retrying rewrite")
		if retry, err2 := g.callLLM(in, userStory, BuildCatalogSummary(filtered, maxLines), true); err2 == nil && !isEchoOfUserInput(retry, userStory) {
			llm = retry
		} else {
			log.Printf("listing_ai: rewrite failed or still echoed — using template draft")
			llm = buildTemplateDraft(in)
		}
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
	draft.Description = mergeNeighborhoodIntoDescription(
		draft.Description,
		draft.NeighborhoodDescription,
	)
	draft.Description = sanitizeListingDescription(draft.Description)

	if len(in.AmenityIDs) > 0 {
		draft.AmenityIDs = append([]uint(nil), in.AmenityIDs...)
	}

	draft.PlotNumber = resolvePlotNumber(in, llm)

	return draft, nil
}

func (g *Generator) callLLM(in GenerateInput, userStory, catalogSummary string, rewrite bool) (llmDraft, error) {
	lang := ResolveListingLanguage(in)

	schema := `{"title":"","description":"","city_name":"","zone_name":"","quartier_name":"","neighborhood_description":"","indoor_features":[],"outdoor_features":[]}`
	plotRules := ""
	if in.Kind == KindLand {
		schema = `{"title":"","description":"","city_name":"","zone_name":"","quartier_name":"","neighborhood_description":"","indoor_features":[],"outdoor_features":[],"plot_number":""}`
		plotRules = `
- plot_number: cadastre parcel number ONLY (digits/alphanumeric as on title deed). Copy exactly from user text when mentioned. Leave empty if not stated — never invent.`
	}

	system := buildListingSystemPrompt(in.Kind, lang, schema, plotRules)
	if rewrite {
		system += `

REWRITE MODE (critical):
- The previous attempt copied the user's notes. You MUST write completely NEW professional copy.
- Do NOT reuse sentences from the user notes. Rephrase every idea.
- Title must be a polished marketplace headline, not the user's first sentence.`
	}
	user := buildListingUserPrompt(in, userStory, lang, catalogSummary)

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
	return buildTemplateDraft(in)
}

func fallbackTitle(in GenerateInput) string {
	return buildTemplateDraft(in).Title
}

func fallbackDescription(in GenerateInput, d *Draft) string {
	return buildTemplateDraft(in).Description
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

// mergeNeighborhoodIntoDescription appends area context when the LLM returned it separately.
func mergeNeighborhoodIntoDescription(description, neighborhood string) string {
	desc := strings.TrimSpace(description)
	hood := strings.TrimSpace(neighborhood)
	if hood == "" {
		return desc
	}
	if desc == "" {
		return hood
	}
	if strings.Contains(strings.ToLower(desc), strings.ToLower(hood)) {
		return desc
	}
	return desc + "\n\n" + hood
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
