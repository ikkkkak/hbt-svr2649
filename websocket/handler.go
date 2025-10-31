package websocket

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	pushsvc "apartments-clone-server/services/push"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/iris/v12"
)

var GlobalHub *Hub

func InitHub() {
	GlobalHub = NewHub()
	go GlobalHub.Run()
}

func HandleWebSocket(ctx iris.Context) {
	log.Printf("🔌 WebSocket connection attempt to group %s", ctx.Params().Get("groupID"))

	// Get token from query parameter (WebSocket doesn't support custom headers)
	token := ctx.URLParam("token")
	if token == "" {
		log.Printf("❌ WS auth failed: missing token query param")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	// Verify the token and extract userID
	userID, err := verifyJWTToken(token)
	if err != nil || userID == 0 {
		log.Printf("❌ WS auth failed: invalid token: %v", err)
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		log.Printf("❌ WS bad request: invalid groupID: %v", err)
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	log.Printf("🔐 WS authenticated: userID=%d groupID=%d", userID, groupID)
	handleWebSocketWithUser(ctx, userID, groupID)
}

func verifyJWTToken(tokenString string) (uint, error) {
	// Use the same secret as the main middleware
	accessTokenSecret := os.Getenv("ACCESS_TOKEN_SECRET")
	if accessTokenSecret == "" {
		return 0, fmt.Errorf("ACCESS_TOKEN_SECRET not set")
	}

	// Parse and verify the JWT token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(accessTokenSecret), nil
	})
	if err != nil {
		return 0, fmt.Errorf("parse token error: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid token")
	}

	// Extract userID from claims
	// Try common keys: id, ID, userID
	if id, ok := claims["id"].(float64); ok {
		return uint(id), nil
	}
	if id, ok := claims["ID"].(float64); ok {
		return uint(id), nil
	}
	if id, ok := claims["userID"].(float64); ok {
		return uint(id), nil
	}

	log.Printf("❌ WS auth failed: userID not found in token claims: %+v", claims)
	return 0, fmt.Errorf("userID not found in token")
}

func handleWebSocketWithUser(ctx iris.Context, userID uint, groupID uint) {
	// Verify membership
	var membership models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, userID).First(&membership).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	// Get user info
	var userModel models.User
	if err := storage.DB.Where("id = ?", userID).First(&userModel).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(ctx.ResponseWriter(), ctx.Request(), nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	client := &Client{
		hub:      GlobalHub,
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   userID,
		groupID:  groupID,
		username: fmt.Sprintf("%s %s", userModel.FirstName, userModel.LastName),
	}

	log.Printf("🔌 Registering client: userID=%d, groupID=%d", userID, groupID)
	GlobalHub.register <- client

	go client.writePump()
	client.readPump()
}

func BroadcastNewMessage(groupID uint, msg models.ChatMessage) {
	var sender models.User
	storage.DB.Where("id = ?", msg.SenderID).First(&sender)

	messageData := &MessageData{
		ID:        msg.ID,
		GroupID:   msg.GroupID,
		SenderID:  msg.SenderID,
		Content:   msg.Content,
		Color:     msg.Color,
		CreatedAt: msg.CreatedAt.Format(time.RFC3339),
		Sender: &User{
			ID:        sender.ID,
			FirstName: sender.FirstName,
			LastName:  sender.LastName,
		},
	}

    log.Printf("📤 Broadcasting message %d to group %d", msg.ID, groupID)
    // Log push candidates before sending
    pushsvc.LogGroupTokens(groupID, msg.SenderID)
	
	// Get push tokens FIRST before broadcast (in case broadcast blocks)
	tokens := pushsvc.GetGroupPushTokens(groupID, msg.SenderID)
	log.Printf("🔔 Push: group=%d sender=%d tokens=%d", groupID, msg.SenderID, len(tokens))
	
	// Send pushes IMMEDIATELY (don't wait for broadcast)
	if len(tokens) > 0 {
		preview := msg.Content
		if len(preview) > 120 { preview = preview[:120] + "…" }
		log.Printf("🔔 Sending push notifications immediately for %d tokens", len(tokens))
		go func() {
			if err := pushsvc.EnqueuePush(tokens, fmt.Sprintf("%s %s", sender.FirstName, sender.LastName), preview); err != nil {
				log.Printf("⚠️ EnqueuePush error: %v", err)
			}
		}()
	}
	
	// Now broadcast to WebSocket clients
	GlobalHub.broadcastToGroup(groupID, &BroadcastMessage{
		GroupID: groupID,
		UserID:  msg.SenderID,
		Type:    "message",
		Message: messageData,
	})
	log.Printf("✅ BroadcastToGroup completed")
}

func BroadcastReadReceipt(groupID, userID, msgID uint) {
	GlobalHub.broadcastToGroup(groupID, &BroadcastMessage{
		GroupID: groupID,
		UserID:  userID,
		Type:    "read_receipt",
		Data: map[string]interface{}{
			"messageId": msgID,
		},
	})
}

func BroadcastTyping(groupID, userID uint, username string) {
	var user models.User
	storage.DB.Where("id = ?", userID).First(&user)

	GlobalHub.broadcastToGroup(groupID, &BroadcastMessage{
		GroupID: groupID,
		UserID:  userID,
		Type:    "typing",
		Data: map[string]interface{}{
			"name": fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		},
	})
}
