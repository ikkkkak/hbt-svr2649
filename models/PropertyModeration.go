package models

import "time"

type HiddenProperty struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" gorm:"index"`

	UserID     *uint  `json:"user_id" gorm:"index;uniqueIndex:idx_hidden_user_property"`
	PropertyID uint   `json:"property_id" gorm:"index;uniqueIndex:idx_hidden_user_property"`
	Reason     string `json:"reason" gorm:"size:255"`
}

type PropertyReport struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" gorm:"index"`

	ReporterID  *uint  `json:"reporter_id" gorm:"index;uniqueIndex:idx_report_user_property"`
	PropertyID  uint   `json:"property_id" gorm:"index;uniqueIndex:idx_report_user_property"`
	Reason      string `json:"reason" gorm:"size:255"`
	Description string `json:"description" gorm:"size:2000"`
	Status      string `json:"status" gorm:"size:32"`
}
