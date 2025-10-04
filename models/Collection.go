package models

import (
	"encoding/json"
	"time"
)

type Collection struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	UserID      uint       `json:"userID" gorm:"not null"`
	Name        string     `json:"name" gorm:"not null"`
	Description string     `json:"description"`
	Color       string     `json:"color" gorm:"default:'#FF385C'"`
	IsDefault   bool       `json:"isDefault" gorm:"default:false"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt" gorm:"index"`

	// Relationships
	User       User                 `json:"user" gorm:"foreignKey:UserID;references:ID"`
	Properties []CollectionProperty `json:"properties" gorm:"foreignKey:CollectionID;references:ID"`
}

type CollectionProperty struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CollectionID uint      `json:"collectionID" gorm:"not null"`
	PropertyID   uint      `json:"propertyID" gorm:"not null"`
	AddedAt      time.Time `json:"addedAt" gorm:"default:CURRENT_TIMESTAMP"`

	// Relationships
	Collection Collection `json:"collection" gorm:"foreignKey:CollectionID;references:ID"`
	Property   Property   `json:"property" gorm:"foreignKey:PropertyID;references:ID"`
}

// MarshalJSON customizes JSON output for Collection
func (c Collection) MarshalJSON() ([]byte, error) {
	type Alias Collection
	aux := struct {
		*Alias
		Properties []CollectionProperty `json:"properties,omitempty"`
	}{
		Alias: (*Alias)(&c),
	}

	// Include properties with property details
	if len(c.Properties) > 0 {
		aux.Properties = c.Properties
	}

	return json.Marshal(aux)
}
