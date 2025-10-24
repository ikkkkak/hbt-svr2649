-- Migration: Add group quit and user blocking functionality
-- This migration adds support for users quitting groups, blocking other users, and tracking quit status

-- Add new columns to group_members table
ALTER TABLE group_members 
ADD COLUMN status VARCHAR(20) DEFAULT 'active' AFTER role,
ADD COLUMN quit_at TIMESTAMP NULL AFTER status;

-- Create index for status and quit_at for better performance
CREATE INDEX idx_group_members_status ON group_members(status);
CREATE INDEX idx_group_members_quit_at ON group_members(quit_at);

-- Create group_user_blocks table for user-to-user blocking within groups
CREATE TABLE group_user_blocks (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    group_id BIGINT UNSIGNED NOT NULL,
    blocker_id BIGINT UNSIGNED NOT NULL,
    blocked_id BIGINT UNSIGNED NOT NULL,
    reason VARCHAR(200) DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    
    INDEX idx_group_user_blocks_group_id (group_id),
    INDEX idx_group_user_blocks_blocker_id (blocker_id),
    INDEX idx_group_user_blocks_blocked_id (blocked_id),
    INDEX idx_group_user_blocks_deleted_at (deleted_at),
    
    -- Ensure a user can only block another user once per group
    UNIQUE KEY unique_group_user_block (group_id, blocker_id, blocked_id, deleted_at)
);

-- Create group_quits table for tracking quit events
CREATE TABLE group_quits (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    group_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    reason VARCHAR(200) DEFAULT '',
    quit_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    
    INDEX idx_group_quits_group_id (group_id),
    INDEX idx_group_quits_user_id (user_id),
    INDEX idx_group_quits_quit_at (quit_at),
    INDEX idx_group_quits_deleted_at (deleted_at)
);

-- Add foreign key constraints
ALTER TABLE group_user_blocks 
ADD CONSTRAINT fk_group_user_blocks_group_id 
FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
ADD CONSTRAINT fk_group_user_blocks_blocker_id 
FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
ADD CONSTRAINT fk_group_user_blocks_blocked_id 
FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE group_quits 
ADD CONSTRAINT fk_group_quits_group_id 
FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
ADD CONSTRAINT fk_group_quits_user_id 
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
