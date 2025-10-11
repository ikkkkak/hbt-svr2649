package models

import (
	"time"

	"gorm.io/gorm"
)

// VideoReport represents a report made against a video
type VideoReport struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	VideoID     uint           `json:"video_id" gorm:"not null;index"`
	Video       Video          `json:"video" gorm:"foreignKey:VideoID"`
	ReporterID  *uint          `json:"reporter_id" gorm:"index"` // Nullable for anonymous reports
	Reporter    *User          `json:"reporter" gorm:"foreignKey:ReporterID"`
	Reason      string         `json:"reason" gorm:"not null"` // inappropriate, spam, harassment, violence, fake, other
	Description string         `json:"description" gorm:"type:text"`
	Status      string         `json:"status" gorm:"default:'pending'"` // pending, reviewed, resolved, dismissed
	AdminNotes  string         `json:"admin_notes" gorm:"type:text"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// UserFlag represents a user being flagged by another user
type UserFlag struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	FlaggedUserID uint           `json:"flagged_user_id" gorm:"not null;index"`
	FlaggedUser   User           `json:"flagged_user" gorm:"foreignKey:FlaggedUserID"`
	FlaggerID     *uint          `json:"flagger_id" gorm:"index"` // Nullable for anonymous flags
	Flagger       *User          `json:"flagger" gorm:"foreignKey:FlaggerID"`
	Reason        string         `json:"reason" gorm:"not null"`
	Description   string         `json:"description" gorm:"type:text"`
	Status        string         `json:"status" gorm:"default:'active'"` // active, reviewed, resolved, dismissed
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// HiddenVideo represents a video hidden from a user's feed
type HiddenVideo struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	VideoID   uint           `json:"video_id" gorm:"not null;index"`
	Video     Video          `json:"video" gorm:"foreignKey:VideoID"`
	UserID    *uint          `json:"user_id" gorm:"index"` // Nullable for anonymous hides
	User      *User          `json:"user" gorm:"foreignKey:UserID"`
	Reason    string         `json:"reason" gorm:"not null"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}
