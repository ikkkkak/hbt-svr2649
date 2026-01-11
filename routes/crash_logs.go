package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// CreateCrashLog handles POST /api/crash-logs
// Public endpoint - no auth required (crashes can happen before login)
func CreateCrashLog(ctx iris.Context) {
	var payload struct {
		Error          string                 `json:"error"`
		Stack          string                 `json:"stack"`
		ComponentStack string                 `json:"component_stack"`
		Phase          string                 `json:"phase"`
		Screen         string                 `json:"screen"`
		Context        map[string]interface{} `json:"context"`
		DeviceInfo     struct {
			Platform    string `json:"platform"`
			OSVersion   string `json:"os_version"`
			DeviceModel string `json:"device_model"`
			AppVersion  string `json:"app_version"`
		} `json:"device_info"`
		IsFatal   bool   `json:"is_fatal"`
		CrashType string `json:"crash_type"`
	}

	if err := ctx.ReadJSON(&payload); err != nil {
		utils.SendError(ctx, iris.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate required fields
	if payload.Error == "" {
		utils.SendError(ctx, iris.StatusBadRequest, "Error message is required", nil)
		return
	}

	// Get user ID from token if available (optional)
	var userID *uint
	if token := ctx.GetHeader("Authorization"); token != "" {
		userIDFromToken := utils.GetUserIDFromToken(token)
		if userIDFromToken > 0 {
			userID = &userIDFromToken
		}
	}

	// Convert context to JSON string
	contextJSON, _ := json.Marshal(payload.Context)

	crashLog := models.CrashLog{
		Error:          payload.Error,
		Stack:          payload.Stack,
		ComponentStack: payload.ComponentStack,
		Phase:          payload.Phase,
		Screen:         payload.Screen,
		Context:        string(contextJSON),
		Platform:       payload.DeviceInfo.Platform,
		OSVersion:      payload.DeviceInfo.OSVersion,
		DeviceModel:    payload.DeviceInfo.DeviceModel,
		AppVersion:     payload.DeviceInfo.AppVersion,
		UserID:         userID,
		IsFatal:        payload.IsFatal,
		CrashType:      payload.CrashType,
	}

	if err := storage.DB.Create(&crashLog).Error; err != nil {
		utils.SendError(ctx, iris.StatusInternalServerError, "Failed to save crash log", err)
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Crash log saved successfully",
		"id":      crashLog.ID,
	})
}

// GetCrashLogs handles GET /api/admin/crash-logs
// Admin only - requires authentication
func GetCrashLogs(ctx iris.Context) {
	// Get query parameters
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 50)
	resolved := ctx.URLParam("resolved") // "true", "false", or empty for all
	platform := ctx.URLParam("platform")  // "ios", "android", or empty for all
	screen := ctx.URLParam("screen")      // Filter by screen name
	search := ctx.URLParam("search")      // Search in error message

	offset := (page - 1) * limit

	query := storage.DB.Model(&models.CrashLog{})

	// Apply filters
	if resolved == "true" {
		query = query.Where("is_resolved = ?", true)
	} else if resolved == "false" {
		query = query.Where("is_resolved = ?", false)
	}

	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	if screen != "" {
		query = query.Where("screen = ?", screen)
	}

	if search != "" {
		query = query.Where("error ILIKE ?", "%"+search+"%")
	}

	// Get total count
	var total int64
	query.Count(&total)

	// Get crash logs with pagination
	var crashLogs []models.CrashLog
	if err := query.
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&crashLogs).Error; err != nil {
		utils.SendError(ctx, iris.StatusInternalServerError, "Failed to fetch crash logs", err)
		return
	}

	// Parse context JSON for each log
	type CrashLogResponse struct {
		models.CrashLog
		ContextParsed map[string]interface{} `json:"context_parsed"`
	}

	response := make([]CrashLogResponse, len(crashLogs))
	for i, log := range crashLogs {
		response[i] = CrashLogResponse{
			CrashLog: log,
		}
		if log.Context != "" {
			json.Unmarshal([]byte(log.Context), &response[i].ContextParsed)
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
		"pagination": iris.Map{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (int(total) + limit - 1) / limit,
		},
	})
}

// GetCrashLog handles GET /api/admin/crash-logs/:id
// Admin only
func GetCrashLog(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.SendError(ctx, iris.StatusBadRequest, "Invalid crash log ID", err)
		return
	}

	var crashLog models.CrashLog
	if err := storage.DB.Preload("User").First(&crashLog, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.SendError(ctx, iris.StatusNotFound, "Crash log not found", err)
		} else {
			utils.SendError(ctx, iris.StatusInternalServerError, "Failed to fetch crash log", err)
		}
		return
	}

	// Parse context JSON
	var contextParsed map[string]interface{}
	if crashLog.Context != "" {
		json.Unmarshal([]byte(crashLog.Context), &contextParsed)
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data": iris.Map{
			"crash_log":      crashLog,
			"context_parsed": contextParsed,
		},
	})
}

// UpdateCrashLog handles PATCH /api/admin/crash-logs/:id
// Admin only - for marking as resolved
func UpdateCrashLog(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.SendError(ctx, iris.StatusBadRequest, "Invalid crash log ID", err)
		return
	}

	var payload struct {
		IsResolved bool   `json:"is_resolved"`
		Notes      string `json:"notes"`
	}

	if err := ctx.ReadJSON(&payload); err != nil {
		utils.SendError(ctx, iris.StatusBadRequest, "Invalid request body", err)
		return
	}

	var crashLog models.CrashLog
	if err := storage.DB.First(&crashLog, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.SendError(ctx, iris.StatusNotFound, "Crash log not found", err)
		} else {
			utils.SendError(ctx, iris.StatusInternalServerError, "Failed to fetch crash log", err)
		}
		return
	}

	// Get admin user ID
	userID := utils.GetUserIDFromContext(ctx)
	if userID == 0 {
		utils.SendError(ctx, iris.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	updates := make(map[string]interface{})
	if payload.IsResolved {
		now := time.Now()
		updates["is_resolved"] = true
		updates["resolved_at"] = &now
		updates["resolved_by"] = userID
	} else {
		updates["is_resolved"] = false
		updates["resolved_at"] = nil
		updates["resolved_by"] = nil
	}

	if payload.Notes != "" {
		updates["notes"] = payload.Notes
	}

	if err := storage.DB.Model(&crashLog).Updates(updates).Error; err != nil {
		utils.SendError(ctx, iris.StatusInternalServerError, "Failed to update crash log", err)
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Crash log updated successfully",
	})
}

// GetCrashLogStats handles GET /api/admin/crash-logs/stats
// Admin only - get statistics about crashes
func GetCrashLogStats(ctx iris.Context) {
	var stats struct {
		Total          int64 `json:"total"`
		Unresolved     int64 `json:"unresolved"`
		Fatal          int64 `json:"fatal"`
		ByPlatform     map[string]int64 `json:"by_platform"`
		ByScreen       map[string]int64 `json:"by_screen"`
		ByCrashType    map[string]int64 `json:"by_crash_type"`
		Last24Hours    int64 `json:"last_24_hours"`
		Last7Days      int64 `json:"last_7_days"`
		Last30Days     int64 `json:"last_30_days"`
	}

	storage.DB.Model(&models.CrashLog{}).Count(&stats.Total)
	storage.DB.Model(&models.CrashLog{}).Where("is_resolved = ?", false).Count(&stats.Unresolved)
	storage.DB.Model(&models.CrashLog{}).Where("is_fatal = ?", true).Count(&stats.Fatal)

	// Last 24 hours
	last24Hours := time.Now().Add(-24 * time.Hour)
	storage.DB.Model(&models.CrashLog{}).Where("created_at >= ?", last24Hours).Count(&stats.Last24Hours)

	// Last 7 days
	last7Days := time.Now().Add(-7 * 24 * time.Hour)
	storage.DB.Model(&models.CrashLog{}).Where("created_at >= ?", last7Days).Count(&stats.Last7Days)

	// Last 30 days
	last30Days := time.Now().Add(-30 * 24 * time.Hour)
	storage.DB.Model(&models.CrashLog{}).Where("created_at >= ?", last30Days).Count(&stats.Last30Days)

	// By platform
	stats.ByPlatform = make(map[string]int64)
	var platformStats []struct {
		Platform string
		Count    int64
	}
	storage.DB.Model(&models.CrashLog{}).
		Select("platform, COUNT(*) as count").
		Group("platform").
		Scan(&platformStats)
	for _, s := range platformStats {
		stats.ByPlatform[s.Platform] = s.Count
	}

	// By screen
	stats.ByScreen = make(map[string]int64)
	var screenStats []struct {
		Screen string
		Count  int64
	}
	storage.DB.Model(&models.CrashLog{}).
		Select("screen, COUNT(*) as count").
		Where("screen != ''").
		Group("screen").
		Scan(&screenStats)
	for _, s := range screenStats {
		stats.ByScreen[s.Screen] = s.Count
	}

	// By crash type
	stats.ByCrashType = make(map[string]int64)
	var typeStats []struct {
		CrashType string
		Count     int64
	}
	storage.DB.Model(&models.CrashLog{}).
		Select("crash_type, COUNT(*) as count").
		Where("crash_type != ''").
		Group("crash_type").
		Scan(&typeStats)
	for _, s := range typeStats {
		stats.ByCrashType[s.CrashType] = s.Count
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    stats,
	})
}
