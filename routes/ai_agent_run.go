package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/ai"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/google/uuid"
	"github.com/kataras/iris/v12"
)

// SendAIAgentRun streams MeskenyGPT agent steps via Server-Sent Events.
// POST /api/ai/agent/run
func SendAIAgentRun(ctx iris.Context) {
	if MeskenyGPTService == nil {
		ctx.StatusCode(http.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "MeskenyGPT not available"})
		return
	}

	type sharedPropertyInput struct {
		ID           uint    `json:"id"`
		Title        string  `json:"title"`
		ListingPrice float64 `json:"listing_price"`
		Address      string  `json:"address"`
		City         string  `json:"city"`
		Image        string  `json:"image"`
		Type         string  `json:"type"`
	}

	var req struct {
		Message            string               `json:"message"`
		SessionID          *uint                `json:"session_id"`
		AnonSessionID      string               `json:"anon_session_id"`
		DeepThink          bool                 `json:"deep_think"`
		Persona            string               `json:"persona"`
		Tier               string               `json:"tier"`
		SharedProperty     *sharedPropertyInput `json:"shared_property"`
		UserPromptTemplate string               `json:"user_prompt_template"`
		History            []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"history"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Message is required"})
		return
	}

	effectiveMessage := req.Message
	if req.SharedProperty != nil && req.SharedProperty.ID > 0 {
		p := req.SharedProperty
		lines := []string{"Property context (shared from app card):", "- internal_ref: hidden"}
		if p.Title != "" {
			lines = append(lines, fmt.Sprintf("- title: %s", p.Title))
		}
		if p.Type != "" {
			lines = append(lines, fmt.Sprintf("- type: %s", p.Type))
		}
		if p.ListingPrice > 0 {
			lines = append(lines, fmt.Sprintf("- listing_price: %.0f", p.ListingPrice))
		}
		if p.City != "" {
			lines = append(lines, fmt.Sprintf("- city: %s", p.City))
		}
		if p.Address != "" {
			lines = append(lines, fmt.Sprintf("- address: %s", p.Address))
		}
		if p.Image != "" {
			lines = append(lines, fmt.Sprintf("- image: %s", p.Image))
		}
		template := strings.TrimSpace(req.UserPromptTemplate)
		if template == "" {
			template = "I want market value insights for this property."
		}
		lines = append(lines, "", "User intent:", template, "", "User message:", req.Message)
		effectiveMessage = strings.Join(lines, "\n")
	}

	userID, _ := ctx.Values().GetUint("userID")
	persona := strings.TrimSpace(req.Persona)
	if persona == "" {
		persona = strings.TrimSpace(ctx.GetHeader("X-Agent-Persona"))
	}
	if persona == "" {
		persona = "buyer"
	}
	requestedTier := strings.TrimSpace(req.Tier)
	if requestedTier == "" {
		requestedTier = strings.TrimSpace(ctx.GetHeader("X-Agent-Tier"))
	}
	tier := resolveAgentTier(userID, persona, requestedTier)

	rateKey := fmt.Sprintf("anon:%s", strings.TrimSpace(req.AnonSessionID))
	if userID > 0 {
		rateKey = fmt.Sprintf("user:%d", userID)
	}
	if !ai.AllowAgentRun(rateKey, tier) {
		ctx.StatusCode(http.StatusTooManyRequests)
		ctx.JSON(iris.Map{"error": "Agent rate limit exceeded", "code": "AGENT_RATE_LIMIT"})
		return
	}

	if aiService.ContainsBadWords(req.Message) {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"success": false, "blocked": true})
		return
	}

	runID := uuid.NewString()
	sessionKey := strings.TrimSpace(req.AnonSessionID)
	if sessionKey == "" {
		sessionKey = fmt.Sprintf("anon_sess_%d", time.Now().UnixNano())
	}

	var dbSessionID uint
	if userID > 0 {
		var session models.AIChatSession
		if req.SessionID != nil && *req.SessionID > 0 {
			if err := storage.DB.Where("id = ? AND user_id = ?", *req.SessionID, userID).First(&session).Error; err != nil {
				ctx.StatusCode(http.StatusNotFound)
				ctx.JSON(iris.Map{"error": "Session not found"})
				return
			}
		} else {
			session = models.AIChatSession{
				UserID: userID,
				Title:  aiService.GenerateSessionTitle(req.Message),
			}
			if err := storage.DB.Create(&session).Error; err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				ctx.JSON(iris.Map{"error": "Failed to create session"})
				return
			}
		}
		dbSessionID = session.ID
		sessionKey = fmt.Sprintf("%d", session.ID)

		_ = storage.DB.Create(&models.AIChatMessage{
			SessionID: session.ID,
			Role:      "user",
			Content:   req.Message,
		}).Error
	}

	hist := make([]ai.ChatMessage, 0, len(req.History))
	for _, h := range req.History {
		r := strings.ToLower(strings.TrimSpace(h.Role))
		if r != "user" && r != "assistant" {
			continue
		}
		c := strings.TrimSpace(h.Content)
		if c == "" {
			continue
		}
		hist = append(hist, ai.ChatMessage{Role: r, Content: c})
	}

	ctx.ContentType("text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	w := ctx.ResponseWriter()
	flusher, ok := w.(http.Flusher)
	if !ok {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	writeEvent := func(ev ai.AgentEvent) {
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
		flusher.Flush()
	}

	reqCtx := ctx.Request().Context()
	out, err := MeskenyGPTService.HandleAgentRun(reqCtx, ai.AgentRunInput{
		ChatInput: ai.ChatInput{
			UserID:    userID,
			SessionID: sessionKey,
			Text:      effectiveMessage,
			DeepThink: req.DeepThink,
			History:   hist,
		},
		RunID:   runID,
		Persona: persona,
		Tier:    tier,
	}, writeEvent)

	if err != nil {
		writeEvent(ai.AgentEvent{
			Type: ai.AgentEventStepError, RunID: runID,
			StepID: "deliver", Error: err.Error(),
		})
		return
	}

	if userID > 0 && dbSessionID > 0 && out.Message.Content != "" {
		qrJSON, _ := json.Marshal(out.QuickReplies)
		_ = storage.DB.Create(&models.AIChatMessage{
			SessionID: dbSessionID,
			Role:      "assistant",
			Content:   sanitizeAIOutput(out.Message.Content),
			Metadata:  string(qrJSON),
		}).Error
		storage.DB.Model(&models.AIChatSession{}).Where("id = ?", dbSessionID).Update("updated_at", time.Now())
	}

	_ = out
}
