package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512KB
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // Allow all origins in dev
}

type Hub struct {
	groups     map[uint]map[uint]*Client // groupID -> userID -> client
	broadcast  chan *BroadcastMessage
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	userID   uint
	groupID  uint
	username string
}

type BroadcastMessage struct {
	GroupID uint         `json:"groupId"`
	UserID  uint         `json:"userId"`
	Type    string       `json:"type"` // message, typing, read_receipt, reply
	Data    interface{}  `json:"data"`
	Message *MessageData `json:"message,omitempty"`
}

type MessageData struct {
	ID          uint         `json:"id"`
	GroupID     uint         `json:"groupId"`
	SenderID    uint         `json:"senderId"`
	Content     string       `json:"content"`
	Color       string       `json:"color"`
	CreatedAt   string       `json:"createdAt"`
	Sender      *User        `json:"sender,omitempty"`
	RepliedToID *uint        `json:"repliedToId,omitempty"`
	RepliedTo   *MessageData `json:"repliedTo,omitempty"`
}

type User struct {
	ID        uint   `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

func NewHub() *Hub {
	return &Hub{
		groups:     make(map[uint]map[uint]*Client),
		broadcast:  make(chan *BroadcastMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.groups[client.groupID] == nil {
				h.groups[client.groupID] = make(map[uint]*Client)
			}
			h.groups[client.groupID][client.userID] = client
			h.mu.Unlock()

			log.Printf("👤 User %d joined group %d", client.userID, client.groupID)

			// Notify others in group
			h.broadcastToGroup(client.groupID, &BroadcastMessage{
				Type:    "user_joined",
				UserID:  client.userID,
				GroupID: client.groupID,
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if group, exists := h.groups[client.groupID]; exists {
				if _, exists := group[client.userID]; exists {
					delete(group, client.userID)
					close(client.send)
					log.Printf("👋 User %d left group %d", client.userID, client.groupID)
				}
				if len(group) == 0 {
					delete(h.groups, client.groupID)
				}
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			log.Printf("📬 Hub received broadcast for group %d, type: %s", msg.GroupID, msg.Type)
			h.mu.RLock()
			group, exists := h.groups[msg.GroupID]
			h.mu.RUnlock()
			if !exists {
				log.Printf("⚠️ No clients connected for group %d", msg.GroupID)
				continue
			}

			log.Printf("📨 Broadcasting to %d clients in group %d", len(group), msg.GroupID)
			data := h.serializeMessage(msg)
			for userID, client := range group {
				select {
				case client.send <- data:
					log.Printf("✅ Message sent to user %d", userID)
				default:
					log.Printf("⚠️ Failed to send message to user %d, closing connection", userID)
					close(client.send)
					delete(group, userID)
				}
			}
		}
	}
}

func (h *Hub) serializeMessage(msg *BroadcastMessage) []byte {
	data, _ := json.Marshal(msg)
	return data
}

func (h *Hub) broadcastToGroup(groupID uint, msg *BroadcastMessage) {
	// Send message to hub's broadcast channel to be processed properly
	h.broadcast <- msg
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming message
		var incoming struct {
			Type string      `json:"type"`
			Data interface{} `json:"data"`
		}
		if err := json.Unmarshal(message, &incoming); err != nil {
			continue
		}

		switch incoming.Type {
		case "typing":
			c.hub.broadcastToGroup(c.groupID, &BroadcastMessage{
				GroupID: c.groupID,
				UserID:  c.userID,
				Type:    "typing",
				Data:    incoming.Data,
			})
		case "read":
			c.hub.broadcastToGroup(c.groupID, &BroadcastMessage{
				GroupID: c.groupID,
				UserID:  c.userID,
				Type:    "read_receipt",
				Data:    incoming.Data,
			})
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
