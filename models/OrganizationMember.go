package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// OrganizationMember represents a member of an organization with RBAC
// This replaces/enhances the Agent model with role-based access control
type OrganizationMember struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	UserID         uint         `json:"user_id" gorm:"not null;uniqueIndex:idx_user_org"`
	User           User         `json:"user" gorm:"foreignKey:UserID"`
	OrganizationID uint         `json:"organization_id" gorm:"not null;uniqueIndex:idx_user_org"`
	Organization   Organization `json:"organization" gorm:"foreignKey:OrganizationID"`

	// RBAC - Role and Permissions
	Role        string         `json:"role" gorm:"default:'viewer'"`  // admin, manager, editor, viewer
	Permissions datatypes.JSON `json:"permissions" gorm:"type:jsonb"` // Granular permissions array

	// Status
	Status   string `json:"status" gorm:"default:'active'"` // active, suspended, removed
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Agent Information (optional, for backward compatibility)
	LicenseNumber  string   `json:"license_number"`
	Specialization string   `json:"specialization"`
	Experience     int      `json:"experience"`
	Bio            string   `json:"bio"`
	Languages      []string `json:"languages" gorm:"type:json"`

	// Performance Metrics
	TotalSales int     `json:"total_sales" gorm:"default:0"`
	TotalValue float64 `json:"total_value" gorm:"default:0"`
	Rating     float64 `json:"rating" gorm:"default:0"`

	// Timestamps
	JoinedAt  time.Time      `json:"joined_at"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	RemovedAt *time.Time     `json:"removed_at"` // When member was removed/kicked
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	AssignedProperties []PropertySale `json:"assigned_properties" gorm:"foreignKey:AgentID"`
}

// OrganizationInviteCode represents a secure invite code for joining an organization
type OrganizationInviteCode struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	OrganizationID uint         `json:"organization_id" gorm:"not null;index"`
	Organization   Organization `json:"organization" gorm:"foreignKey:OrganizationID"`

	// Code (stored in plaintext for user-friendly format, but still secure with usage limits)
	Code string `json:"code" gorm:"not null;uniqueIndex;size:20"` // Plaintext code like "AG-X7K2M9"

	// Configuration
	CreatedBy  uint      `json:"created_by" gorm:"not null"` // User ID who generated the code
	ExpiresAt  *time.Time `json:"expires_at" gorm:"index"`   // Null if never expires
	MaxUses    *int      `json:"max_uses"`                   // Null if unlimited
	CurrentUses int      `json:"current_uses" gorm:"default:0"` // Current usage count
	IsRevoked  bool      `json:"is_revoked" gorm:"default:false"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// IsValid checks if the invite code is still valid
func (ic *OrganizationInviteCode) IsValid() bool {
	if ic.IsRevoked {
		return false
	}
	// Check expiry (if set)
	if ic.ExpiresAt != nil && time.Now().After(*ic.ExpiresAt) {
		return false
	}
	// Check usage limit (if set)
	if ic.MaxUses != nil && ic.CurrentUses >= *ic.MaxUses {
		return false
	}
	return true
}

// Permission constants for granular RBAC
const (
	// Organization permissions
	PermissionOrgRead          = "org.read"
	PermissionOrgEdit          = "org.edit"
	PermissionOrgManageMembers = "org.manage_members"
	PermissionOrgDelete        = "org.delete"

	// Property permissions
	PermissionPropertyCreate = "property.create"
	PermissionPropertyEdit   = "property.edit"
	PermissionPropertyDelete = "property.delete"
	PermissionPropertyView   = "property.view"

	// Land permissions
	PermissionLandCreate = "land.create"
	PermissionLandEdit   = "land.edit"
	PermissionLandDelete = "land.delete"
	PermissionLandView   = "land.view"

	// Analytics permissions
	PermissionAnalyticsView = "analytics.view"
)

// Role definitions with default permissions
var RolePermissions = map[string][]string{
	"admin": {
		PermissionOrgRead,
		PermissionOrgEdit,
		PermissionOrgManageMembers,
		PermissionOrgDelete,
		PermissionPropertyCreate,
		PermissionPropertyEdit,
		PermissionPropertyDelete,
		PermissionPropertyView,
		PermissionLandCreate,
		PermissionLandEdit,
		PermissionLandDelete,
		PermissionLandView,
		PermissionAnalyticsView,
	},
	"manager": {
		PermissionOrgRead,
		PermissionPropertyCreate,
		PermissionPropertyEdit,
		PermissionPropertyView,
		PermissionLandCreate,
		PermissionLandEdit,
		PermissionLandView,
		PermissionAnalyticsView,
	},
	"editor": {
		PermissionOrgRead,
		PermissionPropertyCreate,
		PermissionPropertyEdit,
		PermissionPropertyView,
		PermissionLandCreate,
		PermissionLandEdit,
		PermissionLandView,
	},
	"viewer": {
		PermissionOrgRead,
		PermissionPropertyView,
		PermissionLandView,
	},
}

// GetDefaultPermissions returns the default permissions for a role
func GetDefaultPermissions(role string) []string {
	if perms, ok := RolePermissions[role]; ok {
		return perms
	}
	return RolePermissions["viewer"] // Default to viewer if role not found
}

// HasPermission checks if the member has a specific permission
func (om *OrganizationMember) HasPermission(permission string) bool {
	var permissions []string

	// Try to unmarshal permissions from datatypes.JSON
	if om.Permissions != nil && len(om.Permissions) > 0 {
		if err := json.Unmarshal(om.Permissions, &permissions); err != nil {
			// If unmarshaling fails, fall back to role defaults
			permissions = GetDefaultPermissions(om.Role)
		}
	} else {
		// If permissions field is empty, use role defaults
		permissions = GetDefaultPermissions(om.Role)
	}

	// If still empty after unmarshaling, use role defaults
	if len(permissions) == 0 {
		permissions = GetDefaultPermissions(om.Role)
	}

	// Check if permission exists in the list
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}
