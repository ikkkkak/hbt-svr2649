package models

import (
	"time"

	"gorm.io/gorm"
)

type Video struct {
	gorm.Model
	PropertyID uint     `json:"propertyID" gorm:"not null;index"`
	Property   Property `json:"property" gorm:"foreignKey:PropertyID;references:ID"`

	UserID uint `json:"userID" gorm:"not null;index"`
	User   User `json:"user" gorm:"foreignKey:UserID;references:ID"`

	VideoURL     string  `json:"videoURL" gorm:"not null"`
	ThumbnailURL string  `json:"thumbnailURL"`
	DurationSec  float64 `json:"durationSec"`
	Caption      string  `json:"caption" gorm:"type:text"`

	LikesCount    int64 `json:"likesCount" gorm:"default:0"`
	CommentsCount int64 `json:"commentsCount" gorm:"default:0"`
	SavesCount    int64 `json:"savesCount" gorm:"default:0"`

	// Admin moderation fields
	ViewCount int64  `json:"viewCount" gorm:"default:0;index"`
	IsFlagged bool   `json:"isFlagged" gorm:"default:false;index"`
	Status    string `json:"status" gorm:"type:varchar(20);default:'pending';index"` // pending, approved, rejected
}

type VideoLike struct {
	gorm.Model
	VideoID uint `json:"videoID" gorm:"index;not null"`
	UserID  uint `json:"userID" gorm:"index;not null"`
}

type VideoSave struct {
	gorm.Model
	VideoID uint `json:"videoID" gorm:"index;not null"`
	UserID  uint `json:"userID" gorm:"index;not null"`
}

type VideoComment struct {
	gorm.Model
	VideoID    uint           `json:"videoID" gorm:"index;not null"`
	UserID     uint           `json:"userID" gorm:"index;not null"`
	User       User           `json:"user" gorm:"foreignKey:UserID;references:ID"`
	Content    string         `json:"content" gorm:"type:text;not null"`
	Edited     bool           `json:"edited" gorm:"default:false"`
	ParentID   *uint          `json:"parentID" gorm:"index"` // For replies
	Parent     *VideoComment  `json:"parent" gorm:"foreignKey:ParentID;references:ID"`
	Replies    []VideoComment `json:"replies" gorm:"foreignKey:ParentID;references:ID"`
	LikesCount int64          `json:"likesCount" gorm:"default:0"`
	// For ordering by recency separate from UpdatedAt when edits occur
	PostedAt time.Time `json:"postedAt"`
}

type VideoCommentLike struct {
	gorm.Model
	CommentID uint `json:"commentID" gorm:"index;not null"`
	UserID    uint `json:"userID" gorm:"index;not null"`
}

func (vc *VideoComment) BeforeCreate(tx *gorm.DB) (err error) {
	if vc.PostedAt.IsZero() {
		vc.PostedAt = time.Now()
	}
	return
}
