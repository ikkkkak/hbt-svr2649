package models

import "time"

// LandmarkReport stores reports for publicly visible landmarks
type LandmarkReport struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" gorm:"index"`

	ReporterID  uint   `json:"reporter_id" gorm:"index"`
	LandmarkID  uint   `json:"landmark_id" gorm:"index"`
	Reason      string `json:"reason" gorm:"size:255"`
	Description string `json:"description" gorm:"size:2000"`
}
