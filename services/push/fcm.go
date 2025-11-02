package push

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

var fcmClient *messaging.Client
var fcmInitialized bool

// InitializeFCM initializes Firebase Cloud Messaging client
// Requires GOOGLE_APPLICATION_CREDENTIALS environment variable pointing to service account JSON
// OR FCM_CREDENTIALS_PATH pointing to the service account JSON file
func InitializeFCM() error {
	if fcmInitialized {
		return nil
	}

	ctx := context.Background()

	// Try to get credentials path from environment
	credsPath := os.Getenv("FCM_CREDENTIALS_PATH")
	if credsPath == "" {
		credsPath = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	}
	if credsPath == "" {
		// Try default locations (skip google-services.json as it's for client-side use only)
		possiblePaths := []string{
			"./service-account.json",
			"./fcm-credentials.json",
			"../service-account.json",
		}
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				credsPath = path
				break
			}
		}
	}

	var app *firebase.App
	var err error

	if credsPath != "" {
		// Validate that we're not using google-services.json (client config)
		if strings.Contains(credsPath, "google-services.json") {
			log.Printf("⚠️ google-services.json is for client-side use only, not for FCM Admin SDK")
			log.Printf("⚠️ Please download service-account.json from Firebase Console")
			log.Printf("⚠️ Go to: Firebase Console → Project Settings → Service Accounts → Generate new private key")
			return fmt.Errorf("google-services.json cannot be used for FCM Admin SDK - need service-account.json")
		}

		log.Printf("🔥 Initializing FCM with credentials from: %s", credsPath)
		opt := option.WithCredentialsFile(credsPath)
		app, err = firebase.NewApp(ctx, nil, opt)
		if err != nil {
			log.Printf("⚠️ Failed to initialize FCM with credentials file: %v", err)
			log.Printf("⚠️ FCM will be disabled. Make sure the file is a valid service-account.json from Firebase Console")
			return err
		}
	} else {
		log.Printf("⚠️ No FCM credentials found.")
		log.Printf("⚠️ To enable FCM, download service-account.json from Firebase Console:")
		log.Printf("⚠️   1. Go to Firebase Console → Project Settings → Service Accounts")
		log.Printf("⚠️   2. Click 'Generate new private key'")
		log.Printf("⚠️   3. Save as 'service-account.json' in server root")
		log.Printf("⚠️   4. Or set FCM_CREDENTIALS_PATH environment variable")
		// Try with default credentials (GCP service account)
		app, err = firebase.NewApp(ctx, nil)
		if err != nil {
			log.Printf("⚠️ Failed to initialize FCM: %v", err)
			log.Printf("⚠️ FCM will be disabled. Push notifications will use Expo Push fallback.")
			return err
		}
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		log.Printf("⚠️ Failed to get FCM messaging client: %v", err)
		return err
	}

	fcmClient = client
	fcmInitialized = true
	log.Printf("✅ FCM initialized successfully")
	return nil
}

// SendFCMPush sends push notification via FCM
func SendFCMPush(tokens []string, title, body string) error {
	if !fcmInitialized || fcmClient == nil {
		log.Printf("⚠️ FCM not initialized, cannot send push notifications")
		return nil
	}

	if len(tokens) == 0 {
		return nil
	}

	// Filter out invalid/empty tokens
	validTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		trimmed := strings.TrimSpace(token)
		// Accept multiple token formats:
		// - Expo tokens: "ExponentPushToken[...]" (~41 chars)
		// - FCM tokens: 152+ character strings (Android full FCM registration tokens)
		// - APNs tokens: 64 hex character strings (iOS device tokens)
		if trimmed != "" && len(trimmed) >= 20 {
			// Validate token format
			isExpoToken := strings.HasPrefix(trimmed, "ExponentPushToken") || strings.HasPrefix(trimmed, "ExpoPushToken")
			isAPNsToken := len(trimmed) == 64 && !isExpoToken && isHexString(trimmed)
			isFCMToken := len(trimmed) > 50 && !isExpoToken

			if isExpoToken || isAPNsToken || isFCMToken {
				validTokens = append(validTokens, trimmed)
			} else {
				log.Printf("⚠️ Skipping invalid token format (length=%d): %s...", len(trimmed), trimmed[:min(20, len(trimmed))])
			}
		}
	}

	if len(validTokens) == 0 {
		log.Printf("⚠️ No valid FCM tokens to send to")
		return nil
	}

	// Log preview
	preview := make([]string, len(validTokens))
	copy(preview, validTokens)
	if len(preview) > 3 {
		preview = preview[:3]
	}
	for i, t := range preview {
		if len(t) > 30 {
			preview[i] = t[:30] + "..."
		}
	}
	log.Printf("🔥 Sending FCM push to %d tokens [%s]", len(validTokens), strings.Join(preview, ", "))

	ctx := context.Background()

	// FCM message
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Sound:        "default",
				ChannelID:    "default",
				Priority:     messaging.PriorityMax,
				DefaultSound: true,
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority": "10", // High priority for notifications
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound:            "default",
					Badge:            nil, // Set badge count if needed
					ContentAvailable: false,
				},
			},
		},
		Tokens: validTokens,
	}

	// Send the message
	br, err := fcmClient.SendMulticast(ctx, message)
	if err != nil {
		log.Printf("⚠️ FCM send error: %v", err)
		// Check if it's a token error - we might need to handle invalid tokens
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "not found") {
			// Try sending individually to identify bad tokens
			log.Printf("🔄 Attempting individual sends to identify invalid tokens...")
			return sendIndividualFCMPush(ctx, validTokens, title, body)
		}
		return err
	}

	// Log success/failure counts
	log.Printf("✅ FCM sent: Success=%d, Failure=%d", br.SuccessCount, br.FailureCount)

	// Handle failures - remove invalid tokens
	if br.FailureCount > 0 {
		log.Printf("⚠️ %d tokens failed, checking responses...", br.FailureCount)
		for i, resp := range br.Responses {
			if !resp.Success {
				token := validTokens[i]
				log.Printf("⚠️ Token failed: error=%v", resp.Error)

				// Remove invalid tokens
				if resp.Error != nil {
					errMsg := resp.Error.Error()
					if strings.Contains(strings.ToLower(errMsg), "invalid") ||
						strings.Contains(strings.ToLower(errMsg), "not found") ||
						strings.Contains(strings.ToLower(errMsg), "unregistered") {
						log.Printf("🗑️ Removing invalid FCM token: %s...", token[:min(30, len(token))])
						RemoveTokenGlobally(token)
					}
				}
			}
		}
	}

	return nil
}

// sendIndividualFCMPush sends FCM messages individually (used when batch fails)
func sendIndividualFCMPush(ctx context.Context, tokens []string, title, body string) error {
	for _, token := range tokens {
		message := &messaging.Message{
			Notification: &messaging.Notification{
				Title: title,
				Body:  body,
			},
			Android: &messaging.AndroidConfig{
				Priority: "high",
				Notification: &messaging.AndroidNotification{
					Sound:        "default",
					ChannelID:    "default",
					Priority:     messaging.PriorityMax,
					DefaultSound: true,
				},
			},
			APNS: &messaging.APNSConfig{
				Headers: map[string]string{
					"apns-priority": "10", // High priority for notifications
				},
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Sound:            "default",
						ContentAvailable: false,
					},
				},
			},
			Token: token,
		}

		_, err := fcmClient.Send(ctx, message)
		if err != nil {
			log.Printf("⚠️ Failed to send to token %s...: %v", token[:min(30, len(token))], err)
			// Remove invalid token
			if strings.Contains(strings.ToLower(err.Error()), "invalid") ||
				strings.Contains(strings.ToLower(err.Error()), "not found") ||
				strings.Contains(strings.ToLower(err.Error()), "unregistered") {
				RemoveTokenGlobally(token)
			}
		} else {
			log.Printf("✅ Sent to token %s...", token[:min(30, len(token))])
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
