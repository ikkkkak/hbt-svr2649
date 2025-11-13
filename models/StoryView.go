package models

import (
	"time"

	"gorm.io/gorm"
)

type StoryView struct {
	gorm.Model
	StoryID       uint      `json:"story_id" gorm:"index;not null"`
	ViewerUserID  *uint     `json:"viewer_user_id" gorm:"index"`
	ViewerPhone   *string   `json:"viewer_phone" gorm:"type:varchar(32)"`
	ViewedAt      time.Time `json:"viewed_at" gorm:"autoCreateTime"`
	ViewedSeconds float64   `json:"viewed_seconds" gorm:"default:0"`
	DeviceInfo    *string   `json:"device_info" gorm:"type:text"`
	Story         *Story    `json:"story" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (StoryView) TableName() string {
	return "story_views"
}
