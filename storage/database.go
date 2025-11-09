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
		// Property Selling System Models
		&models.Organization{},
		&models.Agent{},
		&models.PropertySale{},
		&models.PropertyTour{},
		&models.PropertyInquiry{},
		&models.PropertyOffer{},
		&models.Landmark{},
		&models.NotificationPreference{},
		&models.MarketingDevice{},
		// Video Reporting System Models
		&models.VideoReport{},
		&models.UserFlag{},
		&models.HiddenVideo{},
		// Group Management Models
		&models.Group{},
		&models.GroupMember{},
		&models.GroupInvite{},
		&models.GroupMessage{},
		&models.GroupMessageRead{},
		&models.GroupBan{},
		&models.GroupUserBlock{},
		&models.GroupQuit{},
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
}

func InitializeDB() *gorm.DB {
	db := connectToDB()
	performMigrations(db)
	return db
}
