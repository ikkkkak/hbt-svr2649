package routes

import (
	"fmt"
	"strconv"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"

	"github.com/kataras/iris/v12"
)

// Availability Management Routes

type AvailabilityInput struct {
	PropertyID   uint      `json:"propertyID" validate:"required"`
	Date         time.Time `json:"date" validate:"required"`
	IsAvailable  bool      `json:"isAvailable"`
	Price        float64   `json:"price" validate:"required,min=0"`
	MinStay      int       `json:"minStay" validate:"min=1"`
	MaxStay      int       `json:"maxStay" validate:"min=0"`
	CheckInTime  string    `json:"checkInTime"`
	CheckOutTime string    `json:"checkOutTime"`
	Notes        string    `json:"notes"`
}

type BulkAvailabilityInput struct {
	PropertyID   uint      `json:"propertyID" validate:"required"`
	StartDate    time.Time `json:"startDate" validate:"required"`
	EndDate      time.Time `json:"endDate" validate:"required"`
	IsAvailable  bool      `json:"isAvailable"`
	Price        float64   `json:"price" validate:"required,min=0"`
	MinStay      int       `json:"minStay" validate:"min=1"`
	MaxStay      int       `json:"maxStay" validate:"min=0"`
	CheckInTime  string    `json:"checkInTime"`
	CheckOutTime string    `json:"checkOutTime"`
	Notes        string    `json:"notes"`
}

type PricingInput struct {
	PropertyID      uint    `json:"propertyID" validate:"required"`
	BasePrice       float64 `json:"basePrice" validate:"required,min=0"`
	WeekendPrice    float64 `json:"weekendPrice" validate:"min=0"`
	WeeklyPrice     float64 `json:"weeklyPrice" validate:"min=0"`
	MonthlyPrice    float64 `json:"monthlyPrice" validate:"min=0"`
	CleaningFee     float64 `json:"cleaningFee" validate:"min=0"`
	ServiceFee      float64 `json:"serviceFee" validate:"min=0"`
	SecurityDeposit float64 `json:"securityDeposit" validate:"min=0"`
	Currency        string  `json:"currency"`
}

type DiscountInput struct {
	PropertyID uint      `json:"propertyID" validate:"required"`
	Name       string    `json:"name" validate:"required"`
	Type       string    `json:"type" validate:"required,oneof=percentage fixed early_bird last_minute"`
	Value      float64   `json:"value" validate:"required,min=0"`
	MinStay    int       `json:"minStay" validate:"min=1"`
	MaxStay    int       `json:"maxStay" validate:"min=0"`
	StartDate  time.Time `json:"startDate"`
	EndDate    time.Time `json:"endDate"`
	IsActive   bool      `json:"isActive"`
}

type BlockInput struct {
	PropertyID    uint      `json:"propertyID" validate:"required"`
	StartDate     time.Time `json:"startDate" validate:"required"`
	EndDate       time.Time `json:"endDate" validate:"required"`
	Reason        string    `json:"reason"`
	IsMaintenance bool      `json:"isMaintenance"`
}

// Get property availability for a date range
func GetPropertyAvailability(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("propertyID")
	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	startDateStr := ctx.URLParam("startDate")
	endDateStr := ctx.URLParam("endDate")

	if startDateStr == "" || endDateStr == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Start date and end date are required"})
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid start date format"})
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid end date format"})
		return
	}

	var availability []models.PropertyAvailability
	result := storage.DB.Where("property_id = ? AND date >= ? AND date <= ?",
		propertyID, startDate, endDate).Order("date ASC").Find(&availability)

	if result.Error != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch availability"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    availability,
	})
}

// Set single date availability
func SetPropertyAvailability(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var input AvailabilityInput

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Verify property ownership
	var property models.Property
	if err := storage.DB.Where("id = ? AND host_id = ?", input.PropertyID, userID).First(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "Property not found or access denied"})
		return
	}

	// Check if availability already exists for this date
	var existingAvailability models.PropertyAvailability
	result := storage.DB.Where("property_id = ? AND date = ?", input.PropertyID, input.Date).First(&existingAvailability)

	if result.Error == nil {
		// Update existing availability
		existingAvailability.IsAvailable = input.IsAvailable
		existingAvailability.Price = input.Price
		existingAvailability.MinStay = input.MinStay
		existingAvailability.MaxStay = input.MaxStay
		existingAvailability.CheckInTime = input.CheckInTime
		existingAvailability.CheckOutTime = input.CheckOutTime
		existingAvailability.Notes = input.Notes

		if err := storage.DB.Save(&existingAvailability).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"message": "Failed to update availability"})
			return
		}

		ctx.JSON(iris.Map{
			"success": true,
			"message": "Availability updated successfully",
			"data":    existingAvailability,
		})
	} else {
		// Create new availability
		availability := models.PropertyAvailability{
			PropertyID:   input.PropertyID,
			Date:         input.Date,
			IsAvailable:  input.IsAvailable,
			Price:        input.Price,
			MinStay:      input.MinStay,
			MaxStay:      input.MaxStay,
			CheckInTime:  input.CheckInTime,
			CheckOutTime: input.CheckOutTime,
			Notes:        input.Notes,
		}

		if err := storage.DB.Create(&availability).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"message": "Failed to create availability"})
			return
		}

		ctx.JSON(iris.Map{
			"success": true,
			"message": "Availability created successfully",
			"data":    availability,
		})
	}
}

// Set bulk availability for date range
func SetBulkPropertyAvailability(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var input BulkAvailabilityInput

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Verify property ownership
	var property models.Property
	if err := storage.DB.Where("id = ? AND host_id = ?", input.PropertyID, userID).First(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "Property not found or access denied"})
		return
	}

	// Generate dates between start and end
	var availabilities []models.PropertyAvailability
	currentDate := input.StartDate
	for currentDate.Before(input.EndDate) || currentDate.Equal(input.EndDate) {
		availabilities = append(availabilities, models.PropertyAvailability{
			PropertyID:   input.PropertyID,
			Date:         currentDate,
			IsAvailable:  input.IsAvailable,
			Price:        input.Price,
			MinStay:      input.MinStay,
			MaxStay:      input.MaxStay,
			CheckInTime:  input.CheckInTime,
			CheckOutTime: input.CheckOutTime,
			Notes:        input.Notes,
		})
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Use transaction to ensure atomicity
	tx := storage.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Delete existing availability for the date range
	if err := tx.Where("property_id = ? AND date >= ? AND date <= ?",
		input.PropertyID, input.StartDate, input.EndDate).Delete(&models.PropertyAvailability{}).Error; err != nil {
		tx.Rollback()
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to clear existing availability"})
		return
	}

	// Create new availability records
	if err := tx.Create(&availabilities).Error; err != nil {
		tx.Rollback()
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to create bulk availability"})
		return
	}

	tx.Commit()

	ctx.JSON(iris.Map{
		"success": true,
		"message": fmt.Sprintf("Bulk availability set for %d days", len(availabilities)),
		"data":    availabilities,
	})
}

// Get property pricing
func GetPropertyPricing(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("propertyID")
	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	var pricing models.PropertyPricing
	result := storage.DB.Where("property_id = ?", propertyID).First(&pricing)

	if result.Error != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Pricing not found"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    pricing,
	})
}

// Set property pricing
func SetPropertyPricing(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var input PricingInput

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Verify property ownership
	var property models.Property
	if err := storage.DB.Where("id = ? AND host_id = ?", input.PropertyID, userID).First(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "Property not found or access denied"})
		return
	}

	// Check if pricing already exists
	var existingPricing models.PropertyPricing
	result := storage.DB.Where("property_id = ?", input.PropertyID).First(&existingPricing)

	if result.Error == nil {
		// Update existing pricing
		existingPricing.BasePrice = input.BasePrice
		existingPricing.WeekendPrice = input.WeekendPrice
		existingPricing.WeeklyPrice = input.WeeklyPrice
		existingPricing.MonthlyPrice = input.MonthlyPrice
		existingPricing.CleaningFee = input.CleaningFee
		existingPricing.ServiceFee = input.ServiceFee
		existingPricing.SecurityDeposit = input.SecurityDeposit
		existingPricing.Currency = input.Currency

		if err := storage.DB.Save(&existingPricing).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"message": "Failed to update pricing"})
			return
		}

		ctx.JSON(iris.Map{
			"success": true,
			"message": "Pricing updated successfully",
			"data":    existingPricing,
		})
	} else {
		// Create new pricing
		pricing := models.PropertyPricing{
			PropertyID:      input.PropertyID,
			BasePrice:       input.BasePrice,
			WeekendPrice:    input.WeekendPrice,
			WeeklyPrice:     input.WeeklyPrice,
			MonthlyPrice:    input.MonthlyPrice,
			CleaningFee:     input.CleaningFee,
			ServiceFee:      input.ServiceFee,
			SecurityDeposit: input.SecurityDeposit,
			Currency:        input.Currency,
		}

		if err := storage.DB.Create(&pricing).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"message": "Failed to create pricing"})
			return
		}

		ctx.JSON(iris.Map{
			"success": true,
			"message": "Pricing created successfully",
			"data":    pricing,
		})
	}
}

// Get property discounts
func GetPropertyDiscounts(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("propertyID")
	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	var discounts []models.PropertyDiscount
	result := storage.DB.Where("property_id = ? AND is_active = ?", propertyID, true).Order("created_at DESC").Find(&discounts)

	if result.Error != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch discounts"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    discounts,
	})
}

// Create property discount
func CreatePropertyDiscount(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var input DiscountInput

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Verify property ownership
	var property models.Property
	if err := storage.DB.Where("id = ? AND host_id = ?", input.PropertyID, userID).First(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "Property not found or access denied"})
		return
	}

	discount := models.PropertyDiscount{
		PropertyID: input.PropertyID,
		Name:       input.Name,
		Type:       input.Type,
		Value:      input.Value,
		MinStay:    input.MinStay,
		MaxStay:    input.MaxStay,
		StartDate:  input.StartDate,
		EndDate:    input.EndDate,
		IsActive:   input.IsActive,
	}

	if err := storage.DB.Create(&discount).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to create discount"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Discount created successfully",
		"data":    discount,
	})
}

// Block property dates
func BlockPropertyDates(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var input BlockInput

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Verify property ownership
	var property models.Property
	if err := storage.DB.Where("id = ? AND host_id = ?", input.PropertyID, userID).First(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "Property not found or access denied"})
		return
	}

	block := models.PropertyBlock{
		PropertyID:    input.PropertyID,
		StartDate:     input.StartDate,
		EndDate:       input.EndDate,
		Reason:        input.Reason,
		IsMaintenance: input.IsMaintenance,
	}

	if err := storage.DB.Create(&block).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to block dates"})
		return
	}

	// Also update availability for blocked dates
	currentDate := input.StartDate
	for currentDate.Before(input.EndDate) || currentDate.Equal(input.EndDate) {
		var availability models.PropertyAvailability
		result := storage.DB.Where("property_id = ? AND date = ?", input.PropertyID, currentDate).First(&availability)

		if result.Error == nil {
			availability.IsAvailable = false
			availability.Notes = fmt.Sprintf("Blocked: %s", input.Reason)
			storage.DB.Save(&availability)
		} else {
			// Create new availability record as blocked
			availability = models.PropertyAvailability{
				PropertyID:  input.PropertyID,
				Date:        currentDate,
				IsAvailable: false,
				Price:       0,
				MinStay:     1,
				MaxStay:     0,
				Notes:       fmt.Sprintf("Blocked: %s", input.Reason),
			}
			storage.DB.Create(&availability)
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Dates blocked successfully",
		"data":    block,
	})
}

// Get property blocks
func GetPropertyBlocks(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("propertyID")
	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	var blocks []models.PropertyBlock
	result := storage.DB.Where("property_id = ?", propertyID).Order("start_date ASC").Find(&blocks)

	if result.Error != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch blocks"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    blocks,
	})
}

// Calculate booking price with discounts
func CalculateBookingPrice(ctx iris.Context) {
	var input struct {
		PropertyID uint      `json:"propertyID" validate:"required"`
		StartDate  time.Time `json:"startDate" validate:"required"`
		EndDate    time.Time `json:"endDate" validate:"required"`
		Guests     int       `json:"guests" validate:"required,min=1"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Get property pricing
	var pricing models.PropertyPricing
	if err := storage.DB.Where("property_id = ?", input.PropertyID).First(&pricing).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property pricing not found"})
		return
	}

	// Calculate base price
	nights := int(input.EndDate.Sub(input.StartDate).Hours() / 24)
	basePrice := pricing.BasePrice * float64(nights)

	// Apply weekend pricing
	weekendPrice := 0.0
	currentDate := input.StartDate
	for i := 0; i < nights; i++ {
		if currentDate.Weekday() == time.Saturday || currentDate.Weekday() == time.Sunday {
			if pricing.WeekendPrice > 0 {
				weekendPrice += pricing.WeekendPrice - pricing.BasePrice
			}
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Apply weekly/monthly discounts
	var discountAmount float64
	if nights >= 7 && pricing.WeeklyPrice > 0 {
		weeklyNights := nights / 7
		remainingNights := nights % 7
		discountAmount = (pricing.BasePrice - pricing.WeeklyPrice) * float64(weeklyNights*7)
		basePrice = pricing.WeeklyPrice*float64(weeklyNights*7) + pricing.BasePrice*float64(remainingNights)
	} else if nights >= 30 && pricing.MonthlyPrice > 0 {
		monthlyNights := nights / 30
		remainingNights := nights % 30
		discountAmount = (pricing.BasePrice - pricing.MonthlyPrice) * float64(monthlyNights*30)
		basePrice = pricing.MonthlyPrice*float64(monthlyNights*30) + pricing.BasePrice*float64(remainingNights)
	}

	// Apply additional discounts
	var discounts []models.PropertyDiscount
	storage.DB.Where("property_id = ? AND is_active = ? AND start_date <= ? AND end_date >= ?",
		input.PropertyID, true, input.StartDate, input.EndDate).Find(&discounts)

	var appliedDiscounts []map[string]interface{}
	for _, discount := range discounts {
		var discountValue float64
		if discount.Type == "percentage" {
			discountValue = basePrice * (discount.Value / 100)
		} else if discount.Type == "fixed" {
			discountValue = discount.Value
		} else if discount.Type == "early_bird" {
			daysUntilCheckIn := int(input.StartDate.Sub(time.Now()).Hours() / 24)
			if daysUntilCheckIn >= 30 {
				discountValue = basePrice * (discount.Value / 100)
			}
		} else if discount.Type == "last_minute" {
			daysUntilCheckIn := int(input.StartDate.Sub(time.Now()).Hours() / 24)
			if daysUntilCheckIn <= 7 {
				discountValue = basePrice * (discount.Value / 100)
			}
		}

		if discountValue > 0 {
			discountAmount += discountValue
			appliedDiscounts = append(appliedDiscounts, map[string]interface{}{
				"name":           discount.Name,
				"type":           discount.Type,
				"value":          discount.Value,
				"discountAmount": discountValue,
			})
		}
	}

	totalPrice := basePrice + weekendPrice - discountAmount + pricing.CleaningFee + pricing.ServiceFee

	ctx.JSON(iris.Map{
		"success": true,
		"data": map[string]interface{}{
			"basePrice":        basePrice,
			"weekendPrice":     weekendPrice,
			"cleaningFee":      pricing.CleaningFee,
			"serviceFee":       pricing.ServiceFee,
			"securityDeposit":  pricing.SecurityDeposit,
			"discountAmount":   discountAmount,
			"appliedDiscounts": appliedDiscounts,
			"totalPrice":       totalPrice,
			"nights":           nights,
			"currency":         pricing.Currency,
		},
	})
}
