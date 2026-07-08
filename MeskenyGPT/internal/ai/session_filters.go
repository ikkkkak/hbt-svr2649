package ai

import (
	"context"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/session"
)

func (s *service) hydrateSessionFilters(ctx context.Context, sessionID string, msgCtx lang.MessageContext) lang.MessageContext {
	if s.rdb == nil || sessionID == "" {
		return msgCtx
	}
	fc, ok := session.LoadFilterContext(ctx, s.rdb, sessionID)
	if !ok {
		return msgCtx
	}
	msgCtx = session.MergeIntoContext(msgCtx, fc)
	msgCtx = lang.ClearStaleQuartier(msgCtx, msgCtx.RawText)
	msgCtx = lang.ClearStaleType(msgCtx, msgCtx.RawText)
	return msgCtx
}

func (s *service) persistSessionFilters(ctx context.Context, sessionID string, msgCtx lang.MessageContext) {
	if s.rdb == nil || sessionID == "" {
		return
	}
	_ = session.SaveFilterContext(ctx, s.rdb, sessionID, session.FromMessageContext(msgCtx))
}

func (s *service) UpdateSessionFilters(ctx context.Context, sessionID string, patch SessionFilterPatch) (SessionFilterContext, error) {
	if s.rdb == nil || sessionID == "" {
		return SessionFilterContext{}, nil
	}
	sp := session.FilterContext{}
	if patch.City != nil {
		sp.City = *patch.City
	}
	if patch.Zone != nil {
		sp.Zone = *patch.Zone
	}
	if patch.Quartier != nil {
		sp.Quartier = *patch.Quartier
	}
	if patch.Type != nil {
		sp.Type = *patch.Type
	}
	if patch.MinPrice != nil {
		sp.MinPrice = *patch.MinPrice
	}
	if patch.MaxPrice != nil {
		sp.MaxPrice = *patch.MaxPrice
	}
	if patch.Bedrooms != nil {
		sp.Bedrooms = *patch.Bedrooms
	}
	fc, err := session.UpdateFilterContext(ctx, s.rdb, sessionID, sp)
	return sessionFilterToPublic(fc), err
}

func sessionFilterToPublic(fc session.FilterContext) SessionFilterContext {
	return SessionFilterContext{
		City: fc.City, Zone: fc.Zone, Quartier: fc.Quartier, Type: fc.Type,
		MinPrice: fc.MinPrice, MaxPrice: fc.MaxPrice, Bedrooms: fc.Bedrooms,
		UpdatedAt: fc.UpdatedAt,
	}
}
