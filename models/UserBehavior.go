package models

import (
	"time"

	"gorm.io/gorm"
)

// UserBehavior tracks user interactions with properties for learning preferences
type UserBehavior struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	UserID         *uint          `json:"user_id" gorm:"index"` // Nullable for anonymous users
	DeviceID       string         `json:"device_id" gorm:"type:varchar(255);index"` // For anonymous user tracking
	PhoneNumber    *string        `json:"phone_number" gorm:"type:varchar(20);index"` // Phone number for anonymous users
	PropertyID     uint           `json:"property_id" gorm:"index;not null"`
	PropertyType   string         `json:"property_type" gorm:"type:varchar(50)"` // "sale" or "rent"
	InteractionType string        `json:"interaction_type" gorm:"type:varchar(50);not null"` // "view", "click", "favorite", "contact"
	CityID         *uint          `json:"city_id" gorm:"index"`
	CityName       string         `json:"city_name" gorm:"type:varchar(255);index"`
	ZoneID         *uint          `json:"zone_id" gorm:"index"`
	ZoneName       string         `json:"zone_name" gorm:"type:varchar(255)"`
	TimeSpent      int            `json:"time_spent"` // Time in seconds
	Timestamp      time.Time      `json:"timestamp" gorm:"default:CURRENT_TIMESTAMP;index"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName returns the table name for the UserBehavior model
func (UserBehavior) TableName() string {
	return "user_behaviors"
}
