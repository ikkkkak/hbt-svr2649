package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// CreatePropertySale creates a new property for sale
func CreatePropertySale(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization to create properties"})
		return
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
		Amenities []string `json:"amenities"`

		// Step 10: Images
		Images []string `json:"images" validate:"required"`

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
	floorPlansJSON, _ := json.Marshal(input.FloorPlans)
	neighborhoodJSON, _ := json.Marshal(input.Neighborhood)

	// Insert with explicit json casts to avoid 'record' insertion error
	data := map[string]interface{}{
		"organization_id": organization.ID,
		"agent_id":        input.AgentID,
		"title":           input.Title,
		"description":     input.Description,
		"property_type":   input.PropertyType,
		"category":        "residential",
		"address":         input.Address,
		"city":            input.City,
		"state":           input.State,
		"country":         input.Country,
		"postal_code":     input.PostalCode,
		"latitude":        input.Latitude,
		"longitude":       input.Longitude,
		"bedrooms":        input.Bedrooms,
		"bathrooms":       input.Bathrooms,
		"square_footage":  input.Area,
		"year_built":      input.YearBuilt,
		"listing_price":   input.Price,
		"currency":        "USD",
		"price_per_sq_ft": pricePerSqFt,
		"images":          gorm.Expr("?::json", string(imagesJSON)),
		"videos":          gorm.Expr("?::json", string(videosJSON)),
		"features":        gorm.Expr("?::json", string(featuresJSON)),
		"amenities":       gorm.Expr("?::json", string(amenitiesJSON)),
		"floor_plans":     gorm.Expr("?::json", string(floorPlansJSON)),
		"neighborhood":    gorm.Expr("?::json", string(neighborhoodJSON)),
		"status":          "draft",
		"is_verified":     false,
		"is_published":    false,
		"created_at":      time.Now(),
		"updated_at":      time.Now(),
	}

	if err := storage.DB.Table("property_sales").Create(data).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create property", "details": err.Error()})
		return
	}

	// Respond without refetch to avoid scan issues on json[] fields
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{"message": "Property created successfully"})
}

// GetUserPropertySales gets all property sales for user's organization
func GetUserPropertySales(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	var properties []models.PropertySale
	if err := storage.DB.Preload("Agent.User").Where("organization_id = ?", organization.ID).Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
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
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
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

	// TODO: notify owner/organization about new offer (email/push)
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
	var org models.Organization
	if err := storage.DB.First(&org, property.OrganizationID).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Access denied"})
		return
	}
	if org.OwnerID != userID {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Access denied"})
		return
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

// UpdatePropertySale updates a property sale
func UpdatePropertySale(ctx iris.Context) {
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

	var input struct {
		Title         string                   `json:"title"`
		Description   string                   `json:"description"`
		PropertyType  string                   `json:"property_type"`
		Category      string                   `json:"category"`
		Address       string                   `json:"address"`
		City          string                   `json:"city"`
		State         string                   `json:"state"`
		Country       string                   `json:"country"`
		PostalCode    string                   `json:"postal_code"`
		Latitude      float64                  `json:"latitude"`
		Longitude     float64                  `json:"longitude"`
		Bedrooms      int                      `json:"bedrooms"`
		Bathrooms     int                      `json:"bathrooms"`
		SquareFootage int                      `json:"square_footage"`
		LotSize       float64                  `json:"lot_size"`
		YearBuilt     int                      `json:"year_built"`
		ParkingSpaces int                      `json:"parking_spaces"`
		ListingPrice  float64                  `json:"listing_price"`
		Currency      string                   `json:"currency"`
		PropertyTax   float64                  `json:"property_tax"`
		HOA           float64                  `json:"hoa"`
		Images        []string                 `json:"images"`
		Videos        []string                 `json:"videos"`
		VirtualTour   string                   `json:"virtual_tour"`
		FloorPlans    []models.FloorPlan       `json:"floor_plans"`
		Neighborhood  *models.NeighborhoodInfo `json:"neighborhood"`
		Features      []string                 `json:"features"`
		Amenities     []string                 `json:"amenities"`
		AgentID       *uint                    `json:"agent_id"`
		Status        string                   `json:"status"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Update fields
	if input.Title != "" {
		property.Title = input.Title
	}
	if input.Description != "" {
		property.Description = input.Description
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
	if input.SquareFootage != 0 {
		property.SquareFootage = input.SquareFootage
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
	if input.ListingPrice != 0 {
		property.ListingPrice = input.ListingPrice
		// Recalculate price per square foot
		if property.SquareFootage > 0 {
			property.PricePerSqFt = input.ListingPrice / float64(property.SquareFootage)
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
	if input.Features != nil {
		property.Features = input.Features
	}
	if input.Amenities != nil {
		property.Amenities = input.Amenities
	}
	if input.AgentID != nil {
		property.AgentID = input.AgentID
	}
	if input.Status != "" {
		property.Status = input.Status
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

	ctx.JSON(iris.Map{
		"message":  "Property published successfully",
		"property": property,
	})
}

// GetPublishedProperties gets all published properties for public viewing
func GetPublishedProperties(ctx iris.Context) {
	var properties []models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Agent.User").Where("status = ? OR is_published = ?", "published", true).Order("created_at DESC").Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	ctx.JSON(iris.Map{"properties": properties})
}
