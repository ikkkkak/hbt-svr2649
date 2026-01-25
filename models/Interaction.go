package models

import "time"

// EntityType for the Interaction append-only log
const (
	EntityVideo            = "video"
	EntityPropertySaleVideo = "property_sale_video"
	EntityProperty         = "property"
	EntityPropertySale     = "property_sale"
)

// EventType for interactions
const (
	EventVideoView      = "video_view"
	EventPropertyView   = "property_view"
	EventLike           = "like"
	EventSave           = "save"
	EventShare          = "share"
	EventMessageOwner   = "message_owner"
	EventBookingAttempt = "booking_attempt"
)

// Interaction is an append-only log for all user interactions with videos and properties.
// Used for analytics, ML, and the recommendation engine. Never updated or soft-deleted.
type Interaction struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`

	// Entity
	EntityType string `json:"entityType" gorm:"type:varchar(32);not null;index:idx_interactions_entity"`
	EntityID   uint   `json:"entityId" gorm:"not null;index:idx_interactions_entity"` // Video.ID, PropertySaleVideo.ID, Property.ID, or PropertySale.ID

	// Property linkage (for video → property, and for property-level events)
	PropertyID     *uint `json:"propertyId" gorm:"index:idx_interactions_property"`     // Rent: Property.ID
	PropertySaleID *uint `json:"propertySaleId" gorm:"index:idx_interactions_property"` // Sale: PropertySale.ID

	// Event
	EventType string `json:"eventType" gorm:"type:varchar(32);not null;index:idx_interactions_event"`

	// video_view: seconds watched; optional for other events
	WatchDurationSec *float64 `json:"watchDurationSec,omitempty"`

	// Precomputed weight for ranking (e.g. 1.0 base, 2.0 for save, 3.0 for booking_attempt)
	Weight float64 `json:"weight" gorm:"type:decimal(10,4);default:1"`

	// Meaningful view: video ≥3s or ≥30%; property: card opened or gallery viewed
	IsMeaningfulView bool `json:"isMeaningfulView" gorm:"default:false;index"`

	// Actor (at least one of UserID or DeviceID)
	UserID   *uint   `json:"userId" gorm:"index:idx_interactions_user_device"`
	DeviceID *string `json:"deviceId" gorm:"type:varchar(255);index:idx_interactions_user_device"`
}

// TableName for Interaction
func (Interaction) TableName() string {
	return "interactions"
}
