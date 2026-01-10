package models

import (
	"time"

	"gorm.io/gorm"
)

// AnonymousUserPreference stores preferences learned from anonymous user behavior
type AnonymousUserPreference struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	DeviceID       string         `json:"device_id" gorm:"type:varchar(255);uniqueIndex;not null"` // Unique device identifier
	PhoneNumber    *string        `json:"phone_number" gorm:"type:varchar(20);index"` // Phone number if available
	FavoriteCityID *uint          `json:"favorite_city_id" gorm:"index"`
	FavoriteCityName string       `json:"favorite_city_name" gorm:"type:varchar(255)"`
	FavoriteZoneID *uint          `json:"favorite_zone_id" gorm:"index"`
	FavoriteZoneName string       `json:"favorite_zone_name" gorm:"type:varchar(255)"`
	LastActive     time.Time      `json:"last_active" gorm:"default:CURRENT_TIMESTAMP;index"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// TableName returns the table name for the AnonymousUserPreference model
func (AnonymousUserPreference) TableName() string {
	return "anonymous_user_preferences"
}
