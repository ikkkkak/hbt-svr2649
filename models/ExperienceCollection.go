package models

import (
	"encoding/json"
	"time"
)

type ExperienceCollection struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	UserID      uint       `json:"userID" gorm:"not null"`
	Name        string     `json:"name" gorm:"not null"`
	Description string     `json:"description"`
	Color       string     `json:"color" gorm:"default:'#00A699'"`
	IsDefault   bool       `json:"isDefault" gorm:"default:false"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt" gorm:"index"`

	// Relationships
	User        User                       `json:"user" gorm:"foreignKey:UserID;references:ID"`
	Experiences []ExperienceCollectionItem `json:"experiences" gorm:"foreignKey:CollectionID;references:ID"`
}

type ExperienceCollectionItem struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CollectionID uint      `json:"collectionID" gorm:"not null"`
	ExperienceID uint      `json:"experienceID" gorm:"not null"`
	AddedAt      time.Time `json:"addedAt" gorm:"default:CURRENT_TIMESTAMP"`

	// Relationships
	Collection ExperienceCollection `json:"collection" gorm:"foreignKey:CollectionID;references:ID"`
	Experience Experience           `json:"experience" gorm:"foreignKey:ExperienceID;references:ID"`
}

// MarshalJSON customizes JSON output for ExperienceCollection
func (ec ExperienceCollection) MarshalJSON() ([]byte, error) {
	type Alias ExperienceCollection
	aux := struct {
		*Alias
		Experiences []ExperienceCollectionItem `json:"experiences,omitempty"`
	}{
		Alias: (*Alias)(&ec),
	}

	// Include experiences with experience details
	if len(ec.Experiences) > 0 {
		aux.Experiences = ec.Experiences
	}

	return json.Marshal(aux)
}
