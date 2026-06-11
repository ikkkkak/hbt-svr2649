-- Host buyer-match privacy: opt-in consent + exclusive host lock
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS share_profile_with_hosts BOOLEAN,
  ADD COLUMN IF NOT EXISTS host_share_locked_host_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_users_share_profile_with_hosts
  ON users (share_profile_with_hosts)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_host_share_locked_host_id
  ON users (host_share_locked_host_id)
  WHERE deleted_at IS NULL;
