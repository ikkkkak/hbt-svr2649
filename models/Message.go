package models

import (
	"time"

	"gorm.io/gorm"
)

type Message struct {
	gorm.Model
	ConversationID uint
	SenderID       uint   `json:"senderID"`
	ReceiverID     uint   `json:"receiverID"`
	Text           string `json:"text"`
	// Optional typed payload for rich messages (e.g., property card)
	Type            string `json:"type" gorm:"size:32"` // text | property_card
	PreviewTitle    string `json:"previewTitle" gorm:"size:256"`
	PreviewSubtitle string `json:"previewSubtitle" gorm:"size:256"`
	PreviewImageURL string `json:"previewImageURL" gorm:"size:512"`
	RefType         string `json:"refType" gorm:"size:32"` // property
	RefID           *uint  `json:"refID" gorm:"index"`
	// Delivery state
	State       string     `json:"state" gorm:"size:16;index"` // sent|delivered|seen
	DeliveredAt *time.Time `json:"deliveredAt"`
	SeenAt      *time.Time `json:"seenAt"`
}
