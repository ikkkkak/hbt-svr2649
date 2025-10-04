package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Experience struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	HostID uint `json:"hostID" gorm:"not null"`
	Host   User `json:"host" gorm:"foreignKey:HostID"`

	// Basic Info
	Title    string `json:"title" gorm:"not null"`
	City     string `json:"city" gorm:"not null"`
	Language string `json:"language" gorm:"not null"`
	Focus    string `json:"focus" gorm:"not null"`

	// Host Experience
	HasHostedBefore bool   `json:"hasHostedBefore"`
	HostedFor       string `json:"hostedFor"` // "friends", "family", "public"

	// Experience Details
	Description string `json:"description" gorm:"type:text"`
	Duration    int    `json:"duration"` // in minutes
	WhatWeDo    string `json:"whatWeDo" gorm:"type:text"`

	// Requirements
	WhatToBring   string `json:"whatToBring" gorm:"type:text"`
	BringRequired bool   `json:"bringRequired"`

	// Audience
	MinAge          int    `json:"minAge"`
	MaxAge          int    `json:"maxAge"`
	ActivityLevel   string `json:"activityLevel"`   // "light", "moderate", "extreme", "strenuous"
	DifficultyLevel string `json:"difficultyLevel"` // "beginner", "intermediate", "advanced", "expert"

	// Logistics
	GroupSize      int     `json:"groupSize"`
	StartTime      string  `json:"startTime"` // "09:00"
	EndTime        string  `json:"endTime"`   // "17:00"
	PricePerPerson float64 `json:"pricePerPerson"`

	// Group Discounts
	GroupDiscounts datatypes.JSON `json:"groupDiscounts"` // Store discount rules

	// Timing
	ArrivalTime int `json:"arrivalTime"` // minutes before start

	// Policies
	CancellationPolicy string `json:"cancellationPolicy"`

	// Media
	VideoURL string         `json:"videoURL"`
	Photos   datatypes.JSON `json:"photos" gorm:"type:jsonb"` // Store photos as JSON array

	// Status
	Status           string `json:"status"` // "draft", "pending", "approved", "rejected", "live"
	IdentityVerified bool   `json:"identityVerified"`
	ReviewStatus     string `json:"reviewStatus"` // "pending", "approved", "rejected"
	ReviewNotes      string `json:"reviewNotes" gorm:"type:text"`

	// Timestamps
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	ApprovedAt *time.Time `json:"approvedAt"`

	// Relationships
	Bookings []ExperienceBooking `json:"bookings" gorm:"foreignKey:ExperienceID"`
}

type ExperienceBooking struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	ExperienceID     uint           `json:"experience_id" gorm:"not null"`
	GroupID          uint           `json:"group_id" gorm:"not null"`
	ParticipantCount int            `json:"participant_count" gorm:"not null"`
	SelectedDate     time.Time      `json:"selected_date" gorm:"not null"`
	SelectedTime     string         `json:"selected_time"`
	Notes            string         `json:"notes"`
	Status           string         `json:"status" gorm:"default:'confirmed'"` // confirmed, cancelled, completed
	TotalPrice       float64        `json:"total_price" gorm:"not null"`
	UserID           uint           `json:"user_id" gorm:"not null"`
	GuestID          uint           `json:"guest_id" gorm:"not null"`     // Required field for database
	IsRead           bool           `json:"is_read" gorm:"default:false"` // For host dashboard
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Experience Experience      `json:"experience" gorm:"foreignKey:ExperienceID"`
	Group      ExperienceGroup `json:"group" gorm:"foreignKey:GroupID"`
	User       User            `json:"user" gorm:"foreignKey:UserID"`
	Guest      User            `json:"guest" gorm:"foreignKey:GuestID"`
}

// Custom JSON marshaling to handle relationships
func (e *Experience) MarshalJSON() ([]byte, error) {
	type Alias Experience
	return json.Marshal(&struct {
		*Alias
		Bookings []ExperienceBooking `json:"bookings"`
	}{
		Alias:    (*Alias)(e),
		Bookings: e.Bookings,
	})
}
