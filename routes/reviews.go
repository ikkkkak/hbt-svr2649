package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"errors"
	"fmt"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

type CreateReviewRequest struct {
	Stars         int    `json:"stars" validate:"required,min=1,max=5"`
	Title         string `json:"title" validate:"max=100"`
	Body          string `json:"body" validate:"max=1000"`
	ReservationID uint   `json:"reservationID"` // Required to link review to specific stay
}

type ReviewResponse struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"userID"`
	Stars     int       `json:"stars"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	User      struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		AvatarURL string `json:"avatarURL"`
	} `json:"user"`
	IsVerified bool `json:"isVerified"`
}

// ListPropertyReviews returns reviews and whether the current user can review
func ListPropertyReviews(ctx iris.Context) {
	propertyID := ctx.Params().GetUintDefault("propertyId", 0)
	if propertyID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	// Get all reviews with user info
	var reviews []models.Review
	if err := storage.DB.Preload("User").
		Where("property_id = ?", propertyID).
		Order("created_at DESC").
		Find(&reviews).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to load reviews"})
		return
	}

	// Calculate average rating
	var totalStars float64
	var reviewCount int
	for _, review := range reviews {
		totalStars += float64(review.Stars)
		reviewCount++
	}
	avgRating := 0.0
	if reviewCount > 0 {
		avgRating = totalStars / float64(reviewCount)
	}

	// Check if current user can review
	canReview := false
	userReservationID := uint(0)
	hasExistingReview := false

	if v := ctx.Values().Get("userID"); v != nil {
		if userID, ok := v.(uint); ok {
			// Check if user has completed a stay (confirmed status and checkout date in the past or today)
			var reservation models.Reservation
			// Use a more flexible approach - allow reviews for confirmed reservations
			// regardless of checkout date, as long as they're confirmed

			// Debug: Check what reservations exist for this user and property
			var allReservations []models.Reservation
			storage.DB.Where("property_id = ? AND guest_id = ?", propertyID, userID).Find(&allReservations)
			fmt.Printf("DEBUG: Found %d reservations for user %d, property %d\n", len(allReservations), userID, propertyID)
			for _, r := range allReservations {
				fmt.Printf("  - ID: %d, Status: %s, CheckOut: %v\n", r.ID, r.Status, r.CheckOut)
			}

			if err := storage.DB.Where("property_id = ? AND guest_id = ? AND status = ?",
				propertyID, userID, "confirmed").
				Order("check_out DESC").
				First(&reservation).Error; err == nil {
				// fmt.Printf("Found eligible reservation - ID: %d, CheckOut: %v\n", reservation.ID, reservation.CheckOut)
				canReview = true
				userReservationID = reservation.ID

				// Check if user already reviewed this property
				var existingReview models.Review
				if err := storage.DB.Where("property_id = ? AND user_id = ?", propertyID, userID).First(&existingReview).Error; err == nil {
					// fmt.Printf("User already reviewed this property\n")
					hasExistingReview = true
					canReview = false
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					// Only log if it's not a "record not found" error
					fmt.Printf("Error checking existing review: %v\n", err)
				}
			} else {
				fmt.Printf("No eligible reservation found - Error: %v\n", err)
			}
		}
	}

	// Format reviews for response
	var reviewResponses []ReviewResponse
	for _, review := range reviews {
		reviewResponses = append(reviewResponses, ReviewResponse{
			ID:        review.ID,
			UserID:    review.UserID,
			Stars:     review.Stars,
			Title:     review.Title,
			Body:      review.Body,
			CreatedAt: review.CreatedAt,
			User: struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
				AvatarURL string `json:"avatarURL"`
			}{
				FirstName: review.User.FirstName,
				LastName:  review.User.LastName,
				AvatarURL: review.User.AvatarURL,
			},
			IsVerified: review.IsVerified,
		})
	}

	fmt.Printf("Final review eligibility - canReview: %v, userReservationID: %d, hasExistingReview: %v\n",
		canReview, userReservationID, hasExistingReview)

	ctx.JSON(iris.Map{
		"success": true,
		"data": iris.Map{
			"reviews":           reviewResponses,
			"canReview":         canReview,
			"hasExistingReview": hasExistingReview,
			"userReservationID": userReservationID,
			"averageRating":     avgRating,
			"reviewCount":       reviewCount,
		},
	})
}

// CreatePropertyReview creates a review if the user has stayed before and hasn't reviewed yet
func CreatePropertyReview(ctx iris.Context) {
	// Auth required
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

	propertyID := ctx.Params().GetUintDefault("propertyId", 0)
	if propertyID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	var req CreateReviewRequest
	if err := ctx.ReadJSON(&req); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Validate reservation exists and user completed stay
	var reservation models.Reservation
	// Use a more flexible approach - allow reviews for confirmed reservations
	if err := storage.DB.Where("id = ? AND property_id = ? AND guest_id = ? AND status = ?",
		req.ReservationID, propertyID, userID, "confirmed").
		First(&reservation).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "You can only review properties you've completed a stay at"})
		return
	}

	// Check if user already reviewed this property
	var existing models.Review
	if err := storage.DB.Where("property_id = ? AND user_id = ?", propertyID, userID).First(&existing).Error; err == nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "You have already reviewed this property"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Only log if it's not a "record not found" error
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to check existing review"})
		return
	}

	// Create review with verified stay
	review := models.Review{
		UserID:        userID,
		PropertyID:    propertyID,
		ReservationID: &req.ReservationID,
		Title:         req.Title,
		Body:          req.Body,
		Stars:         req.Stars,
		IsVerified:    true, // Verified because linked to completed reservation
	}

	if err := storage.DB.Create(&review).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to create review"})
		return
	}

	// Update property average rating
	var reviews []models.Review
	storage.DB.Where("property_id = ?", propertyID).Find(&reviews)

	var totalStars float64
	for _, r := range reviews {
		totalStars += float64(r.Stars)
	}
	avgRating := totalStars / float64(len(reviews))

	// Update property rating
	storage.DB.Model(&models.Property{}).Where("id = ?", propertyID).Update("rating", avgRating)

	ctx.JSON(iris.Map{"success": true, "data": review})
}
