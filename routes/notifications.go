package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/kataras/iris/v12"
)

// TestNotificationInput represents the input for testing notifications
type TestNotificationInput struct {
	UserID uint   `json:"userId" validate:"required"`
	Title  string `json:"title" validate:"required"`
	Body   string `json:"body" validate:"required"`
	Type   string `json:"type"`
}

// SendTestNotification sends a test notification to a user (admin only)
func SendTestNotification(ctx iris.Context) {
	var input TestNotificationInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	data := services.NotificationData{
		Type:   input.Type,
		UserID: strconv.FormatUint(uint64(input.UserID), 10),
	}

	notificationService := services.NewNotificationService()
	if err := notificationService.SendNotificationToUser(input.UserID, input.Title, input.Body, data); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{
			"success": false,
			"message": "Failed to send notification",
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Test notification sent successfully",
	})
}

// GetUserNotificationSettings returns notification settings for a user
func GetUserNotificationSettings(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User ID not found in context"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Invalid user ID format"})
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "User not found"})
		return
	}

	ctx.JSON(iris.Map{
		"success":             true,
		"allowsNotifications": user.AllowsNotifications,
		"hasTokens":           user.PushTokens != nil,
		"reservations":        true, // Default settings
		"messages":            true,
		"propertyUpdates":     true,
		"experienceBookings":  true,
		"videoInteractions":   true,
		"reminders":           true,
	})
}

// UpdateUserNotificationSettings updates notification preferences
func UpdateUserNotificationSettings(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User ID not found in context"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Invalid user ID format"})
		return
	}

	type NotificationSettingsInput struct {
		AllowsNotifications bool `json:"allowsNotifications"`
		Reservations        bool `json:"reservations"`
		Messages            bool `json:"messages"`
		PropertyUpdates     bool `json:"propertyUpdates"`
		ExperienceBookings  bool `json:"experienceBookings"`
		VideoInteractions   bool `json:"videoInteractions"`
		Reminders           bool `json:"reminders"`
	}

	var input NotificationSettingsInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "User not found"})
		return
	}

	user.AllowsNotifications = &input.AllowsNotifications

	if err := storage.DB.Save(&user).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to update notification settings"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Notification settings updated successfully",
	})
}

// SendWelcomeNotification sends welcome notification to new users
func SendWelcomeNotification(ctx iris.Context) {
	type WelcomeNotificationInput struct {
		UserID uint `json:"userId" validate:"required"`
	}

	var input WelcomeNotificationInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var user models.User
	if err := storage.DB.First(&user, input.UserID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "User not found"})
		return
	}

	notificationService := services.NewNotificationService()
	if err := notificationService.SendWelcomeNotificationToNewUser(input.UserID, user.FirstName); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{
			"success": false,
			"message": "Failed to send welcome notification",
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Welcome notification sent successfully",
	})
}

func SendDetailedTestNotification(ctx iris.Context) {
	userID := ctx.Params().GetIntDefault("userID", 0)
	if userID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	log.Printf("🧪 DETAILED TEST: Starting notification test for user %d", userID)

	// Get user from database
	var user models.User
	result := storage.DB.First(&user, userID)
	if result.Error != nil {
		log.Printf("❌ TEST ERROR: User %d not found: %v", userID, result.Error)
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found", "details": result.Error.Error()})
		return
	}

	log.Printf("🧪 TEST USER: ID=%d, AllowsNotifications=%v", user.ID, user.AllowsNotifications != nil && *user.AllowsNotifications)

	// Check if notifications are enabled
	if user.AllowsNotifications == nil || !*user.AllowsNotifications {
		log.Printf("❌ TEST ERROR: User %d has notifications disabled", userID)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "User has notifications disabled"})
		return
	}

	// Get push tokens
	var tokens []string
	if user.PushTokens != nil {
		if err := json.Unmarshal(user.PushTokens, &tokens); err != nil {
			log.Printf("❌ TEST ERROR: Failed to parse tokens for user %d: %v", userID, err)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to parse push tokens", "details": err.Error()})
			return
		}
	}

	if len(tokens) == 0 {
		log.Printf("❌ TEST ERROR: No push tokens found for user %d", userID)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "No push tokens found for user"})
		return
	}

	log.Printf("🧪 TEST TOKENS: Found %d tokens for user %d", len(tokens), userID)
	for i, token := range tokens {
		log.Printf("🧪 TOKEN %d: %s", i+1, token)
	}

	// Send test notification
	title := "🧪 Detailed Test Notification"
	body := fmt.Sprintf("This is a detailed test for user %d with %d tokens", userID, len(tokens))

	// Create notification service instance
	notificationService := services.NewNotificationService()
	data := services.NotificationData{
		Type:   "test",
		UserID: fmt.Sprintf("%d", user.ID),
	}
	err := notificationService.SendNotificationToUser(user.ID, title, body, data)
	if err != nil {
		log.Printf("❌ TEST ERROR: Failed to send notification: %v", err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{
			"error":        "Failed to send notification",
			"details":      err.Error(),
			"user_id":      userID,
			"tokens_count": len(tokens),
			"tokens":       tokens,
		})
		return
	}

	log.Printf("✅ TEST SUCCESS: Notification sent to user %d", userID)
	ctx.JSON(iris.Map{
		"success":      true,
		"message":      "Detailed test notification sent successfully",
		"user_id":      userID,
		"tokens_count": len(tokens),
		"tokens":       tokens,
		"title":        title,
		"body":         body,
	})
}
