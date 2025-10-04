package models

import (
	"time"

	"gorm.io/gorm"
)

// Reservation models an Airbnb-style booking for a listing (Property).
// It replaces the previous apartment/unit concept.
type Reservation struct {
	gorm.Model
	PropertyID uint      `json:"propertyID"`
	GuestID    uint      `json:"guestID"`
	CheckIn    time.Time `json:"checkIn"`
	CheckOut   time.Time `json:"checkOut"`
	NumGuests  int       `json:"numGuests"`
	TotalPrice float32   `json:"totalPrice"`
	Status     string    `json:"status"` // pending, confirmed, rejected, cancelled, completed, expired
	Note       string    `json:"note"`
	ExpiresAt  time.Time `json:"expiresAt"` // 24h window for pending requests

	// Relationships
	Property *Property `json:"property,omitempty" gorm:"foreignKey:PropertyID"`
	Guest    *User     `json:"guest,omitempty" gorm:"foreignKey:GuestID"`
}
