package models

import "time"

// DiscoveryEngagementLog records localized “spotlight” discovery pushes (sales + land)
// for deduplication and per-user / per-device rate limits.
type DiscoveryEngagementLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`

	UserID   *uint  `json:"userId,omitempty" gorm:"index"`
	DeviceID string `json:"deviceId,omitempty" gorm:"size:191;index"`

	EventType string `json:"eventType" gorm:"size:64;index"`

	PropertySaleID *uint `json:"propertySaleId,omitempty" gorm:"index"`
	LandmarkID     *uint `json:"landmarkId,omitempty" gorm:"index"`
}

func (DiscoveryEngagementLog) TableName() string {
	return "discovery_engagement_logs"
}
