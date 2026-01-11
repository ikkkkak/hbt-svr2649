package models

import (
	"time"

	"gorm.io/gorm"
)

// CrashLog represents a crash report from the mobile app
type CrashLog struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`

	// Error Information
	Error          string         `json:"error" gorm:"type:text;not null"`
	Stack          string         `json:"stack" gorm:"type:text"`
	ComponentStack string         `json:"component_stack" gorm:"type:text"`

	// Context Information
	Phase          string         `json:"phase" gorm:"type:varchar(100)"` // e.g., "render", "api_call", "hooks_initialization"
	Screen         string         `json:"screen" gorm:"type:varchar(100)"` // e.g., "SearchScreen"
	Context        string         `json:"context" gorm:"type:jsonb"` // Additional context as JSON

	// Device Information
	Platform       string         `json:"platform" gorm:"type:varchar(50);not null"` // "ios" or "android"
	OSVersion      string         `json:"os_version" gorm:"type:varchar(50)"`
	DeviceModel    string         `json:"device_model" gorm:"type:varchar(255)"`
	AppVersion     string         `json:"app_version" gorm:"type:varchar(50)"`

	// User Information (optional - may be null for anonymous users)
	UserID         *uint          `json:"user_id" gorm:"index"`
	User           *User           `json:"user,omitempty" gorm:"foreignKey:UserID"`

	// Status
	IsResolved     bool           `json:"is_resolved" gorm:"default:false"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
	ResolvedBy     *uint           `json:"resolved_by"`
	Notes          string         `json:"notes" gorm:"type:text"` // Admin notes

	// Crash Type
	IsFatal        bool           `json:"is_fatal" gorm:"default:false"` // True if app crashed completely
	CrashType      string         `json:"crash_type" gorm:"type:varchar(50)"` // "unhandledError", "unhandledPromiseRejection", "componentError", "apiError"
}

// TableName specifies the table name
func (CrashLog) TableName() string {
	return "crash_logs"
}
