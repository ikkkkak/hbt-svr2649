package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ScrapedSource is an admin-registered web page MeskenyGPT scrapes for
// real-estate market data. The admin supplies the URL and a small set of CSS
// selectors that map the page's structure onto our fields, so ONE reusable
// scraper handles any site (Bayut, listing portals, classifieds) without
// per-site code. Kind routes the extracted rows into the right bucket.
type ScrapedSource struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Name string `json:"name" gorm:"size:160;not null"`
	URL  string `json:"url" gorm:"type:text;not null"`
	// Kind: property_sale | property_rent | land_sale | market_info
	Kind string `json:"kind" gorm:"size:32;index;not null;default:property_sale"`

	// Selectors is a JSON map of field -> CSS selector, e.g.
	// {"item":".listing-card","title":".title","price":".price",
	//  "location":".location","area":".area","bedrooms":".beds",
	//  "bathrooms":".baths","description":".desc","image":"img@src",
	//  "link":"a@href"}.  A "@attr" suffix reads an attribute instead of text.
	Selectors datatypes.JSON `json:"selectors" gorm:"type:jsonb"`

	Active bool `json:"active" gorm:"default:true;index"`

	LastScrapedAt *time.Time `json:"last_scraped_at,omitempty"`
	LastStatus    string     `json:"last_status" gorm:"size:255"`
	LastItemCount int        `json:"last_item_count"`

	CreatedByUserID *uint `json:"created_by_user_id,omitempty" gorm:"index"`
}

func (ScrapedSource) TableName() string { return "scraped_sources" }

// ScrapedListing is one structured row extracted from a ScrapedSource. It is
// the reusable market-data table MeskenyGPT reads from (with citations back
// to SourceURL) and that other features can query. Deduped by ContentHash.
type ScrapedListing struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	SourceID uint   `json:"source_id" gorm:"index;not null"`
	Kind     string `json:"kind" gorm:"size:32;index;not null"`

	Title       string `json:"title" gorm:"type:text"`
	Description  string `json:"description" gorm:"type:text"`
	PriceText    string `json:"price_text" gorm:"size:120"`
	PriceValue   *int64 `json:"price_value,omitempty" gorm:"index"`
	Currency     string `json:"currency" gorm:"size:12"`
	Location     string `json:"location" gorm:"size:255;index"`
	City         string `json:"city" gorm:"size:120;index"`
	AreaM2       *int   `json:"area_m2,omitempty"`
	Bedrooms     *int   `json:"bedrooms,omitempty"`
	Bathrooms    *int   `json:"bathrooms,omitempty"`
	PropertyType string `json:"property_type" gorm:"size:60"`
	ImageURL     string `json:"image_url" gorm:"type:text"`
	// SourceURL is the listing's own page — used as the AI's citation link.
	SourceURL string `json:"source_url" gorm:"type:text;index"`

	// Attributes holds any extra key/value pairs the selectors captured.
	Attributes datatypes.JSON `json:"attributes,omitempty" gorm:"type:jsonb"`

	// ContentHash dedupes re-scrapes of the same listing (source_id+hash unique).
	ContentHash string    `json:"content_hash" gorm:"size:64;index"`
	ScrapedAt   time.Time `json:"scraped_at" gorm:"index"`
}

func (ScrapedListing) TableName() string { return "scraped_listings" }

// ScrapeRun is an audit record of one scrape execution — who/when/what/outcome.
// Enterprise/government traceability: every fetch of external data is logged,
// with duration and item counts, immutable after creation.
type ScrapeRun struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`

	SourceID   uint   `json:"source_id" gorm:"index;not null"`
	URL        string `json:"url" gorm:"type:text"`
	Trigger    string `json:"trigger" gorm:"size:24"` // manual | scheduled
	Status     string `json:"status" gorm:"size:255"`
	OK         bool   `json:"ok" gorm:"index"`
	ItemCount  int    `json:"item_count"`
	Inserted   int    `json:"inserted"`
	Updated    int    `json:"updated"`
	DurationMs int64  `json:"duration_ms"`
}

func (ScrapeRun) TableName() string { return "scrape_runs" }

// ScrapedAPICall is one XHR/fetch/GraphQL response intercepted while a page
// rendered in headless Chromium — the clean JSON behind JS-driven sites, the
// most reliable source of structured data. Stored complete (raw body) so the
// full response is preserved, not just extracted fields.
type ScrapedAPICall struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`

	SourceID     uint      `json:"source_id" gorm:"index;not null"`
	PageURL      string    `json:"page_url" gorm:"type:text"`      // page that triggered the call
	APIURL       string    `json:"api_url" gorm:"type:text;index"` // the intercepted endpoint
	Method       string    `json:"method" gorm:"size:12"`
	ResourceType string    `json:"resource_type" gorm:"size:16"` // XHR | Fetch
	Status       int       `json:"status"`
	ContentType  string    `json:"content_type" gorm:"size:255"`
	Body         string    `json:"body" gorm:"type:text"` // raw response body
	BodySize     int       `json:"body_size"`
	ContentHash  string    `json:"content_hash" gorm:"index;size:64"`
	ScrapedAt    time.Time `json:"scraped_at"`
}

func (ScrapedAPICall) TableName() string { return "scraped_api_calls" }
