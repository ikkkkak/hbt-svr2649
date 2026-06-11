-- Meskeny Guide: AI analyst comments on host listings (Phase 1–2)
-- Run manually on Postgres after deploy.

CREATE TABLE IF NOT EXISTS guide_comments (
    id BIGSERIAL PRIMARY KEY,
    listing_kind VARCHAR(16) NOT NULL DEFAULT 'sale',
    property_sale_id BIGINT REFERENCES property_sales(id) ON DELETE CASCADE,
    property_id BIGINT REFERENCES properties(id) ON DELETE CASCADE,
    landmark_id BIGINT REFERENCES landmarks(id) ON DELETE CASCADE,
    host_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    locale VARCHAR(8) NOT NULL DEFAULT 'en',
    parent_id BIGINT REFERENCES guide_comments(id) ON DELETE CASCADE,
    trigger_event VARCHAR(32) NOT NULL,
    severity VARCHAR(16) NOT NULL DEFAULT 'info',
    category VARCHAR(24) NOT NULL DEFAULT 'engagement',
    tone VARCHAR(16) NOT NULL DEFAULT 'clinical',
    diagnosis TEXT NOT NULL DEFAULT '',
    root_cause TEXT NOT NULL DEFAULT '',
    prescription TEXT NOT NULL DEFAULT '',
    impact_forecast TEXT NOT NULL DEFAULT '',
    algorithm_signals JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'unread',
    host_action VARCHAR(32),
    body TEXT,
    follow_up_scheduled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_guide_comments_property_sale ON guide_comments(property_sale_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_guide_comments_host ON guide_comments(host_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_guide_comments_trigger ON guide_comments(property_sale_id, trigger_event, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_guide_comments_follow_up ON guide_comments(follow_up_scheduled_at) WHERE follow_up_scheduled_at IS NOT NULL AND status = 'implemented';

CREATE TABLE IF NOT EXISTS guide_notifications (
    id BIGSERIAL PRIMARY KEY,
    comment_id BIGINT NOT NULL REFERENCES guide_comments(id) ON DELETE CASCADE,
    host_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel VARCHAR(16) NOT NULL DEFAULT 'in_app',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    deep_link TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_guide_notifications_host ON guide_notifications(host_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_guide_notifications_comment ON guide_notifications(comment_id);

CREATE TABLE IF NOT EXISTS guide_host_preferences (
    host_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    property_sale_id BIGINT NOT NULL REFERENCES property_sales(id) ON DELETE CASCADE,
    consecutive_dismissals INT NOT NULL DEFAULT 0,
    paused_until TIMESTAMPTZ,
    suppressed_categories JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (host_id, property_sale_id)
);
