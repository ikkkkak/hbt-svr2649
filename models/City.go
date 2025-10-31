package models

import (
	"time"

	"gorm.io/gorm"
)

// City represents a city in the system
type City struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"not null"`
	NameAr    string         `json:"name_ar" gorm:"not null"`
	Country   string         `json:"country" gorm:"not null;default:'Mauritania'"`
	CountryAr string         `json:"country_ar" gorm:"not null;default:'موريتانيا'"`
	IsActive  bool           `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Zones []Zone `json:"zones" gorm:"foreignKey:CityID"`
}

// Zone represents a zone/area within a city
type Zone struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	CityID        uint           `json:"city_id" gorm:"not null"`
	Name          string         `json:"name" gorm:"not null"`
	NameAr        string         `json:"name_ar" gorm:"not null"`
	Description   string         `json:"description"`
	DescriptionAr string         `json:"description_ar"`
	IsActive      bool           `json:"is_active" gorm:"default:true"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	City City `json:"city" gorm:"foreignKey:CityID"`
}
