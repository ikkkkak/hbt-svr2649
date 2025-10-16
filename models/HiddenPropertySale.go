package models

import "time"

type HiddenPropertySale struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at" gorm:"index"`

	UserID         *uint  `json:"user_id" gorm:"index;uniqueIndex:idx_hidden_user_property_sale"`
	PropertySaleID uint   `json:"property_sale_id" gorm:"index;uniqueIndex:idx_hidden_user_property_sale"`
	Reason         string `json:"reason" gorm:"size:255"`
}
