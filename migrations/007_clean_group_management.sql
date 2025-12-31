-- Clean Group Management System
-- Drop existing tables if they exist
DROP TABLE IF EXISTS group_user_blocks CASCADE;
DROP TABLE IF EXISTS group_quits CASCADE;
DROP TABLE IF EXISTS user_blocks CASCADE;
DROP TABLE IF EXISTS direct_messages CASCADE;

-- Add status and quit_at to group_members if not exists
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'group_members' AND column_name = 'status') THEN
        ALTER TABLE group_members ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'group_members' AND column_name = 'quit_at') THEN
        ALTER TABLE group_members ADD COLUMN quit_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- Create group_user_blocks table
CREATE TABLE IF NOT EXISTS group_user_blocks (
    id SERIAL PRIMARY KEY,
    group_id BIGINT UNSIGNED NOT NULL,
    blocker_id BIGINT UNSIGNED NOT NULL,
    blocked_id BIGINT UNSIGNED NOT NULL,
    reason VARCHAR(200),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE (group_id, blocker_id, blocked_id)
);

-- Create group_quits table
CREATE TABLE IF NOT EXISTS group_quits (
    id SERIAL PRIMARY KEY,
    group_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    reason VARCHAR(200),
    quit_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create user_blocks table for 1-on-1 blocking
CREATE TABLE IF NOT EXISTS user_blocks (
    id SERIAL PRIMARY KEY,
    blocker_id BIGINT UNSIGNED NOT NULL,
    blocked_id BIGINT UNSIGNED NOT NULL,
    reason VARCHAR(200),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE (blocker_id, blocked_id)
);

-- Create direct_messages table
CREATE TABLE IF NOT EXISTS direct_messages (
    id SERIAL PRIMARY KEY,
    sender_id BIGINT UNSIGNED NOT NULL,
    receiver_id BIGINT UNSIGNED NOT NULL,
    content TEXT NOT NULL,
    type VARCHAR(32) DEFAULT 'text',
    ref_type VARCHAR(32),
    ref_id BIGINT UNSIGNED,
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (receiver_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_group_members_status ON group_members (status);
CREATE INDEX IF NOT EXISTS idx_group_members_quit_at ON group_members (quit_at);
CREATE INDEX IF NOT EXISTS idx_group_user_blocks_group_blocker_blocked ON group_user_blocks (group_id, blocker_id, blocked_id);
CREATE INDEX IF NOT EXISTS idx_group_quits_group_user ON group_quits (group_id, user_id);
CREATE INDEX IF NOT EXISTS idx_user_blocks_blocker_blocked ON user_blocks (blocker_id, blocked_id);
CREATE INDEX IF NOT EXISTS idx_direct_messages_sender_receiver ON direct_messages (sender_id, receiver_id);
CREATE INDEX IF NOT EXISTS idx_direct_messages_ref_id ON direct_messages (ref_id);
CREATE INDEX IF NOT EXISTS idx_direct_messages_deleted_at ON direct_messages (deleted_at);
CREATE INDEX IF NOT EXISTS idx_direct_messages_created_at ON direct_messages (created_at);
