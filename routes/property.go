package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

func CreateProperty(ctx iris.Context) {
	var input CreateListingInput

	err := ctx.ReadJSON(&input)
	if err != nil {
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
		SecureCompoundAcknowledged:       input.SecureCompoundAcknowledged,
		EquipmentViolationPolicyAccepted: input.EquipmentViolationPolicyAccepted,
		UserSafetyPolicyAccepted:         input.UserSafetyPolicyAccepted,
		PropertyPolicyAccepted:           input.PropertyPolicyAccepted,
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

	// Sync amenity links into junction table (property_amenities)
	if len(input.Amenities) > 0 {
		for _, a := range input.Amenities {
			if id, err := strconv.Atoi(a); err == nil {
				// insert if not exists
				storage.DB.Exec(`
                    INSERT INTO property_amenities (property_id, amenity_id, is_active, created_at, updated_at)
                    VALUES (?, ?, TRUE, NOW(), NOW())
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

	ctx.JSON(property)
}

func GetProperty(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	property := GetPropertyAndAssociationsByPropertyID(id, ctx)
	if property == nil {
		return
	}

	ctx.JSON(property)
}

func GetPropertiesByUserID(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	var properties []models.Property
	propertiesExist := storage.DB.Preload(clause.Associations).Where("host_id = ?", id).Find(&properties)

	if propertiesExist.Error != nil {
		utils.CreateError(
			iris.StatusInternalServerError,
			"Error", propertiesExist.Error.Error(), ctx)
		return
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

	var properties []models.Property
	result := storage.DB.Preload("Host").
		Preload("Reviews").
		Where("lat >= ? AND lat <= ? AND lng >= ? AND lng <= ? AND is_active = true AND status IN (?)",
			boundingBox.LatLow, boundingBox.LatHigh, boundingBox.LngLow, boundingBox.LngHigh, []string{"approved", "live"}).
		Order("created_at DESC").
		Find(&properties)

	if result.Error != nil {
		fmt.Printf("GetPropertiesByBoundingBox - Database error: %v\n", result.Error)
		utils.CreateInternalServerError(ctx)
		return
	}

	fmt.Printf("GetPropertiesByBoundingBox - Found %d properties\n", len(properties))

	// Debug: Log property details
	for i, property := range properties {
		fmt.Printf("Property %d - ID: %d, Title: '%s', City: '%s', Price: %.2f, Host: %s %s\n",
			i, property.ID, property.Title, property.City, property.NightlyPrice,
			property.Host.FirstName, property.Host.LastName)
	}

	ctx.JSON(properties)
}

func insertImages(arg InsertImages) []string {
	var imagesArr []string
	for i, image := range arg.images {
		if image == "" {
			continue // Skip empty strings
		}
		if !(strings.Contains(image, "res.cloudinary.com")) {
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
			urlMap := storage.UploadBase64Image(image, publicID)
			if urlMap != nil && urlMap["url"] != "" {
				imagesArr = append(imagesArr, urlMap["url"])
				fmt.Printf("Successfully uploaded image: %s\n", urlMap["url"])
			} else {
				// Log error but continue
				fmt.Printf("Failed to upload image to Cloudinary with publicID: %s\n", publicID)
			}
		} else {
			imagesArr = append(imagesArr, image)
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

	// Delete image from Cloudinary
	if storage.DeleteImageFromCloudinary(imageURL) {
		ctx.JSON(iris.Map{
			"message": "Image deleted successfully",
			"success": true,
		})
	} else {
		// Even if Cloudinary deletion fails, we've removed it from the database
		// Log the error but don't fail the request
		fmt.Printf("WARNING: Failed to delete image from Cloudinary: %s\n", imageURL)
		ctx.JSON(iris.Map{
			"message": "Image removed from property (Cloudinary deletion may have failed)",
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
	CancellationPolicy string   `json:"cancellationPolicy"`
	Images             []string `json:"images"`
	IsActive           *bool    `json:"isActive"`

	// New policy fields
	BookingMode                      string `json:"bookingMode"`
	SecureCompoundAcknowledged       bool   `json:"secureCompoundAcknowledged"`
	EquipmentViolationPolicyAccepted bool   `json:"equipmentViolationPolicyAccepted"`
	UserSafetyPolicyAccepted         bool   `json:"userSafetyPolicyAccepted"`
	PropertyPolicyAccepted           bool   `json:"propertyPolicyAccepted"`

	// Neighborhood & timing & category mapping
	NeighborhoodDescription string              `json:"neighborhoodDescription"`
	NearbyAttractions       []map[string]string `json:"nearbyAttractions"`
	CheckInTime             string              `json:"checkInTime"`
	CheckOutTime            string              `json:"checkOutTime"`
	PropertyCategoryId      uint                `json:"propertyCategoryId"`
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
	CancellationPolicy string   `json:"cancellationPolicy"`
	Images             []string `json:"images"`
	IsActive           *bool    `json:"isActive"`
}

type BoundingBoxInput struct {
	LatLow  float32 `json:"latLow" validate:"required"`
	LatHigh float32 `json:"latHigh" validate:"required"`
	LngLow  float32 `json:"lngLow" validate:"required"`
	LngHigh float32 `json:"lngHigh" validate:"required"`
}
