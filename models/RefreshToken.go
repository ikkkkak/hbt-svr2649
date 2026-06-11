package models

import (
	"time"

	"gorm.io/gorm"
)

// RefreshToken stores refresh tokens for secure token rotation.
// New flow: opaque token, store SHA-256 hash only (token_hash). Legacy: Token (JWT) for backward compat.
type RefreshToken struct {
	gorm.Model
	Token     *string   `json:"-" gorm:"size:500"`                    // Legacy JWT; nil for new opaque flow (avoids unique constraint on empty string)
	TokenHash string    `json:"-" gorm:"column:token_hash;uniqueIndex;size:64"` // SHA-256 hex of opaque token
	UserID    uint      `json:"user_id" gorm:"not null;index"`
	DeviceID  string    `json:"device_id" gorm:"size:255;index"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null;index"`
	Revoked   bool      `json:"revoked" gorm:"default:false;index"`
	RevokedAt *time.Time `json:"revoked_at"`
	User      User      `json:"user" gorm:"foreignKey:UserID"`
}
