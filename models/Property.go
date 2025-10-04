package models

import (
	"encoding/json"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Property struct {
	gorm.Model
	HostID             uint          `json:"hostID"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	PropertyType       string        `json:"propertyType"` // entire_place, private_room, shared_room
	AddressLine1       string        `json:"addressLine1"`
	AddressLine2       string        `json:"addressLine2"`
	City               string        `json:"city"`
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
	Currency           string        `json:"currency"`  // MRO for Mauritania
	Amenities          string        `json:"amenities"` // JSON string
	HouseRules         string        `json:"houseRules"`
	CancellationPolicy string        `json:"cancellationPolicy"`
	Images             string        `json:"images"` // JSON array of URLs
	IsActive           *bool         `json:"isActive"`
	Rating             float32       `json:"rating"`
	Reviews            []Review      `json:"reviews"`
	Reservations       []Reservation `json:"reservations"`
	Host               User          `json:"host" gorm:"foreignKey:HostID;references:ID"`

	// Neighborhood & timing & category mapping
	NeighborhoodDescription string         `json:"neighborhoodDescription" gorm:"column:neighborhood_description;type:text"`
	NearbyAttractions       datatypes.JSON `json:"nearbyAttractions" gorm:"column:nearby_attractions;type:jsonb"`
	CheckInTime             string         `json:"checkInTime" gorm:"column:check_in_time;type:varchar(10)"`
	CheckOutTime            string         `json:"checkOutTime" gorm:"column:check_out_time;type:varchar(10)"`
	PropertyCategoryID      *uint          `json:"propertyCategoryId" gorm:"column:property_category_id"`

	// New policy fields
	BookingMode                      string `json:"bookingMode" gorm:"type:varchar(50);default:'instant'"` // instant, request, hybrid
	SecureCompoundAcknowledged       bool   `json:"secureCompoundAcknowledged" gorm:"default:false"`
	EquipmentViolationPolicyAccepted bool   `json:"equipmentViolationPolicyAccepted" gorm:"default:false"`
	UserSafetyPolicyAccepted         bool   `json:"userSafetyPolicyAccepted" gorm:"default:false"`
	PropertyPolicyAccepted           bool   `json:"propertyPolicyAccepted" gorm:"default:false"`

	// Admin moderation fields
	Status      string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, approved, rejected
	ReviewNotes string `json:"reviewNotes" gorm:"type:text"`
	IsFlagged   bool   `json:"isFlagged" gorm:"default:false;index"`
	FlagReason  string `json:"flagReason" gorm:"type:text"`
}

// Custom JSON marshaling to convert Images and Amenities strings to arrays
func (p *Property) MarshalJSON() ([]byte, error) {
	type Alias Property
	aux := &struct {
		Images    []string `json:"images"`
		Amenities []string `json:"amenities"`
		Host      *User    `json:"host,omitempty"`
		*Alias
	}{
		Images:    []string{},
		Amenities: []string{},
		Host:      nil,
		Alias:     (*Alias)(p),
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
