package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"time"
)

// Default weights for recommendation scoring (aligned with spec)
const (
	WeightVideoView      = 1.0
	WeightPropertyView   = 1.0
	WeightLike           = 2.5
	WeightSave           = 2.5
	WeightShare          = 2.0
	WeightMessageOwner   = 3.0
	WeightBookingAttempt = 4.0
)

// InteractionInput is the input for recording an interaction
type InteractionInput struct {
	EntityType       string   `json:"entityType"`       // video | property_sale_video | property | property_sale
	EntityID         uint     `json:"entityId"`
	PropertyID       *uint    `json:"propertyId"`       // for video (rent) and property
	PropertySaleID   *uint    `json:"propertySaleId"`   // for property_sale_video and property_sale
	EventType        string   `json:"eventType"`        // video_view | property_view | like | save | share | message_owner | booking_attempt
	WatchDurationSec *float64 `json:"watchDurationSec"` // for video_view
	UserID           *uint    `json:"userId"`
	DeviceID         *string  `json:"deviceId"`
	// IsMeaningfulView can be set by caller; otherwise we compute for video_view and property_view
	IsMeaningfulView *bool `json:"isMeaningfulView"`
}

// InteractionService records append-only interactions for ML and recommendations
type InteractionService struct{}

// NewInteractionService returns a new InteractionService
func NewInteractionService() *InteractionService {
	return &InteractionService{}
}

// Record persists an interaction. At least one of UserID or DeviceID must be set.
// Meaningful view: video ≥3s or ≥30% of duration; property view: consider true if event is property_view (card/gallery).
func (s *InteractionService) Record(in InteractionInput) error {
	if (in.UserID == nil || *in.UserID == 0) && (in.DeviceID == nil || *in.DeviceID == "") {
		return nil // skip anonymous without device
	}

	weight := s.weightForEvent(in.EventType)
	meaningful := false
	if in.IsMeaningfulView != nil {
		meaningful = *in.IsMeaningfulView
	} else {
		meaningful = s.computeMeaningful(in)
	}

	rec := models.Interaction{
		EntityType:       in.EntityType,
		EntityID:         in.EntityID,
		PropertyID:       in.PropertyID,
		PropertySaleID:   in.PropertySaleID,
		EventType:        in.EventType,
		WatchDurationSec: in.WatchDurationSec,
		Weight:           weight,
		IsMeaningfulView: meaningful,
		UserID:           in.UserID,
		DeviceID:         in.DeviceID,
	}

	if err := storage.DB.Create(&rec).Error; err != nil {
		log.Printf("interaction.Record error: %v", err)
		return err
	}
	return nil
}

func (s *InteractionService) weightForEvent(e string) float64 {
	switch e {
	case models.EventVideoView:
		return WeightVideoView
	case models.EventPropertyView:
		return WeightPropertyView
	case models.EventLike:
		return WeightLike
	case models.EventSave:
		return WeightSave
	case models.EventShare:
		return WeightShare
	case models.EventMessageOwner:
		return WeightMessageOwner
	case models.EventBookingAttempt:
		return WeightBookingAttempt
	default:
		return 1.0
	}
}

// computeMeaningful: video ≥3s or ≥30% of duration; property_view when it's a card/gallery open we treat as meaningful.
func (s *InteractionService) computeMeaningful(in InteractionInput) bool {
	switch in.EventType {
	case models.EventVideoView:
		if in.WatchDurationSec != nil {
			if *in.WatchDurationSec >= 3 {
				return true
			}
			// ≥30% requires duration from entity; we don't have it here. Caller can pass IsMeaningfulView.
			// For video we fetch duration and check 30% when possible.
			if in.EntityType == models.EntityVideo {
				var v models.Video
				if err := storage.DB.Select("duration_sec").First(&v, in.EntityID).Error; err == nil && v.DurationSec > 0 {
					return (*in.WatchDurationSec / v.DurationSec) >= 0.3
				}
			}
			if in.EntityType == models.EntityPropertySaleVideo {
				var v models.PropertySaleVideo
				if err := storage.DB.Select("duration_sec").First(&v, in.EntityID).Error; err == nil && v.DurationSec > 0 {
					return (*in.WatchDurationSec / v.DurationSec) >= 0.3
				}
			}
		}
		return false
	case models.EventPropertyView:
		// Card opened or gallery viewed → meaningful
		return true
	default:
		return false
	}
}

// RecordVideoView is a convenience that builds InteractionInput for video_view (rent Video).
func (s *InteractionService) RecordVideoView(videoID uint, propertyID *uint, watchDurationSec *float64, userID *uint, deviceID *string) {
	_ = s.Record(InteractionInput{
		EntityType:       models.EntityVideo,
		EntityID:         videoID,
		PropertyID:       propertyID,
		EventType:        models.EventVideoView,
		WatchDurationSec: watchDurationSec,
		UserID:           userID,
		DeviceID:         deviceID,
	})
}

// RecordPropertySaleVideoView for property_sale_video with optional watch_duration.
func (s *InteractionService) RecordPropertySaleVideoView(videoID, propertySaleID uint, watchDurationSec *float64, userID *uint, deviceID *string) {
	pid := propertySaleID
	_ = s.Record(InteractionInput{
		EntityType:       models.EntityPropertySaleVideo,
		EntityID:         videoID,
		PropertySaleID:   &pid,
		EventType:        models.EventVideoView,
		WatchDurationSec: watchDurationSec,
		UserID:           userID,
		DeviceID:         deviceID,
	})
}

// RecordPropertyView for rent Property.
func (s *InteractionService) RecordPropertyView(propertyID uint, userID *uint, deviceID *string) {
	pid := propertyID
	_ = s.Record(InteractionInput{
		EntityType: models.EntityProperty,
		EntityID:   propertyID,
		PropertyID: &pid,
		EventType:  models.EventPropertyView,
		UserID:     userID,
		DeviceID:   deviceID,
	})
}

// RecordPropertySaleView for PropertySale.
func (s *InteractionService) RecordPropertySaleView(propertySaleID uint, userID *uint, deviceID *string) {
	psid := propertySaleID
	_ = s.Record(InteractionInput{
		EntityType:     models.EntityPropertySale,
		EntityID:       propertySaleID,
		PropertySaleID: &psid,
		EventType:      models.EventPropertyView,
		UserID:         userID,
		DeviceID:       deviceID,
	})
}

var (
	interactionService     *InteractionService
	interactionServiceInit time.Time
)

// InteractionServiceInstance returns a shared InteractionService
func InteractionServiceInstance() *InteractionService {
	if interactionService == nil {
		interactionService = NewInteractionService()
		interactionServiceInit = time.Now()
	}
	return interactionService
}
