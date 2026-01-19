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
	To       string                 `json:"to"`
	Sound    string                 `json:"sound,omitempty"`
	Title    string                 `json:"title,omitempty"`
	Body     string                 `json:"body,omitempty"`
	Priority string                 `json:"priority,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"` // Custom data including image/avatar URL
	Image    string                 `json:"image,omitempty"` // Native image support for iOS/Android (Expo Push API v2)
	// iOS-specific fields
	Badge *int `json:"badge,omitempty"`
	// Android-specific fields
	ChannelID string `json:"channelId,omitempty"`
}

func SendExpoPush(tokens []string, title, body string) error {
	return SendExpoPushWithImage(tokens, title, body, "", nil)
}

// SendExpoPushWithImage sends Expo push notifications with optional image/avatar URL
// imageURL: URL to sender's avatar (shown instead of app icon)
// data: Optional custom data map for deep linking (can be nil)
func SendExpoPushWithImage(tokens []string, title, body, imageURL string, data map[string]string) error {
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
	log.Printf("🔔 Sending Expo push to %d tokens [%s] (with image: %v)", len(tokens), strings.Join(preview, ", "), imageURL != "")
	var payload []ExpoMessage
	for _, t := range tokens {
		msg := ExpoMessage{
			To:        t,
			Sound:     "default",
			Title:     title,
			Body:      body,
			Priority:  "high",
			ChannelID: "default",
		}
		// Add image URL - Native Expo Push API v2 image support for iOS/Android
		// The image field displays the image directly in the notification
		msg.Data = make(map[string]interface{})
		
		// Merge custom data if provided
		if data != nil {
			for k, v := range data {
				msg.Data[k] = v
			}
		}
		
		if imageURL != "" {
			msg.Image = imageURL // Native image support - displays in notification on iOS/Android
			// Also include image URL in data for client-side handling
			msg.Data["imageURL"] = imageURL
			msg.Data["avatarURL"] = imageURL // Both keys for compatibility
		}
		
		// Set default type if not provided
		if _, exists := msg.Data["type"]; !exists {
			msg.Data["type"] = "property"
		}
		payload = append(payload, msg)
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
