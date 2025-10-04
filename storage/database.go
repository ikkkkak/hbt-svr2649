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
	)

	// Allow direct chat groups without an experience by making experience_id nullable
	db.Exec("ALTER TABLE experience_groups ALTER COLUMN experience_id DROP NOT NULL;")
}

func InitializeDB() *gorm.DB {
	db := connectToDB()
	performMigrations(db)
	return db
}
