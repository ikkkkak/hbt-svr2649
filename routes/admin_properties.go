package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

// GET /admin/properties
func AdminListProperties(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	status := ctx.URLParamDefault("status", "")
	search := strings.TrimSpace(ctx.URLParamDefault("search", ""))
	hostID := ctx.URLParamDefault("host_id", "")
	location := strings.TrimSpace(ctx.URLParamDefault("location", ""))
	createdFrom := ctx.URLParamDefault("created_from", "")
	createdTo := ctx.URLParamDefault("created_to", "")

	q := storage.DB.Model(&models.Property{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if hostID != "" {
		q = q.Where("host_id = ?", hostID)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		q = q.Where("lower(title) LIKE ? OR lower(description) LIKE ? OR lower(city) LIKE ?", like, like, like)
	}
	if location != "" {
		q = q.Where("lower(city) = ? OR lower(state) = ? OR lower(country) = ?", strings.ToLower(location), strings.ToLower(location), strings.ToLower(location))
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

	var props []models.Property
	if err := q.Preload("Host").Offset((page - 1) * perPage).Limit(perPage).Order("created_at DESC").Find(&props).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	utils.JSONPage(ctx, props, page, perPage, total)
}

// GET /admin/properties/:id?include=host,reservations,media,reviews
func AdminGetProperty(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	include := strings.Split(strings.TrimSpace(ctx.URLParamDefault("include", "")), ",")

	var prop models.Property
	q := storage.DB.Model(&models.Property{})
	for _, inc := range include {
		switch strings.TrimSpace(inc) {
		case "host":
			q = q.Preload("Host")
		case "reservations":
			q = q.Preload("Reservations")
		case "media":
			q = q.Preload("Images")
		case "reviews":
			q = q.Preload("Reviews")
		}
	}
	if err := q.First(&prop, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "property not found")
		return
	}
	ctx.JSON(iris.Map{"data": prop, "meta": iris.Map{}, "links": iris.Map{}})
}

// PATCH /admin/properties/:id/status {status, note}
func AdminUpdatePropertyStatus(ctx iris.Context) {
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
	var prop models.Property
	if err := storage.DB.First(&prop, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "property not found")
		return
	}
	before := prop
	prop.Status = body.Status
	prop.ReviewNotes = body.Note
	if err := storage.DB.Save(&prop).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "property.status_update", "property", prop.ID, before, prop)

	// Send push notification to property host about status change
	if before.Status != prop.Status {
		notificationService := services.NewNotificationService()
		go notificationService.SendPropertyStatusNotificationToHost(
			prop.ID,
			prop.HostID,
			prop.Title,
			prop.Status,
		)

		// Auto-assign to location criteria when approved
		if prop.Status == "approved" || prop.Status == "live" {
			if err := AssignSinglePropertyToLocationCriteria(prop.ID); err != nil {
				fmt.Printf("⚠️ Failed to auto-assign approved property %d to location criteria: %v\n", prop.ID, err)
			}
		}
	}

	ctx.JSON(iris.Map{"data": prop})
}

// POST /admin/properties/:id/flag { reason }
func AdminFlagProperty(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "reason required")
		return
	}
	var prop models.Property
	if err := storage.DB.First(&prop, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "property not found")
		return
	}
	before := prop
	prop.IsFlagged = true
	prop.FlagReason = body.Reason
	if err := storage.DB.Save(&prop).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "property.flag", "property", prop.ID, before, prop)
	ctx.JSON(iris.Map{"data": prop})
}
