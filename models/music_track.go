package models

import "gorm.io/gorm"

// MusicTrack — internal royalty-free / owned music library for auto property videos.
type MusicTrack struct {
	gorm.Model
	Title       string  `json:"title" gorm:"size:200;not null"`
	Category    string  `json:"category" gorm:"size:64;index;not null"` // luxury, land, business, urban, default
	FileURL     string  `json:"file_url" gorm:"size:1024;not null"`
	DurationSec float64 `json:"duration_sec"`
	IsActive    bool    `json:"is_active" gorm:"default:true;index"`
	SortOrder   int     `json:"sort_order" gorm:"default:0"`
	Notes       string  `json:"notes,omitempty" gorm:"type:text"`
}
