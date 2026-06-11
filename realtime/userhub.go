package realtime

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"apartments-clone-server/storage"
)

// UserClient is a single websocket connection for a specific user.
type UserClient struct {
	UserID   uint
	SendChan chan []byte
}

// UserHub broadcasts events to users (direct messaging, inbox updates, typing, read receipts).
// Supports multi-device: a user can have multiple active clients.
type UserHub struct {
	mu    sync.RWMutex
	users map[uint]map[*UserClient]bool
}

var globalUserHub = &UserHub{users: make(map[uint]map[*UserClient]bool)}

func UserHubInstance() *UserHub { return globalUserHub }

func (h *UserHub) Register(c *UserClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.users[c.UserID] == nil {
		h.users[c.UserID] = make(map[*UserClient]bool)
	}
	h.users[c.UserID][c] = true
}

func (h *UserHub) Unregister(c *UserClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.users[c.UserID]; ok {
		if _, exists := set[c]; exists {
			delete(set, c)
			close(c.SendChan)
			if len(set) == 0 {
				delete(h.users, c.UserID)
			}
		}
	}
}

func (h *UserHub) BroadcastToUser(userID uint, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.users[userID]
	for cli := range set {
		select {
		case cli.SendChan <- payload:
		default:
			// drop if blocked
		}
	}
}

// BroadcastToUsers broadcasts the same payload to multiple users.
func (h *UserHub) BroadcastToUsers(userIDs []uint, payload []byte) {
	seen := make(map[uint]bool, len(userIDs))
	for _, uid := range userIDs {
		if uid == 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		h.BroadcastToUser(uid, payload)
	}
}

// Redis fan-out (optional). If Redis is configured, we publish events so multiple server instances can broadcast.
const redisUserEventsChannel = "meskeny:user-events:v1"

func PublishUserEvent(ctx context.Context, userIDs []uint, event any) {
	if storage.Redis == nil {
		return
	}
	b, err := json.Marshal(event)
	if err != nil {
		return
	}
	wrapped := map[string]any{
		"user_ids": userIDs,
		"payload":  json.RawMessage(b),
	}
	wb, err := json.Marshal(wrapped)
	if err != nil {
		return
	}
	_ = storage.Redis.Publish(ctx, redisUserEventsChannel, wb).Err()
}

// StartUserHubRedisSubscriber should be called once at startup (best-effort).
func StartUserHubRedisSubscriber() {
	if storage.Redis == nil {
		return
	}
	go func() {
		ctx := context.Background()
		sub := storage.Redis.Subscribe(ctx, redisUserEventsChannel)
		defer sub.Close()

		ch := sub.Channel()
		for msg := range ch {
			var wrapped struct {
				UserIDs []uint          `json:"user_ids"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &wrapped); err != nil {
				continue
			}
			if len(wrapped.UserIDs) == 0 || len(wrapped.Payload) == 0 {
				continue
			}
			UserHubInstance().BroadcastToUsers(wrapped.UserIDs, wrapped.Payload)
		}
	}()

	// Small delay to avoid noisy logs during boot if Redis is flapping.
	time.AfterFunc(2*time.Second, func() {
		log.Printf("✅ UserHub Redis subscriber started (%s)", redisUserEventsChannel)
	})
}

