package routes

import (
	"encoding/json"
	"fmt"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
)

// CreateExperienceCollection creates a new experience collection
func CreateExperienceCollection(ctx iris.Context) {
	fmt.Println("DEBUG: CreateExperienceCollection called")
	
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	fmt.Printf("DEBUG: User ID: %d\n", userID)

	var input struct {
		Name        string `json:"name" validate:"required"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		fmt.Printf("DEBUG: Error reading JSON: %v\n", err)
		utils.HandleValidationErrors(err, ctx)
		return
	}
	
	fmt.Printf("DEBUG: Input received: %+v\n", input)

	// Set default color if not provided
	if input.Color == "" {
		input.Color = "#00A699"
	}

	collection := models.ExperienceCollection{
		UserID:      userID,
		Name:        input.Name,
		Description: input.Description,
		Color:       input.Color,
		IsDefault:   false,
	}

	fmt.Printf("DEBUG: Creating collection: %+v\n", collection)
	
	if err := storage.DB.Create(&collection).Error; err != nil {
		fmt.Printf("DEBUG: Error creating experience collection: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	fmt.Printf("DEBUG: Collection created successfully with ID: %d\n", collection.ID)
	
	ctx.JSON(iris.Map{
		"success":    true,
		"message":    "Experience collection created",
		"collection": collection,
	})
}

// GetUserExperienceCollections gets all experience collections for a user
func GetUserExperienceCollections(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var collections []models.ExperienceCollection
	if err := storage.DB.Where("user_id = ?", userID).
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

// UpdateExperienceCollection updates an experience collection
func UpdateExperienceCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	collectionID := ctx.Params().Get("id")

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if collection belongs to user
	var collection models.ExperienceCollection
	if err := storage.DB.Where("id = ? AND user_id = ?", collectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Update collection
	updates := make(map[string]interface{})
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Color != "" {
		updates["color"] = input.Color
	}

	if err := storage.DB.Model(&collection).Updates(updates).Error; err != nil {
		fmt.Printf("Error updating experience collection: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"message":    "Experience collection updated",
		"collection": collection,
	})
}

// DeleteExperienceCollection deletes an experience collection
func DeleteExperienceCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	collectionID := ctx.Params().Get("id")

	// Check if collection belongs to user
	var collection models.ExperienceCollection
	if err := storage.DB.Where("id = ? AND user_id = ?", collectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Soft delete collection
	if err := storage.DB.Delete(&collection).Error; err != nil {
		fmt.Printf("Error deleting experience collection: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Experience collection deleted",
	})
}

// AddExperienceToCollection adds an experience to a collection
func AddExperienceToCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		CollectionID uint `json:"collectionID" validate:"required"`
		ExperienceID uint `json:"experienceID" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if collection belongs to user
	var collection models.ExperienceCollection
	if err := storage.DB.Where("id = ? AND user_id = ?", input.CollectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Check if experience exists
	var experience models.Experience
	if err := storage.DB.Where("id = ?", input.ExperienceID).First(&experience).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Experience not found"})
		return
	}

	// Check if experience already exists in collection
	var existingCollectionExperience models.ExperienceCollectionItem
	if err := storage.DB.Where("collection_id = ? AND experience_id = ?", input.CollectionID, input.ExperienceID).
		First(&existingCollectionExperience).Error; err == nil {
		ctx.StatusCode(iris.StatusConflict)
		ctx.JSON(iris.Map{"error": "Experience already in collection"})
		return
	}

	// Add experience to collection
	collectionExperience := models.ExperienceCollectionItem{
		CollectionID: input.CollectionID,
		ExperienceID: input.ExperienceID,
	}

	if err := storage.DB.Create(&collectionExperience).Error; err != nil {
		fmt.Printf("Error creating collection experience: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Also add to user's SavedExperiences for backward compatibility
	var user models.User
	if err := storage.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		fmt.Printf("Error finding user: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Parse existing saved experiences
	var savedExperiences []uint
	if user.SavedExperiences != nil {
		json.Unmarshal(user.SavedExperiences, &savedExperiences)
	}

	// Check if experience is already in saved experiences
	experienceExists := false
	for _, expID := range savedExperiences {
		if expID == input.ExperienceID {
			experienceExists = true
			break
		}
	}

	// Add experience to saved experiences if not already there
	if !experienceExists {
		savedExperiences = append(savedExperiences, input.ExperienceID)
		savedExperiencesJSON, _ := json.Marshal(savedExperiences)
		user.SavedExperiences = savedExperiencesJSON

		if err := storage.DB.Save(&user).Error; err != nil {
			fmt.Printf("Error updating user saved experiences: %v\n", err)
			// Don't fail the request, just log the error
		}
	}

	fmt.Printf("Experience %d added to collection %d and user %d saved experiences\n",
		input.ExperienceID, input.CollectionID, userID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Experience added to collection",
	})
}

// RemoveExperienceFromCollection removes an experience from a collection
func RemoveExperienceFromCollection(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		CollectionID uint `json:"collectionID" validate:"required"`
		ExperienceID uint `json:"experienceID" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if collection belongs to user
	var collection models.ExperienceCollection
	if err := storage.DB.Where("id = ? AND user_id = ?", input.CollectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	// Remove experience from collection
	if err := storage.DB.Where("collection_id = ? AND experience_id = ?", input.CollectionID, input.ExperienceID).
		Delete(&models.ExperienceCollectionItem{}).Error; err != nil {
		fmt.Printf("Error removing experience from collection: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Check if experience is in any other collections
	var otherCollectionsCount int64
	storage.DB.Model(&models.ExperienceCollectionItem{}).
		Where("experience_id = ? AND collection_id != ?", input.ExperienceID, input.CollectionID).
		Count(&otherCollectionsCount)

	// If experience is not in any other collections, remove from SavedExperiences
	if otherCollectionsCount == 0 {
		var user models.User
		if err := storage.DB.Where("id = ?", userID).First(&user).Error; err == nil {
			var savedExperiences []uint
			if user.SavedExperiences != nil {
				json.Unmarshal(user.SavedExperiences, &savedExperiences)
			}

			// Remove experience from saved experiences
			var newSavedExperiences []uint
			for _, expID := range savedExperiences {
				if expID != input.ExperienceID {
					newSavedExperiences = append(newSavedExperiences, expID)
				}
			}

			savedExperiencesJSON, _ := json.Marshal(newSavedExperiences)
			user.SavedExperiences = savedExperiencesJSON
			storage.DB.Save(&user)
		}
	}

	fmt.Printf("Experience %d removed from collection %d\n", input.ExperienceID, input.CollectionID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Experience removed from collection",
	})
}

// GetCollectionExperiences gets all experiences in a collection
func GetCollectionExperiences(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	collectionID := ctx.Params().Get("id")

	// Check if collection belongs to user
	var collection models.ExperienceCollection
	if err := storage.DB.Where("id = ? AND user_id = ?", collectionID, userID).
		First(&collection).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Collection not found"})
		return
	}

	var collectionExperiences []models.ExperienceCollectionItem
	if err := storage.DB.Where("collection_id = ?", collectionID).
		Preload("Experience").
		Preload("Experience.Host").
		Order("added_at DESC").
		Find(&collectionExperiences).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Extract experiences
	var experiences []models.Experience
	for _, ce := range collectionExperiences {
		experiences = append(experiences, ce.Experience)
	}

	ctx.JSON(iris.Map{
		"success":     true,
		"experiences": experiences,
	})
}

// RemoveExperienceFromAllCollections removes an experience from all collections for a user
func RemoveExperienceFromAllCollections(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		ExperienceID uint `json:"experienceID" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Get all collections for user
	var collections []models.ExperienceCollection
	if err := storage.DB.Where("user_id = ?", userID).Find(&collections).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Remove experience from all collections
	for _, collection := range collections {
		storage.DB.Where("collection_id = ? AND experience_id = ?", collection.ID, input.ExperienceID).
			Delete(&models.ExperienceCollectionItem{})
	}

	// Remove from user's SavedExperiences
	var user models.User
	if err := storage.DB.Where("id = ?", userID).First(&user).Error; err == nil {
		var savedExperiences []uint
		if user.SavedExperiences != nil {
			json.Unmarshal(user.SavedExperiences, &savedExperiences)
		}

		// Remove experience from saved experiences
		var newSavedExperiences []uint
		for _, expID := range savedExperiences {
			if expID != input.ExperienceID {
				newSavedExperiences = append(newSavedExperiences, expID)
			}
		}

		savedExperiencesJSON, _ := json.Marshal(newSavedExperiences)
		user.SavedExperiences = savedExperiencesJSON
		storage.DB.Save(&user)
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Experience removed from all collections",
	})
}

// GetUserSavedExperiences gets all saved experiences for a user
func GetUserSavedExperiences(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var user models.User
	if err := storage.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	var savedExperiences []uint
	if user.SavedExperiences != nil {
		json.Unmarshal(user.SavedExperiences, &savedExperiences)
	}

	var experiences []models.Experience
	if len(savedExperiences) > 0 {
		if err := storage.DB.Where("id IN ?", savedExperiences).
			Preload("Host").
			Find(&experiences).Error; err != nil {
			utils.CreateInternalServerError(ctx)
			return
		}
	}

	ctx.JSON(iris.Map{
		"success":     true,
		"experiences": experiences,
	})
}
