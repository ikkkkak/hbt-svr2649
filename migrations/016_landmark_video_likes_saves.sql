-- Landmark video likes and saves (for landmark feed like/save)
CREATE TABLE IF NOT EXISTS landmark_video_likes (
    id SERIAL PRIMARY KEY,
    landmark_id INTEGER NOT NULL REFERENCES landmarks(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(landmark_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_landmark_video_likes_landmark ON landmark_video_likes(landmark_id);
CREATE INDEX IF NOT EXISTS idx_landmark_video_likes_user ON landmark_video_likes(user_id);

CREATE TABLE IF NOT EXISTS landmark_video_saves (
    id SERIAL PRIMARY KEY,
    landmark_id INTEGER NOT NULL REFERENCES landmarks(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(landmark_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_landmark_video_saves_landmark ON landmark_video_saves(landmark_id);
CREATE INDEX IF NOT EXISTS idx_landmark_video_saves_user ON landmark_video_saves(user_id);
