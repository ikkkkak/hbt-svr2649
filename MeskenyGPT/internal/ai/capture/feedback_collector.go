package capture

import (
	"context"
	"fmt"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// FeedbackSignal represents explicit or implicit feedback on an AI turn.
type FeedbackSignal struct {
	InteractionID uint
	Type          string  // e.g. "thumbs_up", "thumbs_down", "property_click"
	Value         float64 // 1.0 positive, 0.0 negative, 0.5 neutral
}

// FeedbackCollector stores feedback signals for later training/eval.
type FeedbackCollector interface {
	Record(ctx context.Context, f FeedbackSignal) error
}

type dbFeedbackCollector struct{}

// NewDBFeedbackCollector creates a collector backed by Postgres.
func NewDBFeedbackCollector() FeedbackCollector {
	return &dbFeedbackCollector{}
}


func (d *dbFeedbackCollector) Record(ctx context.Context, f FeedbackSignal) error {
	if storage.DB == nil {
		fmt.Println("⚠️ MeskenyGPT: DB not initialised, skipping feedback record")
		return nil
	}
	rec := &models.AIFeedback{
		InteractionID: f.InteractionID,
		Signal:        f.Type,
		Value:         f.Value,
		CreatedAt:     time.Now(),
	}
	if err := storage.DB.WithContext(ctx).Create(rec).Error; err != nil {
		fmt.Printf("⚠️ MeskenyGPT: failed to record feedback: %v\n", err)
	}
	return nil
}

