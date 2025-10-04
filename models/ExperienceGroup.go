package models

import "time"

// ExperienceGroup represents a room/lobby for an experience where members join
// until capacity is reached and the host can finalize a reservation.
// status: pending, ready, booked, cancelled
type ExperienceGroup struct {
	ID           uint        `json:"id" gorm:"primaryKey"`
	ExperienceID *uint       `json:"experienceID" gorm:"index"`
	Experience   *Experience `json:"experience" gorm:"foreignKey:ExperienceID"`

	OwnerID uint `json:"ownerID" gorm:"not null;index"`
	Owner   User `json:"owner" gorm:"foreignKey:OwnerID"`

	Name     string `json:"name" gorm:"size:80"`
	Status   string `json:"status" gorm:"size:16;index"`
	Privacy  string `json:"privacy" gorm:"size:16;index"` // public | private
	PhotoURL string `json:"photoURL" gorm:"size:512"`

	// Members relationship
	Members []ExperienceGroupMember `json:"members" gorm:"foreignKey:GroupID"`

	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// ExperienceGroupMember tracks each user's state inside a group.
// state: pending, joined, left, removed
type ExperienceGroupMember struct {
	ID      uint            `json:"id" gorm:"primaryKey"`
	GroupID uint            `json:"groupID" gorm:"not null;index"`
	Group   ExperienceGroup `json:"group" gorm:"foreignKey:GroupID"`

	UserID uint `json:"userID" gorm:"not null;index"`
	User   User `json:"user" gorm:"foreignKey:UserID"`

	State    string     `json:"state" gorm:"size:16;index"`
	Role     string     `json:"role" gorm:"size:16;index"` // owner, cohost, member
	JoinedAt *time.Time `json:"joinedAt"`
	LeftAt   *time.Time `json:"leftAt"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
