-- Broker identity verification (Meskeny verified broker program)
-- Run on PostgreSQL after deploying server with updated User model.

ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_id VARCHAR(32);
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_status VARCHAR(20) NOT NULL DEFAULT 'none';
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_verified_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_verified_by INTEGER REFERENCES users(id);
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_submitted_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_license_url TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_spoken_languages JSONB DEFAULT '[]'::jsonb;
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_rejection_notes TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_broker_id ON users (broker_id) WHERE broker_id IS NOT NULL AND broker_id <> '';
CREATE INDEX IF NOT EXISTS idx_users_broker_status ON users (broker_status);

COMMENT ON COLUMN users.broker_id IS 'Public broker ID e.g. MSK-B-100042';
COMMENT ON COLUMN users.broker_status IS 'none | pending | approved | rejected';
