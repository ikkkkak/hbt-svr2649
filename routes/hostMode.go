package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// RecordHostModeSwitch records when a user switches to host mode
// This is called from the client when user switches to host mode
func RecordHostModeSwitch(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ RecordHostModeSwitch: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	// Check if this is the first switch for this user
	var existingSwitch models.HostModeSwitch
	isFirstSwitch := storage.DB.Where("user_id = ? AND is_first_switch = ?", userID, true).First(&existingSwitch).Error != nil

	// Create or update host mode switch record
	hostModeSwitch := models.HostModeSwitch{
		UserID:        userID,
		SwitchedAt:    time.Now(),
		IsFirstSwitch: isFirstSwitch,
	}

	if err := storage.DB.Create(&hostModeSwitch).Error; err != nil {
		log.Printf("❌ Error recording host mode switch for user %d: %v", userID, err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to record switch"})
		return
	}

	// Record interaction for learning
	interaction := models.HostModeInteraction{
		UserID:           userID,
		InteractionType:  "host_mode_switched",
		InteractionData: `{"is_first_switch": true}`,
		CreatedAt:       time.Now(),
	}
	storage.DB.Create(&interaction)

	log.Printf("✅ Recorded host mode switch for user %d (first switch: %v)", userID, isFirstSwitch)

	ctx.JSON(iris.Map{
		"success":        true,
		"is_first_switch": isFirstSwitch,
		"switched_at":    hostModeSwitch.SwitchedAt,
	})
}

// RecordHostModeInteraction records user interactions for learning
func RecordHostModeInteraction(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ RecordHostModeInteraction: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		InteractionType string `json:"interaction_type" validate:"required"`
		InteractionData string `json:"interaction_data"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid input"})
		return
	}

	// Record interaction
	interaction := models.HostModeInteraction{
		UserID:          userID,
		InteractionType: input.InteractionType,
		InteractionData: input.InteractionData,
		CreatedAt:       time.Now(),
	}

	if err := storage.DB.Create(&interaction).Error; err != nil {
		log.Printf("❌ Error recording interaction for user %d: %v", userID, err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to record interaction"})
		return
	}

	// Update last interaction time in HostModeSwitch if exists
	var hostModeSwitch models.HostModeSwitch
	if err := storage.DB.Where("user_id = ? AND is_first_switch = ?", userID, true).First(&hostModeSwitch).Error; err == nil {
		now := time.Now()
		hostModeSwitch.LastInteractionAt = &now
		storage.DB.Save(&hostModeSwitch)
	}

	// If interaction is property_added, update HostModeSwitch
	if input.InteractionType == "property_added" {
		var hostSwitch models.HostModeSwitch
		if err := storage.DB.Where("user_id = ? AND property_added = ?", userID, false).First(&hostSwitch).Error; err == nil {
			now := time.Now()
			hostSwitch.PropertyAdded = true
			hostSwitch.PropertyAddedAt = &now
			
			// Calculate time to add property
			if !hostSwitch.SwitchedAt.IsZero() {
				duration := now.Sub(hostSwitch.SwitchedAt)
				hostSwitch.TimeToAddProperty = &duration
			}
			
			storage.DB.Save(&hostSwitch)
			log.Printf("✅ User %d added property after %v", userID, hostSwitch.TimeToAddProperty)
		}
	}

	ctx.JSON(iris.Map{"success": true})
}
