package videoprocessing

import (
	"context"
	"encoding/json"
	"sync"

	"apartments-clone-server/realtime"
)

// ProcessingEvent is pushed to upload clients (WebSocket + SSE subscribers).
type ProcessingEvent struct {
	VideoID          uint   `json:"videoID"`
	ProcessingStatus string `json:"processingStatus"`
	ProcessingError  string `json:"processingError,omitempty"`
	Progress         int    `json:"progress"` // 0–100
	HlsURL           string `json:"hlsURL,omitempty"`
	MobileVideoURL   string `json:"mobileVideoURL,omitempty"`
	SpriteSheetURL   string `json:"spriteSheetURL,omitempty"`
	PreviewBlurURL   string `json:"previewBlurURL,omitempty"`
	EntityType       string `json:"entityType,omitempty"` // rent | sale
	Ready            bool   `json:"ready"`
}

var (
	sseMu    sync.RWMutex
	sseSubs  = map[uint]map[chan ProcessingEvent]struct{}{}
)

// PublishProcessing notifies a user via WebSocket and any SSE listeners.
func PublishProcessing(userID uint, ev ProcessingEvent) {
	if userID == 0 {
		return
	}
	broadcastSSE(ev)
	payload, _ := json.Marshal(map[string]any{
		"type": "video:processing",
		"data": ev,
	})
	hub := realtime.UserHubInstance()
	hub.BroadcastToUser(userID, payload)
	realtime.PublishUserEvent(context.Background(), []uint{userID}, map[string]any{
		"type": "video:processing",
		"data": ev,
	})
}

func broadcastSSE(ev ProcessingEvent) {
	sseMu.RLock()
	defer sseMu.RUnlock()
	set := sseSubs[ev.VideoID]
	for ch := range set {
		select {
		case ch <- ev:
		default:
		}
	}
}

// SubscribeSSE registers a listener for one video (caller must call UnsubscribeSSE).
func SubscribeSSE(videoID uint) chan ProcessingEvent {
	ch := make(chan ProcessingEvent, 8)
	sseMu.Lock()
	if sseSubs[videoID] == nil {
		sseSubs[videoID] = make(map[chan ProcessingEvent]struct{})
	}
	sseSubs[videoID][ch] = struct{}{}
	sseMu.Unlock()
	return ch
}

func UnsubscribeSSE(videoID uint, ch chan ProcessingEvent) {
	sseMu.Lock()
	if set, ok := sseSubs[videoID]; ok {
		delete(set, ch)
		if len(set) == 0 {
			delete(sseSubs, videoID)
		}
	}
	sseMu.Unlock()
	close(ch)
}
