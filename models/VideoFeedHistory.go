package models

import (
	"time"

	"gorm.io/gorm"
)

// VideoFeedHistory keeps track of which videos a user has already
// seen in their infinite swipe feed so we can avoid repeating the
// same content too frequently.
type VideoFeedHistory struct {
	gorm.Model

	UserID  uint `json:"userID" gorm:"not null;uniqueIndex:idx_feed_history_user_video"`
	VideoID uint `json:"videoID" gorm:"not null;uniqueIndex:idx_feed_history_user_video"`

	// LastDeliveredAt marks when we last surfaced this video in the feed.
	LastDeliveredAt time.Time `json:"lastDeliveredAt" gorm:"index"`

	// SeenCount increments every time the user sees this video in the feed.
	SeenCount int64 `json:"seenCount" gorm:"default:0"`
}

func (VideoFeedHistory) TableName() string {
	return "video_feed_histories"
}
