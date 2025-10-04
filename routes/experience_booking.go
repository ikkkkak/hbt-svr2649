package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

type ExperienceBookingRequest struct {
	ExperienceID     uint   `json:"experienceId" validate:"required"`
	GroupID          uint   `json:"groupId" validate:"required"`
	ParticipantCount int    `json:"participantCount" validate:"required,min=1"`
	SelectedDate     string `json:"selectedDate" validate:"required"`
	SelectedTime     string `json:"selectedTime"`
	Notes            string `json:"notes"`
}

type ExperienceBookingResponse struct {
	ID               uint                   `json:"id"`
	ExperienceID     uint                   `json:"experienceId"`
	GroupID          uint                   `json:"groupId"`
	ParticipantCount int                    `json:"participantCount"`
	SelectedDate     string                 `json:"selectedDate"`
	SelectedTime     string                 `json:"selectedTime"`
	Notes            string                 `json:"notes"`
	Status           string                 `json:"status"`
	TotalPrice       float64                `json:"totalPrice"`
	CreatedAt        time.Time              `json:"createdAt"`
	Experience       models.Experience      `json:"experience"`
	Group            models.ExperienceGroup `json:"group"`
}

func CreateExperienceBooking(ctx iris.Context) {
	// Get userID from JWT context with proper error handling
	userIDValue := ctx.Values().Get("userID")
	if userIDValue == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User not authenticated"})
		return
	}

	userID, ok := userIDValue.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "Invalid user ID"})
		return
	}

	var request ExperienceBookingRequest

	if err := ctx.ReadJSON(&request); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Verify user is a member of the group
	var group models.ExperienceGroup
	if err := storage.DB.Where("id = ?", request.GroupID).First(&group).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Group not found"})
		return
	}

	// Check if user is a member of the group
	var member models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", request.GroupID, userID).First(&member).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "You are not a member of this group"})
		return
	}

	// Get experience details
	var experience models.Experience
	if err := storage.DB.First(&experience, request.ExperienceID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Experience not found"})
		return
	}

	// Check if participant count exceeds experience capacity
	if request.ParticipantCount > experience.GroupSize {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message":         "Participant count exceeds experience capacity",
			"maxParticipants": experience.GroupSize,
		})
		return
	}

	// Get group member count
	var memberCount int64
	storage.DB.Model(&models.ExperienceGroupMember{}).Where("group_id = ?", request.GroupID).Count(&memberCount)

	// Check if participant count is reasonable (not more than group size + some buffer)
	if request.ParticipantCount > int(memberCount)+2 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message":   "Participant count is too high for this group",
			"groupSize": memberCount,
		})
		return
	}

	// Parse selected date
	selectedDate, err := time.Parse("2006-01-02", request.SelectedDate)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid date format"})
		return
	}

	// Check if date is in the past
	if selectedDate.Before(time.Now().Truncate(24 * time.Hour)) {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Cannot book experiences in the past"})
		return
	}

	// Check for existing bookings on the same date and count total participants
	var existingBookings []models.ExperienceBooking
	storage.DB.Where("experience_id = ? AND selected_date = ? AND status != ?",
		request.ExperienceID, selectedDate, "cancelled").Find(&existingBookings)

	totalExistingParticipants := 0
	for _, booking := range existingBookings {
		totalExistingParticipants += booking.ParticipantCount
	}

	// Check if adding this booking would exceed capacity
	if totalExistingParticipants+request.ParticipantCount > experience.GroupSize {
		ctx.StatusCode(iris.StatusConflict)
		ctx.JSON(iris.Map{
			"message":               "Not enough spots available for this date",
			"maxParticipants":       experience.GroupSize,
			"existingParticipants":  totalExistingParticipants,
			"requestedParticipants": request.ParticipantCount,
			"availableSpots":        experience.GroupSize - totalExistingParticipants,
		})
		return
	}

	// Create the booking
	booking := models.ExperienceBooking{
		ExperienceID:     request.ExperienceID,
		GroupID:          request.GroupID,
		ParticipantCount: request.ParticipantCount,
		SelectedDate:     selectedDate,
		SelectedTime:     request.SelectedTime,
		Notes:            request.Notes,
		Status:           "confirmed",
		TotalPrice:       experience.PricePerPerson * float64(request.ParticipantCount),
		UserID:           userID,
		GuestID:          userID, // Set guest_id to the same as user_id for now
	}

	if err := storage.DB.Create(&booking).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to create booking"})
		return
	}

	// Only block availability if this booking fills the experience
	if totalExistingParticipants+request.ParticipantCount >= experience.GroupSize {
		availability := models.ExperienceAvailability{
			ExperienceID: request.ExperienceID,
			Date:         selectedDate,
			Status:       "blocked",
		}

		// Check if availability already exists
		var existingAvailability models.ExperienceAvailability
		availabilityResult := storage.DB.Where("experience_id = ? AND date = ?",
			request.ExperienceID, selectedDate).First(&existingAvailability)

		if availabilityResult.Error == nil {
			// Update existing availability
			existingAvailability.Status = "blocked"
			storage.DB.Save(&existingAvailability)
		} else {
			// Create new availability record
			storage.DB.Create(&availability)
		}
	}

	// Load related data for response
	storage.DB.Preload("Host").First(&experience, request.ExperienceID)
	storage.DB.First(&group, request.GroupID)

	// Create notification for booking success
	notification := models.Notification{
		UserID: userID,
		Type:   "experience_booking_confirmed",
		Title:  "Experience Booking Confirmed! 🎉",
		Message: fmt.Sprintf("Your booking for '%s' on %s has been confirmed. Total: %.0f MRU",
			experience.Title,
			booking.SelectedDate.Format("January 2, 2006"),
			booking.TotalPrice),
		RefType: "experience_booking",
		RefID:   booking.ID,
	}
	storage.DB.Create(&notification)

	// Send push notification to experience host
	var guest models.User
	if err := storage.DB.First(&guest, userID).Error; err == nil {
		guestName := fmt.Sprintf("%s %s", guest.FirstName, guest.LastName)
		notificationService := services.NewNotificationService()
		go notificationService.SendExperienceBookingNotificationToHost(
			booking.ExperienceID,
			experience.HostID,
			userID,
			guestName,
			experience.Title,
		)
	}

	// Send booking ticket to group chat automatically
	go sendBookingTicketToGroup(booking, experience, group)

	response := ExperienceBookingResponse{
		ID:               booking.ID,
		ExperienceID:     booking.ExperienceID,
		GroupID:          booking.GroupID,
		ParticipantCount: booking.ParticipantCount,
		SelectedDate:     booking.SelectedDate.Format("2006-01-02"),
		SelectedTime:     booking.SelectedTime,
		Notes:            booking.Notes,
		Status:           booking.Status,
		TotalPrice:       booking.TotalPrice,
		CreatedAt:        booking.CreatedAt,
		Experience:       experience,
		Group:            group,
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Booking created successfully",
		"data":    response,
	})
}

func GetExperienceBookings(ctx iris.Context) {
	// Get userID from JWT context with proper error handling
	userIDValue := ctx.Values().Get("userID")
	if userIDValue == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User not authenticated"})
		return
	}

	userID, ok := userIDValue.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "Invalid user ID"})
		return
	}

	var bookings []models.ExperienceBooking
	if err := storage.DB.Where("user_id = ?", userID).
		Preload("Experience").
		Preload("Group").
		Order("created_at DESC").
		Find(&bookings).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch bookings"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    bookings,
	})
}

func CancelExperienceBooking(ctx iris.Context) {
	// Get userID from JWT context with proper error handling
	userIDValue := ctx.Values().Get("userID")
	if userIDValue == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User not authenticated"})
		return
	}

	userID, ok := userIDValue.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "Invalid user ID"})
		return
	}

	bookingID := ctx.Params().GetUintDefault("id", 0)

	if bookingID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid booking ID"})
		return
	}

	var booking models.ExperienceBooking
	if err := storage.DB.Where("id = ? AND user_id = ?", bookingID, userID).First(&booking).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Booking not found"})
		return
	}

	// Check if booking can be cancelled (e.g., not within 24 hours)
	if time.Until(booking.SelectedDate) < 24*time.Hour {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Cannot cancel booking within 24 hours of experience"})
		return
	}

	// Update booking status
	booking.Status = "cancelled"
	if err := storage.DB.Save(&booking).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to cancel booking"})
		return
	}

	// Check if we need to update availability
	var availability models.ExperienceAvailability
	if err := storage.DB.Where("experience_id = ? AND date = ?",
		booking.ExperienceID, booking.SelectedDate).First(&availability).Error; err == nil {
		// Check remaining bookings for this date
		var remainingBookings []models.ExperienceBooking
		storage.DB.Where("experience_id = ? AND selected_date = ? AND status != ?",
			booking.ExperienceID, booking.SelectedDate, "cancelled").Find(&remainingBookings)

		totalRemainingParticipants := 0
		for _, remainingBooking := range remainingBookings {
			totalRemainingParticipants += remainingBooking.ParticipantCount
		}

		// Get experience to check max capacity
		var experience models.Experience
		if err := storage.DB.First(&experience, booking.ExperienceID).Error; err == nil {
			if totalRemainingParticipants < experience.GroupSize {
				// Make available again
				availability.Status = "available"
				storage.DB.Save(&availability)
			}
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Booking cancelled successfully",
	})
}

func GetHostExperienceBookings(ctx iris.Context) {
	// Get userID from JWT context with proper error handling
	userIDValue := ctx.Values().Get("userID")
	if userIDValue == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User not authenticated"})
		return
	}

	userID, ok := userIDValue.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "Invalid user ID"})
		return
	}

	// Get all experience bookings for experiences owned by this host
	var bookings []models.ExperienceBooking
	if err := storage.DB.
		Joins("JOIN experiences ON experience_bookings.experience_id = experiences.id").
		Where("experiences.host_id = ?", userID).
		Preload("Experience").
		Preload("Group").
		Preload("Group.Members").
		Preload("Group.Members.User").
		Order("experience_bookings.created_at DESC").
		Find(&bookings).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch host bookings"})
		return
	}

	// Format response with additional fields
	var response []iris.Map
	for _, booking := range bookings {
		response = append(response, iris.Map{
			"id":               booking.ID,
			"experienceId":     booking.ExperienceID,
			"groupId":          booking.GroupID,
			"participantCount": booking.ParticipantCount,
			"selectedDate":     booking.SelectedDate.Format("2006-01-02"),
			"selectedTime":     booking.SelectedTime,
			"notes":            booking.Notes,
			"status":           booking.Status,
			"totalPrice":       booking.TotalPrice,
			"createdAt":        booking.CreatedAt,
			"isRead":           booking.IsRead,
			"experience": iris.Map{
				"id":             booking.Experience.ID,
				"title":          booking.Experience.Title,
				"city":           booking.Experience.City,
				"pricePerPerson": booking.Experience.PricePerPerson,
				"groupSize":      booking.Experience.GroupSize,
				"photos":         booking.Experience.Photos,
			},
			"group": iris.Map{
				"id":       booking.Group.ID,
				"name":     booking.Group.Name,
				"photoURL": booking.Group.PhotoURL,
				"members":  booking.Group.Members,
			},
		})
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
	})
}

func MarkBookingAsRead(ctx iris.Context) {
	// Get userID from JWT context with proper error handling
	userIDValue := ctx.Values().Get("userID")
	if userIDValue == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "User not authenticated"})
		return
	}

	userID, ok := userIDValue.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"message": "Invalid user ID"})
		return
	}

	bookingID := ctx.Params().GetUintDefault("id", 0)
	if bookingID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid booking ID"})
		return
	}

	// Verify the booking belongs to this host
	var booking models.ExperienceBooking
	if err := storage.DB.
		Joins("JOIN experiences ON experience_bookings.experience_id = experiences.id").
		Where("experience_bookings.id = ? AND experiences.host_id = ?", bookingID, userID).
		First(&booking).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Booking not found"})
		return
	}

	// Mark as read
	booking.IsRead = true
	if err := storage.DB.Save(&booking).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to mark booking as read"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Booking marked as read",
	})
}

// sendBookingTicketToGroup sends a booking confirmation ticket to the group chat
func sendBookingTicketToGroup(booking models.ExperienceBooking, experience models.Experience, group models.ExperienceGroup) {
	// Get participant names from group members
	var participantNames []string
	var members []models.ExperienceGroupMember
	storage.DB.Where("group_id = ? AND state = ?", booking.GroupID, "joined").Preload("User").Find(&members)

	for i, member := range members {
		if i < booking.ParticipantCount {
			participantNames = append(participantNames, fmt.Sprintf("%s %s", member.User.FirstName, member.User.LastName))
		}
	}

	// Format participant names
	participantsText := ""
	if len(participantNames) > 0 {
		participantsText = fmt.Sprintf("Participants: %s", strings.Join(participantNames, ", "))
	} else {
		participantsText = fmt.Sprintf("Participants: %d people", booking.ParticipantCount)
	}

	// Format the booking ticket message (cleaner, less icons)
	message := fmt.Sprintf(`BOOKING CONFIRMED ✅

Experience: %s
Date: %s
Time: %s
%s
Price per person: %.0f MRU
Total: %.0f MRU

You'll receive confirmation details soon!`,
		experience.Title,
		booking.SelectedDate.Format("January 2, 2006"),
		booking.SelectedTime,
		participantsText,
		experience.PricePerPerson,
		booking.TotalPrice)

	// Create the chat message
	chatMessage := models.ChatMessage{
		GroupID:  booking.GroupID,
		SenderID: booking.UserID,
		Content:  message,
		Color:    "#4CAF50", // Green color for booking confirmation
	}

	// Save the message to database
	if err := storage.DB.Create(&chatMessage).Error; err != nil {
		fmt.Printf("Error sending booking ticket to group chat: %v\n", err)
		return
	}

	// Preload sender for display
	storage.DB.Preload("Sender").First(&chatMessage, chatMessage.ID)

	fmt.Printf("Booking ticket sent to group %d successfully\n", booking.GroupID)
}
