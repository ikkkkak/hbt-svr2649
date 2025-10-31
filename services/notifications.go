package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
		// Extract the actual Expo token (before the | separator if present)
		expoToken := token
		if strings.Contains(token, "|") {
			expoToken = strings.Split(token, "|")[0]
			log.Printf("📱 TOKEN EXTRACTED: Full token: %s, Expo token: %s", token, expoToken)
		}

		if err := utils.SendNotification(expoToken, title, body, dataMap); err != nil {
			log.Printf("Failed to send notification to token %s: %v", expoToken, err)
			lastError = err
		}
	}

	return lastError
}

// SendReservationNotificationToHost sends notification when a reservation is made
func (ns *NotificationService) SendReservationNotificationToHost(reservationID, propertyID, hostID, guestID uint, guestName, propertyTitle string) error {
	log.Printf("📱 NOTIFICATION DEBUG: Attempting to send reservation notification to host %d", hostID)
	log.Printf("📱 NOTIFICATION DEBUG: Reservation ID: %d, Property: %s, Guest: %s", reservationID, propertyTitle, guestName)

	title := "🏠 طلب حجز جديد!"
	body := fmt.Sprintf("🎉 %s يريد حجز عقارك '%s'. يرجى الرد بسرعة للتأكيد!", guestName, propertyTitle)

	// Create navigation parameters for deep linking
	params := fmt.Sprintf(`{"reservationId": %d, "propertyId": %d, "guestId": %d}`, reservationID, propertyID, guestID)

	data := NotificationData{
		Type:       "reservation_request",
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
	title := "💬 رسالة جديدة"
	body := fmt.Sprintf("%s أرسل لك رسالة بخصوص %s", senderName, propertyTitle)

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

// SendReservationStatusNotificationToGuest sends notification when reservation status changes
func (ns *NotificationService) SendReservationStatusNotificationToGuest(reservationID, propertyID, guestID, hostID uint, hostName, propertyTitle, status string) error {
	log.Printf("📱 NOTIFICATION DEBUG: Sending reservation status notification to guest %d", guestID)
	log.Printf("📱 NOTIFICATION DEBUG: Status: %s, Property: %s, Host: %s", status, propertyTitle, hostName)

	var title, body string
	var notificationType string

	switch status {
	case "confirmed":
		title = "✅ تم تأكيد الحجز!"
		body = fmt.Sprintf("🎉 أخبار رائعة! %s أكد حجزك لـ '%s'. استعد لإقامة لا تُنسى!", hostName, propertyTitle)
		notificationType = "reservation_confirmed"
	case "cancelled":
		title = "❌ تم إلغاء الحجز"
		body = fmt.Sprintf("للأسف، %s ألغى حجزك لـ '%s'. نساعدك في العثور على بديل.", hostName, propertyTitle)
		notificationType = "reservation_cancelled"
	case "expired":
		title = "⏰ انتهت صلاحية الحجز"
		body = fmt.Sprintf("انتهت صلاحية طلب الحجز لـ '%s'. يمكنك تقديم طلب جديد.", propertyTitle)
		notificationType = "reservation_expired"
	default:
		title = "📋 تحديث الحجز"
		body = fmt.Sprintf("تحديث بخصوص حجزك لـ '%s'.", propertyTitle)
		notificationType = "reservation_update"
	}

	params := fmt.Sprintf(`{"reservationId": %d, "propertyId": %d, "hostId": %d}`, reservationID, propertyID, hostID)

	data := NotificationData{
		Type:       notificationType,
		ID:         fmt.Sprintf("%d", reservationID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", guestID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "MyReservations",
		Params:     params,
		Action:     "view_reservation",
	}

	err := ns.SendNotificationToUser(guestID, title, body, data)
	if err != nil {
		log.Printf("❌ NOTIFICATION ERROR: Failed to send reservation status notification: %v", err)
	} else {
		log.Printf("✅ NOTIFICATION SUCCESS: Reservation status notification sent to guest %d", guestID)
	}
	return err
}

// SendReservationReminderNotificationToHost sends reminder notification to host about pending reservations
func (ns *NotificationService) SendReservationReminderNotificationToHost(reservationID, propertyID, hostID, guestID uint, guestName, propertyTitle string, hoursRemaining int) error {
	log.Printf("📱 NOTIFICATION DEBUG: Sending reservation reminder to host %d", hostID)

	title := "⏰ تذكير: حجز في الانتظار"
	body := fmt.Sprintf("طلب الحجز من %s لـ '%s' سينتهي خلال %d ساعات. يرجى الرد الآن!", guestName, propertyTitle, hoursRemaining)

	params := fmt.Sprintf(`{"reservationId": %d, "propertyId": %d, "guestId": %d}`, reservationID, propertyID, guestID)

	data := NotificationData{
		Type:       "reservation_reminder",
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
		log.Printf("❌ NOTIFICATION ERROR: Failed to send reservation reminder: %v", err)
	} else {
		log.Printf("✅ NOTIFICATION SUCCESS: Reservation reminder sent to host %d", hostID)
	}
	return err
}

// SendVideoInteractionNotificationToHost sends notification when video is liked/commented
func (ns *NotificationService) SendVideoInteractionNotificationToHost(hostID, userID uint, userName, interactionType, videoTitle string) error {
	var title, body string

	switch interactionType {
	case "like":
		title = "❤️ تم الإعجاب بفيديوك!"
		body = fmt.Sprintf("%s أعجب بفيديوك: %s", userName, videoTitle)
	case "comment":
		title = "💬 تعليق جديد!"
		body = fmt.Sprintf("%s علق على فيديوك: %s", userName, videoTitle)
	default:
		title = "📹 تفاعل مع الفيديو"
		body = fmt.Sprintf("%s تفاعل مع فيديوك: %s", userName, videoTitle)
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
	title := "🎉 تم قبول الحجز!"
	body := fmt.Sprintf("%s قبل حجزك لـ %s", hostName, propertyTitle)

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
	title := "😔 تم رفض الحجز"
	body := fmt.Sprintf("%s رفض حجزك لـ %s", hostName, propertyTitle)

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
	title := "🎉 مرحباً بك في habitat!"
	body := fmt.Sprintf("مرحباً %s! اكتشف مساكن رائعة في موريتانيا.", firstName)

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
		title = "⏰ تذكير: الحجز غداً!"
		body = fmt.Sprintf("لا تنس إقامتك في %s غداً!", propertyTitle)
	} else {
		title = "📅 تذكير الحجز"
		body = fmt.Sprintf("تبدأ إقامتك في %s خلال %d أيام!", propertyTitle, daysUntil)
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

// DebugUserNotificationSettings logs detailed information about a user's notification settings
func (ns *NotificationService) DebugUserNotificationSettings(userID uint) {
	log.Printf("🔍 NOTIFICATION DEBUG: Checking notification settings for user %d", userID)

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		log.Printf("❌ NOTIFICATION DEBUG: User %d not found: %v", userID, err)
		return
	}

	log.Printf("📱 NOTIFICATION DEBUG: User %d settings:", userID)
	log.Printf("  - AllowsNotifications: %v", user.AllowsNotifications != nil && *user.AllowsNotifications)
	log.Printf("  - PushTokens: %v", user.PushTokens != nil)

	if user.PushTokens != nil {
		var tokens []string
		if err := json.Unmarshal(user.PushTokens, &tokens); err == nil {
			log.Printf("  - Token count: %d", len(tokens))
			for i, token := range tokens {
				if i < 3 { // Show first 3 tokens
					log.Printf("  - Token %d: %s...", i+1, token[:20])
				}
			}
		} else {
			log.Printf("  - Error unmarshaling tokens: %v", err)
		}
	}
}
