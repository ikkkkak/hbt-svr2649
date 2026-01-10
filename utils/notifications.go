package utils

import (
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
<<<<<<< HEAD
=======

// SendRichNotification sends a notification with an image attachment
// Image URL should be publicly accessible
// Note: Expo Go SDK doesn't support Images field directly, so we include it in the data payload
// The client app can extract and display the image from the notification data
func SendRichNotification(pushToken string, title string, body string, imageURL string, data map[string]string) error {
	token, err := expo.NewExponentPushToken(pushToken)
	if err != nil {
		return err
	}

	// Include image URL in data payload for client-side handling
	// The React Native app can extract this and display it as a rich notification
	// Use "imageUrl" key to match existing pattern in the codebase
	if imageURL != "" {
		data["imageUrl"] = imageURL
	}

	// Create push message
	message := &expo.PushMessage{
		To:       []expo.ExponentPushToken{token},
		Body:     body,
		Sound:    "default",
		Title:    title,
		Priority: expo.DefaultPriority,
		Data:     data,
	}

	response, pushErr := pushClient.Publish(message)

	if pushErr != nil {
		return pushErr
	}

	if response.ValidateResponse() != nil {
		fmt.Println(response.PushMessage.To, "failed")
		return errors.New("Failed to send message")
	}

	return nil
}
>>>>>>> 4698d88 (AFTER ADDING NOTIFICATION PROEPRTIES TO USERS)
