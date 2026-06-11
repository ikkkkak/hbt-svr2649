package models

import (
	"time"
)

// UserBehaviorSummary is a pre-aggregated roll-up of user_behaviors for host suggestions.
// Refresh via routes.RefreshUserBehaviorSummaryTable (admin/cron) — reads are O(1) per user vs CTE scans.
type UserBehaviorSummary struct {
	UserID       uint    `json:"user_id" gorm:"primaryKey"`
	TopCityID    *uint   `json:"top_city_id" gorm:"index"`
	TopZoneID    *uint   `json:"top_zone_id" gorm:"index"`
	Views90d     int64   `json:"views_90d" gorm:"column:views_90d"`
	Favorites90d int64   `json:"favorites_90d" gorm:"column:favorites_90d"`
	Contacts90d  int64   `json:"contacts_90d" gorm:"column:contacts_90d"`
	AvgPrice180d float64 `json:"avg_price_180d" gorm:"column:avg_price_180d"`
	LastUpdated  time.Time `json:"last_updated" gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (UserBehaviorSummary) TableName() string {
	return "user_behavior_summary"
}
