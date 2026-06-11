-- Broker visibility on public listing pages (default: show name + photo)
ALTER TABLE users ADD COLUMN IF NOT EXISTS broker_show_profile_on_listings BOOLEAN NOT NULL DEFAULT TRUE;

COMMENT ON COLUMN users.broker_show_profile_on_listings IS 'When false, verified broker badge remains but name/photo are hidden on listing detail screens';
