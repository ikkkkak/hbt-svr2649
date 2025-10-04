package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
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
		PropertyID:   input.PropertyID,
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
	// Get user ID if authenticated, otherwise use 0 for public access
	var userID uint = 0
	if claims := jsonWT.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			userID = accessToken.ID
		}
	}

	// simple pagination
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	offset := (page - 1) * limit

	// Enhanced filtering parameters for TikTok-quality experience
	city := ctx.URLParam("city")
	propertyType := ctx.URLParam("propertyType")
	minPrice := ctx.URLParamFloat64Default("minPrice", 0)
	maxPrice := ctx.URLParamFloat64Default("maxPrice", 0)
	minBedrooms := ctx.URLParamIntDefault("minBedrooms", 0)
	maxBedrooms := ctx.URLParamIntDefault("maxBedrooms", 0)
	minBathrooms := ctx.URLParamIntDefault("minBathrooms", 0)
	maxBathrooms := ctx.URLParamIntDefault("maxBathrooms", 0)

	// TikTok-style sorting options
	sortBy := ctx.URLParamDefault("sort", "recent")       // recent, most_liked, most_commented, most_viewed, most_saved, price_low, price_high, rating
	sortOrder := ctx.URLParamDefault("sortOrder", "DESC") // ASC, DESC

	// Build query with filters: only videos for approved/live & active properties; exclude flagged/rejected videos
	query := storage.DB.
		Joins("JOIN properties ON videos.property_id = properties.id").
		Where("COALESCE(properties.is_active, ?) = ? AND properties.status IN (?)", true, true, []string{"approved", "live"}).
		// Exclude only explicitly rejected videos; allow pending/empty/approved
		Where("(videos.status IS NULL OR LOWER(videos.status) <> ?)", "rejected").
		Where("COALESCE(videos.is_flagged, ?) = ?", false, false).
		Select("videos.*").
		Preload("Property").Preload("User")

	// Apply property filters
	if city != "" {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.city ILIKE ?", "%"+city+"%")
	}
	if propertyType != "" {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.property_type = ?", propertyType)
	}
	if minPrice > 0 {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.price >= ?", minPrice)
	}
	if maxPrice > 0 {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.price <= ?", maxPrice)
	}
	if minBedrooms > 0 {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.bedrooms >= ?", minBedrooms)
	}
	if maxBedrooms > 0 {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.bedrooms <= ?", maxBedrooms)
	}
	if minBathrooms > 0 {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.bathrooms >= ?", minBathrooms)
	}
	if maxBathrooms > 0 {
		query = query.Joins("JOIN properties ON videos.property_id = properties.id").
			Where("properties.bathrooms <= ?", maxBathrooms)
	}

	// Apply TikTok-style sorting with property correlation
	var orderClause string
	switch sortBy {
	case "recent":
		orderClause = "videos.created_at " + sortOrder
	case "most_liked":
		orderClause = "videos.likes_count " + sortOrder + ", videos.created_at DESC"
	case "most_commented":
		orderClause = "videos.comments_count " + sortOrder + ", videos.created_at DESC"
	case "most_viewed":
		orderClause = "videos.views_count " + sortOrder + ", videos.created_at DESC"
	case "most_saved":
		orderClause = "videos.saves_count " + sortOrder + ", videos.created_at DESC"
	case "price_low":
		orderClause = "properties.nightly_price ASC, videos.created_at DESC"
	case "price_high":
		orderClause = "properties.nightly_price DESC, videos.created_at DESC"
	case "rating":
		orderClause = "properties.rating " + sortOrder + ", videos.created_at DESC"
	case "bedrooms":
		orderClause = "properties.bedrooms " + sortOrder + ", videos.created_at DESC"
	case "bathrooms":
		orderClause = "properties.bathrooms " + sortOrder + ", videos.created_at DESC"
	default:
		orderClause = "videos.created_at " + sortOrder
	}

	var videos []models.Video
	if err := query.Order(orderClause).Limit(limit).Offset(offset).Find(&videos).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Get user's liked and saved video IDs for this batch
	var videoIDs []uint
	for _, v := range videos {
		videoIDs = append(videoIDs, v.ID)
	}

	var likedVideoIDs []uint
	if len(videoIDs) > 0 {
		storage.DB.Model(&models.VideoLike{}).Where("video_id IN ? AND user_id = ?", videoIDs, userID).Pluck("video_id", &likedVideoIDs)
	}

	var savedVideoIDs []uint
	if len(videoIDs) > 0 {
		storage.DB.Model(&models.VideoSave{}).Where("video_id IN ? AND user_id = ?", videoIDs, userID).Pluck("video_id", &savedVideoIDs)
	}

	// Create maps for quick lookup
	likedMap := make(map[uint]bool)
	for _, id := range likedVideoIDs {
		likedMap[id] = true
	}
	savedMap := make(map[uint]bool)
	for _, id := range savedVideoIDs {
		savedMap[id] = true
	}

	// Add isLiked and isSaved to each video
	type VideoWithUserState struct {
		models.Video
		IsLiked bool `json:"isLiked"`
		IsSaved bool `json:"isSaved"`
	}

	var videosWithState []VideoWithUserState
	for _, video := range videos {
		videosWithState = append(videosWithState, VideoWithUserState{
			Video:   video,
			IsLiked: likedMap[video.ID],
			IsSaved: savedMap[video.ID],
		})
	}

	ctx.JSON(iris.Map{"success": true, "videos": videosWithState, "page": page})
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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
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

	// Get user's liked comment IDs
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

	likedMap := make(map[uint]bool)
	for _, id := range likedCommentIDs {
		likedMap[id] = true
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
