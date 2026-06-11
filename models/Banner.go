package models

import (
	"time"

	"gorm.io/gorm"
)

// Banner represents a promotional banner shown in the property sale list feed
type Banner struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	ImageURL  string         `json:"image_url" gorm:"not null"`
	LinkURL   string         `json:"link_url"`   // Optional: where tapping the banner goes
	Width     int            `json:"width" gorm:"default:800"`  // Display width (for aspect ratio)
	Height    int            `json:"height" gorm:"default:200"` // Display height (for aspect ratio)
	SortOrder int            `json:"sort_order" gorm:"default:0"` // Lower = higher priority
	IsActive  bool           `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}
