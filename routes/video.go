package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateVideo stores a new video record after upload is completed (client uploads to CDN)
func CreateVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ CreateVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		PropertyID   uint    `json:"propertyID" validate:"required"`
		VideoURL     string  `json:"videoURL" validate:"required,url"`
		ThumbnailURL string  `json:"thumbnailURL"`
		DurationSec  float64 `json:"durationSec"`
		Caption      string  `json:"caption"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Ensure property exists
	var prop models.Property
	if err := storage.DB.Where("id = ?", input.PropertyID).First(&prop).Error; err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	video := models.Video{
		PropertyID:   &input.PropertyID,
		UserID:       userID,
		VideoURL:     input.VideoURL,
		ThumbnailURL: input.ThumbnailURL,
		DurationSec:  input.DurationSec,
		Caption:      input.Caption,
	}

	if err := storage.DB.Create(&video).Error; err != nil {
		fmt.Printf("Error creating video: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Notify previous viewers of this host's videos about the new video
	go func(videoID uint, hostUserID uint) {
		notifyPreviousViewersOfNewVideo(videoID, hostUserID)
	}(video.ID, userID)

	ctx.JSON(iris.Map{"success": true, "video": video})
}

// GetVideoFeed returns paginated videos with property and user data
func GetVideoFeed(ctx iris.Context) {
	// Identify the viewer (optional authentication)
	var userID uint
	hasAuth := false
	if claims := jsonWT.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			userID = accessToken.ID
			hasAuth = userID > 0
			fmt.Printf("🔍 JWT Token parsed - User ID: %d\n", userID)
		} else {
			fmt.Printf("❌ Failed to parse JWT token as AccessToken\n")
		}
	} else {
		fmt.Printf("❌ No JWT claims found in request\n")
	}

	// Cursor-based pagination (preferred) or fallback to page-based
	cursor := ctx.URLParam("cursor")
	page := ctx.URLParamIntDefault("page", 0) // 0 means use cursor
	limit := ctx.URLParamIntDefault("limit", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	var offset int
	var useCursor bool = cursor != ""
	if !useCursor && page > 0 {
		// Fallback to page-based pagination
		offset = (page - 1) * limit
		useCursor = false
	} else if useCursor {
		// Parse cursor (format: "video_id:timestamp")
		// For now, we'll use video ID as cursor
		useCursor = true
		offset = 0
	} else {
		// Default: first page
		offset = 0
		useCursor = false
	}

	// Filters from query
	city := ctx.URLParam("city")
	propertyType := ctx.URLParam("propertyType")
	minPrice := ctx.URLParamFloat64Default("minPrice", 0)
	maxPrice := ctx.URLParamFloat64Default("maxPrice", 0)
	minBedrooms := ctx.URLParamIntDefault("minBedrooms", 0)
	maxBedrooms := ctx.URLParamIntDefault("maxBedrooms", 0)
	minBathrooms := ctx.URLParamIntDefault("minBathrooms", 0)
	maxBathrooms := ctx.URLParamIntDefault("maxBathrooms", 0)
	sortBy := strings.ToLower(ctx.URLParamDefault("sort", "recent"))
	sortOrder := strings.ToUpper(ctx.URLParamDefault("sortOrder", "DESC"))
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}

	// Base query (non-promotional videos only; promotional handled separately)
	baseQuery := storage.DB.Model(&models.Video{}).
		Where("COALESCE(videos.is_promotional, ?) = ?", false, false).
		Joins("LEFT JOIN properties ON videos.property_id = properties.id").
		Where("properties.id IS NOT NULL AND COALESCE(properties.is_active, ?) = ? AND properties.status IN (?)", true, true, []string{"approved", "live"}).
		Where("(videos.status IS NULL OR LOWER(videos.status) <> ?)", "rejected").
		Where("COALESCE(videos.is_flagged, ?) = ?", false, false).
		Preload("Property").
		Preload("User")

	if hasAuth {
		fmt.Printf("🔍 Applying personalised filters for user %d\n", userID)
		baseQuery = baseQuery.
			Where("videos.id NOT IN (SELECT video_id FROM hidden_videos WHERE user_id = ? AND deleted_at IS NULL)", userID).
			Where("videos.id NOT IN (SELECT video_id FROM video_reports WHERE reporter_id = ? AND deleted_at IS NULL)", userID).
			Where("videos.user_id NOT IN (SELECT flagged_user_id FROM user_flags WHERE flagger_id = ? AND status = 'active')", userID)
	} else {
		fmt.Printf("⚠️ No user authentication - serving public feed\n")
	}

	// Apply property-based filters
	if city != "" {
		baseQuery = baseQuery.Where("properties.city ILIKE ?", "%"+city+"%")
	}
	if propertyType != "" {
		baseQuery = baseQuery.Where("properties.property_type = ?", propertyType)
	}
	if minPrice > 0 {
		baseQuery = baseQuery.Where("properties.price >= ?", minPrice)
	}
	if maxPrice > 0 {
		baseQuery = baseQuery.Where("properties.price <= ?", maxPrice)
	}
	if minBedrooms > 0 {
		baseQuery = baseQuery.Where("properties.bedrooms >= ?", minBedrooms)
	}
	if maxBedrooms > 0 {
		baseQuery = baseQuery.Where("properties.bedrooms <= ?", maxBedrooms)
	}
	if minBathrooms > 0 {
		baseQuery = baseQuery.Where("properties.bathrooms >= ?", minBathrooms)
	}
	if maxBathrooms > 0 {
		baseQuery = baseQuery.Where("properties.bathrooms <= ?", maxBathrooms)
	}

	// Improved rotation: exclude videos seen in last 2 hours for better variety
	// This ensures users see fresh content without being too restrictive
	var excludedVideoIDs []uint
	var recentlySeenMap map[uint]time.Time // Track when each video was last seen
	if hasAuth {
		// Exclude videos seen in the last 2 hours (more reasonable window)
		cutoff := time.Now().Add(-2 * time.Hour)
		if err := storage.DB.Model(&models.VideoFeedHistory{}).
			Where("user_id = ? AND last_delivered_at >= ?", userID, cutoff).
			Limit(200). // Track recent history
			Pluck("video_id", &excludedVideoIDs).Error; err == nil {
			fmt.Printf("🔄 Excluding %d recently seen videos (last 2h)\n", len(excludedVideoIDs))
		}

		// Also get the last seen timestamps for smarter ordering
		var recentHistories []models.VideoFeedHistory
		if err := storage.DB.Model(&models.VideoFeedHistory{}).
			Where("user_id = ?", userID).
			Select("video_id, last_delivered_at").
			Order("last_delivered_at DESC").
			Limit(500).
			Find(&recentHistories).Error; err == nil {
			recentlySeenMap = make(map[uint]time.Time)
			for _, h := range recentHistories {
				recentlySeenMap[h.VideoID] = h.LastDeliveredAt
			}
			fmt.Printf("📊 Tracking %d videos in user history\n", len(recentlySeenMap))
		}
	}

	// Apply exclusion to base query
	if len(excludedVideoIDs) > 0 {
		baseQuery = baseQuery.Where("videos.id NOT IN ?", excludedVideoIDs)
	}

	// Build order clause based on sort parameter
	var orderClause string
	switch sortBy {
	case "most_liked":
		orderClause = "videos.likes_count " + sortOrder + ", videos.created_at DESC"
	case "most_commented":
		orderClause = "videos.comments_count " + sortOrder + ", videos.created_at DESC"
	case "most_viewed":
		orderClause = "videos.view_count " + sortOrder + ", videos.created_at DESC"
	case "most_saved":
		orderClause = "videos.saves_count " + sortOrder + ", videos.created_at DESC"
	case "price_low":
		orderClause = "properties.nightly_price ASC, videos.created_at DESC"
	case "price_high":
		orderClause = "properties.nightly_price DESC, videos.created_at DESC"
	case "rating":
		orderClause = "properties.rating " + sortOrder + ", videos.created_at DESC"
	case "recent":
		orderClause = "videos.created_at " + sortOrder
	default:
		// Default: Smart rotation with better variety
		// Mix of:
		// 1. Videos never seen (highest priority)
		// 2. Videos seen long ago (medium priority)
		// 3. Recent and trending videos (lower priority if recently seen)
		// 4. Randomization to prevent same order every time
		if hasAuth {
			// Prioritize unseen videos first, then by engagement, with randomization
			// Use RANDOM() with a seed based on current hour to get different results each hour
			// This ensures variety while still prioritizing unseen content
			orderClause = fmt.Sprintf(`CASE 
				WHEN videos.id NOT IN (SELECT video_id FROM video_feed_histories WHERE user_id = %d AND deleted_at IS NULL) 
				THEN 0 
				ELSE 1 
			END ASC,
			RANDOM(),
			(videos.likes_count * 2 + videos.comments_count * 3 + videos.saves_count + videos.view_count) DESC, 
			videos.created_at DESC`, userID)
		} else {
			// Default: mix of recent and trending with randomization
			orderClause = "RANDOM(), (videos.likes_count * 2 + videos.comments_count * 3 + videos.saves_count + videos.view_count) DESC, videos.created_at DESC"
		}
	}

	// Fetch videos with proper preloading
	var selectedVideos []models.Video
	query := baseQuery.Order(orderClause)

	var nextCursor string
	var hasMore bool

	if useCursor && cursor != "" {
		// Cursor-based: fetch videos after the cursor
		var cursorID uint
		if _, err := fmt.Sscanf(cursor, "%d", &cursorID); err == nil {
			query = query.Where("videos.id > ?", cursorID)
		}
		query = query.Limit(limit + 1) // Fetch one extra to determine hasMore
	} else {
		// Page-based pagination (fallback)
		query = query.Limit(limit).Offset(offset)
	}

	if err := query.Find(&selectedVideos).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Determine next cursor and hasMore for cursor-based pagination
	if useCursor {
		if len(selectedVideos) > limit {
			hasMore = true
			selectedVideos = selectedVideos[:limit]
			if len(selectedVideos) > 0 {
				nextCursor = fmt.Sprintf("%d", selectedVideos[len(selectedVideos)-1].ID)
			}
		} else {
			hasMore = false
			nextCursor = ""
		}
	} else {
		// For page-based, check if there are more videos
		if len(selectedVideos) == limit {
			var count int64
			baseQuery.Count(&count)
			hasMore = int64(offset+limit) < count
		} else {
			hasMore = false
		}
		nextCursor = ""
	}

	// Promotional videos (admin content) - intersperse every 4-5 videos
	var promotionalVideos []models.Video
	promoQuery := storage.DB.Model(&models.Video{}).
		Where("is_promotional = ?", true).
		Where("(status IS NULL OR LOWER(status) <> ?)", "rejected").
		Where("COALESCE(is_flagged, ?) = ?", false, false).
		Preload("Property").
		Preload("User").
		Order("created_at DESC").
		Limit(5)

	if hasAuth {
		if len(excludedVideoIDs) > 0 {
			promoQuery = promoQuery.Where("id NOT IN ?", excludedVideoIDs)
		}
		promoQuery = promoQuery.
			Where("id NOT IN (SELECT video_id FROM hidden_videos WHERE user_id = ? AND deleted_at IS NULL)", userID).
			Where("id NOT IN (SELECT video_id FROM video_reports WHERE reporter_id = ? AND deleted_at IS NULL)", userID)
	}

	if err := promoQuery.Find(&promotionalVideos).Error; err == nil && len(promotionalVideos) > 0 {
		fmt.Printf("🎬 Found %d promotional videos\n", len(promotionalVideos))
	}

	// Intersperse promotional videos
	finalVideos := make([]models.Video, 0, limit+len(promotionalVideos))
	if len(promotionalVideos) > 0 && len(selectedVideos) > 0 {
		insertInterval := 4
		promoIndex := 0
		for idx, video := range selectedVideos {
			if len(finalVideos) >= limit {
				break
			}
			finalVideos = append(finalVideos, video)
			if (idx+1)%insertInterval == 0 && promoIndex < len(promotionalVideos) && len(finalVideos) < limit {
				finalVideos = append(finalVideos, promotionalVideos[promoIndex])
				promoIndex++
			}
		}
	} else {
		finalVideos = selectedVideos
	}

	if len(finalVideos) > limit {
		finalVideos = finalVideos[:limit]
	}

	fmt.Printf("📹 Returning %d videos for user %d (page %d)\n", len(finalVideos), userID, page)

	// Update feed history asynchronously (don't block response)
	if hasAuth && len(finalVideos) > 0 {
		go func(videos []models.Video, uid uint) {
			now := time.Now()
			histories := make([]models.VideoFeedHistory, 0, len(videos))
			for _, video := range videos {
				histories = append(histories, models.VideoFeedHistory{
					UserID:          uid,
					VideoID:         video.ID,
					LastDeliveredAt: now,
					SeenCount:       1,
				})
			}
			if err := storage.DB.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "video_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"last_delivered_at": now,
					"seen_count":        gorm.Expr("video_feed_histories.seen_count + 1"),
					"updated_at":        now,
				}),
			}).Create(&histories).Error; err != nil {
				fmt.Printf("⚠️ Failed to upsert video feed history: %v\n", err)
			}
		}(finalVideos, userID)
	}

	// Build liked/saved lookup
	likedMap := make(map[uint]bool)
	savedMap := make(map[uint]bool)
	if hasAuth {
		videoIDs := make([]uint, 0, len(finalVideos))
		for _, video := range finalVideos {
			videoIDs = append(videoIDs, video.ID)
		}
		if len(videoIDs) > 0 {
			var likedIDs []uint
			if err := storage.DB.Model(&models.VideoLike{}).
				Where("video_id IN ? AND user_id = ?", videoIDs, userID).
				Pluck("video_id", &likedIDs).Error; err == nil {
				for _, id := range likedIDs {
					likedMap[id] = true
				}
			}
			var savedIDs []uint
			if err := storage.DB.Model(&models.VideoSave{}).
				Where("video_id IN ? AND user_id = ?", videoIDs, userID).
				Pluck("video_id", &savedIDs).Error; err == nil {
				for _, id := range savedIDs {
					savedMap[id] = true
				}
			}
		}
	}

	// Serialize response with user state
	type VideoWithUserState struct {
		models.Video
		IsLiked bool `json:"isLiked"`
		IsSaved bool `json:"isSaved"`
	}

	videosWithState := make([]VideoWithUserState, 0, len(finalVideos))
	for _, video := range finalVideos {
		videosWithState = append(videosWithState, VideoWithUserState{
			Video:   video,
			IsLiked: likedMap[video.ID],
			IsSaved: savedMap[video.ID],
		})
	}

	response := iris.Map{
		"success": true,
		"videos":  videosWithState,
	}

	// Include cursor information for cursor-based pagination
	if useCursor {
		response["nextCursor"] = nextCursor
		response["hasMore"] = hasMore
	} else {
		// Include page for backward compatibility
		response["page"] = page
		response["hasMore"] = hasMore
	}

	ctx.JSON(response)
}

func LikeVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ LikeVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		VideoID uint `json:"videoID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	like := models.VideoLike{VideoID: input.VideoID, UserID: userID}
	if err := storage.DB.Where(&like).FirstOrCreate(&like).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}
	storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("likes_count", gorm.Expr("likes_count + ?", 1))

	// Send push notification to video owner
	var video models.Video
	if err := storage.DB.First(&video, input.VideoID).Error; err == nil {
		var user models.User
		if err := storage.DB.First(&user, userID).Error; err == nil {
			userName := fmt.Sprintf("%s %s", user.FirstName, user.LastName)
			notificationService := services.NewNotificationService()
			go notificationService.SendVideoInteractionNotificationToHost(
				video.UserID,
				userID,
				userName,
				"like",
				video.Caption,
			)
		}
	}

	// Get updated likes count
	var updatedVideo models.Video
	if err := storage.DB.First(&updatedVideo, input.VideoID).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "likesCount": updatedVideo.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true})
	}
}

func UnlikeVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnlikeVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		VideoID uint `json:"videoID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	storage.DB.Where("video_id = ? AND user_id = ?", input.VideoID, userID).Delete(&models.VideoLike{})
	storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)"))

	// Get updated likes count
	var video models.Video
	if err := storage.DB.First(&video, input.VideoID).Error; err == nil {
		log.Printf("✅ UnlikeVideo: User %d unliked video %d, new count: %d", userID, input.VideoID, video.LikesCount)
		ctx.JSON(iris.Map{"success": true, "likesCount": video.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true})
	}
}

func SaveVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ SaveVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		VideoID uint `json:"videoID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	save := models.VideoSave{VideoID: input.VideoID, UserID: userID}
	if err := storage.DB.Where(&save).FirstOrCreate(&save).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}
	storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("saves_count", gorm.Expr("saves_count + ?", 1))

	// Get updated saves count
	var video models.Video
	if err := storage.DB.First(&video, input.VideoID).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "savesCount": video.SavesCount})
	} else {
		ctx.JSON(iris.Map{"success": true})
	}
}

func UnsaveVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnsaveVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		VideoID uint `json:"videoID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	storage.DB.Where("video_id = ? AND user_id = ?", input.VideoID, userID).Delete(&models.VideoSave{})
	storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("saves_count", gorm.Expr("GREATEST(saves_count - 1, 0)"))

	// Get updated saves count
	var video models.Video
	if err := storage.DB.First(&video, input.VideoID).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "savesCount": video.SavesCount})
	} else {
		ctx.JSON(iris.Map{"success": true})
	}
}

func CreateVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ CreateVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		VideoID  uint   `json:"videoID" validate:"required"`
		Content  string `json:"content" validate:"required"`
		ParentID *uint  `json:"parentID"` // For replies
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	comment := models.VideoComment{
		VideoID:  input.VideoID,
		UserID:   userID,
		Content:  input.Content,
		ParentID: input.ParentID,
	}
	if err := storage.DB.Create(&comment).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Update comment count (only for top-level comments, not replies)
	if input.ParentID == nil {
		storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("comments_count", gorm.Expr("comments_count + ?", 1))
	}

	// Load the comment with user data
	storage.DB.Preload("User").First(&comment, comment.ID)

	// Send push notification to video owner
	var video models.Video
	if err := storage.DB.First(&video, input.VideoID).Error; err == nil {
		var user models.User
		if err := storage.DB.First(&user, userID).Error; err == nil {
			userName := fmt.Sprintf("%s %s", user.FirstName, user.LastName)
			notificationService := services.NewNotificationService()
			go notificationService.SendVideoInteractionNotificationToHost(
				video.UserID,
				userID,
				userName,
				"comment",
				video.Caption,
			)
		}
	}

	ctx.JSON(iris.Map{"success": true, "comment": comment})
}

func GetVideoComments(ctx iris.Context) {
	// Safely get user ID if authenticated (optional)
	var userID uint = 0
	if claims := jsonWT.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			userID = accessToken.ID
		}
	}

	videoID := ctx.Params().Get("videoID")

	var comments []models.VideoComment
	err := storage.DB.Where("video_id = ? AND parent_id IS NULL", videoID).
		Preload("User").
		Preload("Replies.User").
		Order("posted_at DESC").
		Find(&comments).Error
	if err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Get user's liked comment IDs (only if authenticated)
	var likedMap map[uint]bool = make(map[uint]bool)
	if userID > 0 {
		var commentIDs []uint
		for _, comment := range comments {
			commentIDs = append(commentIDs, comment.ID)
			for _, reply := range comment.Replies {
				commentIDs = append(commentIDs, reply.ID)
			}
		}

		var likedCommentIDs []uint
		if len(commentIDs) > 0 {
			storage.DB.Model(&models.VideoCommentLike{}).Where("comment_id IN ? AND user_id = ?", commentIDs, userID).Pluck("comment_id", &likedCommentIDs)
		}

		for _, id := range likedCommentIDs {
			likedMap[id] = true
		}
	}

	// Add isLiked to comments
	type CommentWithUserState struct {
		models.VideoComment
		IsLiked bool                   `json:"isLiked"`
		Replies []CommentWithUserState `json:"replies"`
	}

	var commentsWithState []CommentWithUserState
	for _, comment := range comments {
		// Create replies with IsLiked
		var repliesWithState []CommentWithUserState
		for _, reply := range comment.Replies {
			repliesWithState = append(repliesWithState, CommentWithUserState{
				VideoComment: reply,
				IsLiked:      likedMap[reply.ID],
			})
		}

		commentWithState := CommentWithUserState{
			VideoComment: comment,
			IsLiked:      likedMap[comment.ID],
		}
		commentWithState.Replies = repliesWithState
		commentsWithState = append(commentsWithState, commentWithState)
	}

	ctx.JSON(iris.Map{"success": true, "comments": commentsWithState})
}

func UpdateVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UpdateVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id := ctx.Params().Get("id")

	var input struct {
		Content string `json:"content" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var comment models.VideoComment
	if err := storage.DB.Where("id = ? AND user_id = ?", id, userID).First(&comment).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Comment not found"})
		return
	}

	comment.Content = input.Content
	comment.Edited = true
	if err := storage.DB.Save(&comment).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	storage.DB.Preload("User").First(&comment, comment.ID)
	ctx.JSON(iris.Map{"success": true, "comment": comment})
}

func DeleteVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ DeleteVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id := ctx.Params().Get("id")

	var comment models.VideoComment
	if err := storage.DB.Where("id = ? AND user_id = ?", id, userID).First(&comment).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Comment not found"})
		return
	}

	// Delete replies first
	storage.DB.Where("parent_id = ?", comment.ID).Delete(&models.VideoComment{})

	// Delete comment likes
	storage.DB.Where("comment_id = ?", comment.ID).Delete(&models.VideoCommentLike{})

	// Delete the comment
	storage.DB.Delete(&comment)

	// Update comment count (only for top-level comments)
	if comment.ParentID == nil {
		storage.DB.Model(&models.Video{}).Where("id = ?", comment.VideoID).UpdateColumn("comments_count", gorm.Expr("GREATEST(comments_count - 1, 0)"))
	}

	ctx.JSON(iris.Map{"success": true})
}

func LikeVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ LikeVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		CommentID uint `json:"commentID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	like := models.VideoCommentLike{CommentID: input.CommentID, UserID: userID}
	if err := storage.DB.Where(&like).FirstOrCreate(&like).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}
	storage.DB.Model(&models.VideoComment{}).Where("id = ?", input.CommentID).UpdateColumn("likes_count", gorm.Expr("likes_count + ?", 1))

	// Get updated likes count
	var comment models.VideoComment
	if err := storage.DB.First(&comment, input.CommentID).Error; err == nil {
		log.Printf("✅ LikeVideoComment: User %d liked comment %d, new count: %d", userID, input.CommentID, comment.LikesCount)
		ctx.JSON(iris.Map{"success": true, "likesCount": comment.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true})
	}
}

func UnlikeVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnlikeVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		CommentID uint `json:"commentID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	storage.DB.Where("comment_id = ? AND user_id = ?", input.CommentID, userID).Delete(&models.VideoCommentLike{})
	storage.DB.Model(&models.VideoComment{}).Where("id = ?", input.CommentID).UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)"))

	// Get updated likes count
	var comment models.VideoComment
	if err := storage.DB.First(&comment, input.CommentID).Error; err == nil {
		log.Printf("✅ UnlikeVideoComment: User %d unliked comment %d, new count: %d", userID, input.CommentID, comment.LikesCount)
		ctx.JSON(iris.Map{"success": true, "likesCount": comment.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true})
	}
}

// DeleteVideo deletes a video owned by the requester
func DeleteVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ DeleteVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id := ctx.Params().Get("id")

	// Ensure ownership
	res := storage.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Video{})
	if res.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}
	if res.RowsAffected == 0 {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}
	ctx.JSON(iris.Map{"success": true})
}

// GetLikedVideos returns videos liked by the authenticated user
func GetLikedVideos(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetLikedVideos: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var videos []models.Video
	err := storage.DB.
		Joins("JOIN video_likes vl ON vl.video_id = videos.id AND vl.user_id = ?", userID).
		Preload("Property").Preload("User").
		Order("videos.created_at DESC").
		Find(&videos).Error
	if err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}
	ctx.JSON(iris.Map{"success": true, "videos": videos})
}

// GetSavedVideos returns videos saved by the authenticated user
func GetSavedVideos(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetSavedVideos: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var videos []models.Video
	err := storage.DB.
		Joins("JOIN video_saves vs ON vs.video_id = videos.id AND vs.user_id = ?", userID).
		Preload("Property").Preload("User").
		Order("videos.created_at DESC").
		Find(&videos).Error
	if err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}
	ctx.JSON(iris.Map{"success": true, "videos": videos})
}

// RecordVideoView records a view for a video (supports both authenticated and anonymous users)
func RecordVideoView(ctx iris.Context) {
	videoID := ctx.Params().GetUintDefault("videoID", 0)
	if videoID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Get user ID if authenticated (optional)
	var userID *uint
	var deviceID *string

	// Try to get userID from context values (set by optionalAuthMiddleware)
	if uidValue := ctx.Values().Get("userID"); uidValue != nil {
		if uid, ok := uidValue.(uint); ok && uid > 0 {
			userID = &uid
			fmt.Printf("🔍 RecordVideoView: Found userID %d from context values\n", uid)
		}
	}

	// Fallback: Try to get from JWT claims (for compatibility)
	if userID == nil {
		if claims := jsonWT.Get(ctx); claims != nil {
			if accessToken, ok := claims.(*utils.AccessToken); ok && accessToken.ID > 0 {
				uid := accessToken.ID
				userID = &uid
				fmt.Printf("🔍 RecordVideoView: Found userID %d from JWT claims\n", uid)
			}
		}
	}

	if userID == nil {
		fmt.Printf("🔍 RecordVideoView: No userID found - recording as anonymous view\n")
	}

	// Get device ID from request body (for anonymous users)
	var input struct {
		DeviceID *string `json:"deviceID"`
	}
	ctx.ReadJSON(&input)
	if input.DeviceID != nil && *input.DeviceID != "" {
		deviceID = input.DeviceID
	}

	// Get IP address
	ipAddress := ctx.RemoteAddr()
	if forwardedFor := ctx.GetHeader("X-Forwarded-For"); forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			ipAddress = strings.TrimSpace(ips[0])
		}
	}

	// Check if video exists
	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}

	// Check for duplicate view (same user/device within last 5 minutes)
	cutoff := time.Now().Add(-5 * time.Minute)
	var existingView models.VideoView
	query := storage.DB.Where("video_id = ? AND viewed_at >= ?", videoID, cutoff)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	} else if deviceID != nil {
		query = query.Where("device_id = ?", *deviceID)
	} else {
		// No identifier, allow view but don't track duplicates
		ctx.JSON(iris.Map{"success": true, "message": "View recorded (no identifier)"})
		return
	}

	if err := query.First(&existingView).Error; err == nil {
		// View already recorded recently, just update timestamp
		existingView.ViewedAt = time.Now()
		storage.DB.Save(&existingView)
		ctx.JSON(iris.Map{"success": true, "message": "View updated"})
		return
	}

	// Create new view record
	view := models.VideoView{
		VideoID:   videoID,
		UserID:    userID,
		DeviceID:  deviceID,
		IPAddress: ipAddress,
		ViewedAt:  time.Now(),
	}

	if err := storage.DB.Create(&view).Error; err != nil {
		fmt.Printf("Error recording video view: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Increment video view count
	storage.DB.Model(&video).UpdateColumn("view_count", gorm.Expr("view_count + 1"))

	ctx.JSON(iris.Map{"success": true, "message": "View recorded"})
}

// GetVideoViewers returns view analytics for a video (for host dashboard)
func GetVideoViewers(ctx iris.Context) {
	videoID := ctx.Params().GetUintDefault("videoID", 0)
	fmt.Printf("🔍 GetVideoViewers called for videoID: %d\n", videoID)

	if videoID == 0 {
		fmt.Printf("❌ Invalid video ID: %d\n", videoID)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Verify user owns the video - Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetVideoViewers: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}
	fmt.Printf("🔍 Requesting user ID: %d\n", userID)

	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		fmt.Printf("❌ Video not found: %v\n", err)
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}

	fmt.Printf("🔍 Video found - Owner ID: %d, Requesting User ID: %d\n", video.UserID, userID)

	if video.UserID != userID {
		fmt.Printf("❌ Unauthorized: Video owner is %d, but requesting user is %d\n", video.UserID, userID)
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Not authorized"})
		return
	}

	// Get all views for this video
	var views []models.VideoView
	if err := storage.DB.Where("video_id = ?", videoID).
		Order("viewed_at DESC").
		Find(&views).Error; err != nil {
		fmt.Printf("❌ Error fetching video views: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	fmt.Printf("📊 Found %d total views for video %d\n", len(views), videoID)

	// Aggregate views by user_id and device_id
	viewerMap := make(map[string]*models.VideoViewer)

	for _, view := range views {
		// Create a unique key for each viewer (user_id or device_id)
		var key string
		if view.UserID != nil {
			key = fmt.Sprintf("user_%d", *view.UserID)
		} else if view.DeviceID != nil {
			key = fmt.Sprintf("device_%s", *view.DeviceID)
		} else {
			// Skip views without user_id or device_id
			continue
		}

		viewer, exists := viewerMap[key]
		if !exists {
			viewer = &models.VideoViewer{
				UserID:        view.UserID,
				DeviceID:      view.DeviceID,
				ViewCount:     0,
				FirstViewedAt: view.ViewedAt,
				LastViewedAt:  view.ViewedAt,
			}
			viewerMap[key] = viewer
		}

		viewer.ViewCount++
		if view.ViewedAt.Before(viewer.FirstViewedAt) {
			viewer.FirstViewedAt = view.ViewedAt
		}
		if view.ViewedAt.After(viewer.LastViewedAt) {
			viewer.LastViewedAt = view.ViewedAt
		}
	}

	// Convert map to slice and load user data
	viewers := make([]models.VideoViewer, 0, len(viewerMap))
	for _, viewer := range viewerMap {
		// Load user data if authenticated
		if viewer.UserID != nil {
			var user models.User
			if err := storage.DB.First(&user, *viewer.UserID).Error; err == nil {
				viewer.User = &user
			} else {
				fmt.Printf("⚠️ Could not load user %d: %v\n", *viewer.UserID, err)
			}
		}
		viewers = append(viewers, *viewer)
	}

	// Sort by view count (descending), then by last viewed (descending)
	for i := 0; i < len(viewers)-1; i++ {
		for j := i + 1; j < len(viewers); j++ {
			if viewers[i].ViewCount < viewers[j].ViewCount ||
				(viewers[i].ViewCount == viewers[j].ViewCount && viewers[i].LastViewedAt.Before(viewers[j].LastViewedAt)) {
				viewers[i], viewers[j] = viewers[j], viewers[i]
			}
		}
	}

	// Limit to 100 viewers
	if len(viewers) > 100 {
		viewers = viewers[:100]
	}

	fmt.Printf("✅ Returning %d aggregated viewers for video %d\n", len(viewers), videoID)

	ctx.JSON(iris.Map{
		"success":    true,
		"viewers":    viewers,
		"totalViews": video.ViewCount,
	})
}

// notifyPreviousViewersOfNewVideo notifies users who viewed this host's previous videos
func notifyPreviousViewersOfNewVideo(newVideoID uint, hostUserID uint) {
	// Get the new video with property info
	var newVideo models.Video
	if err := storage.DB.Preload("Property").Preload("User").First(&newVideo, newVideoID).Error; err != nil {
		fmt.Printf("❌ Failed to load new video %d: %v\n", newVideoID, err)
		return
	}

	// Get all unique viewers of this host's previous videos (excluding the new video)
	var viewerIDs []uint
	var deviceIDs []string

	// Get authenticated viewers
	if err := storage.DB.Model(&models.VideoView{}).
		Joins("JOIN videos ON video_views.video_id = videos.id").
		Where("videos.user_id = ? AND video_views.video_id != ? AND video_views.user_id IS NOT NULL", hostUserID, newVideoID).
		Distinct("video_views.user_id").
		Pluck("video_views.user_id", &viewerIDs).Error; err != nil {
		fmt.Printf("⚠️ Error fetching authenticated viewers: %v\n", err)
	}

	// Get anonymous viewers by device ID
	if err := storage.DB.Model(&models.VideoView{}).
		Joins("JOIN videos ON video_views.video_id = videos.id").
		Where("videos.user_id = ? AND video_views.video_id != ? AND video_views.device_id IS NOT NULL", hostUserID, newVideoID).
		Distinct("video_views.device_id").
		Pluck("video_views.device_id", &deviceIDs).Error; err != nil {
		fmt.Printf("⚠️ Error fetching anonymous viewers: %v\n", err)
	}

	fmt.Printf("📢 Found %d authenticated viewers and %d anonymous viewers for host %d\n", len(viewerIDs), len(deviceIDs), hostUserID)

	// Get property info for notification
	propertyName := "a property"
	propertyZone := ""
	if newVideo.Property != nil {
		propertyName = newVideo.Property.Title
		propertyZone = newVideo.Property.City
	}

	hostName := "Host"
	if newVideo.User.ID > 0 {
		hostName = fmt.Sprintf("%s %s", newVideo.User.FirstName, newVideo.User.LastName)
	}

	// Send notifications to authenticated viewers
	for _, viewerID := range viewerIDs {
		// Skip if viewer is the host
		if viewerID == hostUserID {
			continue
		}

		// Get viewer's push tokens and language preference
		var viewer models.User
		if err := storage.DB.First(&viewer, viewerID).Error; err != nil {
			continue
		}

		if viewer.AllowsNotifications == nil || !*viewer.AllowsNotifications {
			continue
		}

		var tokens []string
		if viewer.PushTokens != nil {
			json.Unmarshal(viewer.PushTokens, &tokens)
		}

		if len(tokens) == 0 {
			continue
		}

		// Get viewer's language preference from NotificationPreference table
		// Default to 'en' if not found
		viewerLanguage := "en"
		var notificationPref models.NotificationPreference
		if err := storage.DB.Where("user_id = ?", viewerID).First(&notificationPref).Error; err == nil {
			if notificationPref.Language != "" {
				viewerLanguage = notificationPref.Language
			}
		}

		// Format notification message in viewer's language
		// Format: "{user name} check my new video on {property name} in {property zone}"
		title, body := getVideoNotificationText(viewerLanguage, hostName, propertyName, propertyZone)

		// Send notification with thumbnail
		thumbnailURL := newVideo.ThumbnailURL
		if thumbnailURL == "" && newVideo.Property != nil && newVideo.Property.Images != "" {
			// Images is stored as JSON string, need to unmarshal
			var images []string
			if err := json.Unmarshal([]byte(newVideo.Property.Images), &images); err == nil && len(images) > 0 {
				thumbnailURL = images[0]
			}
		}

		// Use push service directly
		if err := push.SendPushWithImage(tokens, title, body, thumbnailURL); err != nil {
			fmt.Printf("⚠️ Failed to send notification to user %d: %v\n", viewerID, err)
		} else {
			fmt.Printf("✅ Sent notification to user %d\n", viewerID)
		}
	}

	// For anonymous viewers, we'd need to store device tokens separately
	// This is a simplified version - in production, you'd want to track device tokens
	fmt.Printf("📱 Note: Anonymous viewers (%d) would need device token tracking for notifications\n", len(deviceIDs))
}

// getVideoNotificationText returns localized notification text for video uploads
// Format: "{user name} check my new video on {property name} in {property zone}"
func getVideoNotificationText(language, hostName, propertyName, propertyZone string) (title, body string) {
	// Default property name and zone if empty (in appropriate language)
	defaultPropertyName := map[string]string{
		"en": "a property",
		"ar": "عقار",
		"fr": "une propriété",
	}
	defaultZone := map[string]string{
		"en": "an unknown zone",
		"ar": "منطقة غير معروفة",
		"fr": "une zone inconnue",
	}

	if propertyName == "" {
		if name, exists := defaultPropertyName[language]; exists {
			propertyName = name
		} else {
			propertyName = defaultPropertyName["en"]
		}
	}
	if propertyZone == "" {
		if zone, exists := defaultZone[language]; exists {
			propertyZone = zone
		} else {
			propertyZone = defaultZone["en"]
		}
	}

	translations := map[string]map[string]string{
		"en": {
			"title": fmt.Sprintf("%s check my new video", hostName),
			"body":  fmt.Sprintf("on %s in %s", propertyName, propertyZone),
		},
		"ar": {
			"title": fmt.Sprintf("%s شاهد فيديو الجديد", hostName),
			"body":  fmt.Sprintf("على %s في %s", propertyName, propertyZone),
		},
		"fr": {
			"title": fmt.Sprintf("%s regardez ma nouvelle vidéo", hostName),
			"body":  fmt.Sprintf("sur %s à %s", propertyName, propertyZone),
		},
	}

	// Get translation for the language, fallback to English
	langTranslations, exists := translations[language]
	if !exists {
		langTranslations = translations["en"]
	}

	title = langTranslations["title"]
	body = langTranslations["body"]

	return title, body
}
