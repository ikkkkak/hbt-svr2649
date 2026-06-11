package listing_ai

import (
	"log"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// RecordUsage persists a listing AI usage event (non-blocking).
func RecordUsage(userID uint, kind Kind, event, jobID string) {
	if userID == 0 || kind == "" || event == "" {
		return
	}
	go func() {
		if storage.DB == nil {
			return
		}
		row := models.ListingAIUsageEvent{
			UserID: userID,
			Kind:   string(kind),
			Event:  event,
			JobID:  jobID,
		}
		if err := storage.DB.Create(&row).Error; err != nil {
			log.Printf("listing_ai usage record (%s/%s): %v", kind, event, err)
		}
	}()
}
