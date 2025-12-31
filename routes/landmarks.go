package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jwt "github.com/kataras/iris/v12/middleware/jwt"
)

// CreateLandmark creates a new landmark for an organization or individual owner
func CreateLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// SECURITY: Check if user is a member of an organization
	// If they are, ALL lands MUST belong to that organization
	var organizationID *uint
	
	// First check if user is owner of an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		// User owns an organization, use it
		organizationID = &organization.ID
	} else {
		// Check if user is a member (not owner) of an organization
		var member models.OrganizationMember
		if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
			Preload("Organization").
			First(&member).Error; err == nil {
			// User is a member - MUST assign to organization
			organizationID = &member.OrganizationID
			log.Printf("🔒 Security: User %d is a member of organization %d - land will be assigned to organization", userID, member.OrganizationID)
		} else {
			// User is not a member - allow creating as individual owner
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
		Currency:    input.Currency,
		Status:      "draft",
		IsPublished: false,
		IsVerified:  false,
	}

	// One-time translations for landmark title & description
	titleTranslations := services.TranslateAllLanguages(input.Title)
	descTranslations := services.TranslateAllLanguages(input.Description)
	if b, err := json.Marshal(titleTranslations); err == nil {
		landmark.TitleTranslations = b
	}
	if b, err := json.Marshal(descTranslations); err == nil {
		landmark.DescriptionTranslations = b
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

	// Check if user has an organization (as owner or member)
	var organization models.Organization
	var hasOrganization bool
	
	// First check if user is owner
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		hasOrganization = true
	} else {
		// Check if user is a member using helper function
		org, _, _ := services.GetUserOrganization(userID)
		if org != nil {
			organization = *org
			hasOrganization = true
		}
	}

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

	// Localize landmark fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range landmarks {
		l := &landmarks[i]
		l.Title = utils.ResolveLocalizedText(l.Title, l.TitleTranslations, lang)
		l.Description = utils.ResolveLocalizedText(l.Description, l.DescriptionTranslations, lang)
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// GetPublicLandmarks gets all verified and published landmarks for public display
// Works for both authenticated and unauthenticated users
// Landmarks are rotated per-request to show fresh content (TikTok-like cycling)
// Landmarks with images are always prioritized at the top
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

	var allLandmarks []models.Landmark
	if err := q.Find(&allLandmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	fmt.Printf("✅ Found %d landmarks before rotation\n", len(allLandmarks))

	// Separate landmarks with images from those without
	var withImages []models.Landmark
	var withoutImages []models.Landmark
	
	for _, landmark := range allLandmarks {
		// Check if landmark has images (Images is stored as datatypes.JSON which is []byte)
		hasImages := false
		if len(landmark.Images) > 0 {
			var images []string
			if err := json.Unmarshal(landmark.Images, &images); err == nil {
				// Check if images array contains non-empty strings
				for _, img := range images {
					if strings.TrimSpace(img) != "" {
						hasImages = true
						break
					}
				}
			}
		}
		
		if hasImages {
			withImages = append(withImages, landmark)
		} else {
			withoutImages = append(withoutImages, landmark)
		}
	}

	fmt.Printf("📸 Landmarks with images: %d, without images: %d\n", len(withImages), len(withoutImages))

	// Apply time-based rotation for TikTok-like cycling
	// Rotation changes per-request with high precision + random component for maximum variety
	// This ensures users see different landmarks on each visit/reload
	now := time.Now()
	// High-precision rotation: includes seconds, nanoseconds, Unix timestamp, and random component
	// Random component ensures different results even if requests happen at exact same time
	rand.Seed(now.UnixNano()) // Seed random with current nanosecond for true randomness
	randomComponent := int64(rand.Intn(10000)) // Random 0-9999
	rotationSeed := int64(now.Second()) + 
		int64(now.Minute())*60 + 
		int64(now.Hour())*3600 + 
		int64(now.Day())*86400 + 
		int64(now.Month())*2678400 + 
		(now.UnixNano() % 1000000) + // Use nanoseconds for microsecond-level variation
		randomComponent // Add random component for guaranteed variation
	
	// Rotate both lists with different offsets for maximum variety
	// Uses multiple rotation passes for better distribution
	rotateLandmarks := func(landmarks []models.Landmark, seed int64) []models.Landmark {
		if len(landmarks) == 0 {
			return landmarks
		}
		if len(landmarks) == 1 {
			return landmarks
		}
		
		// Calculate offset with multiple components for better distribution
		offset1 := int(seed % int64(len(landmarks)))
		offset2 := int((seed * 7) % int64(len(landmarks))) // Different multiplier for variety
		
		// Use the larger offset, but ensure it's not 0
		offset := offset1
		if offset2 > offset1 {
			offset = offset2
		}
		if offset == 0 {
			offset = int((seed * 13) % int64(len(landmarks)-1)) + 1 // Force non-zero with different multiplier
		}
		
		// Perform rotation
		rotated := make([]models.Landmark, len(landmarks))
		copy(rotated, landmarks[offset:])
		copy(rotated[len(landmarks)-offset:], landmarks[:offset])
		return rotated
	}

	withImages = rotateLandmarks(withImages, rotationSeed)
	withoutImages = rotateLandmarks(withoutImages, rotationSeed+3000) // Different offset for variety

	// Combine: landmarks with images first, then without
	var landmarks []models.Landmark
	landmarks = append(landmarks, withImages...)
	landmarks = append(landmarks, withoutImages...)

	fmt.Printf("✅ Returning %d landmarks (rotated, images prioritized)\n", len(landmarks))

	// Localize landmark fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range landmarks {
		l := &landmarks[i]
		l.Title = utils.ResolveLocalizedText(l.Title, l.TitleTranslations, lang)
		l.Description = utils.ResolveLocalizedText(l.Description, l.DescriptionTranslations, lang)
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
	titleChanged := false
	descChanged := false
	if input.Title != "" {
		landmark.Title = input.Title
		titleChanged = true
	}
	if input.Description != "" {
		landmark.Description = input.Description
		descChanged = true
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

	// Update translations if title or description changed
	if titleChanged || descChanged {
		if titleChanged {
			titleTranslations := services.TranslateAllLanguages(landmark.Title)
			if b, err := json.Marshal(titleTranslations); err == nil {
				landmark.TitleTranslations = b
			}
		}
		if descChanged {
			descTranslations := services.TranslateAllLanguages(landmark.Description)
			if b, err := json.Marshal(descTranslations); err == nil {
				landmark.DescriptionTranslations = b
			}
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
