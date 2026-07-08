package routes

import (
	"apartments-clone-server/MeskenyGPT/ai"
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

// Legacy AI Service instance (Gemini/OpenRouter direct calls)
var aiService = services.NewAIService()

// MeskenyGPTService is the new AI backend (structured, multi-package).
// It is initialised from main.go and used here as an orchestrator.
var MeskenyGPTService ai.Service

// SendAIChatMessage handles sending a message to the AI
func SendAIChatMessage(ctx iris.Context) {
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

	if req.Message == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Message is required"})
		return
	}

	effectiveMessage := req.Message
	if req.SharedProperty != nil && req.SharedProperty.ID > 0 {
		p := req.SharedProperty
		lines := []string{
			"Property context (shared from app card):",
			"- internal_ref: redacted",
		}
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
		if offersCtx := buildPropertyOffersAIContext(p.ID); offersCtx != "" {
			lines = append(lines, "", offersCtx)
		}

		template := strings.TrimSpace(req.UserPromptTemplate)
		if template == "" {
			template = "I want to know the market value for this property, considering location and luxury comparables, and share practical insights."
		}
		lines = append(lines,
			"",
			"User intent:",
			template,
			"",
			"User message:",
			req.Message,
		)
		effectiveMessage = strings.Join(lines, "\n")
	}

	userID, _ := ctx.Values().GetUint("userID")

	// If not logged in, run a lightweight, stateless AI response:
	// - No DB session
	// - No message persistence
	// - Still applies bad-word filter
	if userID == 0 {
		// If MeskenyGPT is available, delegate anonymous chat entirely to it.
		if MeskenyGPTService != nil {
			anonSID := strings.TrimSpace(req.AnonSessionID)
			if anonSID == "" {
				anonSID = fmt.Sprintf("anon_sess_%d", time.Now().UnixNano())
			}
			anonHist := make([]ai.ChatMessage, 0, len(req.History))
			for _, h := range req.History {
				r := strings.ToLower(strings.TrimSpace(h.Role))
				if r != "user" && r != "assistant" {
					continue
				}
				c := strings.TrimSpace(h.Content)
				if c == "" {
					continue
				}
				anonHist = append(anonHist, ai.ChatMessage{Role: r, Content: c})
				if len(anonHist) >= 20 {
					break
				}
			}
			// Client often includes the current user turn in history; avoid duplicating it with Text.
			if len(anonHist) > 0 {
				last := anonHist[len(anonHist)-1]
				if last.Role == "user" && strings.TrimSpace(last.Content) == strings.TrimSpace(req.Message) {
					anonHist = anonHist[:len(anonHist)-1]
				}
			}
			out, err := MeskenyGPTService.HandleChatTurn(context.Background(), ai.ChatInput{
				UserID:    0,
				SessionID: anonSID,
				Text:      effectiveMessage,
				DeepThink: req.DeepThink,
				History:   anonHist,
			})
			if err != nil {
				fmt.Printf("❌ MeskenyGPT (anonymous) error: %v\n", err)
				ctx.StatusCode(iris.StatusInternalServerError)
				ctx.JSON(iris.Map{"error": "Failed to get AI response"})
				return
			}

			out.Message.Content = sanitizeAIOutput(out.Message.Content)
			fmt.Printf("🤖 MeskenyGPT (anonymous) final message: %q\n", out.Message.Content)

			resp := iris.Map{
				"success":    true,
				"session_id": nil,
				"anon_session_id": anonSID,
				"message": iris.Map{
					"id":                    fmt.Sprintf("anon_%d", time.Now().UnixNano()),
					"role":                  out.Message.Role,
					"content":               out.Message.Content,
					"timestamp":             time.Now().UnixMilli(),
					"quick_replies":         out.QuickReplies,
					"propertyRecommendations": out.PropertyRecommendations,
					"interaction_id":        out.InteractionID,
				},
			}

			if payload, err := json.MarshalIndent(resp, "", "  "); err == nil {
				fmt.Printf("🤖 MeskenyGPT Chat (anonymous) response payload:\n%s\n", string(payload))
			}

			ctx.JSON(resp)
			return
		}

		// Fallback: legacy aiService path if MeskenyGPT is not initialised.
		// (Existing behavior preserved as backup.)
		if aiService.ContainsBadWords(req.Message) {
			fmt.Println("🚫 AI Chat (anonymous): blocked message for bad words")
			ctx.StatusCode(iris.StatusForbidden)
			ctx.JSON(iris.Map{
				"success": false,
				"blocked": true,
				"message": "Votre message contient un langage inapproprié.",
			})
			return
		}
		aiResponse, err := aiService.SendMessage(effectiveMessage, nil, req.DeepThink)
		if err != nil {
			fmt.Printf("❌ AI Chat (anonymous): error from AI service: %v\n", err)
			aiResponse = "Je suis désolé, une erreur est survenue. Veuillez réessayer."
		}
		quickReplies := aiService.GenerateQuickReplies(req.Message, aiResponse)
		resp := iris.Map{
			"success":    true,
			"session_id": nil,
			"message": iris.Map{
				"id":                    fmt.Sprintf("anon_%d", time.Now().UnixNano()),
				"role":                  "assistant",
				"content":               aiResponse,
				"timestamp":             time.Now().UnixMilli(),
				"quick_replies":         quickReplies,
				"propertyRecommendations": []models.AIPropertyRecommendation{},
			},
		}
		ctx.JSON(resp)
		return
	}

	// Check for bad words
	if aiService.ContainsBadWords(req.Message) {
		fmt.Printf("🚫 AI Chat: Blocked message from user %d for bad words\n", userID)
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{
			"success": false,
			"blocked": true,
			"message": "Votre message contient un langage inapproprié. La conversation a été fermée.",
		})
		return
	}

	// Get or create session
	var session models.AIChatSession
	if req.SessionID != nil && *req.SessionID > 0 {
		if err := storage.DB.Where("id = ? AND user_id = ?", *req.SessionID, userID).First(&session).Error; err != nil {
			ctx.StatusCode(iris.StatusNotFound)
			ctx.JSON(iris.Map{"error": "Session not found"})
			return
		}

		// Check if session is blocked
		if session.IsBlocked {
			ctx.StatusCode(iris.StatusForbidden)
			ctx.JSON(iris.Map{
				"success": false,
				"blocked": true,
				"message": "Cette conversation a été fermée pour violation des règles de conduite.",
			})
			return
		}
	} else {
		// Create new session
		session = models.AIChatSession{
			UserID:    userID,
			Title:     aiService.GenerateSessionTitle(req.Message),
			IsBlocked: false,
		}
		if err := storage.DB.Create(&session).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to create session"})
			return
		}
		fmt.Printf("✅ AI Chat: Created new session %d for user %d\n", session.ID, userID)
	}

	// Save user message
	userMessage := models.AIChatMessage{
		SessionID: session.ID,
		Role:      "user",
		Content:   req.Message,
		IsBlocked: false,
	}
	if err := storage.DB.Create(&userMessage).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to save message"})
		return
	}

	// Prior turns only (current message already saved). Newest 20 before this message.
	var historyMessages []models.AIChatMessage
	storage.DB.Where("session_id = ? AND id < ?", session.ID, userMessage.ID).
		Order("created_at DESC").
		Limit(20).
		Find(&historyMessages)
	for i, j := 0, len(historyMessages)-1; i < j; i, j = i+1, j-1 {
		historyMessages[i], historyMessages[j] = historyMessages[j], historyMessages[i]
	}

	// Build history for legacy AI + MeskenyGPT
	history := make([]map[string]string, 0, len(historyMessages))
	historyForMeskeny := make([]ai.ChatMessage, 0, len(historyMessages))
	for _, msg := range historyMessages {
		history = append(history, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
		if msg.Role == "user" || msg.Role == "assistant" {
			historyForMeskeny = append(historyForMeskeny, ai.ChatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	var aiResponse string
	var quickReplies []map[string]string
	recs := []models.AIPropertyRecommendation{}
	var interactionID uint
	var chatEscalation *ai.EscalationInfo

	// If MeskenyGPT is available, delegate the smart part to it.
	if MeskenyGPTService != nil {
		out, err := MeskenyGPTService.HandleChatTurn(context.Background(), ai.ChatInput{
			UserID:    userID,
			SessionID: fmt.Sprintf("%d", session.ID),
			Text:      effectiveMessage,
			DeepThink: req.DeepThink,
			History:   historyForMeskeny,
		})
		if err != nil {
			fmt.Printf("❌ MeskenyGPT (user %d) error: %v\n", userID, err)
			aiResponse = "Je suis désolé, une erreur est survenue. Veuillez réessayer."
		} else {
			aiResponse = sanitizeAIOutput(out.Message.Content)
			interactionID = out.InteractionID
			if out.Escalation != nil {
				chatEscalation = out.Escalation
				var uid *uint
				if userID > 0 {
					uid = &userID
				}
				if row, err := ai.GetEscalation(context.Background(), storage.DB, out.Escalation.ID); err == nil {
					services.NotifyEscalationCreated(context.Background(), row, uid, fmt.Sprintf("%d", session.ID))
				}
			}
			fmt.Printf("🤖 MeskenyGPT (user %d, session %d) final message: %q\n", userID, session.ID, aiResponse)

			// Map MeskenyGPT quick replies to legacy shape
			for _, qr := range out.QuickReplies {
				quickReplies = append(quickReplies, map[string]string{
					"id":     qr.ID,
					"text":   qr.Text,
					"action": qr.Action,
				})
			}

			// Map MeskenyGPT cards to models.AIPropertyRecommendation
			for _, c := range out.PropertyRecommendations {
				recs = append(recs, models.AIPropertyRecommendation{
					ID:       c.ID,
					Title:    c.Title,
					Price:    c.Price,
					Currency: c.Currency,
					City:     c.City,
					Bedrooms: c.Bedrooms,
					Image:    c.Image,
					Type:     c.Type,
				})
			}
		}
	} else {
		// Fallback to legacy aiService if MeskenyGPT is not initialised.
		rsp, err := aiService.SendMessage(effectiveMessage, history, req.DeepThink)
		if err != nil {
			fmt.Printf("❌ AI Chat: Error from AI service: %v\n", err)
			rsp = "Je suis désolé, une erreur est survenue. Veuillez réessayer."
		}
		aiResponse = sanitizeAIOutput(rsp)
		quickReplies = aiService.GenerateQuickReplies(req.Message, aiResponse)
	}

	// Generate quick replies JSON metadata for persistence
	if quickReplies == nil {
		quickReplies = []map[string]string{}
	}
	quickRepliesJSON, _ := json.Marshal(quickReplies)

	// Save AI response
	aiMessage := models.AIChatMessage{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   aiResponse,
		IsBlocked: false,
		Metadata:  string(quickRepliesJSON),
	}
	if err := storage.DB.Create(&aiMessage).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to save AI response"})
		return
	}

	// Update session timestamp
	storage.DB.Model(&session).Update("updated_at", time.Now())

	fmt.Printf("✅ AI Chat: Processed message for user %d, session %d\n", userID, session.ID)

	resp := iris.Map{
		"success":    true,
		"session_id": session.ID,
		"message": iris.Map{
			"id":                      aiMessage.ID,
			"role":                    aiMessage.Role,
			"content":                 aiMessage.Content,
			"timestamp":               aiMessage.CreatedAt.UnixMilli(),
			"quick_replies":           quickReplies,
			"propertyRecommendations": recs,
			"interaction_id":          interactionID,
		},
	}
	if chatEscalation != nil {
		resp["escalation"] = chatEscalation
	}

	if payload, err := json.MarshalIndent(resp, "", "  "); err == nil {
		fmt.Printf("🤖 AI Chat (user %d) response payload:\n%s\n", userID, string(payload))
	}

	ctx.JSON(resp)
}

func sanitizeAIOutput(in string) string {
	out := in
	// Remove leaked internal ID mentions from model output.
	out = regexp.MustCompile(`(?i)\b(ID|id)\s*[:#]?\s*\d+\b`).ReplaceAllString(out, "listing")
	return out
}

func buildPropertyOffersAIContext(propertyID uint) string {
	if propertyID == 0 {
		return ""
	}

	type offerAgg struct {
		Count int     `gorm:"column:count"`
		Min   float64 `gorm:"column:min"`
		Max   float64 `gorm:"column:max"`
		Avg   float64 `gorm:"column:avg"`
	}

	var agg offerAgg
	if err := storage.DB.
		Raw("SELECT COUNT(*) as count, COALESCE(MIN(amount),0) as min, COALESCE(MAX(amount),0) as max, COALESCE(AVG(amount),0) as avg FROM property_offers WHERE property_id = ?", propertyID).
		Scan(&agg).Error; err != nil {
		fmt.Printf("⚠️ AI Chat: failed to load offer aggregates for property %d: %v\n", propertyID, err)
		return ""
	}

	if agg.Count == 0 {
		return "Offer signals from DB: no offers have been recorded yet for this property."
	}

	var acceptedCount int64
	var pendingCount int64
	var rejectedCount int64

	storage.DB.Model(&models.PropertyOffer{}).Where("property_id = ? AND status = ?", propertyID, "accepted").Count(&acceptedCount)
	storage.DB.Model(&models.PropertyOffer{}).Where("property_id = ? AND status = ?", propertyID, "pending").Count(&pendingCount)
	storage.DB.Model(&models.PropertyOffer{}).Where("property_id = ? AND status = ?", propertyID, "rejected").Count(&rejectedCount)

	var latestOffers []models.PropertyOffer
	if err := storage.DB.
		Where("property_id = ?", propertyID).
		Order("created_at DESC").
		Limit(5).
		Find(&latestOffers).Error; err != nil {
		fmt.Printf("⚠️ AI Chat: failed to load latest offers for property %d: %v\n", propertyID, err)
	}

	lines := []string{
		"Offer signals from DB (use these in valuation confidence):",
		fmt.Sprintf("- offers_count: %d", agg.Count),
		fmt.Sprintf("- offer_min: %.0f", agg.Min),
		fmt.Sprintf("- offer_avg: %.0f", agg.Avg),
		fmt.Sprintf("- offer_max: %.0f", agg.Max),
		fmt.Sprintf("- offer_status_counts: accepted=%d pending=%d rejected=%d", acceptedCount, pendingCount, rejectedCount),
	}

	if len(latestOffers) > 0 {
		lines = append(lines, "- latest_offers:")
		for _, o := range latestOffers {
			lines = append(lines, fmt.Sprintf("  - amount=%.0f status=%s created_at=%s", o.Amount, o.Status, o.CreatedAt.Format(time.RFC3339)))
		}
	}

	return strings.Join(lines, "\n")
}

// buildAIPropertyRecommendationsFromContext turns a parsed MessageContext into a
// small list of real PropertySale cards that the frontend can render as
// Meskeny-style cards with images and IDs.
func buildAIPropertyRecommendationsFromContext(ctx services.MessageContext) []models.AIPropertyRecommendation {
	// Only run for concrete property-search intents; greetings / help / etc
	// should not trigger DB searches.
	switch ctx.Intent {
	case services.IntentSearchRent,
		services.IntentSearchBuy,
		services.IntentSearchAny,
		services.IntentSearchLand,
		services.IntentSearchCommercial,
		services.IntentSearchByBudget,
		services.IntentSearchByLocation,
		services.IntentSearchByRooms,
		services.IntentSearchByType:
		// continue
	default:
		return []models.AIPropertyRecommendation{}
	}

	var sales []models.PropertySale

	q := storage.DB.
		Model(&models.PropertySale{}).
		Where("is_published = ? AND is_deactivated = ? AND is_sold = ?", true, false, false)

	// Filter by city if we detected one
	if len(ctx.Cities) > 0 {
		q = q.Where("LOWER(city) = ?", ctx.Cities[0])
	}

	// Filter by basic property type when possible (Maison, Appartement, etc.)
	if ctx.PropertyType != "" {
		q = q.Where("LOWER(property_type) = ?", strings.ToLower(ctx.PropertyType))
	}

	// Rough budget band filtering if we managed to extract a numeric budget
	minPrice := 0.0
	maxPrice := 0.0
	if ctx.Budget != "" {
		numRe := regexp.MustCompile(`(\d[\d\s,.]*)`)
		if m := numRe.FindStringSubmatch(ctx.Budget); len(m) > 1 {
			raw := strings.ReplaceAll(strings.ReplaceAll(m[1], " ", ""), ",", "")
			raw = strings.ReplaceAll(raw, "٫", ".")
			if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
				// Very rough band: 50%–150% of the mentioned price
				minPrice = v * 0.5
				maxPrice = v * 1.5
			}
		}
	}
	if minPrice > 0 && maxPrice > minPrice {
		q = q.Where("listing_price BETWEEN ? AND ?", minPrice, maxPrice)
	}

	// Prefer featured & newest first if no explicit budget ordering
	if minPrice == 0 {
		q = q.Order("is_gold DESC, is_featured DESC, created_at DESC")
	} else {
		// When we do have a budget, sort by closeness to that budget
		q = q.Order(fmt.Sprintf("ABS(listing_price - %f) ASC", (minPrice+maxPrice)/2))
	}

	if err := q.Limit(8).Find(&sales).Error; err != nil || len(sales) == 0 {
		return []models.AIPropertyRecommendation{}
	}

	recs := make([]models.AIPropertyRecommendation, 0, len(sales))
	for _, s := range sales {
		img := ""
		if len(s.Images) > 0 {
			img = s.Images[0]
		}

		recType := "sale"
		// In the future, if we plug in rental table, we can map rent/search
		// intents differently. For now, PropertySale is strictly "sale".

		recs = append(recs, models.AIPropertyRecommendation{
			ID:       s.ID,
			Title:    s.Title,
			Price:    s.ListingPrice,
			Currency: s.Currency,
			City:     s.City,
			Bedrooms: s.Bedrooms,
			Image:    img,
			Type:     recType,
		})
	}

	return recs
}

// isPropertySearchIntent tells us when a message is really about searching
// inventory; in those cases we never let the LLM invent properties.
func isPropertySearchIntent(intent services.Intent) bool {
	switch intent {
	case services.IntentSearchRent,
		services.IntentSearchBuy,
		services.IntentSearchAny,
		services.IntentSearchLand,
		services.IntentSearchCommercial,
		services.IntentSearchByBudget,
		services.IntentSearchByLocation,
		services.IntentSearchByRooms,
		services.IntentSearchByType:
		return true
	default:
		return false
	}
}

// buildSearchSummaryText returns a short, language-aware message that tells the
// user we found real properties and that they can view them as cards. It does
// NOT invent any fake listing details; all specifics come from the cards.
func buildSearchSummaryText(ctx services.MessageContext, count int) string {
	if count <= 0 {
		return ""
	}

	switch ctx.Lang {
	case services.LangAR, services.LangHASSANIYA:
		return fmt.Sprintf(
			"✅ عثرت على %d عقارًا حقيقيًا في قاعدة بيانات Meskeny يطابق تقريبًا طلبك.\nيمكنك استعراض هذه الخيارات في البطاقات أسفل هذه الرسالة، والضغط على أي عقار لفتح صفحة التفاصيل والصور بالكامل.",
			count,
		)
	case services.LangEN:
		return fmt.Sprintf(
			"✅ I found %d real properties in the Meskeny database that roughly match your request.\nBrowse them in the cards below and tap any property to open the full details and photos.",
			count,
		)
	default: // French
		return fmt.Sprintf(
			"✅ J'ai trouvé %d biens réels dans la base Meskeny qui correspondent à ta demande.\nTu peux les parcourir dans les cartes ci-dessous et appuyer sur un bien pour ouvrir la fiche complète avec les photos.",
			count,
		)
	}
}

// buildNoResultsText is used when the user clearly asked for a property search
// but the DB query found zero matching PropertySale rows.
func buildNoResultsText(ctx services.MessageContext) string {
	switch ctx.Lang {
	case services.LangAR, services.LangHASSANIYA:
		return "لم أجد أي عقار حاليًا في قاعدة بيانات Meskeny يطابق هذا الطلب بدقة.\nجرّب تعديل السعر أو الحي أو نوع العقار، وسأبحث لك من جديد في العقارات الحقيقية الموجودة لدينا."
	case services.LangEN:
		return "I couldn't find any live properties in the Meskeny database that match this request.\nTry adjusting the budget, area, or property type and I'll search again in the real listings we have."
	default: // French
		return "Je n'ai trouvé aucun bien en ligne dans la base Meskeny qui corresponde exactement à cette demande.\nEssaie d'ajuster le budget, le quartier ou le type de bien et je relancerai une recherche dans les annonces réelles."
	}
}

// GetAIChatSessions returns all chat sessions for a user
func GetAIChatSessions(ctx iris.Context) {
	userID, _ := ctx.Values().GetUint("userID")
	if userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	var sessions []models.AIChatSession
	if err := storage.DB.Where("user_id = ?", userID).
		Order("updated_at DESC").
		Limit(50).
		Find(&sessions).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to get sessions"})
		return
	}

	// Format response
	sessionsData := make([]iris.Map, 0)
	for _, s := range sessions {
		sessionsData = append(sessionsData, iris.Map{
			"id":         s.ID,
			"title":      s.Title,
			"is_blocked": s.IsBlocked,
			"created_at": s.CreatedAt.UnixMilli(),
			"updated_at": s.UpdatedAt.UnixMilli(),
		})
	}

	ctx.JSON(iris.Map{
		"success":  true,
		"sessions": sessionsData,
	})
}

// GetAIChatSession returns a specific chat session with messages
func GetAIChatSession(ctx iris.Context) {
	userID, _ := ctx.Values().GetUint("userID")
	sessionID, _ := ctx.Params().GetUint("sessionId")

	if userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	var session models.AIChatSession
	if err := storage.DB.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Session not found"})
		return
	}

	var messages []models.AIChatMessage
	storage.DB.Where("session_id = ?", sessionID).Order("created_at ASC").Find(&messages)

	// Format messages
	messagesData := make([]iris.Map, 0)
	for _, m := range messages {
		msgData := iris.Map{
			"id":         m.ID,
			"role":       m.Role,
			"content":    m.Content,
			"is_blocked": m.IsBlocked,
			"timestamp":  m.CreatedAt.UnixMilli(),
		}

		// Parse quick replies from metadata
		if m.Metadata != "" {
			var quickReplies []map[string]string
			if err := json.Unmarshal([]byte(m.Metadata), &quickReplies); err == nil {
				msgData["quick_replies"] = quickReplies
			}
		}

		messagesData = append(messagesData, msgData)
	}

	ctx.JSON(iris.Map{
		"success": true,
		"session": iris.Map{
			"id":         session.ID,
			"title":      session.Title,
			"is_blocked": session.IsBlocked,
			"created_at": session.CreatedAt.UnixMilli(),
			"updated_at": session.UpdatedAt.UnixMilli(),
			"messages":   messagesData,
		},
	})
}

// CreateAIChatSession creates a new chat session with initial greeting
func CreateAIChatSession(ctx iris.Context) {
	userID, _ := ctx.Values().GetUint("userID")
	if userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	// Create new session
	session := models.AIChatSession{
		UserID:    userID,
		Title:     "Nouvelle conversation",
		IsBlocked: false,
	}
	if err := storage.DB.Create(&session).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create session"})
		return
	}

	// Get initial greeting
	greeting, quickReplies := aiService.GetInitialGreeting()
	quickRepliesJSON, _ := json.Marshal(quickReplies)

	// Save greeting message
	greetingMessage := models.AIChatMessage{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   greeting,
		IsBlocked: false,
		Metadata:  string(quickRepliesJSON),
	}
	storage.DB.Create(&greetingMessage)

	fmt.Printf("✅ AI Chat: Created new session %d with greeting for user %d\n", session.ID, userID)

	ctx.JSON(iris.Map{
		"success":    true,
		"session_id": session.ID,
		"greeting": iris.Map{
			"id":            greetingMessage.ID,
			"role":          greetingMessage.Role,
			"content":       greetingMessage.Content,
			"timestamp":     greetingMessage.CreatedAt.UnixMilli(),
			"quick_replies": quickReplies,
		},
	})
}

// DeleteAIChatSession deletes a chat session
func DeleteAIChatSession(ctx iris.Context) {
	userID, _ := ctx.Values().GetUint("userID")
	sessionID, _ := ctx.Params().GetUint("sessionId")

	if userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	// Verify ownership
	var session models.AIChatSession
	if err := storage.DB.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Session not found"})
		return
	}

	// Delete messages first
	storage.DB.Where("session_id = ?", sessionID).Delete(&models.AIChatMessage{})

	// Delete session
	storage.DB.Delete(&session)

	fmt.Printf("✅ AI Chat: Deleted session %d for user %d\n", sessionID, userID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Session deleted",
	})
}

// GetAIGreeting returns the initial greeting for new conversations
func GetAIGreeting(ctx iris.Context) {
	greeting, quickReplies := aiService.GetInitialGreeting()

	ctx.JSON(iris.Map{
		"success": true,
		"greeting": iris.Map{
			"id":            fmt.Sprintf("msg_%d", time.Now().UnixMilli()),
			"role":          "assistant",
			"content":       greeting,
			"timestamp":     time.Now().UnixMilli(),
			"quick_replies": quickReplies,
		},
	})
}

// SendAIFeedback handles thumbs up/down and property click feedback.
func SendAIFeedback(ctx iris.Context) {
	var req struct {
		InteractionID uint    `json:"interaction_id"`
		Signal        string  `json:"signal"` // thumbs_up, thumbs_down, property_click
		Value         float64 `json:"value"`  // 1.0 positive, 0.0 negative; for property_click, property ID
	}
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	if req.InteractionID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "interaction_id is required"})
		return
	}
	signal := req.Signal
	if signal == "" {
		signal = "thumbs_up"
	}
	if signal != "thumbs_up" && signal != "thumbs_down" && signal != "property_click" {
		signal = "thumbs_up"
	}
	value := req.Value
	if signal == "thumbs_up" {
		value = 1.0
	} else if signal == "thumbs_down" {
		value = 0.0
	}
	_ = ai.RecordAIFeedback(context.Background(), req.InteractionID, signal, value)
	ctx.JSON(iris.Map{"success": true})
}
