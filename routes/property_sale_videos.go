package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
)

// RecordPropertySaleVideoView records a view for a property sale video (supports both authenticated and anonymous users)
func RecordPropertySaleVideoView(ctx iris.Context) {
	propertySaleID, err := ctx.Params().GetUint("id")
	if err != nil || propertySaleID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property sale ID"})
		return
	}

	// Get user ID if authenticated (optional)
	var userID *uint
	var deviceID *string

	// Try to get userID from context values (set by optionalAuthMiddleware)
	if ctxUserID, ok := ctx.Values().Get("userID").(uint); ok && ctxUserID > 0 {
		userID = &ctxUserID
	}

	// Get deviceID from request body (optional)
	var input struct {
		DeviceID *string `json:"deviceID"`
	}
	if err := ctx.ReadJSON(&input); err == nil && input.DeviceID != nil {
		deviceID = input.DeviceID
	}

	// For property sale videos, we don't have a separate PropertySaleVideo table
	// Views are tracked at the PropertySale level or we can create a view tracking table
	// For now, we'll just log the view (can be extended later to track in a separate table)

	log.Printf("📹 Property Sale Video View: propertySaleID=%d, userID=%v, deviceID=%v",
		propertySaleID, userID, deviceID)

	ctx.JSON(iris.Map{
		"success":        true,
		"propertySaleID": propertySaleID,
		"viewed":         true,
	})
}

// CreatePropertySaleVideo stores a new property sale video record after upload is completed
func CreatePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ CreatePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		PropertySaleID uint    `json:"propertySaleID" validate:"required"`
		VideoURL       string  `json:"videoURL" validate:"required,url"`
		ThumbnailURL   string  `json:"thumbnailURL"`
		DurationSec    float64 `json:"durationSec"`
		Caption        string  `json:"caption"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Ensure property sale exists
	var propertySale models.PropertySale
	if err := storage.DB.Where("id = ?", input.PropertySaleID).First(&propertySale).Error; err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property sale not found"})
		return
	}

	video := models.PropertySaleVideo{
		PropertySaleID: input.PropertySaleID,
		UserID:         userID,
		VideoURL:       input.VideoURL,
		ThumbnailURL:   input.ThumbnailURL,
		DurationSec:    input.DurationSec,
		Caption:        input.Caption,
	}

	if err := storage.DB.Create(&video).Error; err != nil {
		fmt.Printf("Error creating property sale video: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{"success": true, "video": video})
}

// GetPropertySaleVideoFeed returns paginated property sale videos extracted from property sales table
func GetPropertySaleVideoFeed(ctx iris.Context) {
	// Get user ID if authenticated, otherwise use 0 for public access
	// Try context values first (set by optionalAuthMiddleware), then JWT claims
	var userID uint = 0

	// Method 1: Get from context values (set by optionalAuthMiddleware)
	if ctxUserID, ok := ctx.Values().Get("userID").(uint); ok && ctxUserID > 0 {
		userID = ctxUserID
		fmt.Printf("✅ Property Sale Video Feed - User ID from context: %d\n", userID)
	} else {
		// Method 2: Try to get from JWT claims in context values
		if jwtClaims := ctx.Values().Get("jwt.claims"); jwtClaims != nil {
			if accessToken, ok := jwtClaims.(*utils.AccessToken); ok && accessToken.ID > 0 {
				userID = accessToken.ID
				fmt.Printf("✅ Property Sale Video Feed - User ID from jwt.claims: %d\n", userID)
			}
		}

		// Method 3: Fallback to jsonWT.Get (for backward compatibility)
		if userID == 0 {
			if claims := jsonWT.Get(ctx); claims != nil {
				if accessToken, ok := claims.(*utils.AccessToken); ok && accessToken.ID > 0 {
					userID = accessToken.ID
					fmt.Printf("✅ Property Sale Video Feed - User ID from jsonWT.Get: %d\n", userID)
				}
			}
		}

		if userID == 0 {
			fmt.Printf("⚠️ Property Sale Video Feed - No user authentication - proceeding as public\n")
		}
	}

	// Simple pagination
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	offset := (page - 1) * limit

	// Get property sales that have videos
	// More lenient filter: show property sales that are published OR have videos
	query := storage.DB.
		Model(&models.PropertySale{}).
		Preload("Organization").
		Where("(status = ? OR is_published = ? OR status IS NULL) AND videos IS NOT NULL", "published", true)

	if userID > 0 {
		fmt.Printf("🔍 Property Sale Video Feed - Filtering for user ID: %d\n", userID)
		// Exclude hidden property sales
		query = query.Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID)
		fmt.Printf("✅ Applied user-specific filters (hidden property sales)\n")
	} else {
		fmt.Printf("⚠️ No user authentication - showing all property sale videos\n")
	}

	var propertySales []models.PropertySale
	if err := query.Order("property_sales.created_at DESC").Limit(limit).Offset(offset).Find(&propertySales).Error; err != nil {
		fmt.Printf("❌ Error fetching property sales: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch property sales with videos"})
		return
	}

	fmt.Printf("📹 Found %d property sales with videos (before filtering empty videos)\n", len(propertySales))

	// Convert property sales to video format
	var videos []map[string]interface{}
	for _, ps := range propertySales {
		// Videos is already a []string array
		videoURLs := ps.Videos

		// Skip if no videos
		if len(videoURLs) == 0 {
			continue
		}

		// Create a video entry for each video URL
		for i, videoURL := range videoURLs {
			videoID := fmt.Sprintf("%d_%d", ps.ID, i)

			// Get counts from like/save/comment tables (synthetic videos - using propertySaleID)
			var likesCount, savesCount, commentsCount int64
			storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", ps.ID).Count(&likesCount)
			storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", ps.ID).Count(&savesCount)
			storage.DB.Model(&models.PropertySaleVideoComment{}).Where("property_sale_video_id = ? AND parent_id IS NULL", ps.ID).Count(&commentsCount)

			// Get list of user IDs who liked this video (for debugging)
			var likedByUserIDs []uint
			if likesCount > 0 {
				var likes []models.PropertySaleVideoLike
				storage.DB.Where("property_sale_video_id = ?", ps.ID).Find(&likes)
				for _, like := range likes {
					likedByUserIDs = append(likedByUserIDs, like.UserID)
				}
			}

			// Check if user has liked/saved this video (if authenticated)
			var liked, saved bool
			if userID > 0 {
				// For synthetic videos, we'll check against the property sale ID
				// Use First() to check existence (more reliable than Count for soft-deleted records)
				var userLike models.PropertySaleVideoLike
				var userSave models.PropertySaleVideoSave
				likeErr := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", ps.ID, userID).First(&userLike).Error
				saveErr := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", ps.ID, userID).First(&userSave).Error
				liked = likeErr == nil
				saved = saveErr == nil

				// Debug logging - especially for property sale ID 10
				if ps.ID == 10 {
					fmt.Printf("🔍 Video %s (PropertySaleID: %d) - Checking User %d:\n", videoID, ps.ID, userID)
					fmt.Printf("   Likes Count: %d\n", likesCount)
					fmt.Printf("   Liked By User IDs: %v\n", likedByUserIDs)
					fmt.Printf("   Current User ID: %d\n", userID)
					fmt.Printf("   User Liked: %v (error: %v)\n", liked, likeErr)
					fmt.Printf("   User Saved: %v (error: %v)\n", saved, saveErr)

					// Check if current user ID is in the liked by list
					userInLikedList := false
					for _, likedByID := range likedByUserIDs {
						if likedByID == userID {
							userInLikedList = true
							break
						}
					}
					fmt.Printf("   User ID in liked list: %v\n", userInLikedList)
					if !userInLikedList && likesCount > 0 {
						fmt.Printf("   ⚠️ MISMATCH: User %d is NOT in liked list but count is %d!\n", userID, likesCount)
					}
				}
			} else {
				// No user authentication
				if ps.ID == 10 && likesCount > 0 {
					fmt.Printf("🔍 Video %s (PropertySaleID: %d) - No authenticated user, but has %d likes from users: %v\n",
						videoID, ps.ID, likesCount, likedByUserIDs)
				}
			}

			// Get organization branding (handle empty organization gracefully)
			orgLogo := ""
			orgName := ""
			orgOwnerID := uint(0)
			orgID := uint(0)
			if ps.OrganizationID != nil && *ps.OrganizationID > 0 && ps.Organization != nil && ps.Organization.ID > 0 {
				orgName = ps.Organization.Name
				if ps.Organization.Logo != "" {
					orgLogo = ps.Organization.Logo
				}
				orgOwnerID = ps.Organization.OwnerID
				orgID = ps.Organization.ID
			}

			video := map[string]interface{}{
				"ID":             videoID, // Unique ID for each video
				"propertySaleID": ps.ID,
				"propertySale":   ps,
				"userID":         orgOwnerID,
				"videoURL":       videoURL,
				"thumbnailURL":   "", // Will be generated from video
				"durationSec":    0,  // Will be calculated
				"caption":        ps.Title,
				"likesCount":     likesCount,
				"commentsCount":  commentsCount,
				"savesCount":     savesCount,
				"viewCount":      0,
				"isFlagged":      false,
				"status":         "approved",
				"liked":          liked,
				"saved":          saved,
				"organization": map[string]interface{}{
					"id":      orgID,
					"name":    orgName,
					"logoURL": orgLogo,
				},
				"CreatedAt": ps.CreatedAt,
				"UpdatedAt": ps.UpdatedAt,
			}

			// Add debug info for property sale ID 10 (include likedByUserIDs in response for debugging)
			if ps.ID == 10 {
				video["debugLikedByUserIDs"] = likedByUserIDs
				video["debugCurrentUserID"] = userID
			}

			videos = append(videos, video)
		}
	}

	fmt.Printf("📹 Property Sale Video Feed - Returning %d videos for user ID: %d (page: %d, limit: %d)\n", len(videos), userID, page, limit)

	ctx.JSON(iris.Map{
		"videos": videos,
		"pagination": iris.Map{
			"page":  page,
			"limit": limit,
			"total": len(videos),
		},
	})
}

// LikePropertySaleVideo handles liking a property sale video
func LikePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ LikePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Check if already liked
	var existingLike models.PropertySaleVideoLike
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existingLike).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "message": "Already liked"})
		return
	}

	// Create like
	like := models.PropertySaleVideoLike{
		PropertySaleVideoID: videoID,
		UserID:              userID,
	}

	if err := storage.DB.Create(&like).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Count total likes for this property sale video (synthetic videos, so count from likes table)
	var likesCount int64
	storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)

	log.Printf("✅ LikePropertySaleVideo: User %d liked video %d, new count: %d", userID, videoID, likesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video liked", "likesCount": likesCount})
}

// UnlikePropertySaleVideo handles unliking a property sale video
func UnlikePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnlikePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Delete like
	result := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).Delete(&models.PropertySaleVideoLike{})
	if result.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Count total likes for this property sale video (synthetic videos, so count from likes table)
	var likesCount int64
	storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)

	log.Printf("✅ UnlikePropertySaleVideo: User %d unliked video %d, new count: %d", userID, videoID, likesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video unliked", "likesCount": likesCount})
}

// SavePropertySaleVideo handles saving a property sale video
func SavePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ SavePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Check if already saved
	var existingSave models.PropertySaleVideoSave
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existingSave).Error; err == nil {
		// Already saved, return current count
		var savesCount int64
		storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Already saved", "savesCount": savesCount})
		return
	}

	// Create save
	save := models.PropertySaleVideoSave{
		PropertySaleVideoID: videoID,
		UserID:              userID,
	}

	if err := storage.DB.Create(&save).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Count total saves for this property sale video (synthetic videos, so count from saves table)
	var savesCount int64
	storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)

	log.Printf("✅ SavePropertySaleVideo: User %d saved video %d, new count: %d", userID, videoID, savesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video saved", "savesCount": savesCount})
}

// UnsavePropertySaleVideo handles unsaving a property sale video
func UnsavePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnsavePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Delete save
	result := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).Delete(&models.PropertySaleVideoSave{})
	if result.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Count total saves for this property sale video (synthetic videos, so count from saves table)
	var savesCount int64
	storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)

	log.Printf("✅ UnsavePropertySaleVideo: User %d unsaved video %d, new count: %d", userID, videoID, savesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video unsaved", "savesCount": savesCount})
}

// GetPropertySaleVideoComments gets comments for a property sale video with nested replies
func GetPropertySaleVideoComments(ctx iris.Context) {
	// Get user ID if authenticated (optional)
	var userID uint = 0
	if claims := jsonWT.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			userID = accessToken.ID
		}
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Get top-level comments (no parent)
	var comments []models.PropertySaleVideoComment
	err = storage.DB.Where("property_sale_video_id = ? AND parent_id IS NULL", videoID).
		Preload("User").
		Preload("Replies.User").
		Order("created_at DESC").
		Find(&comments).Error
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch comments"})
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
			storage.DB.Model(&models.PropertySaleVideoCommentLike{}).Where("comment_id IN ? AND user_id = ?", commentIDs, userID).Pluck("comment_id", &likedCommentIDs)
		}

		for _, id := range likedCommentIDs {
			likedMap[id] = true
		}
	}

	// Add isLiked to comments and replies
	type CommentWithUserState struct {
		models.PropertySaleVideoComment
		IsLiked bool                   `json:"isLiked"`
		Replies []CommentWithUserState `json:"replies"`
	}

	var commentsWithState []CommentWithUserState
	for _, comment := range comments {
		// Create replies with IsLiked
		var repliesWithState []CommentWithUserState
		for _, reply := range comment.Replies {
			repliesWithState = append(repliesWithState, CommentWithUserState{
				PropertySaleVideoComment: reply,
				IsLiked:                  likedMap[reply.ID],
			})
		}

		commentWithState := CommentWithUserState{
			PropertySaleVideoComment: comment,
			IsLiked:                  likedMap[comment.ID],
		}
		commentWithState.Replies = repliesWithState
		commentsWithState = append(commentsWithState, commentWithState)
	}

	ctx.JSON(iris.Map{"success": true, "comments": commentsWithState})
}

// CreatePropertySaleVideoComment creates a comment on a property sale video
func CreatePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ CreatePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	var input struct {
		Content  string `json:"content" validate:"required"`
		ParentID *uint  `json:"parentID"` // Optional parent ID for replies
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate parent comment exists if parentID is provided
	if input.ParentID != nil {
		var parentComment models.PropertySaleVideoComment
		if err := storage.DB.Where("id = ? AND property_sale_video_id = ?", *input.ParentID, videoID).First(&parentComment).Error; err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Parent comment not found"})
			return
		}
	}

	comment := models.PropertySaleVideoComment{
		PropertySaleVideoID: videoID,
		UserID:              userID,
		Content:             input.Content,
		ParentID:            input.ParentID,
	}

	if err := storage.DB.Create(&comment).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Note: For synthetic property sale videos, we don't update a PropertySaleVideo record
	// The comments count is calculated dynamically in the feed endpoint

	// Load the comment with user data
	storage.DB.Preload("User").First(&comment, comment.ID)

	ctx.JSON(iris.Map{"success": true, "comment": comment})
}

// UpdatePropertySaleVideoComment updates a property sale video comment
func UpdatePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UpdatePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	var input struct {
		Content string `json:"content" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var comment models.PropertySaleVideoComment
	if err := storage.DB.Where("id = ? AND user_id = ?", id, userID).First(&comment).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
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

// DeletePropertySaleVideoComment deletes a property sale video comment
func DeletePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ DeletePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	var comment models.PropertySaleVideoComment
	if err := storage.DB.Where("id = ? AND user_id = ?", id, userID).First(&comment).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Comment not found"})
		return
	}

	// Delete replies first
	storage.DB.Where("parent_id = ?", comment.ID).Delete(&models.PropertySaleVideoComment{})

	// Delete comment likes
	storage.DB.Where("comment_id = ?", comment.ID).Delete(&models.PropertySaleVideoCommentLike{})

	// Get video ID before deleting (for logging)
	videoID := comment.PropertySaleVideoID

	// Delete the comment
	storage.DB.Delete(&comment)

	// Note: We use dynamic counting in the feed endpoint, so no need to update a stored count
	// The feed always counts comments dynamically: WHERE property_sale_video_id = ? AND parent_id IS NULL

	log.Printf("✅ DeletePropertySaleVideoComment: User %d deleted comment %d for video %d", userID, id, videoID)
	ctx.JSON(iris.Map{"success": true})
}

// LikePropertySaleVideoComment handles liking a property sale video comment
func LikePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ LikePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	// Check if already liked
	var existingLike models.PropertySaleVideoCommentLike
	if err := storage.DB.Where("comment_id = ? AND user_id = ?", id, userID).First(&existingLike).Error; err == nil {
		// Already liked, return current count
		var comment models.PropertySaleVideoComment
		if err := storage.DB.First(&comment, id).Error; err == nil {
			ctx.JSON(iris.Map{"success": true, "message": "Already liked", "likesCount": comment.LikesCount})
		} else {
			ctx.JSON(iris.Map{"success": true, "message": "Already liked"})
		}
		return
	}

	// Create like
	like := models.PropertySaleVideoCommentLike{
		CommentID: id,
		UserID:    userID,
	}

	if err := storage.DB.Create(&like).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Update likes count
	storage.DB.Model(&models.PropertySaleVideoComment{}).Where("id = ?", id).Update("likes_count", gorm.Expr("likes_count + 1"))

	// Get updated likes count
	var comment models.PropertySaleVideoComment
	if err := storage.DB.First(&comment, id).Error; err == nil {
		log.Printf("✅ LikePropertySaleVideoComment: User %d liked comment %d, new count: %d", userID, id, comment.LikesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Comment liked", "likesCount": comment.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true, "message": "Comment liked"})
	}
}

// UnlikePropertySaleVideoComment handles unliking a property sale video comment
func UnlikePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnlikePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	// Delete like
	result := storage.DB.Where("comment_id = ? AND user_id = ?", id, userID).Delete(&models.PropertySaleVideoCommentLike{})
	if result.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if result.RowsAffected > 0 {
		// Update likes count
		storage.DB.Model(&models.PropertySaleVideoComment{}).Where("id = ?", id).Update("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)"))
	}

	// Get updated likes count
	var comment models.PropertySaleVideoComment
	if err := storage.DB.First(&comment, id).Error; err == nil {
		log.Printf("✅ UnlikePropertySaleVideoComment: User %d unliked comment %d, new count: %d", userID, id, comment.LikesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Comment unliked", "likesCount": comment.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true, "message": "Comment unliked"})
	}
}

// GetPropertySaleVideosByOrganizationOrHost returns property sale videos with stats for admin interface
// Supports filtering by organization_id or owner_id (for individual hosts)
func GetPropertySaleVideosByOrganizationOrHost(ctx iris.Context) {
	// Get user ID from middleware context (must be authenticated)
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetPropertySaleVideosByOrganizationOrHost: Unauthorized")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	// Get query parameters
	organizationID := ctx.URLParamDefault("organization_id", "")
	ownerID := ctx.URLParamDefault("owner_id", "")

	// Build query for property sales with videos
	// The videos field is stored as json/jsonb array, check if it's not empty
	// Use text comparison to avoid type casting issues
	query := storage.DB.Model(&models.PropertySale{}).
		Where("videos IS NOT NULL AND videos::text != '[]' AND videos::text != 'null'").
		Preload("Organization").
		Preload("Owner")

	// Filter by organization or owner
	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	} else if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	} else {
		// If no filter, check if user is admin of any organization or is an owner
		var userOrgs []models.OrganizationMember
		storage.DB.Where("user_id = ? AND role = ?", userID, "admin").Find(&userOrgs)

		var orgIDs []uint
		for _, member := range userOrgs {
			orgIDs = append(orgIDs, member.OrganizationID)
		}

		if len(orgIDs) > 0 {
			query = query.Where("organization_id IN ? OR owner_id = ?", orgIDs, userID)
		} else {
			// User is not admin, only show their own properties
			query = query.Where("owner_id = ?", userID)
		}
	}

	// Get property sales
	var propertySales []models.PropertySale
	if err := query.Order("created_at DESC").Find(&propertySales).Error; err != nil {
		log.Printf("❌ Error fetching property sales: %v", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Extract videos with stats
	type VideoStats struct {
		PropertySaleID   uint   `json:"propertySaleID"`
		PropertyTitle    string `json:"propertyTitle"`
		VideoURL         string `json:"videoURL"`
		ThumbnailURL     string `json:"thumbnailURL"`
		LikesCount       int64  `json:"likesCount"`
		CommentsCount    int64  `json:"commentsCount"`
		SavesCount       int64  `json:"savesCount"`
		ViewCount        int64  `json:"viewCount"`
		OrganizationName string `json:"organizationName,omitempty"`
		OwnerName        string `json:"ownerName,omitempty"`
		CreatedAt        string `json:"createdAt"`
	}

	var videos []VideoStats

	for _, ps := range propertySales {
		// Videos is already a []string array (not JSON)
		videoURLs := ps.Videos

		// Skip if no videos
		if len(videoURLs) == 0 {
			continue
		}

		// Get actual counts from database using property sale ID
		// Note: PropertySaleVideoLike uses PropertySaleVideoID which is the property sale ID
		var likesCount int64
		storage.DB.Model(&models.PropertySaleVideoLike{}).
			Where("property_sale_video_id = ?", ps.ID).
			Count(&likesCount)

		var commentsCount int64
		storage.DB.Model(&models.PropertySaleVideoComment{}).
			Where("property_sale_video_id = ?", ps.ID).
			Count(&commentsCount)

		var savesCount int64
		storage.DB.Model(&models.PropertySaleVideoSave{}).
			Where("property_sale_video_id = ?", ps.ID).
			Count(&savesCount)

		// Get view count - views are not tracked in a separate table yet
		// For now, we'll set it to 0 (can be extended later when view tracking is implemented)
		var viewCount int64 = 0

		// Create a video entry for each video URL
		for _, videoURL := range videoURLs {
			// Try to extract thumbnail from video URL or use a placeholder
			thumbnailURL := videoURL // For now, use video URL as thumbnail (can be enhanced later)

			video := VideoStats{
				PropertySaleID: ps.ID,
				PropertyTitle:  ps.Title,
				VideoURL:       videoURL,
				ThumbnailURL:   thumbnailURL,
				LikesCount:     likesCount,
				CommentsCount:  commentsCount,
				SavesCount:     savesCount,
				ViewCount:      viewCount,
				CreatedAt:      ps.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}

			if ps.Organization != nil {
				video.OrganizationName = ps.Organization.Name
			}
			if ps.Owner != nil {
				// Combine FirstName and LastName
				ownerName := strings.TrimSpace(fmt.Sprintf("%s %s", ps.Owner.FirstName, ps.Owner.LastName))
				if ownerName == "" {
					ownerName = ps.Owner.Email // Fallback to email if no name
				}
				video.OwnerName = ownerName
			}

			videos = append(videos, video)
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"videos":  videos,
		"total":   len(videos),
	})
}
