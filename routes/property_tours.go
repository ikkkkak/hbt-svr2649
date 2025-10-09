package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
)

// BookPropertyTour books a tour for a property
func BookPropertyTour(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Check if property exists and is published
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND status = ? AND is_published = ?", propertyID, "published", true).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found or not available for tours"})
		return
	}

	var input struct {
		TourDate      time.Time `json:"tour_date" validate:"required"`
		TourTime      string    `json:"tour_time" validate:"required"`
		Duration      int       `json:"duration"`
		TourType      string    `json:"tour_type"`
		CustomerNotes string    `json:"customer_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate input
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	// Check if tour date is in the future
	if input.TourDate.Before(time.Now()) {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Tour date must be in the future"})
		return
	}

	// Set default values
	if input.Duration == 0 {
		input.Duration = 60 // Default 1 hour
	}
	if input.TourType == "" {
		input.TourType = "in_person"
	}

	// Create tour booking
	tour := models.PropertyTour{
		PropertySaleID: uint(propertyID),
		CustomerID:     userID,
		TourDate:       input.TourDate,
		TourTime:       input.TourTime,
		Duration:       input.Duration,
		TourType:       input.TourType,
		Status:         "pending",
		CustomerNotes:  input.CustomerNotes,
	}

	if err := storage.DB.Create(&tour).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to book tour"})
		return
	}

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message": "Tour booked successfully",
		"tour":    tour,
	})
}

// GetUserTourBookings gets all tour bookings for a user
func GetUserTourBookings(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	var tours []models.PropertyTour
	if err := storage.DB.Preload("PropertySale.Organization").Preload("PropertySale.Agent.User").Preload("Customer").Where("customer_id = ?", userID).Find(&tours).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch tour bookings"})
		return
	}

	ctx.JSON(iris.Map{"tours": tours})
}

// GetPropertyTourBookings gets all tour bookings for a property (organization/agent only)
func GetPropertyTourBookings(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Check if user has access to this property
	var property models.PropertySale
	if err := storage.DB.Preload("Organization").Where("id = ?", propertyID).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check if user is the organization owner or assigned agent
	if property.Organization.OwnerID != userID && (property.AgentID == nil || *property.AgentID != userID) {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Access denied"})
		return
	}

	var tours []models.PropertyTour
	if err := storage.DB.Preload("Customer").Where("property_sale_id = ?", propertyID).Find(&tours).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch tour bookings"})
		return
	}

	ctx.JSON(iris.Map{"tours": tours})
}

// UpdateTourStatus updates a tour's status
func UpdateTourStatus(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	tourID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var tour models.PropertyTour
	if err := storage.DB.Preload("PropertySale.Organization").Preload("PropertySale.Agent").First(&tour, tourID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Tour not found"})
		return
	}

	// Check if user has access to update this tour
	canUpdate := false
	if tour.CustomerID == userID {
		// Customer can cancel their own tour
		canUpdate = true
	} else if tour.PropertySale.Organization.OwnerID == userID {
		// Organization owner can update any tour for their properties
		canUpdate = true
	} else if tour.PropertySale.AgentID != nil && *tour.PropertySale.AgentID == userID {
		// Assigned agent can update tours for their assigned properties
		canUpdate = true
	}

	if !canUpdate {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Access denied"})
		return
	}

	var input struct {
		Status     string `json:"status" validate:"required,oneof=pending confirmed completed cancelled no_show"`
		AgentNotes string `json:"agent_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate input
	if err := utils.Validate.Struct(input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Validation failed", "details": err.Error()})
		return
	}

	tour.Status = input.Status
	if input.AgentNotes != "" {
		tour.AgentNotes = input.AgentNotes
	}

	if err := storage.DB.Save(&tour).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update tour status"})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Tour status updated successfully",
		"tour":    tour,
	})
}

// GetOrganizationTourBookings gets all tour bookings for an organization
func GetOrganizationTourBookings(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	var tours []models.PropertyTour
	if err := storage.DB.Preload("PropertySale").Preload("Customer").Joins("JOIN property_sales ON property_tours.property_sale_id = property_sales.id").Where("property_sales.organization_id = ?", organization.ID).Find(&tours).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch tour bookings"})
		return
	}
	// Build lightweight response with user name/initials
	resp := make([]iris.Map, 0, len(tours))
	for _, t := range tours {
		fullName := ""
		if t.Customer.FirstName != "" || t.Customer.LastName != "" {
			fullName = (t.Customer.FirstName + " " + t.Customer.LastName)
		}
		resp = append(resp, iris.Map{
			"id":        t.ID,
			"tour_date": t.TourDate,
			"tour_time": t.TourTime,
			"duration":  t.Duration,
			"tour_type": t.TourType,
			"status":    t.Status,
			"customer": iris.Map{
				"id":          t.CustomerID,
				"firstName":   t.Customer.FirstName,
				"lastName":    t.Customer.LastName,
				"email":       t.Customer.Email,
				"avatarURL":   t.Customer.AvatarURL,
				"displayName": fullName,
			},
			"property_sale": iris.Map{
				"id":    t.PropertySaleID,
				"title": t.PropertySale.Title,
			},
		})
	}
	ctx.JSON(iris.Map{"tours": resp})
}

// GetAgentTourBookings gets all tour bookings for an agent
func GetAgentTourBookings(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user is an agent
	var agent models.Agent
	if err := storage.DB.Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must be an agent"})
		return
	}

	var tours []models.PropertyTour
	if err := storage.DB.Preload("PropertySale").Preload("Customer").Joins("JOIN property_sales ON property_tours.property_sale_id = property_sales.id").Where("property_sales.agent_id = ?", agent.ID).Find(&tours).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch tour bookings"})
		return
	}

	ctx.JSON(iris.Map{"tours": tours})
}

// CancelTour cancels a tour booking
func CancelTour(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	tourID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var tour models.PropertyTour
	if err := storage.DB.First(&tour, tourID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Tour not found"})
		return
	}

	// Check if user can cancel this tour
	if tour.CustomerID != userID {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Access denied"})
		return
	}

	// Check if tour can be cancelled
	if tour.Status == "completed" || tour.Status == "cancelled" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Tour cannot be cancelled"})
		return
	}

	tour.Status = "cancelled"
	if err := storage.DB.Save(&tour).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to cancel tour"})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Tour cancelled successfully",
		"tour":    tour,
	})
}
