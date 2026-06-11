package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PropertyDNA stores structured listing signals used by host suggestions.
type PropertyDNA struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	PropertyID     uint           `json:"property_id" gorm:"uniqueIndex;not null;index"`
	HostID         uint           `json:"host_id" gorm:"not null;index"`
	CityID         *uint          `json:"city_id" gorm:"index"`
	ZoneID         *uint          `json:"zone_id" gorm:"index"`
	PropertyType   string         `json:"property_type" gorm:"type:varchar(120);index"`
	Price          float64        `json:"price"`
	Bedrooms       int            `json:"bedrooms"`
	AITags         datatypes.JSON `json:"ai_tags" gorm:"type:jsonb"`
	AIPriceTier    string         `json:"ai_price_tier" gorm:"type:varchar(40);index"`
	AIPersonas     datatypes.JSON `json:"ai_personas" gorm:"type:jsonb"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (PropertyDNA) TableName() string {
	return "property_dna"
}

// AIEnrichedUser stores behavior + preference-derived user intent/profile signals.
type AIEnrichedUser struct {
	ID                  uint           `json:"id" gorm:"primaryKey"`
	UserID              uint           `json:"user_id" gorm:"uniqueIndex;not null;index"`
	FavoriteCityID      *uint          `json:"favorite_city_id" gorm:"index"`
	FavoriteZoneID      *uint          `json:"favorite_zone_id" gorm:"index"`
	Intent              string         `json:"intent" gorm:"type:varchar(80);index"`
	TopCityID           *uint          `json:"top_city_id" gorm:"index"`
	TopZoneID           *uint          `json:"top_zone_id" gorm:"index"`
	BehaviorScore       float64        `json:"behavior_score"`
	EngagementLevel     string         `json:"engagement_level" gorm:"type:varchar(32);index"` // cold/warm/hot
	AIBudgetMin         float64        `json:"ai_budget_min"`
	AIBudgetMax         float64        `json:"ai_budget_max"`
	AIPersonaTags       datatypes.JSON `json:"ai_persona_tags" gorm:"type:jsonb"`
	AIUrgency           string         `json:"ai_urgency" gorm:"type:varchar(32);index"` // browsing/researching/ready_to_buy
	PreferredPropertyType string       `json:"preferred_property_type" gorm:"type:varchar(120);index"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (AIEnrichedUser) TableName() string {
	return "ai_enriched_users"
}

// PropertyMatch stores scored host-user-property recommendation candidates.
type PropertyMatch struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	PropertyID      uint           `json:"property_id" gorm:"index;not null"`
	HostID          uint           `json:"host_id" gorm:"index;not null"`
	SuggestedUserID uint           `json:"suggested_user_id" gorm:"index;not null"`
	MatchScore      float64        `json:"match_score" gorm:"index"`
	MatchTier       string         `json:"match_tier" gorm:"type:varchar(24);index"` // excellent/strong/good
	MatchReasons    datatypes.JSON `json:"match_reasons" gorm:"type:jsonb"`
	Status          string         `json:"status" gorm:"type:varchar(24);index"` // pending/notified/viewed/contacted/dismissed/expired
	ConversationID  *uint          `json:"conversation_id" gorm:"index"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (PropertyMatch) TableName() string {
	return "property_matches"
}
