package routes

import (
	"apartments-clone-server/storage"
	"log"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// refreshUserBehaviorSummarySQL rebuilds user_behavior_summary in one set-based statement (run offline / admin).
const refreshUserBehaviorSummarySQL = `
WITH
city_counts AS (
  SELECT user_id, city_id, COUNT(*)::bigint AS cnt
  FROM user_behaviors
  WHERE user_id IS NOT NULL AND property_type = 'sale' AND city_id IS NOT NULL
    AND timestamp >= NOW() - INTERVAL '90 days' AND deleted_at IS NULL
  GROUP BY user_id, city_id
),
top_city AS (
  SELECT DISTINCT ON (user_id) user_id, city_id AS top_city_id
  FROM city_counts
  ORDER BY user_id, cnt DESC
),
zone_counts AS (
  SELECT user_id, zone_id, COUNT(*)::bigint AS cnt
  FROM user_behaviors
  WHERE user_id IS NOT NULL AND property_type = 'sale' AND zone_id IS NOT NULL
    AND timestamp >= NOW() - INTERVAL '90 days' AND deleted_at IS NULL
  GROUP BY user_id, zone_id
),
top_zone AS (
  SELECT DISTINCT ON (user_id) user_id, zone_id AS top_zone_id
  FROM zone_counts
  ORDER BY user_id, cnt DESC
),
counts90 AS (
  SELECT
    user_id,
    COALESCE(SUM(CASE WHEN interaction_type = 'view' THEN 1 ELSE 0 END), 0)::bigint AS views_90d,
    COALESCE(SUM(CASE WHEN interaction_type = 'favorite' THEN 1 ELSE 0 END), 0)::bigint AS favorites_90d,
    COALESCE(SUM(CASE WHEN interaction_type = 'contact' THEN 1 ELSE 0 END), 0)::bigint AS contacts_90d
  FROM user_behaviors
  WHERE user_id IS NOT NULL AND property_type = 'sale'
    AND timestamp >= NOW() - INTERVAL '90 days' AND deleted_at IS NULL
  GROUP BY user_id
),
avg_price AS (
  SELECT ub.user_id, COALESCE(AVG(ps.listing_price), 0)::double precision AS avg_price_180d
  FROM user_behaviors ub
  INNER JOIN property_sales ps ON ps.id = ub.property_id AND ps.deleted_at IS NULL
  WHERE ub.user_id IS NOT NULL AND ub.property_type = 'sale'
    AND ub.interaction_type IN ('favorite', 'contact', 'view')
    AND ps.listing_price > 0
    AND ub.timestamp >= NOW() - INTERVAL '180 days' AND ub.deleted_at IS NULL
  GROUP BY ub.user_id
)
INSERT INTO user_behavior_summary (
  user_id, top_city_id, top_zone_id, views_90d, favorites_90d, contacts_90d,
  avg_price_180d, last_updated, created_at, updated_at
)
SELECT
  c.user_id,
  tc.top_city_id,
  tz.top_zone_id,
  c.views_90d,
  c.favorites_90d,
  c.contacts_90d,
  COALESCE(ap.avg_price_180d, 0),
  NOW(),
  NOW(),
  NOW()
FROM counts90 c
LEFT JOIN top_city tc ON tc.user_id = c.user_id
LEFT JOIN top_zone tz ON tz.user_id = c.user_id
LEFT JOIN avg_price ap ON ap.user_id = c.user_id
ON CONFLICT (user_id) DO UPDATE SET
  top_city_id = EXCLUDED.top_city_id,
  top_zone_id = EXCLUDED.top_zone_id,
  views_90d = EXCLUDED.views_90d,
  favorites_90d = EXCLUDED.favorites_90d,
  contacts_90d = EXCLUDED.contacts_90d,
  avg_price_180d = EXCLUDED.avg_price_180d,
  last_updated = EXCLUDED.last_updated,
  updated_at = EXCLUDED.updated_at;
`

// RefreshUserBehaviorSummaryTable runs the full summary rebuild. Heavy — use cron or admin only.
func RefreshUserBehaviorSummaryTable() error {
	t0 := time.Now()
	tx := storage.DB.Exec(refreshUserBehaviorSummarySQL)
	if tx.Error != nil {
		return tx.Error
	}
	log.Printf("host_suggestions: user_behavior_summary refresh done rows=%d took=%s", tx.RowsAffected, time.Since(t0))
	return nil
}

// AdminRefreshUserBehaviorSummary POST /api/admin/insights/host-suggestions/refresh-behavior-summary
func AdminRefreshUserBehaviorSummary(ctx iris.Context) {
	if err := RefreshUserBehaviorSummaryTable(); err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"success": true, "message": "user_behavior_summary refreshed"})
}
