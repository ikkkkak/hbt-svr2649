package models

import (
	"time"

	"gorm.io/gorm"
)

// HostModeSwitch tracks when users switch to host mode for the first time
// Used for engagement notifications and learning user behavior
type HostModeSwitch struct {
	gorm.Model
	UserID           uint       `json:"user_id" gorm:"index;not null"`
	User             User       `json:"user,omitempty" gorm:"foreignKey:UserID"`
	SwitchedAt       time.Time  `json:"switched_at" gorm:"not null;index"`
	IsFirstSwitch    bool       `json:"is_first_switch" gorm:"default:true;index"`
	NotificationSent bool       `json:"notification_sent" gorm:"default:false;index"`
	NotificationSentAt *time.Time `json:"notification_sent_at"`
	PropertyAdded    bool       `json:"property_added" gorm:"default:false;index"`
	PropertyAddedAt  *time.Time `json:"property_added_at"`
	// Learning metrics
	TimeToAddProperty *time.Duration `json:"time_to_add_property"` // Time between switch and first property
	UserEngagement    string          `json:"user_engagement" gorm:"type:varchar(50)"` // active, passive, inactive
	LastInteractionAt *time.Time      `json:"last_interaction_at"`
}

// HostModeInteraction tracks user interactions after switching to host mode
// Used for learning and personalizing notifications
type HostModeInteraction struct {
	gorm.Model
	UserID           uint      `json:"user_id" gorm:"index;not null"`
	User             User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
	InteractionType string    `json:"interaction_type" gorm:"type:varchar(50);not null;index"` // property_added, property_viewed, dashboard_opened, etc.
	InteractionData string    `json:"interaction_data" gorm:"type:text"` // JSON string with additional data
	CreatedAt       time.Time `json:"created_at" gorm:"not null;index"`
}
