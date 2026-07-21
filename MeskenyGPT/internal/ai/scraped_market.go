package ai

import (
	"context"
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"

	"gorm.io/gorm"
)

// scrapedMarketRow is a lightweight projection of scraped_listings (no model
// import — keeps the internal AI module decoupled from the server's models).
type scrapedMarketRow struct {
	Title     string
	PriceText string
	Location  string
	City      string
	SourceURL string
	Kind      string
}

// retrieveScrapedMarketBlock returns a system-prompt block of real, admin-
// scraped market listings relevant to the user's message, each with its
// source URL. Injecting this lets MeskenyGPT REASON over current market data
// (prices, comparables) and cite it — the difference between advising from
// live data and guessing. Returns "" when nothing relevant or DB absent.
func retrieveScrapedMarketBlock(ctx context.Context, gdb *gorm.DB, mc lang.MessageContext) string {
	if gdb == nil {
		return ""
	}

	kind := ""
	switch mc.Intent {
	case lang.IntentSearchLand:
		kind = "land_sale"
	}

	q := gdb.WithContext(ctx).
		Table("scraped_listings AS sl").
		Select("sl.title, sl.price_text, sl.location, sl.city, sl.source_url, sl.kind").
		Joins("JOIN scraped_sources ss ON ss.id = sl.source_id AND ss.active = true").
		Where("sl.deleted_at IS NULL")

	if kind != "" {
		q = q.Where("sl.kind = ?", kind)
	}
	// Relevance: prefer city/location matches, else most recent.
	needle := strings.TrimSpace(strings.ToLower(firstNonEmptyStr(mc.City, mc.Zone)))
	if needle != "" {
		like := "%" + needle + "%"
		q = q.Where("LOWER(sl.city) LIKE ? OR LOWER(sl.location) LIKE ? OR LOWER(sl.title) LIKE ?",
			like, like, like)
	}

	var rows []scrapedMarketRow
	if err := q.Order("sl.scraped_at DESC").Limit(6).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n=== LIVE MARKET LISTINGS (scraped, real; use for pricing/comparables and CITE the source URL when you reference one) ===\n")
	for i, r := range rows {
		loc := firstNonEmptyStr(r.Location, r.City)
		b.WriteString(fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(r.Title)))
		if r.PriceText != "" {
			b.WriteString(" — " + strings.TrimSpace(r.PriceText))
		}
		if loc != "" {
			b.WriteString(" (" + strings.TrimSpace(loc) + ")")
		}
		if r.SourceURL != "" {
			b.WriteString("\n   source: " + r.SourceURL)
		}
		b.WriteString("\n")
	}
	b.WriteString("Only cite these when genuinely relevant; never invent listings or prices.\n")
	return b.String()
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
