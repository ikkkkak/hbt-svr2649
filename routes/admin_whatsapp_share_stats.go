package routes

import (
	"net/http"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// GET /api/admin/whatsapp-share/usage — WhatsApp share card analytics for admins.
func AdminGetWhatsAppShareUsageStats(ctx iris.Context) {
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	last30DaysStart := todayStart.AddDate(0, 0, -29)
	last7DaysStart := todayStart.AddDate(0, 0, -6)
	prev7Start := todayStart.AddDate(0, 0, -13)

	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	thisWeekStart := todayStart.AddDate(0, 0, -(weekday - 1))
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)

	type countRow struct {
		Value int64 `gorm:"column:value"`
	}
	countSince := func(event string, since *time.Time) int64 {
		var row countRow
		q := storage.DB.Model(&models.WhatsAppShareUsageEvent{}).
			Select("COUNT(*)::bigint AS value").
			Where("event = ?", event)
		if since != nil {
			q = q.Where("created_at >= ?", *since)
		}
		_ = q.Scan(&row).Error
		return row.Value
	}
	countBetween := func(event string, start, end time.Time) int64 {
		var row countRow
		_ = storage.DB.Model(&models.WhatsAppShareUsageEvent{}).
			Select("COUNT(*)::bigint AS value").
			Where("event = ? AND created_at >= ? AND created_at < ?", event, start, end).
			Scan(&row).Error
		return row.Value
	}
	uniqueUsersEventSince := func(event string, since time.Time) int64 {
		var row countRow
		_ = storage.DB.Model(&models.WhatsAppShareUsageEvent{}).
			Select("COUNT(DISTINCT user_id) FILTER (WHERE user_id > 0)::bigint AS value").
			Where("event = ? AND created_at >= ?", event, since).
			Scan(&row).Error
		return row.Value
	}
	uniqueListingsEventSince := func(event string, since time.Time) int64 {
		var row countRow
		_ = storage.DB.Model(&models.WhatsAppShareUsageEvent{}).
			Select("COUNT(DISTINCT property_sale_id)::bigint AS value").
			Where("event = ? AND created_at >= ?", event, since).
			Scan(&row).Error
		return row.Value
	}

	var totalEvents int64
	_ = storage.DB.Model(&models.WhatsAppShareUsageEvent{}).Count(&totalEvents)

	completed30d := countSince("share_completed", &last30DaysStart)
	started30d := countSince("share_started", &last30DaysStart)
	failed30d := countSince("share_failed", &last30DaysStart)
	opened30d := countSince("sheet_opened", &last30DaysStart)

	completionRate30d := 0.0
	if started30d > 0 {
		completionRate30d = float64(completed30d) / float64(started30d) * 100
	}

	summary := iris.Map{
		"total_events_all_time":          totalEvents,
		"shares_completed_all_time":      countSince("share_completed", nil),
		"sheet_opened_all_time":          countSince("sheet_opened", nil),
		"share_started_all_time":         countSince("share_started", nil),
		"share_failed_all_time":          countSince("share_failed", nil),
		"share_dismissed_all_time":       countSince("share_dismissed", nil),
		"sheet_opened_today":             countSince("sheet_opened", &todayStart),
		"shares_completed_today":         countSince("share_completed", &todayStart),
		"shares_completed_this_week":     countSince("share_completed", &thisWeekStart),
		"shares_completed_previous_week": countBetween("share_completed", lastWeekStart, thisWeekStart),
		"shares_completed_last_7_days":   countSince("share_completed", &last7DaysStart),
		"shares_completed_previous_7_days": countBetween("share_completed", prev7Start, last7DaysStart),
		"shares_completed_last_30_days":  completed30d,
		"sheet_opened_last_30_days":      opened30d,
		"share_started_last_30_days":       started30d,
		"share_failed_last_30_days":      failed30d,
		"share_dismissed_last_30_days":   countSince("share_dismissed", &last30DaysStart),
		"completion_rate_last_30_days":   completionRate30d,
		"unique_users_completed_30d":     uniqueUsersEventSince("share_completed", last30DaysStart),
		"unique_listings_shared_30d":     uniqueListingsEventSince("share_completed", last30DaysStart),
	}

	type dailyRaw struct {
		DateStr    string `gorm:"column:date_str"`
		Opened     int64  `gorm:"column:opened"`
		Started    int64  `gorm:"column:started"`
		Completed  int64  `gorm:"column:completed"`
		Failed     int64  `gorm:"column:failed"`
		Dismissed  int64  `gorm:"column:dismissed"`
		Users      int64  `gorm:"column:users"`
		Listings   int64  `gorm:"column:listings"`
	}
	var dailyRows []dailyRaw
	_ = storage.DB.Raw(`
		SELECT
			TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date_str,
			COUNT(*) FILTER (WHERE event = 'sheet_opened')::bigint AS opened,
			COUNT(*) FILTER (WHERE event = 'share_started')::bigint AS started,
			COUNT(*) FILTER (WHERE event = 'share_completed')::bigint AS completed,
			COUNT(*) FILTER (WHERE event = 'share_failed')::bigint AS failed,
			COUNT(*) FILTER (WHERE event = 'share_dismissed')::bigint AS dismissed,
			COUNT(DISTINCT user_id) FILTER (WHERE event = 'share_completed' AND user_id > 0)::bigint AS users,
			COUNT(DISTINCT property_sale_id) FILTER (WHERE event = 'share_completed')::bigint AS listings
		FROM whatsapp_share_usage_events
		WHERE created_at >= ?
		GROUP BY (created_at AT TIME ZONE 'UTC')::date
		ORDER BY (created_at AT TIME ZONE 'UTC')::date ASC
	`, last30DaysStart).Scan(&dailyRows)

	daily := make([]iris.Map, 0, len(dailyRows))
	for _, r := range dailyRows {
		if r.DateStr == "" {
			continue
		}
		daily = append(daily, iris.Map{
			"date":      r.DateStr,
			"opened":    r.Opened,
			"started":   r.Started,
			"completed": r.Completed,
			"failed":    r.Failed,
			"dismissed": r.Dismissed,
			"users":     r.Users,
			"listings":  r.Listings,
		})
	}

	type platformRaw struct {
		Platform  string `gorm:"column:platform"`
		Completed int64  `gorm:"column:completed"`
	}
	var platformRows []platformRaw
	_ = storage.DB.Raw(`
		SELECT
			COALESCE(NULLIF(platform, ''), 'unknown') AS platform,
			COUNT(*)::bigint AS completed
		FROM whatsapp_share_usage_events
		WHERE event = 'share_completed' AND created_at >= ?
		GROUP BY 1
		ORDER BY completed DESC
	`, last30DaysStart).Scan(&platformRows)

	byPlatform := make([]iris.Map, 0, len(platformRows))
	for _, r := range platformRows {
		byPlatform = append(byPlatform, iris.Map{
			"platform":  r.Platform,
			"completed": r.Completed,
		})
	}

	type recentRow struct {
		ID             uint      `json:"id"`
		CreatedAt      time.Time `json:"created_at"`
		UserID         uint      `json:"user_id"`
		PropertySaleID uint      `json:"property_sale_id"`
		Event          string    `json:"event"`
		Platform       string    `json:"platform,omitempty"`
		PropertyTitle  string    `json:"property_title,omitempty"`
	}
	var recent []recentRow
	if err := storage.DB.Model(&models.WhatsAppShareUsageEvent{}).
		Order("created_at DESC").
		Limit(100).
		Scan(&recent).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_recent", "message": err.Error()})
		return
	}

	if recent == nil {
		recent = []recentRow{}
	}
	if daily == nil {
		daily = []iris.Map{}
	}
	if byPlatform == nil {
		byPlatform = []iris.Map{}
	}

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"summary":     summary,
			"daily":       daily,
			"by_platform": byPlatform,
			"recent":      recent,
		},
	})
}

// GET /api/admin/whatsapp-share/badge — lightweight sidebar count (completed shares, all time).
func AdminGetWhatsAppShareBadgeCount(ctx iris.Context) {
	var count int64
	_ = storage.DB.Model(&models.WhatsAppShareUsageEvent{}).
		Where("event = ?", "share_completed").
		Count(&count)
	ctx.JSON(iris.Map{"data": iris.Map{"count": count}})
}
