package models

import (
	"time"

	"gorm.io/gorm"
)

type StoryType string

const (
	StoryTypeImage StoryType = "image"
	StoryTypeVideo StoryType = "video"
)

type Story struct {
	gorm.Model
	UserID          uint      `json:"user_id" gorm:"index;not null"`
	MediaURL        string    `json:"media_url" gorm:"type:text;not null"`
	ThumbURL        string    `json:"thumb_url" gorm:"type:text"`
	Type            StoryType `json:"type" gorm:"type:varchar(16);not null"`
	DurationSeconds int       `json:"duration_seconds" gorm:"default:0"`
	Caption         string    `json:"caption" gorm:"type:text"`
	LikesCount      int64     `json:"likes_count" gorm:"default:0"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
	ExpiresAt       time.Time `json:"expires_at" gorm:"index"`
}

func (Story) TableName() string {
	return "stories"
}


