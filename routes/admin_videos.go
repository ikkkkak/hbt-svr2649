package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"

	"github.com/kataras/iris/v12"
)

// GET /admin/videos
func AdminListVideos(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}
	status := ctx.URLParamDefault("status", "")
	isFlagged := ctx.URLParamDefault("is_flagged", "")
	propertyID := ctx.URLParamDefault("property_id", "")
	uploaderID := ctx.URLParamDefault("uploader_id", "")
	sort := ctx.URLParamDefault("sort", "newest")

	q := storage.DB.Model(&models.Video{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if isFlagged == "true" {
		q = q.Where("is_flagged = true")
	}
	if propertyID != "" {
		q = q.Where("property_id = ?", propertyID)
	}
	if uploaderID != "" {
		q = q.Where("user_id = ?", uploaderID)
	}

	switch sort {
	case "most_liked":
		q = q.Order("likes_count DESC")
	case "most_commented":
		q = q.Order("comments_count DESC")
	case "most_viewed":
		q = q.Order("view_count DESC")
	default:
		q = q.Order("created_at DESC")
	}

	var total int64
	q.Count(&total)
	var items []models.Video
	if err := q.Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, items, page, perPage, total)
}

// GET /admin/videos/:id
func AdminGetVideo(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var v models.Video
	if err := storage.DB.Preload("Property").Preload("User").First(&v, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "video not found")
		return
	}
	ctx.JSON(iris.Map{"data": v, "meta": iris.Map{}, "links": iris.Map{}})
}

// PATCH /admin/videos/:id/status { status }
func AdminUpdateVideoStatus(ctx iris.Context) {
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
	var v models.Video
	if err := storage.DB.First(&v, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "video not found")
		return
	}
	before := v
	v.Status = body.Status
	if err := storage.DB.Save(&v).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "video.status_update", "video", v.ID, before, v)
	ctx.JSON(iris.Map{"data": v})
}

// POST /admin/videos/:id/force_unpublish { reason }
func AdminForceUnpublishVideo(ctx iris.Context) {
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
	var v models.Video
	if err := storage.DB.First(&v, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "video not found")
		return
	}
	before := v
	v.IsFlagged = true
	v.Status = "rejected"
	if err := storage.DB.Save(&v).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "video.force_unpublish", "video", v.ID, before, v)
	ctx.JSON(iris.Map{"data": v})
}

// GET /admin/videos/:id/comments
func AdminListVideoComments(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}
	q := storage.DB.Model(&models.VideoComment{}).Where("video_id = ?", id)
	var total int64
	q.Count(&total)
	var items []models.VideoComment
	if err := q.Preload("User").Offset((page - 1) * perPage).Limit(perPage).Order("created_at DESC").Find(&items).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, items, page, perPage, total)
}

// DELETE /admin/videos/:id/comments/:comment_id
func AdminDeleteVideoComment(ctx iris.Context) {
	vid, _ := ctx.Params().GetUint("id")
	cid, err := ctx.Params().GetUint("comment_id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var c models.VideoComment
	if err := storage.DB.Where("id = ? AND video_id = ?", cid, vid).First(&c).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "comment not found")
		return
	}
	before := c
	if err := storage.DB.Delete(&c).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.Audit(ctx, "video_comment.delete", "video_comment", before.ID, before, nil)
	ctx.StatusCode(http.StatusNoContent)
}
