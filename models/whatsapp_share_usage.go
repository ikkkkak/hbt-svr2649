package models

import "time"

// WhatsApp share card funnel events (property sale listing cards).
// Events: sheet_opened | share_started | share_completed | share_failed | share_dismissed
type WhatsAppShareUsageEvent struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
	UserID          uint      `gorm:"index;not null;default:0" json:"user_id"`
	PropertySaleID  uint      `gorm:"index;not null" json:"property_sale_id"`
	Event           string    `gorm:"size:24;index;not null" json:"event"`
	Platform        string    `gorm:"size:16" json:"platform,omitempty"`
	PropertyTitle   string    `gorm:"size:255" json:"property_title,omitempty"`
}

func (WhatsAppShareUsageEvent) TableName() string {
	return "whatsapp_share_usage_events"
}
