package services

import (
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// ScrapedCitation is a market listing surfaced to MeskenyGPT with the source
// link it must cite. Returning these turns "the AI made it up" into "the AI
// found these, here are the references" — the difference between a robot and
// a credible agent.
type ScrapedCitation struct {
	Title     string `json:"title"`
	PriceText string `json:"price_text"`
	Location  string `json:"location"`
	SourceURL string `json:"source_url"`
	Kind      string `json:"kind"`
}

// RetrieveScrapedContext returns up to `limit` market listings relevant to the
// user's message, filtered by intent kind when detectable. Used to ground AI
// answers with real, cited data instead of guesses.
func RetrieveScrapedContext(message string, limit int) []ScrapedCitation {
	if storage.DB == nil || limit <= 0 {
		return nil
	}
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return nil
	}

	q := storage.DB.Model(&models.ScrapedListing{}).
		Joins("JOIN scraped_sources ON scraped_sources.id = scraped_listings.source_id AND scraped_sources.active = true")

	// Intent → kind filter (best-effort; broadens if nothing matches).
	switch {
	case containsAnyWord(msg, "rent", "louer", "location", "إيجار", "كراء"):
		q = q.Where("scraped_listings.kind = ?", "property_rent")
	case containsAnyWord(msg, "land", "terrain", "plot", "أرض", "قطعة"):
		q = q.Where("scraped_listings.kind = ?", "land_sale")
	case containsAnyWord(msg, "buy", "sale", "acheter", "vente", "شراء", "بيع"):
		q = q.Where("scraped_listings.kind = ?", "property_sale")
	}

	// Lightweight relevance: match tokens against title/location/city.
	tokens := meaningfulTokens(msg)
	for _, tok := range tokens {
		like := "%" + tok + "%"
		q = q.Or("LOWER(scraped_listings.title) LIKE ? OR LOWER(scraped_listings.location) LIKE ? OR LOWER(scraped_listings.city) LIKE ?",
			like, like, like)
	}

	var rows []models.ScrapedListing
	if err := q.Order("scraped_listings.scraped_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil
	}

	out := make([]ScrapedCitation, 0, len(rows))
	for i := range rows {
		out = append(out, ScrapedCitation{
			Title:     rows[i].Title,
			PriceText: rows[i].PriceText,
			Location:  rows[i].Location,
			SourceURL: rows[i].SourceURL,
			Kind:      rows[i].Kind,
		})
	}
	return out
}

func containsAnyWord(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "you": true,
	"want": true, "need": true, "looking": true, "find": true, "show": true,
	"une": true, "des": true, "les": true, "pour": true, "avec": true,
}

func meaningfulTokens(msg string) []string {
	fields := strings.FieldsFunc(msg, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') &&
			!(r >= 0x0600 && r <= 0x06FF) // keep Arabic
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 3 || stopWords[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= 6 {
			break
		}
	}
	return out
}
