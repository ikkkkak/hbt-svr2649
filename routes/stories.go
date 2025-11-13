package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// RegisterStoriesRoutes mounts stories endpoints.
// auth middlewares (variadic) are applied only to protected routes (order matters).
func RegisterStoriesRoutes(p iris.Party, auth ...iris.Handler) {
	r := p.Party("/stories")
	// Ensure auth middleware always runs for protected routes in this party
	protected := r.Party("")
	if len(auth) > 0 {
		protected.Use(auth...)
	}
	{
		// Protected
		protected.Post("/upload", UploadStory)
		protected.Post("/{storyId:uint}/view", PostStoryView)
		protected.Post("/{storyId:uint}/like", PostStoryLikeToggle)
		protected.Delete("/{storyId:uint}", DeleteStory)

		// Public (inbox can optionally use auth to show read status)
		r.Get("/inbox", GetStoriesInbox)
		r.Get("/{userId:uint}", GetUserStories)
	}

	// Lightweight TTL cleanup for expired stories
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			if err := storage.DB.
				Where("expires_at <= NOW()").
				Delete(&models.Story{}).Error; err != nil {
				fmt.Printf("stories ttl cleanup error: %v\n", err)
			}
		}
	}()
}

// UploadStory handles multipart upload (stub - integrate with actual uploader/CDN)
func UploadStory(ctx iris.Context) {
	// Robust user extraction (middleware may store int vs uint)
	var uid uint
	if v := ctx.Values().Get("userID"); v != nil {
		switch t := v.(type) {
		case uint:
			uid = t
		case int:
			if t > 0 {
				uid = uint(t)
			}
		case int64:
			if t > 0 {
				uid = uint(t)
			}
		case float64:
			if t > 0 {
				uid = uint(t)
			}
		case string:
			var tmp uint
			if _, err := fmt.Sscanf(t, "%d", &tmp); err == nil && tmp > 0 {
				uid = tmp
			}
		}
	}
	if uid == 0 {
		fmt.Println("[stories] uploadStory: userID resolved to 0 -> 401")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	var body struct {
		Type            string `json:"type"`
		MediaURL        string `json:"media_url"`
		MediaBase64     string `json:"media_base64"`
		ThumbnailBase64 string `json:"thumbnail_base64"`
		ThumbURL        string `json:"thumb_url"`
		Caption         string `json:"caption"`
		Duration        int    `json:"duration_seconds"`
		Mime            string `json:"mime"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		fmt.Printf("[stories] uploadStory: invalid JSON: %v\n", err)
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	mediaURL := body.MediaURL
	thumbURL := body.ThumbURL

	if strings.HasPrefix(body.MediaBase64, "data:") {
		publicID := fmt.Sprintf("stories/%d/%d", uid, time.Now().UnixNano())
		switch strings.ToLower(body.Type) {
		case "video":
			mime := body.Mime
			if mime == "" {
				mime = "video/mp4"
			}
			uploaded := storage.UploadBase64Video(body.MediaBase64, publicID, mime)
			if uploaded["url"] == "" {
				ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "video upload failed"})
				return
			}
			mediaURL = uploaded["url"]
			if strings.HasPrefix(body.ThumbnailBase64, "data:") {
				thumbUploaded := storage.UploadBase64Image(body.ThumbnailBase64, publicID+"-thumb")
				if thumbUploaded["url"] != "" {
					thumbURL = thumbUploaded["url"]
				}
			}
		default:
			uploaded := storage.UploadBase64Image(body.MediaBase64, publicID)
			if uploaded["url"] == "" {
				ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "image upload failed"})
				return
			}
			mediaURL = uploaded["url"]
			if strings.HasPrefix(body.ThumbnailBase64, "data:") && body.ThumbnailBase64 != body.MediaBase64 {
				thumbUploaded := storage.UploadBase64Image(body.ThumbnailBase64, publicID+"-thumb")
				if thumbUploaded["url"] != "" {
					thumbURL = thumbUploaded["url"]
				}
			} else {
				thumbURL = mediaURL
			}
		}
	}

	if mediaURL == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing media"})
		return
	}
	if thumbURL == "" {
		thumbURL = mediaURL
	}

	fmt.Printf("[stories] uploadStory: user=%d type=%s media=%s\n", uid, body.Type, mediaURL)
	story := models.Story{
		UserID:          uid,
		MediaURL:        mediaURL,
		ThumbURL:        thumbURL,
		Type:            models.StoryType(body.Type),
		Caption:         body.Caption,
		DurationSeconds: body.Duration,
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	if err := storage.DB.Create(&story).Error; err != nil {
		fmt.Printf("[stories] uploadStory: db create error: %v\n", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(story)
}

// GetStoriesInbox returns minimal payload of users with recent stories and unseen flag
func GetStoriesInbox(ctx iris.Context) {
	type StoryInboxItem struct {
		UserID      uint   `json:"user_id"`
		Username    string `json:"username"`
		AvatarURL   string `json:"avatar_url"`
		HasUnseen   bool   `json:"has_unseen"`
		StoryCount  int64  `json:"story_count"`
		FirstThumb  string `json:"first_thumb"`
		LastUpdated int64  `json:"last_updated"`
	}

	// Try to get authenticated user ID (optional - for read status)
	var viewerUserID uint
	if v := ctx.Values().Get("userID"); v != nil {
		switch t := v.(type) {
		case uint:
			viewerUserID = t
		case int:
			if t > 0 {
				viewerUserID = uint(t)
			}
		case int64:
			if t > 0 {
				viewerUserID = uint(t)
			}
		case float64:
			if t > 0 {
				viewerUserID = uint(t)
			}
		case string:
			var tmp uint
			if _, err := fmt.Sscanf(t, "%d", &tmp); err == nil && tmp > 0 {
				viewerUserID = tmp
			}
		}
	}

	type row struct {
		UserID     uint
		StoryCount int64
		LastTs     time.Time
		ThumbURL   string
	}
	var rows []row
	// Aggregate active stories by user
	if err := storage.DB.
		Table("stories").
		Select("user_id as user_id, COUNT(*) as story_count, MAX(created_at) as last_ts, MIN(thumb_url) as thumb_url").
		Where("expires_at > NOW()").
		Group("user_id").
		Order("last_ts DESC").
		Find(&rows).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	if len(rows) == 0 {
		ctx.JSON(iris.Map{"inbox": []StoryInboxItem{}})
		return
	}

	// Fetch user meta
	userIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		userIDs = append(userIDs, r.UserID)
	}
	type userMeta struct {
		ID        uint
		FirstName string
		LastName  string
		AvatarURL string
	}
	var metas []userMeta
	if err := storage.DB.
		Table("users").
		Select("id, first_name, last_name, avatar_url").
		Where("id IN ?", userIDs).
		Find(&metas).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	metaMap := map[uint]userMeta{}
	for _, m := range metas {
		metaMap[m.ID] = m
	}

	// Pre-compute viewed counts per story owner (single query) when viewer is authenticated
	viewedCounts := map[uint]int64{}
	if viewerUserID > 0 {
		type viewedRow struct {
			UserID     uint
			ViewedCount int64
		}
		var viewedRows []viewedRow
		if err := storage.DB.
			Table("story_views").
			Select("stories.user_id AS user_id, COUNT(DISTINCT stories.id) AS viewed_count").
			Joins("JOIN stories ON stories.id = story_views.story_id").
			Where("story_views.viewer_user_id = ? AND stories.expires_at > NOW()", viewerUserID).
			Group("stories.user_id").
			Find(&viewedRows).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
		for _, vr := range viewedRows {
			viewedCounts[vr.UserID] = vr.ViewedCount
		}
	}

	items := make([]StoryInboxItem, 0, len(rows))
	for _, r := range rows {
		meta := metaMap[r.UserID]
		username := fmt.Sprintf("%s %s", meta.FirstName, meta.LastName)
		hasUnseen := true
		if viewerUserID > 0 {
			viewedCount := viewedCounts[r.UserID]
			hasUnseen = viewedCount < r.StoryCount
		}

		items = append(items, StoryInboxItem{
			UserID:      r.UserID,
			Username:    username,
			AvatarURL:   meta.AvatarURL,
			HasUnseen:   hasUnseen,
			StoryCount:  r.StoryCount,
			FirstThumb:  r.ThumbURL,
			LastUpdated: r.LastTs.Unix(),
		})
	}

	// Sort: unread first, then read (by last updated)
	sort.Slice(items, func(i, j int) bool {
		if items[i].HasUnseen != items[j].HasUnseen {
			return items[i].HasUnseen // Unseen first
		}
		return items[i].LastUpdated > items[j].LastUpdated // Then by most recent
	})

	ctx.JSON(iris.Map{"inbox": items})
}

// GetUserStories returns all active stories for a user ordered by created_at
func GetUserStories(ctx iris.Context) {
	userId, err := ctx.Params().GetUint("userId")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var stories []models.Story
	if err := storage.DB.
		Where("user_id = ? AND expires_at > NOW()", userId).
		Order("created_at ASC").
		Find(&stories).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"stories": stories})
}

// PostStoryView records a view
func PostStoryView(ctx iris.Context) {
	// Extract authenticated user ID from context
	var viewerUserID uint
	if v := ctx.Values().Get("userID"); v != nil {
		switch t := v.(type) {
		case uint:
			viewerUserID = t
		case int:
			if t > 0 {
				viewerUserID = uint(t)
			}
		case int64:
			if t > 0 {
				viewerUserID = uint(t)
			}
		case float64:
			if t > 0 {
				viewerUserID = uint(t)
			}
		case string:
			var tmp uint
			if _, err := fmt.Sscanf(t, "%d", &tmp); err == nil && tmp > 0 {
				viewerUserID = tmp
			}
		}
	}

	storyId, err := ctx.Params().GetUint("storyId")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	var body struct {
		ViewerUserID *uint   `json:"viewer_user_id"` // Optional fallback
		ViewerPhone  *string `json:"viewer_phone"`
		ViewedSecs   float64 `json:"viewed_seconds"`
		DeviceInfo   *string `json:"device_info"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		// Allow empty body for simple view tracking
	}

	// Use authenticated user ID if available, otherwise use body fallback
	finalViewerUserID := &viewerUserID
	if viewerUserID == 0 && body.ViewerUserID != nil {
		finalViewerUserID = body.ViewerUserID
	}

	// Check if view already exists (avoid duplicates)
	var existingView models.StoryView
	if finalViewerUserID != nil && *finalViewerUserID > 0 {
		err := storage.DB.Where("story_id = ? AND viewer_user_id = ?", storyId, *finalViewerUserID).First(&existingView).Error
		if err == nil {
			// View already exists, update viewed_seconds if provided
			if body.ViewedSecs > 0 {
				storage.DB.Model(&existingView).Update("viewed_seconds", body.ViewedSecs)
			}
			ctx.StatusCode(http.StatusNoContent)
			return
		}
	}

	view := models.StoryView{
		StoryID:       storyId,
		ViewerUserID:  finalViewerUserID,
		ViewerPhone:   body.ViewerPhone,
		ViewedSeconds: body.ViewedSecs,
		DeviceInfo:    body.DeviceInfo,
		ViewedAt:      time.Now(),
	}
	if err := storage.DB.Create(&view).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.StatusCode(http.StatusNoContent)
}

// PostStoryLikeToggle toggles like and updates cached count
func PostStoryLikeToggle(ctx iris.Context) {
	userID := ctx.Values().GetUintDefault("userID", 0)
	if userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	storyId, err := ctx.Params().GetUint("storyId")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var like models.StoryLike
	tx := storage.DB.Where("story_id = ? AND user_id = ?", storyId, userID).First(&like)
	if tx.Error == nil {
		// unlike
		storage.DB.Delete(&like)
		storage.DB.Model(&models.Story{}).Where("id = ?", storyId).UpdateColumn("likes_count", gormExprDecrement(1))
		ctx.JSON(iris.Map{"liked": false})
		return
	}
	// like
	newLike := models.StoryLike{StoryID: storyId, UserID: uint(userID)}
	storage.DB.Create(&newLike)
	storage.DB.Model(&models.Story{}).Where("id = ?", storyId).UpdateColumn("likes_count", gormExprIncrement(1))
	ctx.JSON(iris.Map{"liked": true})
}

// DeleteStory removes a story (owner only)
func DeleteStory(ctx iris.Context) {
	// Extract user ID from context
	var userID uint
	if v := ctx.Values().Get("userID"); v != nil {
		switch t := v.(type) {
		case uint:
			userID = t
		case int:
			if t > 0 {
				userID = uint(t)
			}
		case int64:
			if t > 0 {
				userID = uint(t)
			}
		case float64:
			if t > 0 {
				userID = uint(t)
			}
		case string:
			var tmp uint
			if _, err := fmt.Sscanf(t, "%d", &tmp); err == nil && tmp > 0 {
				userID = tmp
			}
		}
	}
	if userID == 0 {
		fmt.Println("[stories] deleteStory: userID resolved to 0 -> 401")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	// Extract story ID from URL parameter
	storyId, err := ctx.Params().GetUint("storyId")
	if err != nil {
		fmt.Printf("[stories] deleteStory: invalid storyId parameter: %v\n", err)
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// First, check if story exists and belongs to user
	var story models.Story
	if err := storage.DB.Where("id = ?", storyId).First(&story).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			fmt.Printf("[stories] deleteStory: story %d not found -> 404\n", storyId)
			ctx.StopWithStatus(http.StatusNotFound)
			return
		}
		fmt.Printf("[stories] deleteStory: db query error: %v\n", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Verify ownership
	if story.UserID != userID {
		fmt.Printf("[stories] deleteStory: user %d attempted to delete story %d owned by user %d -> 403\n", userID, storyId, story.UserID)
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	// Delete associated likes (no cascade constraint on StoryLike)
	if err := storage.DB.Where("story_id = ?", storyId).Delete(&models.StoryLike{}).Error; err != nil {
		fmt.Printf("[stories] deleteStory: failed to delete likes for story %d: %v\n", storyId, err)
		// Continue with story deletion even if likes deletion fails
	}

	// Delete the story (StoryView will be deleted automatically due to CASCADE constraint)
	result := storage.DB.Where("id = ? AND user_id = ?", storyId, userID).Delete(&models.Story{})
	if result.Error != nil {
		fmt.Printf("[stories] deleteStory: db delete error: %v\n", result.Error)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		// This shouldn't happen after our checks, but handle it anyway
		fmt.Printf("[stories] deleteStory: story %d deletion affected 0 rows\n", storyId)
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	fmt.Printf("[stories] deleteStory: user %d successfully deleted story %d\n", userID, storyId)
	ctx.StatusCode(http.StatusNoContent)
}

// helpers (gorm expressions)
func gormExprIncrement(n int) interface{} {
	return gorm.Expr("COALESCE(likes_count,0) + ?", n)
}
func gormExprDecrement(n int) interface{} {
	return gorm.Expr("GREATEST(COALESCE(likes_count,0) - ?, 0)", n)
}
