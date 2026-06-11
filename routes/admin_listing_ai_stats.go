package routes

import (
	"net/http"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// GET /api/admin/ai/listing-usage — Add-with-AI usage (rent / sale / land) for admins.
func AdminGetListingAIUsageStats(ctx iris.Context) {
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
	countSince := func(event string, since *time.Time, kind string) int64 {
		var row countRow
		q := storage.DB.Model(&models.ListingAIUsageEvent{}).
			Select("COUNT(*)::bigint AS value").
			Where("event = ?", event)
		if since != nil {
			q = q.Where("created_at >= ?", *since)
		}
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		_ = q.Scan(&row).Error
		return row.Value
	}
	countBetween := func(event string, start, end time.Time, kind string) int64 {
		var row countRow
		q := storage.DB.Model(&models.ListingAIUsageEvent{}).
			Select("COUNT(*)::bigint AS value").
			Where("event = ? AND created_at >= ? AND created_at < ?", event, start, end)
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		_ = q.Scan(&row).Error
		return row.Value
	}
	uniqueUsersSince := func(since time.Time, kind string) int64 {
		var row countRow
		q := storage.DB.Model(&models.ListingAIUsageEvent{}).
			Select("COUNT(DISTINCT user_id)::bigint AS value").
			Where("created_at >= ?", since)
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		_ = q.Scan(&row).Error
		return row.Value
	}
	uniqueUsersEventSince := func(event string, since time.Time, kind string) int64 {
		var row countRow
		q := storage.DB.Model(&models.ListingAIUsageEvent{}).
			Select("COUNT(DISTINCT user_id)::bigint AS value").
			Where("event = ? AND created_at >= ?", event, since)
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		_ = q.Scan(&row).Error
		return row.Value
	}

	kindSummary := func(kind string) iris.Map {
		return iris.Map{
			"started_all_time":      countSince("started", nil, kind),
			"completed_all_time":    countSince("completed", nil, kind),
			"failed_all_time":       countSince("failed", nil, kind),
			"published_all_time":    countSince("published", nil, kind),
			"unique_users_30d":      uniqueUsersSince(last30DaysStart, kind),
			"started_today":         countSince("started", &todayStart, kind),
			"started_this_week":     countSince("started", &thisWeekStart, kind),
			"started_last_7_days":   countSince("started", &last7DaysStart, kind),
			"published_last_7_days": countSince("published", &last7DaysStart, kind),
		}
	}

	var totalEvents int64
	var uniqueUsersAll int64
	_ = storage.DB.Model(&models.ListingAIUsageEvent{}).Count(&totalEvents)
	_ = storage.DB.Model(&models.ListingAIUsageEvent{}).
		Distinct("user_id").
		Count(&uniqueUsersAll)

	summary := iris.Map{
		"total_events_all_time":        totalEvents,
		"unique_users_all_time":        uniqueUsersAll,
		"started_today":                countSince("started", &todayStart, ""),
		"started_this_week":            countSince("started", &thisWeekStart, ""),
		"started_previous_week":        countBetween("started", lastWeekStart, thisWeekStart, ""),
		"started_last_7_days":          countSince("started", &last7DaysStart, ""),
		"started_previous_7_days":      countBetween("started", prev7Start, last7DaysStart, ""),
		"started_last_30_days":         countSince("started", &last30DaysStart, ""),
		"completed_last_30_days":       countSince("completed", &last30DaysStart, ""),
		"failed_last_30_days":          countSince("failed", &last30DaysStart, ""),
		"published_last_30_days":       countSince("published", &last30DaysStart, ""),
		"published_all_time":           countSince("published", nil, ""),
		"unique_users_started_30d":     uniqueUsersEventSince("started", last30DaysStart, ""),
		"unique_users_published_30d": uniqueUsersEventSince("published", last30DaysStart, ""),
		"by_kind": iris.Map{
			"rent":  kindSummary("rent"),
			"sale":  kindSummary("sale"),
			"land":  kindSummary("land"),
		},
	}

	type dailyRaw struct {
		DateStr   string `gorm:"column:date_str"`
		Started   int64  `gorm:"column:started"`
		Completed int64  `gorm:"column:completed"`
		Failed    int64  `gorm:"column:failed"`
		Published int64  `gorm:"column:published"`
		Users     int64  `gorm:"column:users"`
		Rent      int64  `gorm:"column:rent"`
		Sale      int64  `gorm:"column:sale"`
		Land      int64  `gorm:"column:land"`
	}
	var dailyRows []dailyRaw
	_ = storage.DB.Raw(`
		SELECT
			TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date_str,
			COUNT(*) FILTER (WHERE event = 'started')::bigint AS started,
			COUNT(*) FILTER (WHERE event = 'completed')::bigint AS completed,
			COUNT(*) FILTER (WHERE event = 'failed')::bigint AS failed,
			COUNT(*) FILTER (WHERE event = 'published')::bigint AS published,
			COUNT(DISTINCT user_id) FILTER (WHERE event = 'started')::bigint AS users,
			COUNT(*) FILTER (WHERE event = 'started' AND kind = 'rent')::bigint AS rent,
			COUNT(*) FILTER (WHERE event = 'started' AND kind = 'sale')::bigint AS sale,
			COUNT(*) FILTER (WHERE event = 'started' AND kind = 'land')::bigint AS land
		FROM listing_ai_usage_events
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
			"started":   r.Started,
			"completed": r.Completed,
			"failed":    r.Failed,
			"published": r.Published,
			"users":     r.Users,
			"rent":      r.Rent,
			"sale":      r.Sale,
			"land":      r.Land,
		})
	}

	weeksStart := thisWeekStart.AddDate(0, 0, -7*11)
	type weeklyRaw struct {
		WeekStart string `gorm:"column:week_start"`
		Started   int64  `gorm:"column:started"`
		Completed int64  `gorm:"column:completed"`
		Failed    int64  `gorm:"column:failed"`
		Published int64  `gorm:"column:published"`
		Users     int64  `gorm:"column:users"`
		Rent      int64  `gorm:"column:rent"`
		Sale      int64  `gorm:"column:sale"`
		Land      int64  `gorm:"column:land"`
	}
	var weeklyRows []weeklyRaw
	_ = storage.DB.Raw(`
		SELECT
			TO_CHAR(
				((created_at AT TIME ZONE 'UTC')::date - ((EXTRACT(ISODOW FROM (created_at AT TIME ZONE 'UTC')::date)::int - 1) || ' days')::interval),
				'YYYY-MM-DD'
			) AS week_start,
			COUNT(*) FILTER (WHERE event = 'started')::bigint AS started,
			COUNT(*) FILTER (WHERE event = 'completed')::bigint AS completed,
			COUNT(*) FILTER (WHERE event = 'failed')::bigint AS failed,
			COUNT(*) FILTER (WHERE event = 'published')::bigint AS published,
			COUNT(DISTINCT user_id) FILTER (WHERE event = 'started')::bigint AS users,
			COUNT(*) FILTER (WHERE event = 'started' AND kind = 'rent')::bigint AS rent,
			COUNT(*) FILTER (WHERE event = 'started' AND kind = 'sale')::bigint AS sale,
			COUNT(*) FILTER (WHERE event = 'started' AND kind = 'land')::bigint AS land
		FROM listing_ai_usage_events
		WHERE created_at >= ?
		GROUP BY 1
		ORDER BY 1 ASC
	`, weeksStart).Scan(&weeklyRows)

	weekly := make([]iris.Map, 0, len(weeklyRows))
	for _, r := range weeklyRows {
		weekly = append(weekly, iris.Map{
			"week_start": r.WeekStart,
			"started":    r.Started,
			"completed":  r.Completed,
			"failed":     r.Failed,
			"published":  r.Published,
			"users":      r.Users,
			"rent":       r.Rent,
			"sale":       r.Sale,
			"land":       r.Land,
		})
	}

	type recentRow struct {
		ID        uint      `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UserID    uint      `json:"user_id"`
		Kind      string    `json:"kind"`
		Event     string    `json:"event"`
		JobID     string    `json:"job_id,omitempty"`
	}
	var recent []recentRow
	if err := storage.DB.Model(&models.ListingAIUsageEvent{}).
		Order("created_at DESC").
		Limit(50).
		Scan(&recent).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_recent", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"summary": summary,
			"daily":   daily,
			"weekly":  weekly,
			"recent":  recent,
		},
	})
}
