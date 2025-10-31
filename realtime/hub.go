package realtime

import (
    "sync"
)

type Client struct {
    UserID   uint
    GroupID  uint
    SendChan chan []byte
}

type Hub struct {
    // groupID -> set of clients
    groups map[uint]map[*Client]bool
    mu     sync.RWMutex
}

var globalHub = &Hub{groups: make(map[uint]map[*Client]bool)}

func HubInstance() *Hub { return globalHub }

func (h *Hub) Register(c *Client) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.groups[c.GroupID] == nil {
        h.groups[c.GroupID] = make(map[*Client]bool)
    }
    h.groups[c.GroupID][c] = true
}

func (h *Hub) Unregister(c *Client) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if set, ok := h.groups[c.GroupID]; ok {
        if _, exists := set[c]; exists {
            delete(set, c)
            close(c.SendChan)
            if len(set) == 0 {
                delete(h.groups, c.GroupID)
            }
        }
    }
}

// Broadcast a JSON payload to all clients in a group, optionally excluding a userID
func (h *Hub) Broadcast(groupID uint, payload []byte, excludeUserID uint) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    if set, ok := h.groups[groupID]; ok {
        for cli := range set {
            if excludeUserID != 0 && cli.UserID == excludeUserID {
                continue
            }
            select {
            case cli.SendChan <- payload:
            default:
                // drop if blocked
            }
        }
    }
}



