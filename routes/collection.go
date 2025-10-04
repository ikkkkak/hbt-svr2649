package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

// CreateCollection creates a new collection
func CreateCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		Name        string `json:"name" validate:"required,max=100"`
		Description string `json:"description" validate:"max=500"`
		Color       string `json:"color" validate:"max=20"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Set default color if not provided
	if input.Color == "" {
		input.Color = "#FF385C"
	}

	collection := models.Collection{
		UserID:      userID,
		Name:        input.Name,
		Description: input.Description,
		Color:       input.Color,
		IsDefault:   false,
	}

	fmt.Printf("Creating collection: UserID=%d, Name=%s, Description=%s, Color=%s\n",
		userID, input.Name, input.Description, input.Color)

	if err := storage.DB.Create(&collection).Error; err != nil {
		fmt.Printf("Error creating collection: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	fmt.Printf("Collection created successfully: ID=%d\n", collection.ID)

	ctx.JSON(iris.Map{
		"success":    true,
		"collection": collection,
	})
}

// GetUserCollections gets all collections for a user
func GetUserCollections(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var collections []models.Collection
	if err := storage.DB.Where("user_id = ?", userID).
		Preload("Properties.Property").
		Order("created_at DESC").
		Find(&collections).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":     true,
		"collections": collections,
	})
}

// UpdateCollection updates a collection
func UpdateCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	collectionID := ctx.Params().Get("id")

	var input struct {
		Name        string `json:"name" validate:"max=100"`
		Description string `json:"description" validate:"max=500"`
		Color       string `json:"color" validate:"max=20"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var collection models.Collection
	if err := storage.DB.Where("id = ? AND user_id = ?", collectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Update fields
	if input.Name != "" {
		collection.Name = input.Name
	}
	if input.Description != "" {
		collection.Description = input.Description
	}
	if input.Color != "" {
		collection.Color = input.Color
	}

	if err := storage.DB.Save(&collection).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"collection": collection,
	})
}

// DeleteCollection deletes a collection
func DeleteCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	collectionID := ctx.Params().Get("id")

	// Check if collection exists and belongs to user
	var collection models.Collection
	if err := storage.DB.Where("id = ? AND user_id = ?", collectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Delete collection properties first
	storage.DB.Where("collection_id = ?", collectionID).Delete(&models.CollectionProperty{})

	// Delete collection
	if err := storage.DB.Delete(&collection).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Collection deleted successfully",
	})
}

// AddPropertyToCollection adds a property to a collection
func AddPropertyToCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		CollectionID uint `json:"collectionID" validate:"required"`
		PropertyID   uint `json:"propertyID" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if collection belongs to user
	var collection models.Collection
	if err := storage.DB.Where("id = ? AND user_id = ?", input.CollectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Check if property already exists in collection
	var existingCollectionProperty models.CollectionProperty
	if err := storage.DB.Where("collection_id = ? AND property_id = ?", input.CollectionID, input.PropertyID).
		First(&existingCollectionProperty).Error; err == nil {
		ctx.StatusCode(iris.StatusConflict)
		ctx.JSON(iris.Map{"error": "Property already in collection"})
		return
	}

	// Add property to collection
	collectionProperty := models.CollectionProperty{
		CollectionID: input.CollectionID,
		PropertyID:   input.PropertyID,
	}

	if err := storage.DB.Create(&collectionProperty).Error; err != nil {
		fmt.Printf("Error creating collection property: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Also add to user's SavedProperties for backward compatibility
	var user models.User
	if err := storage.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		fmt.Printf("Error finding user: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Parse existing saved properties
	var savedProperties []uint
	if user.SavedProperties != nil {
		json.Unmarshal(user.SavedProperties, &savedProperties)
	}

	// Check if property is already in saved properties
	propertyExists := false
	for _, propID := range savedProperties {
		if propID == input.PropertyID {
			propertyExists = true
			break
		}
	}

	// Add property to saved properties if not already there
	if !propertyExists {
		savedProperties = append(savedProperties, input.PropertyID)
		savedPropertiesJSON, _ := json.Marshal(savedProperties)
		user.SavedProperties = savedPropertiesJSON

		if err := storage.DB.Save(&user).Error; err != nil {
			fmt.Printf("Error updating user saved properties: %v\n", err)
			// Don't fail the request, just log the error
		}
	}

	fmt.Printf("Property %d added to collection %d and user %d saved properties\n",
		input.PropertyID, input.CollectionID, userID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Property added to collection",
	})
}

// RemovePropertyFromCollection removes a property from a collection
func RemovePropertyFromCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		CollectionID uint `json:"collectionID" validate:"required"`
		PropertyID   uint `json:"propertyID" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if collection belongs to user
	var collection models.Collection
	if err := storage.DB.Where("id = ? AND user_id = ?", input.CollectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Remove property from collection
	if err := storage.DB.Where("collection_id = ? AND property_id = ?", input.CollectionID, input.PropertyID).
		Delete(&models.CollectionProperty{}).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Check if property exists in any other collections for this user
	var otherCollectionsCount int64
	storage.DB.Model(&models.CollectionProperty{}).
		Joins("JOIN collections ON collection_properties.collection_id = collections.id").
		Where("collections.user_id = ? AND collection_properties.property_id = ?", userID, input.PropertyID).
		Count(&otherCollectionsCount)

	// If property is not in any other collections, remove from SavedProperties
	if otherCollectionsCount == 0 {
		var user models.User
		if err := storage.DB.Where("id = ?", userID).First(&user).Error; err == nil {
			var savedProperties []uint
			if user.SavedProperties != nil {
				json.Unmarshal(user.SavedProperties, &savedProperties)
			}

			// Remove property from saved properties
			var newSavedProperties []uint
			for _, propID := range savedProperties {
				if propID != input.PropertyID {
					newSavedProperties = append(newSavedProperties, propID)
				}
			}

			savedPropertiesJSON, _ := json.Marshal(newSavedProperties)
			user.SavedProperties = savedPropertiesJSON
			storage.DB.Save(&user)
		}
	}

	fmt.Printf("Property %d removed from collection %d\n", input.PropertyID, input.CollectionID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Property removed from collection",
	})
}

// GetCollectionProperties gets all properties in a collection
func GetCollectionProperties(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	collectionID := ctx.Params().Get("id")

	// Check if collection belongs to user
	var collection models.Collection
	if err := storage.DB.Where("id = ? AND user_id = ?", collectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	var collectionProperties []models.CollectionProperty
	if err := storage.DB.Where("collection_id = ?", collectionID).
		Preload("Property").
		Order("added_at DESC").
		Find(&collectionProperties).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Extract properties
	var properties []models.Property
	for _, cp := range collectionProperties {
		properties = append(properties, cp.Property)
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"properties": properties,
	})
}

// RemovePropertyFromAllCollections removes a property from all collections for a user
func RemovePropertyFromAllCollections(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		PropertyID uint `json:"propertyID" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Remove property from all collections for this user
	if err := storage.DB.Where("collection_id IN (SELECT id FROM collections WHERE user_id = ?) AND property_id = ?", userID, input.PropertyID).
		Delete(&models.CollectionProperty{}).Error; err != nil {
		fmt.Printf("Error removing property from all collections: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Also remove from user's SavedProperties
	var user models.User
	if err := storage.DB.Where("id = ?", userID).First(&user).Error; err == nil {
		var savedProperties []uint
		if user.SavedProperties != nil {
			json.Unmarshal(user.SavedProperties, &savedProperties)
		}

		// Remove property from saved properties
		var newSavedProperties []uint
		for _, propID := range savedProperties {
			if propID != input.PropertyID {
				newSavedProperties = append(newSavedProperties, propID)
			}
		}

		savedPropertiesJSON, _ := json.Marshal(newSavedProperties)
		user.SavedProperties = savedPropertiesJSON
		storage.DB.Save(&user)
	}

	fmt.Printf("Property %d removed from all collections for user %d\n", input.PropertyID, userID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Property removed from all collections",
	})
}
