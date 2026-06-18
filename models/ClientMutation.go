package models

import "time"

// ClientMutation stores idempotent mobile write ACKs (offline queue retries).
type ClientMutation struct {
	ID               uint      `gorm:"primaryKey"`
	ClientMutationID string    `gorm:"uniqueIndex;size:64;not null"`
	UserID           uint      `gorm:"index;not null"`
	Action           string    `gorm:"size:48;not null;index"`
	EntityID         uint      `gorm:"not null"`
	ResponseJSON     string    `gorm:"type:text"`
	CreatedAt        time.Time `gorm:"index"`
}

func (ClientMutation) TableName() string { return "client_mutations" }
