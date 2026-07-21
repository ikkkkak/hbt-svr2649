package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/PuerkitoBio/goquery"
)

// WebScraper is a single reusable extractor: it fetches a ScrapedSource's URL
// and maps the DOM onto ScrapedListing rows using the source's CSS selectors.
// Free + open source (goquery over net/http). No per-site code — an admin
// registers a URL + selectors and this handles the rest.
type WebScraper struct {
	client *http.Client
}

func NewWebScraper() *WebScraper {
	return &WebScraper{
		client: &http.Client{Timeout: 25 * time.Second},
	}
}

type scrapeSelectors struct {
	Item        string `json:"item"`
	Title       string `json:"title"`
	Price       string `json:"price"`
	Location    string `json:"location"`
	City        string `json:"city"`
	Area        string `json:"area"`
	Bedrooms    string `json:"bedrooms"`
	Bathrooms   string `json:"bathrooms"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Link        string `json:"link"`
	Type        string `json:"type"`
}

var digitsRe = regexp.MustCompile(`[\d][\d\s,\.]*`)

// ScrapeResult summarizes one scrape run.
type ScrapeResult struct {
	ItemCount int
	Inserted  int
	Updated   int
	Status    string
}

// ScrapeSource fetches + extracts + upserts listings for a source. Idempotent
// per listing via ContentHash. Never panics on malformed markup.
func (w *WebScraper) ScrapeSource(ctx context.Context, source *models.ScrapedSource) (*ScrapeResult, error) {
	var sel scrapeSelectors
	if len(source.Selectors) > 0 {
		_ = json.Unmarshal(source.Selectors, &sel)
	}
	if strings.TrimSpace(sel.Item) == "" {
		return nil, fmt.Errorf("source %d has no 'item' selector", source.ID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, err
	}
	// Present as a normal browser so anti-bot heuristics don't 403 us.
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (compatible; MeskenyBot/1.0; +https://meskeny.com/bot)")
	req.Header.Set("Accept-Language", "en,fr,ar")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	base, _ := url.Parse(source.URL)
	now := time.Now()
	result := &ScrapeResult{}

	doc.Find(sel.Item).Each(func(_ int, s *goquery.Selection) {
		title := fieldText(s, sel.Title)
		if title == "" {
			return // skip empty cards
		}
		priceText := fieldText(s, sel.Price)
		link := absURL(base, fieldText(s, sel.Link))

		listing := models.ScrapedListing{
			SourceID:     source.ID,
			Kind:         source.Kind,
			Title:        clip(title, 500),
			Description:  clip(fieldText(s, sel.Description), 4000),
			PriceText:    clip(priceText, 120),
			PriceValue:   parseAmount(priceText),
			Currency:     detectCurrency(priceText),
			Location:     clip(fieldText(s, sel.Location), 255),
			City:         clip(fieldText(s, sel.City), 120),
			AreaM2:       parseIntPtr(fieldText(s, sel.Area)),
			Bedrooms:     parseIntPtr(fieldText(s, sel.Bedrooms)),
			Bathrooms:    parseIntPtr(fieldText(s, sel.Bathrooms)),
			PropertyType: clip(fieldText(s, sel.Type), 60),
			ImageURL:     absURL(base, fieldText(s, sel.Image)),
			SourceURL:    link,
			ScrapedAt:    now,
		}
		listing.ContentHash = hashListing(source.ID, &listing)

		// Upsert on (source_id, content_hash).
		var existing models.ScrapedListing
		err := storage.DB.Where("source_id = ? AND content_hash = ?", source.ID, listing.ContentHash).
			First(&existing).Error
		if err == nil {
			listing.ID = existing.ID
			listing.CreatedAt = existing.CreatedAt
			if storage.DB.Save(&listing).Error == nil {
				result.Updated++
			}
		} else {
			if storage.DB.Create(&listing).Error == nil {
				result.Inserted++
			}
		}
		result.ItemCount++
	})

	result.Status = fmt.Sprintf("ok — %d items (%d new, %d updated)",
		result.ItemCount, result.Inserted, result.Updated)

	source.LastScrapedAt = &now
	source.LastStatus = result.Status
	source.LastItemCount = result.ItemCount
	storage.DB.Model(source).Updates(map[string]any{
		"last_scraped_at": now,
		"last_status":     result.Status,
		"last_item_count": result.ItemCount,
	})

	return result, nil
}

// fieldText resolves a selector against a card. A "@attr" suffix reads that
// attribute (e.g. "img@src", "a@href"); otherwise trimmed text is returned.
// An empty selector yields "". A "." selector means the card element itself.
func fieldText(s *goquery.Selection, selector string) string {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return ""
	}
	attr := ""
	if i := strings.LastIndex(selector, "@"); i >= 0 {
		attr = selector[i+1:]
		selector = selector[:i]
	}
	target := s
	if selector != "" && selector != "." {
		target = s.Find(selector).First()
	}
	if target.Length() == 0 {
		return ""
	}
	if attr != "" {
		v, _ := target.Attr(attr)
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(target.Text())
}

func absURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || base == nil {
		return raw
	}
	if u, err := url.Parse(raw); err == nil {
		return base.ResolveReference(u).String()
	}
	return raw
}

func parseAmount(s string) *int64 {
	m := digitsRe.FindString(s)
	if m == "" {
		return nil
	}
	clean := strings.NewReplacer(" ", "", ",", "", ".", "").Replace(m)
	if clean == "" {
		return nil
	}
	if v, err := strconv.ParseInt(clean, 10, 64); err == nil {
		return &v
	}
	return nil
}

func parseIntPtr(s string) *int {
	m := digitsRe.FindString(s)
	if m == "" {
		return nil
	}
	clean := strings.NewReplacer(" ", "", ",", "").Replace(m)
	if i := strings.IndexByte(clean, '.'); i >= 0 {
		clean = clean[:i]
	}
	if clean == "" {
		return nil
	}
	if v, err := strconv.Atoi(clean); err == nil {
		return &v
	}
	return nil
}

func detectCurrency(s string) string {
	up := strings.ToUpper(s)
	switch {
	case strings.Contains(up, "MRU") || strings.Contains(s, "أوقية"):
		return "MRU"
	case strings.Contains(up, "SAR") || strings.Contains(s, "ريال"):
		return "SAR"
	case strings.Contains(up, "AED") || strings.Contains(s, "درهم"):
		return "AED"
	case strings.Contains(up, "USD") || strings.Contains(s, "$"):
		return "USD"
	case strings.Contains(up, "EUR") || strings.Contains(s, "€"):
		return "EUR"
	}
	return ""
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func hashListing(sourceID uint, l *models.ScrapedListing) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s",
		sourceID, l.Title, l.PriceText, l.Location, l.SourceURL)))
	return hex.EncodeToString(h[:])
}
