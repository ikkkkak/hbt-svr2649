package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/datatypes"
)

func CreateProperty(ctx iris.Context) {
	var input CreateListingInput

	err := ctx.ReadJSON(&input)
	if err != nil {
		log.Printf("[CreateProperty] JSON read error: %v", err)
		utils.HandleValidationErrors(err, ctx)
		return
	}
	if err := utils.Validate.Struct(input); err != nil {
		log.Printf("[CreateProperty] validation failed host=%d type=%q: %v", input.HostID, input.PropertyType, err)
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Ensure arrays are never null
	amenities := input.Amenities
	if amenities == nil {
		amenities = []string{}
	}
	amenitiesJSON, _ := json.Marshal(amenities)

	// Nearby attractions JSON
	nearby := input.NearbyAttractions
	if nearby == nil {
		nearby = []map[string]string{}
	}
	nearbyJSON, _ := json.Marshal(nearby)

	imagesArr := insertImages(InsertImages{
		images: input.Images,
	})
	if imagesArr == nil {
		imagesArr = []string{}
	}
	imagesJSON, _ := json.Marshal(imagesArr)

	property := models.Property{
		HostID:             input.HostID,
		Title:              input.Title,
		Description:        input.Description,
		PropertyType:       input.PropertyType,
		AddressLine1:       input.AddressLine1,
		AddressLine2:       input.AddressLine2,
		City:               input.City,
		State:              input.State,
		Zip:                input.Zip,
		Country:            input.Country,
		CityID:             input.CityID,
		ZoneID:             input.ZoneID,
		QuartierID:         input.QuartierID,
		Lat:                input.Lat,
		Lng:                input.Lng,
		Capacity:           input.Capacity,
		Bedrooms:           input.Bedrooms,
		Beds:               input.Beds,
		Bathrooms:          input.Bathrooms,
		NightlyPrice:       input.NightlyPrice,
		CleaningFee:        input.CleaningFee,
		ServiceFee:         input.ServiceFee,
		Currency:           input.Currency,
		Amenities:          string(amenitiesJSON),
		HouseRules:         input.HouseRules,
		CancellationPolicy: input.CancellationPolicy,
		Images:             string(imagesJSON),
		IsActive:           input.IsActive,

		// Neighborhood & timing & category mapping
		NeighborhoodDescription: input.NeighborhoodDescription,
		NearbyAttractions:       datatypes.JSON(nearbyJSON),
		CheckInTime:             input.CheckInTime,
		CheckOutTime:            input.CheckOutTime,

		// New policy fields
		BookingMode:                      input.BookingMode,
		SecureCompoundAcknowledged:       input.SecureCompoundAcknowledged != nil && *input.SecureCompoundAcknowledged,
		EquipmentViolationPolicyAccepted: input.EquipmentViolationPolicyAccepted != nil && *input.EquipmentViolationPolicyAccepted,
		UserSafetyPolicyAccepted:         input.UserSafetyPolicyAccepted != nil && *input.UserSafetyPolicyAccepted,
		PropertyPolicyAccepted:           input.PropertyPolicyAccepted != nil && *input.PropertyPolicyAccepted,
		HostPrivateNote:                  sanitizeHostPrivateNote(input.HostPrivateNote),
	}

	// One-time translations for core text fields (title, description, neighborhood)
	titleTranslations := services.TranslateAllLanguages(input.Title)
	descTranslations := services.TranslateAllLanguages(input.Description)
	neighTranslations := services.TranslateAllLanguages(input.NeighborhoodDescription)

	if b, err := json.Marshal(titleTranslations); err == nil {
		property.TitleTranslations = b
	}
	if b, err := json.Marshal(descTranslations); err == nil {
		property.DescriptionTranslations = b
	}
	if b, err := json.Marshal(neighTranslations); err == nil {
		property.NeighborhoodDescriptionTranslations = b
	}

	if cid, cName, cNameAr := resolveListingCountry(input.CountryID, input.CityID, input.Country); cid != nil {
		property.CountryID = cid
		property.Country = cName
		_ = cNameAr
	}

	// Optional property category id
	if input.PropertyCategoryId > 0 {
		pc := input.PropertyCategoryId
		property.PropertyCategoryID = &pc
	}

	// DEBUG: Log input and constructed property before saving
	fmt.Printf("[CreateProperty] Input payload summary => hostID=%d, title=%q, propertyType=%q, categoryId=%d, neighDesc=%q, checkIn=%q, checkOut=%q, nearbyAttractions.len=%d\n",
		input.HostID,
		input.Title,
		input.PropertyType,
		input.PropertyCategoryId,
		input.NeighborhoodDescription,
		input.CheckInTime,
		input.CheckOutTime,
		len(input.NearbyAttractions),
	)
	fmt.Printf("[CreateProperty] Constructed model => categoryId(ptr)=%v, neighDesc.len=%d, checkIn=%q, checkOut=%q, amenitiesStr.len=%d, imagesStr.len=%d\n",
		property.PropertyCategoryID,
		len(property.NeighborhoodDescription),
		property.CheckInTime,
		property.CheckOutTime,
		len(property.Amenities),
		len(property.Images),
	)

	result := storage.DB.Create(&property)
	if result.Error != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create property"})
		return
	}

	services.NotifyAdminNewListing(services.ListingAdminNotifyInput{
		Kind:         services.ListingKindRent,
		ID:           property.ID,
		Title:        property.Title,
		City:         property.City,
		Price:        float64(property.NightlyPrice),
		Currency:     property.Currency,
		PropertyType: property.PropertyType,
		HostUserID:   property.HostID,
		Status:       "active",
	})

	// Track property creation for host mode engagement
	// Get user ID from middleware context
	if userID, ok := ctx.Values().Get("userID").(uint); ok && userID > 0 {
		// Update HostModeSwitch if exists
		var hostSwitch models.HostModeSwitch
		if err := storage.DB.Where("user_id = ? AND property_added = ?", userID, false).First(&hostSwitch).Error; err == nil {
			now := time.Now()
			hostSwitch.PropertyAdded = true
			hostSwitch.PropertyAddedAt = &now

			// Calculate time to add property
			if !hostSwitch.SwitchedAt.IsZero() {
				duration := now.Sub(hostSwitch.SwitchedAt)
				hostSwitch.TimeToAddProperty = &duration
			}

			storage.DB.Save(&hostSwitch)
			log.Printf("✅ User %d added property after %v", userID, hostSwitch.TimeToAddProperty)
		}

		// Record interaction for learning
		interaction := models.HostModeInteraction{
			UserID:          userID,
			InteractionType: "property_added",
			InteractionData: fmt.Sprintf(`{"property_id": %d, "property_type": "%s"}`, property.ID, property.PropertyType),
			CreatedAt:       time.Now(),
		}
		storage.DB.Create(&interaction)
	}

	// Sync amenity links into junction table (property_amenities)
	if len(input.Amenities) > 0 {
		for _, a := range input.Amenities {
			if id, err := strconv.Atoi(a); err == nil {
				// insert if not exists
				// Note: property_amenities only has (property_id, amenity_id), no is_active column
				storage.DB.Exec(`
                    INSERT INTO property_amenities (property_id, amenity_id)
                    VALUES (?, ?)
                    ON CONFLICT DO NOTHING
                `, property.ID, id)
			}
		}
	}

	// Auto-assign property to location criteria
	if err := AssignSinglePropertyToLocationCriteria(property.ID); err != nil {
		// Log the error but don't fail the property creation
		fmt.Printf("⚠️ Failed to auto-assign property %d to location criteria: %v\n", property.ID, err)
	}

	// Send push notification to all users with push tokens about the new property (in Arabic)
	go func() {
		allTokens := pushsvc.GetAllUsersWithPushTokens()
		if len(allTokens) == 0 {
			log.Printf("🔔 No users with push tokens found, skipping property notification")
			return
		}

		// Format Arabic notification message
		propertyTitle := property.Title
		if len(propertyTitle) > 40 {
			propertyTitle = propertyTitle[:40] + "…"
		}

		location := property.City
		if location == "" && property.State != "" {
			location = property.State
		}
		if location == "" {
			location = property.Country
		}
		if location == "" {
			location = "موقع جديد"
		}

		// Arabic message: "تمت إضافة عقار جديد: [title] في [location]"
		arabicTitle := fmt.Sprintf("تمت إضافة عقار جديد")
		arabicBody := fmt.Sprintf("تحقق من هذا العقار في %s: %s", location, propertyTitle)

		// If we have property images, use the first one as notification image
		var notificationImageURL string
		if property.Images != "" {
			var images []string
			if err := json.Unmarshal([]byte(property.Images), &images); err == nil && len(images) > 0 {
				notificationImageURL = images[0]
			}
		}

		// Send push notification to all users with image
		err := pushsvc.SendPushWithImage(allTokens, arabicTitle, arabicBody, notificationImageURL, nil)
		if err != nil {
			log.Printf("⚠️ Failed to send property notification to users: %v", err)
		} else {
			log.Printf("✅ Sent property notification to %d users: %s - %s", len(allTokens), arabicTitle, arabicBody)
		}
	}()

	ctx.JSON(property)
}

func GetProperty(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	property := GetPropertyAndAssociationsByPropertyID(id, ctx)
	if property == nil {
		return
	}

	// Resolve localized fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	property.Title = utils.ResolveLocalizedText(property.Title, property.TitleTranslations, lang)
	property.Description = utils.ResolveLocalizedText(property.Description, property.DescriptionTranslations, lang)
	property.NeighborhoodDescription = utils.ResolveLocalizedText(
		property.NeighborhoodDescription,
		property.NeighborhoodDescriptionTranslations,
		lang,
	)

	redactRentPropertyHostNote(property, optionalAuthUserID(ctx))
	redactRentPropertyBrokerProfile(property)

	ctx.JSON(property)
}

func GetPropertiesByUserID(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")
	excludeID := ctx.URLParam("exclude") // Optional: exclude a specific property ID

	// Resolve authenticated user ID (for "own list" vs "other's list")
	var authUserID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if uid, ok := v.(uint); ok {
			authUserID = uid
		}
	}
	if authUserID == 0 && ctx.GetHeader("Authorization") != "" {
		// Try to parse token for optional auth
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					authUserID = claims.ID
				}
			}
		}
	}

	hostID, _ := strconv.ParseUint(id, 10, 32)
	isOwnList := authUserID > 0 && uint(hostID) == authUserID

	// Host's own list: include pending so they see newly created properties
	// Use case-insensitive comparison to handle any status variations
	query := storage.DB.Preload("Host").Preload("Reviews").
		Where("host_id = ?", id).
		Where("COALESCE(is_active, true) = ?", true)

	if isOwnList {
		// Host sees their own pending properties too
		query = query.Where("LOWER(status) IN (?)", []string{"approved", "live", "published", "pending"})
	} else {
		// Public: only approved/live
		query = query.Where("LOWER(status) IN (?)", []string{"approved", "live", "published"})
	}

	// Exclude specific property if provided
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	// Apply user-specific exclusions if authenticated
	if authUserID > 0 {
		userID := authUserID
		query = query.Where("id NOT IN (SELECT property_id FROM hidden_properties WHERE user_id = ?)", userID)
		query = query.Where("id NOT IN (SELECT property_id FROM property_reports WHERE reporter_id = ?)", userID)
		query = query.Where("host_id NOT IN (SELECT flagged_user_id FROM user_flags WHERE flagger_id = ? AND status='active')", userID)
	}

	var properties []models.Property
	propertiesExist := query.Order("created_at DESC").Limit(20).Find(&properties)

	if propertiesExist.Error != nil {
		utils.CreateError(
			iris.StatusInternalServerError,
			"Error", propertiesExist.Error.Error(), ctx)
		return
	}

	for i := range properties {
		redactRentPropertyHostNote(&properties[i], authUserID)
	}

	ctx.JSON(properties)
}

// GetHostPropertiesByPropertyID returns other properties by the same host as the given property
// URL: /api/property/host-properties/{propertyID}?exclude={id}
func GetHostPropertiesByPropertyID(ctx iris.Context) {
	propertyID := ctx.Params().Get("id")
	excludeID := ctx.URLParam("exclude")

	// Load the property to find the host_id
	var base models.Property
	if err := storage.DB.Select("id, host_id").
		Where("id = ?", propertyID).
		First(&base).Error; err != nil {
		utils.CreateError(iris.StatusNotFound, "Not Found", "Property not found", ctx)
		return
	}

	// Build query for same host's properties
	// Use case-insensitive comparison to handle any status variations
	query := storage.DB.Preload("Host").Preload("Reviews").
		Where("host_id = ?", base.HostID).
		Where("is_active = ?", true).
		Where("LOWER(status) IN (?)", []string{"approved", "live", "published"})

	// Exclude the current property or any explicitly excluded id
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	} else {
		query = query.Where("id != ?", propertyID)
	}

	// Optional auth for user-specific exclusions
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
	if userID > 0 {
		query = query.Where("id NOT IN (SELECT property_id FROM hidden_properties WHERE user_id = ?)", userID)
		query = query.Where("id NOT IN (SELECT property_id FROM property_reports WHERE reporter_id = ?)", userID)
		query = query.Where("host_id NOT IN (SELECT flagged_user_id FROM user_flags WHERE flagger_id = ? AND status='active')", userID)
	}

	var properties []models.Property
	if err := query.Order("created_at DESC").Limit(20).Find(&properties).Error; err != nil {
		utils.CreateError(iris.StatusInternalServerError, "Error", err.Error(), ctx)
		return
	}
	for i := range properties {
		redactRentPropertyHostNote(&properties[i], userID)
	}
	ctx.JSON(properties)
}
func DeleteProperty(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	var property models.Property
	propertyExists := storage.DB.Find(&property, id)

	if propertyExists.RowsAffected == 0 {
		utils.CreateNotFound(ctx)
		return
	}

	claims := jwt.Get(ctx).(*utils.AccessToken)

	if property.HostID != claims.ID {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}

	propertyDeleted := storage.DB.Delete(&models.Property{}, id)

	if propertyDeleted.Error != nil {
		utils.CreateError(
			iris.StatusInternalServerError,
			"Error", propertyDeleted.Error.Error(), ctx)
		return
	}

	storage.DB.Where("property_id = ?", id).Delete(&models.Reservation{})
	ctx.StatusCode(iris.StatusNoContent)
}

func UpdateProperty(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	property := GetPropertyAndAssociationsByPropertyID(id, ctx)
	if property == nil {
		return
	}

	claims := jwt.Get(ctx).(*utils.AccessToken)

	if property.HostID != claims.ID {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}

	var input UpdateListingInput
	err := ctx.ReadJSON(&input)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	amenities, _ := json.Marshal(input.Amenities)

	imagesArr := insertImages(InsertImages{
		images:     input.Images,
		propertyID: strconv.FormatUint(uint64(property.ID), 10),
	})

	jsonImgs, _ := json.Marshal(imagesArr)

	property.Title = input.Title
	property.Description = input.Description
	property.PropertyType = input.PropertyType
	property.AddressLine1 = input.AddressLine1
	property.AddressLine2 = input.AddressLine2
	property.City = input.City
	property.State = input.State
	property.Zip = input.Zip
	property.Country = input.Country
	property.Lat = input.Lat
	property.Lng = input.Lng
	property.Capacity = input.Capacity
	property.Bedrooms = input.Bedrooms
	property.Beds = input.Beds
	property.Bathrooms = input.Bathrooms
	property.NightlyPrice = input.NightlyPrice
	property.CleaningFee = input.CleaningFee
	property.ServiceFee = input.ServiceFee
	property.Currency = input.Currency
	property.Amenities = string(amenities)
	property.HouseRules = input.HouseRules
	property.CancellationPolicy = input.CancellationPolicy
	property.Images = string(jsonImgs)
	property.IsActive = input.IsActive

	rowsUpdated := storage.DB.Model(&property).Updates(property)

	if rowsUpdated.Error != nil {
		utils.CreateError(
			iris.StatusInternalServerError,
			"Error", rowsUpdated.Error.Error(), ctx)
		return
	}

	// Auto-reassign property to location criteria if coordinates changed
	if err := AssignSinglePropertyToLocationCriteria(property.ID); err != nil {
		// Log the error but don't fail the property update
		fmt.Printf("⚠️ Failed to auto-reassign property %d to location criteria: %v\n", property.ID, err)
	}

	ctx.StatusCode(iris.StatusNoContent)
}

func GetPropertyAndAssociationsByPropertyID(id string, ctx iris.Context) *models.Property {

	var property models.Property
	propertyExists := storage.DB.Preload("Host").
		Preload("Reviews").
		Find(&property, id)

	if propertyExists.Error != nil {
		utils.CreateInternalServerError(ctx)
		return nil
	}

	if propertyExists.RowsAffected == 0 {
		utils.CreateNotFound(ctx)
		return nil
	}

	return &property
}

func GetPropertiesByBoundingBox(ctx iris.Context) {
	var boundingBox BoundingBoxInput
	err := ctx.ReadJSON(&boundingBox)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	fmt.Printf("GetPropertiesByBoundingBox - Searching in bounds: lat[%f-%f], lng[%f-%f]\n",
		boundingBox.LatLow, boundingBox.LatHigh, boundingBox.LngLow, boundingBox.LngHigh)

	// Extract userID for user-specific exclusions (optional auth)
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

	// Build base query
	query := storage.DB.Preload("Host").
		Preload("Reviews").
		Where("lat >= ? AND lat <= ? AND lng >= ? AND lng <= ? AND is_active = true AND COALESCE(is_flagged, false) = false AND LOWER(status) IN (?)",
			boundingBox.LatLow, boundingBox.LatHigh, boundingBox.LngLow, boundingBox.LngHigh, []string{"approved", "published"})

	// Apply user-specific exclusions if authenticated
	if userID > 0 {
		query = query.Where("id NOT IN (SELECT property_id FROM hidden_properties WHERE user_id = ?)", userID)
		query = query.Where("id NOT IN (SELECT property_id FROM property_reports WHERE reporter_id = ?)", userID)
		query = query.Where("host_id NOT IN (SELECT flagged_user_id FROM user_flags WHERE flagger_id = ? AND status='active')", userID)
	}

	var allProperties []models.Property
	result := query.Find(&allProperties)

	if result.Error != nil {
		fmt.Printf("GetPropertiesByBoundingBox - Database error: %v\n", result.Error)
		utils.CreateInternalServerError(ctx)
		return
	}

	fmt.Printf("GetPropertiesByBoundingBox - Found %d properties before rotation\n", len(allProperties))

	// Separate properties with images from those without
	var withImages []models.Property
	var withoutImages []models.Property
	
	for _, prop := range allProperties {
		// Check if property has images (Images is stored as JSON string)
		hasImages := false
		if prop.Images != "" {
			var images []string
			if err := json.Unmarshal([]byte(prop.Images), &images); err == nil {
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
			withImages = append(withImages, prop)
		} else {
			withoutImages = append(withoutImages, prop)
		}
	}

	fmt.Printf("📸 Properties with images: %d, without images: %d\n", len(withImages), len(withoutImages))

	// Apply time-based rotation for TikTok-like cycling
	// Rotation changes every hour, creating variety for users
	now := time.Now()
	rotationSeed := int64(now.Hour() + now.Day()*24 + int(now.Month())*24*31)
	
	// Rotate both lists
	rotateProperties := func(props []models.Property, seed int64) []models.Property {
		if len(props) == 0 {
			return props
		}
		offset := int(seed % int64(len(props)))
		if offset == 0 {
			return props
		}
		rotated := make([]models.Property, len(props))
		copy(rotated, props[offset:])
		copy(rotated[len(props)-offset:], props[:offset])
		return rotated
	}

	withImages = rotateProperties(withImages, rotationSeed)
	withoutImages = rotateProperties(withoutImages, rotationSeed+1500) // Different offset for variety

	// Combine: properties with images first, then without
	var properties []models.Property
	properties = append(properties, withImages...)
	properties = append(properties, withoutImages...)

	fmt.Printf("✅ Returning %d properties (rotated, images prioritized)\n", len(properties))

	// Resolve localized fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
		p.NeighborhoodDescription = utils.ResolveLocalizedText(
			p.NeighborhoodDescription,
			p.NeighborhoodDescriptionTranslations,
			lang,
		)
	}

	for i := range properties {
		redactRentPropertyHostNote(&properties[i], userID)
	}

	ctx.JSON(properties)
}

func insertImages(arg InsertImages) []string {
	var imagesArr []string
	for i, image := range arg.images {
		if image == "" {
			continue // Skip empty strings
		}
		trimmed := strings.TrimSpace(image)
		if trimmed == "" {
			continue
		}

		// If the client already provided a hosted URL (GCS, Cloudinary, etc),
		// store it as-is. Trying to "upload" an https URL as base64 will fail
		// and silently drop the image, resulting in [] persisted.
		if strings.HasPrefix(trimmed, "http://") ||
			strings.HasPrefix(trimmed, "https://") ||
			strings.Contains(trimmed, "res.cloudinary.com") ||
			strings.Contains(trimmed, "storage.googleapis.com") {
			imagesArr = append(imagesArr, trimmed)
			continue
		}

		// Otherwise, treat it as base64 / data URL and upload.
		if true {
			// Generate unique filename with timestamp and index
			timestamp := time.Now().UnixNano() / int64(time.Millisecond) // milliseconds since epoch
			publicID := fmt.Sprintf("property_%d_%d", timestamp, i)

			if arg.propertyID != "" {
				publicID = "property/" + arg.propertyID + "/" + publicID
			}
			if arg.apartmentID != nil {
				publicID = "property/" + arg.propertyID + "/apartment/" + *arg.apartmentID + "/" + publicID
			}

			fmt.Printf("Uploading image with publicID: %s\n", publicID)
			urlMap := storage.UploadBase64Image(trimmed, publicID)
			if urlMap != nil && urlMap["url"] != "" {
				imagesArr = append(imagesArr, urlMap["url"])
				fmt.Printf("Successfully uploaded image: %s\n", urlMap["url"])
			} else {
				// Log error but continue
				fmt.Printf("Failed to upload image with publicID: %s\n", publicID)
			}
		}
	}
	return imagesArr
}

// DeletePropertyImage deletes a single image from a property
func DeletePropertyImage(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Get parameters from query string instead of body
	propertyIDStr := ctx.URLParam("propertyID")
	imageURL := ctx.URLParam("imageURL")

	fmt.Printf("DEBUG: Received propertyID: %s\n", propertyIDStr)
	fmt.Printf("DEBUG: Received imageURL: %s\n", imageURL)

	if propertyIDStr == "" || imageURL == "" {
		fmt.Printf("ERROR: Missing parameters - propertyID: '%s', imageURL: '%s'\n", propertyIDStr, imageURL)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message": "propertyID and imageURL are required",
		})
		return
	}

	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message": "Invalid propertyID",
		})
		return
	}

	// Verify the property belongs to the user
	var property models.Property
	if err := storage.DB.Where("id = ? AND host_id = ?", propertyID, userID).First(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{
			"message": "Property not found or not owned by user",
		})
		return
	}

	// Parse existing images
	var images []string
	if property.Images != "" {
		if err := json.Unmarshal([]byte(property.Images), &images); err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{
				"message": "Failed to parse property images",
			})
			return
		}
	}

	// Find and remove the image
	imageIndex := -1
	for i, img := range images {
		if img == imageURL {
			imageIndex = i
			break
		}
	}

	if imageIndex == -1 {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{
			"message": "Image not found in property",
		})
		return
	}

	// Remove image from array
	images = append(images[:imageIndex], images[imageIndex+1:]...)

	// Update property with new images array
	imagesJSON, _ := json.Marshal(images)
	property.Images = string(imagesJSON)

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{
			"message": "Failed to update property",
		})
		return
	}

	// Delete image from configured CDN when possible
	if storage.DeleteMedia(imageURL) {
		ctx.JSON(iris.Map{
			"message": "Image deleted successfully",
			"success": true,
		})
	} else {
		// Even if Cloudinary deletion fails, we've removed it from the database
		// Log the error but don't fail the request
		fmt.Printf("WARNING: Failed to delete image from CDN: %s\n", imageURL)
		ctx.JSON(iris.Map{
			"message": "Image removed from property (CDN deletion may have failed)",
			"success": true,
		})
	}
}

type InsertImages struct {
	images      []string
	propertyID  string
	apartmentID *string
}

type CreateListingInput struct {
	HostID             uint     `json:"hostID" validate:"required"`
	Title              string   `json:"title" validate:"required,max=256"`
	Description        string   `json:"description"`
	PropertyType       string   `json:"propertyType" validate:"required,oneof=entire_place private_room shared_room"`
	AddressLine1       string   `json:"addressLine1" validate:"required,max=512"`
	AddressLine2       string   `json:"addressLine2" validate:"max=512"`
	City               string   `json:"city" validate:"required,max=256"`
	State              string   `json:"state" validate:"required,max=256"`
	Zip                string   `json:"zip" validate:"required,max=32"`
	Country            string   `json:"country" validate:"required,max=128"`
	CountryID          *uint    `json:"country_id"`
	CityID             *uint    `json:"city_id"`
	ZoneID             *uint    `json:"zone_id"`
	QuartierID         *uint    `json:"quartier_id"`
	Lat                float32  `json:"lat" validate:"required"`
	Lng                float32  `json:"lng" validate:"required"`
	Capacity           int      `json:"capacity" validate:"required,gte=1,lte=16"`
	Bedrooms           int      `json:"bedrooms" validate:"required,gte=0,lte=10"`
	Beds               int      `json:"beds" validate:"required,gte=0,lte=20"`
	Bathrooms          float32  `json:"bathrooms" validate:"required,gte=0,lte=10"`
	NightlyPrice       float32  `json:"nightlyPrice" validate:"required,gte=0"`
	CleaningFee        float32  `json:"cleaningFee"`
	ServiceFee         float32  `json:"serviceFee"`
	Currency           string   `json:"currency" validate:"required"`
	Amenities          []string `json:"amenities"`
	HouseRules         string   `json:"houseRules"`
	CancellationPolicy string   `json:"cancellationPolicy,omitempty"`
	Images             []string `json:"images"`
	IsActive           *bool    `json:"isActive"`

	// New policy fields
	BookingMode                      string `json:"bookingMode"`
	SecureCompoundAcknowledged       *bool  `json:"secureCompoundAcknowledged,omitempty"`
	EquipmentViolationPolicyAccepted *bool  `json:"equipmentViolationPolicyAccepted,omitempty"`
	UserSafetyPolicyAccepted         *bool  `json:"userSafetyPolicyAccepted,omitempty"`
	PropertyPolicyAccepted           *bool  `json:"propertyPolicyAccepted,omitempty"`

	// Neighborhood & timing & category mapping
	NeighborhoodDescription string              `json:"neighborhoodDescription"`
	NearbyAttractions       []map[string]string `json:"nearbyAttractions"`
	CheckInTime             string              `json:"checkInTime"`
	CheckOutTime            string              `json:"checkOutTime"`
	PropertyCategoryId      uint                `json:"propertyCategoryId"`

	// Optional host-only note (never shown to guests on public reads)
	HostPrivateNote string `json:"hostPrivateNote"`
}

type UpdateListingInput struct {
	Title              string   `json:"title" validate:"required,max=256"`
	Description        string   `json:"description"`
	PropertyType       string   `json:"propertyType" validate:"required,oneof=entire_place private_room shared_room"`
	AddressLine1       string   `json:"addressLine1" validate:"required,max=512"`
	AddressLine2       string   `json:"addressLine2" validate:"max=512"`
	City               string   `json:"city" validate:"required,max=256"`
	State              string   `json:"state" validate:"required,max=256"`
	Zip                string   `json:"zip" validate:"required,max=32"`
	Country            string   `json:"country" validate:"required,max=128"`
	Lat                float32  `json:"lat" validate:"required"`
	Lng                float32  `json:"lng" validate:"required"`
	Capacity           int      `json:"capacity" validate:"required,gte=1,lte=16"`
	Bedrooms           int      `json:"bedrooms" validate:"required,gte=0,lte=10"`
	Beds               int      `json:"beds" validate:"required,gte=0,lte=20"`
	Bathrooms          float32  `json:"bathrooms" validate:"required,gte=0,lte=10"`
	NightlyPrice       float32  `json:"nightlyPrice" validate:"required,gte=0"`
	CleaningFee        float32  `json:"cleaningFee"`
	ServiceFee         float32  `json:"serviceFee"`
	Currency           string   `json:"currency" validate:"required"`
	Amenities          []string `json:"amenities"`
	HouseRules         string   `json:"houseRules"`
	CancellationPolicy string   `json:"cancellationPolicy,omitempty"`
	Images             []string `json:"images"`
	IsActive           *bool    `json:"isActive"`
}

type BoundingBoxInput struct {
	LatLow  float32 `json:"latLow" validate:"required"`
	LatHigh float32 `json:"latHigh" validate:"required"`
	LngLow  float32 `json:"lngLow" validate:"required"`
	LngHigh float32 `json:"lngHigh" validate:"required"`
}
