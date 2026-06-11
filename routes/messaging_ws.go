package routes

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"apartments-clone-server/realtime"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/kataras/iris/v12"
)

var messagingUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type wsEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// MessagingWS is the unified websocket endpoint for messaging.
// URL: /ws?token=ACCESS_JWT
func MessagingWS(ctx iris.Context) {
	token := strings.TrimSpace(ctx.URLParam("token"))
	if token == "" {
		auth := strings.TrimSpace(ctx.GetHeader("Authorization"))
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
	}
	userID := parseUserIDFromJWT(token)
	if userID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	conn, err := messagingUpgrader.Upgrade(ctx.ResponseWriter(), ctx.Request(), nil)
	if err != nil {
		log.Printf("ws upgrade (messaging): %v", err)
		return
	}

	client := &realtime.UserClient{UserID: userID, SendChan: make(chan []byte, 32)}
	hub := realtime.UserHubInstance()
	hub.Register(client)

	// writer
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range client.SendChan {
			_ = conn.WriteMessage(websocket.TextMessage, msg)
		}
	}()

	// ping loop (helps mobile networks keepalive)
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(3*time.Second))
			}
		}
	}()

	// reader: we only accept lightweight realtime signals (typing/read).
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var env wsEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case "typing":
			// forward typing to the other user(s)
			var payload struct {
				ToUserID uint `json:"toUserId"`
			}
			if err := json.Unmarshal(env.Data, &payload); err != nil || payload.ToUserID == 0 {
				continue
			}
			out := map[string]any{
				"type": "typing",
				"data": json.RawMessage(env.Data),
			}
			b, _ := json.Marshal(out)
			hub.BroadcastToUser(payload.ToUserID, b)

		case "read":
			// forward read receipts to the other user(s)
			var payload struct {
				ToUserID uint `json:"toUserId"`
			}
			if err := json.Unmarshal(env.Data, &payload); err != nil || payload.ToUserID == 0 {
				continue
			}
			out := map[string]any{
				"type": "read",
				"data": json.RawMessage(env.Data),
			}
			b, _ := json.Marshal(out)
			hub.BroadcastToUser(payload.ToUserID, b)
		}
	}

	hub.Unregister(client)
	_ = conn.Close()
}

func parseUserIDFromJWT(tokenString string) uint {
	if tokenString == "" {
		return 0
	}
	secret := strings.TrimSpace(os.Getenv("ACCESS_TOKEN_SECRET"))
	if secret == "" {
		return 0
	}
	tok, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || tok == nil || !tok.Valid {
		return 0
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0
	}
	// existing tokens use "ID" (from utils.AccessToken)
	if v, ok := claims["ID"]; ok {
		switch vv := v.(type) {
		case float64:
			return uint(vv)
		case int:
			return uint(vv)
		}
	}
	if v, ok := claims["id"]; ok {
		switch vv := v.(type) {
		case float64:
			return uint(vv)
		case int:
			return uint(vv)
		}
	}
	return 0
}

