package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// TrackUserBehavior tracks user interactions with properties
func TrackUserBehavior(ctx iris.Context) {
	var input struct {
		PropertyID      uint   `json:"property_id" validate:"required"`
		PropertyType    string `json:"property_type" validate:"required,oneof=sale rent"`
		InteractionType string `json:"interaction_type" validate:"required,oneof=view click favorite contact"`
		CityID          *uint  `json:"city_id"`
		CityName        string `json:"city_name"`
		ZoneID          *uint  `json:"zone_id"`
		ZoneName        string `json:"zone_name"`
		TimeSpent       int    `json:"time_spent"` // Time in seconds
		DeviceID        string `json:"device_id"` // Device identifier for anonymous users
		PhoneNumber     string `json:"phone_number"` // Phone number for anonymous users
	}

	if err := ctx.ReadJSON(&input); err != nil {
		log.Printf("❌ Error parsing JSON: %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON", "details": err.Error()})
		return
	}

	// Get user ID (nullable for anonymous users)
	var userID *uint
	if userIDValue := ctx.Values().Get("userID"); userIDValue != nil {
		uid := userIDValue.(uint)
		userID = &uid
		log.Printf("📊 Tracking behavior for logged-in user ID: %d", uid)
	} else {
		log.Printf("📊 Tracking behavior for anonymous user (device_id: %s, phone: %s)", input.DeviceID, input.PhoneNumber)
	}

	// Normalize phone number if provided
	var phoneNumber *string
	if input.PhoneNumber != "" {
		normalized := input.PhoneNumber
		phoneNumber = &normalized
	}

	// Create behavior record
	behavior := models.UserBehavior{
		UserID:          userID,
		DeviceID:        input.DeviceID,
		PhoneNumber:     phoneNumber,
		PropertyID:      input.PropertyID,
		PropertyType:    input.PropertyType,
		InteractionType: input.InteractionType,
		CityID:          input.CityID,
		CityName:        input.CityName,
		ZoneID:          input.ZoneID,
		ZoneName:        input.ZoneName,
		TimeSpent:       input.TimeSpent,
		Timestamp:       time.Now(),
	}

	if err := storage.DB.Create(&behavior).Error; err != nil {
		log.Printf("❌ Error tracking user behavior: %v", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to track behavior", "details": err.Error()})
		return
	}

	log.Printf("✅ Behavior tracked successfully: user_id=%v, device_id=%s, property_id=%d, city=%s, interaction=%s, time_spent=%d",
		userID, func() string {
			if input.DeviceID != "" {
				if len(input.DeviceID) > 10 {
					return input.DeviceID[:10] + "..."
				}
				return input.DeviceID
			}
			return "none"
		}(), input.PropertyID, input.CityName, input.InteractionType, input.TimeSpent)

	// Update favorite city inference (works for both logged-in and anonymous users)
	// Always try to infer for anonymous users if device_id is provided (even if user is logged in)
	// This allows tracking across devices and when user logs out
	if input.DeviceID != "" {
		go inferAndUpdateFavoriteCityForAnonymous(input.DeviceID, input.PhoneNumber)
	}
	// Also infer for logged-in users
	if userID != nil {
		go inferAndUpdateFavoriteCity(*userID, input.DeviceID)
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{
		"success": true,
		"message": "Behavior tracked",
		"behavior_id": behavior.ID,
	})
}

// SetFavoriteCity manually sets user's favorite city
func SetFavoriteCity(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var input struct {
		CityID   *uint  `json:"city_id"`
		CityName string `json:"city_name"`
		ZoneID   *uint  `json:"zone_id"`
		ZoneName string `json:"zone_name"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found"})
		return
	}

	// Update favorite city
	user.FavoriteCityID = input.CityID
	user.FavoriteCityName = input.CityName
	user.FavoriteZoneID = input.ZoneID
	user.FavoriteZoneName = input.ZoneName

	if err := storage.DB.Save(&user).Error; err != nil {
		log.Printf("❌ Error updating favorite city: %v", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update favorite city"})
		return
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{
		"success": true,
		"message": "Favorite city updated",
		"favoriteCity": iris.Map{
			"cityId":   user.FavoriteCityID,
			"cityName": user.FavoriteCityName,
			"zoneId":   user.FavoriteZoneID,
			"zoneName": user.FavoriteZoneName,
		},
	})
}

// GetFavoriteCity retrieves user's favorite city
func GetFavoriteCity(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found"})
		return
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{
		"success": true,
		"favoriteCity": iris.Map{
			"cityId":   user.FavoriteCityID,
			"cityName": user.FavoriteCityName,
			"zoneId":   user.FavoriteZoneID,
			"zoneName": user.FavoriteZoneName,
		},
	})
}

// inferAndUpdateFavoriteCity analyzes user behavior to infer favorite city
func inferAndUpdateFavoriteCity(userID uint, deviceID string) {
	// Get all user behaviors from last 30 days
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	
	var behaviors []models.UserBehavior
	query := storage.DB.Where("user_id = ? AND timestamp >= ?", userID, thirtyDaysAgo)
	if deviceID != "" {
		// Also include behaviors from same device if user was anonymous before
		query = storage.DB.Where("(user_id = ? OR device_id = ?) AND timestamp >= ?", userID, deviceID, thirtyDaysAgo)
	}
	
	if err := query.Find(&behaviors).Error; err != nil {
		log.Printf("❌ Error fetching behaviors for user %d: %v", userID, err)
		return
	}

	log.Printf("📊 Analyzing %d behaviors for user %d", len(behaviors), userID)

	if len(behaviors) == 0 {
		log.Printf("⚠️ No behaviors found for user %d", userID)
		return
	}

	// Count interactions by city
	cityScores := make(map[string]int)
	zoneScores := make(map[string]int)
	cityNames := make(map[string]string)
	zoneNames := make(map[string]string)
	cityIDs := make(map[string]*uint)
	zoneIDs := make(map[string]*uint)

	for _, behavior := range behaviors {
		// Weight different interaction types
		weight := 1
		switch behavior.InteractionType {
		case "view":
			weight = 1
		case "click":
			weight = 2
		case "favorite":
			weight = 5
		case "contact":
			weight = 10
		}

		// Add time spent bonus (1 point per 10 seconds)
		timeBonus := behavior.TimeSpent / 10

		score := weight + timeBonus

		if behavior.CityID != nil {
			cityKey := fmt.Sprintf("%d", *behavior.CityID)
			cityScores[cityKey] += score
			cityNames[cityKey] = behavior.CityName
			cityIDs[cityKey] = behavior.CityID
		}

		if behavior.ZoneID != nil {
			zoneKey := fmt.Sprintf("%d", *behavior.ZoneID)
			zoneScores[zoneKey] += score
			zoneNames[zoneKey] = behavior.ZoneName
			zoneIDs[zoneKey] = behavior.ZoneID
		}
	}

	// Find top city
	var topCityKey string
	var topCityScore int
	for key, score := range cityScores {
		if score > topCityScore {
			topCityScore = score
			topCityKey = key
		}
	}

	// Find top zone
	var topZoneKey string
	var topZoneScore int
	for key, score := range zoneScores {
		if score > topZoneScore {
			topZoneScore = score
			topZoneKey = key
		}
	}

	// Update user's favorite city if we found a clear winner (at least 5 interactions)
	if topCityScore >= 5 {
		var user models.User
		if err := storage.DB.First(&user, userID).Error; err != nil {
			log.Printf("❌ Error fetching user %d: %v", userID, err)
			return
		}

		user.FavoriteCityID = cityIDs[topCityKey]
		user.FavoriteCityName = cityNames[topCityKey]

		if topZoneKey != "" && topZoneScore >= 3 {
			user.FavoriteZoneID = zoneIDs[topZoneKey]
			user.FavoriteZoneName = zoneNames[topZoneKey]
		}

		if err := storage.DB.Save(&user).Error; err != nil {
			log.Printf("❌ Error updating favorite city for user %d: %v", userID, err)
		} else {
			log.Printf("✅ Updated favorite city for user %d: %s (score: %d, behaviors: %d)", userID, cityNames[topCityKey], topCityScore, len(behaviors))
		}
	} else {
		log.Printf("⚠️ Not enough interactions for user %d (score: %d, need: 5)", userID, topCityScore)
	}
}

// inferAndUpdateFavoriteCityForAnonymous analyzes behavior for anonymous users
func inferAndUpdateFavoriteCityForAnonymous(deviceID string, phoneNumber string) {
	if deviceID == "" {
		return
	}

	// Get all behaviors from last 30 days for this device
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	
	var behaviors []models.UserBehavior
	query := storage.DB.Where("device_id = ? AND timestamp >= ?", deviceID, thirtyDaysAgo)
	if phoneNumber != "" {
		// Also include behaviors with same phone number
		query = storage.DB.Where("(device_id = ? OR phone_number = ?) AND timestamp >= ?", deviceID, phoneNumber, thirtyDaysAgo)
	}
	
	if err := query.Find(&behaviors).Error; err != nil {
		log.Printf("❌ Error fetching behaviors for device %s: %v", deviceID, err)
		return
	}

	log.Printf("📊 Analyzing %d behaviors for anonymous device %s", len(behaviors), deviceID)

	if len(behaviors) == 0 {
		return
	}

	// Count interactions by city (same logic as logged-in users)
	cityScores := make(map[string]int)
	zoneScores := make(map[string]int)
	cityNames := make(map[string]string)
	zoneNames := make(map[string]string)
	cityIDs := make(map[string]*uint)
	zoneIDs := make(map[string]*uint)

	for _, behavior := range behaviors {
		// Weight different interaction types
		weight := 1
		switch behavior.InteractionType {
		case "view":
			weight = 1
		case "click":
			weight = 2
		case "favorite":
			weight = 5
		case "contact":
			weight = 10
		}

		// Add time spent bonus (1 point per 10 seconds)
		timeBonus := behavior.TimeSpent / 10
		score := weight + timeBonus

		if behavior.CityID != nil {
			cityKey := fmt.Sprintf("%d", *behavior.CityID)
			cityScores[cityKey] += score
			cityNames[cityKey] = behavior.CityName
			cityIDs[cityKey] = behavior.CityID
		}

		if behavior.ZoneID != nil {
			zoneKey := fmt.Sprintf("%d", *behavior.ZoneID)
			zoneScores[zoneKey] += score
			zoneNames[zoneKey] = behavior.ZoneName
			zoneIDs[zoneKey] = behavior.ZoneID
		}
	}

	// Find top city
	var topCityKey string
	var topCityScore int
	for key, score := range cityScores {
		if score > topCityScore {
			topCityScore = score
			topCityKey = key
		}
	}

	// Find top zone
	var topZoneKey string
	var topZoneScore int
	for key, score := range zoneScores {
		if score > topZoneScore {
			topZoneScore = score
			topZoneKey = key
		}
	}

	// Update anonymous user preference if we found a clear winner (at least 3 interactions for anonymous)
	if topCityScore >= 3 {
		var phonePtr *string
		if phoneNumber != "" {
			phonePtr = &phoneNumber
		}

		var pref models.AnonymousUserPreference
		if err := storage.DB.Where("device_id = ?", deviceID).First(&pref).Error; err != nil {
			// Create new preference
			pref = models.AnonymousUserPreference{
				DeviceID:        deviceID,
				PhoneNumber:     phonePtr,
				FavoriteCityID:  cityIDs[topCityKey],
				FavoriteCityName: cityNames[topCityKey],
				FavoriteZoneID:  nil,
				FavoriteZoneName: "",
				LastActive:      time.Now(),
			}
			if topZoneKey != "" && topZoneScore >= 2 {
				pref.FavoriteZoneID = zoneIDs[topZoneKey]
				pref.FavoriteZoneName = zoneNames[topZoneKey]
			}
			if err := storage.DB.Create(&pref).Error; err != nil {
				log.Printf("❌ Error creating AnonymousUserPreference: %v", err)
			} else {
				log.Printf("✅ Created AnonymousUserPreference for device %s: %s (score: %d, behaviors: %d)", 
					deviceID, cityNames[topCityKey], topCityScore, len(behaviors))
			}
		} else {
			// Update existing preference
			pref.FavoriteCityID = cityIDs[topCityKey]
			pref.FavoriteCityName = cityNames[topCityKey]
			if phonePtr != nil {
				pref.PhoneNumber = phonePtr
			}
			if topZoneKey != "" && topZoneScore >= 2 {
				pref.FavoriteZoneID = zoneIDs[topZoneKey]
				pref.FavoriteZoneName = zoneNames[topZoneKey]
			}
			pref.LastActive = time.Now()
			if err := storage.DB.Save(&pref).Error; err != nil {
				log.Printf("❌ Error updating anonymous preference: %v", err)
			} else {
				log.Printf("✅ Updated AnonymousUserPreference for device %s: %s (score: %d, behaviors: %d)", 
					deviceID, cityNames[topCityKey], topCityScore, len(behaviors))
			}
		}
	}
}

// GetBehaviorStats returns statistics about stored behaviors (for debugging)
func GetBehaviorStats(ctx iris.Context) {
	var totalBehaviors int64
	var loggedInBehaviors int64
	var anonymousBehaviors int64
	var recentBehaviors int64
	var totalAnonymousPrefs int64
	var recentAnonymousPrefs int64

	storage.DB.Model(&models.UserBehavior{}).Count(&totalBehaviors)
	storage.DB.Model(&models.UserBehavior{}).Where("user_id IS NOT NULL").Count(&loggedInBehaviors)
	storage.DB.Model(&models.UserBehavior{}).Where("user_id IS NULL").Count(&anonymousBehaviors)
	storage.DB.Model(&models.UserBehavior{}).Where("timestamp >= ?", time.Now().AddDate(0, 0, -7)).Count(&recentBehaviors)
	
	// Count AnonymousUserPreference records
	storage.DB.Model(&models.AnonymousUserPreference{}).Count(&totalAnonymousPrefs)
	storage.DB.Model(&models.AnonymousUserPreference{}).Where("last_active >= ?", time.Now().AddDate(0, 0, -7)).Count(&recentAnonymousPrefs)

	// Get recent behaviors sample
	var recentSamples []models.UserBehavior
	storage.DB.Order("timestamp DESC").Limit(10).Find(&recentSamples)

	// Get recent AnonymousUserPreference samples
	var recentPrefs []models.AnonymousUserPreference
	storage.DB.Order("last_active DESC").Limit(10).Find(&recentPrefs)

	// Get city distribution
	type CityCount struct {
		CityName string
		Count    int64
	}
	var cityCounts []CityCount
	storage.DB.Model(&models.UserBehavior{}).
		Select("city_name, COUNT(*) as count").
		Where("city_name != ''").
		Group("city_name").
		Order("count DESC").
		Limit(10).
		Scan(&cityCounts)

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{
		"success": true,
		"stats": iris.Map{
			"total_behaviors":        totalBehaviors,
			"logged_in_behaviors":    loggedInBehaviors,
			"anonymous_behaviors":    anonymousBehaviors,
			"recent_behaviors_7d":    recentBehaviors,
			"recent_samples":         recentSamples,
			"top_cities":             cityCounts,
			"anonymous_preferences": iris.Map{
				"total":  totalAnonymousPrefs,
				"recent_7d": recentAnonymousPrefs,
				"recent_samples": recentPrefs,
			},
		},
	})
}
