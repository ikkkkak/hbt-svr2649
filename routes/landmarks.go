package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
)

// CreateLandmark creates a new landmark for an organization
func CreateLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Get user's agent record to find organization
	var agent models.Agent
	if err := storage.DB.Preload("Organization").Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User must be an agent to create landmarks"})
		return
	}

	var input struct {
		Title          string   `json:"title"`
		Description    string   `json:"description"`
		Images         []string `json:"images"`
		Area           float64  `json:"area"`
		AreaUnit       string   `json:"area_unit"`
		LandType       string   `json:"land_type"`
		Zoning         string   `json:"zoning"`
		Utilities      []string `json:"utilities"`
		Point1Lat      float64  `json:"point1_lat"`
		Point1Lng      float64  `json:"point1_lng"`
		Point2Lat      float64  `json:"point2_lat"`
		Point2Lng      float64  `json:"point2_lng"`
		Point3Lat      float64  `json:"point3_lat"`
		Point3Lng      float64  `json:"point3_lng"`
		Point4Lat      float64  `json:"point4_lat"`
		Point4Lng      float64  `json:"point4_lng"`
		PropertyPapers []string `json:"property_papers"`
		// New optional fields
		District        string   `json:"district"`
		Region          string   `json:"region"`
		PlotNumber      string   `json:"plot_number"`
		ElevationMeters float64  `json:"elevation_m"`
		Sides           []string `json:"sides"`
		Price           float64  `json:"price"`
		Currency        string   `json:"currency"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate required fields
	if input.Title == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Title is required"})
		return
	}

	if input.Point1Lat == 0 || input.Point1Lng == 0 || input.Point2Lat == 0 || input.Point2Lng == 0 ||
		input.Point3Lat == 0 || input.Point3Lng == 0 || input.Point4Lat == 0 || input.Point4Lng == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "All coordinate points are required"})
		return
	}

	// Validate coordinates are within reasonable bounds
	if !isValidCoordinate(input.Point1Lat, input.Point1Lng) ||
		!isValidCoordinate(input.Point2Lat, input.Point2Lng) ||
		!isValidCoordinate(input.Point3Lat, input.Point3Lng) ||
		!isValidCoordinate(input.Point4Lat, input.Point4Lng) {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid coordinates"})
		return
	}

	// Convert arrays to JSON
	imagesJSON, _ := json.Marshal(input.Images)
	utilitiesJSON, _ := json.Marshal(input.Utilities)
	papersJSON, _ := json.Marshal(input.PropertyPapers)
	sidesJSON, _ := json.Marshal(input.Sides)

	landmark := models.Landmark{
		OrganizationID: agent.OrganizationID,
		Title:          input.Title,
		Description:    input.Description,
		Images:         imagesJSON,
		Area:           input.Area,
		AreaUnit:       input.AreaUnit,
		LandType:       input.LandType,
		Zoning:         input.Zoning,
		Utilities:      utilitiesJSON,
		Point1Lat:      input.Point1Lat,
		Point1Lng:      input.Point1Lng,
		Point2Lat:      input.Point2Lat,
		Point2Lng:      input.Point2Lng,
		Point3Lat:      input.Point3Lat,
		Point3Lng:      input.Point3Lng,
		Point4Lat:      input.Point4Lat,
		Point4Lng:      input.Point4Lng,
		PropertyPapers: papersJSON,
		// New fields
		District:        input.District,
		Region:          input.Region,
		PlotNumber:      input.PlotNumber,
		ElevationMeters: input.ElevationMeters,
		Sides:           sidesJSON,
		Price:           input.Price,
		Currency:        input.Currency,
		Status:          "draft",
		IsPublished:     false,
		IsVerified:      false,
	}

	if err := storage.DB.Create(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create landmark"})
		return
	}

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(landmark)
}

// GetOrganizationLandmarks gets all landmarks for a user's organization
func GetOrganizationLandmarks(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Get user's agent record to find organization
	var agent models.Agent
	if err := storage.DB.Preload("Organization").Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User must be an agent to view landmarks"})
		return
	}

	var landmarks []models.Landmark
	if err := storage.DB.Where("organization_id = ?", agent.OrganizationID).Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// GetPublicLandmarks gets all verified and published landmarks for public display
func GetPublicLandmarks(ctx iris.Context) {
	var landmarks []models.Landmark
	if err := storage.DB.Preload("Organization").Where("is_verified = ? AND is_published = ? AND status = ?", true, true, "verified").Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// UpdateLandmark updates an existing landmark
func UpdateLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Get user's agent record to find organization
	var agent models.Agent
	if err := storage.DB.Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User must be an agent to update landmarks"})
		return
	}

	// Check if landmark exists and belongs to user's organization
	var landmark models.Landmark
	if err := storage.DB.Where("id = ? AND organization_id = ?", landmarkID, agent.OrganizationID).First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	var input struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Point1Lat   float64 `json:"point1_lat"`
		Point1Lng   float64 `json:"point1_lng"`
		Point2Lat   float64 `json:"point2_lat"`
		Point2Lng   float64 `json:"point2_lng"`
		Point3Lat   float64 `json:"point3_lat"`
		Point3Lng   float64 `json:"point3_lng"`
		Point4Lat   float64 `json:"point4_lat"`
		Point4Lng   float64 `json:"point4_lng"`
		Status      string  `json:"status"`
		// New optional fields
		District        string   `json:"district"`
		Region          string   `json:"region"`
		PlotNumber      string   `json:"plot_number"`
		ElevationMeters float64  `json:"elevation_m"`
		Sides           []string `json:"sides"`
		Price           float64  `json:"price"`
		Currency        string   `json:"currency"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Update fields if provided
	if input.Title != "" {
		landmark.Title = input.Title
	}
	if input.Description != "" {
		landmark.Description = input.Description
	}
	if input.Status != "" {
		landmark.Status = input.Status
	}

	// Location updates
	if input.Point1Lat != 0 && input.Point1Lng != 0 {
		if !isValidCoordinate(input.Point1Lat, input.Point1Lng) {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid coordinates"})
			return
		}
		landmark.Point1Lat = input.Point1Lat
		landmark.Point1Lng = input.Point1Lng
	}
	if input.Point2Lat != 0 && input.Point2Lng != 0 {
		if !isValidCoordinate(input.Point2Lat, input.Point2Lng) {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid coordinates"})
			return
		}
		landmark.Point2Lat = input.Point2Lat
		landmark.Point2Lng = input.Point2Lng
	}
	if input.Point3Lat != 0 && input.Point3Lng != 0 {
		if !isValidCoordinate(input.Point3Lat, input.Point3Lng) {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid coordinates"})
			return
		}
		landmark.Point3Lat = input.Point3Lat
		landmark.Point3Lng = input.Point3Lng
	}
	if input.Point4Lat != 0 && input.Point4Lng != 0 {
		if !isValidCoordinate(input.Point4Lat, input.Point4Lng) {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid coordinates"})
			return
		}
		landmark.Point4Lat = input.Point4Lat
		landmark.Point4Lng = input.Point4Lng
	}

	// New metadata updates
	if input.District != "" {
		landmark.District = input.District
	}
	if input.Region != "" {
		landmark.Region = input.Region
	}
	if input.PlotNumber != "" {
		landmark.PlotNumber = input.PlotNumber
	}
	if input.ElevationMeters != 0 {
		landmark.ElevationMeters = input.ElevationMeters
	}
	if input.Price != 0 {
		landmark.Price = input.Price
	}
	if input.Currency != "" {
		landmark.Currency = input.Currency
	}
	if input.Sides != nil {
		if b, err := json.Marshal(input.Sides); err == nil {
			landmark.Sides = b
		}
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark"})
		return
	}

	ctx.JSON(landmark)
}

// DeleteLandmark soft deletes a landmark
func DeleteLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Get user's agent record to find organization
	var agent models.Agent
	if err := storage.DB.Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User must be an agent to delete landmarks"})
		return
	}

	// Check if landmark exists and belongs to user's organization
	var landmark models.Landmark
	if err := storage.DB.Where("id = ? AND organization_id = ?", landmarkID, agent.OrganizationID).First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Soft delete by setting status to inactive
	landmark.Status = "inactive"
	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to delete landmark"})
		return
	}

	ctx.JSON(iris.Map{"message": "Landmark deleted successfully"})
}

// SubmitLandmarkForVerification submits a landmark for admin verification
func SubmitLandmarkForVerification(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Get user's agent record
	var agent models.Agent
	if err := storage.DB.Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User must be an agent"})
		return
	}

	// Check if landmark exists and belongs to user's organization
	var landmark models.Landmark
	if err := storage.DB.Where("id = ? AND organization_id = ?", landmarkID, agent.OrganizationID).First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Update status to pending verification
	landmark.Status = "pending_verification"
	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to submit landmark for verification"})
		return
	}

	ctx.JSON(iris.Map{"message": "Landmark submitted for verification"})
}

// VerifyLandmark verifies a landmark (admin only)
func VerifyLandmark(ctx iris.Context) {
	fmt.Println("VerifyLandmark called")

	// Get userID from context with proper error handling
	userIDInterface := ctx.Values().Get("userID")
	fmt.Printf("userIDInterface: %v, type: %T\n", userIDInterface, userIDInterface)

	if userIDInterface == nil {
		fmt.Println("userID is nil")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User not authenticated"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		fmt.Printf("Failed to convert userID to uint, got type: %T\n", userIDInterface)
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	fmt.Printf("userID: %d\n", userID)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	fmt.Printf("landmarkID: %d\n", landmarkID)

	var input struct {
		IsVerified        bool   `json:"is_verified"`
		VerificationNotes string `json:"verification_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Get landmark
	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Update verification status
	landmark.IsVerified = input.IsVerified
	landmark.VerificationNotes = input.VerificationNotes
	landmark.VerifiedBy = &userID

	if input.IsVerified {
		now := time.Now()
		landmark.VerifiedAt = &now
		landmark.Status = "verified"
		landmark.IsPublished = true
	} else {
		landmark.Status = "rejected"
		landmark.IsPublished = false
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark verification"})
		return
	}

	ctx.JSON(iris.Map{"message": "Landmark verification updated"})
}

// GetPendingLandmarks gets landmarks pending verification (admin only)
func GetPendingLandmarks(ctx iris.Context) {
	fmt.Println("GetPendingLandmarks called")

	// First, let's see ALL landmarks to debug
	var allLandmarks []models.Landmark
	if err := storage.DB.Preload("Organization").Find(&allLandmarks).Error; err != nil {
		fmt.Printf("Error fetching all landmarks: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	fmt.Printf("Found %d total landmarks\n", len(allLandmarks))
	for i, landmark := range allLandmarks {
		fmt.Printf("Landmark %d: ID=%d, Title=%s, Status=%s, IsVerified=%t\n",
			i+1, landmark.ID, landmark.Title, landmark.Status, landmark.IsVerified)
	}

	// Now get pending ones
	var landmarks []models.Landmark
	if err := storage.DB.Preload("Organization").Where("status = ?", "pending_verification").Find(&landmarks).Error; err != nil {
		fmt.Printf("Error fetching pending landmarks: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch pending landmarks"})
		return
	}

	fmt.Printf("Found %d pending landmarks\n", len(landmarks))
	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// AdminGetAllLandmarks gets all landmarks for admin review
func AdminGetAllLandmarks(ctx iris.Context) {
	var landmarks []models.Landmark
	if err := storage.DB.Preload("Organization").Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// Helper function to validate coordinates
func isValidCoordinate(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}
