package ai

import "apartments-clone-server/MeskenyGPT/internal/ai/response"

// AgentEvent is one SSE payload (JSON); Type discriminates the shape.
type AgentEvent struct {
	Type string `json:"type"`

	RunID   string `json:"run_id,omitempty"`
	Persona string `json:"persona,omitempty"`
	Tier    string `json:"tier,omitempty"`

	Role string `json:"role,omitempty"` // PropertySearcher | PropertyAdvisor | MarketAnalyst
	Lang string `json:"lang,omitempty"` // ar | fr | en | zh
	RTL  bool   `json:"rtl,omitempty"`

	StepID string `json:"step_id,omitempty"`
	Label  string `json:"label,omitempty"`
	MS     int64  `json:"ms,omitempty"`
	Detail any    `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`

	Tool string `json:"tool,omitempty"`
	Args any    `json:"args,omitempty"`

	MatchesIntent bool     `json:"matches_intent,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	Assumptions   []string `json:"assumptions,omitempty"`
	Gaps          []string `json:"gaps,omitempty"`

	Message                 response.Message      `json:"message,omitempty"`
	PropertyRecommendations []any                 `json:"propertyRecommendations,omitempty"`
	QuickReplies            []response.QuickReply `json:"quick_replies,omitempty"`
	FollowUps               []string              `json:"follow_ups,omitempty"`
	SessionID               string                `json:"session_id,omitempty"`
	InteractionID           uint                  `json:"interaction_id,omitempty"`
	Blocked                 bool                  `json:"blocked,omitempty"`

	TotalMS int64 `json:"total_ms,omitempty"`

	Delta string `json:"delta,omitempty"`
	Steps []StepPlanItem `json:"steps,omitempty"`
}

// StepPlanItem is one planned reasoning step for the client timeline.
type StepPlanItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AgentRunInput extends a chat turn with agent UX metadata.
type AgentRunInput struct {
	ChatInput
	RunID   string
	Persona string // buyer | broker
	Tier    string // free | pro | broker
}

const (
	AgentEventRunStarted    = "run_started"
	AgentEventStreamStart   = "stream_start"
	AgentEventStepPlan      = "step_plan"
	AgentEventStepStart     = "step_start"
	AgentEventStepDone      = "step_done"
	AgentEventStepError     = "step_error"
	AgentEventToolCall      = "tool_call"
	AgentEventVerification  = "verification"
	AgentEventFinal         = "final"
	AgentEventFollowUps     = "follow_ups"
	AgentEventTextDelta     = "text_delta"
	AgentEventRunComplete   = "run_complete"
	AgentEventBlocked       = "blocked"
)

type agentRunEmitter func(AgentEvent)
