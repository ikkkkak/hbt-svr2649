package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

// GET /admin/experiences
func AdminListExperiences(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	status := ctx.URLParamDefault("status", "")
	search := strings.TrimSpace(ctx.URLParamDefault("search", ""))
	hostID := ctx.URLParamDefault("host_id", "")
	createdFrom := ctx.URLParamDefault("created_from", "")
	createdTo := ctx.URLParamDefault("created_to", "")

	q := storage.DB.Model(&models.Experience{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if hostID != "" {
		q = q.Where("host_id = ?", hostID)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		q = q.Where("lower(title) LIKE ? OR lower(description) LIKE ?", like, like)
	}
	if createdFrom != "" {
		if t, err := time.Parse(time.RFC3339, createdFrom); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if createdTo != "" {
		if t, err := time.Parse(time.RFC3339, createdTo); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}

	var total int64
	q.Count(&total)

	var items []models.Experience
	if err := q.Preload("Host").Offset((page - 1) * perPage).Limit(perPage).Order("created_at DESC").Find(&items).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, items, page, perPage, total)
}

// GET /admin/experiences/:id
func AdminGetExperience(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var exp models.Experience
	if err := storage.DB.Preload("Host").Preload("Bookings").First(&exp, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "experience not found")
		return
	}
	ctx.JSON(iris.Map{"data": exp, "meta": iris.Map{}, "links": iris.Map{}})
}

// PATCH /admin/experiences/:id/status {status, note}
func AdminUpdateExperienceStatus(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Status == "" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "status required")
		return
	}
	var exp models.Experience
	if err := storage.DB.First(&exp, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "experience not found")
		return
	}
	before := exp
	exp.Status = body.Status
	exp.ReviewNotes = body.Note
	if err := storage.DB.Save(&exp).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "experience.status_update", "experience", exp.ID, before, exp)
	ctx.JSON(iris.Map{"data": exp})
}
