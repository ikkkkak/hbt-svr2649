package models

import (
	"encoding/json"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PropertyListingVideo is attached by APIs when a rent-linked tour exists in `videos` (not stored on properties row).
// Prefer approved rows; pending may appear until admins approve (see location-discovery attachment logic).
type PropertyListingVideo struct {
	VideoID      uint    `json:"videoID"`
	PropertyID   uint    `json:"propertyID"`
	VideoURL     string  `json:"videoURL"`
	ThumbnailURL string  `json:"thumbnailURL,omitempty"`
	Caption      string  `json:"caption,omitempty"`
	DurationSec  float64 `json:"durationSec,omitempty"`
	Status       string  `json:"status,omitempty"` // approved | pending (normalized lowercase)
}

type Property struct {
	gorm.Model
	HostID             uint          `json:"hostID"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	PropertyType       string        `json:"propertyType"` // entire_place, private_room, shared_room
	AddressLine1       string        `json:"addressLine1"`
	AddressLine2       string        `json:"addressLine2"`
	City               string        `json:"city"`
	CityID             *uint         `json:"city_id" gorm:"column:city_id"`
	ZoneID             *uint         `json:"zone_id" gorm:"column:zone_id"`
	QuartierID         *uint         `json:"quartier_id" gorm:"column:quartier_id"`
	CountryID          *uint         `json:"country_id" gorm:"column:country_id;index"`
	State              string        `json:"state"`
	Zip                string        `json:"zip"`
	Country            string        `json:"country"`
	Lat                float32       `json:"lat"`
	Lng                float32       `json:"lng"`
	Capacity           int           `json:"capacity"`
	Bedrooms           int           `json:"bedrooms"`
	Beds               int           `json:"beds"`
	Bathrooms          float32       `json:"bathrooms"`
	NightlyPrice       float32       `json:"nightlyPrice"`
	CleaningFee        float32       `json:"cleaningFee"`
	ServiceFee         float32       `json:"serviceFee"`
	Currency           string        `json:"currency"`  // MRU for Mauritania
	Amenities          string        `json:"amenities"` // JSON string
	HouseRules         string        `json:"houseRules"`
	CancellationPolicy string        `json:"cancellationPolicy"`
	Images             string        `json:"images"` // JSON array of URLs
	IsActive           *bool         `json:"isActive"`
	Rating             float32       `json:"rating"`
	Reviews            []Review      `json:"reviews"`
	Reservations       []Reservation `json:"reservations"`
	Host               User          `json:"host" gorm:"foreignKey:HostID;references:ID"`
	CountryRef         *Country      `json:"countryRef" gorm:"foreignKey:CountryID;references:ID"`
	CityRef            *City         `json:"cityRef" gorm:"foreignKey:CityID;references:ID"`
	ZoneRef            *Zone         `json:"zoneRef" gorm:"foreignKey:ZoneID;references:ID"`

	// Neighborhood & timing & category mapping
	NeighborhoodDescription string         `json:"neighborhoodDescription" gorm:"column:neighborhood_description;type:text"`
	NearbyAttractions       datatypes.JSON `json:"nearbyAttractions" gorm:"column:nearby_attractions;type:jsonb"`
	CheckInTime             string         `json:"checkInTime" gorm:"column:check_in_time;type:varchar(10)"`
	CheckOutTime            string         `json:"checkOutTime" gorm:"column:check_out_time;type:varchar(10)"`
	PropertyCategoryID      *uint          `json:"propertyCategoryId" gorm:"column:property_category_id"`

	// Stored translations (title, description, neighborhood) as JSONB maps { "en": "...", "fr": "...", "ar": "..." }
	TitleTranslations                     datatypes.JSON `json:"title_translations" gorm:"column:title_translations;type:jsonb"`
	DescriptionTranslations               datatypes.JSON `json:"description_translations" gorm:"column:description_translations;type:jsonb"`
	NeighborhoodDescriptionTranslations   datatypes.JSON `json:"neighborhood_description_translations" gorm:"column:neighborhood_description_translations;type:jsonb"`

	// New policy fields
	BookingMode                      string `json:"bookingMode" gorm:"type:varchar(50);default:'instant'"` // instant, request, hybrid
	SecureCompoundAcknowledged       bool   `json:"secureCompoundAcknowledged" gorm:"default:false"`
	EquipmentViolationPolicyAccepted bool   `json:"equipmentViolationPolicyAccepted" gorm:"default:false"`
	UserSafetyPolicyAccepted         bool   `json:"userSafetyPolicyAccepted" gorm:"default:false"`
	PropertyPolicyAccepted           bool   `json:"propertyPolicyAccepted" gorm:"default:false"`

	// Host-only private note (never show to guests; redact on public API responses)
	HostPrivateNote string `json:"hostPrivateNote,omitempty" gorm:"type:text;column:host_private_note"`

	// Admin moderation fields
	Status      string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, approved, rejected
	ReviewNotes string `json:"reviewNotes" gorm:"type:text"`
	IsFlagged   bool   `json:"isFlagged" gorm:"default:false;index"`
	FlagReason  string `json:"flagReason" gorm:"type:text"`

	// ListingVideo is set by discovery/list endpoints when a matching approved video exists (not a DB column).
	ListingVideo *PropertyListingVideo `json:"-" gorm:"-"`
}

// Custom JSON marshaling to convert Images and Amenities strings to arrays
func (p *Property) MarshalJSON() ([]byte, error) {
	type Alias Property
	aux := &struct {
		Images       []string              `json:"images"`
		Amenities    []string              `json:"amenities"`
		Host         *User                 `json:"host,omitempty"`
		ListingVideo *PropertyListingVideo `json:"listingVideo,omitempty"`
		*Alias
	}{
		Images:       []string{},
		Amenities:    []string{},
		Host:         nil,
		ListingVideo: p.ListingVideo,
		Alias:        (*Alias)(p),
	}

	// Parse the JSON string to array for Images
	if p.Images != "" {
		var images []string
		if err := json.Unmarshal([]byte(p.Images), &images); err == nil {
			aux.Images = images
		}
	}

	// Parse the JSON string to array for Amenities
	if p.Amenities != "" {
		var amenities []string
		if err := json.Unmarshal([]byte(p.Amenities), &amenities); err == nil {
			aux.Amenities = amenities
		}
	}

	// Only include host if it has an ID (is loaded) and avoid circular reference
	if p.Host.ID > 0 {
		// Create a copy of the host without the Properties field to avoid circular reference
		hostCopy := p.Host
		hostCopy.Properties = nil // Remove the Properties field to prevent circular reference
		aux.Host = &hostCopy
	}

	return json.Marshal(aux)
}
