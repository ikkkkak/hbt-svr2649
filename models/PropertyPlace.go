package models

import "time"

// PropertyPlace stores a nearby place (restaurant, hospital, school) for a property sale.
// Enriched via Google Places API; used to show "Nearby" sections on property detail.
type PropertyPlace struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	PropertySaleID uint     `json:"property_sale_id" gorm:"not null;index"`
	PropertySale *PropertySale `json:"-" gorm:"foreignKey:PropertySaleID"`

	PlaceID     string  `json:"place_id" gorm:"type:text;not null"`
	Name        string  `json:"name" gorm:"type:text;not null"`
	Type        string  `json:"type" gorm:"column:place_type;type:varchar(40);not null"` // restaurant, hospital, school — column name place_type to avoid PostgreSQL reserved word
	Rating      float64 `json:"rating"`
	ReviewsCount int    `json:"reviews_count" gorm:"column:reviews_count"`
	Address     string  `json:"address" gorm:"type:text"`
	Phone       string  `json:"phone" gorm:"type:varchar(50)"`
	Website     string  `json:"website" gorm:"type:text"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	PhotoURL    string  `json:"photo_url" gorm:"column:photo_url;type:text"`
	DistanceKm  float64 `json:"distance_km" gorm:"column:distance_km"` // from property

	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastFetchedAt *time.Time `json:"last_fetched_at" gorm:"column:last_fetched_at"` // refresh if older than 30 days
}

func (PropertyPlace) TableName() string { return "property_places" }
