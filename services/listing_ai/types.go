// Package listing_ai generates property / land listing drafts with LLM + DB location matching.
package listing_ai

// Kind is the listing type the user is creating.
type Kind string

const (
	KindRent  Kind = "rent"
	KindSale  Kind = "sale"
	KindLand  Kind = "land"
)

// GenerateInput is sent by the mobile app after the user fills the quick AI wizard.
type GenerateInput struct {
	Kind         Kind     `json:"kind"`
	Details      string   `json:"details"`
	Price        float64  `json:"price"`
	Currency     string   `json:"currency"`
	Bedrooms     *int     `json:"bedrooms,omitempty"`
	Bathrooms    *int     `json:"bathrooms,omitempty"`
	Area         float64  `json:"area"`
	AreaUnit     string   `json:"area_unit"`
	CityHint     string   `json:"city_hint"`
	ZoneHint     string   `json:"zone_hint"`
	QuartierHint string   `json:"quartier_hint"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	ImageURLs    []string `json:"image_urls"`
	VideoURLs    []string `json:"video_urls"`
	LandType     string   `json:"land_type"`
	PropertyType         string   `json:"property_type"`
	PropertyCategoryID   uint     `json:"property_category_id,omitempty"`
	Language             string   `json:"language"`
	PlotNumber   string   `json:"plot_number"`
	AmenityIDs   []uint   `json:"amenity_ids,omitempty"`
	AmenityNames []string `json:"amenity_names,omitempty"`
}

// Draft is returned to pre-fill the manual listing forms.
type Draft struct {
	Title                     string   `json:"title"`
	Description               string   `json:"description"`
	CityID                    uint     `json:"city_id,omitempty"`
	CityName                  string   `json:"city_name"`
	ZoneID                    uint     `json:"zone_id,omitempty"`
	ZoneName                  string   `json:"zone_name"`
	QuartierID                uint     `json:"quartier_id,omitempty"`
	QuartierName              string   `json:"quartier_name"`
	Bedrooms                  int      `json:"bedrooms,omitempty"`
	Bathrooms                 int      `json:"bathrooms,omitempty"`
	Area                      float64  `json:"area,omitempty"`
	AreaUnit                  string   `json:"area_unit,omitempty"`
	Price                     float64  `json:"price,omitempty"`
	NightlyPrice              float64  `json:"nightly_price,omitempty"`
	PropertyType              string   `json:"property_type,omitempty"`
	PropertyCategoryID        uint     `json:"property_category_id,omitempty"`
	LandType                  string   `json:"land_type,omitempty"`
	Latitude                  float64  `json:"latitude,omitempty"`
	Longitude                 float64  `json:"longitude,omitempty"`
	NeighborhoodDescription   string   `json:"neighborhood_description,omitempty"`
	IndoorFeatures            []string `json:"indoor_features,omitempty"`
	OutdoorFeatures           []string `json:"outdoor_features,omitempty"`
	ImageURLs                 []string `json:"image_urls"`
	VideoURLs                 []string `json:"video_urls"`
	LocationMatchConfidence   string   `json:"location_match_confidence,omitempty"` // high | medium | low
	PlotNumber                string   `json:"plot_number,omitempty"`
	AmenityIDs                []uint   `json:"amenity_ids,omitempty"`
}

// JobStatus tracks async generation progress for the worker.
type JobStatus string

const (
	JobPending    JobStatus = "pending"
	JobProcessing JobStatus = "processing"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
)

// Job is an in-memory generation task (worker processes these).
type Job struct {
	ID        string        `json:"id"`
	UserID    uint          `json:"user_id,omitempty"`
	Status    JobStatus     `json:"status"`
	Progress  string        `json:"progress"`
	Input     GenerateInput `json:"input"`
	Result    *Draft        `json:"result,omitempty"`
	Error     string        `json:"error,omitempty"`
}
