package models

import "time"

// ExperienceAvailability marks a single day as available or blocked for an experience.
// We prefer per-day rows for simplicity and fast queries.
type ExperienceAvailability struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	ExperienceID uint       `json:"experienceID" gorm:"not null;index"`
	Experience   Experience `json:"experience" gorm:"foreignKey:ExperienceID"`

	Date   time.Time `json:"date" gorm:"type:date;index:idx_exp_date,unique"`
	Status string    `json:"status" gorm:"size:12;index"` // available | blocked

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
