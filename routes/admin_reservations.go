package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// GET /admin/reservations
func AdminListReservations(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	status := ctx.URLParamDefault("status", "")
	hostID := ctx.URLParamDefault("host_id", "")
	guestID := ctx.URLParamDefault("guest_id", "")
	dateFrom := ctx.URLParamDefault("date_from", "")
	dateTo := ctx.URLParamDefault("date_to", "")

	q := storage.DB.Model(&models.Reservation{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if hostID != "" {
		q = q.Joins("JOIN properties ON properties.id = reservations.property_id").Where("properties.host_id = ?", hostID)
	}
	if guestID != "" {
		q = q.Where("guest_id = ?", guestID)
	}
	if dateFrom != "" {
		if t, err := time.Parse(time.RFC3339, dateFrom); err == nil {
			q = q.Where("check_in >= ?", t)
		}
	}
	if dateTo != "" {
		if t, err := time.Parse(time.RFC3339, dateTo); err == nil {
			q = q.Where("check_out <= ?", t)
		}
	}

	var total int64
	q.Count(&total)

	var items []models.Reservation
	if err := q.Preload("Property").Preload("Guest").Offset((page - 1) * perPage).Limit(perPage).Order("created_at DESC").Find(&items).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, items, page, perPage, total)
}

// GET /admin/reservations/:id
func AdminGetReservation(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var res models.Reservation
	if err := storage.DB.Preload("Property").Preload("Guest").First(&res, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "reservation not found")
		return
	}
	ctx.JSON(iris.Map{"data": res, "meta": iris.Map{}, "links": iris.Map{}})
}

// POST /admin/reservations/:id/cancel { reason }
func AdminCancelReservation(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Reason == "" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "reason required")
		return
	}
	var res models.Reservation
	if err := storage.DB.First(&res, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "reservation not found")
		return
	}
	before := res
	res.Status = "cancelled"
	res.Note = body.Reason
	if err := storage.DB.Save(&res).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "reservation.cancel", "reservation", res.ID, before, res)
	ctx.JSON(iris.Map{"data": res})
}

// PATCH /admin/reservations/:id/status { status }
func AdminUpdateReservationStatus(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Status == "" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "status required")
		return
	}
	var res models.Reservation
	if err := storage.DB.First(&res, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "reservation not found")
		return
	}
	before := res
	res.Status = body.Status
	if err := storage.DB.Save(&res).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "reservation.status_update", "reservation", res.ID, before, res)
	ctx.JSON(iris.Map{"data": res})
}
