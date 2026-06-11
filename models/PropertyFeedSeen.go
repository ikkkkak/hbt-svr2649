package models

import "time"

// PropertyFeedSeen tracks which property sale cards were delivered
// to a user/device in the smart sale feed, used to reduce repetition.
type PropertyFeedSeen struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	UserID     *uint     `json:"user_id" gorm:"index;uniqueIndex:idx_user_property"`
	DeviceID   string    `json:"device_id" gorm:"type:varchar(255);index;uniqueIndex:idx_device_property"`
	PropertyID uint      `json:"property_id" gorm:"index;not null;uniqueIndex:idx_user_property;uniqueIndex:idx_device_property"`
	SeenAt     time.Time `json:"seen_at" gorm:"index;default:CURRENT_TIMESTAMP"`
	CreatedAt  time.Time `json:"created_at"`
}

func (PropertyFeedSeen) TableName() string {
	return "property_feed_seen"
}

