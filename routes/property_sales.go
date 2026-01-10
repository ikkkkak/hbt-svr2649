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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jwt "github.com/kataras/iris/v12/middleware/jwt"
)

<<<<<<< HEAD
=======
// triggerNewPropertyNotification sends notifications when a property is published
func triggerNewPropertyNotification(property models.PropertySale) {
	// Get first image URL for rich notification
	var imageURL string
	if len(property.Images) > 0 && property.Images[0] != "" {
		imageURL = property.Images[0]
	}

	// Get city/zone names
	cityName := property.City
	zoneName := ""
	if property.ZoneRef != nil {
		zoneName = property.ZoneRef.Name
	} else if property.ZoneID != nil {
		var zone models.Zone
		if err := storage.DB.First(&zone, *property.ZoneID).Error; err == nil {
			zoneName = zone.Name
		}
	}

	// Try to send targeted notifications first
	err := services.NotificationServiceInstance.SendNewPropertyNotification(
		property.ID,
		property.Title,
		property.CityID,
		cityName,
		property.ZoneID,
		zoneName,
		property.Bedrooms,
		property.Bathrooms,
		property.SquareFootage,
		imageURL,
	)

	// If targeted notifications fail or no users match, send generic notifications
	// This ensures we always notify some users even if personalization fails
	if err != nil {
		log.Printf("⚠️ Targeted notifications failed, sending generic notifications: %v", err)
		
		// Get all users with notifications enabled (fallback)
		var users []models.User
		if err := storage.DB.Where("allows_notifications = ?", true).
			Limit(100). // Limit to avoid spamming too many users
			Find(&users).Error; err == nil {
			var userIDs []uint
			for _, user := range users {
				userIDs = append(userIDs, user.ID)
			}
			
			services.NotificationServiceInstance.SendGenericPropertyNotification(
				property.ID,
				property.Title,
				cityName,
				property.Bedrooms,
				property.Bathrooms,
				property.SquareFootage,
				imageURL,
				userIDs,
			)
		}
	}
}

>>>>>>> 4698d88 (AFTER ADDING NOTIFICATION PROEPRTIES TO USERS)
// CreatePropertySale creates a new property for sale
func CreatePropertySale(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// SECURITY: Check if user is a member of an organization
	// If they are, ALL properties MUST belong to that organization
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
			log.Printf("🔒 Security: User %d is a member of organization %d - property will be assigned to organization", userID, member.OrganizationID)
		} else {
			// User is not a member - allow creating as individual owner
			organizationID = nil
		}
	}

	var input struct {
		// Step 1: Property Title Only
		Title string `json:"title" validate:"required"`

		// Step 2: Property Description Only
		Description string `json:"description" validate:"required"`

		// Step 3: Property Type Only
		PropertyType string `json:"property_type" validate:"required"`

		// Step 4: Pricing Only
		Price float64 `json:"price" validate:"required"`

		// Step 5: Basic Details
		Bedrooms  int `json:"bedrooms" validate:"required"`
		Bathrooms int `json:"bathrooms" validate:"required"`
		Area      int `json:"area" validate:"required"`
		YearBuilt int `json:"year_built"`

		// Step 6: Location with Map
		Address    string  `json:"address" validate:"required"`
		City       string  `json:"city" validate:"required"`
		CityID     *uint   `json:"city_id"`
		ZoneID     *uint   `json:"zone_id"`
		QuartierID *uint   `json:"quartier_id"`
		State      string  `json:"state"`
		Country    string  `json:"country"`
		PostalCode string  `json:"postal_code"`
		Latitude   float64 `json:"latitude" validate:"required"`
		Longitude  float64 `json:"longitude" validate:"required"`

		// Step 7: Indoor Features
		IndoorFeatures []string `json:"indoor_features"`

		// Step 8: Outdoor Features
		OutdoorFeatures []string `json:"outdoor_features"`

		// Step 9: Amenities (Zillow-style)
		Amenities  []string `json:"amenities"`   // Legacy - kept for backward compatibility
		AmenityIDs []uint   `json:"amenity_ids"` // New - amenity IDs from database

		// Step 10: Images
		Images []string `json:"images" validate:"required"`

		// Step 10b: Classified Photos (by room type)
		ClassifiedPhotos []models.ClassifiedPhoto `json:"classified_photos"`

		// Step 11: Video Walkthrough
		Videos []string `json:"videos"`

		// Step 12: Floor Plans (per floor) and Neighborhood
		FloorPlans   []models.FloorPlan       `json:"floor_plans"`
		Neighborhood *models.NeighborhoodInfo `json:"neighborhood"`

		// Optional fields
		AgentID *uint `json:"agent_id"`
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

	// Calculate price per square foot
	var pricePerSqFt float64
	if input.Area > 0 {
		pricePerSqFt = input.Price / float64(input.Area)
	}

	// Combine indoor and outdoor features
	var allFeatures []string
	allFeatures = append(allFeatures, input.IndoorFeatures...)
	allFeatures = append(allFeatures, input.OutdoorFeatures...)

	// Marshal array fields as JSON for Postgres json columns
	imagesJSON, _ := json.Marshal(input.Images)
	videosJSON, _ := json.Marshal(input.Videos)
	featuresJSON, _ := json.Marshal(allFeatures)
	amenitiesJSON, _ := json.Marshal(input.Amenities)
	classifiedPhotosJSON, _ := json.Marshal(input.ClassifiedPhotos)
	floorPlansJSON, _ := json.Marshal(input.FloorPlans)
	neighborhoodJSON, _ := json.Marshal(input.Neighborhood)

	// Create property sale with owner_id ALWAYS set
	ownerIDPtr := &userID
	property := models.PropertySale{
		OrganizationID: organizationID,
		OwnerID:        ownerIDPtr, // ALWAYS set owner_id
		AgentID:        input.AgentID,
		Title:          input.Title,
		Description:    input.Description,
		PropertyType:   input.PropertyType,
		Category:       "residential",
		Address:        input.Address,
		City:           input.City,
		CityID:         input.CityID,
		ZoneID:         input.ZoneID,
		QuartierID:     input.QuartierID,
		State:          input.State,
		Country:        input.Country,
		PostalCode:     input.PostalCode,
		Latitude:       input.Latitude,
		Longitude:      input.Longitude,
		Bedrooms:       input.Bedrooms,
		Bathrooms:      input.Bathrooms,
		SquareFootage:  input.Area,
		YearBuilt:      input.YearBuilt,
		ListingPrice:   input.Price,
		Currency:       "USD",
		PricePerSqFt:   pricePerSqFt,
		Status:         "draft",
		IsVerified:     false,
		IsPublished:    false,
	}

	// One-time translations for title & description
	titleTranslations := services.TranslateAllLanguages(property.Title)
	descTranslations := services.TranslateAllLanguages(property.Description)
	if b, err := json.Marshal(titleTranslations); err == nil {
		property.TitleTranslations = b
	}
	if b, err := json.Marshal(descTranslations); err == nil {
		property.DescriptionTranslations = b
	}

	// Set JSON fields using raw SQL to avoid GORM serialization issues
	// First create the record with owner_id set
	if err := storage.DB.Create(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create property", "details": err.Error()})
		return
	}

	// Update JSON fields separately using raw SQL
	updateSQL := `
		UPDATE property_sales 
		SET images = ?::json,
		    videos = ?::json,
		    features = ?::json,
		    amenities = ?::json,
		    classified_photos = ?::json,
		    floor_plans = ?::json,
		    neighborhood = ?::json
		WHERE id = ?
	`
	if err := storage.DB.Exec(updateSQL,
		string(imagesJSON),
		string(videosJSON),
		string(featuresJSON),
		string(amenitiesJSON),
		string(classifiedPhotosJSON),
		string(floorPlansJSON),
		string(neighborhoodJSON),
		property.ID,
	).Error; err != nil {
		// If JSON update fails, delete the property to avoid orphaned records
		storage.DB.Delete(&property)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update property JSON fields", "details": err.Error()})
		return
	}

	// Associate amenities from database (if amenity_ids provided)
	if len(input.AmenityIDs) > 0 {
		var amenities []models.Amenity
		if err := storage.DB.Where("id IN ?", input.AmenityIDs).Find(&amenities).Error; err == nil {
			// Associate amenities using Many2Many
			if err := storage.DB.Model(&property).Association("AmenityList").Replace(amenities); err != nil {
				log.Printf("⚠️  Warning: Failed to associate amenities: %v", err)
			}
		}
	}

	// Respond without refetch to avoid scan issues on json[] fields
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{"message": "Property created successfully"})
}

// GetUserPropertySales gets all property sales for user (organization or individual)
func GetUserPropertySales(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an ACTIVE organization (as owner or active member)
	var organization models.Organization
	var hasOrganization bool

	// First check if user is owner
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		hasOrganization = true
	} else {
		// Check if user is an ACTIVE member (not removed) - directly query to ensure status is "active"
		var member models.OrganizationMember
		if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
			Preload("Organization").
			First(&member).Error; err == nil {
			organization = member.Organization
			hasOrganization = true
		}
	}

	var properties []models.PropertySale
	query := storage.DB.Preload("Agent.User").Preload("Organization").Preload("Owner")

	if hasOrganization {
		// Fetch properties from organization OR individual properties owned by user
		// CRITICAL: Include personal properties (organization_id IS NULL AND owner_id = userID)
		// This ensures personal properties created BEFORE joining are still visible
		query = query.Where(
			"organization_id = ? OR (organization_id IS NULL AND owner_id = ?)",
			organization.ID, userID,
		)
	} else {
		// User has no organization - fetch ALL individual properties owned by user
		// CRITICAL: After leaving an agency, personal properties (organization_id IS NULL) must be visible
		// This ensures personal properties are visible after leaving
		query = query.Where("organization_id IS NULL AND owner_id = ?", userID)
	}

	if err := query.Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	// Localize titles/descriptions based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
	}

	ctx.JSON(iris.Map{"properties": properties})
}

// GetPropertySale gets a specific property sale
func GetPropertySale(ctx iris.Context) {
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Agent.User").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	property.Title = utils.ResolveLocalizedText(property.Title, property.TitleTranslations, lang)
	property.Description = utils.ResolveLocalizedText(property.Description, property.DescriptionTranslations, lang)

	ctx.JSON(iris.Map{"property": property})
}

// CreateOffer allows an authenticated user to submit an offer on a property sale
func CreateOffer(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	propertyID := uint(propertyIDU64)

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var payload struct {
		Amount  float64 `json:"amount"`
		Message string  `json:"message"`
	}
	if err := ctx.ReadJSON(&payload); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid payload"})
		return
	}
	if payload.Amount <= 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Amount must be greater than zero"})
		return
	}

	offer := models.PropertyOffer{
		PropertyID: property.ID,
		UserID:     userID,
		Amount:     payload.Amount,
		Message:    payload.Message,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}
	if err := storage.DB.Create(&offer).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create offer"})
		return
	}

	// Get user info for the message
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		fmt.Printf("❌ Failed to get user info: %v\n", err)
	}
	userName := user.FirstName + " " + user.LastName
	if userName == " " {
		userName = "مستخدم"
	}

	// Determine property owner ID
	var ownerID uint
	if property.OwnerID != nil {
		ownerID = *property.OwnerID
	} else if property.OrganizationID != nil && property.Organization.OwnerID != 0 {
		ownerID = property.Organization.OwnerID
	}

	if ownerID > 0 && ownerID != userID {
		// Create Arabic message for the offer
		arabicMessage := fmt.Sprintf(
			"🏠 عرض شراء جديد!\n\n"+
				"السلام عليكم،\n\n"+
				"أنا مهتم بشراء عقارك '%s'.\n\n"+
				"💰 **مبلغ العرض:** %.0f MRU\n\n",
			property.Title, payload.Amount)

		if payload.Message != "" {
			arabicMessage += fmt.Sprintf("📝 **رسالتي:**\n%s\n\n", payload.Message)
		}

		arabicMessage += "أرجو التواصل معي لمناقشة التفاصيل.\n\nشكراً لك! 🙏"

		// Create direct message between user and owner
		directMessage := models.DirectMessage{
			SenderID:   userID,
			ReceiverID: ownerID,
			Content:    arabicMessage,
			IsRead:     false,
		}
		if err := storage.DB.Create(&directMessage).Error; err != nil {
			fmt.Printf("❌ Failed to create direct message for offer: %v\n", err)
		} else {
			fmt.Printf("✅ Created direct message for offer from user %d to owner %d\n", userID, ownerID)
		}

		// Send push notification to owner
		go func() {
			services.NotificationServiceInstance.SendPropertyOfferNotificationToHost(
				offer.ID,
				property.ID,
				ownerID,
				userID,
				userName,
				property.Title,
				payload.Amount,
			)
		}()
	}

	ctx.JSON(iris.Map{"offer": offer, "ok": true})
}

// GetOrganizationOffers lists all offers for properties owned by the authenticated user's organization
func GetOrganizationOffers(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	// Find organization by owner
	var org models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&org).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	// Join offers with property_sales to filter by organization
	type OfferWithJoins struct {
		models.PropertyOffer
		Property models.PropertySale `gorm:"embedded"`
		User     models.User         `gorm:"embedded"`
	}

	var offers []models.PropertyOffer
	if err := storage.DB.
		Preload("Property").
		Preload("Property.Organization").
		Preload("User").
		Where("property_id IN (SELECT id FROM property_sales WHERE organization_id = ?)", org.ID).
		Order("created_at DESC").
		Find(&offers).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch offers"})
		return
	}

	// Build lightweight response with user name/initials
	resp := make([]iris.Map, 0, len(offers))
	for _, o := range offers {
		fullName := ""
		if o.User.FirstName != "" || o.User.LastName != "" {
			fullName = (o.User.FirstName + " " + o.User.LastName)
		}
		resp = append(resp, iris.Map{
			"id":         o.ID,
			"amount":     o.Amount,
			"message":    o.Message,
			"status":     o.Status,
			"created_at": o.CreatedAt,
			"user": iris.Map{
				"id":          o.UserID,
				"firstName":   o.User.FirstName,
				"lastName":    o.User.LastName,
				"email":       o.User.Email,
				"avatarURL":   o.User.AvatarURL,
				"displayName": fullName,
			},
			"property": iris.Map{
				"id":    o.PropertyID,
				"title": o.Property.Title,
			},
		})
	}
	ctx.JSON(iris.Map{"offers": resp})
}

// UpdateOfferStatus updates an offer's status (accept/reject/withdraw)
func UpdateOfferStatus(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	offerIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	offerID := uint(offerIDU64)

	var offer models.PropertyOffer
	if err := storage.DB.Preload("Property").First(&offer, offerID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Offer not found"})
		return
	}

	// Only org owner of the property can update
	var property models.PropertySale
	if err := storage.DB.First(&property, offer.PropertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	// Check if property has an organization
	if property.OrganizationID == nil {
		// Property belongs to individual owner - need to check ownership differently
		// For now, allow if user is authenticated (we may need to add user_id to PropertySale later)
		// TODO: Add user_id field to PropertySale to track individual owners
	} else {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err != nil {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Access denied"})
			return
		}
		if org.OwnerID != userID {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Access denied"})
			return
		}
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := ctx.ReadJSON(&payload); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	if payload.Status != "accepted" && payload.Status != "rejected" && payload.Status != "withdrawn" && payload.Status != "pending" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid status"})
		return
	}

	offer.Status = payload.Status
	if err := storage.DB.Save(&offer).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update offer"})
		return
	}

	ctx.JSON(iris.Map{"offer": offer, "ok": true})
}

// PublicOfferInsights returns aggregated offer insights for a published property
func PublicOfferInsights(ctx iris.Context) {
	propertyIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	propertyID := uint(propertyIDU64)

	// Only for published properties
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND status = ? AND is_published = ?", propertyID, "published", true).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Aggregate offers
	type Row struct {
		Count int64
		Min   float64
		Max   float64
		Avg   float64
	}
	var row Row
	if err := storage.DB.
		Raw("SELECT COUNT(*) as count, COALESCE(MIN(amount),0) as min, COALESCE(MAX(amount),0) as max, COALESCE(AVG(amount),0) as avg FROM property_offers WHERE property_id = ?", propertyID).
		Scan(&row).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to compute insights"})
		return
	}

	ctx.JSON(iris.Map{
		"offers": iris.Map{
			"count":    row.Count,
			"lowest":   row.Min,
			"highest":  row.Max,
			"average":  row.Avg,
			"currency": "MRU",
		},
		"property": iris.Map{"id": property.ID, "title": property.Title},
	})
}

// GetPublicPropertyOffers returns all individual offers for a published property (for chart visualization)
func GetPublicPropertyOffers(ctx iris.Context) {
	propertyIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	propertyID := uint(propertyIDU64)

	// Only for published properties
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND status = ? AND is_published = ?", propertyID, "published", true).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var offers []models.PropertyOffer
	if err := storage.DB.Where("property_id = ?", propertyID).
		Order("created_at ASC").
		Find(&offers).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch offers"})
		return
	}

	// Build lightweight response
	resp := make([]iris.Map, 0, len(offers))
	for _, o := range offers {
		resp = append(resp, iris.Map{
			"id":         o.ID,
			"amount":     o.Amount,
			"price":      o.Amount, // Alias for compatibility
			"created_at": o.CreatedAt.Format(time.RFC3339),
			"status":     o.Status,
		})
	}

	ctx.JSON(iris.Map{"offers": resp})
}

// UpdatePropertySale updates a property sale
// Allows: property owner, organization owner, assigned agent, or organization members with edit permissions
func UpdatePropertySale(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check if user has permission to edit:
	// 1. User is the property owner (owner_id matches)
	// 2. User is the organization owner
	// 3. User is the assigned agent
	// 4. User is an organization member with edit permissions
	canEdit := false

	// Check 1: Property owner
	if property.OwnerID != nil && *property.OwnerID == userID {
		canEdit = true
		log.Printf("✅ User %d is property owner - allowed to edit", userID)
	}

	// Check 2: Organization owner
	if !canEdit && property.OrganizationID != nil && property.Organization != nil {
		if property.Organization.OwnerID == userID {
			canEdit = true
			log.Printf("✅ User %d is organization owner - allowed to edit", userID)
		}
	}

	// Check 3: Assigned agent
	if !canEdit && property.AgentID != nil {
		var agent models.Agent
		if err := storage.DB.Where("id = ? AND user_id = ?", property.AgentID, userID).First(&agent).Error; err == nil {
			canEdit = true
			log.Printf("✅ User %d is assigned agent - allowed to edit", userID)
		}
	}

		// Check 4: Organization member with edit permissions
		if !canEdit && property.OrganizationID != nil {
			var member models.OrganizationMember
			if err := storage.DB.Where("user_id = ? AND organization_id = ? AND status = ? AND is_active = ?",
				userID, property.OrganizationID, "active", true).First(&member).Error; err == nil {
				// Check if member has property edit permission or has a role that allows editing
				if member.HasPermission(models.PermissionPropertyEdit) || member.Role == "admin" || member.Role == "manager" || member.Role == "editor" {
					canEdit = true
					log.Printf("✅ User %d is organization member with edit permissions - allowed to edit", userID)
				}
			}
		}

	if !canEdit {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You do not have permission to edit this property"})
		return
	}

	var input struct {
		Title           string                   `json:"title"`
		Description     string                   `json:"description"`
		PropertyType    string                   `json:"property_type"`
		Category        string                   `json:"category"`
		Address         string                   `json:"address"`
		City            string                   `json:"city"`
		CityID          *uint                    `json:"city_id"`
		ZoneID          *uint                    `json:"zone_id"`
		QuartierID      *uint                    `json:"quartier_id"`
		State           string                   `json:"state"`
		Country         string                   `json:"country"`
		PostalCode      string                   `json:"postal_code"`
		Latitude        float64                  `json:"latitude"`
		Longitude       float64                  `json:"longitude"`
		Bedrooms        int                      `json:"bedrooms"`
		Bathrooms       int                      `json:"bathrooms"`
		SquareFootage   int                      `json:"square_footage"`
		Area            int                      `json:"area"` // Alias for square_footage
		LotSize         float64                  `json:"lot_size"`
		YearBuilt       int                      `json:"year_built"`
		ParkingSpaces   int                      `json:"parking_spaces"`
		ListingPrice    float64                  `json:"listing_price"`
		Price           float64                  `json:"price"` // Alias for listing_price
		Currency        string                   `json:"currency"`
		PropertyTax     float64                  `json:"property_tax"`
		HOA             float64                  `json:"hoa"`
		Images          []string                 `json:"images"`
		Videos          []string                 `json:"videos"`
		VirtualTour     string                   `json:"virtual_tour"`
		FloorPlans      []models.FloorPlan       `json:"floor_plans"`
		Neighborhood    *models.NeighborhoodInfo `json:"neighborhood"`
		IndoorFeatures  []string                 `json:"indoor_features"`
		OutdoorFeatures []string                 `json:"outdoor_features"`
		Features        []string                 `json:"features"`    // Legacy
		Amenities       []string                 `json:"amenities"`   // Legacy
		AmenityIDs      []uint                   `json:"amenity_ids"` // New - amenity IDs from database
		AgentID         *uint                    `json:"agent_id"`
		Status          string                   `json:"status"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Update fields
	titleChanged := false
	descChanged := false
	if input.Title != "" {
		property.Title = input.Title
		titleChanged = true
	}
	if input.Description != "" {
		property.Description = input.Description
		descChanged = true
	}
	if input.PropertyType != "" {
		property.PropertyType = input.PropertyType
	}
	if input.Category != "" {
		property.Category = input.Category
	}
	if input.Address != "" {
		property.Address = input.Address
	}
	if input.City != "" {
		property.City = input.City
	}
	if input.CityID != nil {
		property.CityID = input.CityID
	}
	if input.ZoneID != nil {
		property.ZoneID = input.ZoneID
	}
	if input.QuartierID != nil {
		property.QuartierID = input.QuartierID
	}
	if input.State != "" {
		property.State = input.State
	}
	if input.Country != "" {
		property.Country = input.Country
	}
	if input.PostalCode != "" {
		property.PostalCode = input.PostalCode
	}
	if input.Latitude != 0 {
		property.Latitude = input.Latitude
	}
	if input.Longitude != 0 {
		property.Longitude = input.Longitude
	}
	if input.Bedrooms != 0 {
		property.Bedrooms = input.Bedrooms
	}
	if input.Bathrooms != 0 {
		property.Bathrooms = input.Bathrooms
	}
	// Handle both square_footage and area (alias)
	if input.SquareFootage != 0 {
		property.SquareFootage = input.SquareFootage
	} else if input.Area != 0 {
		property.SquareFootage = input.Area
	}
	if input.LotSize != 0 {
		property.LotSize = input.LotSize
	}
	if input.YearBuilt != 0 {
		property.YearBuilt = input.YearBuilt
	}
	if input.ParkingSpaces != 0 {
		property.ParkingSpaces = input.ParkingSpaces
	}
	// Handle both listing_price and price (alias)
	if input.ListingPrice != 0 {
		property.ListingPrice = input.ListingPrice
		// Recalculate price per square foot
		if property.SquareFootage > 0 {
			property.PricePerSqFt = input.ListingPrice / float64(property.SquareFootage)
		}
	} else if input.Price != 0 {
		property.ListingPrice = input.Price
		// Recalculate price per square foot
		if property.SquareFootage > 0 {
			property.PricePerSqFt = input.Price / float64(property.SquareFootage)
		}
	}
	if input.Currency != "" {
		property.Currency = input.Currency
	}
	if input.PropertyTax != 0 {
		property.PropertyTax = input.PropertyTax
	}
	if input.HOA != 0 {
		property.HOA = input.HOA
	}
	if input.Images != nil {
		property.Images = input.Images
	}
	if input.Videos != nil {
		property.Videos = input.Videos
	}
	if input.VirtualTour != "" {
		property.VirtualTour = input.VirtualTour
	}
	if input.FloorPlans != nil {
		property.FloorPlans = input.FloorPlans
	}
	if input.Neighborhood != nil {
		property.Neighborhood = input.Neighborhood
	}
	// Handle indoor and outdoor features
	if input.IndoorFeatures != nil || input.OutdoorFeatures != nil {
		var allFeatures []string
		if input.IndoorFeatures != nil {
			allFeatures = append(allFeatures, input.IndoorFeatures...)
		}
		if input.OutdoorFeatures != nil {
			allFeatures = append(allFeatures, input.OutdoorFeatures...)
		}
		property.Features = allFeatures
	} else if input.Features != nil {
		property.Features = input.Features
	}

	// Handle amenities - update both legacy and new format
	if input.AmenityIDs != nil && len(input.AmenityIDs) > 0 {
		// Update Many2Many relationship
		var amenities []models.Amenity
		if err := storage.DB.Where("id IN ?", input.AmenityIDs).Find(&amenities).Error; err == nil {
			if err := storage.DB.Model(&property).Association("AmenityList").Replace(amenities); err != nil {
				log.Printf("⚠️  Warning: Failed to update amenities: %v", err)
			}
		}
		// Also update legacy JSON field for backward compatibility
		if input.Amenities != nil {
			property.Amenities = input.Amenities
		}
	} else if input.Amenities != nil {
		property.Amenities = input.Amenities
	}
	if input.AgentID != nil {
		property.AgentID = input.AgentID
	}
	if input.Status != "" {
		property.Status = input.Status
	}

	// Update translations if title or description changed
	if titleChanged || descChanged {
		if titleChanged {
			titleTranslations := services.TranslateAllLanguages(property.Title)
			if b, err := json.Marshal(titleTranslations); err == nil {
				property.TitleTranslations = b
			}
		}
		if descChanged {
			descTranslations := services.TranslateAllLanguages(property.Description)
			if b, err := json.Marshal(descTranslations); err == nil {
				property.DescriptionTranslations = b
			}
		}
	}

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update property"})
		return
	}

	ctx.JSON(iris.Map{
		"message":  "Property updated successfully",
		"property": property,
	})
}

// SubmitPropertyForVerification submits a property for verification
func SubmitPropertyForVerification(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND organization_id = ?", propertyID, organization.ID).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check if property is in draft status
	if property.Status != "draft" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property must be in draft status to submit for verification"})
		return
	}

	// Update status to pending verification
	property.Status = "pending_verification"
	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to submit property for verification"})
		return
	}

	ctx.JSON(iris.Map{
		"message":  "Property submitted for verification successfully",
		"property": property,
	})
}

// AdminGetPropertySales gets all property sales (admin only)
func AdminGetPropertySales(ctx iris.Context) {
	var properties []models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Agent.User").Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	ctx.JSON(iris.Map{"properties": properties})
}

// AdminVerifyProperty verifies a property (admin only)
func AdminVerifyProperty(ctx iris.Context) {
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	uid := ctx.Values().Get("userID")
	var adminID uint
	if v, ok := uid.(uint); ok {
		adminID = v
	} else if v2, ok := uid.(int); ok {
		adminID = uint(v2)
	} else {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var input struct {
		IsVerified        bool   `json:"is_verified"`
		VerificationNotes string `json:"verification_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	property.IsVerified = input.IsVerified
	property.VerificationNotes = input.VerificationNotes
	property.VerifiedBy = &adminID

	if input.IsVerified {
		property.Status = "verified"
		property.VerifiedAt = &[]time.Time{time.Now()}[0]
	} else {
		property.Status = "draft"
		property.VerifiedAt = nil
	}

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to verify property"})
		return
	}

	ctx.JSON(iris.Map{
		"message":  "Property verification updated successfully",
		"property": property,
	})
}

// PublishProperty publishes a verified property
func PublishProperty(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND organization_id = ?", propertyID, organization.ID).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check if property is verified
	if !property.IsVerified {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property must be verified before publishing"})
		return
	}

	// Update status to published
	property.Status = "published"
	property.IsPublished = true
	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to publish property"})
		return
	}

<<<<<<< HEAD
=======
	// Trigger notification to users with matching favorite city
	go triggerNewPropertyNotification(property)

>>>>>>> 4698d88 (AFTER ADDING NOTIFICATION PROEPRTIES TO USERS)
	ctx.JSON(iris.Map{
		"message":  "Property published successfully",
		"property": property,
	})
}

// AdminPublishProperty allows admins to publish a verified property sale regardless of ownership
func AdminPublishProperty(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Where("id = ?", id).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	if !property.IsVerified {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property must be verified before publishing"})
		return
	}

	property.Status = "published"
	property.IsPublished = true
	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to publish property"})
		return
	}

<<<<<<< HEAD
=======
	// Trigger notification to users with matching favorite city
	go triggerNewPropertyNotification(property)

>>>>>>> 4698d88 (AFTER ADDING NOTIFICATION PROEPRTIES TO USERS)
	ctx.JSON(iris.Map{"message": "Property published successfully", "property": property})
}

// GetPublishedProperties gets all published properties for public viewing
// Works for both authenticated and unauthenticated users
// Properties are rotated per-request to show fresh content (TikTok-like cycling)
// Properties with images are always prioritized at the top
func GetPublishedProperties(ctx iris.Context) {
	// Optional auth: extract userID from context or Authorization header
	var userID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
			fmt.Printf("🔍 GetPublishedProperties: User ID from context: %d\n", userID)
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			fmt.Printf("🔍 GetPublishedProperties: Parsing Authorization header\n")
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
					fmt.Printf("🔍 GetPublishedProperties: User ID from token: %d\n", userID)
				}
			}
		}
	}

	q := storage.DB.Model(&models.PropertySale{}).
		Preload("Organization").
		Preload("Agent.User").
		Where("property_sales.status = ? OR property_sales.is_published = ?", "published", true)

	if userID > 0 {
		fmt.Printf("🔍 GetPublishedProperties: Applying filters for user ID: %d\n", userID)
		// Exclude items from blocked users (either the agent's user or the organization's owner)
		q = q.Joins("LEFT JOIN agents ON agents.id = property_sales.agent_id").
			Joins("LEFT JOIN organizations ON organizations.id = property_sales.organization_id").
			Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagger_id = ? AND uf.status = 'active' AND (uf.flagged_user_id = agents.user_id OR uf.flagged_user_id = organizations.owner_id))", userID)

		// Exclude hidden property sales
		q = q.Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID)
		// Exclude properties from organizations explicitly blocked by the user
		q = q.Where("NOT EXISTS (SELECT 1 FROM user_blocked_organizations ubo WHERE ubo.user_id = ? AND ubo.organization_id = property_sales.organization_id AND ubo.status = 'active')", userID)
		fmt.Printf("🔍 GetPublishedProperties: Applied blocking and hiding filters\n")
	} else {
		fmt.Printf("🔍 GetPublishedProperties: No user ID, showing all property sales\n")
	}

	// Apply optional filters
	if v := ctx.URLParam("bedrooms"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q = q.Where("property_sales.bedrooms >= ?", n)
		}
	}
	if v := ctx.URLParam("bathrooms"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q = q.Where("property_sales.bathrooms >= ?", n)
		}
	}
	// Year built filter - minimum year built (properties built in this year or later)
	// IMPORTANT: Exclude properties with NULL or 0 year_built values
	if v := ctx.URLParam("year_built"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			fmt.Printf("🔍 GetPublishedProperties: Applying year_built filter: >= %d (excluding NULL/0 values)\n", n)
			q = q.Where("property_sales.year_built >= ? AND property_sales.year_built > 0", n)
		}
	}
	// City filter
	if v := ctx.URLParam("city_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.city_id = ?", n)
		}
	}
	// Zone filter
	if v := ctx.URLParam("zone_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.zone_id = ?", n)
		}
	}
	// Area filter with range logic (e.g., if user selects 500, show 450-550 with priority to exactly 500)
	var areaPriorityValue *int // Store the target area for priority sorting
	if minAreaStr := ctx.URLParam("min_area"); minAreaStr != "" {
		if minArea, err := strconv.Atoi(minAreaStr); err == nil && minArea > 0 {
			if maxAreaStr := ctx.URLParam("max_area"); maxAreaStr != "" {
				// Both min and max provided - exact range filter
				if maxArea, err := strconv.Atoi(maxAreaStr); err == nil && maxArea > 0 {
					q = q.Where("property_sales.square_footage >= ? AND property_sales.square_footage <= ?", minArea, maxArea)
					fmt.Printf("🔍 GetPublishedProperties: Applying area filter: %d-%d m²\n", minArea, maxArea)
				}
			} else {
				// Only min provided - use range logic (10% buffer)
				// If user selects 500, show 450-550 (500 ± 50, which is 10% of 500)
				buffer := int(float64(minArea) * 0.1) // 10% buffer
				if buffer < 10 {
					buffer = 10 // Minimum 10 m² buffer
				}
				rangeMin := minArea - buffer
				rangeMax := minArea + buffer
				if rangeMin < 0 {
					rangeMin = 0
				}
				q = q.Where("property_sales.square_footage >= ? AND property_sales.square_footage <= ?", rangeMin, rangeMax)
				areaPriorityValue = &minArea // Store target for priority sorting
				fmt.Printf("🔍 GetPublishedProperties: Applying area filter with range logic: %d m² (range: %d-%d m²)\n", minArea, rangeMin, rangeMax)
			}
		}
	} else if maxAreaStr := ctx.URLParam("max_area"); maxAreaStr != "" {
		// Only max provided
		if maxArea, err := strconv.Atoi(maxAreaStr); err == nil && maxArea > 0 {
			q = q.Where("property_sales.square_footage <= ?", maxArea)
			fmt.Printf("🔍 GetPublishedProperties: Applying max area filter: <= %d m²\n", maxArea)
		}
	}
	// Quartier filter
	if v := ctx.URLParam("quartier_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.quartier_id = ?", n)
			fmt.Printf("🔍 GetPublishedProperties: Applying quartier_id filter: %d\n", n)
		}
	}
	// Price range filter for property sales
	if v := ctx.URLParam("min_price"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			q = q.Where("property_sales.listing_price >= ?", n)
			fmt.Printf("🔍 GetPublishedProperties: Applying min_price filter: %.2f\n", n)
		}
	}
	if v := ctx.URLParam("max_price"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			q = q.Where("property_sales.listing_price <= ?", n)
			fmt.Printf("🔍 GetPublishedProperties: Applying max_price filter: %.2f\n", n)
		}
	}

	var allProperties []models.PropertySale
	if err := q.Find(&allProperties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	fmt.Printf("✅ Found %d property sales before rotation\n", len(allProperties))

	// Sort by area priority if area filter with target value is applied
	// Exact matches (e.g., square_footage = 500) come first, then others in the range
	if areaPriorityValue != nil {
		sort.Slice(allProperties, func(i, j int) bool {
			iArea := allProperties[i].SquareFootage
			jArea := allProperties[j].SquareFootage
			iExact := (iArea == *areaPriorityValue)
			jExact := (jArea == *areaPriorityValue)
			
			// Exact matches come first
			if iExact && !jExact {
				return true
			}
			if !iExact && jExact {
				return false
			}
			// If both are exact or both are not exact, maintain original order (stability)
			return false
		})
		fmt.Printf("🔍 Sorted properties by area priority (exact matches first for %d m²)\n", *areaPriorityValue)
	}

	// Separate properties with images from those without
	var withImages []models.PropertySale
	var withoutImages []models.PropertySale

	for _, prop := range allProperties {
		// Check if property has images (Images is stored as JSON)
		hasImages := false
		if prop.Images != nil && len(prop.Images) > 0 {
			// Check if images array contains non-empty strings
			for _, img := range prop.Images {
				if strings.TrimSpace(img) != "" {
					hasImages = true
					break
				}
			}
		}

		if hasImages {
			withImages = append(withImages, prop)
		} else {
			withoutImages = append(withoutImages, prop)
		}
	}

	fmt.Printf("📸 Property sales with images: %d, without images: %d\n", len(withImages), len(withoutImages))

	// Apply time-based rotation for TikTok-like cycling
	// Rotation changes per-request with high precision + random component for maximum variety
	// This ensures users see different properties on each visit/reload
	now := time.Now()
	// High-precision rotation: includes seconds, nanoseconds, Unix timestamp, and random component
	// Random component ensures different results even if requests happen at exact same time
	rand.Seed(now.UnixNano())                  // Seed random with current nanosecond for true randomness
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
	rotateProperties := func(props []models.PropertySale, seed int64) []models.PropertySale {
		if len(props) == 0 {
			return props
		}
		if len(props) == 1 {
			return props
		}

		// Calculate offset with multiple components for better distribution
		offset1 := int(seed % int64(len(props)))
		offset2 := int((seed * 7) % int64(len(props))) // Different multiplier for variety

		// Use the larger offset, but ensure it's not 0
		offset := offset1
		if offset2 > offset1 {
			offset = offset2
		}
		if offset == 0 {
			offset = int((seed*13)%int64(len(props)-1)) + 1 // Force non-zero with different multiplier
		}

		// Perform rotation
		rotated := make([]models.PropertySale, len(props))
		copy(rotated, props[offset:])
		copy(rotated[len(props)-offset:], props[:offset])
		return rotated
	}

	withImages = rotateProperties(withImages, rotationSeed)
	withoutImages = rotateProperties(withoutImages, rotationSeed+2000) // Different offset for variety

	// Combine: properties with images first, then without
	var properties []models.PropertySale
	properties = append(properties, withImages...)
	properties = append(properties, withoutImages...)

	fmt.Printf("✅ Returning %d property sales (rotated, images prioritized)\n", len(properties))

	// Localize titles/descriptions based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
	}

	ctx.JSON(iris.Map{"properties": properties})
}

// GetPublishedProperty - Get single published property (public access)
func GetPublishedProperty(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Agent.User").Preload("Owner").Preload("AmenityList").Where("id = ? AND (status = ? OR is_published = ?)", id, "published", true).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	property.Title = utils.ResolveLocalizedText(property.Title, property.TitleTranslations, lang)
	property.Description = utils.ResolveLocalizedText(property.Description, property.DescriptionTranslations, lang)

	// Build translated amenities array from AmenityList
	translatedAmenities := make([]string, 0)
	for _, amenity := range property.AmenityList {
		// Get translated name based on language
		var name string
		switch lang {
		case "fr":
			name = amenity.Name.Fr
		case "ar":
			name = amenity.Name.Ar
		default:
			name = amenity.Name.En
		}
		if name != "" {
			translatedAmenities = append(translatedAmenities, name)
		}
	}

	// If we have translated amenities, use them; otherwise fall back to legacy string array
	if len(translatedAmenities) > 0 {
		property.Amenities = translatedAmenities
	}

	ctx.JSON(iris.Map{"property": property})
}

// ReportPublishedPropertySale allows an authenticated user to report a published property sale
func ReportPublishedPropertySale(ctx iris.Context) {
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
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	// Ensure property is published
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND (status = ? OR is_published = ?)", id, "published", true).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
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

	report := models.PropertySaleReport{
		ReporterID:     userID,
		PropertySaleID: property.ID,
		Reason:         body.Reason,
		Description:    body.Description,
	}
	if err := storage.DB.Create(&report).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create report"})
		return
	}

	ctx.JSON(iris.Map{"success": true})
}

// HidePropertySale allows an authenticated user to hide a property sale
func HidePropertySale(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	propertySaleID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property sale ID"})
		return
	}

	var input struct {
		Reason string `json:"reason" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Check if property sale exists and is published
	var propertySale models.PropertySale
	if err := storage.DB.Where("id = ? AND (status = ? OR is_published = ?)", propertySaleID, "published", true).First(&propertySale).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property sale not found or not published"})
		return
	}

	// Check if already hidden by this user
	var existingHide models.HiddenPropertySale
	if err := storage.DB.Where("user_id = ? AND property_sale_id = ? AND deleted_at IS NULL", userID, propertySaleID).First(&existingHide).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "message": "Property sale already hidden"})
		return
	}

	hide := models.HiddenPropertySale{
		UserID:         &userID,
		PropertySaleID: propertySaleID,
		Reason:         input.Reason,
	}

	if err := storage.DB.Create(&hide).Error; err != nil {
		fmt.Printf("❌ Error hiding property sale: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to hide property sale"})
		return
	}

	fmt.Printf("✅ Property sale %d hidden by user %d\n", propertySaleID, userID)
	ctx.JSON(iris.Map{"success": true, "message": "Property sale hidden successfully"})
}
