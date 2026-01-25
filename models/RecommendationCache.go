package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RecommendationCache stores precomputed feed snapshots per user/device to avoid
// recomputing on every request. TTL enforced in application logic.
type RecommendationCache struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"deletedAt" gorm:"index"`

	UserID   *uint  `json:"userId" gorm:"index:idx_rec_cache_user_device"`
	DeviceID *string `json:"deviceId" gorm:"type:varchar(255);index:idx_rec_cache_user_device"`

	// Serialized feed: array of {type: "video"|"property"|"property_sale", id, score, ...}
	FeedSnapshot datatypes.JSON `json:"feedSnapshot" gorm:"type:jsonb"`

	// When this cache was computed (for TTL)
	ComputedAt time.Time `json:"computedAt" gorm:"index"`

	// Optional: cursor or page hint for pagination
	NextCursor string `json:"nextCursor" gorm:"type:varchar(64)"`
}

// TableName for RecommendationCache
func (RecommendationCache) TableName() string {
	return "recommendation_caches"
}

// FeedItem represents a single item in the cached feed (for JSON marshalling)
type FeedItem struct {
	Type    string  `json:"type"`    // "video", "property_sale_video", "property", "property_sale"
	ID      uint    `json:"id"`      // entity id
	Score   float64 `json:"score"`
	ExtraID *uint   `json:"extraId,omitempty"` // e.g. property_sale_id for property_sale_video
}

// MarshalFeedSnapshot serializes []FeedItem to JSON for storage
func MarshalFeedSnapshot(items []FeedItem) ([]byte, error) {
	return json.Marshal(items)
}

// UnmarshalFeedSnapshot deserializes JSON into []FeedItem
func UnmarshalFeedSnapshot(data []byte) ([]FeedItem, error) {
	var items []FeedItem
	err := json.Unmarshal(data, &items)
	return items, err
}
