package services

import (
	"bufio"
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
	"sync"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/PuerkitoBio/goquery"
)

// userAgentToken is how we identify ourselves in robots.txt + requests.
const userAgentToken = "MeskenyBot"

// Per-domain politeness: never hit the same host more than once per interval.
var (
	domainLastHit   sync.Map // host -> time.Time
	minDomainSpacing = 4 * time.Second
	robotsCache      sync.Map // host -> []string (disallowed path prefixes)
)

// waitForDomain enforces a minimum spacing between requests to the same host.
func waitForDomain(host string) {
	if v, ok := domainLastHit.Load(host); ok {
		if last, ok := v.(time.Time); ok {
			if d := minDomainSpacing - time.Since(last); d > 0 {
				time.Sleep(d)
			}
		}
	}
	domainLastHit.Store(host, time.Now())
}

// robotsDisallows returns Disallow path-prefixes that apply to us for a host,
// fetched once per host and cached. Best-effort: on any error we return no
// rules (fail-open is standard for robots fetch failures).
func (w *WebScraper) robotsDisallows(ctx context.Context, u *url.URL) []string {
	if u == nil {
		return nil
	}
	if v, ok := robotsCache.Load(u.Host); ok {
		if rules, ok := v.([]string); ok {
			return rules
		}
	}
	robotsURL := u.Scheme + "://" + u.Host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		robotsCache.Store(u.Host, []string(nil))
		return nil
	}
	req.Header.Set("User-Agent", userAgentToken)
	resp, err := w.client.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			resp.Body.Close()
		}
		robotsCache.Store(u.Host, []string(nil))
		return nil
	}
	defer resp.Body.Close()

	// Parse groups; collect Disallow lines under User-agent: * or MeskenyBot.
	var disallows []string
	applies := false
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "user-agent:") {
			ua := strings.TrimSpace(line[len("user-agent:"):])
			applies = ua == "*" || strings.Contains(strings.ToLower(ua), "meskenybot")
			continue
		}
		if applies && strings.HasPrefix(lower, "disallow:") {
			path := strings.TrimSpace(line[len("disallow:"):])
			if path != "" {
				disallows = append(disallows, path)
			}
		}
	}
	robotsCache.Store(u.Host, disallows)
	return disallows
}

// robotsAllowed reports whether robots.txt permits fetching u for us.
func (w *WebScraper) robotsAllowed(ctx context.Context, u *url.URL) bool {
	for _, dis := range w.robotsDisallows(ctx, u) {
		if dis == "/" || strings.HasPrefix(u.Path, dis) {
			return false
		}
	}
	return true
}

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
// per listing via ContentHash. Never panics on malformed markup. `trigger`
// ("manual"|"scheduled") is recorded in the audit trail.
func (w *WebScraper) ScrapeSource(ctx context.Context, source *models.ScrapedSource) (*ScrapeResult, error) {
	return w.scrapeSourceWithTrigger(ctx, source, "manual")
}

// ScrapeSourceScheduled is the audit-tagged variant used by the background refresh.
func (w *WebScraper) ScrapeSourceScheduled(ctx context.Context, source *models.ScrapedSource) (*ScrapeResult, error) {
	return w.scrapeSourceWithTrigger(ctx, source, "scheduled")
}

func (w *WebScraper) scrapeSourceWithTrigger(ctx context.Context, source *models.ScrapedSource, trigger string) (result *ScrapeResult, retErr error) {
	startedAt := time.Now()
	// Audit EVERY run (success or failure) via defer, so no return path escapes
	// the trail — enterprise/government traceability of external data fetches.
	defer func() {
		run := models.ScrapeRun{
			SourceID:   source.ID,
			URL:        source.URL,
			Trigger:    trigger,
			OK:         retErr == nil,
			DurationMs: time.Since(startedAt).Milliseconds(),
		}
		if retErr != nil {
			run.Status = clip("error: "+retErr.Error(), 255)
		} else if result != nil {
			run.Status = clip(result.Status, 255)
			run.ItemCount = result.ItemCount
			run.Inserted = result.Inserted
			run.Updated = result.Updated
		}
		storage.DB.Create(&run)
	}()

	target, perr := url.Parse(source.URL)
	if perr != nil || target.Host == "" {
		return nil, fmt.Errorf("invalid source URL")
	}

	var sel scrapeSelectors
	if len(source.Selectors) > 0 {
		_ = json.Unmarshal(source.Selectors, &sel)
	}
	// No 'item' selector → CONTENT-CRAWL mode: map the whole site and extract
	// each page's readable text (for ministry / cadastre / procedures info),
	// instead of listing-card extraction. This is the default for informational
	// government/institutional sources.
	if strings.TrimSpace(sel.Item) == "" {
		result, retErr = w.crawlSiteContent(ctx, source, target)
		return result, retErr
	}

	// Respect robots.txt — refuse disallowed paths (compliance/legal).
	if !w.robotsAllowed(ctx, target) {
		return nil, fmt.Errorf("blocked by robots.txt for %s", target.Host)
	}
	// Per-domain politeness — never hammer a host.
	waitForDomain(target.Host)

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
	result = &ScrapeResult{}

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

// Crawl limits — bounded so a single source can't run away.
const (
	crawlMaxPages = 60
	crawlMaxDepth = 2
	crawlMinChars = 120 // skip near-empty pages
)

// Real-estate relevance: only KEEP pages whose text/URL relates to housing,
// land, cadastre, urbanism, or the relevant ministries/procedures. Links are
// still followed broadly (a hub page may link to relevant sub-pages), but
// stored content is filtered so the AI's knowledge stays on-topic.
var realEstateTerms = []string{
	"immobilier", "logement", "urbanisme", "foncier", "terrain", "cadastre",
	"habitat", "propriété", "propriete", "parcelle", "lotissement", "construction",
	"land", "housing", "property", "real estate", "plot", "deed", "title",
	"عقار", "أرض", "سكن", "إسكان", "عمران", "مساحة", "قطعة", "بناء", "ملكية",
	"procédure", "procedure", "ministère", "ministere", "ministry",
}

type crawlPage struct {
	url   string
	depth int
}

// crawlSiteContent maps a site (BFS, same-host, bounded) and stores each
// relevant page's readable text as a ScrapedListing (Title = page title,
// Description = cleaned text, SourceURL = page URL). This powers the AI's
// knowledge of ministry/cadastre/land procedures with citations.
func (w *WebScraper) crawlSiteContent(ctx context.Context, source *models.ScrapedSource, start *url.URL) (*ScrapeResult, error) {
	result := &ScrapeResult{}
	visited := map[string]bool{}
	queue := []crawlPage{{url: start.String(), depth: 0}}
	now := time.Now()

	for len(queue) > 0 && result.ItemCount < crawlMaxPages {
		if ctx.Err() != nil {
			break
		}
		page := queue[0]
		queue = queue[1:]
		if visited[page.url] {
			continue
		}
		visited[page.url] = true

		pu, err := url.Parse(page.url)
		if err != nil || pu.Host != start.Host {
			continue
		}
		if !w.robotsAllowed(ctx, pu) {
			continue
		}
		waitForDomain(pu.Host)

		doc, err := w.fetchDoc(ctx, page.url)
		if err != nil || doc == nil {
			continue
		}

		title, text := extractReadable(doc)
		if pageIsRelevant(title, text, page.url) && len(text) >= crawlMinChars {
			listing := models.ScrapedListing{
				SourceID:    source.ID,
				Kind:        source.Kind,
				Title:       clip(title, 500),
				Description: clip(text, 6000),
				SourceURL:   page.url,
				ScrapedAt:   now,
			}
			listing.ContentHash = hashListing(source.ID, &listing)
			var existing models.ScrapedListing
			if storage.DB.Where("source_id = ? AND content_hash = ?", source.ID, listing.ContentHash).
				First(&existing).Error == nil {
				listing.ID = existing.ID
				listing.CreatedAt = existing.CreatedAt
				if storage.DB.Save(&listing).Error == nil {
					result.Updated++
				}
			} else if storage.DB.Create(&listing).Error == nil {
				result.Inserted++
			}
			result.ItemCount++
		}

		// Discover same-host links to crawl next (breadth-first, bounded depth).
		if page.depth < crawlMaxDepth {
			doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
				href, ok := s.Attr("href")
				if !ok {
					return
				}
				next := absURL(start, strings.TrimSpace(href))
				nu, err := url.Parse(next)
				if err != nil || nu.Host != start.Host {
					return
				}
				nu.Fragment = ""
				clean := nu.String()
				if !visited[clean] && len(queue) < 400 {
					queue = append(queue, crawlPage{url: clean, depth: page.depth + 1})
				}
			})
		}
	}

	result.Status = fmt.Sprintf("crawled — %d relevant pages (%d new, %d updated)",
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

func (w *WebScraper) fetchDoc(ctx context.Context, rawURL string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (compatible; MeskenyBot/1.0; +https://meskeny.com/bot)")
	req.Header.Set("Accept-Language", "en,fr,ar")
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "html") {
		return nil, fmt.Errorf("non-html")
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

var wsCollapse = regexp.MustCompile(`\s+`)

// extractReadable pulls a page's title and main text, stripping chrome.
func extractReadable(doc *goquery.Document) (title, text string) {
	title = strings.TrimSpace(doc.Find("title").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	doc.Find("script, style, noscript, nav, footer, header, form, svg").Remove()
	body := doc.Find("main")
	if body.Length() == 0 {
		body = doc.Find("body")
	}
	text = wsCollapse.ReplaceAllString(strings.TrimSpace(body.Text()), " ")
	return title, text
}

func pageIsRelevant(title, text, pageURL string) bool {
	hay := strings.ToLower(title + " " + text + " " + pageURL)
	for _, term := range realEstateTerms {
		if strings.Contains(hay, term) {
			return true
		}
	}
	return false
}

// ScrapeAllActiveSources re-scrapes every active source, sequentially and
// politely (a short delay between sources so we never hammer a site). Used by
// the scheduled refresh so the AI's market data stays fresh without manual
// admin runs. Errors are logged per-source and never abort the batch.
func ScrapeAllActiveSources() {
	if storage.DB == nil {
		return
	}
	var sources []models.ScrapedSource
	if err := storage.DB.Where("active = true").Find(&sources).Error; err != nil {
		return
	}
	scraper := NewWebScraper()
	for i := range sources {
		func(src *models.ScrapedSource) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := scraper.ScrapeSourceScheduled(ctx, src); err != nil {
				src.LastStatus = "error: " + err.Error()
				storage.DB.Model(src).Update("last_status", src.LastStatus)
			}
		}(&sources[i])
		time.Sleep(3 * time.Second) // be polite between sites
	}
}
