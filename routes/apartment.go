package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// Reservations endpoints (Airbnb-like)

func GetReservationsByPropertyID(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	var reservations []models.Reservation
	res := storage.DB.Preload("Property").Preload("Guest").Where("property_id = ?", id).Order("created_at DESC").Find(&reservations)

	if res.Error != nil {
		utils.CreateError(
			iris.StatusInternalServerError,
			"Error", res.Error.Error(), ctx)
		return
	}

	ctx.JSON(reservations)
}

// GetHostReservations returns reservations for all properties owned by the authenticated host
func GetHostReservations(ctx iris.Context) {
	tok := jwt.Get(ctx)
	if tok == nil {
		utils.CreateError(iris.StatusUnauthorized, "Unauthorized", "Missing token", ctx)
		return
	}
	user := tok.(*utils.AccessToken)

	var reservations []models.Reservation
	// Join reservations with properties to filter by host id
	res := storage.DB.
		Joins("JOIN properties p ON p.id = reservations.property_id").
		Where("p.host_id = ?", user.ID).
		Preload("Property").
		Preload("Property.Host").
		Preload("Guest").
		Order("reservations.created_at DESC").
		Find(&reservations)

	if res.Error != nil {
		utils.CreateError(iris.StatusInternalServerError, "Error", res.Error.Error(), ctx)
		return
	}

	ctx.JSON(reservations)
}

// MarkReservationAsViewed marks a reservation as viewed by the host
func MarkReservationAsViewed(ctx iris.Context) {
	tok := jwt.Get(ctx)
	if tok == nil {
		utils.CreateError(iris.StatusUnauthorized, "Unauthorized", "Missing token", ctx)
		return
	}
	user := tok.(*utils.AccessToken)

	params := ctx.Params()
	reservationID := params.Get("id")

	// Verify that the reservation belongs to this host
	var reservation models.Reservation
	err := storage.DB.
		Joins("JOIN properties p ON p.id = reservations.property_id").
		Where("reservations.id = ? AND p.host_id = ?", reservationID, user.ID).
		First(&reservation).Error

	if err != nil {
		utils.CreateError(iris.StatusNotFound, "Not Found", "Reservation not found or not owned by host", ctx)
		return
	}

	// Mark as viewed
	hostViewed := true
	reservation.HostViewed = &hostViewed
	storage.DB.Save(&reservation)

	ctx.JSON(iris.Map{"success": true, "message": "Reservation marked as viewed"})
}

func GetUserReservations(ctx iris.Context) {
	params := ctx.Params()
	userID := params.Get("id")

	fmt.Printf("GetUserReservations: Looking for reservations for user ID: %s\n", userID)

	var reservations []models.Reservation
	res := storage.DB.Preload("Property").Preload("Property.Host").Preload("Guest").Where("guest_id = ?", userID).Order("created_at DESC").Find(&reservations)

	if res.Error != nil {
		fmt.Printf("GetUserReservations: Database error: %v\n", res.Error)
		utils.CreateError(
			iris.StatusInternalServerError,
			"Error", res.Error.Error(), ctx)
		return
	}

	fmt.Printf("GetUserReservations: Found %d reservations for user %s\n", len(reservations), userID)
	ctx.JSON(reservations)
}

type CreateReservationInput struct {
	CheckIn   time.Time `json:"checkIn" validate:"required"`
	CheckOut  time.Time `json:"checkOut" validate:"required"`
	NumGuests int       `json:"numGuests" validate:"required,gte=1,lte=16"`
	Note      string    `json:"note"`
}

func CreateReservation(ctx iris.Context) {
	params := ctx.Params()
	propertyID := params.Get("id")

	claims := jwt.Get(ctx).(*utils.AccessToken)

	var input CreateReservationInput
	err := ctx.ReadJSON(&input)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Basic sanity: check check-in < check-out
	if !input.CheckIn.Before(input.CheckOut) {
		utils.CreateError(iris.StatusBadRequest, "Validation Error", "checkIn must be before checkOut", ctx)
		return
	}

	// Get property details for price calculation
	var property models.Property
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		utils.CreateError(iris.StatusNotFound, "Not Found", "Property not found", ctx)
		return
	}

	// Calculate total price
	nights := int(input.CheckOut.Sub(input.CheckIn).Hours() / 24)
	if nights < 1 {
		nights = 1
	}

	nightlyPrice := property.NightlyPrice
	cleaningFee := float32(0)
	serviceFee := float32(0)

	// Calculate cleaning fee (2% of nightly price)
	cleaningFee = nightlyPrice * 0.02

	// Calculate service fee (0% for now, can be configured later)
	serviceFee = 0

	totalPrice := (nightlyPrice * float32(nights)) + cleaningFee + serviceFee

	// Persist reservation
	var reservation models.Reservation
	parsedID, _ := strconv.ParseUint(propertyID, 10, 64)
	reservation.PropertyID = uint(parsedID)
	reservation.GuestID = claims.ID
	reservation.CheckIn = input.CheckIn
	reservation.CheckOut = input.CheckOut
	reservation.NumGuests = input.NumGuests
	reservation.TotalPrice = totalPrice
	reservation.Status = "pending"
	reservation.Note = input.Note
	reservation.ExpiresAt = time.Now().Add(24 * time.Hour)

	createRes := storage.DB.Create(&reservation)
	if createRes.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Reload with relationships for response
	storage.DB.Preload("Property").Preload("Guest").First(&reservation, reservation.ID)

	// Create notification for host
	var notification models.Notification
	notification.UserID = property.HostID
	notification.Title = "🏠 طلب حجز جديد!"
	notification.Message = fmt.Sprintf("لديك طلب حجز جديد لـ '%s' من %s إلى %s", property.Title, input.CheckIn.Format("Jan 2, 2006"), input.CheckOut.Format("Jan 2, 2006"))
	notification.Type = "reservation_request"
	notification.RefID = uint(reservation.ID)
	notification.RefType = "reservation"
	notification.IsRead = false

	storage.DB.Create(&notification)

	// Create 1:1 conversation between guest and host for this reservation
	// Check if conversation already exists
	var existingConv models.Conversation
	storage.DB.Where("property_id = ? AND tenant_id = ? AND owner_id = ?",
		property.ID, claims.ID, property.HostID).First(&existingConv)

	if existingConv.ID == 0 {
		// Create new conversation with reservation message
		guestMessage := fmt.Sprintf("مرحباً! طلبت حجز %s من %s إلى %s لـ %d ضيوف.",
			property.Title, input.CheckIn.Format("Jan 2"), input.CheckOut.Format("Jan 2"), input.NumGuests)

		var messages []models.Message
		messages = append(messages, models.Message{
			SenderID:        claims.ID,
			ReceiverID:      property.HostID,
			Type:            "property_card",
			RefType:         "property",
			RefID:           &property.ID,
			PreviewTitle:    property.Title,
			PreviewSubtitle: property.City,
			PreviewImageURL: "",
		})
		messages = append(messages, models.Message{
			SenderID:   claims.ID,
			ReceiverID: property.HostID,
			Text:       guestMessage,
			Type:       "text",
		})

		conversation := models.Conversation{
			TenantID:   claims.ID,
			OwnerID:    property.HostID,
			PropertyID: property.ID,
			Messages:   messages,
		}

		if err := storage.DB.Create(&conversation).Error; err == nil {
			log.Printf("✅ Created 1:1 conversation between guest %d and host %d for property %d",
				claims.ID, property.HostID, property.ID)
		} else {
			log.Printf("❌ Failed to create conversation: %v", err)
		}
	} else {
		// Conversation exists, but we still need to add the new reservation message
		log.Printf("ℹ️ Conversation already exists between guest %d and host %d for property %d - Adding new reservation message",
			claims.ID, property.HostID, property.ID)

		// Create new reservation message for existing conversation
		guestMessage := fmt.Sprintf("مرحباً! طلبت حجز %s من %s إلى %s لـ %d ضيوف.",
			property.Title, input.CheckIn.Format("Jan 2"), input.CheckOut.Format("Jan 2"), input.NumGuests)

		// Add property card message
		propertyCardMessage := models.Message{
			ConversationID:  existingConv.ID,
			SenderID:        claims.ID,
			ReceiverID:      property.HostID,
			Type:            "property_card",
			RefType:         "property",
			RefID:           &property.ID,
			PreviewTitle:    property.Title,
			PreviewSubtitle: property.City,
			PreviewImageURL: "",
		}

		// Add text message
		textMessage := models.Message{
			ConversationID: existingConv.ID,
			SenderID:       claims.ID,
			ReceiverID:     property.HostID,
			Text:           guestMessage,
			Type:           "text",
		}

		// Save both messages
		if err := storage.DB.Create(&propertyCardMessage).Error; err != nil {
			log.Printf("❌ Failed to create property card message: %v", err)
		} else {
			log.Printf("✅ Created property card message for existing conversation")
		}

		if err := storage.DB.Create(&textMessage).Error; err != nil {
			log.Printf("❌ Failed to create text message: %v", err)
		} else {
			log.Printf("✅ Created reservation message for existing conversation")
		}
	}

	// Send push notification to host
	var guest models.User
	if err := storage.DB.First(&guest, claims.ID).Error; err == nil {
		guestName := fmt.Sprintf("%s %s", guest.FirstName, guest.LastName)

		// Debug the property and host information
		log.Printf("🏠 RESERVATION DEBUG: Property ID=%d, Title='%s', HostID=%d", property.ID, property.Title, property.HostID)
		log.Printf("👤 RESERVATION DEBUG: Guest ID=%d, Name='%s'", claims.ID, guestName)

		notificationService := services.NewNotificationService()

		// Debug host notification settings
		notificationService.DebugUserNotificationSettings(property.HostID)

		go notificationService.SendReservationNotificationToHost(
			reservation.ID,
			property.ID,
			property.HostID,
			claims.ID,
			guestName,
			property.Title,
		)

		// Send push notification to guest (reservation is pending)
		go func() {
			var guestUser models.User
			if err := storage.DB.First(&guestUser, claims.ID).Error; err == nil && guestUser.AllowsNotifications != nil && *guestUser.AllowsNotifications {
				// Create notification for guest
				var guestNotification models.Notification
				guestNotification.UserID = claims.ID
				guestNotification.Title = "📋 تم إرسال طلب الحجز"
				guestNotification.Message = fmt.Sprintf("طلب حجزك لـ '%s' قيد الانتظار. سيرد المالك قريباً.", property.Title)
				guestNotification.Type = "reservation_pending"
				guestNotification.RefID = uint(reservation.ID)
				guestNotification.RefType = "reservation"
				guestNotification.IsRead = false
				storage.DB.Create(&guestNotification)

				// Send push notification
				data := services.NotificationData{
					Type:       "reservation_pending",
					ID:         fmt.Sprintf("%d", reservation.ID),
					PropertyID: fmt.Sprintf("%d", property.ID),
					UserID:     fmt.Sprintf("%d", claims.ID),
				}

				notificationService.SendNotificationToUser(
					claims.ID,
					"📋 تم إرسال طلب الحجز",
					fmt.Sprintf("طلبك لـ '%s' قيد الانتظار. سيرد المالك قريباً.", property.Title),
					data,
				)
			}
		}()
	}

	ctx.JSON(reservation)
}

// Update reservation status (host action): confirm or reject
type UpdateReservationStatusInput struct {
	Status string `json:"status" validate:"required,oneof=confirmed rejected cancelled"`
}

func UpdateReservationStatus(ctx iris.Context) {
	id := ctx.Params().Get("id")

	var input UpdateReservationStatusInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var reservation models.Reservation
	if err := storage.DB.Preload("Property").First(&reservation, id).Error; err != nil {
		utils.CreateError(iris.StatusNotFound, "Not Found", "Reservation not found", ctx)
		return
	}

	// Auto-expire if past ExpiresAt and still pending
	if reservation.Status == "pending" && time.Now().After(reservation.ExpiresAt) {
		reservation.Status = "expired"
	} else {
		reservation.Status = input.Status
	}

	if err := storage.DB.Save(&reservation).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// When confirmed, block dates in availability as not available
	if reservation.Status == "confirmed" {
		start := reservation.CheckIn
		end := reservation.CheckOut
		for d := start; d.Before(end); d = d.Add(24 * time.Hour) {
			day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
			var avail models.PropertyAvailability
			if err := storage.DB.Where("property_id = ? AND date = ?", reservation.PropertyID, day).First(&avail).Error; err == nil && avail.ID != 0 {
				storage.DB.Model(&avail).Updates(map[string]interface{}{
					"is_available": false,
					"notes":        "booked",
				})
			} else {
				price := float64(reservation.Property.NightlyPrice)
				newAvail := models.PropertyAvailability{
					PropertyID:   reservation.PropertyID,
					Date:         day,
					IsAvailable:  false,
					Price:        price,
					MinStay:      1,
					MaxStay:      0,
					CheckInTime:  "15:00",
					CheckOutTime: "11:00",
					Notes:        "booked",
				}
				storage.DB.Where("property_id = ? AND date = ?", reservation.PropertyID, day).FirstOrCreate(&newAvail)
			}
		}
	}

	// Create notification for guest about status change
	var notification models.Notification
	notification.UserID = reservation.GuestID
	notification.Title = "Reservation Status Updated"
	notification.Message = fmt.Sprintf("Your reservation for %s has been %s", reservation.Property.Title, input.Status)
	notification.Type = "reservation_status"
	notification.RefID = uint(reservation.ID)
	notification.RefType = "reservation"
	notification.IsRead = false

	storage.DB.Create(&notification)

	// Send push notification to guest
	var host models.User
	if err := storage.DB.First(&host, reservation.Property.HostID).Error; err == nil {
		hostName := fmt.Sprintf("%s %s", host.FirstName, host.LastName)
		notificationService := services.NewNotificationService()

		// Send status notification to guest
		go notificationService.SendReservationStatusNotificationToGuest(
			reservation.ID,
			reservation.PropertyID,
			reservation.GuestID,
			reservation.Property.HostID,
			hostName,
			reservation.Property.Title,
			input.Status,
		)
	}

	ctx.JSON(reservation)
}

// CancelReservation handles reservation cancellation with policy validation
func CancelReservation(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	reservationID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid reservation ID"})
		return
	}

	// Get reservation with property details
	var reservation models.Reservation
	if err := storage.DB.Preload("Property").Where("id = ? AND guest_id = ?", reservationID, userID).First(&reservation).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Reservation not found"})
		return
	}

	// Check if reservation can be cancelled
	if reservation.Status == "confirmed" || reservation.Status == "completed" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Cannot cancel confirmed or completed reservations"})
		return
	}

	if reservation.Status == "cancelled" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Reservation is already cancelled"})
		return
	}

	// Calculate refund based on cancellation policy
	refundAmount, canCancel, reason := calculateRefund(reservation)

	if !canCancel {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message": "Cannot cancel reservation",
			"reason":  reason,
		})
		return
	}

	// Update reservation status
	reservation.Status = "cancelled"
	if err := storage.DB.Save(&reservation).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to cancel reservation"})
		return
	}

	// Free up the dates in availability
	start := reservation.CheckIn
	end := reservation.CheckOut
	for d := start; d.Before(end); d = d.Add(24 * time.Hour) {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		storage.DB.Model(&models.PropertyAvailability{}).
			Where("property_id = ? AND date = ?", reservation.PropertyID, day).
			Updates(map[string]interface{}{
				"is_available": true,
				"notes":        "",
			})
	}

	// Create notification
	var notification models.Notification
	notification.UserID = reservation.GuestID
	notification.Title = "Reservation Cancelled"
	notification.Message = fmt.Sprintf("Your reservation for %s has been cancelled. Refund: %.2f %s",
		reservation.Property.Title, refundAmount, reservation.Property.Currency)
	notification.Type = "reservation_cancelled"
	notification.RefID = uint(reservation.ID)
	notification.RefType = "reservation"
	notification.IsRead = false
	storage.DB.Create(&notification)

	ctx.JSON(iris.Map{
		"message":       "Reservation cancelled successfully",
		"refund_amount": refundAmount,
		"currency":      reservation.Property.Currency,
		"reason":        reason,
	})
}

// calculateRefund determines refund amount based on cancellation policy
func calculateRefund(reservation models.Reservation) (float32, bool, string) {
	now := time.Now()
	checkIn := reservation.CheckIn
	policy := reservation.Property.CancellationPolicy

	// Calculate days until check-in
	daysUntilCheckIn := int(checkIn.Sub(now).Hours() / 24)

	switch policy {
	case "flexible":
		// Full refund if cancelled 24+ hours before check-in
		if daysUntilCheckIn >= 1 {
			return reservation.TotalPrice, true, "Full refund - cancelled 24+ hours before check-in"
		}
		return 0, false, "No refund - cancelled less than 24 hours before check-in"

	case "moderate":
		// Full refund if cancelled 5+ days before check-in
		if daysUntilCheckIn >= 5 {
			return reservation.TotalPrice, true, "Full refund - cancelled 5+ days before check-in"
		}
		// 50% refund if cancelled 1-4 days before check-in
		if daysUntilCheckIn >= 1 {
			return reservation.TotalPrice * 0.5, true, "50% refund - cancelled 1-4 days before check-in"
		}
		return 0, false, "No refund - cancelled less than 24 hours before check-in"

	case "strict":
		// 50% refund if cancelled 7+ days before check-in
		if daysUntilCheckIn >= 7 {
			return reservation.TotalPrice * 0.5, true, "50% refund - cancelled 7+ days before check-in"
		}
		return 0, false, "No refund - cancelled less than 7 days before check-in"

	default:
		// Default to flexible policy
		if daysUntilCheckIn >= 1 {
			return reservation.TotalPrice, true, "Full refund - cancelled 24+ hours before check-in"
		}
		return 0, false, "No refund - cancelled less than 24 hours before check-in"
	}
}

// ValidateAvailabilityInput is used to check if a date range is free for booking
type ValidateAvailabilityInput struct {
	CheckIn  time.Time `json:"checkIn" validate:"required"`
	CheckOut time.Time `json:"checkOut" validate:"required"`
}

// ValidateReservationAvailability checks for conflicts before attempting to create a reservation
func ValidateReservationAvailability(ctx iris.Context) {
	propertyID := ctx.Params().Get("id")

	var input ValidateAvailabilityInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	if !input.CheckIn.Before(input.CheckOut) {
		utils.CreateError(iris.StatusBadRequest, "Validation Error", "checkIn must be before checkOut", ctx)
		return
	}

	// Check overlapping confirmed reservations
	var conflicts int64
	storage.DB.Model(&models.Reservation{}).
		Where("property_id = ? AND status = ? AND check_in < ? AND check_out > ?", propertyID, "confirmed", input.CheckOut, input.CheckIn).
		Count(&conflicts)

	// Additionally, check availability rows explicitly blocked (IsAvailable = false)
	// Iterate dates and verify none are blocked
	blocked := 0
	start := input.CheckIn
	end := input.CheckOut
	for d := start; d.Before(end); d = d.Add(24 * time.Hour) {
		day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		var avail models.PropertyAvailability
		if err := storage.DB.Where("property_id = ? AND date = ?", propertyID, day).First(&avail).Error; err == nil {
			if !avail.IsAvailable {
				blocked++
			}
		}
	}

	if conflicts > 0 || blocked > 0 {
		ctx.StatusCode(iris.StatusConflict)
		ctx.JSON(iris.Map{
			"ok":        false,
			"conflicts": conflicts,
			"blocked":   blocked,
			"message":   "Selected dates are not available",
		})
		return
	}

	ctx.JSON(iris.Map{"ok": true})
}

// Cron-like endpoint to expire old pending reservations (can be called by a scheduler)
func ExpirePendingReservations(ctx iris.Context) {
	// Set any pending reservations older than 24h to expired
	storage.DB.Model(&models.Reservation{}).
		Where("status = ? AND expires_at < ?", "pending", time.Now()).
		Update("status", "expired")
	ctx.JSON(iris.Map{"ok": true})
}
