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

// parsePastedItems turns any JSON payload into a flat list of records.
func parsePastedItems(text string) []pastedItem {
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
