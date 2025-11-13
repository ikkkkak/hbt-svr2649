package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateVideo stores a new video record after upload is completed (client uploads to CDN)
func CreateVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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

	// Pagination parameters
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	offset := (page - 1) * limit

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
	if err := baseQuery.
		Order(orderClause).
		Limit(limit).
		Offset(offset).
		Find(&selectedVideos).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
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

	ctx.JSON(iris.Map{
		"success": true,
		"videos":  videosWithState,
		"page":    page,
	})
}

func LikeVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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

	ctx.JSON(iris.Map{"success": true})
}

func UnlikeVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	var input struct {
		VideoID uint `json:"videoID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	storage.DB.Where("video_id = ? AND user_id = ?", input.VideoID, userID).Delete(&models.VideoLike{})
	storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)"))
	ctx.JSON(iris.Map{"success": true})
}

func SaveVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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
	ctx.JSON(iris.Map{"success": true})
}

func UnsaveVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	var input struct {
		VideoID uint `json:"videoID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	storage.DB.Where("video_id = ? AND user_id = ?", input.VideoID, userID).Delete(&models.VideoSave{})
	storage.DB.Model(&models.Video{}).Where("id = ?", input.VideoID).UpdateColumn("saves_count", gorm.Expr("GREATEST(saves_count - 1, 0)"))
	ctx.JSON(iris.Map{"success": true})
}

func CreateVideoComment(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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
	ctx.JSON(iris.Map{"success": true})
}

func UnlikeVideoComment(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	var input struct {
		CommentID uint `json:"commentID" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	storage.DB.Where("comment_id = ? AND user_id = ?", input.CommentID, userID).Delete(&models.VideoCommentLike{})
	storage.DB.Model(&models.VideoComment{}).Where("id = ?", input.CommentID).UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)"))
	ctx.JSON(iris.Map{"success": true})
}

// DeleteVideo deletes a video owned by the requester
func DeleteVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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
