-- Fix broker_id signup failures: GORM uniqueIndex treated '' as one global value.
-- Only non-empty broker IDs must be unique (assigned on broker approval).

UPDATE users SET broker_id = NULL WHERE broker_id = '';

-- Drop full unique indexes GORM may have created (no partial WHERE clause).
DROP INDEX IF EXISTS idx_users_broker_id;
DROP INDEX IF EXISTS uni_users_broker_id;

DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT indexname
    FROM pg_indexes
    WHERE schemaname = 'public'
      AND tablename = 'users'
      AND indexdef ILIKE '%broker_id%'
      AND indexdef NOT ILIKE '%where%'
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I', r.indexname);
  END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_broker_id
  ON users (broker_id)
  WHERE broker_id IS NOT NULL AND broker_id <> '';
