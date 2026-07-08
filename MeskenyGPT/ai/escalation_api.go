package ai

import (
	"context"

	"apartments-clone-server/MeskenyGPT/internal/ai/escalation"
	"apartments-clone-server/models"

	"gorm.io/gorm"
)

var escalationEngine *escalation.Engine

func escalationSvc(db *gorm.DB) *escalation.Engine {
	if escalationEngine == nil {
		escalationEngine = escalation.NewEngine(db)
	}
	return escalationEngine
}

// RequestEscalation creates an escalation row for a chat session.
func RequestEscalation(ctx context.Context, db *gorm.DB, sessionID string, userID *uint, reason string) (*models.AIEscalation, error) {
	if reason == "" {
		reason = "User requested a specialist"
	}
	trig := &escalation.Trigger{
		Type:    "explicit",
		Score:   1,
		Reason:  reason,
		Urgency: "high",
		Context: reason,
	}
	return escalationSvc(db).Execute(ctx, trig, sessionID, userID)
}

// GetEscalation loads an escalation by ID.
func GetEscalation(ctx context.Context, db *gorm.DB, id uint) (*models.AIEscalation, error) {
	return escalationSvc(db).GetByID(ctx, id)
}

// ResolveEscalation marks an escalation resolved.
func ResolveEscalation(ctx context.Context, db *gorm.DB, id uint, notes string) error {
	return escalationSvc(db).Resolve(ctx, id, notes)
}

// NotifyEscalationSideEffects sends user + admin notifications after escalation is created.
func NotifyEscalationSideEffects(ctx context.Context, row *models.AIEscalation, userID *uint, sessionID string) {
	if row == nil {
		return
	}
	// Implemented in services to avoid import cycles from internal packages.
	notifyEscalationFn(ctx, row, userID, sessionID)
}

var notifyEscalationFn = func(context.Context, *models.AIEscalation, *uint, string) {}

// RegisterEscalationNotifier wires notification delivery from main (services package).
func RegisterEscalationNotifier(fn func(context.Context, *models.AIEscalation, *uint, string)) {
	if fn != nil {
		notifyEscalationFn = fn
	}
}
