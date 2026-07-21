package storage

import (
	"apartments-clone-server/models"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// SQLDB exposes the underlying *sql.DB for fast raw queries (create listing, etc.).
func SQLDB() (*sql.DB, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return DB.DB()
}

func connectToDB() *gorm.DB {
	// Only load .env in development (when RENDER env var is not set)
	if os.Getenv("RENDER") == "" {
		err := godotenv.Load()
		if err != nil {
			log.Println("Warning: Could not load .env file (this is normal in production)")
		}
	}

	dsn := os.Getenv("DB_CONNECTION_STRING")
	if dsn == "" {
		log.Panic("DB_CONNECTION_STRING environment variable is required")
	}
	if !strings.Contains(dsn, "connect_timeout") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "connect_timeout=5"
	}
	// Optional query timeout — do NOT default to 15s (kills legitimate slow reads under load).
	// Set DB_STATEMENT_TIMEOUT_MS=45000 in env if you need a safety cap.
	if timeoutMs := envInt("DB_STATEMENT_TIMEOUT_MS", 0); timeoutMs > 0 {
		if !strings.Contains(dsn, "statement_timeout") {
			if strings.Contains(dsn, " ") && !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
				dsn += fmt.Sprintf(" options='-c statement_timeout=%d'", timeoutMs)
			} else {
				sep := "?"
				if strings.Contains(dsn, "?") {
					sep = "&"
				}
				dsn += fmt.Sprintf("%sstatement_timeout=%d", sep, timeoutMs)
			}
		}
	}

	slowMs := envInt("DB_SLOW_MS", 2000)
	logLevel := logger.Warn
	if os.Getenv("DB_LOG_SQL") == "0" {
		logLevel = logger.Error
	} else if os.Getenv("DB_LOG_SQL") == "1" {
		logLevel = logger.Info
	}
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Duration(slowMs) * time.Millisecond,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, dbError := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
	if dbError != nil {
		log.Panic("error connection to db: " + dbError.Error())
	}

	// Connection pool tuning (important on Cloud Run/Cloud SQL to avoid stalls/timeouts).
	if sqlDB, err := db.DB(); err == nil {
		maxOpen := envInt("DB_MAX_OPEN_CONNS", 50)
		maxIdle := envInt("DB_MAX_IDLE_CONNS", 15)
		sqlDB.SetMaxOpenConns(maxOpen)
		sqlDB.SetMaxIdleConns(maxIdle)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
		log.Printf("🔧 DB pool maxOpen=%d maxIdle=%d", maxOpen, maxIdle)
	} else {
		log.Printf("⚠️ Could not access sql.DB for pool tuning: %v", err)
	}

	DB = db
	return db
}

func performMigrations(db *gorm.DB) {
	ensureOrganizationInviteCodesBackfill(db)

	err := db.AutoMigrate(
		&models.Conversation{}, // create table containing many side first
		&models.Message{},
		&models.User{},
		&models.Property{},
		&models.Review{},
		&models.Reservation{},
		&models.Collection{},
		&models.CollectionProperty{},
		&models.Video{},
		&models.VideoLike{},
		&models.VideoSave{},
		&models.VideoComment{},
		&models.VideoCommentLike{},
		&models.VideoFeedHistory{},
		&models.Experience{},
		&models.ExperienceBooking{},
		&models.ExperienceCollection{},
		&models.ExperienceCollectionItem{},
		&models.ExperienceInvite{},
		&models.ExperienceParticipant{},
		&models.ExperienceGroup{},
		&models.ExperienceGroupMember{},
		&models.ExperienceAvailability{},
		&models.ChatMessage{},
		&models.GroupWishlistItem{},
		&models.GroupWishlistLike{},
		&models.GroupJoinRequest{},
		&models.Notification{},
		&models.UserProfile{},
		&models.PropertyAvailability{},
		&models.PropertyPricing{},
		&models.PropertyDiscount{},
		&models.PropertyBlock{},
		&models.LocationCriteria{},
		&models.LocationCriteriaProperty{},
		&models.IdentityVerification{},
		&models.AuditLog{},
		&models.Feedback{},
		// Categories and Amenities (for property sales)
		&models.Category{},
		&models.Amenity{},
		// Property Selling System Models
		&models.Organization{},
		&models.Agent{},
		&models.OrganizationMember{},     // RBAC member system
		&models.OrganizationInviteCode{}, // Secure invite codes
		&models.OrganizationAuditLog{},   // Audit logging
		&models.PropertySale{},
		&models.PropertyPlace{},
		&models.PropertyTour{},
		&models.PropertyInquiry{},
		&models.GuideComment{},
		&models.GuideNotification{},
		&models.GuideHostPreference{},
		&models.PropertyOffer{},
		&models.PropertySaleVideo{},
		&models.PropertySaleVideoLike{},
		&models.PropertySaleVideoSave{},
		&models.PropertySaleVideoComment{},
		&models.PropertySaleVideoCommentLike{},
		&models.PropertySaleVideoReport{},
		&models.DeviceRegistration{},
		&models.DeviceSession{},
		&models.HiddenPropertySaleVideo{},
		&models.Landmark{},
		&models.LandmarkVideoLike{},
		&models.LandmarkVideoSave{},
		&models.NotificationPreference{},
		&models.MarketingDevice{},
		&models.CrashLog{},
		// Video Reporting System Models
		&models.VideoReport{},
		&models.UserFlag{},
		&models.HiddenVideo{},
		&models.Banner{},
		// Stories
		&models.Story{},
		&models.StoryView{},
		&models.StoryLike{},
		// Group Management Models
		&models.Group{},
		&models.GroupMember{},
		&models.GroupInvite{},
		&models.GroupMessage{},
		&models.GroupMessageRead{},
		&models.GroupBan{},
		&models.GroupUserBlock{},
		&models.GroupQuit{},
		&models.VideoViewer{},
		&models.VideoView{},
		// Direct Messages and User Blocking
		&models.DirectMessage{},
		&models.UserBlock{},
		&models.MessageReaction{},
		// Host Mode Tracking
		&models.HostModeSwitch{},
		&models.HostModeInteraction{},
		// User Behavior Tracking
		&models.UserBehavior{},
		&models.AnonymousUserPreference{}, // Anonymous user preferences for intelligent notifications
		&models.PropertyFeedSeen{}, // Smart property feed seen-history
		// Token Management
		&models.RefreshToken{},
		// Recommendation & notification system (TikTok-style)
		&models.Interaction{},
		&models.ClientMutation{},
		&models.RecommendationCache{},
		&models.NotificationEvent{},
		&models.NotificationDeliveryLog{},
		&models.DiscoveryEngagementLog{},
		&models.GoldPropertyStat{},
		// MeskenyGPT AI analytics
		&models.AIInteraction{},
		&models.AIFeedback{},
		&models.MarketSnapshot{},
		&models.MeskenyKnowledgeEntry{},
		&models.ScrapedSource{},
		&models.ScrapedListing{},
		&models.AIEscalation{},
		&models.AINotification{},
		&models.AIConversationMemory{},
		&models.PropertyFeedSeen{},
		&models.ListingAIUsageEvent{},
		&models.WhatsAppShareUsageEvent{},
		&models.HabitatPlan{},
		&models.HabitatSector{},
		&models.HabitatSubSector{},
		&models.HabitatPlot{},
		&models.MusicTrack{},
		&models.PropertyVideoGenerationJob{},
	)
	if err != nil {
		log.Printf("❌ AutoMigrate error: %v", err)
	}

	ensureBrokerIDPartialUniqueIndex(db)
	ensureSlideshowVideoSystem(db)

	ensureHabitatGISRelations(db)
	ensureHabitatSubSectorColumns(db)
	ensureHabitatPostGIS(db)
	ensureLandmarkWishlistTables(db)

	// CRITICAL: Ensure smart-feed seen table exists even if AutoMigrate partially fails.
	// This prevents the app from behaving like there is no rotation state.
	if !db.Migrator().HasTable(&models.PropertyFeedSeen{}) {
		log.Println("⚠️ property_feed_seen table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.PropertyFeedSeen{}); err != nil {
			log.Printf("❌ Failed to create property_feed_seen: %v", err)
		} else {
			log.Println("✅ property_feed_seen table created successfully")
		}
	}
	// MeskenyGPT admin knowledge (RAG) — ensure table exists if AutoMigrate skipped or failed partway.
	if !db.Migrator().HasTable(&models.MeskenyKnowledgeEntry{}) {
		log.Println("⚠️ meskeny_knowledge_entries table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.MeskenyKnowledgeEntry{}); err != nil {
			log.Printf("❌ Failed to create meskeny_knowledge_entries: %v", err)
		} else {
			log.Println("✅ meskeny_knowledge_entries table created successfully")
		}
	}
	if !db.Migrator().HasTable(&models.WhatsAppShareUsageEvent{}) {
		log.Println("⚠️ whatsapp_share_usage_events table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.WhatsAppShareUsageEvent{}); err != nil {
			log.Printf("❌ Failed to create whatsapp_share_usage_events: %v", err)
		} else {
			log.Println("✅ whatsapp_share_usage_events table created successfully")
		}
	}
	// Indexes for fast RAG snippet selection (active + locale + priority).
	db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_meskeny_knowledge_active_locale_priority
		ON meskeny_knowledge_entries(active, locale, priority DESC, id DESC)
		WHERE deleted_at IS NULL
	`)
	// Make sure the unique indexes used by smart-feed de-dupe are present.
	// (Partial indexes to support both user_id and anonymous device_id.)
	db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_property_feed_seen_user_property
		ON property_feed_seen(user_id, property_id)
		WHERE user_id IS NOT NULL
	`)
	db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_property_feed_seen_device_property
		ON property_feed_seen(device_id, property_id)
		WHERE user_id IS NULL
	`)
	db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_property_feed_seen_user_seen_at
		ON property_feed_seen(user_id, seen_at DESC)
		WHERE user_id IS NOT NULL
	`)
	db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_property_feed_seen_device_seen_at
		ON property_feed_seen(device_id, seen_at DESC)
		WHERE user_id IS NULL
	`)

	// Ensure users.id has a sequence default (fix "null value in column id" on INSERT)
	db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_sequences WHERE schemaname = 'public' AND sequencename = 'users_id_seq') THEN
				CREATE SEQUENCE users_id_seq;
				ALTER SEQUENCE users_id_seq OWNED BY users.id;
			END IF;
			ALTER TABLE users ALTER COLUMN id SET DEFAULT nextval('users_id_seq'::regclass);
			PERFORM setval('users_id_seq', GREATEST(COALESCE((SELECT MAX(id) FROM users), 0), 1));
		EXCEPTION
			WHEN undefined_table THEN NULL;
			WHEN undefined_column THEN NULL;
		END $$;
	`)

	// CRITICAL: Ensure refresh_tokens table exists (safety check)
	if !db.Migrator().HasTable(&models.RefreshToken{}) {
		log.Println("⚠️ refresh_tokens table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.RefreshToken{}); err != nil {
			log.Printf("❌ Failed to create refresh_tokens table: %v", err)
		} else {
			log.Println("✅ refresh_tokens table created successfully")
		}
	} else {
		log.Println("✅ refresh_tokens table exists")
	}

	// Ensure refresh_tokens has token_hash column and proper constraints (idempotent).
	// Mirrors migrations/015_refresh_token_hash_and_90days.sql but safe to run on every startup.
	db.Exec(`ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS token_hash TEXT`)
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash ON refresh_tokens(token_hash) WHERE token_hash IS NOT NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash_not_deleted ON refresh_tokens(token_hash, deleted_at) WHERE token_hash IS NOT NULL`)
	db.Exec(`ALTER TABLE refresh_tokens ALTER COLUMN token DROP NOT NULL`)

	// notification_preferences: backfill missing smart notification columns on old DBs.
	// Prevents 500 on login when inserting/updating preference payloads from mobile.
	db.Exec(`ALTER TABLE notification_preferences ADD COLUMN IF NOT EXISTS timezone VARCHAR(64) DEFAULT ''`)
	db.Exec(`ALTER TABLE notification_preferences ADD COLUMN IF NOT EXISTS quiet_start_hour INTEGER DEFAULT 22`)
	db.Exec(`ALTER TABLE notification_preferences ADD COLUMN IF NOT EXISTS quiet_end_hour INTEGER DEFAULT 7`)
	db.Exec(`ALTER TABLE notification_preferences ADD COLUMN IF NOT EXISTS max_smart_per_day INTEGER DEFAULT 2`)
	// videos: adaptive streaming (HLS) columns
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS hls_url TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS mobile_video_url TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_status VARCHAR(32) DEFAULT 'ready'`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_error TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_width INTEGER DEFAULT 0`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS source_height INTEGER DEFAULT 0`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS renditions_json JSONB DEFAULT '[]'::jsonb`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_videos_processing_status ON videos(processing_status)`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_progress INTEGER DEFAULT 0`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS sprite_sheet_url TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE videos ADD COLUMN IF NOT EXISTS sprite_vtt_url TEXT DEFAULT ''`)

	// landmarks: admin-curated high-visibility flag (used in ranking & badges).
	db.Exec(`ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS is_gold BOOLEAN DEFAULT false`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_landmarks_is_gold ON landmarks(is_gold)`)

	// property_places: nearby restaurants, hospitals, schools (Google Places) — ensure table exists
	if !db.Migrator().HasTable(&models.PropertyPlace{}) {
		log.Println("⚠️ property_places table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.PropertyPlace{}); err != nil {
			log.Printf("❌ Failed to create property_places table: %v", err)
		} else {
			log.Println("✅ property_places table created successfully")
		}
	}
	if db.Migrator().HasTable(&models.PropertyPlace{}) {
		db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS property_place_unique ON property_places(property_sale_id, place_id)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS property_places_property_id_idx ON property_places(property_sale_id)`)
	}

	// Allow direct chat groups without an experience by making experience_id nullable
	db.Exec("ALTER TABLE experience_groups ALTER COLUMN experience_id DROP NOT NULL;")

	// Allow promotional videos without a property by making property_id nullable
	db.Exec("ALTER TABLE videos ALTER COLUMN property_id DROP NOT NULL;")

	// Make organization_id nullable in property_sales to allow individual owners
	db.Exec("ALTER TABLE property_sales ALTER COLUMN organization_id DROP NOT NULL;")

	// Add truckeck column (quality control validated by admin) - IF NOT EXISTS for idempotency
	db.Exec("ALTER TABLE property_sales ADD COLUMN IF NOT EXISTS truckeck BOOLEAN DEFAULT FALSE;")
	// Host-private notes for sale/rent/landmark creation flows (owner/internal use only)
	db.Exec("ALTER TABLE property_sales ADD COLUMN IF NOT EXISTS host_private_note TEXT;")
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS host_private_note TEXT;")
	db.Exec("ALTER TABLE properties ADD COLUMN IF NOT EXISTS host_private_note TEXT;")

	// Landmarks: structured location + cadastre plot confirmation (see migrations/20260526_landmark_location_verification.sql)
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS city_id BIGINT;")
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS zone_id BIGINT;")
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS quartier_id BIGINT;")
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS habitat_plot_id BIGINT;")
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS plot_confirmed BOOLEAN NOT NULL DEFAULT false;")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_landmarks_city_id ON landmarks(city_id);")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_landmarks_zone_id ON landmarks(zone_id);")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_landmarks_quartier_id ON landmarks(quartier_id);")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_landmarks_habitat_plot_id ON landmarks(habitat_plot_id);")

	// Add true_broker column to users (admin-verified broker; all their properties show TrueBroker)
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS true_broker BOOLEAN DEFAULT FALSE;")

	// Banners table for promotional content in property sale feed
	db.Exec(`
		CREATE TABLE IF NOT EXISTS banners (
			id SERIAL PRIMARY KEY,
			image_url TEXT NOT NULL,
			link_url TEXT DEFAULT '',
			width INTEGER DEFAULT 800,
			height INTEGER DEFAULT 200,
			sort_order INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			deleted_at TIMESTAMP WITH TIME ZONE
		);
	`)
	db.Exec("ALTER TABLE banners ADD COLUMN IF NOT EXISTS width INTEGER DEFAULT 800;")
	db.Exec("ALTER TABLE banners ADD COLUMN IF NOT EXISTS height INTEGER DEFAULT 200;")

	// Update foreign key constraint for property_sales to allow NULL
	db.Exec(`
		DO $$ 
		BEGIN
			-- Drop existing constraint if it exists
			IF EXISTS (
				SELECT 1 FROM pg_constraint 
				WHERE conname = 'property_sales_organization_id_fkey'
			) THEN
				ALTER TABLE property_sales DROP CONSTRAINT property_sales_organization_id_fkey;
			END IF;
			
			-- Recreate with ON DELETE SET NULL
			ALTER TABLE property_sales 
				ADD CONSTRAINT property_sales_organization_id_fkey 
				FOREIGN KEY (organization_id) 
				REFERENCES organizations(id) 
				ON DELETE SET NULL;
		END $$;
	`)

	// Make organization_id nullable in landmarks to allow individual owners
	db.Exec("ALTER TABLE landmarks ALTER COLUMN organization_id DROP NOT NULL;")

	// Landmarks: make map highlight optional (allow NULL coordinates) + optional lots
	// NOTE: AutoMigrate does NOT reliably drop NOT NULL constraints, so we do a best-effort SQL fix.
	db.Exec("ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS lots INTEGER;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point1_lat DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point1_lng DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point2_lat DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point2_lng DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point3_lat DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point3_lng DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point4_lat DROP NOT NULL;")
	db.Exec("ALTER TABLE landmarks ALTER COLUMN point4_lng DROP NOT NULL;")

	// Update foreign key constraint for landmarks to allow NULL
	db.Exec(`
		DO $$ 
		BEGIN
			-- Drop existing constraint if it exists
			IF EXISTS (
				SELECT 1 FROM pg_constraint 
				WHERE conname = 'landmarks_organization_id_fkey'
			) THEN
				ALTER TABLE landmarks DROP CONSTRAINT landmarks_organization_id_fkey;
			END IF;
			
			-- Recreate with ON DELETE SET NULL
			ALTER TABLE landmarks 
				ADD CONSTRAINT landmarks_organization_id_fkey 
				FOREIGN KEY (organization_id) 
				REFERENCES organizations(id) 
				ON DELETE SET NULL;
		END $$;
	`)

	// Add owner_id to property_sales to track individual owners
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add owner_id column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'property_sales' AND column_name = 'owner_id'
			) THEN
				ALTER TABLE property_sales ADD COLUMN owner_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
				CREATE INDEX IF NOT EXISTS idx_property_sales_owner_id ON property_sales(owner_id);
			END IF;
		END $$;
	`)

	// Backfill owner_id for existing properties based on organization ownership
	db.Exec(`
		UPDATE property_sales ps
		SET owner_id = o.owner_id
		FROM organizations o
		WHERE ps.organization_id = o.id
		AND ps.owner_id IS NULL
		AND o.owner_id IS NOT NULL;
	`)

	// For individual properties (organization_id IS NULL) created recently without owner_id,
	// we can't automatically determine the owner, so they will need to be manually updated
	// or the user will need to recreate them. For now, we'll leave them as-is.
	// Users can contact support to have their properties linked to their account.

	// Add owner_id to landmarks to track individual owners
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add owner_id column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'landmarks' AND column_name = 'owner_id'
			) THEN
				ALTER TABLE landmarks ADD COLUMN owner_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
				CREATE INDEX IF NOT EXISTS idx_landmarks_owner_id ON landmarks(owner_id);
			END IF;
		END $$;
	`)

	// Backfill owner_id for existing landmarks based on organization ownership
	db.Exec(`
		UPDATE landmarks l
		SET owner_id = o.owner_id
		FROM organizations o
		WHERE l.organization_id = o.id
		AND l.owner_id IS NULL
		AND o.owner_id IS NOT NULL;
	`)

	// Update organization_invite_codes table structure
	// Add code column if it doesn't exist
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add code column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'organization_invite_codes' AND column_name = 'code'
			) THEN
				ALTER TABLE organization_invite_codes ADD COLUMN code VARCHAR(20);
			END IF;

			-- Backfill legacy rows before NOT NULL constraint
			UPDATE organization_invite_codes
			SET code = 'LEG-' || LPAD(id::text, 8, '0')
			WHERE code IS NULL OR TRIM(code) = '';

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
	`)

	// Make code column NOT NULL after migration (GORM will handle this via AutoMigrate, but ensure it's set)
	// Only if the column exists and there are no NULL values
	db.Exec(`
		DO $$ 
		BEGIN
			-- Only add NOT NULL constraint if code column exists and has no NULL values
			IF EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'organization_invite_codes' AND column_name = 'code'
			) AND NOT EXISTS (
				SELECT 1 FROM organization_invite_codes WHERE code IS NULL
			) THEN
				-- Alter column to NOT NULL (this will fail if there are NULLs, but we checked)
				ALTER TABLE organization_invite_codes ALTER COLUMN code SET NOT NULL;
			END IF;
		END $$;
	`)

	// Remove foreign key constraint from property_sale_video_comments.property_sale_video_id
	// because property sale videos are synthetic (stored in PropertySale.Videos array, not as separate records)
	db.Exec(`
		DO $$ 
		DECLARE
			constraint_name_var TEXT;
		BEGIN
			-- Find and drop the foreign key constraint by checking pg_constraint
			SELECT conname INTO constraint_name_var
			FROM pg_constraint
			WHERE conrelid = 'property_sale_video_comments'::regclass
			AND contype = 'f'
			AND confrelid = 'property_sale_videos'::regclass
			LIMIT 1;
			
			IF constraint_name_var IS NOT NULL THEN
				EXECUTE 'ALTER TABLE property_sale_video_comments DROP CONSTRAINT ' || constraint_name_var;
			END IF;
		EXCEPTION
			WHEN OTHERS THEN
				-- If constraint doesn't exist or error occurs, continue
				NULL;
		END $$;
	`)

	// Also try dropping by the common naming patterns (if the above didn't work)
	db.Exec(`ALTER TABLE property_sale_video_comments DROP CONSTRAINT IF EXISTS fk_property_sale_video_comments_property_sale_video;`)
	db.Exec(`ALTER TABLE property_sale_video_comments DROP CONSTRAINT IF EXISTS property_sale_video_comments_property_sale_video_id_fkey;`)

	// Ensure property_sale_video_comment_likes table exists
	// This is a safety check in case AutoMigrate didn't create it
	db.Exec(`
		DO $$ 
		BEGIN
			-- Create table if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.tables 
				WHERE table_name = 'property_sale_video_comment_likes'
			) THEN
				CREATE TABLE property_sale_video_comment_likes (
					id SERIAL PRIMARY KEY,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMP WITH TIME ZONE,
					comment_id INTEGER NOT NULL,
					user_id INTEGER NOT NULL,
					CONSTRAINT fk_property_sale_video_comment_likes_comment 
						FOREIGN KEY (comment_id) 
						REFERENCES property_sale_video_comments(id) 
						ON DELETE CASCADE,
					CONSTRAINT fk_property_sale_video_comment_likes_user 
						FOREIGN KEY (user_id) 
						REFERENCES users(id) 
						ON DELETE CASCADE
				);
				
				-- Create indexes
				CREATE INDEX idx_property_sale_video_comment_likes_comment_id ON property_sale_video_comment_likes(comment_id);
				CREATE INDEX idx_property_sale_video_comment_likes_user_id ON property_sale_video_comment_likes(user_id);
				CREATE INDEX idx_property_sale_video_comment_likes_deleted_at ON property_sale_video_comment_likes(deleted_at);
				
				-- Create unique index to prevent duplicate likes
				CREATE UNIQUE INDEX idx_property_sale_video_comment_likes_unique 
					ON property_sale_video_comment_likes(comment_id, user_id) 
					WHERE deleted_at IS NULL;
			END IF;
		END $$;
	`)

	// Add favorite city columns to users table for intelligent notifications
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add favorite_city_id column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'users' AND column_name = 'favorite_city_id'
			) THEN
				ALTER TABLE users ADD COLUMN favorite_city_id INTEGER;
				CREATE INDEX IF NOT EXISTS idx_users_favorite_city_id ON users(favorite_city_id);
			END IF;

			-- Add favorite_city_name column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'users' AND column_name = 'favorite_city_name'
			) THEN
				ALTER TABLE users ADD COLUMN favorite_city_name VARCHAR(255);
			END IF;

			-- Add favorite_zone_id column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'users' AND column_name = 'favorite_zone_id'
			) THEN
				ALTER TABLE users ADD COLUMN favorite_zone_id INTEGER;
				CREATE INDEX IF NOT EXISTS idx_users_favorite_zone_id ON users(favorite_zone_id);
			END IF;

			-- Add favorite_zone_name column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'users' AND column_name = 'favorite_zone_name'
			) THEN
				ALTER TABLE users ADD COLUMN favorite_zone_name VARCHAR(255);
			END IF;
		END $$;
	`)

	// Add saved_property_sales column to users table for property sale favorites
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add saved_property_sales column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'users' AND column_name = 'saved_property_sales'
			) THEN
				ALTER TABLE users ADD COLUMN saved_property_sales JSONB DEFAULT '[]'::jsonb;
			END IF;
		END $$;
	`)

	// Add device_id column to notification_preferences table for anonymous user tracking
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add device_id column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'notification_preferences' AND column_name = 'device_id'
			) THEN
				ALTER TABLE notification_preferences ADD COLUMN device_id VARCHAR(255);
				CREATE INDEX IF NOT EXISTS idx_notification_preferences_device_id ON notification_preferences(device_id);
			END IF;
		END $$;
	`)

	// Add property management and view tracking columns to property_sales table
	db.Exec(`
		DO $$ 
		BEGIN
			-- Add view_count column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'property_sales' AND column_name = 'view_count'
			) THEN
				ALTER TABLE property_sales ADD COLUMN view_count BIGINT DEFAULT 0;
				CREATE INDEX IF NOT EXISTS idx_property_sales_view_count ON property_sales(view_count);
			END IF;
			
			-- Add is_deactivated column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'property_sales' AND column_name = 'is_deactivated'
			) THEN
				ALTER TABLE property_sales ADD COLUMN is_deactivated BOOLEAN DEFAULT FALSE;
				CREATE INDEX IF NOT EXISTS idx_property_sales_is_deactivated ON property_sales(is_deactivated);
			END IF;
			
			-- Add is_sold column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'property_sales' AND column_name = 'is_sold'
			) THEN
				ALTER TABLE property_sales ADD COLUMN is_sold BOOLEAN DEFAULT FALSE;
				CREATE INDEX IF NOT EXISTS idx_property_sales_is_sold ON property_sales(is_sold);
			END IF;
			
			-- Add last_milestone_notified column if it doesn't exist
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_name = 'property_sales' AND column_name = 'last_milestone_notified'
			) THEN
				ALTER TABLE property_sales ADD COLUMN last_milestone_notified BIGINT DEFAULT 0;
			END IF;
		END $$;
	`)
}

// ensureLandmarkWishlistTables creates landmark like/save tables if missing (wishlist hearts).
// Mirrors migrations/016_landmark_video_likes_saves.sql — safe on every startup.
func ensureLandmarkWishlistTables(db *gorm.DB) {
	if !db.Migrator().HasTable(&models.Landmark{}) {
		log.Println("⚠️ landmarks table missing; skipping landmark_video_saves setup")
		return
	}

	if !db.Migrator().HasTable(&models.LandmarkVideoLike{}) {
		log.Println("⚠️ landmark_video_likes table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.LandmarkVideoLike{}); err != nil {
			log.Printf("❌ GORM create landmark_video_likes failed: %v — trying SQL fallback", err)
		}
	}

	if !db.Migrator().HasTable(&models.LandmarkVideoSave{}) {
		log.Println("⚠️ landmark_video_saves table not found, creating it...")
		if err := db.Migrator().CreateTable(&models.LandmarkVideoSave{}); err != nil {
			log.Printf("❌ GORM create landmark_video_saves failed: %v — trying SQL fallback", err)
		}
	}

	// Idempotent SQL (handles partial GORM failures and old DBs that never ran migration 016).
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS landmark_video_likes (
			id SERIAL PRIMARY KEY,
			landmark_id INTEGER NOT NULL REFERENCES landmarks(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			UNIQUE(landmark_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_landmark_video_likes_landmark ON landmark_video_likes(landmark_id)`,
		`CREATE INDEX IF NOT EXISTS idx_landmark_video_likes_user ON landmark_video_likes(user_id)`,
		`CREATE TABLE IF NOT EXISTS landmark_video_saves (
			id SERIAL PRIMARY KEY,
			landmark_id INTEGER NOT NULL REFERENCES landmarks(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			UNIQUE(landmark_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_landmark_video_saves_landmark ON landmark_video_saves(landmark_id)`,
		`CREATE INDEX IF NOT EXISTS idx_landmark_video_saves_user ON landmark_video_saves(user_id)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			log.Printf("❌ ensureLandmarkWishlistTables SQL: %v", err)
			return
		}
	}

	if db.Migrator().HasTable(&models.LandmarkVideoSave{}) {
		log.Println("✅ landmark_video_saves table ready")
	}
}

// ensureHabitatGISRelations adds plan_id on plots and indexes (idempotent).
func ensureHabitatGISRelations(db *gorm.DB) {
	if !db.Migrator().HasTable(&models.HabitatPlot{}) {
		return
	}
	if !db.Migrator().HasColumn(&models.HabitatPlot{}, "PlanID") {
		_ = db.Migrator().AddColumn(&models.HabitatPlot{}, "PlanID")
	}
	db.Exec(`
		UPDATE habitat_plots p
		SET plan_id = s.plan_id
		FROM habitat_sectors s
		WHERE p.sector_id = s.id AND (p.plan_id IS NULL OR p.plan_id <> s.plan_id)
	`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_habitat_plots_plan ON habitat_plots(plan_id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_habitat_plots_plan_sector ON habitat_plots(plan_id, sector_id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_habitat_plots_number ON habitat_plots(plot_number)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_habitat_sectors_plan ON habitat_sectors(plan_id)`)
	// GORM may create geom_geo_json; migrations/import use geom_geojson — align once.
	db.Exec(`
		DO $$ BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'habitat_plots' AND column_name = 'geom_geo_json'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'habitat_plots' AND column_name = 'geom_geojson'
			) THEN
				ALTER TABLE habitat_plots RENAME COLUMN geom_geo_json TO geom_geojson;
			ELSIF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'habitat_plots' AND column_name = 'geom_geo_json'
			) AND EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'habitat_plots' AND column_name = 'geom_geojson'
			) THEN
				UPDATE habitat_plots
				SET geom_geojson = geom_geo_json
				WHERE (geom_geojson IS NULL OR geom_geojson::text = 'null')
				  AND geom_geo_json IS NOT NULL
				  AND geom_geo_json::text <> 'null';
				ALTER TABLE habitat_plots DROP COLUMN geom_geo_json;
			END IF;
		END $$
	`)
	var merged int64
	db.Raw(`
		SELECT COUNT(*) FROM habitat_plots
		WHERE geom_geojson IS NOT NULL AND geom_geojson::text <> 'null'
	`).Scan(&merged)
	if merged > 0 {
		log.Printf("✅ habitat_plots geom_geojson rows with geometry: %d", merged)
	}
	// GORM may create bounds_geo_json; migrations use bounds_geojson — align on plans + sectors.
	for _, table := range []string{"habitat_plans", "habitat_sectors"} {
		db.Exec(fmt.Sprintf(`
			DO $$ BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = '%s' AND column_name = 'bounds_geo_json'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = '%s' AND column_name = 'bounds_geojson'
				) THEN
					ALTER TABLE %s RENAME COLUMN bounds_geo_json TO bounds_geojson;
				ELSIF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = '%s' AND column_name = 'bounds_geo_json'
				) AND EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = '%s' AND column_name = 'bounds_geojson'
				) THEN
					UPDATE %s
					SET bounds_geojson = bounds_geo_json
					WHERE (bounds_geojson IS NULL OR bounds_geojson::text = 'null')
					  AND bounds_geo_json IS NOT NULL
					  AND bounds_geo_json::text <> 'null';
					ALTER TABLE %s DROP COLUMN bounds_geo_json;
				END IF;
			END $$
		`, table, table, table, table, table, table, table))
	}
	// GORM may create a broken unique index on plot_number alone — replace with (sector_id, plot_number).
	db.Exec(`DROP INDEX IF EXISTS unique_plot_per_sector`)
	db.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'unique_plot_per_sector'
			) THEN
				ALTER TABLE habitat_plots
					ADD CONSTRAINT unique_plot_per_sector UNIQUE (sector_id, plot_number);
			END IF;
		EXCEPTION
			WHEN duplicate_object THEN NULL;
			WHEN unique_violation THEN
				RAISE NOTICE 'habitat_plots: duplicate (sector_id, plot_number) rows — fix data before adding unique_plot_per_sector';
		END $$
	`)

	ensurePerformanceIndexes(db)
}

// ensureHabitatSubSectorColumns defends against habitat_sub_sectors having
// been created directly in the database (ahead of this model landing here)
// without the deleted_at column GORM's soft-delete support requires —
// every query GORM builds for a gorm.DeletedAt-bearing model appends
// "WHERE deleted_at IS NULL", so a missing column turns into a hard 500
// (SQLSTATE 42703) on literally every request, not just deletes. AutoMigrate
// above is supposed to add missing columns to existing tables on its own,
// but this runs unconditionally as a safety net, same pattern as the
// refresh_tokens/notification_preferences/videos checks above.
func ensureHabitatSubSectorColumns(db *gorm.DB) {
	if !db.Migrator().HasTable(&models.HabitatSubSector{}) {
		return
	}
	if !db.Migrator().HasColumn(&models.HabitatSubSector{}, "DeletedAt") {
		log.Println("⚠️ habitat_sub_sectors.deleted_at missing, adding it...")
		if err := db.Migrator().AddColumn(&models.HabitatSubSector{}, "DeletedAt"); err != nil {
			log.Printf("❌ GORM AddColumn deleted_at on habitat_sub_sectors failed: %v — trying raw SQL fallback", err)
			db.Exec(`ALTER TABLE habitat_sub_sectors ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`)
			db.Exec(`CREATE INDEX IF NOT EXISTS idx_habitat_sub_sectors_deleted_at ON habitat_sub_sectors(deleted_at)`)
		}
	}
}

// HabitatPostGISReady is true once habitat_plots.geom (a real PostGIS geometry
// column) and its GIST index are confirmed present. routes.GetHabitatSectorVectorTile
// checks this to pick the fast ST_AsMVT query path over the legacy per-row
// Go/orb JSON-decode fallback. Read from multiple goroutines (HTTP handlers);
// only ever written once during startup, so a plain bool is safe here.
var HabitatPostGISReady bool

// ensureHabitatPostGIS enables PostGIS and adds a real geometry column + GIST
// index on habitat_plots so vector tiles can be generated inside Postgres
// (ST_AsMVT) instead of decoded from JSONB and re-encoded in Go on every
// request — the standard Mapbox/Mapnik tile-serving pattern, and the only way
// this scales to 10K+ plot sectors / nationwide cadastre data.
//
// Additive and idempotent — safe to run on every boot. If the environment
// can't CREATE EXTENSION (some restricted managed Postgres hosts), this logs
// a warning and the tile handler keeps using the existing JSON fallback.
func ensureHabitatPostGIS(db *gorm.DB) {
	if !db.Migrator().HasTable(&models.HabitatPlot{}) {
		return
	}

	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS postgis`).Error; err != nil {
		log.Printf("⚠️ habitat: PostGIS extension unavailable, staying on JSON tile fallback: %v", err)
		return
	}

	var hasGeomColumn bool
	db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'habitat_plots' AND column_name = 'geom'
		)
	`).Scan(&hasGeomColumn)

	if !hasGeomColumn {
		if err := db.Exec(`ALTER TABLE habitat_plots ADD COLUMN geom geometry(Geometry, 4326)`).Error; err != nil {
			log.Printf("⚠️ habitat: failed to add geom column, staying on JSON tile fallback: %v", err)
			return
		}
		log.Println("✅ habitat_plots.geom column added")
	}

	// CONCURRENTLY cannot run inside a transaction block; plain db.Exec here
	// autocommits (gorm only wraps Create/Update/Delete helpers, not raw Exec),
	// so this is safe. IF NOT EXISTS makes subsequent boots an instant no-op.
	if err := db.Exec(`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_habitat_plots_geom ON habitat_plots USING GIST(geom)`).Error; err != nil {
		log.Printf("⚠️ habitat: failed to create GIST index on geom (will retry next boot): %v", err)
		return
	}

	HabitatPostGISReady = true
	log.Println("✅ habitat PostGIS ready — vector tiles now use ST_AsMVT")

	// Bounded per boot and idempotent (only touches geom IS NULL rows), so a
	// large backlog catches up over a few restarts without blocking startup.
	go backfillHabitatPlotGeom(db)
}

// BackfillHabitatPlotGeomNow re-runs the geom backfill on demand — called on
// a schedule from main so plots imported AFTER server startup become visible
// to the tile engine without a redeploy. (Observed in prod: a quartier with
// 7,798 plots served tiles for only the 464 rows whose geom was backfilled
// before its import — the tile query filters `geom IS NOT NULL`.)
func BackfillHabitatPlotGeomNow() {
	if !HabitatPostGISReady || DB == nil {
		return
	}
	backfillHabitatPlotGeom(DB)
}

// BackfillHabitatPlotGeomToCompletion loops the backfill until NO backfillable
// rows remain (or a batch makes no progress — all remaining rows malformed).
// Each backfillHabitatPlotGeom call handles up to 300k rows, so a large
// dataset needs several passes; the admin trigger uses this to finish now
// instead of chipping away 300k/hour via the ticker.
func BackfillHabitatPlotGeomToCompletion() {
	if !HabitatPostGISReady || DB == nil {
		return
	}
	for i := 0; i < 100; i++ {
		var before int64
		DB.Raw(`
			SELECT COUNT(*) FROM habitat_plots
			WHERE geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')
		`).Scan(&before)
		if before == 0 {
			return
		}
		backfillHabitatPlotGeom(DB)
		var after int64
		DB.Raw(`
			SELECT COUNT(*) FROM habitat_plots
			WHERE geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')
		`).Scan(&after)
		if after >= before {
			// No progress — remaining rows are all malformed; stop looping.
			return
		}
	}
}

// backfillHabitatPlotGeom populates geom from the existing geom_geojson JSONB
// column. Runs per-row inside a PL/pgSQL loop with exception handling so one
// malformed geometry doesn't abort the whole batch — those rows are simply
// skipped and the tile handler keeps serving them from the legacy fallback.
func backfillHabitatPlotGeom(db *gorm.DB) {
	var pending int64
	db.Raw(`
		SELECT COUNT(*) FROM habitat_plots
		WHERE geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')
	`).Scan(&pending)
	if pending == 0 {
		return
	}
	log.Printf("🔧 habitat: backfilling geom for %d plots from geom_geojson...", pending)

	if err := db.Exec(`
		DO $$
		DECLARE
			r RECORD;
		BEGIN
			FOR r IN
				SELECT id, geom_geojson FROM habitat_plots
				WHERE geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')
				LIMIT 300000
			LOOP
				BEGIN
					UPDATE habitat_plots
					SET geom = ST_SetSRID(
						ST_GeomFromGeoJSON(
							CASE
								WHEN r.geom_geojson->>'type' = 'Feature' THEN r.geom_geojson->'geometry'
								WHEN r.geom_geojson->>'type' = 'FeatureCollection' THEN r.geom_geojson->'features'->0->'geometry'
								ELSE r.geom_geojson
							END
						), 4326)
					WHERE id = r.id;
				EXCEPTION WHEN OTHERS THEN
					-- Malformed geometry for this row — skip it; tile handler falls
					-- back to geom_geojson decode for rows where geom stays NULL.
					CONTINUE;
				END;
			END LOOP;
		END $$;
	`).Error; err != nil {
		log.Printf("⚠️ habitat: geom backfill batch error: %v", err)
		return
	}
	log.Println("✅ habitat: geom backfill batch complete")

	// Centroids drive the TileJSON bounds + plot_count (the map's camera fit
	// and health metric). Rows imported with geometry but no centroid left
	// the quartier's reported extent/count based only on the subset that had
	// centroids. Derive any missing centroid from the freshly-populated geom
	// so bounds cover the whole quartier and plot_count is truthful.
	backfillHabitatPlotCentroids(db)
}

// backfillHabitatPlotCentroids fills centroid_lat/centroid_lng from geom for
// rows missing a centroid. Set-based (fast), idempotent, additive.
func backfillHabitatPlotCentroids(db *gorm.DB) {
	var pending int64
	db.Raw(`
		SELECT COUNT(*) FROM habitat_plots
		WHERE (centroid_lat IS NULL OR centroid_lng IS NULL) AND geom IS NOT NULL
	`).Scan(&pending)
	if pending == 0 {
		return
	}
	log.Printf("🔧 habitat: backfilling centroids for %d plots from geom...", pending)
	if err := db.Exec(`
		UPDATE habitat_plots
		SET centroid_lat = ST_Y(ST_Centroid(geom)),
		    centroid_lng = ST_X(ST_Centroid(geom))
		WHERE (centroid_lat IS NULL OR centroid_lng IS NULL) AND geom IS NOT NULL
	`).Error; err != nil {
		log.Printf("⚠️ habitat: centroid backfill error: %v", err)
		return
	}
	log.Println("✅ habitat: centroid backfill complete")
}

// ensurePerformanceIndexes adds indexes for hot paths that were stalling the DB pool.
func ensurePerformanceIndexes(db *gorm.DB) {
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_guide_comments_host_parent_sale
		ON guide_comments(host_id, parent_id, property_sale_id)
		WHERE deleted_at IS NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_guide_comments_host_status
		ON guide_comments(host_id, status)
		WHERE parent_id IS NULL AND deleted_at IS NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_notification_preferences_user_updated
		ON notification_preferences(user_id, updated_at DESC)
		WHERE deleted_at IS NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_notification_preferences_user_enabled
		ON notification_preferences(user_id, enabled)
		WHERE deleted_at IS NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_organizations_owner
		ON organizations(owner_id)
		WHERE deleted_at IS NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_landmarks_org_owner
		ON landmarks(organization_id, owner_id)`)
}

// ensureOrganizationInviteCodesBackfill fills legacy rows before GORM sets code NOT NULL.
func ensureOrganizationInviteCodesBackfill(db *gorm.DB) {
	if !db.Migrator().HasTable(&models.OrganizationInviteCode{}) {
		return
	}
	if !db.Migrator().HasColumn(&models.OrganizationInviteCode{}, "Code") {
		return
	}
	res := db.Exec(`
		UPDATE organization_invite_codes
		SET code = 'LEG-' || LPAD(id::text, 8, '0')
		WHERE code IS NULL OR TRIM(code) = ''
	`)
	if res.Error != nil {
		log.Printf("⚠️ organization_invite_codes backfill: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("✅ Backfilled %d organization_invite_codes with legacy codes", res.RowsAffected)
	}
}

// ensureBrokerIDPartialUniqueIndex prevents signup failures when many users have no broker ID yet.
// GORM uniqueIndex on broker_id treated '' as one value — only assigned IDs must be unique.
func ensureBrokerIDPartialUniqueIndex(db *gorm.DB) {
	db.Exec(`UPDATE users SET broker_id = NULL WHERE broker_id = ''`)
	db.Exec(`DROP INDEX IF EXISTS idx_users_broker_id`)
	db.Exec(`DROP INDEX IF EXISTS uni_users_broker_id`)
	db.Exec(`
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
END $$`)
	db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_broker_id
  ON users (broker_id)
  WHERE broker_id IS NOT NULL AND broker_id <> ''`)
}

func ensureSlideshowVideoSystem(db *gorm.DB) {
	db.Exec(`
CREATE TABLE IF NOT EXISTS music_tracks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  title VARCHAR(200) NOT NULL,
  category VARCHAR(64) NOT NULL DEFAULT 'default',
  file_url VARCHAR(1024) NOT NULL DEFAULT '',
  duration_sec DOUBLE PRECISION DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  notes TEXT
)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_music_tracks_category ON music_tracks (category) WHERE deleted_at IS NULL`)
	db.Exec(`
CREATE TABLE IF NOT EXISTS property_video_generation_jobs (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  user_id BIGINT NOT NULL,
  entity_type VARCHAR(16) NOT NULL,
  entity_id BIGINT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  progress INTEGER NOT NULL DEFAULT 0,
  error_message TEXT,
  image_urls JSONB,
  music_track_id BIGINT,
  output_video_url VARCHAR(1024) DEFAULT '',
  property_type VARCHAR(64) DEFAULT '',
  overlay_title VARCHAR(300) DEFAULT '',
  overlay_location VARCHAR(300) DEFAULT '',
  overlay_area VARCHAR(64) DEFAULT '',
  overlay_price VARCHAR(64) DEFAULT '',
  overlay_cta VARCHAR(120) DEFAULT ''
)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_pvg_jobs_entity ON property_video_generation_jobs (entity_type, entity_id) WHERE deleted_at IS NULL`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_pvg_jobs_status ON property_video_generation_jobs (status) WHERE deleted_at IS NULL`)
}

func InitializeDB() *gorm.DB {
	db := connectToDB()
	performMigrations(db)
	return db
}

// envInt reads a positive integer from the environment or returns default.
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
