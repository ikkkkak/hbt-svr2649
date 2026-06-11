-- Meskeny AI Notification Orchestrator (v1) — PostgreSQL
-- Run after deploy; AutoMigrate also creates these tables when models are registered.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS notification_candidates (
  id                  CHAR(36) PRIMARY KEY,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  notification_type   VARCHAR(64),
  title               VARCHAR(255),
  body                TEXT,
  payload             JSONB,
  image_url           VARCHAR(500),
  relevance_score     INTEGER NOT NULL DEFAULT 70,
  urgency_level       VARCHAR(20),
  property_sale_id    BIGINT,
  match_score         INTEGER,
  ai_score            INTEGER NOT NULL DEFAULT 0,
  ai_decision         VARCHAR(20) NOT NULL DEFAULT 'pending',
  ai_reason           TEXT,
  requested_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  scheduled_for       TIMESTAMPTZ,
  sent_at             TIMESTAMPTZ,
  delivered           BOOLEAN NOT NULL DEFAULT FALSE,
  opened              BOOLEAN NOT NULL DEFAULT FALSE,
  opened_at           TIMESTAMPTZ,
  dismissed           BOOLEAN NOT NULL DEFAULT FALSE,
  dismissed_at        TIMESTAMPTZ,
  batch_id            CHAR(36)
);

CREATE INDEX IF NOT EXISTS idx_nc_user_decision_sched
  ON notification_candidates (user_id, ai_decision, scheduled_for);
CREATE INDEX IF NOT EXISTS idx_nc_user_type_created
  ON notification_candidates (user_id, notification_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_nc_scheduled_send
  ON notification_candidates (scheduled_for)
  WHERE ai_decision = 'send';
CREATE INDEX IF NOT EXISTS idx_nc_batch_pending
  ON notification_candidates (user_id, ai_decision)
  WHERE ai_decision = 'batch';

CREATE TABLE IF NOT EXISTS user_notification_quota (
  id               CHAR(36) PRIMARY KEY,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  window_start     TIMESTAMPTZ NOT NULL,
  window_end       TIMESTAMPTZ NOT NULL,
  sent_count       INTEGER NOT NULL DEFAULT 0,
  opened_count     INTEGER NOT NULL DEFAULT 0,
  dismissed_count  INTEGER NOT NULL DEFAULT 0,
  daily_limit      INTEGER NOT NULL DEFAULT 4,
  last_sent_at     TIMESTAMPTZ,
  last_opened_at   TIMESTAMPTZ,
  UNIQUE (user_id, window_start)
);

CREATE INDEX IF NOT EXISTS idx_unq_user_window
  ON user_notification_quota (user_id, window_start DESC);

CREATE TABLE IF NOT EXISTS user_notification_preferences (
  id                      CHAR(36) PRIMARY KEY,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  user_id                 BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  preferred_hour_start    INTEGER,
  preferred_hour_end      INTEGER,
  peak_open_hour          INTEGER,
  peak_open_day           INTEGER,
  open_rate_7d            DOUBLE PRECISION NOT NULL DEFAULT 0.4,
  dismiss_rate_7d         DOUBLE PRECISION NOT NULL DEFAULT 0.2,
  avg_open_delay_seconds  INTEGER NOT NULL DEFAULT 0,
  match_open_rate         DOUBLE PRECISION NOT NULL DEFAULT 0.5,
  message_open_rate       DOUBLE PRECISION NOT NULL DEFAULT 0.5,
  digest_preference       BOOLEAN NOT NULL DEFAULT FALSE,
  do_not_disturb_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
  quiet_hours_start       INTEGER NOT NULL DEFAULT 23,
  quiet_hours_end         INTEGER NOT NULL DEFAULT 7,
  daily_limit_override    INTEGER NOT NULL DEFAULT 0
);
