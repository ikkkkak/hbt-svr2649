-- Migration: Add 1-on-1 user blocking functionality
-- This migration adds support for users blocking other users in direct messages

-- Create user_blocks table for user-to-user blocking
CREATE TABLE user_blocks (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    blocker_id BIGINT UNSIGNED NOT NULL,
    blocked_id BIGINT UNSIGNED NOT NULL,
    reason VARCHAR(200) DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    
    INDEX idx_user_blocks_blocker_id (blocker_id),
    INDEX idx_user_blocks_blocked_id (blocked_id),
    INDEX idx_user_blocks_deleted_at (deleted_at),
    
    -- Ensure a user can only block another user once
    UNIQUE KEY unique_user_block (blocker_id, blocked_id, deleted_at)
);

-- Create direct_messages table for 1-on-1 conversations
CREATE TABLE direct_messages (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    sender_id BIGINT UNSIGNED NOT NULL,
    receiver_id BIGINT UNSIGNED NOT NULL,
    content TEXT NOT NULL,
    type VARCHAR(32) DEFAULT 'text',
    ref_type VARCHAR(32) DEFAULT NULL,
    ref_id BIGINT UNSIGNED DEFAULT NULL,
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    
    INDEX idx_direct_messages_sender_id (sender_id),
    INDEX idx_direct_messages_receiver_id (receiver_id),
    INDEX idx_direct_messages_ref_id (ref_id),
    INDEX idx_direct_messages_deleted_at (deleted_at),
    INDEX idx_direct_messages_created_at (created_at)
);

-- Add foreign key constraints
ALTER TABLE user_blocks 
ADD CONSTRAINT fk_user_blocks_blocker_id 
FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
ADD CONSTRAINT fk_user_blocks_blocked_id 
FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE direct_messages 
ADD CONSTRAINT fk_direct_messages_sender_id 
FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE CASCADE,
ADD CONSTRAINT fk_direct_messages_receiver_id 
FOREIGN KEY (receiver_id) REFERENCES users(id) ON DELETE CASCADE;
