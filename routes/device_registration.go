package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// RegisterDevice handles silent device registration for analytics
// This endpoint is public and doesn't require authentication
func RegisterDevice(ctx iris.Context) {
	var input struct {
		DeviceID    string `json:"deviceId" validate:"required"`
		DeviceModel string `json:"deviceModel"`
		DeviceType  string `json:"deviceType"`
		Platform    string `json:"platform" validate:"required"`
		OSVersion   string `json:"osVersion"`
		AppVersion  string `json:"appVersion"`
		UserID      *uint  `json:"userId"` // Optional - if user is logged in
	}

	if err := ctx.ReadJSON(&input); err != nil {
		log.Printf("❌ RegisterDevice: Invalid JSON input: %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid request body"})
		return
	}

	// Hash device ID for privacy (one-way hash)
	hashedDeviceID := hashDeviceID(input.DeviceID)

	// Get current timestamp
	now := time.Now().Unix()

	// Check if device already exists
	var existingDevice models.DeviceRegistration
	result := storage.DB.Where("device_id = ?", hashedDeviceID).First(&existingDevice)

	if result.Error != nil {
		// Device doesn't exist, create new registration
		newDevice := models.DeviceRegistration{
			DeviceID:    hashedDeviceID,
			DeviceModel: input.DeviceModel,
			DeviceType:  input.DeviceType,
			Platform:    input.Platform,
			OSVersion:   input.OSVersion,
			AppVersion:  input.AppVersion,
			FirstSeenAt: now,
			LastSeenAt:  now,
			UserID:      input.UserID,
			IsActive:    true,
		}

		if err := storage.DB.Create(&newDevice).Error; err != nil {
			log.Printf("❌ RegisterDevice: Failed to create device registration: %v", err)
			utils.CreateInternalServerError(ctx)
			return
		}

		log.Printf("✅ RegisterDevice: New device registered - Platform: %s, Model: %s", input.Platform, input.DeviceModel)
		ctx.JSON(iris.Map{
			"success": true,
			"message": "Device registered successfully",
		})
	} else {
		// Device exists, update last seen timestamp and user ID if provided
		updates := map[string]interface{}{
			"last_seen_at": now,
			"is_active":    true,
		}

		// Update user ID if provided and different
		if input.UserID != nil && existingDevice.UserID == nil {
			updates["user_id"] = input.UserID
		}

		// Update device info if provided (in case of app updates)
		if input.DeviceModel != "" {
			updates["device_model"] = input.DeviceModel
		}
		if input.OSVersion != "" {
			updates["os_version"] = input.OSVersion
		}
		if input.AppVersion != "" {
			updates["app_version"] = input.AppVersion
		}

		if err := storage.DB.Model(&existingDevice).Updates(updates).Error; err != nil {
			log.Printf("❌ RegisterDevice: Failed to update device registration: %v", err)
			utils.CreateInternalServerError(ctx)
			return
		}

		ctx.JSON(iris.Map{
			"success": true,
			"message": "Device updated successfully",
		})
	}
}

// GetDeviceAnalytics returns aggregated device analytics for admin dashboard
func GetDeviceAnalytics(ctx iris.Context) {
	// Get user ID from middleware (must be admin)
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetDeviceAnalytics: Unauthorized")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	// Check if user is admin (you can add your admin check logic here)
	// For now, we'll allow any authenticated user to view analytics

	type PlatformStats struct {
		Platform   string  `json:"platform"`
		Count      int64   `json:"count"`
		Percentage float64 `json:"percentage"`
	}

	type DeviceModelStats struct {
		DeviceModel string `json:"deviceModel"`
		Count       int64  `json:"count"`
		Platform    string `json:"platform"`
	}

	type TimeSeriesData struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}

	// Get total device count
	var totalDevices int64
	storage.DB.Model(&models.DeviceRegistration{}).Where("is_active = ?", true).Count(&totalDevices)

	// Get platform distribution
	var platformStats []PlatformStats
	storage.DB.Model(&models.DeviceRegistration{}).
		Select("platform, COUNT(*) as count").
		Where("is_active = ?", true).
		Group("platform").
		Scan(&platformStats)

	// Calculate percentages
	for i := range platformStats {
		if totalDevices > 0 {
			platformStats[i].Percentage = float64(platformStats[i].Count) / float64(totalDevices) * 100
		}
	}

	// Get top device models
	var deviceModelStats []DeviceModelStats
	storage.DB.Model(&models.DeviceRegistration{}).
		Select("device_model, platform, COUNT(*) as count").
		Where("is_active = ? AND device_model != ''", true).
		Group("device_model, platform").
		Order("count DESC").
		Limit(20).
		Scan(&deviceModelStats)

	// Get registration trends over time (last 30 days)
	var timeSeriesData []TimeSeriesData
	storage.DB.Raw(`
		SELECT 
			DATE(TO_TIMESTAMP(first_seen_at)) as date,
			COUNT(*) as count
		FROM device_registrations
		WHERE is_active = true 
			AND first_seen_at >= EXTRACT(EPOCH FROM NOW() - INTERVAL '30 days')
		GROUP BY DATE(TO_TIMESTAMP(first_seen_at))
		ORDER BY date ASC
	`).Scan(&timeSeriesData)

	// Get active devices count (seen in last 7 days)
	var activeDevices int64
	sevenDaysAgo := time.Now().AddDate(0, 0, -7).Unix()
	storage.DB.Model(&models.DeviceRegistration{}).
		Where("is_active = ? AND last_seen_at >= ?", true, sevenDaysAgo).
		Count(&activeDevices)

	// Get usage statistics
	type UsageStats struct {
		TotalSessions     int64   `json:"totalSessions"`
		TotalUsageSeconds int64   `json:"totalUsageSeconds"`
		AverageSessionSec float64 `json:"averageSessionSec"`
		TotalUsageHours   float64 `json:"totalUsageHours"`
		DailyAverageSec   float64 `json:"dailyAverageSec"`
		DailyAverageHours float64 `json:"dailyAverageHours"`
	}

	var usageStats UsageStats

	// Today's unique app opens (distinct devices active today).
	// Use BOTH session starts and device last_seen heartbeat because some
	// app launches may update registration before/without session logging.
	var todayUniqueDevices int64
	storage.DB.Raw(`
		SELECT COUNT(DISTINCT device_id)
		FROM (
			SELECT device_id
			FROM device_sessions
			WHERE session_start >= EXTRACT(EPOCH FROM DATE_TRUNC('day', NOW()))
			UNION
			SELECT device_id
			FROM device_registrations
			WHERE is_active = true
			  AND last_seen_at >= EXTRACT(EPOCH FROM DATE_TRUNC('day', NOW()))
		) AS today_devices
	`).Scan(&todayUniqueDevices)

	type TodayOpenedDevice struct {
		DeviceID   string `json:"deviceId"`
		DeviceModel string `json:"deviceModel"`
		Platform   string `json:"platform"`
		AppVersion string `json:"appVersion"`
		LastSeenAt int64  `json:"lastSeenAt"`
	}
	var todayOpenedDevices []TodayOpenedDevice
	storage.DB.Raw(`
		SELECT
			dr.device_id as device_id,
			COALESCE(NULLIF(dr.device_model, ''), 'Unknown') as device_model,
			COALESCE(NULLIF(dr.platform, ''), 'unknown') as platform,
			COALESCE(NULLIF(dr.app_version, ''), 'Unknown') as app_version,
			dr.last_seen_at as last_seen_at
		FROM device_registrations dr
		WHERE dr.is_active = true
			AND dr.device_id IN (
				SELECT device_id
				FROM (
					SELECT device_id
					FROM device_sessions
					WHERE session_start >= EXTRACT(EPOCH FROM DATE_TRUNC('day', NOW()))
					UNION
					SELECT device_id
					FROM device_registrations
					WHERE is_active = true
					  AND last_seen_at >= EXTRACT(EPOCH FROM DATE_TRUNC('day', NOW()))
				) AS today_devices
			)
		ORDER BY dr.last_seen_at DESC
		LIMIT 100
	`).Scan(&todayOpenedDevices)

	// Total completed sessions
	storage.DB.Model(&models.DeviceSession{}).
		Where("is_active = ? AND duration_sec IS NOT NULL", false).
		Count(&usageStats.TotalSessions)

	// Total usage time (sum of all session durations)
	var totalUsageResult struct {
		Total int64
	}
	storage.DB.Model(&models.DeviceSession{}).
		Select("COALESCE(SUM(duration_sec), 0) as total").
		Where("is_active = ? AND duration_sec IS NOT NULL", false).
		Scan(&totalUsageResult)
	usageStats.TotalUsageSeconds = totalUsageResult.Total
	usageStats.TotalUsageHours = float64(usageStats.TotalUsageSeconds) / 3600.0

	// Average session duration
	if usageStats.TotalSessions > 0 {
		usageStats.AverageSessionSec = float64(usageStats.TotalUsageSeconds) / float64(usageStats.TotalSessions)
	}

	// Daily average (last 30 days)
	var dailyAvgResult struct {
		Avg float64
	}
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Unix()
	storage.DB.Raw(`
		SELECT COALESCE(AVG(duration_sec), 0) as avg
		FROM device_sessions
		WHERE is_active = false 
			AND duration_sec IS NOT NULL
			AND session_start >= ?
	`, thirtyDaysAgo).Scan(&dailyAvgResult)
	usageStats.DailyAverageSec = dailyAvgResult.Avg
	usageStats.DailyAverageHours = dailyAvgResult.Avg / 3600.0

	// Get usage time series data (last 30 days)
	type UsageTimeSeries struct {
		Date              string  `json:"date"`
		TotalSessions     int64   `json:"totalSessions"`
		TotalUsageSec     int64   `json:"totalUsageSec"`
		TotalUsageHours   float64 `json:"totalUsageHours"`
		AverageSessionSec float64 `json:"averageSessionSec"`
	}

	var usageTimeSeries []UsageTimeSeries
	storage.DB.Raw(`
		SELECT 
			DATE(TO_TIMESTAMP(session_start)) as date,
			COUNT(*) as total_sessions,
			COALESCE(SUM(duration_sec), 0) as total_usage_sec,
			COALESCE(AVG(duration_sec), 0) as average_session_sec
		FROM device_sessions
		WHERE is_active = false 
			AND duration_sec IS NOT NULL
			AND session_start >= ?
		GROUP BY DATE(TO_TIMESTAMP(session_start))
		ORDER BY date ASC
	`, thirtyDaysAgo).Scan(&usageTimeSeries)

	// Convert to proper format
	var formattedUsageTimeSeries []map[string]interface{}
	for _, item := range usageTimeSeries {
		formattedUsageTimeSeries = append(formattedUsageTimeSeries, map[string]interface{}{
			"date":              item.Date,
			"totalSessions":     item.TotalSessions,
			"totalUsageSec":     item.TotalUsageSec,
			"totalUsageHours":   float64(item.TotalUsageSec) / 3600.0,
			"averageSessionSec": item.AverageSessionSec,
		})
	}

	ctx.JSON(iris.Map{
		"success": true,
		"analytics": iris.Map{
			"totalDevices":     totalDevices,
			"activeDevices":    activeDevices,
			"todayUniqueDevices": todayUniqueDevices,
			"todayOpenedDevices": todayOpenedDevices,
			"todayDate":        time.Now().Format("2006-01-02"),
			"generatedAt":      time.Now().UTC().Format(time.RFC3339),
			"platformStats":    platformStats,
			"deviceModelStats": deviceModelStats,
			"timeSeriesData":   timeSeriesData,
			"usageStats":       usageStats,
			"usageTimeSeries":  formattedUsageTimeSeries,
		},
	})
}

// StartDeviceSession records when a user opens the app (session start)
func StartDeviceSession(ctx iris.Context) {
	var input struct {
		DeviceID   string `json:"deviceId" validate:"required"`
		UserID     *uint  `json:"userId"` // Optional - if user is logged in
		AppVersion string `json:"appVersion"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		log.Printf("❌ StartDeviceSession: Invalid JSON input: %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid request body"})
		return
	}

	// Hash device ID for privacy
	hashedDeviceID := hashDeviceID(input.DeviceID)

	// Get current timestamp
	now := time.Now().Unix()

	// Create new session
	session := models.DeviceSession{
		DeviceID:     hashedDeviceID,
		SessionStart: now,
		SessionEnd:   nil,
		DurationSec:  nil,
		UserID:       input.UserID,
		IsActive:     true,
		AppVersion:   input.AppVersion,
	}

	if err := storage.DB.Create(&session).Error; err != nil {
		log.Printf("❌ StartDeviceSession: Failed to create session: %v", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":   true,
		"sessionId": session.ID,
		"message":   "Session started",
	})
}

// EndDeviceSession records when a user closes the app (session end)
func EndDeviceSession(ctx iris.Context) {
	var input struct {
		DeviceID  string `json:"deviceId" validate:"required"`
		SessionID *uint  `json:"sessionId"` // Optional - specific session ID to end
		UserID    *uint  `json:"userId"`    // Optional - if user is logged in
	}

	if err := ctx.ReadJSON(&input); err != nil {
		log.Printf("❌ EndDeviceSession: Invalid JSON input: %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid request body"})
		return
	}

	// Hash device ID for privacy
	hashedDeviceID := hashDeviceID(input.DeviceID)

	// Get current timestamp
	now := time.Now().Unix()

	// Find active session(s) to end
	var sessions []models.DeviceSession
	query := storage.DB.Where("device_id = ? AND is_active = ?", hashedDeviceID, true)

	if input.SessionID != nil {
		query = query.Where("id = ?", *input.SessionID)
	}
	if input.UserID != nil {
		query = query.Where("user_id = ?", *input.UserID)
	}

	if err := query.Find(&sessions).Error; err != nil {
		log.Printf("❌ EndDeviceSession: Failed to find sessions: %v", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// End all matching active sessions
	for _, session := range sessions {
		duration := now - session.SessionStart
		session.SessionEnd = &now
		session.DurationSec = &duration
		session.IsActive = false

		if err := storage.DB.Save(&session).Error; err != nil {
			log.Printf("❌ EndDeviceSession: Failed to update session %d: %v", session.ID, err)
			continue
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Session(s) ended",
		"ended":   len(sessions),
	})
}

// GetDeviceDailyUsage returns detailed daily usage statistics per device
// This provides precise tracking: visits per day, usage time per day for each device
func GetDeviceDailyUsage(ctx iris.Context) {
	// Get user ID from middleware (must be admin)
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetDeviceDailyUsage: Unauthorized")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	// Get date range from query params (default: last 30 days)
	days := ctx.URLParamIntDefault("days", 30)
	if days > 365 {
		days = 365 // Max 1 year
	}
	if days < 1 {
		days = 30
	}

	// Calculate date range
	startDate := time.Now().AddDate(0, 0, -days)
	startTimestamp := startDate.Unix()

	// Get total unique devices (installations) - each device counted once
	var totalUniqueDevices int64
	storage.DB.Model(&models.DeviceRegistration{}).
		Where("is_active = ?", true).
		Count(&totalUniqueDevices)

	// Get daily usage per device
	type DeviceDailyUsage struct {
		DeviceID     string  `json:"deviceId"`
		DeviceModel  string  `json:"deviceModel"`
		Platform     string  `json:"platform"`
		Date         string  `json:"date"`
		VisitCount   int64   `json:"visitCount"`   // Number of times app was opened on this day
		UsageSeconds int64   `json:"usageSeconds"` // Total usage time in seconds for this day
		UsageMinutes float64 `json:"usageMinutes"` // Total usage time in minutes
		UsageHours   float64 `json:"usageHours"`   // Total usage time in hours
		SessionCount int64   `json:"sessionCount"` // Number of sessions on this day
	}

	var dailyUsage []DeviceDailyUsage
	storage.DB.Raw(`
		SELECT 
			ds.device_id as device_id,
			COALESCE(dr.device_model, 'Unknown') as device_model,
			COALESCE(dr.platform, 'unknown') as platform,
			DATE(TO_TIMESTAMP(ds.session_start)) as date,
			COUNT(*) as session_count,
			COALESCE(SUM(ds.duration_sec), 0) as usage_seconds
		FROM device_sessions ds
		LEFT JOIN device_registrations dr ON ds.device_id = dr.device_id
		WHERE ds.session_start >= ?
			AND ds.is_active = false
			AND ds.duration_sec IS NOT NULL
		GROUP BY ds.device_id, dr.device_model, dr.platform, DATE(TO_TIMESTAMP(ds.session_start))
		ORDER BY date DESC, usage_seconds DESC
	`, startTimestamp).Scan(&dailyUsage)

	// Calculate usage in minutes and hours
	// Visit count = session count (each session is a visit/app open)
	for i := range dailyUsage {
		dailyUsage[i].UsageMinutes = float64(dailyUsage[i].UsageSeconds) / 60.0
		dailyUsage[i].UsageHours = float64(dailyUsage[i].UsageSeconds) / 3600.0
		// Visit count equals session count (each session is one app open/visit)
		dailyUsage[i].VisitCount = dailyUsage[i].SessionCount
	}

	// Get aggregated device summary (total stats per device across all days)
	type DeviceSummary struct {
		DeviceID          string  `json:"deviceId"`
		DeviceModel       string  `json:"deviceModel"`
		Platform          string  `json:"platform"`
		TotalVisits       int64   `json:"totalVisits"`       // Total sessions across all days
		TotalUsageSec     int64   `json:"totalUsageSec"`     // Total usage time in seconds
		TotalUsageHours   float64 `json:"totalUsageHours"`   // Total usage time in hours
		DaysActive        int64   `json:"daysActive"`        // Number of days device was used
		AverageDailySec   float64 `json:"averageDailySec"`   // Average usage per day in seconds
		AverageDailyHours float64 `json:"averageDailyHours"` // Average usage per day in hours
		FirstSeen         string  `json:"firstSeen"`         // First seen date
		LastSeen          string  `json:"lastSeen"`          // Last seen date
	}

	var deviceSummaries []DeviceSummary
	storage.DB.Raw(`
		SELECT 
			ds.device_id as device_id,
			COALESCE(dr.device_model, 'Unknown') as device_model,
			COALESCE(dr.platform, 'unknown') as platform,
			COUNT(*) as total_visits,
			COALESCE(SUM(ds.duration_sec), 0) as total_usage_sec,
			COUNT(DISTINCT DATE(TO_TIMESTAMP(ds.session_start))) as days_active,
			MIN(DATE(TO_TIMESTAMP(ds.session_start))) as first_seen,
			MAX(DATE(TO_TIMESTAMP(ds.session_start))) as last_seen
		FROM device_sessions ds
		LEFT JOIN device_registrations dr ON ds.device_id = dr.device_id
		WHERE ds.session_start >= ?
			AND ds.is_active = false
			AND ds.duration_sec IS NOT NULL
		GROUP BY ds.device_id, dr.device_model, dr.platform
		ORDER BY total_usage_sec DESC
	`, startTimestamp).Scan(&deviceSummaries)

	// Calculate averages
	for i := range deviceSummaries {
		deviceSummaries[i].TotalUsageHours = float64(deviceSummaries[i].TotalUsageSec) / 3600.0
		if deviceSummaries[i].DaysActive > 0 {
			deviceSummaries[i].AverageDailySec = float64(deviceSummaries[i].TotalUsageSec) / float64(deviceSummaries[i].DaysActive)
			deviceSummaries[i].AverageDailyHours = deviceSummaries[i].AverageDailySec / 3600.0
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data": iris.Map{
			"totalUniqueDevices": totalUniqueDevices,
			"dailyUsage":         dailyUsage,
			"deviceSummaries":    deviceSummaries,
			"dateRange": iris.Map{
				"startDate": startDate.Format("2006-01-02"),
				"endDate":   time.Now().Format("2006-01-02"),
				"days":      days,
			},
		},
	})
}

// hashDeviceID creates a SHA256 hash of the device ID for privacy
func hashDeviceID(deviceID string) string {
	hash := sha256.Sum256([]byte(deviceID))
	return hex.EncodeToString(hash[:])
}
