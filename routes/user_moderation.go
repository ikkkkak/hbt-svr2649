package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kataras/iris/v12"
)

// GetBlockedUsers - GET /api/user/blocked
func GetBlockedUsers(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	var blockedUsers []struct {
		ID             uint   `json:"id"`
		FirstName      string `json:"firstName"`
		LastName       string `json:"lastName"`
		ProfilePicture string `json:"profilePicture"`
		BlockedAt      string `json:"blockedAt"`
	}

	// Get users blocked by this user
	result := storage.DB.Table("user_flags").
		Select("users.id, users.first_name, users.last_name, users.avatar_url as profile_picture, user_flags.created_at as blocked_at").
		Joins("JOIN users ON user_flags.flagged_user_id = users.id").
		Where("user_flags.flagger_id = ? AND user_flags.status = ?", userID, "active").
		Order("user_flags.created_at DESC").
		Scan(&blockedUsers)

	if result.Error != nil {
		fmt.Printf("❌ Error fetching blocked users: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch blocked users"})
		return
	}

	fmt.Printf("✅ Found %d blocked users for user %d\n", len(blockedUsers), userID)
	ctx.JSON(blockedUsers)
}

// GetHiddenProperties - GET /api/user/hidden-properties
func GetHiddenProperties(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	var hiddenProperties []struct {
		ID       uint   `json:"id"`
		Title    string `json:"title"`
		Images   string `json:"images"` // Store as string to handle JSON
		City     string `json:"city"`
		HiddenAt string `json:"hiddenAt"`
	}

	// Get properties hidden by this user
	result := storage.DB.Table("hidden_properties").
		Select("properties.id, properties.title, properties.images, properties.city, hidden_properties.created_at as hidden_at").
		Joins("JOIN properties ON hidden_properties.property_id = properties.id").
		Where("hidden_properties.user_id = ?", userID).
		Order("hidden_properties.created_at DESC").
		Scan(&hiddenProperties)

	if result.Error != nil {
		fmt.Printf("❌ Error fetching hidden properties: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch hidden properties"})
		return
	}

	// Parse JSON images for each property
	type PropertyWithParsedImages struct {
		ID       uint     `json:"id"`
		Title    string   `json:"title"`
		Images   []string `json:"images"`
		City     string   `json:"city"`
		HiddenAt string   `json:"hiddenAt"`
	}

	var propertiesWithImages []PropertyWithParsedImages
	for _, prop := range hiddenProperties {
		var images []string
		if prop.Images != "" {
			// Try to parse JSON array
			if err := json.Unmarshal([]byte(prop.Images), &images); err != nil {
				// If parsing fails, treat as single image
				images = []string{prop.Images}
			}
		}

		propertiesWithImages = append(propertiesWithImages, PropertyWithParsedImages{
			ID:       prop.ID,
			Title:    prop.Title,
			Images:   images,
			City:     prop.City,
			HiddenAt: prop.HiddenAt,
		})
	}

	fmt.Printf("✅ Found %d hidden properties for user %d\n", len(propertiesWithImages), userID)
	ctx.JSON(propertiesWithImages)
}

// GetHiddenVideos - GET /api/user/hidden-videos
func GetHiddenVideos(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	var hiddenVideos []struct {
		ID        uint   `json:"id"`
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
		HiddenAt  string `json:"hiddenAt"`
	}

	// Get videos hidden by this user
	result := storage.DB.Table("hidden_videos").
		Select("videos.id, videos.caption as title, videos.thumbnail_url as thumbnail, hidden_videos.created_at as hidden_at").
		Joins("JOIN videos ON hidden_videos.video_id = videos.id").
		Where("hidden_videos.user_id = ? AND hidden_videos.deleted_at IS NULL", userID).
		Order("hidden_videos.created_at DESC").
		Scan(&hiddenVideos)

	if result.Error != nil {
		fmt.Printf("❌ Error fetching hidden videos: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch hidden videos"})
		return
	}

	fmt.Printf("✅ Found %d hidden videos for user %d\n", len(hiddenVideos), userID)
	ctx.JSON(hiddenVideos)
}

// UnblockUser - DELETE /api/users/{id}/unblock
func UnblockUser(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	targetUserID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	// Remove the block (set status to inactive)
	result := storage.DB.Model(&models.UserFlag{}).
		Where("flagger_id = ? AND flagged_user_id = ? AND status = ?", userID, targetUserID, "active").
		Update("status", "inactive")

	if result.Error != nil {
		fmt.Printf("❌ Error unblocking user: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to unblock user"})
		return
	}

	if result.RowsAffected == 0 {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found in blocked list"})
		return
	}

	fmt.Printf("✅ User %d unblocked by user %d\n", targetUserID, userID)
	ctx.JSON(iris.Map{"success": true, "message": "User unblocked successfully"})
}

// UnhideProperty - DELETE /api/properties/{id}/unhide
func UnhideProperty(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	// Remove the hidden property
	result := storage.DB.Where("user_id = ? AND property_id = ?", userID, propertyID).Delete(&models.HiddenProperty{})

	if result.Error != nil {
		fmt.Printf("❌ Error unhiding property: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to unhide property"})
		return
	}

	if result.RowsAffected == 0 {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found in hidden list"})
		return
	}

	fmt.Printf("✅ Property %d unhidden by user %d\n", propertyID, userID)
	ctx.JSON(iris.Map{"success": true, "message": "Property unhidden successfully"})
}

// UnhideVideo - DELETE /api/videos/{id}/unhide
func UnhideVideo(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Remove the hidden video (primary: owned by this user). Use Unscoped to ensure hard-delete.
	result := storage.DB.Unscoped().Where("user_id = ? AND video_id = ?", userID, videoID).Delete(&models.HiddenVideo{})

	if result.Error != nil {
		fmt.Printf("❌ Error unhiding video: %v\n", result.Error)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to unhide video"})
		return
	}

	if result.RowsAffected == 0 {
		// Legacy/anonymous hides: try deleting records with NULL user_id for this video
		legacy := storage.DB.Unscoped().Where("user_id IS NULL AND video_id = ?", videoID).Delete(&models.HiddenVideo{})
		if legacy.Error != nil {
			fmt.Printf("❌ Error unhiding legacy video: %v\n", legacy.Error)
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to unhide video"})
			return
		}
		if legacy.RowsAffected == 0 {
			// Idempotent: treat as already unhidden
			fmt.Printf("ℹ️  Unhide noop: video %d not in hidden list for user %d\n", videoID, userID)
			ctx.JSON(iris.Map{"success": true, "message": "Video already unhidden"})
			return
		}
	}

	fmt.Printf("✅ Video %d unhidden by user %d\n", videoID, userID)
	ctx.JSON(iris.Map{"success": true, "message": "Video unhidden successfully"})
}

// BlockUser blocks a user (creates a user flag)
func BlockUser(ctx iris.Context) {
	// Log request intent early for observability
	fmt.Printf("➡️  BlockUser request: method=%s path=%s authUserID=%v targetParam=%s\n",
		ctx.Method(), ctx.Path(), ctx.Values().Get("userID"), ctx.Params().GetString("id"))
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"success": false, "error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	targetUserID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "Invalid user ID"})
		return
	}

	if userID == targetUserID {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "Cannot block yourself"})
		return
	}

	// Check if already blocked
	var existingFlag models.UserFlag
	if err := storage.DB.Where("flagger_id = ? AND flagged_user_id = ? AND status = 'active'", userID, targetUserID).First(&existingFlag).Error; err == nil {
		fmt.Printf("ℹ️  User %d already blocked by user %d\n", targetUserID, userID)
		ctx.JSON(iris.Map{"success": true, "blocked": true, "message": "User already blocked"})
		return
	}

	// Create new block
	flag := models.UserFlag{
		FlaggerID:     &userID,
		FlaggedUserID: targetUserID,
		Status:        "active",
	}
	if err := storage.DB.Create(&flag).Error; err != nil {
		fmt.Printf("❌ Error blocking user: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "Failed to block user"})
		return
	}

	fmt.Printf("✅ User %d blocked by user %d\n", targetUserID, userID)
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{"success": true, "blocked": true, "message": "User blocked successfully"})
}
