package models

import (
	"time"

	"gorm.io/gorm"
)

// MeskenyKnowledgeEntry is admin-authored, structured knowledge injected into
// MeskenyGPT via retrieval (RAG-style) with strict token/char budgets — not raw chat dumps.
type MeskenyKnowledgeEntry struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// DocType: policy | pricing | zones | faq | product | legal_other
	DocType string `json:"doc_type" gorm:"size:32;index;not null"`
	// Locale: any | ar | fr | en
	Locale string `json:"locale" gorm:"size:8;index;default:any;not null"`
	// IntentScope: comma-separated keys (e.g. search_buy,greeting) or "all"
	IntentScope string `json:"intent_scope" gorm:"type:varchar(255);not null"`
	// MatchKeywords: optional comma-separated substrings; user message must contain one when non-empty
	MatchKeywords string `json:"match_keywords" gorm:"type:varchar(500)"`
	Title string `json:"title" gorm:"size:180;not null"`
	Body  string `json:"body" gorm:"type:text;not null"`

	Priority int  `json:"priority" gorm:"default:0;index"`
	Active   bool `json:"active" gorm:"default:true;index"`

	CreatedByUserID *uint `json:"created_by_user_id,omitempty" gorm:"index"`
}

func (MeskenyKnowledgeEntry) TableName() string {
	return "meskeny_knowledge_entries"
}
