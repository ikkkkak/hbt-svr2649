package models

import (
	"time"

	"gorm.io/gorm"
)

type Video struct {
	gorm.Model
	PropertyID *uint    `json:"propertyID" gorm:"index"` // Nullable for promotional videos
	Property   *Property `json:"property" gorm:"foreignKey:PropertyID;references:ID"`

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
	
	// Promotional video fields (admin-uploaded app demos, tutorials, promotional content)
	IsPromotional bool   `json:"isPromotional" gorm:"default:false;index"` // Mark as promotional/admin video
	Title         string `json:"title" gorm:"type:varchar(255)"`            // Title for promotional videos (e.g., "How to Book a Property")
	Description   string `json:"description" gorm:"type:text"`             // Description for promotional videos
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

// VideoView tracks individual video views (both authenticated and anonymous)
type VideoView struct {
	gorm.Model
	VideoID  uint    `json:"videoID" gorm:"index;not null"`
	Video    Video   `json:"video" gorm:"foreignKey:VideoID;references:ID"`
	UserID   *uint   `json:"userID" gorm:"index"` // Nullable for anonymous viewers
	User     *User   `json:"user" gorm:"foreignKey:UserID;references:ID"`
	DeviceID *string `json:"deviceID" gorm:"index;size:191"` // For anonymous viewers
	IPAddress string `json:"ipAddress" gorm:"size:45"` // IPv6 max length
	ViewedAt  time.Time `json:"viewedAt" gorm:"index"`
}

// VideoViewer represents a viewer for dashboard analytics (aggregated)
type VideoViewer struct {
	UserID    *uint   `json:"userID"`
	User      *User   `json:"user"`
	DeviceID  *string `json:"deviceID"`
	ViewCount int64   `json:"viewCount"`
	FirstViewedAt time.Time `json:"firstViewedAt"`
	LastViewedAt  time.Time `json:"lastViewedAt"`
}
