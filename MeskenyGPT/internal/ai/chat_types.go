package ai

import "time"

// SessionFilterContext is persisted picker/search state per chat session (Spec v2 §7).
type SessionFilterContext struct {
	City      string    `json:"city,omitempty"`
	Zone      string    `json:"zone,omitempty"`
	Quartier  string    `json:"quartier,omitempty"`
	Type      string    `json:"type,omitempty"`
	MinPrice  int64     `json:"min_price,omitempty"`
	MaxPrice  int64     `json:"max_price,omitempty"`
	Bedrooms  int       `json:"bedrooms,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// SessionFilterPatch updates a subset of session filters.
type SessionFilterPatch struct {
	City     *string
	Zone     *string
	Quartier *string
	Type     *string
	MinPrice *int64
	MaxPrice *int64
	Bedrooms *int
}

// ChatMessage is one turn in the conversation (OpenAI-compatible roles).
type ChatMessage struct {
	Role    string `json:"role"`    // "user" | "assistant" | "system"
	Content string `json:"content"`
}
