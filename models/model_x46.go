package models

import (
	"time"

	"gorm.io/datatypes"
)

// AIEscalation tracks human handoffs from Meskeny Model X46 chat.
type AIEscalation struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	SessionID       string         `json:"session_id" gorm:"type:varchar(255);index"`
	UserID          *uint          `json:"user_id" gorm:"index"`
	GuestName       string         `json:"guest_name" gorm:"type:varchar(120)"`
	GuestEmail      string         `json:"guest_email" gorm:"type:varchar(255)"`
	GuestPhone      string         `json:"guest_phone" gorm:"type:varchar(40)"`
	TriggerType     string         `json:"trigger_type" gorm:"type:varchar(30)"`
	TriggerScore    float64        `json:"trigger_score"`
	Reason          string         `json:"reason" gorm:"type:text"`
	Urgency         string         `json:"urgency" gorm:"type:varchar(10);index"`
	ContextSummary  string         `json:"context_summary" gorm:"type:text"`
	Status          string         `json:"status" gorm:"type:varchar(20);default:'pending';index"`
	AssignedAgentID *uint          `json:"assigned_agent_id"`
	ResolutionNotes string         `json:"resolution_notes" gorm:"type:text"`
	ResolvedAt      *time.Time     `json:"resolved_at"`
	Metadata        datatypes.JSON `json:"metadata" gorm:"type:jsonb"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	User            *User          `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

func (AIEscalation) TableName() string { return "ai_escalations" }

// AINotification stores Model X46 smart notifications (in-app inbox).
type AINotification struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	UserID         uint           `json:"user_id" gorm:"index"`
	Type           string         `json:"type" gorm:"type:varchar(30);index"`
	Title          string         `json:"title" gorm:"type:varchar(120)"`
	Body           string         `json:"body" gorm:"type:text"`
	RelevanceScore float64        `json:"relevance_score"`
	ActionType     string         `json:"action_type" gorm:"type:varchar(30)"`
	ActionPayload  datatypes.JSON `json:"action_payload" gorm:"type:jsonb"`
	Status         string         `json:"status" gorm:"type:varchar(20);default:'pending';index"`
	ScheduledAt    *time.Time     `json:"scheduled_at"`
	SentAt         *time.Time     `json:"sent_at"`
	ReadAt         *time.Time     `json:"read_at"`
	CreatedAt      time.Time      `json:"created_at"`
}

func (AINotification) TableName() string { return "ai_notifications" }

// AIConversationMemory stores cross-session user preferences extracted by Model X46.
type AIConversationMemory struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	UserID          uint       `json:"user_id" gorm:"index"`
	MemoryType      string     `json:"memory_type" gorm:"type:varchar(30)"`
	Key             string     `json:"key" gorm:"type:varchar(100)"`
	Value           string     `json:"value" gorm:"type:text"`
	Confidence      float64    `json:"confidence"`
	SourceSessionID string     `json:"source_session_id" gorm:"type:varchar(255)"`
	IsActive        bool       `json:"is_active" gorm:"default:true;index"`
	ExpiresAt       *time.Time `json:"expires_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (AIConversationMemory) TableName() string { return "ai_conversation_memory" }
