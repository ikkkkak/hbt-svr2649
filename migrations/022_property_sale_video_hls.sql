-- Adaptive streaming metadata for property sale videos (mirrors rent videos table)
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS hls_url TEXT DEFAULT '';
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS mobile_video_url TEXT DEFAULT '';
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS processing_status VARCHAR(32) DEFAULT 'pending';
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS processing_error TEXT DEFAULT '';
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS source_width INTEGER DEFAULT 0;
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS source_height INTEGER DEFAULT 0;
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS renditions_json JSONB DEFAULT '[]'::jsonb;
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS processing_progress INTEGER DEFAULT 0;
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS sprite_sheet_url TEXT DEFAULT '';
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS sprite_vtt_url TEXT DEFAULT '';
ALTER TABLE property_sale_videos ADD COLUMN IF NOT EXISTS preview_blur_url TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_property_sale_videos_processing_status ON property_sale_videos(processing_status);



