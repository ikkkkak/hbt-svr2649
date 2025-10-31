package push

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type ExpoMessage struct {
	To       string `json:"to"`
	Sound    string `json:"sound,omitempty"`
	Title    string `json:"title,omitempty"`
	Body     string `json:"body,omitempty"`
	Priority string `json:"priority,omitempty"`
}

func SendExpoPush(tokens []string, title, body string) error {
	if len(tokens) == 0 {
		return nil
	}
	// Log before sending for visibility (make a COPY to avoid mutating original tokens)
	preview := make([]string, len(tokens))
	copy(preview, tokens)
	if len(preview) > 3 {
		preview = preview[:3]
	}
	for i, t := range preview {
		if len(t) > 16 {
			preview[i] = t[:16] + "…"
		}
	}
	log.Printf("🔔 Sending Expo push to %d tokens [%s]", len(tokens), strings.Join(preview, ", "))
	var payload []ExpoMessage
	for _, t := range tokens {
		payload = append(payload, ExpoMessage{To: t, Sound: "default", Title: title, Body: body, Priority: "high"})
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://exp.host/--/api/v2/push/send", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := ioutil.ReadAll(resp.Body)
	log.Printf("🔔 Expo push response: status=%d body=%s", resp.StatusCode, string(rb))
	// prune invalid tokens
	var parsed struct {
		Data []struct {
			Status  string `json:"status"`
			Details struct {
				Error     string `json:"error"`
				ExpoToken string `json:"expoPushToken"`
			} `json:"details"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rb, &parsed)
	for _, it := range parsed.Data {
		if strings.EqualFold(it.Details.Error, "DeviceNotRegistered") || strings.Contains(strings.ToLower(it.Status), "error") {
			token := it.Details.ExpoToken
			if token == "" && len(tokens) == 1 {
				token = tokens[0]
			}
			if token != "" {
				RemoveTokenGlobally(token)
			}
		}
	}
	return nil
}
