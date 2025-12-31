package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// GenerateInviteCode generates a secure, user-friendly invite code for an organization
// expiryDays: 0 = never expires, otherwise number of days
// maxUses: 0 = unlimited, otherwise maximum number of uses
func GenerateInviteCode(organizationID uint, createdBy uint, expiryDays int, maxUses int) (string, *models.OrganizationInviteCode, error) {
	// Generate user-friendly code: 6-8 characters, uppercase, with dashes
	// Format: AG-XXXXXX (e.g., AG-X7K2M9)
	// Exclude confusing chars: 0/O, 1/I/l
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Excludes 0, O, 1, I, l
	const codeLength = 6
	
	// Generate random code
	codeBytes := make([]byte, codeLength)
	for i := range codeBytes {
		b := make([]byte, 1)
		rand.Read(b)
		codeBytes[i] = charset[int(b[0])%len(charset)]
	}
	
	codePart := string(codeBytes)
	code := fmt.Sprintf("AG-%s", codePart)
	
	// Check for uniqueness (retry if duplicate)
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		var existing models.OrganizationInviteCode
		if err := storage.DB.Where("code = ?", code).First(&existing).Error; err != nil {
			// Code is unique, break
			break
		}
		// Regenerate if duplicate
		for j := range codeBytes {
			b := make([]byte, 1)
			rand.Read(b)
			codeBytes[j] = charset[int(b[0])%len(charset)]
		}
		codePart = string(codeBytes)
		code = fmt.Sprintf("AG-%s", codePart)
	}
	
	// Calculate expiry
	var expiresAt *time.Time
	if expiryDays > 0 {
		exp := time.Now().Add(time.Duration(expiryDays) * 24 * time.Hour)
		expiresAt = &exp
	}
	
	// Set max uses (nil if unlimited)
	var maxUsesPtr *int
	if maxUses > 0 {
		maxUsesPtr = &maxUses
	}
	
	// Create invite code record
	inviteCode := &models.OrganizationInviteCode{
		OrganizationID: organizationID,
		Code:           code,
		CreatedBy:      createdBy,
		ExpiresAt:      expiresAt,
		MaxUses:        maxUsesPtr,
		CurrentUses:    0,
		IsRevoked:      false,
	}
	
	if err := storage.DB.Create(inviteCode).Error; err != nil {
		return "", nil, fmt.Errorf("failed to create invite code: %v", err)
	}
	
	log.Printf("✅ Generated invite code %s for organization %d", code, organizationID)
	
	return code, inviteCode, nil
}

// ValidateInviteCode validates an invite code and increments usage (but doesn't mark as fully used since codes can be reused)
// Returns the invite code if valid, error otherwise
func ValidateInviteCode(code string) (*models.OrganizationInviteCode, error) {
	// Normalize code (uppercase, trim)
	code = strings.ToUpper(strings.TrimSpace(code))
	
	// Find the code (exact match on plaintext)
	var inviteCode models.OrganizationInviteCode
	if err := storage.DB.Where("code = ?", code).First(&inviteCode).Error; err != nil {
		return nil, fmt.Errorf("invalid invite code")
	}
	
	// Check if still valid (expiry, usage limit, revoked)
	if !inviteCode.IsValid() {
		if inviteCode.IsRevoked {
			return nil, fmt.Errorf("invite code has been revoked")
		}
		if inviteCode.ExpiresAt != nil && time.Now().After(*inviteCode.ExpiresAt) {
			return nil, fmt.Errorf("invite code has expired")
		}
		if inviteCode.MaxUses != nil && inviteCode.CurrentUses >= *inviteCode.MaxUses {
			return nil, fmt.Errorf("invite code usage limit exceeded")
		}
		return nil, fmt.Errorf("invite code is no longer valid")
	}
	
	return &inviteCode, nil
}

// IncrementInviteCodeUsage increments the usage count of an invite code
func IncrementInviteCodeUsage(codeID uint) error {
	return storage.DB.Model(&models.OrganizationInviteCode{}).
		Where("id = ?", codeID).
		UpdateColumn("current_uses", gorm.Expr("current_uses + 1")).Error
}

// JoinOrganization adds a user to an organization with default viewer role
// If user previously left this organization, reactivates their membership instead of creating a new one
// Increments the invite code usage count
func JoinOrganization(userID uint, organizationID uint, inviteCodeID uint) (*models.OrganizationMember, error) {
	// Check if user is already an ACTIVE member of a DIFFERENT organization
	var activeMember models.OrganizationMember
	if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
		First(&activeMember).Error; err == nil {
		if activeMember.OrganizationID != organizationID {
			return nil, fmt.Errorf("user is already a member of organization %d. You must leave your current organization before joining another", activeMember.OrganizationID)
		}
		// User is already an active member of this organization - return existing
		return &activeMember, nil
	}

	// Check if organization exists
	var org models.Organization
	if err := storage.DB.First(&org, organizationID).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %v", err)
	}

	// Check if user previously left this organization (has a removed membership record)
	var previousMember models.OrganizationMember
	if err := storage.DB.Where("user_id = ? AND organization_id = ?", userID, organizationID).
		First(&previousMember).Error; err == nil {
		// User previously left - reactivate their membership
		now := time.Now()
		previousMember.Status = "active"
		previousMember.IsActive = true
		previousMember.RemovedAt = nil
		previousMember.JoinedAt = now // Update joined_at to reflect rejoin
		
		// Reset to default viewer role and permissions
		defaultPermissions := models.GetDefaultPermissions("viewer")
		permissionsJSON, err := json.Marshal(defaultPermissions)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal permissions: %v", err)
		}
		previousMember.Role = "viewer"
		previousMember.Permissions = permissionsJSON

		if err := storage.DB.Save(&previousMember).Error; err != nil {
			return nil, fmt.Errorf("failed to reactivate organization member: %v", err)
		}

		// Increment invite code usage
		if err := IncrementInviteCodeUsage(inviteCodeID); err != nil {
			log.Printf("⚠️ Failed to increment invite code usage: %v", err)
		}

		// Log audit event
		LogOrganizationAudit(organizationID, userID, models.ActionMemberJoined, models.ActionTypeMemberManagement, "user", &userID, fmt.Sprintf("User %d rejoined organization (membership reactivated)", userID), "", "")

		log.Printf("✅ User %d rejoined organization %d (membership reactivated)", userID, organizationID)

		return &previousMember, nil
	}

	// User has never been a member of this organization - create new membership
	// Get default permissions for viewer role
	defaultPermissions := models.GetDefaultPermissions("viewer")
	permissionsJSON, err := json.Marshal(defaultPermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %v", err)
	}

	// Create organization member
	member := &models.OrganizationMember{
		UserID:         userID,
		OrganizationID: organizationID,
		Role:           "viewer", // Default role
		Permissions:    permissionsJSON,
		Status:          "active",
		IsActive:        true,
		JoinedAt:       time.Now(),
	}

	if err := storage.DB.Create(member).Error; err != nil {
		return nil, fmt.Errorf("failed to create organization member: %v", err)
	}

	// Increment invite code usage
	if err := IncrementInviteCodeUsage(inviteCodeID); err != nil {
		log.Printf("⚠️ Failed to increment invite code usage: %v", err)
	}

	// Log audit event
	LogOrganizationAudit(organizationID, userID, models.ActionMemberJoined, models.ActionTypeMemberManagement, "user", &userID, fmt.Sprintf("User %d joined organization", userID), "", "")

	log.Printf("✅ User %d joined organization %d as viewer", userID, organizationID)

	return member, nil
}

// RemoveMember removes a member from an organization
func RemoveMember(organizationID uint, memberID uint, removedBy uint) error {
	var member models.OrganizationMember
	if err := storage.DB.Where("id = ? AND organization_id = ?", memberID, organizationID).
		First(&member).Error; err != nil {
		return fmt.Errorf("member not found: %v", err)
	}

	// Prevent removing the organization owner
	var org models.Organization
	if err := storage.DB.First(&org, organizationID).Error; err != nil {
		return fmt.Errorf("organization not found: %v", err)
	}

	if member.UserID == org.OwnerID {
		return fmt.Errorf("cannot remove the organization owner")
	}

	// Soft delete: mark as removed
	now := time.Now()
	member.Status = "removed"
	member.IsActive = false
	member.RemovedAt = &now

	if err := storage.DB.Save(&member).Error; err != nil {
		return fmt.Errorf("failed to remove member: %v", err)
	}

	// Log audit event
	LogOrganizationAudit(organizationID, removedBy, models.ActionMemberRemoved, models.ActionTypeMemberManagement, "user", &member.UserID, fmt.Sprintf("Member %d removed from organization", member.UserID), "", "")

	log.Printf("✅ Member %d removed from organization %d", memberID, organizationID)

	return nil
}

// UpdateMemberRole updates a member's role and permissions
func UpdateMemberRole(organizationID uint, memberID uint, newRole string, customPermissions []string, updatedBy uint) error {
	var member models.OrganizationMember
	if err := storage.DB.Where("id = ? AND organization_id = ?", memberID, organizationID).
		First(&member).Error; err != nil {
		return fmt.Errorf("member not found: %v", err)
	}

	oldRole := member.Role

	// Get default permissions for the new role
	var permissions []string
	if len(customPermissions) > 0 {
		permissions = customPermissions
	} else {
		permissions = models.GetDefaultPermissions(newRole)
	}

	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %v", err)
	}

	// Update member
	member.Role = newRole
	member.Permissions = permissionsJSON

	if err := storage.DB.Save(&member).Error; err != nil {
		return fmt.Errorf("failed to update member role: %v", err)
	}

	// Log audit event
	oldValue := fmt.Sprintf(`{"role": "%s"}`, oldRole)
	newValue := fmt.Sprintf(`{"role": "%s", "permissions": %s}`, newRole, string(permissionsJSON))
	LogOrganizationAudit(organizationID, updatedBy, models.ActionRoleChanged, models.ActionTypeMemberManagement, "user", &member.UserID, fmt.Sprintf("Role changed from %s to %s", oldRole, newRole), oldValue, newValue)

	log.Printf("✅ Member %d role updated from %s to %s in organization %d", memberID, oldRole, newRole, organizationID)

	return nil
}

// GetOrganizationMembers returns all active members of an organization
func GetOrganizationMembers(organizationID uint) ([]models.OrganizationMember, error) {
	var members []models.OrganizationMember
	if err := storage.DB.Where("organization_id = ? AND status = ? AND is_active = ?", organizationID, "active", true).
		Preload("User").
		Order("joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch members: %v", err)
	}

	return members, nil
}

// GetUserOrganization returns the organization the user belongs to (if any)
// Checks both if user is owner OR member
func GetUserOrganization(userID uint) (*models.Organization, *models.OrganizationMember, error) {
	// First check if user is owner of an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		// User is owner - return organization with nil member (owner is not in members table)
		return &organization, nil, nil
	}

	// If not owner, check if user is a member
	var member models.OrganizationMember
	if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
		Preload("Organization").
		Preload("User").
		First(&member).Error; err != nil {
		return nil, nil, nil // User is not a member of any organization
	}

	return &member.Organization, &member, nil
}

// CheckUserCanCreatePersonalContent checks if a user can create personal properties/lands
// Returns false if user is a member of an organization
func CheckUserCanCreatePersonalContent(userID uint) (bool, error) {
	var member models.OrganizationMember
	if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
		First(&member).Error; err != nil {
		// User is not a member - can create personal content
		return true, nil
	}

	// User is a member - cannot create personal content
	return false, nil
}

// LeaveOrganization allows a member to leave an organization themselves
// Security: Only members (not owners) can leave. Owners must delete the organization.
func LeaveOrganization(userID uint) error {
	// Check if user is a member (not owner)
	var member models.OrganizationMember
	if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
		Preload("Organization").
		First(&member).Error; err != nil {
		return fmt.Errorf("you are not a member of any organization")
	}

	// Prevent organization owner from leaving (they must delete the organization instead)
	var org models.Organization
	if err := storage.DB.First(&org, member.OrganizationID).Error; err != nil {
		return fmt.Errorf("organization not found: %v", err)
	}

	if org.OwnerID == userID {
		return fmt.Errorf("organization owners cannot leave. Please delete the organization instead or transfer ownership")
	}

	// Soft delete: mark as removed
	now := time.Now()
	member.Status = "removed"
	member.IsActive = false
	member.RemovedAt = &now

	if err := storage.DB.Save(&member).Error; err != nil {
		return fmt.Errorf("failed to leave organization: %v", err)
	}

	// Log audit event
	LogOrganizationAudit(member.OrganizationID, userID, models.ActionMemberRemoved, models.ActionTypeMemberManagement, "user", &userID, fmt.Sprintf("User %d left organization voluntarily", userID), "", "")

	log.Printf("✅ User %d left organization %d", userID, member.OrganizationID)

	return nil
}

// LogOrganizationAudit logs an audit event for an organization
func LogOrganizationAudit(organizationID uint, actorID uint, action string, actionType string, targetType string, targetID *uint, details string, oldValue string, newValue string) {
	auditLog := &models.OrganizationAuditLog{
		OrganizationID: organizationID,
		ActorID:       &actorID,
		Action:        action,
		ActionType:    actionType,
		TargetType:    targetType,
		TargetID:      targetID,
		Details:       details,
		OldValue:      oldValue,
		NewValue:      newValue,
	}

	if err := storage.DB.Create(auditLog).Error; err != nil {
		log.Printf("⚠️ Failed to create audit log: %v", err)
	}
}

