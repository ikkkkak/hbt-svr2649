package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// NotificationDeliveryLog records each notification sent for deduplication and cooldown.
// Fingerprint = hash(user_id + property_ref + event_type + time_window e.g. date).
// Cooldown: do not notify same user about same property within X hours.
type NotificationDeliveryLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`

	UserID   uint   `json:"userId" gorm:"not null;index:idx_ndl_user_property"`
	EventType string `json:"eventType" gorm:"type:varchar(64);not null;index"`

	// Unified property reference
	PropertyKind   string `json:"propertyKind" gorm:"type:varchar(16);not null;index:idx_ndl_user_property"` // "rent" | "sale"
	PropertyID     *uint  `json:"propertyId" gorm:"index:idx_ndl_user_property"`                             // rent
	PropertySaleID *uint  `json:"propertySaleId" gorm:"index:idx_ndl_user_property"`                         // sale

	// Fingerprint for dedup: hash(user_id, property_ref, event_type, date)
	Fingerprint string `json:"fingerprint" gorm:"type:varchar(64);uniqueIndex:idx_ndl_fingerprint"`
}

// TableName for NotificationDeliveryLog
func (NotificationDeliveryLog) TableName() string {
	return "notification_delivery_logs"
}

// BuildFingerprint creates (user_id + property_ref + event_type + time_window).
// time_window: date only (YYYY-MM-DD) to allow one per day per user/property/event;
// for stricter cooldown we can use hour bucket.
func BuildFingerprint(userID uint, propertyKind string, propertyID, propertySaleID *uint, eventType, timeWindow string) string {
	var ref string
	if propertyKind == "rent" && propertyID != nil {
		ref = fmt.Sprintf("rent:%d", *propertyID)
	} else if propertyKind == "sale" && propertySaleID != nil {
		ref = fmt.Sprintf("sale:%d", *propertySaleID)
	} else {
		ref = "unknown"
	}
	raw := fmt.Sprintf("%d|%s|%s|%s", userID, ref, eventType, timeWindow)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
