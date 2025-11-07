package routes

import (
	"errors"
	"strings"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

type registerMarketingDeviceRequest struct {
	DeviceID       string `json:"deviceId" validate:"required"`
	FCMToken       string `json:"fcmToken" validate:"required"`
	MarketingOptIn *bool  `json:"marketingOptIn"`
	Locale         string `json:"locale"`
	Timezone       string `json:"timezone"`
	Platform       string `json:"platform"`
	AppVersion     string `json:"appVersion"`
	SDKVersion     string `json:"sdkVersion"`
}

type linkMarketingDeviceRequest struct {
	DeviceID string `json:"deviceId" validate:"required"`
}

func RegisterMarketingDevice(ctx iris.Context) {
	var req registerMarketingDeviceRequest
	if err := ctx.ReadJSON(&req); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	fcmToken := strings.TrimSpace(req.FCMToken)
	if deviceID == "" || fcmToken == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "deviceId and fcmToken are required"})
		return
	}

	marketingOptIn := true
	if req.MarketingOptIn != nil {
		marketingOptIn = *req.MarketingOptIn
	}

	var device models.MarketingDevice
	result := storage.DB.Where("device_id = ?", deviceID).First(&device)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		now := time.Now()
		nextSend := now.Add(6 * time.Hour)
		device = models.MarketingDevice{
			DeviceID:       deviceID,
			FCMToken:       fcmToken,
			MarketingOptIn: marketingOptIn,
			Locale:         strings.TrimSpace(req.Locale),
			Timezone:       strings.TrimSpace(req.Timezone),
			Platform:       strings.TrimSpace(req.Platform),
			AppVersion:     strings.TrimSpace(req.AppVersion),
			SDKVersion:     strings.TrimSpace(req.SDKVersion),
			NextSendAt:     &nextSend,
		}

		if err := storage.DB.Create(&device).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"success": false, "error": "failed to register device"})
			return
		}
	} else if result.Error == nil {
		updates := map[string]interface{}{
			"fcm_token":        fcmToken,
			"marketing_opt_in": marketingOptIn,
			"locale":           strings.TrimSpace(req.Locale),
			"timezone":         strings.TrimSpace(req.Timezone),
			"platform":         strings.TrimSpace(req.Platform),
			"app_version":      strings.TrimSpace(req.AppVersion),
			"sdk_version":      strings.TrimSpace(req.SDKVersion),
		}

		if strings.TrimSpace(req.FCMToken) != "" && req.FCMToken != device.FCMToken {
			nextSend := time.Now().Add(6 * time.Hour)
			updates["next_send_at"] = nextSend
		}

		if !marketingOptIn {
			updates["next_send_at"] = nil
		}

		if err := storage.DB.Model(&device).Updates(updates).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"success": false, "error": "failed to update device"})
			return
		}
	} else {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "failed to query device"})
		return
	}

	ctx.JSON(iris.Map{"success": true})
}

func LinkMarketingDeviceToUser(ctx iris.Context) {
	userIDValue := ctx.Values().Get("userID")
	userID, ok := userIDValue.(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"success": false, "error": "unauthorized"})
		return
	}

	var req linkMarketingDeviceRequest
	if err := ctx.ReadJSON(&req); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"success": false, "error": "deviceId is required"})
		return
	}

	var device models.MarketingDevice
	if err := storage.DB.Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.StatusCode(iris.StatusNotFound)
			ctx.JSON(iris.Map{"success": false, "error": "device not found"})
			return
		}
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "failed to query device"})
		return
	}

	updates := map[string]interface{}{
		"user_id":          userID,
		"marketing_opt_in": true,
	}

	if device.NextSendAt == nil {
		next := time.Now().Add(6 * time.Hour)
		updates["next_send_at"] = next
	}

	if err := storage.DB.Model(&device).Updates(updates).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"success": false, "error": "failed to link device"})
		return
	}

	ctx.JSON(iris.Map{"success": true})
}
