package models

import (
	"time"

	"gorm.io/gorm"
)

type NotificationPreference struct {
	ID                   uint           `json:"id" gorm:"primaryKey"`
	UserID               *uint          `json:"user_id" gorm:"index"` // Nullable for anonymous users
	PushToken            string         `json:"push_token" gorm:"not null;index"`
	Language             string         `json:"language" gorm:"not null;default:'en'"`
	Location             string         `json:"location" gorm:"not null"`
	Latitude             float64        `json:"latitude" gorm:"not null"`
	Longitude            float64        `json:"longitude" gorm:"not null"`
	Enabled              bool           `json:"enabled" gorm:"default:true"`
	LastActive           *time.Time     `json:"last_active" gorm:"default:CURRENT_TIMESTAMP"`
	LastNotificationSent *time.Time     `json:"last_notification_sent"` // Track when notifications were last sent
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName returns the table name for the NotificationPreference model
func (NotificationPreference) TableName() string {
	return "notification_preferences"
}
