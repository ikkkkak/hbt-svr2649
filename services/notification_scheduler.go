package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
)

// StartSmartNotificationScheduler runs a lightweight hourly decision engine.
// It focuses on engagement notifications (continue browsing, trending, digest)
// and relies on NotificationDeliveryLog for anti-spam.
func StartSmartNotificationScheduler() {
	if storage.DB == nil {
		log.Printf("⚠️ Smart notification scheduler disabled: DB is nil")
		return
	}

	log.Printf("🚀 Starting Smart Notification Scheduler...")
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		runSmartNotificationTick(time.Now())
		for range ticker.C {
			runSmartNotificationTick(time.Now())
		}
	}()
}

func runSmartNotificationTick(now time.Time) {
	ns := NotificationServiceInstance
	// Meskeny discovery engine: localized spotlight pushes (papers + investment flags + deep links).
	if (now.Hour() >= 12 && now.Hour() <= 13) || (now.Hour() >= 17 && now.Hour() <= 20) {
		ns.SendDiscoverySpotlightForUsers(now, 40)
	}
	ns.sendContinueBrowsingNotifications(now)
	// Backfill for newly published properties within the last ~2 days, so users
	// also get strong matches that were added hours ago (not only "just now").
	if now.Hour() >= 11 && now.Hour() <= 12 {
		ns.sendRecentPropertyBackfillNotifications(now)
	}
	if now.Hour() >= 19 && now.Hour() <= 21 {
		ns.sendRecentPropertyBackfillNotifications(now)
	}

	// Nearby pushes in both morning + evening slots.
	if (now.Hour() >= 7 && now.Hour() <= 9) || (now.Hour() >= 18 && now.Hour() <= 21) {
		ns.sendNearbyNotifications(now)
	}

 	// Rent suggestions: match approved rent properties to user interests.
 	// Run in high-attention windows to maximize engagement.
 	if (now.Hour() >= 12 && now.Hour() <= 13) || (now.Hour() >= 18 && now.Hour() <= 21) {
 		ns.sendRentSuggestionNotifications(now)
 	}

	// Weekend trend push: Saturday morning.
	if now.Weekday() == time.Saturday && now.Hour() >= 8 && now.Hour() <= 11 {
		ns.sendTrendingNotifications(now)
	}

	// Weekly digest: Friday evening or Sunday evening.
	if (now.Weekday() == time.Friday || now.Weekday() == time.Sunday) && now.Hour() >= 18 && now.Hour() <= 21 {
		ns.sendWeeklyDigestNotifications(now)
	}
}

func (ns *NotificationService) sendRentSuggestionNotifications(now time.Time) {
 	// Pick a recent approved rent property per user based on favorite city/zone.
 	// Uses the same smart_* anti-spam gate as the rest of the scheduler.
 	var users []models.User
 	if err := storage.DB.Where("allows_notifications = ?", true).Limit(400).Find(&users).Error; err != nil {
 		return
 	}

 	for _, u := range users {
 		if !ns.canSendSmartType(u.ID, "smart_rent_suggestion", now) {
 			continue
 		}

 		// Prefer offline users (avoid annoying users currently in-app).
 		var reg models.DeviceRegistration
 		_ = storage.DB.Where("user_id = ?", u.ID).Order("last_seen_at DESC").First(&reg).Error
 		if !isOffline(reg.LastSeenAt, now) {
 			continue
 		}

 		q := storage.DB.Model(&models.Property{}).
 			Where("is_active = ?", true).
 			Where("LOWER(status) IN (?)", []string{"approved", "live", "published"}).
 			Order("created_at DESC").
 			Limit(14)

 		if u.FavoriteCityID != nil && *u.FavoriteCityID > 0 {
 			q = q.Where("city_id = ?", *u.FavoriteCityID)
 		}
 		if u.FavoriteZoneID != nil && *u.FavoriteZoneID > 0 {
 			q = q.Where("zone_id = ?", *u.FavoriteZoneID)
 		}

 		var candidates []models.Property
 		if err := q.Find(&candidates).Error; err != nil || len(candidates) == 0 {
 			continue
 		}

 		// Avoid repeats: skip properties we already pushed recently for this user/type.
 		var chosen *models.Property
 		for i := range candidates {
 			pid := uint(candidates[i].ID)
 			var last models.NotificationDeliveryLog
 			if err := storage.DB.
 				Where("user_id = ? AND event_type = ? AND property_kind = ? AND property_id = ?", u.ID, "smart_rent_suggestion", "rent", pid).
 				Order("created_at DESC").
 				First(&last).Error; err == nil {
 				if now.Sub(last.CreatedAt) < 10*24*time.Hour {
 					continue
 				}
 			}
 			chosen = &candidates[i]
 			break
 		}
 		if chosen == nil {
 			continue
 		}
		if ns.wasPropertyRecentlyPushedToUser(u.ID, uint(chosen.ID), now) {
			continue
		}

 		// Language: prefer notification_preferences.language; fallback to "en".
 		lang := "en"
 		var pref models.NotificationPreference
 		if err := storage.DB.
 			Where("user_id = ? AND enabled = ?", u.ID, true).
 			Order("updated_at DESC").
 			First(&pref).Error; err == nil {
 			if strings.TrimSpace(pref.Language) != "" {
 				lang = strings.ToLower(strings.TrimSpace(pref.Language))
 			}
 		}
 		if len(lang) > 2 {
 			lang = lang[:2]
 		}

 		// Localized copy (stable, no external MT dependency for push templates).
 		title := "🏠 Property for rent"
 		bodyPrefix := "Check this out"
 		if lang == "ar" {
 			title = "🏠 عقار للإيجار"
 			bodyPrefix = "شاهد هذا"
 		} else if lang == "fr" {
 			title = "🏠 Location disponible"
 			bodyPrefix = "À découvrir"
 		}

 		// Best-effort image from Property.Images JSON string.
 		imageURL := ""
 		if strings.TrimSpace(chosen.Images) != "" {
 			var imgs []string
 			if err := json.Unmarshal([]byte(chosen.Images), &imgs); err == nil && len(imgs) > 0 {
 				imageURL = strings.TrimSpace(imgs[0])
 			}
 		}

 		propTitle := strings.TrimSpace(chosen.Title)
 		if propTitle == "" {
 			propTitle = fmt.Sprintf("Property #%d", chosen.ID)
 		}

 		body := fmt.Sprintf("%s: %s", bodyPrefix, propTitle)
 		data := map[string]string{
 			"type":       "rent_suggestion",
 			"id":         fmt.Sprintf("%d", chosen.ID),
 			"propertyId": fmt.Sprintf("%d", chosen.ID),
 			"screen":     "PropertyDetails",
 			// IMPORTANT: app expects propertyID (not propertyId) for this screen.
 			"params": fmt.Sprintf(`{"propertyID": %d}`, chosen.ID),
 			"action": "view_property",
 		}

		if NotificationOrchestratorEnabled() {
			data["legacy_event_type"] = "smart_rent_suggestion"
			data["legacy_property_kind"] = "rent"
		}
		if ns.sendToUserWithImage(u.ID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				ns.logSmartDeliveryRent(u.ID, "smart_rent_suggestion", uint(chosen.ID), now)
			}
		}
 	}
}

func (ns *NotificationService) logSmartDeliveryRent(userID uint, eventType string, propertyID uint, now time.Time) {
 	timeWindow := now.Format("2006-01-02")
 	pid := propertyID
 	fp := models.BuildFingerprint(userID, "rent", &pid, nil, eventType, timeWindow)
 	entry := models.NotificationDeliveryLog{
 		UserID:       userID,
 		EventType:    eventType,
 		PropertyKind: "rent",
 		PropertyID:   &pid,
 		Fingerprint:  fp,
 		ABVariant:    ABVariantForUser(userID, eventType),
 	}
 	_ = storage.DB.Create(&entry).Error
}

func (ns *NotificationService) sendRecentPropertyBackfillNotifications(now time.Time) {
	// Look at homes that are recent but not "brand new right now".
	windowEnd := now.Add(-3 * time.Hour)
	windowStart := now.Add(-(48*time.Hour + 5*time.Hour))

	var properties []models.PropertySale
	if err := storage.DB.
		Where("status = ? AND is_published = ? AND created_at BETWEEN ? AND ?", "published", true, windowStart, windowEnd).
		Order("created_at DESC").
		Limit(8).
		Find(&properties).Error; err != nil || len(properties) == 0 {
		return
	}

	for _, p := range properties {
		// Skip if this property already generated this smart event recently.
		var recentDeliveries int64
		_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
			Where("event_type = ? AND property_sale_id = ? AND created_at >= ?", "smart_new_property_match", p.ID, now.Add(-24*time.Hour)).
			Count(&recentDeliveries).Error
		if recentDeliveries > 0 {
			continue
		}

		imageURL := ""
		if len(p.Images) > 0 {
			imageURL = p.Images[0]
		}

		_ = ns.SendNewPropertyNotification(
			p.ID,
			p.Title,
			p.CityID,
			p.City,
			p.ZoneID,
			"",
			p.Bedrooms,
			p.Bathrooms,
			p.SquareFootage,
			imageURL,
			p.CreatedAt,
		)
	}
}

func (ns *NotificationService) sendContinueBrowsingNotifications(now time.Time) {
	// Evening slot only for continue-browsing.
	if now.Hour() < 18 || now.Hour() > 21 {
		return
	}

	var users []models.User
	if err := storage.DB.Where("allows_notifications = ?", true).Limit(300).Find(&users).Error; err != nil {
		return
	}

	for _, u := range users {
		if !ns.canSendSmartType(u.ID, "smart_continue_browsing", now) {
			continue
		}

		// Skip very active users; we nudge only inactive users.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", u.ID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			continue
		}

		// Suggest one recent property from favorite city if possible, but
		// avoid boring repeats: skip properties seen in the last 10 days.
		q := storage.DB.Where("status = ? AND is_published = ?", "published", true).
			Order("created_at DESC").
			Limit(12)
		if u.FavoriteCityID != nil {
			q = q.Where("city_id = ?", *u.FavoriteCityID)
		}

		var candidates []models.PropertySale
		if err := q.Find(&candidates).Error; err != nil || len(candidates) == 0 {
			continue
		}

		candidateIDs := make([]uint, 0, len(candidates))
		for _, p := range candidates {
			candidateIDs = append(candidateIDs, p.ID)
		}

		seenMap := make(map[uint]bool, len(candidateIDs))
		seenCut := now.Add(-seenRecentlyCutoff)
		var seenRows []models.PropertyFeedSeen
		_ = storage.DB.
			Where("user_id = ? AND property_id IN ? AND seen_at >= ?", u.ID, candidateIDs, seenCut).
			Find(&seenRows).Error
		for _, s := range seenRows {
			// PropertyFeedSeen.UserID can be nil in some models; guard just in case.
			if s.UserID == nil {
				continue
			}
			seenMap[s.PropertyID] = true
		}

		var prop *models.PropertySale
		for i := range candidates {
			if seenMap[candidates[i].ID] {
				continue
			}
			prop = &candidates[i]
			break
		}
		if prop == nil {
			continue
		}
		if ns.wasPropertyRecentlyPushedToUser(u.ID, prop.ID, now) {
			continue
		}

		imageURL := ""
		if len(prop.Images) > 0 {
			imageURL = prop.Images[0]
		}

		title := "Continue where you left off"
		body := "New properties are waiting for you in your area."
		data := map[string]string{
			"type":       "continue_browsing",
			"id":         fmt.Sprintf("%d", prop.ID),
			"propertyId": fmt.Sprintf("%d", prop.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, prop.ID),
			"action":     "view_property",
		}

		if NotificationOrchestratorEnabled() {
			data["legacy_event_type"] = "smart_continue_browsing"
			data["legacy_property_kind"] = "sale"
		}
		if ns.sendToUserWithImage(u.ID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				ns.logSmartDelivery(u.ID, "smart_continue_browsing", &prop.ID, now)
			}
		}
	}
}

func (ns *NotificationService) sendTrendingNotifications(now time.Time) {
	// Top trending properties from last 24h interactions.
	type trendingRow struct {
		PropertySaleID uint `gorm:"column:property_sale_id"`
		Score          int  `gorm:"column:score"`
	}

	var rows []trendingRow
	cut := now.Add(-24 * time.Hour)
	storage.DB.Model(&models.Interaction{}).
		Select("property_sale_id, COUNT(*) as score").
		Where("property_sale_id IS NOT NULL AND created_at >= ?", cut).
		Group("property_sale_id").
		Order("score DESC").
		Limit(20).
		Scan(&rows)
	if len(rows) == 0 {
		return
	}

	// Candidate property ids from trending.
	candidateIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		candidateIDs = append(candidateIDs, r.PropertySaleID)
	}

	var users []models.User
	if err := storage.DB.Where("allows_notifications = ?", true).Limit(400).Find(&users).Error; err != nil {
		return
	}

	for _, u := range users {
		if !ns.canSendSmartType(u.ID, "smart_trending", now) {
			continue
		}

		// Avoid spamming users who are active in the app.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", u.ID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			continue
		}

		// Avoid boring repeats: skip properties seen in the last 10 days.
		seenMap := make(map[uint]bool, len(candidateIDs))
		cut := now.Add(-seenRecentlyCutoff)
		var seenRows []models.PropertyFeedSeen
		_ = storage.DB.
			Where("user_id = ? AND property_id IN ? AND seen_at >= ?", u.ID, candidateIDs, cut).
			Find(&seenRows).Error
		for _, s := range seenRows {
			if s.UserID == nil {
				continue
			}
			seenMap[s.PropertyID] = true
		}

		var selected models.PropertySale
		found := false
		for _, r := range rows {
			if seenMap[r.PropertySaleID] {
				continue
			}

			var p models.PropertySale
			if err := storage.DB.Where("id = ? AND status = ? AND is_published = ?", r.PropertySaleID, "published", true).First(&p).Error; err != nil {
				continue
			}
			if u.FavoriteCityID != nil && p.CityID != nil && *u.FavoriteCityID != *p.CityID {
				continue
			}
			selected = p
			found = true
			break
		}
		if !found {
			continue
		}
		if ns.wasPropertyRecentlyPushedToUser(u.ID, selected.ID, now) {
			continue
		}

		imageURL := ""
		if len(selected.Images) > 0 {
			imageURL = selected.Images[0]
		}

		title := "Trending in your area"
		body := "These properties are getting a lot of attention. Check them now."
		data := map[string]string{
			"type":       "trending_properties",
			"id":         fmt.Sprintf("%d", selected.ID),
			"propertyId": fmt.Sprintf("%d", selected.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, selected.ID),
			"action":     "view_property",
		}

		if NotificationOrchestratorEnabled() {
			data["legacy_event_type"] = "smart_trending"
			data["legacy_property_kind"] = "sale"
		}
		if ns.sendToUserWithImage(u.ID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				ns.logSmartDelivery(u.ID, "smart_trending", &selected.ID, now)
			}
		}
	}
}

func (ns *NotificationService) sendWeeklyDigestNotifications(now time.Time) {
	var users []models.User
	if err := storage.DB.Where("allows_notifications = ?", true).Limit(400).Find(&users).Error; err != nil {
		return
	}

	weekCut := now.AddDate(0, 0, -7)
	for _, u := range users {
		if !ns.canSendSmartType(u.ID, "smart_weekly_digest", now) {
			continue
		}

		// Avoid spamming users who are active in the app.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", u.ID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			continue
		}

		var props []models.PropertySale
		q := storage.DB.Where("status = ? AND is_published = ? AND created_at >= ?", "published", true, weekCut).
			Order("created_at DESC").
			Limit(3)
		if u.FavoriteCityID != nil {
			q = q.Where("city_id = ?", *u.FavoriteCityID)
		}
		if err := q.Find(&props).Error; err != nil || len(props) == 0 {
			continue
		}

		// Avoid boring repeats: skip properties seen in the last 10 days.
		candidateIDs := make([]uint, 0, len(props))
		for _, p := range props {
			candidateIDs = append(candidateIDs, p.ID)
		}

		seenMap := make(map[uint]bool, len(candidateIDs))
		cut := now.Add(-seenRecentlyCutoff)
		var seenRows []models.PropertyFeedSeen
		_ = storage.DB.
			Where("user_id = ? AND property_id IN ? AND seen_at >= ?", u.ID, candidateIDs, cut).
			Find(&seenRows).Error
		for _, s := range seenRows {
			if s.UserID == nil {
				continue
			}
			seenMap[s.PropertyID] = true
		}

		// Choose the first weekly-digest property not recently seen.
		var chosen *models.PropertySale
		for i := range props {
			if !seenMap[props[i].ID] {
				chosen = &props[i]
				break
			}
		}
		if chosen == nil {
			continue
		}
		if ns.wasPropertyRecentlyPushedToUser(u.ID, chosen.ID, now) {
			continue
		}

		imageURL := ""
		if len(chosen.Images) > 0 {
			imageURL = chosen.Images[0]
		}

		title := "Top properties this week"
		body := fmt.Sprintf("Best picks for you: %s", chosen.Title)
		data := map[string]string{
			"type":       "weekly_digest",
			"id":         fmt.Sprintf("%d", chosen.ID),
			"propertyId": fmt.Sprintf("%d", chosen.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, chosen.ID),
			"action":     "view_property",
		}

		if NotificationOrchestratorEnabled() {
			data["legacy_event_type"] = "smart_weekly_digest"
			data["legacy_property_kind"] = "sale"
		}
		if ns.sendToUserWithImage(u.ID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				ns.logSmartDelivery(u.ID, "smart_weekly_digest", &chosen.ID, now)
			}
		}
	}
}

func (ns *NotificationService) sendNearbyNotifications(now time.Time) {
	// Nearby event: distance < 5km, avoid showing repeats.
	const radiusKm = 5.0

	var users []models.User
	if err := storage.DB.Where("allows_notifications = ?", true).Limit(400).Find(&users).Error; err != nil {
		return
	}

	for _, u := range users {
		if !ns.canSendSmartType(u.ID, "smart_nearby", now) {
			continue
		}

		// Skip active users.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", u.ID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			continue
		}

		// Use server-stored GPS for this user (from location notification preferences).
		var pref models.NotificationPreference
		if err := storage.DB.Where("user_id = ? AND enabled = ?", u.ID, true).First(&pref).Error; err != nil {
			continue
		}

		// Rough bounding box for DB filtering.
		deltaLat := radiusKm / 111.0
		deltaLon := radiusKm / (111.0 * math.Cos(pref.Latitude*math.Pi/180.0))

		latMin := pref.Latitude - deltaLat
		latMax := pref.Latitude + deltaLat
		lonMin := pref.Longitude - deltaLon
		lonMax := pref.Longitude + deltaLon

		var props []models.PropertySale
		if err := storage.DB.Where(
			"status = ? AND is_published = ? AND latitude BETWEEN ? AND ? AND longitude BETWEEN ? AND ?",
			"published", true, latMin, latMax, lonMin, lonMax,
		).
			Order("created_at DESC").
			Limit(25).
			Find(&props).Error; err != nil || len(props) == 0 {
			continue
		}

		// Avoid boring repeats: skip properties seen in the last 10 days.
		candidateIDs := make([]uint, 0, len(props))
		for _, p := range props {
			candidateIDs = append(candidateIDs, p.ID)
		}

		seenMap := make(map[uint]bool, len(candidateIDs))
		seenCut := now.Add(-seenRecentlyCutoff)
		var seenRows []models.PropertyFeedSeen
		_ = storage.DB.Where("user_id = ? AND property_id IN ? AND seen_at >= ?", u.ID, candidateIDs, seenCut).Find(&seenRows).Error
		for _, s := range seenRows {
			if s.UserID == nil {
				continue
			}
			seenMap[s.PropertyID] = true
		}

		// Choose nearest unseen property (compute true distance in Go).
		var chosen *models.PropertySale
		var chosenDistKm float64
		for i := range props {
			p := &props[i]
			if seenMap[p.ID] {
				continue
			}
			if p.Latitude == 0 && p.Longitude == 0 {
				continue
			}
			d := haversineKm(pref.Latitude, pref.Longitude, p.Latitude, p.Longitude)
			if d > radiusKm {
				continue
			}
			if chosen == nil || d < chosenDistKm {
				chosen = p
				chosenDistKm = d
			}
		}
		if chosen == nil {
			continue
		}
		if ns.wasPropertyRecentlyPushedToUser(u.ID, chosen.ID, now) {
			continue
		}

		imageURL := ""
		if len(chosen.Images) > 0 {
			imageURL = chosen.Images[0]
		}

		title := "New property near you"
		body := fmt.Sprintf(
			"A new apartment is available within %.1f km of your location. Check \"%s\".",
			chosenDistKm,
			chosen.Title,
		)
		data := map[string]string{
			"type":       "nearby_property",
			"id":         fmt.Sprintf("%d", chosen.ID),
			"propertyId": fmt.Sprintf("%d", chosen.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, chosen.ID),
			"action":     "view_property",
		}

		if NotificationOrchestratorEnabled() {
			data["legacy_event_type"] = "smart_nearby"
			data["legacy_property_kind"] = "sale"
		}
		if ns.sendToUserWithImage(u.ID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				ns.logSmartDelivery(u.ID, "smart_nearby", &chosen.ID, now)
			}
		}
	}
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	// Haversine formula. Returns distance in kilometers.
	const earthKm = 6371.0
	lat1r := lat1 * math.Pi / 180.0
	lat2r := lat2 * math.Pi / 180.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1r)*math.Cos(lat2r)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthKm * c
}

func (ns *NotificationService) canSendSmartType(userID uint, eventType string, now time.Time) bool {
	// Per-user quiet hours + per-day budget (timezone-aware).
	//
	// Note: this function intentionally does a small DB lookup for user prefs,
	// because it is the single gate used by all smart_* notification paths.
	var pref models.NotificationPreference
	_ = storage.DB.
		Where("user_id = ? AND enabled = ?", userID, true).
		Order("updated_at DESC").
		First(&pref).Error

	quietStart := notificationSleepStartHour
	quietEnd := notificationSleepEndHour
	if pref.QuietStartHour >= 0 && pref.QuietStartHour <= 23 {
		quietStart = pref.QuietStartHour
	}
	if pref.QuietEndHour >= 0 && pref.QuietEndHour <= 23 {
		quietEnd = pref.QuietEndHour
	}

	localNow := now
	if strings.TrimSpace(pref.Timezone) != "" {
		if loc, err := time.LoadLocation(strings.TrimSpace(pref.Timezone)); err == nil && loc != nil {
			localNow = now.In(loc)
		}
	}
	// Quiet window spans midnight: [quietStart..24:00) U [00:00..quietEnd)
	h := localNow.Hour()
	if h >= quietStart || h < quietEnd {
		return false
	}

	maxPerDay := 2
	if pref.MaxSmartPerDay > 0 && pref.MaxSmartPerDay <= 20 {
		maxPerDay = pref.MaxSmartPerDay
	}

	// Global anti-spam budget: max N smart notifications/day.
	var cnt int64
	if err := storage.DB.Model(&models.NotificationDeliveryLog{}).
		Where("user_id = ? AND event_type LIKE ? AND created_at >= ?", userID, "smart_%", now.Add(-24*time.Hour)).
		Count(&cnt).Error; err == nil && cnt >= int64(maxPerDay) {
		return false
	}

	// “Last notification” rule:
	// If the user received any smart notification recently, don't send another
	// within 6 hours. This prevents confusing back-to-back messages.
	var lastAny models.NotificationDeliveryLog
	if err := storage.DB.Where("user_id = ? AND event_type LIKE ?", userID, "smart_%").
		Order("created_at DESC").
		First(&lastAny).Error; err == nil {
		if now.Sub(lastAny.CreatedAt) < 6*time.Hour {
			return false
		}
	}

	// Per-type cooldown: 24h.
	var last models.NotificationDeliveryLog
	if err := storage.DB.Where("user_id = ? AND event_type = ?", userID, eventType).
		Order("created_at DESC").
		First(&last).Error; err == nil {
		if now.Sub(last.CreatedAt) < 24*time.Hour {
			return false
		}
	}
	return true
}

// wasPropertyRecentlyPushedToUser prevents repeating the same sale listing
// across different notification campaigns for one user.
func (ns *NotificationService) wasPropertyRecentlyPushedToUser(userID uint, propertySaleID uint, now time.Time) bool {
	var cnt int64
	_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
		Where("user_id = ? AND property_sale_id = ? AND created_at >= ?", userID, propertySaleID, now.Add(-7*24*time.Hour)).
		Count(&cnt).Error
	return cnt > 0
}

// DeliverPushDirectToUser sends rich push immediately (bypasses orchestrator).
// Used by the orchestrator executor and when orchestrator is disabled.
func (ns *NotificationService) DeliverPushDirectToUser(userID uint, title, body, imageURL string, data map[string]string) bool {
	tokens, err := ns.getUserPushTokens(userID)
	if err != nil || len(tokens) == 0 {
		return false
	}
	if data == nil {
		data = map[string]string{}
	}
	if strings.TrimSpace(imageURL) != "" {
		data["imageURL"] = imageURL
		data["houseImage"] = imageURL
		data["propertyImage"] = imageURL
	}

	ok := false
	for _, token := range tokens {
		expoToken := token
		if strings.Contains(expoToken, "|") {
			expoToken = strings.Split(expoToken, "|")[0]
		}
		if err := utils.SendRichNotification(expoToken, title, body, imageURL, data); err == nil {
			ok = true
		}
	}
	return ok
}

func (ns *NotificationService) sendToUserWithImage(userID uint, title, body, imageURL string, data map[string]string) bool {
	if NotificationOrchestratorEnabled() {
		typ := "push"
		if data != nil {
			if t := strings.TrimSpace(data["type"]); t != "" {
				typ = t
			}
		}
		_, err := SubmitNotificationCandidate(NotificationCandidateInput{
			UserID:           userID,
			NotificationType: typ,
			Title:            title,
			Body:             body,
			ImageURL:         imageURL,
			Data:             data,
		})
		return err == nil
	}
	return ns.DeliverPushDirectToUser(userID, title, body, imageURL, data)
}

func (ns *NotificationService) logSmartDelivery(userID uint, eventType string, propertySaleID *uint, now time.Time) {
	timeWindow := now.Format("2006-01-02")
	fp := models.BuildFingerprint(userID, "sale", nil, propertySaleID, eventType, timeWindow)
	entry := models.NotificationDeliveryLog{
		UserID:         userID,
		EventType:      eventType,
		PropertyKind:   "sale",
		PropertySaleID: propertySaleID,
		Fingerprint:    fp,
		ABVariant:      ABVariantForUser(userID, eventType),
	}
	// Best effort, dedup-safe.
	_ = storage.DB.Create(&entry).Error
}

