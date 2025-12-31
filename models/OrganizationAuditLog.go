package models

import (
	"time"

	"gorm.io/gorm"
)

// OrganizationAuditLog tracks all important actions within an organization
// This provides security, accountability, and debugging capabilities
type OrganizationAuditLog struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	OrganizationID uint         `json:"organization_id" gorm:"not null;index"`
	Organization   Organization `json:"organization" gorm:"foreignKey:OrganizationID"`

	// Action Details
	Action     string `json:"action" gorm:"not null"` // member_joined, member_removed, role_changed, property_created, etc.
	ActionType string `json:"action_type"`             // member_management, property_management, land_management, organization_settings

	// Actor (who performed the action)
	ActorID *uint `json:"actor_id"` // User ID who performed the action
	Actor   *User `json:"actor" gorm:"foreignKey:ActorID"`

	// Target (what was affected)
	TargetType string `json:"target_type"` // user, property, land, organization
	TargetID   *uint  `json:"target_id"`   // ID of the affected entity

	// Details
	Details     string         `json:"details" gorm:"type:text"` // JSON or text description of what changed
	OldValue    string         `json:"old_value" gorm:"type:text"` // Previous value (for updates)
	NewValue    string         `json:"new_value" gorm:"type:text"` // New value (for updates)

	// Metadata
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// Audit action constants
const (
	// Member Management
	ActionMemberJoined    = "member_joined"
	ActionMemberRemoved   = "member_removed"
	ActionMemberSuspended = "member_suspended"
	ActionMemberActivated = "member_activated"
	ActionRoleChanged     = "role_changed"
	ActionPermissionsChanged = "permissions_changed"

	// Property Management
	ActionPropertyCreated = "property_created"
	ActionPropertyUpdated = "property_updated"
	ActionPropertyDeleted = "property_deleted"
	ActionPropertyPublished = "property_published"

	// Land Management
	ActionLandCreated = "land_created"
	ActionLandUpdated = "land_updated"
	ActionLandDeleted = "land_deleted"
	ActionLandPublished = "land_published"

	// Organization Settings
	ActionOrgSettingsUpdated = "org_settings_updated"
	ActionInviteCodeGenerated = "invite_code_generated"
	ActionInviteCodeRevoked   = "invite_code_revoked"
)

// Action type constants
const (
	ActionTypeMemberManagement    = "member_management"
	ActionTypePropertyManagement = "property_management"
	ActionTypeLandManagement     = "land_management"
	ActionTypeOrganizationSettings = "organization_settings"
)

