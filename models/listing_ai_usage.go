package models

import "time"

// ListingAIUsageEvent records Add-with-AI funnel steps (per user, per listing kind).
// Events: started, completed, failed, published
type ListingAIUsageEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Kind      string    `gorm:"size:16;index;not null" json:"kind"`  // rent | sale | land
	Event     string    `gorm:"size:24;index;not null" json:"event"` // started | completed | failed | published
	JobID     string    `gorm:"size:32;index" json:"job_id,omitempty"`
}

func (ListingAIUsageEvent) TableName() string {
	return "listing_ai_usage_events"
}
