package models

import (
	"time"
)

type IdentityVerification struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	UserID       uint       `json:"user_id" gorm:"not null;index"`
	DocumentType string     `json:"document_type" gorm:"size:50;not null"`
	DocumentURL  string     `json:"document_url" gorm:"size:512;not null"`
	Status       string     `json:"status" gorm:"size:20;default:'pending';index"` // pending, verified, rejected
	ReviewedBy   *uint      `json:"reviewed_by" gorm:"index"`
	ReviewedAt   *time.Time `json:"reviewed_at"`
	Notes        string     `json:"notes" gorm:"type:text"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
