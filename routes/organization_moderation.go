package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"net/http"

	"github.com/kataras/iris/v12"
)

// BlockOrganization - POST /api/organizations/{id:uint}/block
func BlockOrganization(ctx iris.Context) {
	fmt.Printf("➡️  BlockOrganization request: method=%s path=%s authUserID=%v targetOrgParam=%s\n",
		ctx.Method(), ctx.Path(), ctx.Values().Get("userID"), ctx.Params().GetString("id"))

	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"success": false, "error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	orgID, err := ctx.Params().GetUint("id")
	if err != nil || orgID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "Invalid organization ID"})
		return
	}

	// If already blocked and active, return idempotent success
	var existing models.UserBlockedOrganization
	if err := storage.DB.Where("user_id = ? AND organization_id = ? AND status = 'active'", userID, orgID).First(&existing).Error; err == nil {
		fmt.Printf("ℹ️  Organization %d already blocked by user %d\n", orgID, userID)
		ctx.JSON(iris.Map{"success": true, "blocked": true, "message": "Organization already blocked"})
		return
	}

	rec := models.UserBlockedOrganization{UserID: userID, OrganizationID: orgID, Status: "active"}
	if err := storage.DB.Create(&rec).Error; err != nil {
		fmt.Printf("❌ Error blocking organization: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "Failed to block organization"})
		return
	}

	fmt.Printf("✅ Organization %d blocked by user %d\n", orgID, userID)
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{"success": true, "blocked": true, "message": "Organization blocked successfully"})
}

// UnblockOrganization - DELETE /api/organizations/{id:uint}/unblock
func UnblockOrganization(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"success": false, "error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	orgID, err := ctx.Params().GetUint("id")
	if err != nil || orgID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "Invalid organization ID"})
		return
	}

	result := storage.DB.Model(&models.UserBlockedOrganization{}).
		Where("user_id = ? AND organization_id = ? AND status = 'active'", userID, orgID).
		Update("status", "inactive")

	if result.Error != nil {
		fmt.Printf("❌ Error unblocking organization: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "Failed to unblock organization"})
		return
	}

	if result.RowsAffected == 0 {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"success": false, "error": "Organization not found in blocked list"})
		return
	}

	fmt.Printf("✅ Organization %d unblocked by user %d\n", orgID, userID)
	ctx.JSON(iris.Map{"success": true, "message": "Organization unblocked successfully"})
}

// GetBlockedOrganizations - GET /api/organization/blocked
// Returns list of organizations blocked by the authenticated user
func GetBlockedOrganizations(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	type BlockedOrg struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		Logo      string `json:"logo"`
		BlockedAt string `json:"blockedAt"`
	}

	var blocked []BlockedOrg
	result := storage.DB.Table("user_blocked_organizations ubo").
		Select("organizations.id, organizations.name, organizations.logo, ubo.created_at as blocked_at").
		Joins("JOIN organizations ON organizations.id = ubo.organization_id").
		Where("ubo.user_id = ? AND ubo.status = 'active'", userID).
		Order("ubo.created_at DESC").
		Scan(&blocked)

	if result.Error != nil {
		fmt.Printf("❌ Error fetching blocked organizations: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch blocked organizations"})
		return
	}

	ctx.JSON(blocked)
}
