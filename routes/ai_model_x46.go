package routes

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/ai"
	"apartments-clone-server/MeskenyGPT/semantic"
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// SemanticSearch handles POST /api/ai/search/semantic
func SemanticSearch(ctx iris.Context) {
	if SemanticEngine == nil || !SemanticEngine.Enabled() {
		ctx.StatusCode(iris.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "semantic_search_disabled"})
		return
	}
	var req struct {
		Query        string  `json:"query"`
		City         string  `json:"city"`
		Zone         string  `json:"zone"`
		PropertyType string  `json:"property_type"`
		Purpose      string  `json:"purpose"`
		MinPrice     float64 `json:"min_price"`
		MaxPrice     float64 `json:"max_price"`
		Bedrooms     int     `json:"bedrooms"`
		Limit        int     `json:"limit"`
	}
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Query) == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "query_required"})
		return
	}
	f := semanticSearchFilters(req.City, req.Zone, req.PropertyType, req.Purpose, req.MinPrice, req.MaxPrice, req.Bedrooms, req.Query)
	limit := req.Limit
	if limit <= 0 {
		limit = 12
	}
	props, scores, err := SemanticEngine.Search(context.Background(), req.Query, f, limit)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"success": true, "results": semanticResults(props, scores), "count": len(props)})
}

// SimilarProperties handles POST /api/ai/search/similar
func SimilarProperties(ctx iris.Context) {
	if SemanticEngine == nil || !SemanticEngine.Enabled() {
		ctx.StatusCode(iris.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "semantic_search_disabled"})
		return
	}
	var req struct {
		Source     string `json:"source"`
		PropertyID uint   `json:"property_id"`
		Limit      int    `json:"limit"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.PropertyID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "property_id_required"})
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = semantic.SourceSale
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	props, err := SemanticEngine.Similar(context.Background(), source, req.PropertyID, limit)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	fromProps := make([]iris.Map, 0, len(props))
	for _, p := range props {
		fromProps = append(fromProps, iris.Map{
			"id": p.ID, "title": p.Title, "price": p.Price, "currency": p.Currency,
			"city": p.City, "bedrooms": p.Bedrooms, "image": p.Image, "type": p.Type, "source": p.Source,
		})
	}
	ctx.JSON(iris.Map{"success": true, "results": fromProps, "count": len(fromProps)})
}

// SearchSuggestions handles GET /api/ai/search/suggestions?q=
func SearchSuggestions(ctx iris.Context) {
	q := strings.TrimSpace(ctx.URLParam("q"))
	suggestions := []string{}
	if q != "" {
		lower := strings.ToLower(q)
		for _, c := range []string{
			"quiet family home near good schools",
			"high-yield investment near the metro",
			"3 bedroom apartment in Tevragh Zeina",
			"villa for sale under 30 million MRU",
			"land plot with cadastre in Nouakchott",
			"furnished studio for rent in Ksar",
		} {
			if strings.Contains(strings.ToLower(c), lower) {
				suggestions = append(suggestions, c)
			}
		}
	}
	ctx.JSON(iris.Map{"success": true, "suggestions": suggestions})
}

// AdminReindexSemantic rebuilds the Qdrant index (admin).
func AdminReindexSemantic(ctx iris.Context) {
	if SemanticEngine == nil || !SemanticEngine.Enabled() {
		ctx.StatusCode(iris.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "semantic_search_disabled"})
		return
	}
	count, err := SemanticEngine.ReindexAll(context.Background())
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"success": true, "indexed": count})
}

// AdminIndexProperty indexes one listing (admin).
func AdminIndexProperty(ctx iris.Context) {
	if SemanticEngine == nil || !SemanticEngine.Enabled() {
		ctx.StatusCode(iris.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "semantic_search_disabled"})
		return
	}
	var req struct {
		Source string `json:"source"`
		ID     uint   `json:"id"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.ID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_payload"})
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = semantic.SourceSale
	}
	if err := SemanticEngine.IndexListing(context.Background(), source, req.ID); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"success": true, "source": source, "id": req.ID})
}

// AdminSemanticStatus returns whether semantic search is enabled.
func AdminSemanticStatus(ctx iris.Context) {
	enabled := SemanticEngine != nil && SemanticEngine.Enabled()
	ctx.JSON(iris.Map{"enabled": enabled})
}

// RequestEscalation handles POST /api/ai/escalate
func RequestEscalation(ctx iris.Context) {
	userID := ctx.Values().GetUintDefault("userID", 0)
	var req struct {
		SessionID     json.RawMessage `json:"session_id"`
		AnonSessionID string          `json:"anon_session_id"`
		Reason        string          `json:"reason"`
		GuestName     string          `json:"guest_name"`
		GuestEmail    string          `json:"guest_email"`
		GuestPhone    string          `json:"guest_phone"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_request"})
		return
	}
	sessionID := parseFlexibleSessionID(req.SessionID, req.AnonSessionID)
	if sessionID == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "session_id_required"})
		return
	}
	var uid *uint
	if userID > 0 {
		uid = &userID
	}
	row, err := ai.RequestEscalation(context.Background(), storage.DB, sessionID, uid, req.Reason)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}

	var user models.User
	if userID > 0 {
		storage.DB.First(&user, userID)
	}
	guestName := strings.TrimSpace(req.GuestName)
	guestEmail := strings.TrimSpace(req.GuestEmail)
	guestPhone := strings.TrimSpace(req.GuestPhone)
	if userID > 0 {
		if guestName == "" {
			guestName = strings.TrimSpace(user.FirstName + " " + user.LastName)
		}
		if guestEmail == "" {
			guestEmail = strings.TrimSpace(user.Email)
		}
		if guestPhone == "" && user.PhoneNumber != nil {
			guestPhone = strings.TrimSpace(*user.PhoneNumber)
		}
	} else if guestEmail == "" && guestPhone == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "guest_contact_required"})
		return
	}
	contextSummary := buildEscalationContextSummary(sessionID, req.Reason)
	updates := map[string]any{
		"guest_name":      guestName,
		"guest_email":     guestEmail,
		"guest_phone":     guestPhone,
		"context_summary": contextSummary,
	}
	storage.DB.Model(row).Updates(updates)
	row.GuestName = guestName
	row.GuestEmail = guestEmail
	row.GuestPhone = guestPhone
	row.ContextSummary = contextSummary

	services.NotifyEscalationCreated(context.Background(), row, uid, sessionID)
	var userPtr *models.User
	if user.ID > 0 {
		userPtr = &user
	}
	services.NotifyAdminNewEscalation(row, userPtr)

	ctx.JSON(iris.Map{"success": true, "escalation": row})
}

func parseFlexibleSessionID(raw json.RawMessage, anonFallback string) string {
	if len(raw) > 0 {
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			if s := strings.TrimSpace(asString); s != "" {
				return s
			}
		}
		var asNumber float64
		if err := json.Unmarshal(raw, &asNumber); err == nil && asNumber > 0 {
			return strconv.FormatInt(int64(asNumber), 10)
		}
	}
	if s := strings.TrimSpace(anonFallback); s != "" {
		return s
	}
	return ""
}

func buildEscalationContextSummary(sessionID, reason string) string {
	var b strings.Builder
	if reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n\n")
	}
	var turns []models.AIInteraction
	storage.DB.Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Limit(8).
		Find(&turns)
	if len(turns) == 0 {
		b.WriteString("No prior chat turns logged for this session.")
		return b.String()
	}
	b.WriteString("Recent chat:\n")
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		userMsg := strings.TrimSpace(t.UserMessage)
		aiMsg := strings.TrimSpace(t.AIResponse)
		if len(userMsg) > 220 {
			userMsg = userMsg[:220] + "..."
		}
		if len(aiMsg) > 220 {
			aiMsg = aiMsg[:220] + "..."
		}
		if userMsg != "" {
			b.WriteString("User: ")
			b.WriteString(userMsg)
			b.WriteString("\n")
		}
		if aiMsg != "" {
			b.WriteString("AI: ")
			b.WriteString(aiMsg)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// EscalationStatus handles GET /api/ai/escalate/status/:id
func EscalationStatus(ctx iris.Context) {
	id, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 64)
	if id == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_id"})
		return
	}
	row, err := ai.GetEscalation(context.Background(), storage.DB, uint(id))
	if err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "not_found"})
		return
	}
	ctx.JSON(iris.Map{"success": true, "escalation": row})
}

// ResolveEscalation handles POST /api/ai/escalate/resolve
func ResolveEscalation(ctx iris.Context) {
	var req struct {
		EscalationID uint   `json:"escalation_id"`
		Notes        string `json:"notes"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.EscalationID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "escalation_id_required"})
		return
	}
	if err := ai.ResolveEscalation(context.Background(), storage.DB, req.EscalationID, req.Notes); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"success": true})
}

// GetAINotifications handles GET /api/ai/notifications
func GetAINotifications(ctx iris.Context) {
	userID := ctx.Values().GetUintDefault("userID", 0)
	if userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}
	limit := ctx.URLParamIntDefault("limit", 20)
	var rows []models.AINotification
	storage.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(limit).Find(&rows)
	ctx.JSON(iris.Map{"success": true, "notifications": rows})
}

// MarkAINotificationRead handles POST /api/ai/notifications/:id/read
func MarkAINotificationRead(ctx iris.Context) {
	userID := ctx.Values().GetUintDefault("userID", 0)
	id, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 64)
	if userID == 0 || id == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_request"})
		return
	}
	now := time.Now()
	storage.DB.Model(&models.AINotification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"status": "read", "read_at": &now})
	ctx.JSON(iris.Map{"success": true})
}

func semanticSearchFilters(city, zone, propertyType, purpose string, minPrice, maxPrice float64, bedrooms int, query string) semantic.Filters {
	return semantic.Filters{
		City: city, Zone: zone, Type: propertyType, Purpose: purpose,
		BudgetMin: minPrice, BudgetMax: maxPrice, Bedrooms: bedrooms, Query: query,
	}
}

func semanticResults(props []semantic.Property, scores []float32) []iris.Map {
	out := make([]iris.Map, 0, len(props))
	for i, p := range props {
		score := float32(0)
		if i < len(scores) {
			score = scores[i]
		}
		out = append(out, iris.Map{
			"id": p.ID, "title": p.Title, "price": p.Price, "currency": p.Currency,
			"city": p.City, "bedrooms": p.Bedrooms, "image": p.Image,
			"type": p.Type, "source": p.Source, "matchScore": score,
		})
	}
	return out
}
