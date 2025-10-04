package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

// POST /api/feedback — create feedback (auth required, must have profile)
func CreateFeedback(ctx iris.Context) {
	token := jsonWT.Get(ctx)
	if token == nil {
		utils.JSONError(ctx, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	claims, ok := token.(*utils.AccessToken)
	if !ok {
		utils.JSONError(ctx, http.StatusUnauthorized, "unauthorized", "invalid token")
		return
	}

	// Ensure user has a profile record
	var profile models.UserProfile
	if err := storage.DB.Where("user_id = ?", claims.ID).First(&profile).Error; err != nil || profile.ID == 0 {
		utils.JSONError(ctx, http.StatusForbidden, "no_profile", "Please complete your profile before submitting feedback")
		return
	}

	var input struct {
		Title      string `json:"title"`
		Message    string `json:"message"`
		Rating     *int   `json:"rating"`
		Context    string `json:"context"`
		AppVersion string `json:"appVersion"`
		DeviceInfo string `json:"deviceInfo"`
	}
	if err := ctx.ReadJSON(&input); err != nil || input.Message == "" {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "message is required")
		return
	}

	fb := models.Feedback{
		UserID:     claims.ID,
		Title:      input.Title,
		Message:    input.Message,
		Rating:     input.Rating,
		Context:    input.Context,
		AppVersion: input.AppVersion,
		DeviceInfo: input.DeviceInfo,
	}
	if err := storage.DB.Create(&fb).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": fb})
}

// GET /api/admin/feedback — list feedbacks (admin)
func AdminListFeedback(ctx iris.Context) {
	var list []models.Feedback
	if err := storage.DB.Preload("User").Order("created_at DESC").Find(&list).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": list})
}
