package models

import (
	"time"

	"gorm.io/gorm"
)

// AIChatSession represents a conversation session with the AI
type AIChatSession struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"index;not null" json:"user_id"`
	Title     string         `gorm:"size:255" json:"title"`
	IsBlocked bool           `gorm:"default:false" json:"is_blocked"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	User     User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Messages []AIChatMessage  `gorm:"foreignKey:SessionID" json:"messages,omitempty"`
}

// AIChatMessage represents a single message in an AI conversation
type AIChatMessage struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	SessionID uint           `gorm:"index;not null" json:"session_id"`
	Role      string         `gorm:"size:20;not null" json:"role"` // "user", "assistant", "system"
	Content   string         `gorm:"type:text;not null" json:"content"`
	IsBlocked bool           `gorm:"default:false" json:"is_blocked"`
	Metadata  string         `gorm:"type:text" json:"metadata,omitempty"` // JSON for quick replies, recommendations
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Session AIChatSession `gorm:"foreignKey:SessionID" json:"-"`
}

// AIQuickReply represents a quick reply button
type AIQuickReply struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Action string `json:"action"`
}

// AIPropertyRecommendation represents a property recommendation from AI
type AIPropertyRecommendation struct {
	ID       uint    `json:"id"`
	Title    string  `json:"title"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	City     string  `json:"city"`
	Bedrooms int     `json:"bedrooms"`
	Image    string  `json:"image"`
	Type     string  `json:"type"` // "rent" or "sale"
}

// TableName specifies the table name for AIChatSession
func (AIChatSession) TableName() string {
	return "ai_chat_sessions"
}

// TableName specifies the table name for AIChatMessage
func (AIChatMessage) TableName() string {
	return "ai_chat_messages"
}

