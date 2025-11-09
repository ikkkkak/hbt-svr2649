package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	jwt "github.com/kataras/iris/v12/middleware/jwt"
)

// CreateLandmark creates a new landmark for an organization or individual owner
func CreateLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an organization (optional - can be nil for individual owners)
	var organizationID *uint
	
	// Try to get user's organization directly
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		// User has an organization, use it
		organizationID = &organization.ID
	} else {
		// Try to get through agent
		var agent models.Agent
		if err := storage.DB.Preload("Organization").Where("user_id = ?", userID).First(&agent).Error; err == nil {
			// User is an agent, use their organization
			organizationID = &agent.OrganizationID
		} else {
			// User doesn't have an organization - allow creating as individual owner
			// organizationID will remain nil
			organizationID = nil
		}
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

	// Ensure owner_id is always set (use pointer to uint)
	ownerIDPtr := &userID
	landmark := models.Landmark{
		OrganizationID: organizationID, // Can be nil for individual owners
		OwnerID:        ownerIDPtr,     // ALWAYS set owner_id to track individual owner
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

// GetOrganizationLandmarks gets all landmarks for a user (organization or individual)
func GetOrganizationLandmarks(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an organization
	var organization models.Organization
	hasOrganization := storage.DB.Where("owner_id = ?", userID).First(&organization).Error == nil

	var landmarks []models.Landmark
	query := storage.DB.Preload("Organization").Preload("Owner")
	
	if hasOrganization {
		// Fetch landmarks from organization OR individual landmarks owned by user
		query = query.Where(
			"organization_id = ? OR (organization_id IS NULL AND owner_id = ?)",
			organization.ID, userID,
		)
	} else {
		// User has no organization - fetch only individual landmarks
		query = query.Where("organization_id IS NULL AND owner_id = ?", userID)
	}
	
	if err := query.Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// GetPublicLandmarks gets all verified and published landmarks for public display
func GetPublicLandmarks(ctx iris.Context) {
	// Optional auth: extract userID
	var userID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
				}
			}
		}
	}

	q := storage.DB.Model(&models.Landmark{}).
		Preload("Organization").
		Where("landmarks.is_verified = ? AND landmarks.is_published = ? AND landmarks.status = ?", true, true, "verified")

	if userID > 0 {
		// Exclude landmarks from blocked organizations' owners
		q = q.Joins("LEFT JOIN organizations ON organizations.id = landmarks.organization_id").
			Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagger_id = ? AND uf.status = 'active' AND uf.flagged_user_id = organizations.owner_id)", userID)
	}

	var landmarks []models.Landmark
	if err := q.Order("landmarks.created_at DESC").Find(&landmarks).Error; err != nil {
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

// ReportLandmark allows an authenticated user to report a public landmark
func ReportLandmark(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	// Ensure landmark is public/verified
	var lm models.Landmark
	if err := storage.DB.Where("id = ? AND is_verified = ? AND is_published = ? AND status = ?", id, true, true, "verified").First(&lm).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	var body struct {
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	rep := models.LandmarkReport{
		ReporterID:  userID,
		LandmarkID:  lm.ID,
		Reason:      body.Reason,
		Description: body.Description,
	}
	if err := storage.DB.Create(&rep).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create report"})
		return
	}

	ctx.JSON(iris.Map{"success": true})
}
