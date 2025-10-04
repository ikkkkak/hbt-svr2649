package models

import (
	"time"
)

type ExperienceInvite struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	ExperienceID uint       `json:"experienceID" gorm:"not null"`
	Experience   Experience `json:"experience" gorm:"foreignKey:ExperienceID"`

	InviterID uint `json:"inviterID" gorm:"not null"`
	Inviter   User `json:"inviter" gorm:"foreignKey:InviterID"`

	InviteeUserID *uint `json:"inviteeUserID"`
	Invitee       *User `json:"invitee" gorm:"foreignKey:InviteeUserID"`

	// Link-based invite (nullable). Use pointer so NULL does not violate unique index across rows
	LinkToken *string    `json:"linkToken" gorm:"uniqueIndex;size:64"`
	ExpiresAt *time.Time `json:"expiresAt"`

	// pending, accepted, declined, expired, cancelled
	Status string `json:"status" gorm:"index;size:16"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ExperienceParticipant struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	ExperienceID uint       `json:"experienceID" gorm:"not null;index"`
	Experience   Experience `json:"experience" gorm:"foreignKey:ExperienceID"`

	UserID uint `json:"userID" gorm:"not null;index"`
	User   User `json:"user" gorm:"foreignKey:UserID"`

	// joined, removed
	Status   string     `json:"status" gorm:"size:16"`
	JoinedAt time.Time  `json:"joinedAt"`
	LeftAt   *time.Time `json:"leftAt"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
