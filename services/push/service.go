package push

import (
	"log"
	"strings"
)

// isExpoToken checks if a token is an Expo push token
func isExpoToken(token string) bool {
	trimmed := strings.TrimSpace(token)
	return strings.HasPrefix(trimmed, "ExponentPushToken") || strings.HasPrefix(trimmed, "ExpoPushToken")
}

// SendPush sends push notifications using FCM if available, falls back to Expo
func SendPush(tokens []string, title, body string) error {
	return SendPushWithImage(tokens, title, body, "")
}

// SendPushWithImage sends push notifications with optional image/avatar URL
// imageURL: URL to sender's avatar/image (shown instead of app icon)
// Automatically routes Expo tokens to Expo Push service and FCM/APNs tokens to FCM
func SendPushWithImage(tokens []string, title, body, imageURL string) error {
	if len(tokens) == 0 {
		return nil
	}

	// Separate tokens by type
	expoTokens := make([]string, 0)
	fcmTokens := make([]string, 0)

	for _, token := range tokens {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			continue
		}

		if isExpoToken(trimmed) {
			expoTokens = append(expoTokens, trimmed)
		} else {
			// FCM tokens, APNs tokens, or other native tokens
			fcmTokens = append(fcmTokens, trimmed)
		}
	}

	var expoErr, fcmErr error

	// Send Expo tokens via Expo Push service
	if len(expoTokens) > 0 {
		log.Printf("🔔 Sending %d Expo token(s) via Expo Push (with image: %v)", len(expoTokens), imageURL != "")
		expoErr = SendExpoPushWithImage(expoTokens, title, body, imageURL)
		if expoErr != nil {
			log.Printf("⚠️ Expo Push error: %v", expoErr)
		}
	}

	// Send FCM/APNs tokens via FCM if available
	if len(fcmTokens) > 0 {
		if fcmInitialized && fcmClient != nil {
			log.Printf("🔥 Sending %d FCM/APNs token(s) via FCM (with image: %v)", len(fcmTokens), imageURL != "")
			fcmErr = SendFCMPushWithImage(fcmTokens, title, body, imageURL)
			if fcmErr != nil {
				log.Printf("⚠️ FCM send error: %v", fcmErr)
			}
		} else {
			log.Printf("⚠️ FCM not available, cannot send %d FCM/APNs token(s)", len(fcmTokens))
			fcmErr = nil // Not an error, just not available
		}
	}

	// Return error if either failed
	if expoErr != nil {
		return expoErr
	}
	return fcmErr
}
