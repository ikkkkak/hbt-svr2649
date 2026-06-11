-- Blurred first-frame placeholder for TikTok-style feed (server-generated)
ALTER TABLE videos ADD COLUMN IF NOT EXISTS preview_blur_url TEXT DEFAULT '';
