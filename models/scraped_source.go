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
