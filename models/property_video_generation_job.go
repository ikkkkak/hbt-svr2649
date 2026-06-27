package models

import (
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PropertyVideoGenerationJob tracks async slideshow generation from listing images.
// Status: pending | processing | completed | failed
type PropertyVideoGenerationJob struct {
	gorm.Model
	UserID         uint           `json:"user_id" gorm:"index;not null"`
	EntityType     string         `json:"entity_type" gorm:"size:16;index;not null"` // sale | rent
	EntityID       uint           `json:"entity_id" gorm:"index;not null"`
	Status         string         `json:"status" gorm:"size:20;default:pending;index"`
	Progress       int            `json:"progress" gorm:"default:0"`
	ErrorMessage   string         `json:"error_message,omitempty" gorm:"type:text"`
	ImageURLs      datatypes.JSON `json:"image_urls" gorm:"type:jsonb"`
	MusicTrackID   *uint          `json:"music_track_id" gorm:"index"`
	OutputVideoURL string         `json:"output_video_url" gorm:"size:1024"`
	PropertyType   string         `json:"property_type" gorm:"size:64"`
	OverlayTitle   string         `json:"overlay_title" gorm:"size:300"`
	OverlayLocation string        `json:"overlay_location" gorm:"size:300"`
	OverlayArea    string         `json:"overlay_area" gorm:"size:64"`
	OverlayPrice   string         `json:"overlay_price" gorm:"size:64"`
	OverlayCTA     string         `json:"overlay_cta" gorm:"size:120"`
}
