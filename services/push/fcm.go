package push

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

var fcmClient *messaging.Client
var fcmInitialized bool

// InitializeFCM initializes Firebase Cloud Messaging client
// Supports multiple credential sources:
// 1. FCM_CREDENTIALS_JSON - JSON content as environment variable (BEST for Render/production)
// 2. FCM_CREDENTIALS_PATH - Path to service account JSON file
// 3. GOOGLE_APPLICATION_CREDENTIALS - Standard Google Cloud credentials path
// 4. Default file locations - ./service-account.json, etc.
func InitializeFCM() error {
	if fcmInitialized {
		return nil
	}

	ctx := context.Background()

	var app *firebase.App
	var err error

	// Option 1: Check for FCM_CREDENTIALS_JSON (JSON content as environment variable) - BEST for Render
	credsJSON := os.Getenv("FCM_CREDENTIALS_JSON")
	if credsJSON != "" {
		log.Printf("🔥 Initializing FCM with credentials from FCM_CREDENTIALS_JSON environment variable")

		// Parse JSON to validate format
		var jsonData map[string]interface{}
		if err := json.Unmarshal([]byte(credsJSON), &jsonData); err != nil {
			log.Printf("⚠️ FCM_CREDENTIALS_JSON is not valid JSON: %v", err)
			log.Printf("⚠️ FCM will be disabled. Make sure FCM_CREDENTIALS_JSON contains valid service-account.json content")
			return fmt.Errorf("invalid JSON in FCM_CREDENTIALS_JSON: %v", err)
		}

		// Create a temporary file with the credentials
		tmpFile, err := os.CreateTemp("", "fcm-credentials-*.json")
		if err != nil {
			log.Printf("⚠️ Failed to create temporary file for FCM credentials: %v", err)
			return fmt.Errorf("failed to create temp file: %v", err)
		}

		// Write JSON to temporary file
		if _, err := tmpFile.Write([]byte(credsJSON)); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			log.Printf("⚠️ Failed to write FCM credentials to temp file: %v", err)
			return fmt.Errorf("failed to write temp file: %v", err)
		}
		tmpFile.Close()

		// Use temporary file for credentials
		log.Printf("🔥 Using temporary file: %s", tmpFile.Name())
		opt := option.WithCredentialsFile(tmpFile.Name())
		app, err = firebase.NewApp(ctx, nil, opt)

		// Clean up temp file after initialization (with small delay to ensure it's read)
		go func() {
			time.Sleep(2 * time.Second)
			os.Remove(tmpFile.Name())
			log.Printf("🧹 Cleaned up temporary FCM credentials file")
		}()

		if err != nil {
			log.Printf("⚠️ Failed to initialize FCM with FCM_CREDENTIALS_JSON: %v", err)
			log.Printf("⚠️ FCM will be disabled. Make sure the JSON content is valid service-account.json")
			return err
		}
	} else {
		// Option 2: Try to get credentials path from environment
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
			log.Printf("⚠️ To enable FCM, use one of these options:")
			log.Printf("⚠️   1. (BEST for Render) Set FCM_CREDENTIALS_JSON environment variable with JSON content")
			log.Printf("⚠️   2. Set FCM_CREDENTIALS_PATH to point to service-account.json file")
			log.Printf("⚠️   3. Place service-account.json in server root directory")
			log.Printf("⚠️   Download from: Firebase Console → Project Settings → Service Accounts → Generate new private key")
			// Try with default credentials (GCP service account)
			app, err = firebase.NewApp(ctx, nil)
			if err != nil {
				log.Printf("⚠️ Failed to initialize FCM: %v", err)
				log.Printf("⚠️ FCM will be disabled. Push notifications will use Expo Push fallback.")
				return err
			}
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

// SendFCMPush sends push notification via FCM (without image)
func SendFCMPush(tokens []string, title, body string) error {
	return SendFCMPushWithImage(tokens, title, body, "")
}

// SendFCMPushWithImage sends push notification via FCM with optional image/avatar URL
// imageURL: URL to sender's avatar (shown instead of app icon, positioned below)
func SendFCMPushWithImage(tokens []string, title, body, imageURL string) error {
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

	// FCM message with image support
	message := &messaging.MulticastMessage{
		Notification: &messaging.Notification{
			Title:    title,
			Body:     body,
			ImageURL: imageURL, // Sender avatar URL (replaces app icon)
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Sound:        "default",
				ChannelID:    "default",
				Priority:     messaging.PriorityMax,
				DefaultSound: true,
				ImageURL:     imageURL, // Android: Large icon (avatar)
				// Icon appears below notification title on Android
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority": "10", // High priority for notifications
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound:            "default",
					Badge:            nil,
					ContentAvailable: false,
				},
				// Custom data for iOS to display avatar
				CustomData: map[string]interface{}{
					"imageURL":  imageURL,
					"avatarURL": imageURL, // iOS custom key for avatar
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
			return sendIndividualFCMPushWithImage(ctx, validTokens, title, body, imageURL)
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
	return sendIndividualFCMPushWithImage(ctx, tokens, title, body, "")
}

// sendIndividualFCMPushWithImage sends FCM messages individually with image
func sendIndividualFCMPushWithImage(ctx context.Context, tokens []string, title, body, imageURL string) error {
	for _, token := range tokens {
		message := &messaging.Message{
			Notification: &messaging.Notification{
				Title:    title,
				Body:     body,
				ImageURL: imageURL, // Sender avatar URL
			},
			Android: &messaging.AndroidConfig{
				Priority: "high",
				Notification: &messaging.AndroidNotification{
					Sound:        "default",
					ChannelID:    "default",
					Priority:     messaging.PriorityMax,
					DefaultSound: true,
					ImageURL:     imageURL, // Android: Large icon (avatar)
				},
			},
			APNS: &messaging.APNSConfig{
				Headers: map[string]string{
					"apns-priority": "10",
				},
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Sound:            "default",
						ContentAvailable: false,
					},
					CustomData: map[string]interface{}{
						"imageURL":  imageURL,
						"avatarURL": imageURL,
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
