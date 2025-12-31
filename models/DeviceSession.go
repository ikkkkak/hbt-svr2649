package models

import (
	"gorm.io/gorm"
)

// DeviceSession represents an app usage session for a device
type DeviceSession struct {
	gorm.Model
	DeviceID      string `json:"deviceId" gorm:"index;not null"` // Hashed device identifier
	SessionStart  int64  `json:"sessionStart" gorm:"not null;index"` // Unix timestamp when app was opened
	SessionEnd    *int64 `json:"sessionEnd" gorm:"index"`            // Unix timestamp when app was closed (null if still active)
	DurationSec   *int64 `json:"durationSec"`                       // Session duration in seconds (null if session still active)
	UserID        *uint  `json:"userId" gorm:"index"`               // Optional - if user is logged in
	IsActive      bool   `json:"isActive" gorm:"default:true;index"` // True if session is still active
	AppVersion    string `json:"appVersion" gorm:"type:varchar(50)"` // App version during this session
}

// TableName specifies the table name for DeviceSession
func (DeviceSession) TableName() string {
	return "device_sessions"
}

