package models

import (
	"time"

	"gorm.io/gorm"
)

// LocationCriteria represents different location-based discovery criteria
type LocationCriteria struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null"`         // "Tevragh Zeina", "Palais des Congrès"
	DisplayName string         `json:"displayName" gorm:"not null"`  // "Properties in Tevragh Zeina"
	Description string         `json:"description"`                  // "Luxury stays in diplomatic quarter"
	CenterLat   float64        `json:"centerLat" gorm:"not null"`    // Center latitude
	CenterLng   float64        `json:"centerLng" gorm:"not null"`    // Center longitude
	Radius      float64        `json:"radius" gorm:"not null"`       // Radius in kilometers
	Priority    int            `json:"priority" gorm:"default:0"`    // Higher priority = shown first
	IsActive    bool           `json:"isActive" gorm:"default:true"` // Enable/disable criteria
	Icon        string         `json:"icon"`                         // Icon name for frontend
	Color       string         `json:"color"`                        // Color theme
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `json:"deletedAt" gorm:"index"`
}

// LocationCriteriaProperty represents the relationship between criteria and properties
// This ensures no property appears in multiple criteria
type LocationCriteriaProperty struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	LocationCriteriaID uint           `json:"locationCriteriaId" gorm:"not null"`
	PropertyID         uint           `json:"propertyId" gorm:"not null"`
	Distance           float64        `json:"distance"`                     // Distance from center in km
	IsActive           bool           `json:"isActive" gorm:"default:true"` // Enable/disable this assignment
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `json:"deletedAt" gorm:"index"`

	// Relationships
	LocationCriteria LocationCriteria `json:"locationCriteria" gorm:"foreignKey:LocationCriteriaID"`
	Property         Property         `json:"property" gorm:"foreignKey:PropertyID"`
}

// GetLocationCriteriaResponse represents the API response for location criteria
type GetLocationCriteriaResponse struct {
	ID            uint    `json:"id"`
	Name          string  `json:"name"`
	DisplayName   string  `json:"displayName"`
	Description   string  `json:"description"`
	CenterLat     float64 `json:"centerLat"`
	CenterLng     float64 `json:"centerLng"`
	Radius        float64 `json:"radius"`
	Priority      int     `json:"priority"`
	IsActive      bool    `json:"isActive"`
	Icon          string  `json:"icon"`
	Color         string  `json:"color"`
	PropertyCount int     `json:"propertyCount"` // Number of properties in this criteria
}

// GetLocationPropertiesResponse represents properties for a specific location criteria
type GetLocationPropertiesResponse struct {
	LocationCriteria GetLocationCriteriaResponse `json:"locationCriteria"`
	Properties       []Property                  `json:"properties"`
	TotalCount       int                         `json:"totalCount"`
}
