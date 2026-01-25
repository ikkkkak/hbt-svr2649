package models

import "time"

// NotificationEventType for new-video and future events
const (
	NotificationEventNewVideoForProperty = "new_video_for_property"
)

// NotificationEvent represents an event that may trigger notifications (e.g. new video for a property).
// Used for audit and optional async processing. property_kind + property_id or property_sale_id
// identify the "property" in a unified way.
type NotificationEvent struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`

	EventType string `json:"eventType" gorm:"type:varchar(64);not null;index"`

	// Unified property: one of these is set
	PropertyKind   string `json:"propertyKind" gorm:"type:varchar(16);index"` // "rent" | "sale"
	PropertyID     *uint  `json:"propertyId" gorm:"index"`                    // rent: properties.id
	PropertySaleID *uint  `json:"propertySaleId" gorm:"index"`                 // sale: property_sales.id

	// Video that was added
	VideoID     *uint  `json:"videoId" gorm:"index"`     // videos.id (rent)
	VideoSaleID *uint  `json:"videoSaleId" gorm:"index"` // property_sale_videos.id (sale)

	// Processing state
	ProcessedAt *time.Time `json:"processedAt" gorm:"index"`
}

// TableName for NotificationEvent
func (NotificationEvent) TableName() string {
	return "notification_events"
}
