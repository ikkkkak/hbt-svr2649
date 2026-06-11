package meskenyguide

import (
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

const (
	minHoursBetweenSameTrigger = 6
	maxCommentsPerListingPer6h = 1
	maxCommentsPerHostPerDay    = 3
	dismissPauseAfter           = 5
	dismissPauseDays            = 7
	categorySuppressDays        = 7
)

func isHostPaused(hostID, propertySaleID uint, now time.Time) bool {
	var pref models.GuideHostPreference
	if err := storage.DB.Where("host_id = ? AND property_sale_id = ?", hostID, propertySaleID).
		First(&pref).Error; err != nil {
		return false
	}
	if pref.PausedUntil != nil && now.Before(*pref.PausedUntil) {
		return true
	}
	return false
}

func isCategorySuppressed(hostID, propertySaleID uint, category string, now time.Time) bool {
	var pref models.GuideHostPreference
	if err := storage.DB.Where("host_id = ? AND property_sale_id = ?", hostID, propertySaleID).
		First(&pref).Error; err != nil {
		return false
	}
	if pref.SuppressedCategories == nil {
		return false
	}
	raw, ok := pref.SuppressedCategories[category]
	if !ok {
		return false
	}
	untilStr, ok := raw.(string)
	if !ok {
		return false
	}
	until, err := time.Parse(time.RFC3339, untilStr)
	if err != nil {
		return false
	}
	return now.Before(until)
}

func canCreateComment(hostID, propertySaleID uint, trigger string, now time.Time) bool {
	if isHostPaused(hostID, propertySaleID, now) {
		return false
	}

	since6h := now.Add(-minHoursBetweenSameTrigger * time.Hour)
	var recentSame int64
	storage.DB.Model(&models.GuideComment{}).
		Where("property_sale_id = ? AND trigger_event = ? AND parent_id IS NULL AND created_at >= ?",
			propertySaleID, trigger, since6h).
		Count(&recentSame)
	if recentSame > 0 {
		return false
	}

	var recentListing int64
	storage.DB.Model(&models.GuideComment{}).
		Where("property_sale_id = ? AND parent_id IS NULL AND created_at >= ?", propertySaleID, since6h).
		Count(&recentListing)
	if recentListing >= maxCommentsPerListingPer6h {
		return false
	}

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var hostToday int64
	storage.DB.Model(&models.GuideComment{}).
		Where("host_id = ? AND parent_id IS NULL AND created_at >= ?", hostID, dayStart).
		Count(&hostToday)
	return hostToday < maxCommentsPerHostPerDay
}

func recordDismiss(hostID, propertySaleID uint, category string, now time.Time) {
	var pref models.GuideHostPreference
	err := storage.DB.Where("host_id = ? AND property_sale_id = ?", hostID, propertySaleID).First(&pref).Error
	if err != nil {
		pref = models.GuideHostPreference{
			HostID:         hostID,
			PropertySaleID: propertySaleID,
			SuppressedCategories: models.JSONMap{},
		}
	}
	if pref.SuppressedCategories == nil {
		pref.SuppressedCategories = models.JSONMap{}
	}
	until := now.Add(categorySuppressDays * 24 * time.Hour).Format(time.RFC3339)
	pref.SuppressedCategories[category] = until
	pref.ConsecutiveDismissals++
	if pref.ConsecutiveDismissals >= dismissPauseAfter {
		pauseUntil := now.Add(dismissPauseDays * 24 * time.Hour)
		pref.PausedUntil = &pauseUntil
		pref.ConsecutiveDismissals = 0
	}
	pref.UpdatedAt = now
	storage.DB.Save(&pref)
}

func resetDismissStreak(hostID, propertySaleID uint, now time.Time) {
	storage.DB.Model(&models.GuideHostPreference{}).
		Where("host_id = ? AND property_sale_id = ?", hostID, propertySaleID).
		Updates(map[string]interface{}{
			"consecutive_dismissals": 0,
			"paused_until":           nil,
			"updated_at":             now,
		})
}
