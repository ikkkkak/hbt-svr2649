package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
)

// GET /admin/reviews?property_id=&rating=&page=&per_page=
func AdminListReviews(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	propertyID := ctx.URLParamDefault("property_id", "")
	rating := ctx.URLParamDefault("rating", "")

	q := storage.DB.Model(&models.Review{})
	if propertyID != "" {
		q = q.Where("property_id = ?", propertyID)
	}
	if rating != "" {
		if r, err := strconv.Atoi(rating); err == nil {
			q = q.Where("stars = ?", r)
		}
	}

	var total int64
	q.Count(&total)

	var items []models.Review
	if err := q.Preload("User").Preload("Property").Offset((page - 1) * perPage).Limit(perPage).Order("created_at DESC").Find(&items).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, items, page, perPage, total)
}

// PATCH /admin/reviews/:id/status { visible, reason }
func AdminUpdateReviewVisibility(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var body struct {
		Visible bool   `json:"visible"`
		Reason  string `json:"reason"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "invalid body")
		return
	}
	var rev models.Review
	if err := storage.DB.First(&rev, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "review not found")
		return
	}
	before := rev
	// Using ReviewNotes field on property is separate; for now store reason in Title suffix
	if !body.Visible {
		rev.Title = rev.Title + " [HIDDEN]"
	}
	if err := storage.DB.Save(&rev).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "review.visibility_update", "review", rev.ID, before, rev)
	ctx.JSON(iris.Map{"data": rev})
}

// DELETE /admin/reviews/:id { reason }
func AdminDeleteReview(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var rev models.Review
	if err := storage.DB.First(&rev, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "review not found")
		return
	}
	before := rev
	if err := storage.DB.Delete(&rev).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "review.delete", "review", before.ID, before, nil)
	ctx.StatusCode(http.StatusNoContent)
}
