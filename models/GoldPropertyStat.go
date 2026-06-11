package models

import "time"

// GoldPropertyStat aggregates distribution & engagement metrics for admin-curated
// Gold listings (separate from core property_sales row — analytics layer).
type GoldPropertyStat struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	PropertySaleID      uint      `json:"property_sale_id" gorm:"uniqueIndex;not null"`
	FeedImpressions     int64     `json:"feed_impressions" gorm:"default:0"`
	DetailViews         int64     `json:"detail_views" gorm:"default:0"`
	NotificationsSent   int64     `json:"notifications_sent" gorm:"default:0"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (GoldPropertyStat) TableName() string {
	return "gold_property_stats"
}
