package storage

import (
	"apartments-clone-server/models"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

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

	db, dbError := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if dbError != nil {
		log.Panic("error connection to db: " + dbError.Error())
	}

	DB = db
	return db
}

func performMigrations(db *gorm.DB) {
	db.AutoMigrate(
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
		&models.PropertyTour{},
		&models.PropertyInquiry{},
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
		&models.NotificationPreference{},
		&models.MarketingDevice{},
		&models.CrashLog{},
		// Video Reporting System Models
		&models.VideoReport{},
		&models.UserFlag{},
		&models.HiddenVideo{},
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
	)

	// Allow direct chat groups without an experience by making experience_id nullable
	db.Exec("ALTER TABLE experience_groups ALTER COLUMN experience_id DROP NOT NULL;")

	// Allow promotional videos without a property by making property_id nullable
	db.Exec("ALTER TABLE videos ALTER COLUMN property_id DROP NOT NULL;")

	// Make organization_id nullable in property_sales to allow individual owners
	db.Exec("ALTER TABLE property_sales ALTER COLUMN organization_id DROP NOT NULL;")

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

	// Add view_count column to property_sales table for tracking property views
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
		END $$;
	`)
}

func InitializeDB() *gorm.DB {
	db := connectToDB()
	performMigrations(db)
	return db
}
