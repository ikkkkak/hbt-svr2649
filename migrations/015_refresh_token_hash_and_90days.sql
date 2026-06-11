-- Zero Re-Login: Add token_hash for secure storage, support 90-day refresh tokens
-- Run this migration on your database before deploying the new auth flow.

-- Add token_hash column (SHA-256 of opaque token; never store raw token)
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS token_hash TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash ON refresh_tokens(token_hash) WHERE token_hash IS NOT NULL;

-- Make token column nullable for new opaque-token records (we store only hash)
ALTER TABLE refresh_tokens ALTER COLUMN token DROP NOT NULL;
