package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// ImportPastedJSON ingests an arbitrary JSON payload an admin pasted (the
// output of an external crawl/scrape done elsewhere) and stores it as citable
// knowledge in scraped_listings — the same table MeskenyGPT reads for RAG. The
// AI then treats it as context and cites it in answers to user questions.
//
// The payload shape is flexible: an array of objects, a single object, a
// wrapper object containing a data/items/results array, or raw non-JSON text.
// For each record we derive a title, a description (the searchable body), and a
// source URL for citation. Deduped by content hash so re-pasting is safe.
func ImportPastedJSON(name, kind, sourceURL, payload string) (sourceID uint, inserted int, err error) {
	if storage.DB == nil {
		return 0, 0, fmt.Errorf("database unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Pasted data"
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "manual_json"
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0, 0, fmt.Errorf("empty payload")
	}

	url := strings.TrimSpace(sourceURL)
	if url == "" {
		url = "pasted://" + slugify(name)
	}

	// Upsert the source (one per URL) so re-pasting updates the same bucket.
	src := models.ScrapedSource{Name: name, URL: url, Kind: kind, Active: true}
	var existing models.ScrapedSource
	if storage.DB.Where("url = ?", url).First(&existing).Error == nil {
		src.ID = existing.ID
	} else if e := storage.DB.Create(&src).Error; e != nil {
		return 0, 0, e
	}

	items := parsePastedItems(payload)
	if len(items) == 0 {
		return src.ID, 0, fmt.Errorf("could not extract any records from the JSON")
	}

	// Re-paste replaces: clear this source's previous rows so a stale
	// whole-file blob from an earlier import can't linger and pollute grounding.
	storage.DB.Where("source_id = ?", src.ID).Delete(&models.ScrapedListing{})

	now := time.Now()
	for _, it := range items {
		desc := sanitizeUTF8(strings.TrimSpace(it.Description))
		if desc == "" {
			continue
		}
		listing := models.ScrapedListing{
			SourceID:    src.ID,
			Kind:        kind,
			Title:       clip(sanitizeUTF8(it.Title), 500),
			Description: clip(desc, 6000),
			SourceURL:   clip(firstNonEmptyStr(it.URL, url), 2000),
			ScrapedAt:   now,
		}
		listing.ContentHash = hashListing(src.ID, &listing)
		var ex models.ScrapedListing
		if storage.DB.Where("source_id = ? AND content_hash = ?", src.ID, listing.ContentHash).
			First(&ex).Error != nil {
			if storage.DB.Create(&listing).Error == nil {
				inserted++
			}
		}
	}

	storage.DB.Model(&models.ScrapedSource{}).Where("id = ?", src.ID).Updates(map[string]any{
		"last_scraped_at": now,
		"last_status":     fmt.Sprintf("pasted — %d records imported", inserted),
		"last_item_count": inserted,
	})
	return src.ID, inserted, nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type pastedItem struct{ Title, Description, URL string }

// procDetail is one language variant of a procedures.gov.mr procedure.
type procDetail struct {
	URL               string         `json:"url"`
	Title             string         `json:"title"`
	MetadataFields    map[string]any `json:"metadata_fields"`
	RequiredDocuments []string       `json:"required_documents"`
}

// parseProceduresExport recognises the procedures.gov.mr structured export
// (target_categories[].procedures[].french_details/arabic_details) and flattens
// it into one rich, citable record per language per procedure — title, the
// metadata (responsible entity, delay, fees), and the required documents, with
// the real source URL. Returns nil if the payload isn't that shape.
func parseProceduresExport(text string) []pastedItem {
	var doc struct {
		TargetCategories []struct {
			TitleFR    string `json:"title_fr"`
			TitleAR    string `json:"title_ar"`
			Procedures []struct {
				FrenchDetails *procDetail `json:"french_details"`
				ArabicDetails *procDetail `json:"arabic_details"`
			} `json:"procedures"`
		} `json:"target_categories"`
	}
	if json.Unmarshal([]byte(text), &doc) != nil || len(doc.TargetCategories) == 0 {
		return nil
	}
	var out []pastedItem
	for _, cat := range doc.TargetCategories {
		catTitle := firstNonEmptyStr(cat.TitleAR, cat.TitleFR)
		for _, p := range cat.Procedures {
			for _, d := range []*procDetail{p.ArabicDetails, p.FrenchDetails} {
				if d == nil || strings.TrimSpace(d.Title) == "" {
					continue
				}
				out = append(out, procedureRecord(catTitle, d))
			}
		}
	}
	return out
}

func procedureRecord(catTitle string, d *procDetail) pastedItem {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(d.Title))
	if catTitle != "" {
		b.WriteString(" — " + catTitle)
	}
	b.WriteString("\n")
	if len(d.MetadataFields) > 0 {
		keys := make([]string, 0, len(d.MetadataFields))
		for k := range d.MetadataFields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v := strings.TrimSpace(fmt.Sprintf("%v", d.MetadataFields[k])); v != "" && v != "<nil>" {
				fmt.Fprintf(&b, "%s: %s\n", k, v)
			}
		}
	}
	if len(d.RequiredDocuments) > 0 {
		b.WriteString("الوثائق المطلوبة / Documents requis:\n")
		for _, doc := range d.RequiredDocuments {
			if s := strings.TrimSpace(doc); s != "" {
				b.WriteString("- " + s + "\n")
			}
		}
	}
	return pastedItem{
		Title:       strings.TrimSpace(d.Title),
		Description: strings.TrimSpace(b.String()),
		URL:         strings.TrimSpace(d.URL),
	}
}

// parseMinistriesExport handles the ijraati.gov.mr export shape:
// { source, ministries:[ { ministry_ar, procedures:[ { title, description,
// category, concerned_parties{}, required_documents (string|array),
// duration_and_cost{} } ] } ] }. Flattens to ONE rich record per procedure.
func parseMinistriesExport(text string) []pastedItem {
	var doc struct {
		Source     string `json:"source"`
		Ministries []struct {
			MinistryAR string `json:"ministry_ar"`
			MinistryEN string `json:"ministry_en"`
			Procedures []struct {
				Title             string         `json:"title"`
				Description       string         `json:"description"`
				Category          string         `json:"category"`
				ConcernedParties  map[string]any `json:"concerned_parties"`
				RequiredDocuments any            `json:"required_documents"`
				DurationAndCost   map[string]any `json:"duration_and_cost"`
				URL               string         `json:"url"`
			} `json:"procedures"`
		} `json:"ministries"`
	}
	if json.Unmarshal([]byte(text), &doc) != nil || len(doc.Ministries) == 0 {
		return nil
	}
	var out []pastedItem
	for _, m := range doc.Ministries {
		for _, p := range m.Procedures {
			title := strings.TrimSpace(p.Title)
			if title == "" {
				continue
			}
			var b strings.Builder
			b.WriteString(title)
			if m.MinistryAR != "" {
				b.WriteString(" — " + strings.TrimSpace(m.MinistryAR))
			}
			b.WriteString("\n")
			if d := strings.TrimSpace(p.Description); d != "" {
				b.WriteString(d + "\n")
			}
			if docs := anyToText(p.RequiredDocuments); docs != "" {
				b.WriteString("الوثائق المطلوبة / Documents requis: " + docs + "\n")
			}
			for _, k := range sortedMapKeys(p.DurationAndCost) {
				if v := valToText(p.DurationAndCost[k]); v != "" {
					fmt.Fprintf(&b, "%s: %s\n", k, v)
				}
			}
			for _, k := range sortedMapKeys(p.ConcernedParties) {
				if v := valToText(p.ConcernedParties[k]); v != "" {
					fmt.Fprintf(&b, "%s: %s\n", k, v)
				}
			}
			out = append(out, pastedItem{
				Title:       title,
				Description: strings.TrimSpace(b.String()),
				URL:         firstNonEmptyStr(strings.TrimSpace(p.URL), strings.TrimSpace(doc.Source)),
			})
		}
	}
	return out
}

func anyToText(v any) string {
	switch vv := v.(type) {
	case string:
		return strings.TrimSpace(vv)
	case []any:
		parts := make([]string, 0, len(vv))
		for _, e := range vv {
			if s := valToText(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}

func valToText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func sortedMapKeys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parsePastedItems turns any JSON payload into a flat list of records.
func parsePastedItems(text string) []pastedItem {
	// 0a) ijraati.gov.mr export (ministries[].procedures[]).
	if items := parseMinistriesExport(text); len(items) > 0 {
		return items
	}
	// 0b) procedures.gov.mr export (target_categories → fr/ar details).
	if items := parseProceduresExport(text); len(items) > 0 {
		return items
	}
	// 1) Array of objects.
	var arr []map[string]any
	if json.Unmarshal([]byte(text), &arr) == nil && len(arr) > 0 {
		return itemsFromObjects(arr)
	}
	// 2) Single object — possibly a wrapper around a data/items/results array.
	var obj map[string]any
	if json.Unmarshal([]byte(text), &obj) == nil && len(obj) > 0 {
		for _, k := range []string{"data", "items", "results", "records", "rows", "listings", "pages", "documents"} {
			if v, ok := obj[k]; ok {
				if a, ok := toObjectArray(v); ok && len(a) > 0 {
					return itemsFromObjects(a)
				}
			}
		}
		return itemsFromObjects([]map[string]any{obj})
	}
	// 3) A bare JSON array of strings.
	var strs []string
	if json.Unmarshal([]byte(text), &strs) == nil && len(strs) > 0 {
		out := make([]pastedItem, 0, len(strs))
		for i, s := range strs {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, pastedItem{Title: firstSentence(s, 80), Description: s, URL: ""})
			_ = i
		}
		return out
	}
	// 4) Not JSON — keep the raw text as a single knowledge record.
	return []pastedItem{{Title: firstSentence(text, 80), Description: text}}
}

func itemsFromObjects(objs []map[string]any) []pastedItem {
	out := make([]pastedItem, 0, len(objs))
	for i, o := range objs {
		title := pickString(o, "title", "name", "titre", "heading", "question", "subject", "label", "اسم", "عنوان")
		url := pickString(o, "url", "source_url", "sourceurl", "source", "link", "lien", "href", "page_url")
		desc := pickString(o, "description", "content", "text", "body", "answer", "contenu", "details", "summary", "محتوى", "نص")
		if desc == "" {
			desc = objectToText(o)
		}
		if title == "" {
			title = firstNonEmptyStr(firstSentence(desc, 80), fmt.Sprintf("Record %d", i+1))
		}
		out = append(out, pastedItem{Title: title, Description: desc, URL: url})
	}
	return out
}

// objectToText renders an object as readable "Key: value" lines so nothing is
// lost even when there is no obvious description field.
func objectToText(o map[string]any) string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := o[k]
		switch vv := v.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(vv) == "" {
				continue
			}
			fmt.Fprintf(&b, "%s: %s\n", k, strings.TrimSpace(vv))
		case float64, bool:
			fmt.Fprintf(&b, "%s: %v\n", k, vv)
		default:
			if raw, e := json.Marshal(vv); e == nil {
				s := strings.TrimSpace(string(raw))
				if s != "" && s != "null" && s != "{}" && s != "[]" {
					fmt.Fprintf(&b, "%s: %s\n", k, s)
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func toObjectArray(v any) ([]map[string]any, bool) {
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, len(out) > 0
}

func pickString(o map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := o[k]; ok {
			switch vv := v.(type) {
			case string:
				if s := strings.TrimSpace(vv); s != "" {
					return s
				}
			case float64:
				return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", vv), "0"), ".")
			}
		}
	}
	return ""
}

// firstSentence returns up to maxRunes of the first line/sentence — a title.
func firstSentence(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".\n!؟?"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	return firstRunes(s, maxRunes)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("src-%d", time.Now().Unix())
	}
	return out
}
