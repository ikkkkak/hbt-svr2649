-- Migration to ensure GroupInvite table exists
CREATE TABLE IF NOT EXISTS group_invites (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    group_id BIGINT UNSIGNED NOT NULL,
    token VARCHAR(64) NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_by BIGINT UNSIGNED NOT NULL,
    used_by BIGINT UNSIGNED,
    used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    INDEX idx_group_invites_group_id (group_id),
    INDEX idx_group_invites_token (token),
    INDEX idx_group_invites_created_by (created_by),
    INDEX idx_group_invites_used_by (used_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

