package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kataras/iris/v12"
)

// AI Service instance
var aiService = services.NewAIService()

// SendAIChatMessage handles sending a message to the AI
func SendAIChatMessage(ctx iris.Context) {
	userID, _ := ctx.Values().GetUint("userID")
	if userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	var req struct {
		Message   string `json:"message"`
		SessionID *uint  `json:"session_id"`
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

	// Get conversation history
	var historyMessages []models.AIChatMessage
	storage.DB.Where("session_id = ? AND id < ?", session.ID, userMessage.ID).
		Order("created_at ASC").
		Limit(20).
		Find(&historyMessages)

	// Build history for AI
	history := make([]map[string]string, 0)
	for _, msg := range historyMessages {
		history = append(history, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	// Send to AI
	aiResponse, err := aiService.SendMessage(req.Message, history)
	if err != nil {
		fmt.Printf("❌ AI Chat: Error from Gemini API: %v\n", err)
		aiResponse = "Je suis désolé, une erreur est survenue. Veuillez réessayer."
	}

	// Generate quick replies
	quickReplies := aiService.GenerateQuickReplies(req.Message, aiResponse)
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

	ctx.JSON(iris.Map{
		"success":    true,
		"session_id": session.ID,
		"message": iris.Map{
			"id":            aiMessage.ID,
			"role":          aiMessage.Role,
			"content":       aiMessage.Content,
			"timestamp":     aiMessage.CreatedAt.UnixMilli(),
			"quick_replies": quickReplies,
		},
	})
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
