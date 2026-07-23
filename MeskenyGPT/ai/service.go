package ai

import (
	"context"

	internal "apartments-clone-server/MeskenyGPT/internal/ai"
	"apartments-clone-server/MeskenyGPT/internal/ai/capture"
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

// MessageIsPropertySearch reports whether a message is a property/land SEARCH
// (rent/buy/land/etc.). The server uses this to suppress scraped-knowledge
// references on search results — citations belong on info/help/procedure
// answers, not on property listing cards.
func MessageIsPropertySearch(msg string) bool {
	return lang.IsPropertySearchIntent(lang.AnalyzeMessage(msg).Intent)
}

// Re-export core MeskenyGPT AI types so that the main server can depend on
// this public package instead of importing internal/ directly.

type (
	Config                = internal.Config
	ChatInput             = internal.ChatInput
	ChatOutput            = internal.ChatOutput
	ChatMessage           = internal.ChatMessage
	AgentRunInput         = internal.AgentRunInput
	AgentEvent            = internal.AgentEvent
	Service               = internal.Service
	SessionFilterContext  = internal.SessionFilterContext
	SessionFilterPatch    = internal.SessionFilterPatch
	EscalationInfo        = internal.EscalationInfo
)

const (
	AgentEventRunStarted   = internal.AgentEventRunStarted
	AgentEventStreamStart  = internal.AgentEventStreamStart
	AgentEventStepStart      = internal.AgentEventStepStart
	AgentEventStepDone       = internal.AgentEventStepDone
	AgentEventStepError      = internal.AgentEventStepError
	AgentEventToolCall       = internal.AgentEventToolCall
	AgentEventVerification   = internal.AgentEventVerification
	AgentEventFinal          = internal.AgentEventFinal
	AgentEventFollowUps      = internal.AgentEventFollowUps
	AgentEventRunComplete    = internal.AgentEventRunComplete
	AgentEventBlocked        = internal.AgentEventBlocked
	AgentEventSources        = internal.AgentEventSources
)

var feedbackCollector = capture.NewDBFeedbackCollector()

func DefaultConfigFromEnv() Config {
	return internal.DefaultConfigFromEnv()
}

func NewService(cfg Config, db any, cache any) Service {
	return internal.NewService(cfg, db, cache)
}

// AllowAgentRun enforces per-tier agent rate limits (exported for HTTP routes).
func AllowAgentRun(key, tier string) bool {
	return internal.AllowAgentRun(key, tier)
}

// RecordAIFeedback records thumbs up/down or property click for an interaction.
func RecordAIFeedback(ctx context.Context, interactionID uint, signal string, value float64) error {
	return feedbackCollector.Record(ctx, capture.FeedbackSignal{
		InteractionID: interactionID,
		Type:          signal,
		Value:         value,
	})
}

