package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// NotificationService handles all push notification logic
type NotificationService struct{}

// NewNotificationService creates a new notification service instance
func NewNotificationService() *NotificationService {
	return &NotificationService{}
}

// NotificationData represents the data payload for notifications
type NotificationData struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	PropertyID string `json:"propertyId,omitempty"`
	UserID     string `json:"userId,omitempty"`
	HostID     string `json:"hostId,omitempty"`
	// Deep linking data
	Screen string `json:"screen"`           // Target screen to navigate to
	Params string `json:"params"`           // JSON string of navigation parameters
	Action string `json:"action,omitempty"` // Specific action to perform
}

// getUserPushTokens retrieves all push tokens for a user
func (ns *NotificationService) getUserPushTokens(userID uint) ([]string, error) {
	log.Printf("📱 TOKENS DEBUG: Getting push tokens for user %d", userID)

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		log.Printf("❌ TOKENS ERROR: User %d not found: %v", userID, err)
		return nil, fmt.Errorf("user not found: %v", err)
	}

	log.Printf("📱 TOKENS DEBUG: User %d found - AllowsNotifications: %v, HasTokens: %v",
		userID, user.AllowsNotifications != nil && *user.AllowsNotifications, user.PushTokens != nil)

	if user.AllowsNotifications == nil || !*user.AllowsNotifications || user.PushTokens == nil {
		log.Printf("❌ TOKENS ERROR: User %d has notifications disabled or no tokens", userID)
		return nil, fmt.Errorf("user has notifications disabled or no tokens")
	}

	var tokens []string
	if err := json.Unmarshal(user.PushTokens, &tokens); err != nil {
		log.Printf("❌ TOKENS ERROR: Failed to unmarshal push tokens for user %d: %v", userID, err)
		return nil, fmt.Errorf("failed to unmarshal push tokens: %v", err)
	}

	log.Printf("✅ TOKENS SUCCESS: Found %d push tokens for user %d", len(tokens), userID)
	return tokens, nil
}

// SendNotificationToUser sends a notification to a specific user
func (ns *NotificationService) SendNotificationToUser(userID uint, title, body string, data NotificationData) error {
	tokens, err := ns.getUserPushTokens(userID)
	if err != nil {
		log.Printf("Failed to get push tokens for user %d: %v", userID, err)
		return err
	}

	dataMap := map[string]string{
		"type":       data.Type,
		"id":         data.ID,
		"propertyId": data.PropertyID,
		"userId":     data.UserID,
		"hostId":     data.HostID,
	}

	var lastError error
	for _, token := range tokens {
		if err := utils.SendNotification(token, title, body, dataMap); err != nil {
			log.Printf("Failed to send notification to token %s: %v", token, err)
			lastError = err
		}
	}

	return lastError
}

// SendReservationNotificationToHost sends notification when a reservation is made
func (ns *NotificationService) SendReservationNotificationToHost(reservationID, propertyID, hostID, guestID uint, guestName, propertyTitle string) error {
	log.Printf("📱 NOTIFICATION DEBUG: Attempting to send reservation notification to host %d", hostID)
	log.Printf("📱 NOTIFICATION DEBUG: Reservation ID: %d, Property: %s, Guest: %s", reservationID, propertyTitle, guestName)

	title := "🏠 Nouvelle Réservation!"
	body := fmt.Sprintf("%s a fait une réservation pour %s", guestName, propertyTitle)

	// Create navigation parameters for deep linking
	params := fmt.Sprintf(`{"reservationId": %d, "propertyId": %d, "guestId": %d}`, reservationID, propertyID, guestID)

	data := NotificationData{
		Type:       "reservation_created",
		ID:         fmt.Sprintf("%d", reservationID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", guestID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "HostReservations",
		Params:     params,
		Action:     "view_reservation",
	}

	err := ns.SendNotificationToUser(hostID, title, body, data)
	if err != nil {
		log.Printf("❌ NOTIFICATION ERROR: Failed to send reservation notification: %v", err)
	} else {
		log.Printf("✅ NOTIFICATION SUCCESS: Reservation notification sent to host %d", hostID)
	}
	return err
}

// SendMessageNotificationToHost sends notification when a message is received
func (ns *NotificationService) SendMessageNotificationToHost(hostID, senderID uint, senderName, propertyTitle string) error {
	title := "💬 Nouveau Message"
	body := fmt.Sprintf("%s vous a envoyé un message concernant %s", senderName, propertyTitle)

	// Create navigation parameters for deep linking to messages
	params := fmt.Sprintf(`{"senderId": %d, "senderName": "%s"}`, senderID, senderName)

	data := NotificationData{
		Type:   "message_received",
		UserID: fmt.Sprintf("%d", senderID),
		HostID: fmt.Sprintf("%d", hostID),
		Screen: "Messages",
		Params: params,
		Action: "view_conversation",
	}

	return ns.SendNotificationToUser(hostID, title, body, data)
}

// SendVideoInteractionNotificationToHost sends notification when video is liked/commented
func (ns *NotificationService) SendVideoInteractionNotificationToHost(hostID, userID uint, userName, interactionType, videoTitle string) error {
	var title, body string

	switch interactionType {
	case "like":
		title = "❤️ Votre Vidéo a été Aimée!"
		body = fmt.Sprintf("%s a aimé votre vidéo: %s", userName, videoTitle)
	case "comment":
		title = "💬 Nouveau Commentaire!"
		body = fmt.Sprintf("%s a commenté votre vidéo: %s", userName, videoTitle)
	default:
		title = "📹 Interaction Vidéo"
		body = fmt.Sprintf("%s a interagi avec votre vidéo: %s", userName, videoTitle)
	}

	// Create navigation parameters for deep linking to videos
	params := fmt.Sprintf(`{"userId": %d, "userName": "%s", "interactionType": "%s"}`, userID, userName, interactionType)

	data := NotificationData{
		Type:   fmt.Sprintf("video_%s", interactionType),
		UserID: fmt.Sprintf("%d", userID),
		HostID: fmt.Sprintf("%d", hostID),
		Screen: "VideoFeed",
		Params: params,
		Action: "view_video",
	}

	return ns.SendNotificationToUser(hostID, title, body, data)
}

// SendExperienceBookingNotificationToHost sends notification when experience is booked
func (ns *NotificationService) SendExperienceBookingNotificationToHost(experienceID, hostID, guestID uint, guestName, experienceTitle string) error {
	title := "🎯 Nouvelle Réservation d'Expérience!"
	body := fmt.Sprintf("%s a réservé votre expérience: %s", guestName, experienceTitle)

	// Create navigation parameters for deep linking to experiences
	params := fmt.Sprintf(`{"experienceId": %d, "guestId": %d, "guestName": "%s"}`, experienceID, guestID, guestName)

	data := NotificationData{
		Type:   "experience_booked",
		ID:     fmt.Sprintf("%d", experienceID),
		UserID: fmt.Sprintf("%d", guestID),
		HostID: fmt.Sprintf("%d", hostID),
		Screen: "ExperienceBookings",
		Params: params,
		Action: "view_booking",
	}

	return ns.SendNotificationToUser(hostID, title, body, data)
}

// SendPropertyStatusNotificationToHost sends notification when property status changes
func (ns *NotificationService) SendPropertyStatusNotificationToHost(propertyID, hostID uint, propertyTitle, status string) error {
	var title, body string

	switch status {
	case "approved":
		title = "✅ Propriété Approuvée!"
		body = fmt.Sprintf("Félicitations! Votre propriété '%s' a été approuvée et est maintenant visible.", propertyTitle)
	case "rejected":
		title = "❌ Propriété Rejetée"
		body = fmt.Sprintf("Votre propriété '%s' a été rejetée. Veuillez vérifier les détails et soumettre à nouveau.", propertyTitle)
	case "under_review":
		title = "🔍 Propriété en Révision"
		body = fmt.Sprintf("Votre propriété '%s' est en cours de révision par nos équipes.", propertyTitle)
	default:
		title = "🏠 Mise à Jour de Propriété"
		body = fmt.Sprintf("Le statut de votre propriété '%s' a été mis à jour: %s", propertyTitle, status)
	}

	// Create navigation parameters for deep linking to property details
	params := fmt.Sprintf(`{"propertyId": %d, "status": "%s"}`, propertyID, status)

	data := NotificationData{
		Type:       "property_status_changed",
		ID:         fmt.Sprintf("%d", propertyID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "MyProperties",
		Params:     params,
		Action:     "view_property",
	}

	return ns.SendNotificationToUser(hostID, title, body, data)
}

// SendReservationAcceptanceNotificationToGuest sends notification when reservation is accepted
func (ns *NotificationService) SendReservationAcceptanceNotificationToGuest(reservationID, propertyID, guestID, hostID uint, hostName, propertyTitle string) error {
	title := "🎉 Réservation Acceptée!"
	body := fmt.Sprintf("%s a accepté votre réservation pour %s", hostName, propertyTitle)

	// Create navigation parameters for deep linking to guest reservations
	params := fmt.Sprintf(`{"reservationId": %d, "propertyId": %d, "hostId": %d}`, reservationID, propertyID, hostID)

	data := NotificationData{
		Type:       "reservation_accepted",
		ID:         fmt.Sprintf("%d", reservationID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", guestID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "MyReservations",
		Params:     params,
		Action:     "view_reservation",
	}

	return ns.SendNotificationToUser(guestID, title, body, data)
}

// SendReservationRejectionNotificationToGuest sends notification when reservation is rejected
func (ns *NotificationService) SendReservationRejectionNotificationToGuest(reservationID, propertyID, guestID, hostID uint, hostName, propertyTitle string) error {
	title := "😔 Réservation Refusée"
	body := fmt.Sprintf("%s a refusé votre réservation pour %s", hostName, propertyTitle)

	// Create navigation parameters for deep linking to guest reservations
	params := fmt.Sprintf(`{"reservationId": %d, "propertyId": %d, "hostId": %d}`, reservationID, propertyID, hostID)

	data := NotificationData{
		Type:       "reservation_rejected",
		ID:         fmt.Sprintf("%d", reservationID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", guestID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "MyReservations",
		Params:     params,
		Action:     "view_reservation",
	}

	return ns.SendNotificationToUser(guestID, title, body, data)
}

// SendWelcomeNotificationToNewUser sends welcome notification to new users
func (ns *NotificationService) SendWelcomeNotificationToNewUser(userID uint, firstName string) error {
	title := "🎉 Bienvenue sur habitat!"
	body := fmt.Sprintf("Bonjour %s! Découvrez des logements incroyables en Mauritanie.", firstName)

	data := NotificationData{
		Type:   "welcome",
		UserID: fmt.Sprintf("%d", userID),
	}

	// Wait a bit to ensure push token is registered
	time.Sleep(2 * time.Second)
	return ns.SendNotificationToUser(userID, title, body, data)
}

// SendReminderNotificationToGuest sends reminder notifications for upcoming reservations
func (ns *NotificationService) SendReminderNotificationToGuest(reservationID, propertyID, guestID uint, propertyTitle string, daysUntil int) error {
	var title, body string

	if daysUntil == 1 {
		title = "⏰ Rappel: Réservation Demain!"
		body = fmt.Sprintf("N'oubliez pas votre séjour à %s demain!", propertyTitle)
	} else {
		title = "📅 Rappel de Réservation"
		body = fmt.Sprintf("Votre séjour à %s commence dans %d jours!", propertyTitle, daysUntil)
	}

	data := NotificationData{
		Type:       "reservation_reminder",
		ID:         fmt.Sprintf("%d", reservationID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", guestID),
	}

	return ns.SendNotificationToUser(guestID, title, body, data)
}

// Global notification service instance
var NotificationServiceInstance = NewNotificationService()
