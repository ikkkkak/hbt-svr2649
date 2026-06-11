-- Add video support to landmarks
ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS video_url TEXT;
ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS media_type VARCHAR(20) DEFAULT 'images'; -- 'images' | 'video' | 'both'
