package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// GET /api/device/preferences?deviceId=...
// Public endpoint (anonymous). Used on app open to decide whether to show personalization modal.
func GetDevicePreferences(ctx iris.Context) {
	deviceID := strings.TrimSpace(ctx.URLParamDefault("deviceId", ""))
	if deviceID == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "deviceId is required"})
		return
	}

	var pref models.AnonymousUserPreference
	err := storage.DB.Where("device_id = ?", deviceID).First(&pref).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "failed to query preferences"})
		return
	}

	// Determine whether we have an Expo push token linked for this device.
	var md models.MarketingDevice
	hasPush := storage.DB.Where("device_id = ? AND marketing_opt_in = true AND fcm_token != ''", deviceID).First(&md).Error == nil

	if err == gorm.ErrRecordNotFound {
		ctx.JSON(iris.Map{
			"success": true,
			"data": iris.Map{
				"exists":       false,
				"hasPushToken": hasPush,
			},
		})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data": iris.Map{
			"exists":            true,
			"hasPushToken":      hasPush,
			"interests":         pref.Interests,
			"favorite_city_id":  pref.FavoriteCityID,
			"favorite_city_name": pref.FavoriteCityName,
			"favorite_zone_id":  pref.FavoriteZoneID,
			"favorite_zone_name": pref.FavoriteZoneName,
			"last_active":       pref.LastActive,
		},
	})
}

// PUT /api/device/preferences
// Public endpoint (anonymous). Upserts device preferences from onboarding/personalization modal.
func UpsertDevicePreferences(ctx iris.Context) {
	var req struct {
		DeviceID        string   `json:"deviceId"`
		Interests       []string `json:"interests"`
		FavoriteCityID  *uint    `json:"favorite_city_id"`
		FavoriteCityName string  `json:"favorite_city_name"`
		FavoriteZoneID  *uint    `json:"favorite_zone_id"`
		FavoriteZoneName string  `json:"favorite_zone_name"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "invalid json"})
		return
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "deviceId is required"})
		return
	}
	for i := range req.Interests {
		req.Interests[i] = strings.ToLower(strings.TrimSpace(req.Interests[i]))
	}

	now := time.Now()
	var pref models.AnonymousUserPreference
	err := storage.DB.Where("device_id = ?", deviceID).First(&pref).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "failed to query preferences"})
		return
	}

	if err == gorm.ErrRecordNotFound {
		pref = models.AnonymousUserPreference{
			DeviceID:         deviceID,
			Interests:        req.Interests,
			FavoriteCityID:   req.FavoriteCityID,
			FavoriteCityName: strings.TrimSpace(req.FavoriteCityName),
			FavoriteZoneID:   req.FavoriteZoneID,
			FavoriteZoneName: strings.TrimSpace(req.FavoriteZoneName),
			LastActive:       now,
		}
		if err := storage.DB.Create(&pref).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"success": false, "error": "failed to create preferences"})
			return
		}
	} else {
		updates := map[string]interface{}{
			"last_active": now,
		}
		// Only overwrite interests if provided (non-nil slice).
		if req.Interests != nil {
			interestsJSON, marshalErr := json.Marshal(req.Interests)
			if marshalErr != nil {
				ctx.StatusCode(http.StatusBadRequest)
				ctx.JSON(iris.Map{"success": false, "error": "invalid interests payload"})
				return
			}
			updates["interests"] = gorm.Expr("?::jsonb", string(interestsJSON))
		}
		if req.FavoriteCityID != nil {
			updates["favorite_city_id"] = req.FavoriteCityID
		}
		if strings.TrimSpace(req.FavoriteCityName) != "" {
			updates["favorite_city_name"] = strings.TrimSpace(req.FavoriteCityName)
		}
		if req.FavoriteZoneID != nil {
			updates["favorite_zone_id"] = req.FavoriteZoneID
		}
		if strings.TrimSpace(req.FavoriteZoneName) != "" {
			updates["favorite_zone_name"] = strings.TrimSpace(req.FavoriteZoneName)
		}
		if err := storage.DB.Model(&pref).Updates(updates).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"success": false, "error": "failed to update preferences"})
			return
		}
	}

	ctx.JSON(iris.Map{"success": true})
}

