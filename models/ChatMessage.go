package models

import "time"

// ChatMessage stores a single message in a group's chat
// Ephemeral by default: messages can have an ExpiresAt for TTL deletion
type ChatMessage struct {
	ID      uint `json:"id" gorm:"primaryKey"`
	GroupID uint `json:"groupID" gorm:"not null;index"`
	Group   ExperienceGroup

	SenderID uint `json:"senderID" gorm:"not null;index"`
	Sender   User `json:"sender" gorm:"foreignKey:SenderID"`

	Content string `json:"content" gorm:"type:text"`
	Color   string `json:"color" gorm:"size:12"`

	// Optional preview attachment (e.g., wishlist card)
	Type               string `json:"type" gorm:"size:24"`    // system|message|wishlist
	RefType            string `json:"refType" gorm:"size:24"` // property|experience
	RefID              *uint  `json:"refID" gorm:"index"`
	PreviewTitle       string `json:"previewTitle" gorm:"size:256"`
	PreviewSubtitle    string `json:"previewSubtitle" gorm:"size:256"`
	PreviewDescription string `json:"previewDescription" gorm:"size:1024"`
	PreviewImageURL    string `json:"previewImageURL" gorm:"size:512"`

	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt" gorm:"index"`
}
