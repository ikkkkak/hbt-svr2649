package models

import "gorm.io/gorm"

// UserBlockedOrganization maps a user blocking an organization and should be used to
// exclude organization-owned content from user-visible queries.
type UserBlockedOrganization struct {
	gorm.Model
	UserID         uint   `gorm:"index;not null"`
	OrganizationID uint   `gorm:"index;not null"`
	Status         string `gorm:"type:varchar(16);default:active"`
}
