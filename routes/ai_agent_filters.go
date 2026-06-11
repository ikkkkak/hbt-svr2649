package routes

import (
	"net/http"
	"strings"

	"apartments-clone-server/MeskenyGPT/ai"

	"github.com/kataras/iris/v12"
)

// UpdateAIAgentFilters persists picker filter state for a chat session (Spec v2 §7).
// POST /api/ai/agent/filters
func UpdateAIAgentFilters(ctx iris.Context) {
	if MeskenyGPTService == nil {
		ctx.StatusCode(http.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "MeskenyGPT not available"})
		return
	}

	var req struct {
		SessionID     string  `json:"session_id"`
		AnonSessionID string  `json:"anon_session_id"`
		City          *string `json:"city"`
		Zone          *string `json:"zone"`
		Quartier      *string `json:"quartier"`
		Type          *string `json:"type"`
		MinPrice      *int64  `json:"min_price"`
		MaxPrice      *int64  `json:"max_price"`
		Bedrooms      *int    `json:"bedrooms"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	sessionKey := strings.TrimSpace(req.SessionID)
	if sessionKey == "" {
		sessionKey = strings.TrimSpace(req.AnonSessionID)
	}
	if sessionKey == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "session_id or anon_session_id required"})
		return
	}

	patch := ai.SessionFilterPatch{
		City:     req.City,
		Zone:     req.Zone,
		Quartier: req.Quartier,
		Type:     req.Type,
		MinPrice: req.MinPrice,
		MaxPrice: req.MaxPrice,
		Bedrooms: req.Bedrooms,
	}
	fc, err := MeskenyGPTService.UpdateSessionFilters(ctx.Request().Context(), sessionKey, patch)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to save filters"})
		return
	}
	ctx.JSON(iris.Map{"success": true, "filters": fc})
}
