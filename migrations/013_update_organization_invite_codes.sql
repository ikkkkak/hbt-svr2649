-- Migration to update organization_invite_codes table
-- Changes from code_hash (bcrypt) to code (plaintext) for user-friendly codes
-- Adds expiry/usage limit columns

DO $$ 
BEGIN
    -- Add code column if it doesn't exist
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'code'
    ) THEN
        ALTER TABLE organization_invite_codes ADD COLUMN code VARCHAR(20);
    END IF;

    -- Add expires_at column if it doesn't exist (make it nullable for "never expires")
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'expires_at'
    ) THEN
        ALTER TABLE organization_invite_codes ADD COLUMN expires_at TIMESTAMP WITH TIME ZONE;
    END IF;

    -- Add max_uses column if it doesn't exist (nullable for unlimited)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'max_uses'
    ) THEN
        ALTER TABLE organization_invite_codes ADD COLUMN max_uses INTEGER;
    END IF;

    -- Add current_uses column if it doesn't exist
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'current_uses'
    ) THEN
        ALTER TABLE organization_invite_codes ADD COLUMN current_uses INTEGER DEFAULT 0;
    END IF;

    -- Drop old code_hash column and its unique index if they exist
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'code_hash'
    ) THEN
        -- Drop unique index on code_hash if it exists
        DROP INDEX IF EXISTS idx_organization_invite_codes_code_hash;
        DROP INDEX IF EXISTS organization_invite_codes_code_hash_key;
        -- Drop the column
        ALTER TABLE organization_invite_codes DROP COLUMN code_hash;
    END IF;

    -- Drop old used_at and used_by columns if they exist (replaced by current_uses)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'used_at'
    ) THEN
        ALTER TABLE organization_invite_codes DROP COLUMN used_at;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'organization_invite_codes' AND column_name = 'used_by'
    ) THEN
        ALTER TABLE organization_invite_codes DROP COLUMN used_by;
    END IF;

    -- Create unique index on code column if it doesn't exist
    CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_invite_codes_code ON organization_invite_codes(code) WHERE deleted_at IS NULL;

    -- Create index on expires_at if it doesn't exist
    CREATE INDEX IF NOT EXISTS idx_organization_invite_codes_expires_at ON organization_invite_codes(expires_at);
END $$;

