package services

import (
	meskenyai "apartments-clone-server/MeskenyGPT/ai"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// Minimal scalable notification queue (Redis-backed).
// Architecture:
// - real-time events enqueue delayed jobs (ZSET) or immediate jobs (LIST)
// - a poller moves due delayed jobs -> ready queue
// - workers BLPOP ready queue and send notifications using NotificationService
//
// This is intentionally small and reliable; AI text generation can be plugged in later.

const (
	redisNotifJobsQueue   = "meskeny:notif:jobs"    // LIST (RPUSH/BLPOP)
	redisNotifDelayedZSet = "meskeny:notif:delayed" // ZSET (score=unix seconds; member=json)
)

const (
	reengageOfflineCutoff = 7 * 24 * time.Hour
)

type AINotificationJob struct {
	JobID        string `json:"job_id"`
	Kind         string `json:"kind"` // e.g. "sale_view_reminder"
	UserID       uint   `json:"user_id"`
	PropertyKind string `json:"property_kind"` // "sale" | "rent"
	PropertyID   *uint  `json:"property_id,omitempty"`
	PropertySale *uint  `json:"property_sale_id,omitempty"`

	// Scheduling / retries
	CreatedAtUnix int64 `json:"created_at_unix"`
	Attempt       int   `json:"attempt"`
}

func EnqueueAINotificationDelayed(job AINotificationJob, runAt time.Time) {
	if storage.Redis == nil {
		return
	}
	if job.JobID == "" {
		job.JobID = fmt.Sprintf("job_%d_%d", time.Now().UnixNano(), job.UserID)
	}
	if job.CreatedAtUnix == 0 {
		job.CreatedAtUnix = time.Now().Unix()
	}
	b, _ := json.Marshal(job)
	ctx := context.Background()
	_ = storage.Redis.ZAdd(ctx, redisNotifDelayedZSet, &redis.Z{Score: float64(runAt.Unix()), Member: string(b)}).Err()
}

func EnqueueAINotificationNow(job AINotificationJob) {
	if storage.Redis == nil {
		return
	}
	if job.JobID == "" {
		job.JobID = fmt.Sprintf("job_%d_%d", time.Now().UnixNano(), job.UserID)
	}
	if job.CreatedAtUnix == 0 {
		job.CreatedAtUnix = time.Now().Unix()
	}
	b, _ := json.Marshal(job)
	ctx := context.Background()
	_ = storage.Redis.RPush(ctx, redisNotifJobsQueue, string(b)).Err()
}

// StartAINotificationQueue launches a due-job mover + N workers.
// Safe to call once during startup.
func StartAINotificationQueue() {
	if storage.Redis == nil || storage.DB == nil {
		log.Printf("⚠️ AI notification queue disabled: redis/db not ready")
		return
	}
	log.Printf("🚀 Starting AI notification queue workers...")

	// 1) Delayed mover: moves due jobs into the ready queue.
	go func() {
		ticker := time.NewTicker(800 * time.Millisecond)
		defer ticker.Stop()
		ctx := context.Background()

		for range ticker.C {
			now := float64(time.Now().Unix())
			// Small batch to keep latency low.
			items, err := storage.Redis.ZRangeByScore(ctx, redisNotifDelayedZSet, &redis.ZRangeBy{
				Min:   "-inf",
				Max:   fmt.Sprintf("%f", now),
				Count: 50,
			}).Result()
			if err != nil || len(items) == 0 {
				continue
			}
			// Best-effort remove then push.
			_ = storage.Redis.ZRem(ctx, redisNotifDelayedZSet, interfaceSlice(items)...).Err()
			for _, raw := range items {
				_ = storage.Redis.RPush(ctx, redisNotifJobsQueue, raw).Err()
			}
		}
	}()

	// 2) Workers: handle jobs.
	const workers = 3
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			ctx := context.Background()
			for {
				// BLPOP returns [queue, value]
				res, err := storage.Redis.BLPop(ctx, 20*time.Second, redisNotifJobsQueue).Result()
				if err != nil || len(res) < 2 {
					continue
				}
				raw := res[1]
				var job AINotificationJob
				if err := json.Unmarshal([]byte(raw), &job); err != nil {
					continue
				}
				handleAINotificationJob(job)
			}
		}(i + 1)
	}

	// 3) Re-engagement scheduler (hybrid: ticker acts like lightweight cron, outputs to queue).
	// Goal: never “stuck feed” perception — bring users back with a single best item.
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			enqueueReengageBatch(time.Now())
		}
	}()
}

func interfaceSlice(in []string) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// handleAINotificationJob is the “decision + delivery” core.
// For now we implement one high-value flow:
// - sale_view_reminder: after a meaningful property_sale view, remind user later if they went offline.
func handleAINotificationJob(job AINotificationJob) {
	now := time.Now()
	if job.UserID == 0 {
		return
	}

	switch job.Kind {
	case "sale_view_reminder":
		if job.PropertySale == nil || *job.PropertySale == 0 {
			return
		}

		ns := NotificationServiceInstance
		if !ns.canSendSmartType(job.UserID, "smart_sale_view_reminder", now) {
			return
		}

		// Only notify offline users (avoid spamming active sessions).
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", job.UserID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			return
		}

		// Cooldown/dedup: once per day per property per user.
		eventType := "smart_sale_view_reminder"
		fp := models.BuildFingerprint(job.UserID, "sale", nil, job.PropertySale, eventType, now.Format("2006-01-02"))
		var dup models.NotificationDeliveryLog
		if storage.DB.Where("fingerprint = ?", fp).First(&dup).Error == nil {
			return
		}

		// If user already saved this property, skip reminder.
		var saved int64
		_ = storage.DB.Model(&models.Interaction{}).
			Where("user_id = ? AND property_sale_id = ? AND event_type = ?", job.UserID, *job.PropertySale, models.EventSave).
			Count(&saved).Error
		if saved > 0 {
			return
		}

		var p models.PropertySale
		if err := storage.DB.Select("id", "title", "city", "currency", "listing_price", "images", "is_deactivated", "is_sold").
			Where("id = ? AND deleted_at IS NULL", *job.PropertySale).
			First(&p).Error; err != nil {
			return
		}
		if p.IsDeactivated || p.IsSold {
			return
		}

		lang := NormalizeNotificationLang(ResolveUserNotificationLang(job.UserID))
		title, body := saleViewReminderCopy(lang, p.Title, p.City, p.ListingPrice)
		title, body = maybeAICopy(
			job.UserID,
			lang,
			job.Kind,
			title,
			body,
			fmt.Sprintf(`User viewed this sale listing earlier (45m reminder). Listing: title=%q city=%q price=%.0f currency=%q. Goal: gentle nudge to reopen, not spam.`, p.Title, p.City, p.ListingPrice, p.Currency),
		)
		imageURL := ""
		if len(p.Images) > 0 {
			imageURL = p.Images[0]
		}

		data := map[string]string{
			"type":       "viewed_property_reminder",
			"id":         fmt.Sprintf("%d", p.ID),
			"propertyId": fmt.Sprintf("%d", p.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, p.ID),
			"action":     "view_property",
		}
		data["legacy_event_type"] = eventType
		data["legacy_property_kind"] = "sale"

		if ns.sendToUserWithImage(job.UserID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				_ = storage.DB.Create(&models.NotificationDeliveryLog{
					UserID:         job.UserID,
					EventType:      eventType,
					PropertyKind:   "sale",
					PropertySaleID: job.PropertySale,
					Fingerprint:    fp,
					ABVariant:      ABVariantForUser(job.UserID, ExperimentMeskenyPushCopyV1),
				}).Error
			}
		}
	case "sale_still_available":
		if job.PropertySale == nil || *job.PropertySale == 0 {
			return
		}

		ns := NotificationServiceInstance
		if !ns.canSendSmartType(job.UserID, "smart_sale_still_available", now) {
			return
		}

		// Only notify offline users.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", job.UserID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			return
		}

		// Dedupe: once/day per property.
		eventType := "smart_sale_still_available"
		fp := models.BuildFingerprint(job.UserID, "sale", nil, job.PropertySale, eventType, now.Format("2006-01-02"))
		var dup models.NotificationDeliveryLog
		if storage.DB.Where("fingerprint = ?", fp).First(&dup).Error == nil {
			return
		}

		// If user already saved this property, don't nag.
		var saved int64
		_ = storage.DB.Model(&models.Interaction{}).
			Where("user_id = ? AND property_sale_id = ? AND event_type = ?", job.UserID, *job.PropertySale, models.EventSave).
			Count(&saved).Error
		if saved > 0 {
			return
		}

		var p models.PropertySale
		if err := storage.DB.Select("id", "title", "city", "currency", "listing_price", "images", "is_deactivated", "is_sold").
			Where("id = ? AND deleted_at IS NULL", *job.PropertySale).
			First(&p).Error; err != nil {
			return
		}
		if p.IsDeactivated || p.IsSold {
			return
		}

		lang := NormalizeNotificationLang(ResolveUserNotificationLang(job.UserID))
		title, body := saleStillAvailableCopy(lang, p.Title, p.City, p.ListingPrice)
		title, body = maybeAICopy(
			job.UserID,
			lang,
			job.Kind,
			title,
			body,
			fmt.Sprintf(`User viewed a sale listing earlier. It is still available. Listing: title=%q city=%q price=%.0f currency=%q. Goal: urgency without pressure, prompt to open details.`, p.Title, p.City, p.ListingPrice, p.Currency),
		)

		imageURL := ""
		if len(p.Images) > 0 {
			imageURL = p.Images[0]
		}
		data := map[string]string{
			"type":       "still_available",
			"id":         fmt.Sprintf("%d", p.ID),
			"propertyId": fmt.Sprintf("%d", p.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, p.ID),
			"action":     "view_property",
		}
		data["legacy_event_type"] = eventType
		data["legacy_property_kind"] = "sale"

		if ns.sendToUserWithImage(job.UserID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				_ = storage.DB.Create(&models.NotificationDeliveryLog{
					UserID:         job.UserID,
					EventType:      eventType,
					PropertyKind:   "sale",
					PropertySaleID: job.PropertySale,
					Fingerprint:    fp,
					ABVariant:      ABVariantForUser(job.UserID, ExperimentMeskenyPushCopyV1),
				}).Error
			}
		}
	case "sale_similar_reco":
		if job.PropertySale == nil || *job.PropertySale == 0 {
			return
		}

		ns := NotificationServiceInstance
		// Use existing anti-spam (smart_* budget and cooldown behavior).
		if !ns.canSendSmartType(job.UserID, "smart_similar_properties", now) {
			return
		}

		// Avoid spamming active sessions.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", job.UserID).Order("last_seen_at DESC").First(&reg).Error
		if !isOffline(reg.LastSeenAt, now) {
			return
		}

		// Dedupe: once/day per saved property.
		eventType := "smart_similar_properties"
		fp := models.BuildFingerprint(job.UserID, "sale", nil, job.PropertySale, eventType, now.Format("2006-01-02"))
		var dup models.NotificationDeliveryLog
		if storage.DB.Where("fingerprint = ?", fp).First(&dup).Error == nil {
			return
		}

		// Load the saved property to build similarity query.
		var base models.PropertySale
		if err := storage.DB.
			Select("id", "title", "city_id", "zone_id", "quartier_id", "bedrooms", "bathrooms", "listing_price").
			Where("id = ? AND deleted_at IS NULL", *job.PropertySale).
			First(&base).Error; err != nil {
			return
		}

		// Similar candidates (simple + fast).
		minPrice := 0.0
		maxPrice := 0.0
		if base.ListingPrice > 0 {
			minPrice = base.ListingPrice * 0.80
			maxPrice = base.ListingPrice * 1.25
		}

		q := storage.DB.Model(&models.PropertySale{}).
			Where("(status = ? OR is_published = ?) AND is_deactivated = ? AND is_sold = ? AND deleted_at IS NULL", "published", true, false, false).
			Where("id <> ?", base.ID).
			Order("created_at DESC, id DESC").
			Limit(3)
		if base.CityID != nil {
			q = q.Where("city_id = ?", *base.CityID)
		}
		if base.ZoneID != nil {
			q = q.Where("zone_id = ?", *base.ZoneID)
		}
		if base.QuartierID != nil {
			q = q.Where("quartier_id = ?", *base.QuartierID)
		}
		if base.Bedrooms > 0 {
			q = q.Where("bedrooms BETWEEN ? AND ?", int(math.Max(0, float64(base.Bedrooms-1))), base.Bedrooms+1)
		}
		if base.Bathrooms > 0 {
			q = q.Where("bathrooms BETWEEN ? AND ?", int(math.Max(0, float64(base.Bathrooms-1))), base.Bathrooms+1)
		}
		if minPrice > 0 && maxPrice > 0 {
			q = q.Where("listing_price BETWEEN ? AND ?", minPrice, maxPrice)
		}

		var sims []models.PropertySale
		if err := q.Select("id", "title", "city", "listing_price", "images").Find(&sims).Error; err != nil {
			return
		}
		if len(sims) == 0 {
			return
		}

		// Language from notification preference.
		lang := NormalizeNotificationLang(ResolveUserNotificationLang(job.UserID))
		title, body := saleSimilarCopy(lang, base.Title, sims)
		simTitles := make([]string, 0, len(sims))
		for _, s := range sims {
			if strings.TrimSpace(s.Title) != "" {
				simTitles = append(simTitles, s.Title)
			}
		}
		title, body = maybeAICopy(
			job.UserID,
			lang,
			job.Kind,
			title,
			body,
			fmt.Sprintf(`User saved a sale listing. Saved: title=%q. Similar picks count=%d titles=%q. Goal: short recommendation, high intent.`, base.Title, len(sims), simTitles),
		)
		imageURL := ""
		if len(sims[0].Images) > 0 {
			imageURL = sims[0].Images[0]
		}

		// Deep link to the first recommended property; include the rest in params for future UI.
		ids := make([]uint, 0, len(sims))
		for _, s := range sims {
			ids = append(ids, s.ID)
		}

		data := map[string]string{
			"type":       "similar_properties",
			"id":         fmt.Sprintf("%d", sims[0].ID),
			"propertyId": fmt.Sprintf("%d", sims[0].ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d, "recommendedIds": %v}`, sims[0].ID, ids),
			"action":     "view_property",
		}
		data["legacy_event_type"] = eventType
		data["legacy_property_kind"] = "sale"

		if ns.sendToUserWithImage(job.UserID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				_ = storage.DB.Create(&models.NotificationDeliveryLog{
					UserID:         job.UserID,
					EventType:      eventType,
					PropertyKind:   "sale",
					PropertySaleID: job.PropertySale,
					Fingerprint:    fp,
					ABVariant:      ABVariantForUser(job.UserID, ExperimentMeskenyPushCopyV1),
				}).Error
			}
		}
	case "reengage_digest":
		ns := NotificationServiceInstance
		if !ns.canSendSmartType(job.UserID, "smart_reengage_digest", now) {
			return
		}

		// Must be offline long enough to justify a re-engagement.
		var reg models.DeviceRegistration
		_ = storage.DB.Where("user_id = ?", job.UserID).Order("last_seen_at DESC").First(&reg).Error
		if reg.LastSeenAt > 0 && now.Sub(time.Unix(reg.LastSeenAt, 0)) < reengageOfflineCutoff {
			return
		}

		eventType := "smart_reengage_digest"
		fp := models.BuildFingerprint(job.UserID, "sale", nil, nil, eventType, now.Format("2006-01-02"))
		var dup models.NotificationDeliveryLog
		if storage.DB.Where("fingerprint = ?", fp).First(&dup).Error == nil {
			return
		}

		// Pick one strong listing (favorite city if available).
		var u models.User
		if err := storage.DB.Select("id", "favorite_city_id").First(&u, job.UserID).Error; err != nil {
			return
		}

		q := storage.DB.Model(&models.PropertySale{}).
			Where("(status = ? OR is_published = ?) AND is_deactivated = ? AND is_sold = ? AND deleted_at IS NULL", "published", true, false, false).
			Order("created_at DESC, id DESC").
			Limit(12)
		if u.FavoriteCityID != nil {
			q = q.Where("city_id = ?", *u.FavoriteCityID)
		}
		var props []models.PropertySale
		_ = q.Select("id", "title", "city", "listing_price", "images").Find(&props).Error
		if len(props) == 0 {
			return
		}
		chosen := props[0]

		lang := NormalizeNotificationLang(ResolveUserNotificationLang(job.UserID))
		title, body := reengageCopy(lang, chosen.Title, chosen.City, chosen.ListingPrice)
		title, body = maybeAICopy(
			job.UserID,
			lang,
			job.Kind,
			title,
			body,
			fmt.Sprintf(`User offline >= 7 days. Re-engagement digest with one strong sale listing: title=%q city=%q price=%.0f currency=%q. Goal: bring back to app.`, chosen.Title, chosen.City, chosen.ListingPrice, chosen.Currency),
		)
		imageURL := ""
		if len(chosen.Images) > 0 {
			imageURL = chosen.Images[0]
		}

		data := map[string]string{
			"type":       "reengage_digest",
			"id":         fmt.Sprintf("%d", chosen.ID),
			"propertyId": fmt.Sprintf("%d", chosen.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, chosen.ID),
			"action":     "view_property",
		}
		data["legacy_event_type"] = eventType
		data["legacy_property_kind"] = "sale"

		if ns.sendToUserWithImage(job.UserID, title, body, imageURL, data) {
			if !NotificationOrchestratorEnabled() {
				_ = storage.DB.Create(&models.NotificationDeliveryLog{
					UserID:       job.UserID,
					EventType:    eventType,
					PropertyKind: "sale",
					PropertySaleID: &chosen.ID,
					Fingerprint:  fp,
					ABVariant:    ABVariantForUser(job.UserID, ExperimentMeskenyPushCopyV1),
				}).Error
			}
		}
	default:
		return
	}
}

func saleViewReminderCopy(lang, title, city string, price float64) (string, string) {
	switch lang {
	case "ar":
		t := "🏠 ما زال العقار متاحًا"
		b := fmt.Sprintf("العقار الذي شاهدته ما زال متاحًا: %s", title)
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	case "fr":
		t := "🏠 Toujours disponible"
		b := fmt.Sprintf("Le bien que vous avez consulté est toujours disponible : %s", title)
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	default:
		t := "🏠 Still available"
		b := fmt.Sprintf("The home you viewed is still available: %s", title)
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	}
}

func saleSimilarCopy(lang, baseTitle string, sims []models.PropertySale) (string, string) {
	top := sims[0]
	switch lang {
	case "ar":
		t := "🏡 عقارات مشابهة قد تعجبك"
		b := "وجدنا عقارات مشابهة لما حفظته."
		if strings.TrimSpace(baseTitle) != "" {
			b = fmt.Sprintf("بناءً على ما حفظته: %s", baseTitle)
		}
		b += fmt.Sprintf("\n• %s", top.Title)
		if top.City != "" {
			b += fmt.Sprintf(" • %s", top.City)
		}
		return t, b
	case "fr":
		t := "🏡 Biens similaires"
		b := "Nous avons trouvé des biens similaires à celui que vous avez enregistré."
		if strings.TrimSpace(baseTitle) != "" {
			b = fmt.Sprintf("D’après votre favori : %s", baseTitle)
		}
		b += fmt.Sprintf("\n• %s", top.Title)
		if top.City != "" {
			b += fmt.Sprintf(" • %s", top.City)
		}
		return t, b
	default:
		t := "🏡 Similar homes"
		b := "We found homes similar to what you saved."
		if strings.TrimSpace(baseTitle) != "" {
			b = fmt.Sprintf("Based on your saved home: %s", baseTitle)
		}
		b += fmt.Sprintf("\n• %s", top.Title)
		if top.City != "" {
			b += fmt.Sprintf(" • %s", top.City)
		}
		return t, b
	}
}

func saleStillAvailableCopy(lang, title, city string, price float64) (string, string) {
	switch lang {
	case "ar":
		t := "⏳ ما زال متاحًا"
		b := "العقار الذي شاهدته ما زال متاحًا. افتح التفاصيل الآن."
		if title != "" {
			b = fmt.Sprintf("ما زال متاحًا: %s", title)
		}
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	case "fr":
		t := "⏳ Toujours disponible"
		b := "Le bien que vous avez consulté est toujours disponible."
		if title != "" {
			b = fmt.Sprintf("Toujours disponible : %s", title)
		}
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	default:
		t := "⏳ Still available"
		b := "The home you viewed is still available. Tap to see details."
		if title != "" {
			b = fmt.Sprintf("Still available: %s", title)
		}
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	}
}

func reengageCopy(lang, title, city string, price float64) (string, string) {
	switch lang {
	case "ar":
		t := "🔥 عقارات جديدة بانتظارك"
		b := "ارجع وشاهد أحدث العقارات."
		if title != "" {
			b = fmt.Sprintf("قد يعجبك هذا العقار: %s", title)
		}
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	case "fr":
		t := "🔥 De nouveaux biens vous attendent"
		b := "Revenez découvrir les dernières annonces."
		if title != "" {
			b = fmt.Sprintf("Vous pourriez aimer : %s", title)
		}
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	default:
		t := "🔥 New homes waiting"
		b := "Come back—new listings just dropped."
		if title != "" {
			b = fmt.Sprintf("You might like: %s", title)
		}
		if city != "" {
			b += fmt.Sprintf(" • %s", city)
		}
		if price > 0 {
			b += fmt.Sprintf(" • %.0f MRU", price)
		}
		return t, b
	}
}

func enqueueReengageBatch(now time.Time) {
	if storage.DB == nil {
		return
	}
	if isInSleepTime(now) {
		return
	}
	// Only run in “good” hours to avoid annoying users.
	if now.Hour() < 11 || now.Hour() > 20 {
		return
	}

	// Find users who allow notifications and have been offline long enough.
	// Limit batch to avoid spikes.
	type row struct {
		UserID     uint  `gorm:"column:user_id"`
		LastSeenAt int64 `gorm:"column:last_seen_at"`
	}
	var rows []row
	// NOTE: DeviceRegistration stores last_seen_at; we join to users to ensure allows_notifications.
	storage.DB.Table("device_registrations dr").
		Select("dr.user_id, MAX(dr.last_seen_at) as last_seen_at").
		Joins("JOIN users u ON u.id = dr.user_id").
		Where("u.allows_notifications = ?", true).
		Where("dr.user_id IS NOT NULL").
		Group("dr.user_id").
		Order("MAX(dr.last_seen_at) ASC").
		Limit(120).
		Scan(&rows)

	for _, r := range rows {
		if r.UserID == 0 {
			continue
		}
		if r.LastSeenAt > 0 && now.Sub(time.Unix(r.LastSeenAt, 0)) < reengageOfflineCutoff {
			continue
		}
		uid := r.UserID
		EnqueueAINotificationNow(AINotificationJob{
			Kind:   "reengage_digest",
			UserID: uid,
		})
	}
}

type aiPushCopy struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func maybeAICopy(userID uint, lang string, kind string, fallbackTitle, fallbackBody, contextText string) (string, string) {
	// A/B: variant A = template only (control); variant B = attempt AI copy when service is up.
	if ABVariantForUser(userID, ExperimentMeskenyPushCopyV1) != "B" {
		return fallbackTitle, fallbackBody
	}
	if MeskenyGPTService == nil {
		return fallbackTitle, fallbackBody
	}

	sys := "You are MeskenyGPT writing push notifications for a real estate app (Meskeny). " +
		"Return ONLY valid JSON with keys: title, body. " +
		"No markdown, no extra text. " +
		"Keep title <= 52 chars, body <= 120 chars. " +
		"Do NOT include internal IDs. Do NOT invent prices or locations."

	langHint := "Respond in English."
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "ar", "arabic":
		langHint = "Respond in Arabic."
	case "fr", "french":
		langHint = "Réponds en français."
	}

	prompt := sys + "\n" + langHint + "\n\n" +
		"Notification kind: " + kind + "\n" +
		"Context: " + contextText + "\n\n" +
		"Fallback JSON (use same meaning if unsure):\n" +
		`{"title": ` + jsonString(fallbackTitle) + `, "body": ` + jsonString(fallbackBody) + `}` + "\n"

	out, err := MeskenyGPTService.HandleChatTurn(context.Background(), meskenyai.ChatInput{
		UserID:    userID,
		SessionID: fmt.Sprintf("push_%s_%d", kind, time.Now().UnixNano()),
		Text:      prompt,
	})
	if err != nil {
		return fallbackTitle, fallbackBody
	}
	raw := strings.TrimSpace(out.Message.Content)
	if raw == "" {
		return fallbackTitle, fallbackBody
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return fallbackTitle, fallbackBody
	}
	raw = raw[start : end+1]

	var c aiPushCopy
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return fallbackTitle, fallbackBody
	}
	title := strings.TrimSpace(c.Title)
	body := strings.TrimSpace(c.Body)
	if title == "" || body == "" {
		return fallbackTitle, fallbackBody
	}
	return title, body
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

