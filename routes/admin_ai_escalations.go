package routes

import (
	"net/http"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"

	"github.com/kataras/iris/v12"
)

type adminEscalationRow struct {
	models.AIEscalation
	User *models.User `json:"user,omitempty"`
}

// AdminListAIEscalations GET /api/admin/ai/escalations
func AdminListAIEscalations(ctx iris.Context) {
	status := ctx.URLParam("status")
	limit := ctx.URLParamIntDefault("limit", 100)

	q := storage.DB.Preload("User").Order("created_at DESC").Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var rows []models.AIEscalation
	if err := q.Find(&rows).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": rows})
}

// AdminResolveAIEscalation PATCH /api/admin/ai/escalations/{id:uint}/resolve
func AdminResolveAIEscalation(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid escalation id")
		return
	}
	var body struct {
		Notes  string `json:"notes"`
		Status string `json:"status"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "invalid json")
		return
	}
	status := body.Status
	if status == "" {
		status = "resolved"
	}
	now := time.Now()
	updates := map[string]any{
		"status":            status,
		"resolution_notes":  body.Notes,
		"resolved_at":       &now,
	}
	if err := storage.DB.Model(&models.AIEscalation{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"success": true})
}
