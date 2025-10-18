package models

import (
	"time"

	"gorm.io/gorm"
)

// Group represents a chat/community
type Group struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null;size:120"`
	Description string         `json:"description" gorm:"size:500"`
	IsPublic    bool           `json:"is_public" gorm:"default:false"`
	OwnerID     uint           `json:"owner_id" gorm:"index;not null"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// GroupMember holds membership and role
type GroupMember struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	GroupID   uint           `json:"group_id" gorm:"index;not null"`
	UserID    uint           `json:"user_id" gorm:"index;not null"`
	Role      string         `json:"role" gorm:"size:20;default:'member'"` // owner, admin, moderator, member
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// GroupInvite represents secure invite tokens
type GroupInvite struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	GroupID   uint           `json:"group_id" gorm:"index;not null"`
	Token     string         `json:"token" gorm:"uniqueIndex;size:64;not null"`
	ExpiresAt time.Time      `json:"expires_at"`
	CreatedBy uint           `json:"created_by" gorm:"index;not null"`
	UsedBy    *uint          `json:"used_by" gorm:"index"`
	UsedAt    *time.Time     `json:"used_at"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// GroupMessage stores messages
type GroupMessage struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	GroupID   uint           `json:"group_id" gorm:"index;not null"`
	UserID    uint           `json:"user_id" gorm:"index;not null"`
	Content   string         `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// GroupMessageRead tracks reads per user
type GroupMessageRead struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	GroupID   uint      `json:"group_id" gorm:"index;not null"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	MessageID uint      `json:"message_id" gorm:"index;not null"`
	ReadAt    time.Time `json:"read_at"`
}

// GroupBan blocks a user from a group
type GroupBan struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	GroupID   uint           `json:"group_id" gorm:"index;not null"`
	UserID    uint           `json:"user_id" gorm:"index;not null"`
	Reason    string         `json:"reason" gorm:"size:200"`
	CreatedBy uint           `json:"created_by" gorm:"index;not null"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}
