package models

import "time"

// GroupJoinRequest represents a request from a traveler to join a group
type GroupJoinRequest struct {
	ID      uint            `json:"id" gorm:"primaryKey"`
	GroupID uint            `json:"groupID" gorm:"not null;index"`
	Group   ExperienceGroup `json:"group" gorm:"foreignKey:GroupID"`

	RequesterID uint `json:"requesterID" gorm:"not null;index"`
	Requester   User `json:"requester" gorm:"foreignKey:RequesterID"`

	Status  string `json:"status" gorm:"size:16;index"` // pending, accepted, declined
	Message string `json:"message" gorm:"size:500"`     // optional message from requester

	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	RespondedAt *time.Time `json:"respondedAt"`
}

// Notification represents system notifications for users
type Notification struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	UserID uint `json:"userID" gorm:"not null;index"`
	User   User `json:"user" gorm:"foreignKey:UserID"`

	Type    string `json:"type" gorm:"size:32;index"` // group_join_request, group_invite, etc.
	Title   string `json:"title" gorm:"size:100"`
	Message string `json:"message" gorm:"size:500"`

	// Reference data
	RefType string `json:"refType" gorm:"size:32"` // group, experience, etc.
	RefID   uint   `json:"refID"`

	IsRead    bool       `json:"isRead" gorm:"default:false"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt"`
}
