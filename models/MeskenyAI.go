package models

import "time"

// AIInteraction stores a single MeskenyGPT turn for training/analytics.
type AIInteraction struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	SessionID    string    `json:"session_id" gorm:"type:varchar(255);index"`
	UserID       *uint     `json:"user_id" gorm:"index"`
	TurnIndex    int       `json:"turn_index"`
	Lang         string    `json:"lang" gorm:"type:varchar(10)"`
	Intent       string    `json:"intent" gorm:"type:varchar(50)"`
	UserMessage  string    `json:"user_message" gorm:"type:text"`
	SystemPrompt string    `json:"system_prompt" gorm:"type:text"`
	AIResponse   string    `json:"ai_response" gorm:"type:text"`
	ModelUsed    string    `json:"model_used" gorm:"type:varchar(80)"`
	TokensUsed   int       `json:"tokens_used"`
	LatencyMS    int64     `json:"latency_ms"`
	Cities       string    `json:"cities" gorm:"type:text"`
	Zones        string    `json:"zones" gorm:"type:text"`
	PropertyType string    `json:"property_type" gorm:"type:varchar(50)"`
	Budget       string    `json:"budget" gorm:"type:varchar(30)"`
	Purpose      string    `json:"purpose" gorm:"type:varchar(10)"`
	CreatedAt    time.Time `json:"created_at"`
}

func (AIInteraction) TableName() string {
	return "ai_interactions"
}

// AIFeedback stores explicit/implicit signals attached to an interaction.
type AIFeedback struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	InteractionID  uint      `json:"interaction_id" gorm:"index"`
	Signal         string    `json:"signal" gorm:"type:varchar(40)"`
	Value          float64   `json:"value"`
	CreatedAt      time.Time `json:"created_at"`
}

func (AIFeedback) TableName() string {
	return "ai_feedback"
}

// MarketSnapshot is an aggregate row for a (city, zone, type, rooms, purpose).
type MarketSnapshot struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	City          string    `json:"city" gorm:"type:varchar(50);index"`
	Zone          string    `json:"zone" gorm:"type:varchar(80);index"`
	PropertyType  string    `json:"property_type" gorm:"type:varchar(50)"`
	Rooms         int       `json:"rooms"`
	Purpose       string    `json:"purpose" gorm:"type:varchar(10)"`
	PriceMin      int64     `json:"price_min"`
	PriceAvg      int64     `json:"price_avg"`
	PriceMax      int64     `json:"price_max"`
	ListingCount  int       `json:"listing_count"`
	Period        time.Time `json:"period"` // snapshot date
	CreatedAt     time.Time `json:"created_at"`
}

func (MarketSnapshot) TableName() string {
	return "market_snapshots"
}

