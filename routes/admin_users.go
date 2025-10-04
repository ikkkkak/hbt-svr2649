package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
)

// GET /admin/users handled in admin.go (AdminListUsers)

// GET /admin/users/:id — full user info + verification history + recent activity
func AdminGetUser(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}

	var user models.User
	if err := storage.DB.First(&user, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "user not found")
		return
	}

	var verifs []models.IdentityVerification
	storage.DB.Where("user_id = ?", id).Order("created_at DESC").Find(&verifs)

	var actions []models.AuditLog
	storage.DB.Where("admin_user_id = ?", id).Order("created_at DESC").Limit(50).Find(&actions)

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"user":               user,
			"verifications":      verifs,
			"recentAdminActions": actions,
		},
		"meta":  iris.Map{},
		"links": iris.Map{},
	})
}

// POST /admin/users/:id/verify { status, notes }
func AdminVerifyUser(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}

	var body struct {
		Status string `json:"status"` // pending/verified/rejected
		Notes  string `json:"notes"`
	}
	if err := ctx.ReadJSON(&body); err != nil || (body.Status != "verified" && body.Status != "rejected" && body.Status != "pending") {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "status must be pending/verified/rejected")
		return
	}

	var user models.User
	if err := storage.DB.First(&user, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "user not found")
		return
	}

	before := user
	user.VerificationStatus = body.Status
	if body.Status == "verified" {
		v := true
		user.IsVerified = &v
	}
	if err := storage.DB.Save(&user).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	// Append a verification history row
	claimsStatus := strings.ToLower(body.Status)
	iv := models.IdentityVerification{UserID: user.ID, DocumentType: "admin_review", DocumentURL: "", Status: claimsStatus, Notes: body.Notes}
	storage.DB.Create(&iv)

	utils.Audit(ctx, "user.verify", "user", user.ID, before, user)
	ctx.JSON(iris.Map{"data": iris.Map{"user": user, "verification": iv}})
}
