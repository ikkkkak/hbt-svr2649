-- Adaptive streaming metadata for rent feed videos (HLS + mobile fallback)
ALTER TABLE videos ADD COLUMN IF NOT EXISTS hls_url TEXT DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS mobile_video_url TEXT DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_status VARCHAR(32) DEFAULT 'ready';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_error TEXT DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_width INTEGER DEFAULT 0;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_height INTEGER DEFAULT 0;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS renditions_json JSONB DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_videos_processing_status ON videos(processing_status);
