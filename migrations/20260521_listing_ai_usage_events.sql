-- Add-with-AI usage analytics (rent / sale / land listing flows)
CREATE TABLE IF NOT EXISTS listing_ai_usage_events (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL,
    event VARCHAR(24) NOT NULL,
    job_id VARCHAR(32)
);

CREATE INDEX IF NOT EXISTS idx_listing_ai_usage_created_at ON listing_ai_usage_events (created_at);
CREATE INDEX IF NOT EXISTS idx_listing_ai_usage_user_id ON listing_ai_usage_events (user_id);
CREATE INDEX IF NOT EXISTS idx_listing_ai_usage_kind ON listing_ai_usage_events (kind);
CREATE INDEX IF NOT EXISTS idx_listing_ai_usage_event ON listing_ai_usage_events (event);
CREATE INDEX IF NOT EXISTS idx_listing_ai_usage_job_id ON listing_ai_usage_events (job_id);
