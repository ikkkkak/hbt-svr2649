package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
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
	isPromotional := ctx.URLParamDefault("is_promotional", "")
	propertyID := ctx.URLParamDefault("property_id", "")
	uploaderID := ctx.URLParamDefault("uploader_id", "")
	sort := ctx.URLParamDefault("sort", "newest")

	q := storage.DB.Model(&models.Video{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if isPromotional == "true" {
		q = q.Where("COALESCE(is_promotional, false) = ?", true)
	} else if isPromotional == "false" {
		q = q.Where("COALESCE(is_promotional, false) = ?", false)
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
	if err := q.Preload("Property").Preload("User").Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
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
	go func() {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		videoFeedCache := services.NewVideoFeedCacheService(cacheService, cacheConfig)
		_ = videoFeedCache.InvalidateVideoFeed(bgCtx)
	}()
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
	go func() {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		videoFeedCache := services.NewVideoFeedCacheService(cacheService, cacheConfig)
		_ = videoFeedCache.InvalidateVideoFeed(bgCtx)
	}()
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

// POST /admin/videos/promotional - Create promotional video (app demo, tutorial, etc.)
func AdminCreatePromotionalVideo(ctx iris.Context) {
	var input struct {
		VideoURL      string  `json:"videoURL"`
		VideoURLSnake string  `json:"video_url"`
		ThumbnailURL  string  `json:"thumbnailURL"`
		DurationSec   float64 `json:"durationSec"`
		Title         string  `json:"title" validate:"required"`
		Description   string  `json:"description"`
		Caption       string  `json:"caption"`
		PropertyID    *uint   `json:"propertyID"` // Optional - can be null for promotional videos
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}

	videoURL := strings.TrimSpace(input.VideoURL)
	if videoURL == "" {
		videoURL = strings.TrimSpace(input.VideoURLSnake)
	}
	if videoURL == "" {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "videoURL is required")
		return
	}
	parsed, err := url.ParseRequestURI(videoURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "videoURL must be a valid http(s) URL")
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "title is required")
		return
	}
	
	// Read user id from middleware context (set by accessTokenVerifierMiddleware/AdminOnlyMiddleware).
	adminUserID, ok := ctx.Values().Get("userID").(uint)
	if !ok || adminUserID == 0 {
		// Backward-compatible fallback for routes wired with JWT middleware.
		if claims := jsonWT.Get(ctx); claims != nil {
			if accessToken, tokenOK := claims.(*utils.AccessToken); tokenOK && accessToken.ID > 0 {
				adminUserID = accessToken.ID
			}
		}
	}
	if adminUserID == 0 {
		utils.JSONError(ctx, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	
	// If PropertyID is provided, verify it exists
	if input.PropertyID != nil && *input.PropertyID > 0 {
		var prop models.Property
		if err := storage.DB.Where("id = ?", *input.PropertyID).First(&prop).Error; err != nil {
			utils.JSONError(ctx, http.StatusBadRequest, "property_not_found", "property not found")
			return
		}
	}
	
	// Create promotional video
	// For promotional videos, PropertyID should be nil (not set)
	var propertyID *uint = nil
	if input.PropertyID != nil && *input.PropertyID > 0 {
		propertyID = input.PropertyID
	}
	
	video := models.Video{
		UserID:        adminUserID,
		PropertyID:    propertyID, // nil for promotional videos
		VideoURL:      videoURL,
		ThumbnailURL:  input.ThumbnailURL,
		DurationSec:   input.DurationSec,
		Caption:       input.Caption,
		IsPromotional: true,
		Title:         input.Title,
		Description:   input.Description,
		Status:        "approved", // Auto-approve promotional videos
	}
	
	if err := storage.DB.Create(&video).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	
	// Load relationships
	storage.DB.Preload("Property").Preload("User").First(&video, video.ID)
	
	utils.Audit(ctx, "promotional_video.create", "video", video.ID, nil, video)
	ctx.JSON(iris.Map{"success": true, "data": video})
}

// GET /admin/videos/promotional - List all promotional videos
func AdminListPromotionalVideos(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}
	
	q := storage.DB.Model(&models.Video{}).
		Where("is_promotional = ?", true).
		Order("created_at DESC")
	
	var total int64
	q.Count(&total)
	var items []models.Video
	if err := q.Preload("Property").Preload("User").Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, items, page, perPage, total)
}

// PATCH /admin/videos/promotional/:id - Update promotional video
func AdminUpdatePromotionalVideo(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	
	var v models.Video
	if err := storage.DB.First(&v, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "video not found")
		return
	}
	
	if !v.IsPromotional {
		utils.JSONError(ctx, http.StatusBadRequest, "not_promotional", "video is not promotional")
		return
	}
	
	var body struct {
		VideoURL      *string `json:"videoURL"`
		VideoURLSnake *string `json:"video_url"`
		ThumbnailURL  *string `json:"thumbnailURL"`
		Title         *string `json:"title"`
		Description   *string `json:"description"`
		Caption       *string `json:"caption"`
		Status        *string `json:"status"`
	}

	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", err.Error())
		return
	}

	before := v
	nextURL := ""
	if body.VideoURL != nil && strings.TrimSpace(*body.VideoURL) != "" {
		nextURL = strings.TrimSpace(*body.VideoURL)
	} else if body.VideoURLSnake != nil && strings.TrimSpace(*body.VideoURLSnake) != "" {
		nextURL = strings.TrimSpace(*body.VideoURLSnake)
	}
	if nextURL != "" {
		parsed, err := url.ParseRequestURI(nextURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "videoURL must be a valid http(s) URL")
			return
		}
		v.VideoURL = nextURL
	}
	if body.ThumbnailURL != nil {
		v.ThumbnailURL = *body.ThumbnailURL
	}
	if body.Title != nil {
		v.Title = *body.Title
	}
	if body.Description != nil {
		v.Description = *body.Description
	}
	if body.Caption != nil {
		v.Caption = *body.Caption
	}
	if body.Status != nil {
		v.Status = *body.Status
	}
	
	if err := storage.DB.Save(&v).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	go func() {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		videoFeedCache := services.NewVideoFeedCacheService(cacheService, cacheConfig)
		_ = videoFeedCache.InvalidateVideoFeed(bgCtx)
	}()

	utils.Audit(ctx, "promotional_video.update", "video", v.ID, before, v)
	ctx.JSON(iris.Map{"success": true, "data": v})
}

// DELETE /admin/videos/promotional/:id - Delete promotional video
func AdminDeletePromotionalVideo(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	
	var v models.Video
	if err := storage.DB.First(&v, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "video not found")
		return
	}
	
	if !v.IsPromotional {
		utils.JSONError(ctx, http.StatusBadRequest, "not_promotional", "video is not promotional")
		return
	}
	
	before := v
	if err := storage.DB.Delete(&v).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	go func() {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		videoFeedCache := services.NewVideoFeedCacheService(cacheService, cacheConfig)
		_ = videoFeedCache.InvalidateVideoFeed(bgCtx)
	}()

	utils.Audit(ctx, "promotional_video.delete", "video", before.ID, before, nil)
	ctx.StatusCode(http.StatusNoContent)
}
