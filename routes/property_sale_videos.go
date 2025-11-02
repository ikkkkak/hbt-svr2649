package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"net/http"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
)

// CreatePropertySaleVideo stores a new property sale video record after upload is completed
func CreatePropertySaleVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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
	var userID uint = 0
	if claims := jsonWT.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			userID = accessToken.ID
			fmt.Printf("🔍 Property Sale Video Feed - JWT Token parsed - User ID: %d\n", userID)
		} else {
			fmt.Printf("❌ Failed to parse JWT token as AccessToken\n")
		}
	} else {
		fmt.Printf("❌ No JWT claims found in request\n")
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

			// Check if user has liked/saved this video (if authenticated)
			var liked, saved bool
			if userID > 0 {
				// For synthetic videos, we'll check against the property sale ID
				var likeCount, saveCount int64
				storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ? AND user_id = ?", ps.ID, userID).Count(&likeCount)
				storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ? AND user_id = ?", ps.ID, userID).Count(&saveCount)
				liked = likeCount > 0
				saved = saveCount > 0
			}

			// Get organization branding (handle empty organization gracefully)
			orgLogo := ""
			orgName := ""
			orgOwnerID := uint(0)
			orgID := uint(0)
			if ps.OrganizationID > 0 && ps.Organization.ID > 0 {
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
				"likesCount":     0,
				"commentsCount":  0,
				"savesCount":     0,
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
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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

	// Update likes count
	storage.DB.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Update("likes_count", gorm.Expr("likes_count + 1"))

	ctx.JSON(iris.Map{"success": true, "message": "Video liked"})
}

// UnlikePropertySaleVideo handles unliking a property sale video
func UnlikePropertySaleVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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

	if result.RowsAffected > 0 {
		// Update likes count
		storage.DB.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Update("likes_count", gorm.Expr("likes_count - 1"))
	}

	ctx.JSON(iris.Map{"success": true, "message": "Video unliked"})
}

// SavePropertySaleVideo handles saving a property sale video
func SavePropertySaleVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Check if already saved
	var existingSave models.PropertySaleVideoSave
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existingSave).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "message": "Already saved"})
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

	// Update saves count
	storage.DB.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Update("saves_count", gorm.Expr("saves_count + 1"))

	ctx.JSON(iris.Map{"success": true, "message": "Video saved"})
}

// UnsavePropertySaleVideo handles unsaving a property sale video
func UnsavePropertySaleVideo(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

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

	if result.RowsAffected > 0 {
		// Update saves count
		storage.DB.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Update("saves_count", gorm.Expr("saves_count - 1"))
	}

	ctx.JSON(iris.Map{"success": true, "message": "Video unsaved"})
}

// GetPropertySaleVideoComments gets comments for a property sale video
func GetPropertySaleVideoComments(ctx iris.Context) {
	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Simple pagination
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	var comments []models.PropertySaleVideoComment
	if err := storage.DB.
		Preload("User").
		Where("property_sale_video_id = ?", videoID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&comments).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch comments"})
		return
	}

	ctx.JSON(iris.Map{
		"comments": comments,
		"pagination": iris.Map{
			"page":  page,
			"limit": limit,
		},
	})
}

// CreatePropertySaleVideoComment creates a comment on a property sale video
func CreatePropertySaleVideoComment(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
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

	comment := models.PropertySaleVideoComment{
		PropertySaleVideoID: videoID,
		UserID:              userID,
		Content:             input.Content,
	}

	if err := storage.DB.Create(&comment).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Update comments count
	storage.DB.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Update("comments_count", gorm.Expr("comments_count + 1"))

	// Load the comment with user data
	storage.DB.Preload("User").First(&comment, comment.ID)

	ctx.JSON(iris.Map{"success": true, "comment": comment})
}
