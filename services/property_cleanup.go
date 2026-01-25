package services

import (
	"apartments-clone-server/storage"
	"log"
	"time"

	"gorm.io/gorm"
)

// PropertyCleanupService handles scheduled cleanup of deleted properties
type PropertyCleanupService struct{}

// NewPropertyCleanupService creates a new cleanup service
func NewPropertyCleanupService() *PropertyCleanupService {
	return &PropertyCleanupService{}
}

// CleanupDeletedProperties permanently deletes properties that were soft-deleted 15+ days ago
func (pcs *PropertyCleanupService) CleanupDeletedProperties() error {
	// Calculate cutoff date (15 days ago)
	cutoffDate := time.Now().AddDate(0, 0, -15)

	// Find properties that were deleted 15+ days ago
	var deletedCount int64
	result := storage.DB.Unscoped().
		Where("deleted_at IS NOT NULL").
		Where("deleted_at < ?", cutoffDate).
		Model(&struct {
			ID        uint
			DeletedAt gorm.DeletedAt
		}{}).
		Count(&deletedCount)

	if result.Error != nil {
		log.Printf("❌ Error counting deleted properties: %v", result.Error)
		return result.Error
	}

	if deletedCount == 0 {
		log.Printf("✅ No properties to permanently delete (cutoff: %s)", cutoffDate.Format("2006-01-02"))
		return nil
	}

	// Permanently delete properties (using Unscoped to bypass soft delete)
	// Note: This uses raw SQL to permanently delete from property_sales table
	permanentDeleteSQL := `
		DELETE FROM property_sales 
		WHERE deleted_at IS NOT NULL 
		AND deleted_at < ?
	`

	result = storage.DB.Exec(permanentDeleteSQL, cutoffDate)
	if result.Error != nil {
		log.Printf("❌ Error permanently deleting properties: %v", result.Error)
		return result.Error
	}

	log.Printf("✅ Permanently deleted %d properties (deleted before %s)", result.RowsAffected, cutoffDate.Format("2006-01-02"))
	return nil
}

// StartCleanupScheduler starts a background goroutine that runs cleanup daily
func (pcs *PropertyCleanupService) StartCleanupScheduler() {
	go func() {
		// Run cleanup immediately on start
		if err := pcs.CleanupDeletedProperties(); err != nil {
			log.Printf("❌ Initial cleanup failed: %v", err)
		}

		// Then run daily at 2 AM
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			// Only run at 2 AM
			if now.Hour() == 2 && now.Minute() < 5 {
				if err := pcs.CleanupDeletedProperties(); err != nil {
					log.Printf("❌ Scheduled cleanup failed: %v", err)
				}
			}
		}
	}()
	log.Printf("✅ Property cleanup scheduler started (runs daily at 2 AM)")
}
