package services

import (
	"context"

	"apartments-clone-server/models"
)

// NotifyEscalationCreated sends Model X46 escalation notifications to user and admins.
func NotifyEscalationCreated(ctx context.Context, row *models.AIEscalation, userID *uint, sessionID string) {
	if row == nil {
		return
	}
	if userID != nil && *userID > 0 {
		lang := ResolveUserNotificationLang(*userID)
		title, body := ModelX46EscalationUserCopy(lang, row.Urgency)
		SendModelX46Notification(ctx, *userID, "escalation_update", title, body, map[string]any{
			"escalation_id": row.ID,
			"session_id":    sessionID,
			"screen":        "AIChat",
		}, row.TriggerScore, row.Urgency)
	}
	agentTitle, agentBody := ModelX46EscalationAgentCopy("en", row.Urgency, row.TriggerType, row.Reason)
	NotifyAgentsEscalation(ctx, row.ID, sessionID, userID, agentTitle, agentBody, row.Urgency)
}
