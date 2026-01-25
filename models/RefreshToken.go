package models

import (
	"time"

	"gorm.io/gorm"
)

// RefreshToken stores refresh tokens for secure token rotation
type RefreshToken struct {
	gorm.Model
	Token     string    `json:"token" gorm:"uniqueIndex;not null;size:500"`
	UserID    uint      `json:"user_id" gorm:"not null;index"`
	DeviceID  string    `json:"device_id" gorm:"size:255;index"` // Optional: track device sessions
	ExpiresAt time.Time `json:"expires_at" gorm:"not null;index"`
	Revoked   bool      `json:"revoked" gorm:"default:false;index"`
	RevokedAt *time.Time `json:"revoked_at"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
}
