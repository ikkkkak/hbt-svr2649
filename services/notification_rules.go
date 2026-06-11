package services

import (
	"apartments-clone-server/models"
	"time"
)

const (
	// ExperimentMeskenyPushCopyV1: A/B for AI-generated push title/body vs template (see maybeAICopy).
	ExperimentMeskenyPushCopyV1 = "meskeny_push_copy_v1"

	// Sleep window (server local time). No notifications during this period.
	notificationSleepStartHour = 22 // 10:00 PM
	notificationSleepEndHour   = 7  // 07:00 AM

	// Smart notification anti-spam tuning.
	maxNotificationsPer24hNewProperty = 2
	newPropertyScoreMin               = 70
	seenRecentlyCutoff                = 10 * 24 * time.Hour
	freshnessBonusWindow              = 7 * 24 * time.Hour

	// Offline threshold: if user hasn't been seen in the app for >= 24h.
	offlineThreshold = 24 * time.Hour
)

func isInSleepTime(now time.Time) bool {
	// Sleep window spans midnight: [22:00..24:00) U [00:00..07:00)
	h := now.Hour()
	return h >= notificationSleepStartHour || h < notificationSleepEndHour
}

// computeNewPropertyScore creates a relevance score for new property push.
// - Location match (favorite city/zone) contributes up to 40.
// - Unseen contributes up to 60 (not seen recently, i.e. older than seenRecentlyCutoff).
// - Freshness contributes up to 10 (created within freshnessBonusWindow).
func computeNewPropertyScore(
	u models.User,
	cityID, zoneID *uint,
	seenAt *time.Time,
	propertyCreatedAt time.Time,
	now time.Time,
) int {
	score := 0

	// Location relevance.
	if cityID != nil && u.FavoriteCityID != nil && *u.FavoriteCityID == *cityID {
		score += 40
	} else if zoneID != nil && u.FavoriteZoneID != nil && *u.FavoriteZoneID == *zoneID {
		// Zone match is slightly weaker than exact city match.
		score += 30
	}

	// Unseen / re-appearance.
	// If we've never shown this property recently, boost strongly.
	if seenAt == nil {
		score += 60
	} else if now.Sub(*seenAt) > seenRecentlyCutoff {
		score += 60
	}

	// Freshness.
	if now.Sub(propertyCreatedAt) <= freshnessBonusWindow {
		score += 10
	}

	return score
}

// ABVariantForUser returns a stable A/B bucket per user for an experiment key (analytics).
func ABVariantForUser(userID uint, experiment string) string {
	x := uint64(userID)*1315423911 + uint64(len(experiment))
	for _, c := range experiment {
		x = x*31 + uint64(c)
	}
	if x%2 == 0 {
		return "A"
	}
	return "B"
}

func isOffline(lastSeenAtUnix int64, now time.Time) bool {
	// If we don't have telemetry, treat as "offline enough" for engagement.
	if lastSeenAtUnix <= 0 {
		return true
	}
	return now.Sub(time.Unix(lastSeenAtUnix, 0)) >= offlineThreshold
}

func withinNewPropertyNotifBudget(count24h int) bool {
	return count24h < maxNotificationsPer24hNewProperty
}

