package models

import (
	"time"

	"gorm.io/gorm"
)

// City represents a city in the system (child of Country).
type City struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CountryID *uint          `json:"country_id" gorm:"index"`
	Name      string         `json:"name" gorm:"not null"`
	NameAr    string         `json:"name_ar" gorm:"not null"`
	Country   string         `json:"country" gorm:"not null;default:'Mauritania'"`
	CountryAr string         `json:"country_ar" gorm:"not null;default:'موريتانيا'"`
	IsActive  bool           `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	CountryRef *Country `json:"countryRef,omitempty" gorm:"foreignKey:CountryID;references:ID"`
	Zones      []Zone   `json:"zones" gorm:"foreignKey:CityID"`
}

// Zone represents a zone/area within a city
type Zone struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	CityID        uint           `json:"city_id" gorm:"not null"`
	HabitatPlanID *uint          `json:"habitat_plan_id,omitempty" gorm:"index"`
	Name          string         `json:"name" gorm:"not null"`
	NameAr        string         `json:"name_ar" gorm:"not null"`
	Description   string         `json:"description"`
	DescriptionAr string         `json:"description_ar"`
	IsActive      bool           `json:"is_active" gorm:"default:true"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	City      City       `json:"city" gorm:"foreignKey:CityID"`
	Quartiers []Quartier `json:"quartiers" gorm:"foreignKey:ZoneID"`
}

// Quartier represents a quartier/neighborhood within a zone
type Quartier struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	ZoneID           uint           `json:"zone_id" gorm:"not null"`
	HabitatSectorID  *uint          `json:"habitat_sector_id,omitempty" gorm:"index"`
	ParentQuartierID *uint          `json:"parent_quartier_id"` // For sub-quartiers
	Name             string         `json:"name" gorm:"not null"`
	NameAr           string         `json:"name_ar" gorm:"not null"`
	IsActive         bool           `json:"is_active" gorm:"default:true"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Zone           Zone       `json:"zone" gorm:"foreignKey:ZoneID"`
	ParentQuartier *Quartier  `json:"parent_quartier" gorm:"foreignKey:ParentQuartierID"`
	SubQuartiers   []Quartier `json:"sub_quartiers" gorm:"foreignKey:ParentQuartierID"`
}
