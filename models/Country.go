package models

import (
	"time"

	"gorm.io/gorm"
)

// Country is the root of the location hierarchy: Country → City → Zone → Quartier.
type Country struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Code      string         `json:"code" gorm:"size:8;uniqueIndex;not null"` // ISO-style: MR, SN, MA
	Name      string         `json:"name" gorm:"not null"`
	NameAr    string         `json:"name_ar" gorm:"not null"`
	NameFr    string         `json:"name_fr" gorm:""`
	IsActive  bool           `json:"is_active" gorm:"default:true"`
	SortOrder int            `json:"sort_order" gorm:"default:0"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	Cities []City `json:"cities,omitempty" gorm:"foreignKey:CountryID"`
}
