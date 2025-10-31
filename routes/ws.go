package routes

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"apartments-clone-server/realtime"

	"github.com/gorilla/websocket"
	"github.com/kataras/iris/v12"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type wsMessage struct {
	Type     string      `json:"type"`
	GroupID  uint        `json:"groupId"`
	SenderID uint        `json:"senderId"`
	Message  interface{} `json:"message"`
}

func GroupWS(ctx iris.Context) {
	gidStr := ctx.Params().Get("groupID")
	gid64, _ := strconv.ParseUint(gidStr, 10, 64)
	groupID := uint(gid64)

	// In a real app, extract user from JWT middleware context
	userID := uint(0)
	if id := ctx.GetHeader("X-User-ID"); id != "" {
		if u, err := strconv.ParseUint(id, 10, 64); err == nil {
			userID = uint(u)
		}
	}

	conn, err := upgrader.Upgrade(ctx.ResponseWriter(), ctx.Request(), nil)
	if err != nil {
		log.Println("ws upgrade:", err)
		return
	}

	client := &realtime.Client{UserID: userID, GroupID: groupID, SendChan: make(chan []byte, 16)}
	hub := realtime.HubInstance()
	hub.Register(client)

	go func() {
		for msg := range client.SendChan {
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	}()

	for {
		// We only read to keep connection alive (and optional typing events)
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		// echo typing events to others in group (optional)
		var incoming map[string]interface{}
		_ = json.Unmarshal(data, &incoming)
		if incoming["type"] == "typing" {
			b, _ := json.Marshal(incoming)
			hub.Broadcast(groupID, b, userID)
		}
	}

	hub.Unregister(client)
	conn.Close()
}
