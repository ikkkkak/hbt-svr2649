package utils

import (
	"apartments-clone-server/services/push"
	"errors"
	"fmt"

	expo "github.com/oliveroneill/exponent-server-sdk-golang/sdk"
)

var pushClient *expo.PushClient = expo.NewPushClient(nil)

func SendNotification(pushToken string, title string, body string, data map[string]string) error {
	token, err := expo.NewExponentPushToken(pushToken)
	if err != nil {
		return err
	}

	response, pushErr := pushClient.Publish(
		&expo.PushMessage{
			To:       []expo.ExponentPushToken{token},
			Body:     body,
			Sound:    "default",
			Title:    title,
			Priority: expo.DefaultPriority,
			Data:     data,
		},
	)

	if pushErr != nil {
		return pushErr
	}

	if response.ValidateResponse() != nil {
		fmt.Println(response.PushMessage.To, "failed")
		return errors.New("Failed to send message")
	}

	return nil
}

// SendRichNotification sends a notification with an image attachment
// Uses Expo Push API v2 native image support for iOS/Android
// Image URL should be publicly accessible and will be displayed directly in the notification
func SendRichNotification(pushToken string, title string, body string, imageURL string, data map[string]string) error {
	// Use the professional push service which supports native Expo Push API v2 image support
	// This ensures images are displayed directly in iOS/Android notifications
	tokens := []string{pushToken}
	
	// Use the push service which has native image support for Expo/EAS
	// Pass the data map for deep linking support
	return push.SendPushWithImage(tokens, title, body, imageURL, data)
}