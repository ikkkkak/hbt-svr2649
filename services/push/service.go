package push

import (
	"log"
)

// SendPush sends push notifications using FCM if available, falls back to Expo
func SendPush(tokens []string, title, body string) error {
	if fcmInitialized && fcmClient != nil {
		// Use FCM if initialized
		log.Printf("🔥 Using FCM for push notifications")
		return SendFCMPush(tokens, title, body)
	}
	
	// Fallback to Expo if FCM not available
	log.Printf("🔔 FCM not available, falling back to Expo Push")
	return SendExpoPush(tokens, title, body)
}

