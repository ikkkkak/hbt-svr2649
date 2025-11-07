package models

import "time"

// MarketingDevice represents a device that has opted in to marketing notifications
type MarketingDevice struct {
	ID uint `json:"id" gorm:"primaryKey"`

	DeviceID       string `json:"deviceId" gorm:"uniqueIndex;size:191"`
	FCMToken       string `json:"fcmToken" gorm:"size:512"`
	MarketingOptIn bool   `json:"marketingOptIn" gorm:"default:true"`

	Locale     string `json:"locale" gorm:"size:32"`
	Timezone   string `json:"timezone" gorm:"size:64"`
	Platform   string `json:"platform" gorm:"size:32"`
	AppVersion string `json:"appVersion" gorm:"size:32"`
	SDKVersion string `json:"sdkVersion" gorm:"size:32"`

	UserID *uint `json:"userId" gorm:"index"`
	User   *User `json:"user"`

	LastSentAt *time.Time `json:"lastSentAt"`
	NextSendAt *time.Time `json:"nextSendAt"`

	CreatedAt time.Time
	UpdatedAt time.Time
}
