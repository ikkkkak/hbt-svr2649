package capture

import (
	"context"
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// Interaction holds the core telemetry for a single AI turn.
type Interaction struct {
	SessionID   string
	UserID      uint
	TurnIndex   int
	Lang        lang.Lang
	Intent      lang.Intent
	UserMessage string
	SystemPrompt string
	AIResponse  string
	ModelUsed   string
	TokensUsed  int
	LatencyMS   int64
	Cities      []string
	Zones       []string
	PropertyType string
	Budget      string
	Purpose     string
	CreatedAt   time.Time
}

// Logger is a pluggable sink for interactions (DB, file, etc.).
// Log returns the created interaction ID (0 if not persisted).
type Logger interface {
	Log(ctx context.Context, i Interaction) (uint, error)
}

// stdoutLogger prints to stdout and returns 0 (no persisted ID).
type stdoutLogger struct{}

func NewStdoutLogger() Logger {
	return &stdoutLogger{}
}

func (l *stdoutLogger) Log(ctx context.Context, i Interaction) (uint, error) {
	fmt.Printf("📝 MeskenyGPT interaction: intent=%s type=%q user=%q → %d chars\n",
		intentToString(i.Intent), i.PropertyType, truncate(i.UserMessage, 80), len(i.AIResponse))
	return 0, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// dbLogger writes interactions into the ai_interactions table.
type dbLogger struct{}

// NewDBLogger returns a logger backed by Postgres via GORM.
func NewDBLogger() Logger {
	return &dbLogger{}
}

func (l *dbLogger) Log(ctx context.Context, i Interaction) (uint, error) {
	if storage.DB == nil {
		fmt.Println("⚠️ MeskenyGPT: DB not initialised, skipping interaction log")
		return 0, nil
	}
	rec := &models.AIInteraction{
		SessionID:    i.SessionID,
		UserID:       nilIfZero(i.UserID),
		TurnIndex:    i.TurnIndex,
		Lang:         langToString(i.Lang),
		Intent:       intentToString(i.Intent),
		UserMessage:  i.UserMessage,
		SystemPrompt: i.SystemPrompt,
		AIResponse:   i.AIResponse,
		ModelUsed:    i.ModelUsed,
		TokensUsed:   i.TokensUsed,
		LatencyMS:    i.LatencyMS,
		Cities:       strings.Join(i.Cities, ","),
		Zones:        strings.Join(i.Zones, ","),
		PropertyType: i.PropertyType,
		Budget:       truncateTo(i.Budget, 30),
		Purpose:      i.Purpose,
		CreatedAt:    i.CreatedAt,
	}
	if err := storage.DB.WithContext(ctx).Create(rec).Error; err != nil {
		fmt.Printf("⚠️ MeskenyGPT: failed to log interaction: %v\n", err)
		return 0, err
	}
	return rec.ID, nil
}

func langToString(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "ar"
	case lang.LangEN:
		return "en"
	default:
		return "fr"
	}
}

func intentToString(it lang.Intent) string {
	return IntentLabel(it)
}

func truncateTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func nilIfZero(u uint) *uint {
	if u == 0 {
		return nil
	}
	return &u
}

