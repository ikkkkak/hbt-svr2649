package routes

import (
	"net/http"

	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// GET /api/admin/insights/mobile-ai
// Returns compact admin metrics for mobile investment targeting + AI usage.
func AdminGetMobileAIInsights(ctx iris.Context) {
	type counts struct {
		Value int64 `gorm:"column:value"`
	}

	var investmentEnabled counts
	if err := storage.DB.Raw(`
		SELECT COUNT(DISTINCT p.device_id) AS value
		FROM anonymous_user_preferences p
		JOIN marketing_devices m ON m.device_id = p.device_id
		WHERE m.marketing_opt_in = true
		  AND COALESCE(m.fcm_token, '') <> ''
		  AND p.interests @> '["investment"]'::jsonb
	`).Scan(&investmentEnabled).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_investment_enabled_count"})
		return
	}

	var aiAnon30d counts
	if err := storage.DB.Raw(`
		SELECT COUNT(DISTINCT session_id) AS value
		FROM ai_interactions
		WHERE user_id IS NULL
		  AND COALESCE(session_id, '') <> ''
		  AND created_at >= NOW() - INTERVAL '30 days'
	`).Scan(&aiAnon30d).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_ai_anon_count"})
		return
	}

	var aiAuth30d counts
	if err := storage.DB.Raw(`
		SELECT COUNT(DISTINCT user_id) AS value
		FROM ai_interactions
		WHERE user_id IS NOT NULL
		  AND created_at >= NOW() - INTERVAL '30 days'
	`).Scan(&aiAuth30d).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_ai_auth_count"})
		return
	}

	var aiAnonAllTime counts
	if err := storage.DB.Raw(`
		SELECT COUNT(DISTINCT session_id) AS value
		FROM ai_interactions
		WHERE user_id IS NULL
		  AND COALESCE(session_id, '') <> ''
	`).Scan(&aiAnonAllTime).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_ai_anon_all_time_count"})
		return
	}

	var aiAuthAllTime counts
	if err := storage.DB.Raw(`
		SELECT COUNT(DISTINCT user_id) AS value
		FROM ai_interactions
		WHERE user_id IS NOT NULL
	`).Scan(&aiAuthAllTime).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_ai_auth_all_time_count"})
		return
	}

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"investment_enabled_phones": investmentEnabled.Value,
			"ai_active_phones_30d":      aiAnon30d.Value + aiAuth30d.Value,
			"ai_active_phones_all_time": aiAnonAllTime.Value + aiAuthAllTime.Value,
		},
	})
}

