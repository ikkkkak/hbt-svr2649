package models

import (
	"time"

	"gorm.io/gorm"
)

// UserBlock represents when a user blocks another user for direct messages
type UserBlock struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	BlockerID uint           `json:"blocker_id" gorm:"index;not null"` // User who is blocking
	BlockedID uint           `json:"blocked_id" gorm:"index;not null"` // User who is being blocked
	Reason    string         `json:"reason" gorm:"size:200"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// DirectMessage represents a direct message conversation between two users
type DirectMessage struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	SenderID   uint           `json:"sender_id" gorm:"index;not null"`
	ReceiverID uint           `json:"receiver_id" gorm:"index;not null"`
	Content    string         `json:"content" gorm:"type:text;not null"`
	Type       string         `json:"type" gorm:"size:32;default:'text'"` // text, image, property_card
	RefType    string         `json:"ref_type" gorm:"size:32"`            // property, experience
	RefID      *uint          `json:"ref_id" gorm:"index"`
	IsRead     bool           `json:"is_read" gorm:"default:false"`
	ReadAt     *time.Time     `json:"read_at"`
	// Reply support
	ReplyToID *uint          `json:"reply_to_id" gorm:"index"`
	ReplyTo   *DirectMessage `json:"reply_to,omitempty" gorm:"foreignKey:ReplyToID"`
	// Reactions (stored as JSON: {"👍": [1,2], "❤️": [3]})
	Reactions string `json:"reactions" gorm:"type:text"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// MessageReaction represents a reaction to a message
type MessageReaction struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	MessageID uint      `json:"message_id" gorm:"index;not null"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	Emoji     string    `json:"emoji" gorm:"size:16;not null"`
	CreatedAt time.Time `json:"created_at"`
}
