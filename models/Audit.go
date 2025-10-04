package models

import (
	"time"
)

type AuditLog struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	AdminUserID  uint      `json:"adminUserID" gorm:"index;not null"`
	Action       string    `json:"action" gorm:"size:64;index"`
	ResourceType string    `json:"resourceType" gorm:"size:64;index"`
	ResourceID   uint      `json:"resourceID" gorm:"index"`
	BeforeJSON   string    `json:"beforeJSON" gorm:"type:text"`
	AfterJSON    string    `json:"afterJSON" gorm:"type:text"`
	IPAddress    string    `json:"ipAddress" gorm:"size:64"`
	CreatedAt    time.Time `json:"createdAt"`
}
