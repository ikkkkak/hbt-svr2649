package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"apartments-clone-server/MeskenyGPT/internal/ai"
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

// RegisterRoutes mounts MeskenyGPT HTTP endpoints on the given ServeMux.
// NOTE: currently unused inside apartmentscloneserver; kept for future
// standalone MeskenyGPT deployment. It does not introduce any dependency.
func RegisterRoutes(mux *http.ServeMux, svc ai.Service) {
	mux.HandleFunc("/ai/chat", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Message   string `json:"message"`
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		out, err := svc.HandleChatTurn(context.Background(), ai.ChatInput{
			Text:      body.Message,
			SessionID: body.SessionID,
		})
		if err != nil {
			http.Error(w, "ai error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/ai/greeting", func(w http.ResponseWriter, req *http.Request) {
		// For now, default to French; later we can inspect headers.
		out, _ := svc.GetGreeting(context.Background(), lang.LangFR)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

