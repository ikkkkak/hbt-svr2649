package routes

import (
	"apartments-clone-server/models"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func firstLandmarkImageURL(landmark models.Landmark) string {
	if len(landmark.Images) == 0 {
		return ""
	}
	var imgs []string
	if err := json.Unmarshal(landmark.Images, &imgs); err != nil {
		return ""
	}
	for _, u := range imgs {
		if strings.TrimSpace(u) != "" {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

// triggerLandmarkInvestmentOpportunityNotification sends a targeted push to devices
// explicitly interested in investment opportunities.
func triggerLandmarkInvestmentOpportunityNotification(landmark models.Landmark) {
	var prefs []models.AnonymousUserPreference
	if err := storage.DB.Where("last_active >= ?", time.Now().AddDate(0, -3, 0)).Find(&prefs).Error; err != nil {
		log.Printf("⚠️ triggerLandmarkInvestmentOpportunityNotification: failed loading preferences: %v", err)
		return
	}
	if len(prefs) == 0 {
		return
	}

	interestedDevice := make(map[string]struct{})
	for _, p := range prefs {
		if strings.TrimSpace(p.DeviceID) == "" {
			continue
		}
		for _, it := range p.Interests {
			if strings.EqualFold(strings.TrimSpace(it), "investment") {
				interestedDevice[p.DeviceID] = struct{}{}
				break
			}
		}
	}
	if len(interestedDevice) == 0 {
		return
	}

	var devices []models.MarketingDevice
	if err := storage.DB.
		Where("marketing_opt_in = true AND fcm_token != ''").
		Find(&devices).Error; err != nil {
		log.Printf("⚠️ triggerLandmarkInvestmentOpportunityNotification: failed loading marketing devices: %v", err)
		return
	}

	var tokens []string
	now := time.Now()
	allowedDeviceIDs := make([]uint, 0, len(devices))
	for _, d := range devices {
		if _, ok := interestedDevice[d.DeviceID]; !ok {
			continue
		}
		// Per-device anti-spam: never send marketing/investment bursts in the same minute.
		if d.NextSendAt != nil && now.Before(*d.NextSendAt) {
			continue
		}
		if d.LastSentAt != nil && now.Sub(*d.LastSentAt) < time.Minute {
			continue
		}
		t := strings.TrimSpace(d.FCMToken)
		if t == "" {
			continue
		}
		tokens = append(tokens, t)
		allowedDeviceIDs = append(allowedDeviceIDs, d.ID)
	}
	if len(tokens) == 0 {
		return
	}

	imageURL := firstLandmarkImageURL(landmark)
	title := "Prime Land Investment Alert"
	body := fmt.Sprintf("New investment opportunity: %s. Tap to inspect this landmark now.", strings.TrimSpace(landmark.Title))
	data := map[string]string{
		"type":       "landmark_investment_opportunity",
		"screen":     "LandmarkDetails",
		"landmarkId": strconv.FormatUint(uint64(landmark.ID), 10),
	}
	if err := pushsvc.SendExpoPushWithImage(tokens, title, body, imageURL, data); err != nil {
		log.Printf("⚠️ triggerLandmarkInvestmentOpportunityNotification: push failed: %v", err)
		return
	}
	// Record send timestamps to prevent duplicate bursts to the same device.
	next := now.Add(time.Minute)
	if len(allowedDeviceIDs) > 0 {
		_ = storage.DB.Model(&models.MarketingDevice{}).
			Where("id IN ?", allowedDeviceIDs).
			Updates(map[string]interface{}{
				"last_sent_at": now,
				"next_send_at": next,
			}).Error
	}
}

// AdminUpdateLandmark updates mutable admin-only fields for a landmark.
func AdminUpdateLandmark(ctx iris.Context) {
	landmarkID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Full admin edit: every field is optional (pointer) so we only update what
	// was sent. Images/Sides accept a string array and are stored as JSON.
	var input struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Price       *float64 `json:"price"`
		Area        *float64 `json:"area"`
		AreaUnit    *string  `json:"area_unit"`
		LandType    *string  `json:"land_type"`
		Zoning      *string  `json:"zoning"`
		District    *string  `json:"district"`
		Region      *string  `json:"region"`
		PlotNumber  *string  `json:"plot_number"`
		Elevation   *float64 `json:"elevation_m"`
		VideoURL    *string  `json:"video_url"`
		MediaType   *string  `json:"media_type"`
		Images      *[]string `json:"images"`
		Sides       *[]string `json:"sides"`

		IsInvestmentOpportunity *bool `json:"is_investment_opportunity"`
		IsGoodDeal              *bool `json:"is_good_deal"`
		IsGold                  *bool `json:"is_gold"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	if input.Title != nil {
		landmark.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		landmark.Description = *input.Description
	}
	if input.Price != nil {
		landmark.Price = *input.Price
	}
	if input.Area != nil {
		landmark.Area = *input.Area
	}
	if input.AreaUnit != nil {
		landmark.AreaUnit = strings.TrimSpace(*input.AreaUnit)
	}
	if input.LandType != nil {
		landmark.LandType = strings.TrimSpace(*input.LandType)
	}
	if input.Zoning != nil {
		landmark.Zoning = strings.TrimSpace(*input.Zoning)
	}
	if input.District != nil {
		landmark.District = strings.TrimSpace(*input.District)
	}
	if input.Region != nil {
		landmark.Region = strings.TrimSpace(*input.Region)
	}
	if input.PlotNumber != nil {
		landmark.PlotNumber = strings.TrimSpace(*input.PlotNumber)
	}
	if input.Elevation != nil {
		landmark.ElevationMeters = *input.Elevation
	}
	if input.VideoURL != nil {
		v := strings.TrimSpace(*input.VideoURL)
		landmark.VideoURL = &v
	}
	if input.MediaType != nil {
		landmark.MediaType = strings.TrimSpace(*input.MediaType)
	}
	if input.Images != nil {
		if b, err := json.Marshal(*input.Images); err == nil {
			landmark.Images = datatypes.JSON(b)
		}
	}
	if input.Sides != nil {
		if b, err := json.Marshal(*input.Sides); err == nil {
			landmark.Sides = datatypes.JSON(b)
		}
	}

	wasInvestment := landmark.IsInvestmentOpportunity
	if input.IsInvestmentOpportunity != nil {
		landmark.IsInvestmentOpportunity = *input.IsInvestmentOpportunity
	}
	if input.IsGoodDeal != nil {
		landmark.IsGoodDeal = *input.IsGoodDeal
	}
	if input.IsGold != nil {
		landmark.IsGold = *input.IsGold
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark"})
		return
	}

	if !wasInvestment && landmark.IsInvestmentOpportunity && landmark.IsVerified && landmark.IsPublished && landmark.Status == "verified" {
		go triggerLandmarkInvestmentOpportunityNotification(landmark)
	}

	ctx.JSON(iris.Map{
		"message":  "Landmark updated successfully",
		"landmark": landmark,
	})
}

// AdminUpdateLandmarkCoordinates allows admin to update landmark coordinates
func AdminUpdateLandmarkCoordinates(ctx iris.Context) {
	landmarkID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	// Check if landmark exists
	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	var input struct {
		Point1Lat *float64 `json:"point1_lat"`
		Point1Lng *float64 `json:"point1_lng"`
		Point2Lat *float64 `json:"point2_lat"`
		Point2Lng *float64 `json:"point2_lng"`
		Point3Lat *float64 `json:"point3_lat"`
		Point3Lng *float64 `json:"point3_lng"`
		Point4Lat *float64 `json:"point4_lat"`
		Point4Lng *float64 `json:"point4_lng"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate and update coordinates
	if input.Point1Lat != nil {
		if *input.Point1Lat < -90 || *input.Point1Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point1_lat coordinate"})
			return
		}
		landmark.Point1Lat = input.Point1Lat
	}
	if input.Point1Lng != nil {
		if *input.Point1Lng < -180 || *input.Point1Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point1_lng coordinate"})
			return
		}
		landmark.Point1Lng = input.Point1Lng
	}
	if input.Point2Lat != nil {
		if *input.Point2Lat < -90 || *input.Point2Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point2_lat coordinate"})
			return
		}
		landmark.Point2Lat = input.Point2Lat
	}
	if input.Point2Lng != nil {
		if *input.Point2Lng < -180 || *input.Point2Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point2_lng coordinate"})
			return
		}
		landmark.Point2Lng = input.Point2Lng
	}
	if input.Point3Lat != nil {
		if *input.Point3Lat < -90 || *input.Point3Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point3_lat coordinate"})
			return
		}
		landmark.Point3Lat = input.Point3Lat
	}
	if input.Point3Lng != nil {
		if *input.Point3Lng < -180 || *input.Point3Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point3_lng coordinate"})
			return
		}
		landmark.Point3Lng = input.Point3Lng
	}
	if input.Point4Lat != nil {
		if *input.Point4Lat < -90 || *input.Point4Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point4_lat coordinate"})
			return
		}
		landmark.Point4Lat = input.Point4Lat
	}
	if input.Point4Lng != nil {
		if *input.Point4Lng < -180 || *input.Point4Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point4_lng coordinate"})
			return
		}
		landmark.Point4Lng = input.Point4Lng
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark coordinates"})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Landmark coordinates updated successfully",
		"landmark": landmark,
	})
}

// AdminSetLandmarkOrganization sets or clears landmark.organization_id (admin only).
func AdminSetLandmarkOrganization(ctx iris.Context) {
	landmarkID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	var raw map[string]json.RawMessage
	if err := ctx.ReadJSON(&raw); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	v, ok := raw["organization_id"]
	if !ok {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "organization_id is required (use null to clear)"})
		return
	}

	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	switch string(v) {
	case "null":
		landmark.OrganizationID = nil
	default:
		var oid uint
		if err := json.Unmarshal(v, &oid); err != nil || oid == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "organization_id must be a positive integer or null"})
			return
		}
		var org models.Organization
		if err := storage.DB.First(&org, oid).Error; err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Organization not found"})
			return
		}
		landmark.OrganizationID = &oid
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark organization"})
		return
	}

	if err := storage.DB.Preload("Organization").First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to reload landmark"})
		return
	}

	ctx.JSON(iris.Map{
		"message":  "Landmark organization updated successfully",
		"landmark": landmark,
	})
}

// AdminDeleteLandmark permanently deletes a landmark and related moderation/feed rows.
func AdminDeleteLandmark(ctx iris.Context) {
	landmarkID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	if err := storage.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("landmark_id = ?", landmarkID).Delete(&models.LandmarkVideoLike{}).Error; err != nil {
			return err
		}
		if err := tx.Where("landmark_id = ?", landmarkID).Delete(&models.LandmarkVideoSave{}).Error; err != nil {
			return err
		}
		if err := tx.Where("landmark_id = ?", landmarkID).Delete(&models.LandmarkReport{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Landmark{}, landmarkID).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to delete landmark"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": "Landmark deleted successfully"})
}

