package models

import "time"

// PropertySaleReport stores reports for property sales (public details screen)
type PropertySaleReport struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" gorm:"index"`

	ReporterID     uint   `json:"reporter_id" gorm:"index"`
	PropertySaleID uint   `json:"property_sale_id" gorm:"index"`
	Reason         string `json:"reason" gorm:"size:255"`
	Description    string `json:"description" gorm:"size:2000"`
}
