-- Auto property slideshow video generation + internal music library
-- Also applied via GORM AutoMigrate on deploy.

CREATE TABLE IF NOT EXISTS music_tracks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  title VARCHAR(200) NOT NULL,
  category VARCHAR(64) NOT NULL DEFAULT 'default',
  file_url VARCHAR(1024) NOT NULL DEFAULT '',
  duration_sec DOUBLE PRECISION DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  notes TEXT
);
CREATE INDEX IF NOT EXISTS idx_music_tracks_category ON music_tracks (category) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_music_tracks_active ON music_tracks (is_active) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS property_video_generation_jobs (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  user_id BIGINT NOT NULL,
  entity_type VARCHAR(16) NOT NULL,
  entity_id BIGINT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  progress INTEGER NOT NULL DEFAULT 0,
  error_message TEXT,
  image_urls JSONB,
  music_track_id BIGINT,
  output_video_url VARCHAR(1024) DEFAULT '',
  property_type VARCHAR(64) DEFAULT '',
  overlay_title VARCHAR(300) DEFAULT '',
  overlay_location VARCHAR(300) DEFAULT '',
  overlay_area VARCHAR(64) DEFAULT '',
  overlay_price VARCHAR(64) DEFAULT '',
  overlay_cta VARCHAR(120) DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pvg_jobs_entity ON property_video_generation_jobs (entity_type, entity_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_pvg_jobs_status ON property_video_generation_jobs (status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_pvg_jobs_user ON property_video_generation_jobs (user_id) WHERE deleted_at IS NULL;

COMMENT ON TABLE music_tracks IS 'Internal Meskeny music library for auto listing videos (no external APIs).';
COMMENT ON TABLE property_video_generation_jobs IS 'Async FFmpeg slideshow jobs: images → vertical MP4 → feed.';
