package push

import (
	"log"
)

// SendPush sends push notifications using FCM if available, falls back to Expo
func SendPush(tokens []string, title, body string) error {
	return SendPushWithImage(tokens, title, body, "")
}

// SendPushWithImage sends push notifications with optional image/avatar URL
// imageURL: URL to sender's avatar/image (shown instead of app icon)
func SendPushWithImage(tokens []string, title, body, imageURL string) error {
	if fcmInitialized && fcmClient != nil {
		// Use FCM if initialized
		log.Printf("🔥 Using FCM for push notifications (with image: %v)", imageURL != "")
		return SendFCMPushWithImage(tokens, title, body, imageURL)
	}

	// Fallback to Expo if FCM not available
	log.Printf("🔔 FCM not available, falling back to Expo Push (with image: %v)", imageURL != "")
	return SendExpoPushWithImage(tokens, title, body, imageURL)
}
