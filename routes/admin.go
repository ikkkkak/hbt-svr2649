package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

// ListUsers - GET /admin/users?role=&q=&page=&per_page=
func AdminListUsers(ctx iris.Context) {
	// Basic pagination
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	var users []models.User
	q := strings.TrimSpace(ctx.URLParamDefault("q", ""))
	role := strings.TrimSpace(ctx.URLParamDefault("role", ""))

	query := storage.DB.Model(&models.User{})
	if role != "" {
		query = query.Where("role = ?", role)
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query = query.Where("lower(first_name) LIKE ? OR lower(last_name) LIKE ? OR lower(email) LIKE ?", like, like, like)
	}

	var total int64
	query.Count(&total)
	query = query.Offset((page - 1) * perPage).Limit(perPage)
	if err := query.Find(&users).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "server_error", "message": err.Error()})
		return
	}

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonthStart := monthStart.AddDate(0, 1, 0)
	prevMonthStart := monthStart.AddDate(0, -1, 0)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	last30DaysStart := todayStart.AddDate(0, 0, -29)
	last7DaysStart := todayStart.AddDate(0, 0, -6)
	prev7PeriodStart := todayStart.AddDate(0, 0, -13)
	prev30PeriodStart := todayStart.AddDate(0, 0, -59)

	// Monday 00:00 UTC as start of "this week"
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	thisWeekStart := todayStart.AddDate(0, 0, -(weekday - 1))
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)

	var totalUsersAll int64
	var hostsCount int64
	var staffCount int64
	var verifiedIdentityCount int64
	_ = storage.DB.Model(&models.User{}).Count(&totalUsersAll)
	_ = storage.DB.Model(&models.User{}).Where("role = ?", "host").Count(&hostsCount)
	_ = storage.DB.Model(&models.User{}).Where("role IN ?", []string{"admin", "super_admin"}).Count(&staffCount)
	_ = storage.DB.Model(&models.User{}).Where("LOWER(TRIM(verification_status)) = ?", "verified").Count(&verifiedIdentityCount)

	var createdThisMonth int64
	var createdPreviousMonth int64
	var createdToday int64
	var createdLast30Days int64
	var createdThisWeek int64
	var createdPreviousWeek int64
	var rolling7d int64
	var rollingPrev7d int64
	var rollingPrev30d int64

	storage.DB.Model(&models.User{}).Where("created_at >= ? AND created_at < ?", monthStart, nextMonthStart).Count(&createdThisMonth)
	storage.DB.Model(&models.User{}).Where("created_at >= ? AND created_at < ?", prevMonthStart, monthStart).Count(&createdPreviousMonth)
	storage.DB.Model(&models.User{}).Where("created_at >= ?", todayStart).Count(&createdToday)
	storage.DB.Model(&models.User{}).Where("created_at >= ?", last30DaysStart).Count(&createdLast30Days)
	storage.DB.Model(&models.User{}).Where("created_at >= ?", thisWeekStart).Count(&createdThisWeek)
	storage.DB.Model(&models.User{}).Where("created_at >= ? AND created_at < ?", lastWeekStart, thisWeekStart).Count(&createdPreviousWeek)
	storage.DB.Model(&models.User{}).Where("created_at >= ?", last7DaysStart).Count(&rolling7d)
	storage.DB.Model(&models.User{}).Where("created_at >= ? AND created_at < ?", prev7PeriodStart, last7DaysStart).Count(&rollingPrev7d)
	storage.DB.Model(&models.User{}).Where("created_at >= ? AND created_at < ?", prev30PeriodStart, last30DaysStart).Count(&rollingPrev30d)

	// Daily buckets must use explicit YYYY-MM-DD strings. Scanning PostgreSQL
	// `date` into struct fields often yields time.Time; JSON then becomes full
	// RFC3339 timestamps, while the dashboard chart keys by date-only — mismatch
	// shows a flat zero line. TO_CHAR + GROUP BY UTC calendar day fixes that.
	type dailySignup struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}
	var dailyRaw []struct {
		DateStr string `gorm:"column:date_str"`
		Cnt     int64  `gorm:"column:cnt"`
	}
	if err := storage.DB.Model(&models.User{}).
		Select(`TO_CHAR((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS date_str, COUNT(*)::bigint AS cnt`).
		Where("created_at >= ?", last30DaysStart).
		Group("(created_at AT TIME ZONE 'UTC')::date").
		Order("(created_at AT TIME ZONE 'UTC')::date ASC").
		Scan(&dailyRaw).Error; err != nil {
		log.Printf("admin users daily_signups query: %v", err)
	}
	dailyRows := make([]dailySignup, 0, len(dailyRaw))
	for _, r := range dailyRaw {
		if r.DateStr == "" {
			continue
		}
		dailyRows = append(dailyRows, dailySignup{Date: r.DateStr, Count: r.Cnt})
	}

	ctx.JSON(iris.Map{
		"data": users,
		"meta": iris.Map{
			"page":     page,
			"per_page": perPage,
			"total":    total,
			"metrics": iris.Map{
				"total_users":                   totalUsersAll,
				"hosts_count":                   hostsCount,
				"staff_count":                   staffCount,
				"verified_identity_count":       verifiedIdentityCount,
				"created_this_month":            createdThisMonth,
				"created_previous_month":        createdPreviousMonth,
				"created_today":                 createdToday,
				"created_last_30_days":          createdLast30Days,
				"created_this_week":             createdThisWeek,
				"created_previous_week":         createdPreviousWeek,
				"rolling_7d":                    rolling7d,
				"rolling_prev_7d":               rollingPrev7d,
				"rolling_prev_30d":              rollingPrev30d,
				"daily_signups_last_30_days":    dailyRows,
			},
		},
		"links": iris.Map{},
	})
}

// Change role - PATCH /admin/users/:id/role
func AdminChangeUserRole(ctx iris.Context) {
	// Only super_admin
	// Middleware enforces super admin. Here perform change.
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid_id"})
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Role == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid_role"})
		return
	}

	var user models.User
	if err := storage.DB.First(&user, id).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "not_found"})
		return
	}

	before := user
	user.Role = body.Role
	if err := storage.DB.Save(&user).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "server_error"})
		return
	}

	// Audit
	utils.Audit(ctx, "user.role_update", "user", user.ID, before, user)

	ctx.JSON(iris.Map{"data": user})
}
