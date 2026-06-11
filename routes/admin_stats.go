package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

// GET /admin/stats
func AdminStats(ctx iris.Context) {
	var pendingProperties int64
	storage.DB.Model(&models.Property{}).Where("status = ?", "pending").Count(&pendingProperties)
	var pendingVerifications int64
	storage.DB.Model(&models.IdentityVerification{}).Where("status = ?", "pending").Count(&pendingVerifications)
	var pendingVideos int64
	storage.DB.Model(&models.Video{}).Where("status = ?", "pending").Count(&pendingVideos)

	var pendingPropertySales int64
	storage.DB.Model(&models.PropertySale{}).
		Where("is_verified = ? AND LOWER(status) IN ?", false, []string{"draft", "pending_verification"}).
		Count(&pendingPropertySales)

	var pendingLandmarks int64
	storage.DB.Model(&models.Landmark{}).
		Where("LOWER(status) = ?", "pending_verification").
		Count(&pendingLandmarks)

	var pendingBrokerVerifications int64
	storage.DB.Model(&models.User{}).
		Where("broker_status = ?", "pending").
		Count(&pendingBrokerVerifications)

	since7 := time.Now().AddDate(0, 0, -7)
	since30 := time.Now().AddDate(0, 0, -30)
	var newRes7, newRes30 int64
	storage.DB.Model(&models.Reservation{}).Where("created_at >= ?", since7).Count(&newRes7)
	storage.DB.Model(&models.Reservation{}).Where("created_at >= ?", since30).Count(&newRes30)

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"pending_properties":      pendingProperties,
			"pending_property_sales":  pendingPropertySales,
			"pending_landmarks":       pendingLandmarks,
			"pending_verifications":   pendingVerifications,
			"pending_broker_verifications": pendingBrokerVerifications,
			"pending_videos":          pendingVideos,
			"new_reservations_7d":   newRes7,
			"new_reservations_30d":  newRes30,
		},
		"meta":  iris.Map{},
		"links": iris.Map{},
	})
}

// GET /admin/activity
func AdminActivity(ctx iris.Context) {
	var logs []models.AuditLog
	storage.DB.Order("created_at DESC").Limit(100).Find(&logs)
	ctx.JSON(iris.Map{"data": logs, "meta": iris.Map{}, "links": iris.Map{}})
}

// GET /admin/notifications/new-homes
// Admin analytics for "new homes" push notifications.
// Covers:
// - smart_rent_suggestion (rent feed suggestions)
// - smart_new_property_match (newly published property sale matches)
func AdminNewHomesNotificationStats(ctx iris.Context) {
	const rentEventType = "smart_rent_suggestion"
	const saleEventType = "smart_new_property_match"

	type summaryRow struct {
		TotalSent         int64      `json:"total_sent"`
		UniqueUsers       int64      `json:"unique_users"`
		UniqueDevices     int64      `json:"unique_devices"`
		LastSentAt        *time.Time `json:"last_sent_at"`
		Last24hSent       int64      `json:"last_24h_sent"`
		Last7dSent        int64      `json:"last_7d_sent"`
		ThrottledNowCount int64      `json:"throttled_now_count"`
	}
	type dailyRow struct {
		Day           string `json:"day"`
		SentCount     int64  `json:"sent_count"`
		UniqueUsers   int64  `json:"unique_users"`
		UniqueDevices int64  `json:"unique_devices"`
	}
	type topPropertyRow struct {
		PropertyKind   string     `json:"property_kind"`
		ReferenceID    uint       `json:"reference_id"`
		Title          string     `json:"title"`
		City           string     `json:"city"`
		SentCount      int64      `json:"sent_count"`
		UniqueUsers    int64      `json:"unique_users"`
		UniqueDevices  int64      `json:"unique_devices"`
		LastSentAt     *time.Time `json:"last_sent_at"`
	}
	type recentRow struct {
		SentAt       time.Time `json:"sent_at"`
		UserID       uint      `json:"user_id"`
		PropertyKind string    `json:"property_kind"`
		ReferenceID  uint      `json:"reference_id"`
		Title        string    `json:"title"`
		City         string    `json:"city"`
		DeviceCount  int64     `json:"device_count"`
	}
	type deviceTimingRow struct {
		DeviceID   uint       `json:"device_id"`
		UserID     *uint      `json:"user_id"`
		Platform   string     `json:"platform"`
		Locale     string     `json:"locale"`
		AppVersion string     `json:"app_version"`
		LastSentAt *time.Time `json:"last_sent_at"`
		NextSendAt *time.Time `json:"next_send_at"`
		IsThrottled bool      `json:"is_throttled"`
		UpdatedAt  time.Time  `json:"updated_at"`
	}

	var summary summaryRow

	// Core totals
	_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
		Where("(event_type = ? AND property_kind = ?) OR (event_type = ? AND property_kind = ?)",
			rentEventType, "rent", saleEventType, "sale").
		Count(&summary.TotalSent).Error

	_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
		Where("(event_type = ? AND property_kind = ?) OR (event_type = ? AND property_kind = ?)",
			rentEventType, "rent", saleEventType, "sale").
		Distinct("user_id").
		Count(&summary.UniqueUsers).Error

	_ = storage.DB.Raw(`
		SELECT COUNT(DISTINCT np.push_token)
		FROM notification_preferences np
		JOIN notification_delivery_logs ndl ON ndl.user_id = np.user_id
		WHERE (
			(ndl.event_type = ? AND ndl.property_kind = 'rent')
			OR
			(ndl.event_type = ? AND ndl.property_kind = 'sale')
		)
		AND np.enabled = true AND np.push_token <> ''
	`, rentEventType, saleEventType).Scan(&summary.UniqueDevices).Error

	_ = storage.DB.Raw(`
		SELECT MAX(created_at)
		FROM notification_delivery_logs
		WHERE (event_type = ? AND property_kind = 'rent')
		   OR (event_type = ? AND property_kind = 'sale')
	`, rentEventType, saleEventType).Scan(&summary.LastSentAt).Error

	_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
		Where("((event_type = ? AND property_kind = ?) OR (event_type = ? AND property_kind = ?)) AND created_at >= ?",
			rentEventType, "rent", saleEventType, "sale", time.Now().Add(-24*time.Hour)).
		Count(&summary.Last24hSent).Error

	_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
		Where("((event_type = ? AND property_kind = ?) OR (event_type = ? AND property_kind = ?)) AND created_at >= ?",
			rentEventType, "rent", saleEventType, "sale", time.Now().Add(-7*24*time.Hour)).
		Count(&summary.Last7dSent).Error

	_ = storage.DB.Model(&models.MarketingDevice{}).
		Where("marketing_opt_in = ? AND fcm_token <> '' AND next_send_at IS NOT NULL AND next_send_at > ?", true, time.Now()).
		Count(&summary.ThrottledNowCount).Error

	var daily []dailyRow
	_ = storage.DB.Raw(`
		SELECT
			TO_CHAR(DATE(ndl.created_at), 'YYYY-MM-DD') AS day,
			COUNT(*) AS sent_count,
			COUNT(DISTINCT ndl.user_id) AS unique_users,
			COUNT(DISTINCT np.push_token) AS unique_devices
		FROM notification_delivery_logs ndl
		LEFT JOIN notification_preferences np ON np.user_id = ndl.user_id AND np.enabled = true
		WHERE (
			(ndl.event_type = ? AND ndl.property_kind = 'rent')
			OR
			(ndl.event_type = ? AND ndl.property_kind = 'sale')
		)
		  AND ndl.created_at >= NOW() - INTERVAL '30 days'
		GROUP BY DATE(ndl.created_at)
		ORDER BY DATE(ndl.created_at) DESC
	`, rentEventType, saleEventType).Scan(&daily).Error

	var topProperties []topPropertyRow
	_ = storage.DB.Raw(`
		SELECT
			ndl.property_kind AS property_kind,
			COALESCE(ndl.property_id, ndl.property_sale_id) AS reference_id,
			COALESCE(rp.title, sp.title, CONCAT('Listing #', COALESCE(ndl.property_id, ndl.property_sale_id)::text)) AS title,
			COALESCE(rp.city, sp.city, '') AS city,
			COUNT(*) AS sent_count,
			COUNT(DISTINCT ndl.user_id) AS unique_users,
			COUNT(DISTINCT np.push_token) AS unique_devices,
			MAX(ndl.created_at) AS last_sent_at
		FROM notification_delivery_logs ndl
		LEFT JOIN properties rp ON ndl.property_kind = 'rent' AND rp.id = ndl.property_id
		LEFT JOIN property_sales sp ON ndl.property_kind = 'sale' AND sp.id = ndl.property_sale_id
		LEFT JOIN notification_preferences np ON np.user_id = ndl.user_id AND np.enabled = true
		WHERE (
			(ndl.event_type = ? AND ndl.property_kind = 'rent' AND ndl.property_id IS NOT NULL)
			OR
			(ndl.event_type = ? AND ndl.property_kind = 'sale' AND ndl.property_sale_id IS NOT NULL)
		)
		GROUP BY ndl.property_kind, COALESCE(ndl.property_id, ndl.property_sale_id), rp.title, sp.title, rp.city, sp.city
		ORDER BY sent_count DESC, last_sent_at DESC
		LIMIT 30
	`, rentEventType, saleEventType).Scan(&topProperties).Error

	var recentDeliveries []recentRow
	_ = storage.DB.Raw(`
		SELECT
			ndl.created_at AS sent_at,
			ndl.user_id AS user_id,
			ndl.property_kind AS property_kind,
			COALESCE(ndl.property_id, ndl.property_sale_id) AS reference_id,
			COALESCE(rp.title, sp.title, CONCAT('Listing #', COALESCE(ndl.property_id, ndl.property_sale_id)::text)) AS title,
			COALESCE(rp.city, sp.city, '') AS city,
			COALESCE(dev.device_count, 0) AS device_count
		FROM notification_delivery_logs ndl
		LEFT JOIN properties rp ON ndl.property_kind = 'rent' AND rp.id = ndl.property_id
		LEFT JOIN property_sales sp ON ndl.property_kind = 'sale' AND sp.id = ndl.property_sale_id
		LEFT JOIN (
			SELECT user_id, COUNT(DISTINCT push_token) AS device_count
			FROM notification_preferences
			WHERE enabled = true AND push_token <> ''
			GROUP BY user_id
		) dev ON dev.user_id = ndl.user_id
		WHERE (
			(ndl.event_type = ? AND ndl.property_kind = 'rent')
			OR
			(ndl.event_type = ? AND ndl.property_kind = 'sale')
		)
		ORDER BY ndl.created_at DESC
		LIMIT 200
	`, rentEventType, saleEventType).Scan(&recentDeliveries).Error

	var deviceTiming []deviceTimingRow
	_ = storage.DB.Raw(`
		SELECT
			id AS device_id,
			user_id,
			platform,
			locale,
			app_version,
			last_sent_at,
			next_send_at,
			CASE
				WHEN next_send_at IS NOT NULL AND next_send_at > NOW() THEN true
				ELSE false
			END AS is_throttled,
			updated_at
		FROM marketing_devices
		WHERE marketing_opt_in = true
		  AND fcm_token <> ''
		  AND (last_sent_at IS NOT NULL OR next_send_at IS NOT NULL)
		ORDER BY COALESCE(last_sent_at, updated_at) DESC
		LIMIT 500
	`).Scan(&deviceTiming).Error

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"summary":               summary,
			"daily":                 daily,
			"top_properties":        topProperties,
			"recent_deliveries":     recentDeliveries,
			"device_timing_details": deviceTiming,
		},
		"meta": iris.Map{
			"event_types": []string{rentEventType, saleEventType},
		},
		"links": iris.Map{},
	})
}

// GET /admin/notifications/new-homes/devices
// Returns paginated per-device notification timing for admin monitoring.
func AdminNewHomesNotificationDeviceTiming(ctx iris.Context) {
	page, _ := strconv.Atoi(strings.TrimSpace(ctx.URLParamDefault("page", "1")))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(strings.TrimSpace(ctx.URLParamDefault("per_page", "50")))
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	offset := (page - 1) * perPage

	userIDFilter, _ := strconv.Atoi(strings.TrimSpace(ctx.URLParamDefault("user_id", "0")))
	platformFilter := strings.TrimSpace(ctx.URLParamDefault("platform", ""))
	onlyThrottled := strings.EqualFold(strings.TrimSpace(ctx.URLParamDefault("only_throttled", "false")), "true") ||
		strings.TrimSpace(ctx.URLParamDefault("only_throttled", "0")) == "1"

	type deviceTimingRow struct {
		DeviceID    uint       `json:"device_id"`
		UserID      *uint      `json:"user_id"`
		Platform    string     `json:"platform"`
		Locale      string     `json:"locale"`
		AppVersion  string     `json:"app_version"`
		LastSentAt  *time.Time `json:"last_sent_at"`
		NextSendAt  *time.Time `json:"next_send_at"`
		IsThrottled bool       `json:"is_throttled"`
		UpdatedAt   time.Time  `json:"updated_at"`
	}

	q := storage.DB.Model(&models.MarketingDevice{}).
		Where("marketing_opt_in = true AND fcm_token <> '' AND (last_sent_at IS NOT NULL OR next_send_at IS NOT NULL)")

	if userIDFilter > 0 {
		q = q.Where("user_id = ?", userIDFilter)
	}
	if platformFilter != "" {
		q = q.Where("LOWER(platform) = LOWER(?)", platformFilter)
	}
	if onlyThrottled {
		q = q.Where("next_send_at IS NOT NULL AND next_send_at > ?", time.Now())
	}

	var total int64
	_ = q.Count(&total).Error

	var rows []deviceTimingRow
	_ = q.Select(`
		id AS device_id,
		user_id,
		platform,
		locale,
		app_version,
		last_sent_at,
		next_send_at,
		CASE
			WHEN next_send_at IS NOT NULL AND next_send_at > NOW() THEN true
			ELSE false
		END AS is_throttled,
		updated_at
	`).Order("COALESCE(last_sent_at, updated_at) DESC").
		Offset(offset).
		Limit(perPage).
		Scan(&rows).Error

	ctx.JSON(iris.Map{
		"data": rows,
		"meta": iris.Map{
			"page":           page,
			"per_page":       perPage,
			"total":          total,
			"only_throttled": onlyThrottled,
			"user_id": func() interface{} {
				if userIDFilter > 0 {
					return userIDFilter
				}
				return nil
			}(),
			"platform": func() interface{} {
				if platformFilter != "" {
					return platformFilter
				}
				return nil
			}(),
		},
		"links": iris.Map{},
	})
}
