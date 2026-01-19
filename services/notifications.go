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

// SendPropertyOfferNotificationToHost sends notification when an offer is made on a property for sale
func (ns *NotificationService) SendPropertyOfferNotificationToHost(offerID, propertyID, hostID, userID uint, userName, propertyTitle string, offerAmount float64) error {
	log.Printf("📱 NOTIFICATION DEBUG: Sending property offer notification to host %d", hostID)

	title := "💰 عرض شراء جديد!"
	body := fmt.Sprintf("🏠 %s قدم عرض شراء بقيمة %.0f MRU لعقارك '%s'. تواصل معه الآن!", userName, offerAmount, propertyTitle)

	params := fmt.Sprintf(`{"offerId": %d, "propertyId": %d, "userId": %d}`, offerID, propertyID, userID)

	data := NotificationData{
		Type:       "property_offer",
		ID:         fmt.Sprintf("%d", offerID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", userID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "Messages",
		Params:     params,
		Action:     "view_conversation",
	}

	err := ns.SendNotificationToUser(hostID, title, body, data)
	if err != nil {
		log.Printf("❌ NOTIFICATION ERROR: Failed to send property offer notification: %v", err)
	} else {
		log.Printf("✅ NOTIFICATION SUCCESS: Property offer notification sent to host %d", hostID)
	}
	return err
}

// SendPropertyTourNotificationToHost sends notification when a tour is requested for a property for sale
func (ns *NotificationService) SendPropertyTourNotificationToHost(tourID, propertyID, hostID, userID uint, userName, propertyTitle, tourDate, tourTime, tourType string) error {
	log.Printf("📱 NOTIFICATION DEBUG: Sending property tour notification to host %d", hostID)

	tourTypeArabic := "حضوري"
	if tourType == "video" {
		tourTypeArabic = "فيديو"
	}

	title := "🏠 طلب زيارة جديد!"
	body := fmt.Sprintf("📅 %s يريد زيارة عقارك '%s' يوم %s الساعة %s (زيارة %s). تواصل معه الآن!", userName, propertyTitle, tourDate, tourTime, tourTypeArabic)

	params := fmt.Sprintf(`{"tourId": %d, "propertyId": %d, "userId": %d}`, tourID, propertyID, userID)

	data := NotificationData{
		Type:       "property_tour",
		ID:         fmt.Sprintf("%d", tourID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		UserID:     fmt.Sprintf("%d", userID),
		HostID:     fmt.Sprintf("%d", hostID),
		Screen:     "Messages",
		Params:     params,
		Action:     "view_conversation",
	}

	err := ns.SendNotificationToUser(hostID, title, body, data)
	if err != nil {
		log.Printf("❌ NOTIFICATION ERROR: Failed to send property tour notification: %v", err)
	} else {
		log.Printf("✅ NOTIFICATION SUCCESS: Property tour notification sent to host %d", hostID)
	}
	return err
}

// SendNewPropertyNotification sends notification to users when a new property matches their favorite city
// Uses professional matching algorithm: Prioritizes exact city ID match, then zone ID, then name-based matching
func (ns *NotificationService) SendNewPropertyNotification(propertyID uint, propertyTitle string, cityID *uint, cityName string, zoneID *uint, zoneName string, bedrooms int, bathrooms int, squareFootage int, imageURL string) error {
	log.Printf("🔔 Sending new property notification for property %d in %s (CityID: %v, ZoneID: %v)", propertyID, cityName, cityID, zoneID)

	// SECURITY: Find all logged-in users with favorite city matching this property
	// Professional matching algorithm: Prioritize ID-based matching (most accurate), then fallback to name-based
	var users []models.User
	query := storage.DB.Where("allows_notifications = ?", true)
	
	// Professional matching: Match by city ID first (most accurate), then zone ID, then name-based
	if cityID != nil {
		// Exact city ID match (highest priority)
		if zoneID != nil {
			// Match by both city ID and zone ID (most specific)
			query = query.Where("(favorite_city_id = ? OR favorite_zone_id = ?)", *cityID, *zoneID)
		} else {
			// Match by city ID only
			query = query.Where("favorite_city_id = ?", *cityID)
		}
	} else if zoneID != nil {
		// Fallback: Match by zone ID if city ID not available
		query = query.Where("favorite_zone_id = ?", *zoneID)
	} else if cityName != "" {
		// Fallback: Name-based matching (case-insensitive for better matching)
		query = query.Where("(LOWER(favorite_city_name) = LOWER(?) OR LOWER(favorite_zone_name) = LOWER(?))", cityName, zoneName)
	} else {
		log.Printf("⚠️ No city or zone information provided for property %d", propertyID)
		return fmt.Errorf("insufficient location data for matching")
	}

	// SECURITY: Only notify users who have explicitly enabled notifications
	// This prevents spam and respects user preferences
	if err := query.Find(&users).Error; err != nil {
		log.Printf("❌ Error finding users for notification: %v", err)
		return err
	}

	log.Printf("📱 Found %d logged-in users to notify (matching algorithm: cityID=%v, zoneID=%v, cityName=%s)", len(users), cityID, zoneID, cityName)

	// Build notification content (needed for both logged-in and anonymous users)
	details := fmt.Sprintf("%d bedrooms • %d bathrooms • %d m²", bedrooms, bathrooms, squareFootage)
	if cityName != "" {
		details += fmt.Sprintf(" • %s", cityName)
	}
	if zoneName != "" {
		details += fmt.Sprintf(", %s", zoneName)
	}

	title := "🏠 New Property Available!"
	body := fmt.Sprintf("%s\n%s", propertyTitle, details)

	// Notification data for deep linking
	data := NotificationData{
		Type:       "new_property",
		ID:         fmt.Sprintf("%d", propertyID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		Screen:     "PropertySaleDetails",
		Params:     fmt.Sprintf(`{"propertyId": %d}`, propertyID),
		Action:     "view_property",
	}

	dataMap := map[string]string{
		"type":       data.Type,
		"id":         data.ID,
		"propertyId": data.PropertyID,
		"screen":     data.Screen,
		"params":     data.Params,
		"action":     data.Action,
	}

	// Initialize counters
	var lastError error
	successCount := 0

	// Also find anonymous users with matching favorite city (same professional algorithm)
	var anonymousUsers []models.AnonymousUserPreference
	anonQuery := storage.DB.Where("last_active >= ?", time.Now().AddDate(0, 0, -30))
	
	// Same professional matching algorithm for anonymous users
	if cityID != nil {
		if zoneID != nil {
			anonQuery = anonQuery.Where("(favorite_city_id = ? OR favorite_zone_id = ?)", *cityID, *zoneID)
		} else {
			anonQuery = anonQuery.Where("favorite_city_id = ?", *cityID)
		}
	} else if zoneID != nil {
		anonQuery = anonQuery.Where("favorite_zone_id = ?", *zoneID)
	} else if cityName != "" {
		anonQuery = anonQuery.Where("(LOWER(favorite_city_name) = LOWER(?) OR LOWER(favorite_zone_name) = LOWER(?))", cityName, zoneName)
	}
	if err := anonQuery.Find(&anonymousUsers).Error; err == nil {
		log.Printf("📱 Found %d anonymous users with matching preferences", len(anonymousUsers))
		
		// Get push tokens for anonymous users from NotificationPreference table
		for _, anonUser := range anonymousUsers {
			var prefs []models.NotificationPreference
			prefQuery := storage.DB.Where("enabled = ?", true)
			
			// Match by device ID (direct match)
			if anonUser.DeviceID != "" {
				prefQuery = prefQuery.Where("device_id = ?", anonUser.DeviceID)
			}
			
			// Also try to match by phone number if available
			if anonUser.PhoneNumber != nil && *anonUser.PhoneNumber != "" {
				prefQuery = prefQuery.Or("user_id IN (SELECT id FROM users WHERE phone_number = ?)", *anonUser.PhoneNumber)
			}
			
			if err := prefQuery.Find(&prefs).Error; err == nil && len(prefs) > 0 {
				log.Printf("📱 Found %d push tokens for anonymous device %s", len(prefs), func() string {
					if len(anonUser.DeviceID) > 10 {
						return anonUser.DeviceID[:10] + "..."
					}
					return anonUser.DeviceID
				}())
				
				for _, pref := range prefs {
					expoToken := pref.PushToken
					if strings.Contains(expoToken, "|") {
						expoToken = strings.Split(expoToken, "|")[0]
					}
					if err := utils.SendRichNotification(expoToken, title, body, imageURL, dataMap); err != nil {
						deviceIDPreview := anonUser.DeviceID
						if len(deviceIDPreview) > 10 {
							deviceIDPreview = deviceIDPreview[:10]
						}
						log.Printf("⚠️ Failed to send notification to anonymous user (device: %s): %v", deviceIDPreview, err)
					} else {
						successCount++
						log.Printf("✅ Sent notification to anonymous user (device: %s)", func() string {
							if len(anonUser.DeviceID) > 10 {
								return anonUser.DeviceID[:10] + "..."
							}
							return anonUser.DeviceID
						}())
					}
				}
			} else {
				log.Printf("⚠️ No push tokens found for anonymous device %s", func() string {
					if len(anonUser.DeviceID) > 10 {
						return anonUser.DeviceID[:10] + "..."
					}
					return anonUser.DeviceID
				}())
			}
		}
	}

	// Send to all matching logged-in users
	for _, user := range users {
		if user.AllowsNotifications == nil || !*user.AllowsNotifications {
			continue
		}

		tokens, err := ns.getUserPushTokens(user.ID)
		if err != nil {
			log.Printf("⚠️ Failed to get tokens for user %d: %v", user.ID, err)
			continue
		}

		for _, token := range tokens {
			expoToken := token
			if strings.Contains(token, "|") {
				expoToken = strings.Split(token, "|")[0]
			}

			// Use rich notification with image
			if err := utils.SendRichNotification(expoToken, title, body, imageURL, dataMap); err != nil {
				log.Printf("⚠️ Failed to send notification to user %d: %v", user.ID, err)
				lastError = err
			} else {
				successCount++
			}
		}
	}

	log.Printf("✅ Sent %d notifications successfully", successCount)
	return lastError
}

// SendGenericPropertyNotification sends a generic property notification when personalization fails
func (ns *NotificationService) SendGenericPropertyNotification(propertyID uint, propertyTitle string, cityName string, bedrooms int, bathrooms int, squareFootage int, imageURL string, userIDs []uint) error {
	log.Printf("🔔 Sending generic property notification for property %d", propertyID)

	details := fmt.Sprintf("%d bedrooms • %d bathrooms • %d m²", bedrooms, bathrooms, squareFootage)
	if cityName != "" {
		details += fmt.Sprintf(" • %s", cityName)
	}

	title := "🏠 New Property Available!"
	body := fmt.Sprintf("%s\n%s", propertyTitle, details)

	data := NotificationData{
		Type:       "new_property",
		ID:         fmt.Sprintf("%d", propertyID),
		PropertyID: fmt.Sprintf("%d", propertyID),
		Screen:     "PropertySaleDetails",
		Params:     fmt.Sprintf(`{"propertyId": %d}`, propertyID),
		Action:     "view_property",
	}

	dataMap := map[string]string{
		"type":       data.Type,
		"id":         data.ID,
		"propertyId": data.PropertyID,
		"screen":     data.Screen,
		"params":     data.Params,
		"action":     data.Action,
	}

	var lastError error
	successCount := 0

	for _, userID := range userIDs {
		tokens, err := ns.getUserPushTokens(userID)
		if err != nil {
			continue
		}

		for _, token := range tokens {
			expoToken := token
			if strings.Contains(token, "|") {
				expoToken = strings.Split(token, "|")[0]
			}

			if err := utils.SendRichNotification(expoToken, title, body, imageURL, dataMap); err != nil {
				lastError = err
			} else {
				successCount++
			}
		}
	}

	log.Printf("✅ Sent %d generic notifications successfully", successCount)
	return lastError
}

// NotifyUserAboutExistingProperties sends notifications to a user about existing published properties
// matching their favorite city when they set it for the first time
func (ns *NotificationService) NotifyUserAboutExistingProperties(userID uint, cityID *uint, cityName string, zoneID *uint, zoneName string) error {
	log.Printf("🔔 Notifying user %d about existing properties in favorite city: %s", userID, cityName)

	// Get user to check notification preferences
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		log.Printf("❌ User %d not found: %v", userID, err)
		return err
	}

	// Check if user has notifications enabled
	if user.AllowsNotifications == nil || !*user.AllowsNotifications {
		log.Printf("⚠️ User %d has notifications disabled, skipping", userID)
		return nil
	}

	// Find published properties matching the favorite city
	var properties []models.PropertySale
	query := storage.DB.Where("status = ? AND is_published = ?", "published", true)

	// Professional matching algorithm: Match by city ID first (most accurate), then by name
	if cityID != nil {
		query = query.Where("city_id = ?", *cityID)
	} else if cityName != "" {
		query = query.Where("LOWER(city) = LOWER(?)", cityName)
	}

	// Also match by zone if provided
	if zoneID != nil {
		query = query.Or("zone_id = ?", *zoneID)
	} else if zoneName != "" {
		query = query.Or("LOWER(zone_name) = LOWER(?)", zoneName)
	}

	// Limit to recent properties (last 30 days) to avoid overwhelming users
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	query = query.Where("created_at >= ?", thirtyDaysAgo)

	// Order by creation date (newest first) and limit to top 5
	query = query.Order("created_at DESC").Limit(5)

	if err := query.Find(&properties).Error; err != nil {
		log.Printf("❌ Error finding properties for user %d: %v", userID, err)
		return err
	}

	if len(properties) == 0 {
		log.Printf("📭 No existing properties found for user %d in city %s", userID, cityName)
		return nil
	}

	log.Printf("📦 Found %d existing properties for user %d in city %s", len(properties), userID, cityName)

	// Get user's push tokens
	tokens, err := ns.getUserPushTokens(userID)
	if err != nil {
		log.Printf("⚠️ Failed to get push tokens for user %d: %v", userID, err)
		return err
	}

	if len(tokens) == 0 {
		log.Printf("⚠️ No push tokens found for user %d", userID)
		return nil
	}

	// Send notifications for each property (batch them intelligently)
	// Send first property immediately, then batch the rest
	successCount := 0
	for i, property := range properties {
		// Get first image URL
		var imageURL string
		if len(property.Images) > 0 && property.Images[0] != "" {
			imageURL = property.Images[0]
		}

		// Build notification content
		details := fmt.Sprintf("%d bedrooms • %d bathrooms • %d m²", property.Bedrooms, property.Bathrooms, property.SquareFootage)
		if cityName != "" {
			details += fmt.Sprintf(" • %s", cityName)
		}

		title := "🏠 Properties Available in Your Favorite City!"
		body := fmt.Sprintf("%s\n%s", property.Title, details)

		// Notification data for deep linking
		dataMap := map[string]string{
			"type":       "existing_property",
			"id":         fmt.Sprintf("%d", property.ID),
			"propertyId": fmt.Sprintf("%d", property.ID),
			"screen":     "PropertySaleDetails",
			"params":     fmt.Sprintf(`{"propertyId": %d}`, property.ID),
			"action":     "view_property",
		}

		// Send notification with image
		for _, token := range tokens {
			expoToken := token
			if strings.Contains(token, "|") {
				expoToken = strings.Split(token, "|")[0]
			}

			if err := utils.SendRichNotification(expoToken, title, body, imageURL, dataMap); err != nil {
				log.Printf("⚠️ Failed to send notification to user %d for property %d: %v", userID, property.ID, err)
			} else {
				successCount++
			}
		}

		// Add delay between notifications to avoid overwhelming the user
		if i < len(properties)-1 {
			time.Sleep(2 * time.Second)
		}
	}

	log.Printf("✅ Sent %d notifications about existing properties to user %d", successCount, userID)
	return nil
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
