-- Create Group Invites Table
-- This table stores invite codes/tokens that allow users to join groups
CREATE TABLE IF NOT EXISTS group_invites (
    id SERIAL PRIMARY KEY,
    group_id BIGINT UNSIGNED NOT NULL,
    token VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_by BIGINT UNSIGNED NOT NULL,
    used_by BIGINT UNSIGNED,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (used_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_group_invites_group_id ON group_invites (group_id);
CREATE INDEX IF NOT EXISTS idx_group_invites_token ON group_invites (token);
CREATE INDEX IF NOT EXISTS idx_group_invites_created_by ON group_invites (created_by);
CREATE INDEX IF NOT EXISTS idx_group_invites_expires_at ON group_invites (expires_at);
CREATE INDEX IF NOT EXISTS idx_group_invites_deleted_at ON group_invites (deleted_at);

