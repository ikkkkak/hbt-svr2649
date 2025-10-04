package models

import "gorm.io/gorm"

type Review struct {
	gorm.Model
	UserID        uint         `json:"userID" gorm:"not null;index;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	PropertyID    uint         `json:"propertyID" gorm:"not null;index;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ReservationID *uint        `json:"reservationID" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // Link to specific reservation
	Reservation   *Reservation `json:"reservation,omitempty" gorm:"foreignKey:ReservationID"`
	User          User         `json:"user" gorm:"foreignKey:UserID"`
	Property      Property     `json:"property" gorm:"foreignKey:PropertyID"`
	Title         string       `json:"title"`
	Body          string       `json:"body" gorm:"type:text"`
	Stars         int          `json:"stars" gorm:"not null;check:stars >= 1 AND stars <= 5"`
	IsVerified    bool         `json:"isVerified" gorm:"default:false"` // Verified stay
}
