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
	// iOS-specific fields
	Badge    *int   `json:"badge,omitempty"`
	// Android-specific fields
	ChannelID string `json:"channelId,omitempty"`
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
	bodyStr := string(rb)
	log.Printf("🔔 Expo push response: status=%d body=%s", resp.StatusCode, bodyStr)
	
	// Parse response to check for errors
	var parsed struct {
		Data []struct {
			Status  string `json:"status"`
			ID      string `json:"id,omitempty"`
			Details struct {
				Error     string `json:"error"`
				ExpoToken string `json:"expoPushToken"`
				Reason    string `json:"reason,omitempty"`
			} `json:"details,omitempty"`
		} `json:"data"`
	}
	
	if err := json.Unmarshal(rb, &parsed); err != nil {
		log.Printf("⚠️ Failed to parse Expo response: %v", err)
		return nil
	}
	
	// Check each response and log/remove invalid tokens
	for i, it := range parsed.Data {
		token := it.Details.ExpoToken
		if token == "" && len(tokens) > i {
			token = tokens[i]
		}
		
		if it.Status != "ok" {
			log.Printf("⚠️ Push failed for token %d: status=%s error=%s reason=%s", i+1, it.Status, it.Details.Error, it.Details.Reason)
			
			// Remove invalid tokens
			if strings.EqualFold(it.Details.Error, "DeviceNotRegistered") || 
			   strings.EqualFold(it.Details.Error, "InvalidCredentials") ||
			   strings.Contains(strings.ToLower(it.Status), "error") {
				if token != "" {
					tokenPreview := token
					if len(token) > 30 {
						tokenPreview = token[:30] + "..."
					}
					log.Printf("🗑️ Removing invalid token: %s", tokenPreview)
					RemoveTokenGlobally(token)
				}
			}
		} else {
			log.Printf("✅ Push sent successfully for token %d: id=%s", i+1, it.ID)
		}
	}
	
	if resp.StatusCode != 200 {
		log.Printf("⚠️ Expo API returned non-200 status: %d", resp.StatusCode)
	}
	
	return nil
}
