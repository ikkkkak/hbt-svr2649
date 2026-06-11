package ai

import (
	"context"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/capture"
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

func (s *service) recordTurn(
	ctx context.Context,
	path capture.TurnPath,
	sessionID string,
	userID uint,
	turnIndex int,
	msgCtx lang.MessageContext,
	userMessage string,
	aiResponse string,
	resultCount int,
	latencyMS int64,
) uint {
	parsed := capture.ParsedFromMessageContext(msgCtx)
	capture.LogTurnToStdout(capture.TurnLogInput{
		Path:        path,
		SessionID:   sessionID,
		UserID:      userID,
		UserMessage: userMessage,
		AIResponse:  aiResponse,
		Parsed:      parsed,
		ResultCount: resultCount,
		LatencyMS:   latencyMS,
		ModelUsed:   s.cfg.Model,
	})
	id, _ := s.logger.Log(ctx, capture.Interaction{
		SessionID:    sessionID,
		UserID:       userID,
		TurnIndex:    turnIndex,
		Lang:         msgCtx.Lang,
		Intent:       msgCtx.Intent,
		UserMessage:  userMessage,
		AIResponse:   aiResponse,
		ModelUsed:    s.cfg.Model,
		LatencyMS:    latencyMS,
		Cities:       []string{msgCtx.City},
		Zones:        []string{msgCtx.Zone},
		PropertyType: msgCtx.Type,
		Budget:       parsed.Budget,
		Purpose:      parsed.Purpose,
		CreatedAt:    time.Now(),
	})
	return id
}
