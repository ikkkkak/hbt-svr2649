package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"log"
	"time"
)

// StartHostModeNotificationScheduler starts a background goroutine that checks for users
// who switched to host mode 2 hours ago without adding a property
func StartHostModeNotificationScheduler() {
	log.Println("🚀 Starting Host Mode Notification Scheduler...")

	go func() {
		ticker := time.NewTicker(15 * time.Minute) // Check every 15 minutes
		defer ticker.Stop()

		// Run immediately on startup
		checkAndSendNotifications()

		for range ticker.C {
			checkAndSendNotifications()
		}
	}()
}

// checkAndSendNotifications finds users who switched to host mode 2 hours ago
// and haven't added a property, then sends engaging Arabic notifications
func checkAndSendNotifications() {
	log.Println("🔍 Checking for host mode notification candidates...")

	// Calculate the time threshold (2 hours ago)
	twoHoursAgo := time.Now().Add(-2 * time.Hour)

	// Find users who:
	// 1. Switched to host mode between 1.5 and 2.5 hours ago (to catch the 2-hour mark)
	// 2. Haven't added a property yet
	// 3. Haven't received a notification yet
	var candidates []models.HostModeSwitch
	err := storage.DB.
		Where("is_first_switch = ? AND property_added = ? AND notification_sent = ?", true, false, false).
		Where("switched_at >= ? AND switched_at <= ?", twoHoursAgo.Add(-30*time.Minute), twoHoursAgo.Add(30*time.Minute)).
		Preload("User").
		Find(&candidates).Error

	if err != nil {
		log.Printf("❌ Error querying host mode switches: %v", err)
		return
	}

	log.Printf("📊 Found %d candidates for host mode notification", len(candidates))

	notificationService := NewNotificationService()

	for _, candidate := range candidates {
		// Double-check it's been at least 2 hours
		timeSinceSwitch := time.Since(candidate.SwitchedAt)
		if timeSinceSwitch < 2*time.Hour {
			log.Printf("⏳ User %d switched %v ago, waiting for 2 hours...", candidate.UserID, timeSinceSwitch)
			continue
		}

		// Check if user has notifications enabled
		if candidate.User.AllowsNotifications == nil || !*candidate.User.AllowsNotifications {
			log.Printf("🔕 User %d has notifications disabled, skipping", candidate.UserID)
			// Mark as sent anyway to avoid repeated checks
			candidate.NotificationSent = true
			now := time.Now()
			candidate.NotificationSentAt = &now
			storage.DB.Save(&candidate)
			continue
		}

		// Send localized host-mode reminder
		userName := candidate.User.FirstName
		lang := ResolveUserNotificationLang(candidate.UserID)
		title, body := HostModeReminderCopy(lang, userName)

		// Create notification data with deep linking to AddProperty screen
		notificationData := NotificationData{
			Type:   "host_mode_reminder",
			UserID: fmt.Sprintf("%d", candidate.UserID),
			Screen: "AddProperty",
			Params: `{"source": "host_mode_notification"}`,
			Action: "add_property",
		}

		// Send notification
		err := notificationService.SendNotificationToUser(candidate.UserID, title, body, notificationData)
		if err != nil {
			log.Printf("❌ Failed to send notification to user %d: %v", candidate.UserID, err)
			continue
		}

		// Mark notification as sent
		candidate.NotificationSent = true
		now := time.Now()
		candidate.NotificationSentAt = &now
		storage.DB.Save(&candidate)

		// Record interaction for learning
		interaction := models.HostModeInteraction{
			UserID:          candidate.UserID,
			InteractionType: "notification_sent",
			InteractionData: `{"notification_type": "host_mode_reminder", "time_since_switch": "` + timeSinceSwitch.String() + `"}`,
			CreatedAt:       time.Now(),
		}
		storage.DB.Create(&interaction)

		log.Printf("✅ Sent host mode reminder notification to user %d (%s)", candidate.UserID, userName)
	}
}
