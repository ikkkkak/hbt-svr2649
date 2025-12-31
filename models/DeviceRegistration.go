package models

import (
	"gorm.io/gorm"
)

// DeviceRegistration represents a device registration record for analytics
type DeviceRegistration struct {
	gorm.Model
	DeviceID      string `json:"deviceId" gorm:"index;not null"` // Hashed or anonymized device identifier
	DeviceModel   string `json:"deviceModel" gorm:"type:varchar(255)"`
	DeviceType    string `json:"deviceType" gorm:"type:varchar(50)"` // phone, tablet, etc.
	Platform      string `json:"platform" gorm:"type:varchar(20);not null;index"` // ios, android
	OSVersion     string `json:"osVersion" gorm:"type:varchar(50)"`
	AppVersion    string `json:"appVersion" gorm:"type:varchar(50)"`
	FirstSeenAt   int64  `json:"firstSeenAt" gorm:"not null;index"` // Unix timestamp
	LastSeenAt    int64  `json:"lastSeenAt" gorm:"index"`           // Unix timestamp
	UserID        *uint  `json:"userId" gorm:"index"`                // Optional - if user is logged in
	IsActive      bool   `json:"isActive" gorm:"default:true;index"`
}

// TableName specifies the table name for DeviceRegistration
func (DeviceRegistration) TableName() string {
	return "device_registrations"
}

