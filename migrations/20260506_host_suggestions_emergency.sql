-- Host suggestions emergency performance (run manually on Postgres when ready)
-- Safe to re-run: uses IF NOT EXISTS where applicable.

-- Summary table (also created by GORM AutoMigrate when UserBehaviorSummary is registered)
CREATE TABLE IF NOT EXISTS user_behavior_summary (
  user_id         BIGINT PRIMARY KEY,
  top_city_id     BIGINT,
  top_zone_id     BIGINT,
  views_90d       BIGINT NOT NULL DEFAULT 0,
  favorites_90d   BIGINT NOT NULL DEFAULT 0,
  contacts_90d    BIGINT NOT NULL DEFAULT 0,
  avg_price_180d  DOUBLE PRECISION NOT NULL DEFAULT 0,
  last_updated    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ,
  updated_at      TIMESTAMPTZ
);

-- Behaviors: filter paths used by candidate + enrichment
CREATE INDEX IF NOT EXISTS idx_user_behaviors_user_timestamp
  ON user_behaviors (user_id, timestamp DESC)
  WHERE user_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_behaviors_sale_user_city
  ON user_behaviors (user_id, city_id)
  WHERE property_type = 'sale' AND city_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_behaviors_sale_user_zone
  ON user_behaviors (user_id, zone_id)
  WHERE property_type = 'sale' AND zone_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_behaviors_sale_user_prop_time
  ON user_behaviors (user_id, property_id, timestamp DESC)
  WHERE property_type = 'sale' AND deleted_at IS NULL;

-- Property sales join for avg price
CREATE INDEX IF NOT EXISTS idx_property_sales_id_price
  ON property_sales (id, listing_price)
  WHERE deleted_at IS NULL AND listing_price > 0;

-- Enriched users + matches
CREATE INDEX IF NOT EXISTS idx_ai_enriched_users_user_id
  ON ai_enriched_users (user_id)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_property_matches_host_status
  ON property_matches (host_id, status)
  WHERE deleted_at IS NULL;

-- One active match row per (property, host, suggested user) for fast ON CONFLICT upserts
CREATE UNIQUE INDEX IF NOT EXISTS uq_property_matches_prop_host_suggested
  ON property_matches (property_id, host_id, suggested_user_id)
  WHERE deleted_at IS NULL;
