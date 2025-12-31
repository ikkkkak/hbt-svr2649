package middleware

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
)

// OrganizationIsolationMiddleware ensures all queries are scoped to the user's organization
// This is CRITICAL for multi-tenant security
func OrganizationIsolationMiddleware(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	userID := userIDInterface.(uint)

	// Get user's organization membership
	var member models.OrganizationMember
	if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
		First(&member).Error; err != nil {
		// User is not a member of any organization - allow access to personal resources only
		ctx.Values().Set("organizationID", nil)
		ctx.Values().Set("isOrgMember", false)
		ctx.Next()
		return
	}

	// Set organization context
	ctx.Values().Set("organizationID", member.OrganizationID)
	ctx.Values().Set("isOrgMember", true)
	ctx.Values().Set("memberRole", member.Role)
	ctx.Values().Set("memberPermissions", member.Permissions)
	ctx.Values().Set("memberID", member.ID)

	ctx.Next()
}

// RequireOrganizationMiddleware ensures the user is a member of an organization
func RequireOrganizationMiddleware(ctx iris.Context) {
	isOrgMember := ctx.Values().Get("isOrgMember")
	if isOrgMember == nil || !isOrgMember.(bool) {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You must be a member of an organization to perform this action"})
		return
	}
	ctx.Next()
}

// RequirePermissionMiddleware checks if the user has a specific permission
func RequirePermissionMiddleware(permission string) iris.Handler {
	return func(ctx iris.Context) {
		// Admin always has all permissions
		memberRole := ctx.Values().Get("memberRole")
		if memberRole != nil && memberRole.(string) == "admin" {
			ctx.Next()
			return
		}

		// Check permissions
		permissionsJSON := ctx.Values().Get("memberPermissions")
		if permissionsJSON == nil {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Insufficient permissions"})
			return
		}

		var permissions []string
		if err := json.Unmarshal(permissionsJSON.([]byte), &permissions); err != nil {
			log.Printf("⚠️ Error unmarshaling permissions: %v", err)
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Insufficient permissions"})
			return
		}

		// Check if user has the required permission
		hasPermission := false
		for _, perm := range permissions {
			if perm == permission {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": fmt.Sprintf("Permission required: %s", permission)})
			return
		}

		ctx.Next()
	}
}

// RequireRoleMiddleware checks if the user has a specific role or higher
func RequireRoleMiddleware(requiredRole string) iris.Handler {
	roleHierarchy := map[string]int{
		"viewer":  1,
		"editor":  2,
		"manager": 3,
		"admin":   4,
	}

	return func(ctx iris.Context) {
		memberRole := ctx.Values().Get("memberRole")
		if memberRole == nil {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "You must be a member of an organization"})
			return
		}

		userRole := memberRole.(string)
		userLevel := roleHierarchy[userRole]
		requiredLevel := roleHierarchy[requiredRole]

		if userLevel < requiredLevel {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": fmt.Sprintf("Role required: %s or higher", requiredRole)})
			return
		}

		ctx.Next()
	}
}

// RequireOrganizationOwnerMiddleware ensures the user is the owner of the organization
func RequireOrganizationOwnerMiddleware(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	userID := userIDInterface.(uint)

	// Get organization ID from URL or context
	orgIDStr := ctx.Params().Get("organizationId")
	if orgIDStr == "" {
		orgIDStr = ctx.Params().Get("id")
	}

	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid organization ID"})
		return
	}

	// Check if user is the owner
	var org models.Organization
	if err := storage.DB.First(&org, uint(orgID)).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	if org.OwnerID != userID {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Only the organization owner can perform this action"})
		return
	}

	ctx.Values().Set("organizationID", uint(orgID))
	ctx.Next()
}

// GetOrganizationIDFromContext safely gets the organization ID from context
func GetOrganizationIDFromContext(ctx iris.Context) (*uint, error) {
	orgIDInterface := ctx.Values().Get("organizationID")
	if orgIDInterface == nil {
		return nil, fmt.Errorf("organization ID not found in context")
	}

	orgID, ok := orgIDInterface.(uint)
	if !ok {
		return nil, fmt.Errorf("invalid organization ID type")
	}

	return &orgID, nil
}

// GetMemberFromContext safely gets the member info from context
func GetMemberFromContext(ctx iris.Context) (*MemberContext, error) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		return nil, fmt.Errorf("user ID not found in context")
	}

	userID := userIDInterface.(uint)

	isOrgMember := ctx.Values().Get("isOrgMember")
	if isOrgMember == nil || !isOrgMember.(bool) {
		return &MemberContext{
			UserID:         userID,
			IsOrgMember:    false,
			OrganizationID: nil,
		}, nil
	}

	orgIDInterface := ctx.Values().Get("organizationID")
	memberRole := ctx.Values().Get("memberRole")
	memberIDInterface := ctx.Values().Get("memberID")

	var orgID *uint
	if orgIDInterface != nil {
		id := orgIDInterface.(uint)
		orgID = &id
	}

	var role string
	if memberRole != nil {
		role = memberRole.(string)
	}

	var memberID *uint
	if memberIDInterface != nil {
		id := memberIDInterface.(uint)
		memberID = &id
	}

	return &MemberContext{
		UserID:         userID,
		IsOrgMember:    true,
		OrganizationID: orgID,
		Role:           role,
		MemberID:       memberID,
	}, nil
}

// MemberContext holds the member information from context
type MemberContext struct {
	UserID         uint
	IsOrgMember    bool
	OrganizationID *uint
	Role           string
	MemberID       *uint
}

