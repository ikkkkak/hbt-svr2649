package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"net/http"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

type AddToWishlistInput struct {
	PropertyID uint `json:"propertyID"`
}

// GetUserWishlist - Get user's personal wishlist
func GetUserWishlist(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	// Get user's saved properties
	var userModel models.User
	if err := storage.DB.First(&userModel, user.ID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Parse saved properties JSON
	var savedPropertyIDs []uint
	if userModel.SavedProperties != nil {
		if err := json.Unmarshal(userModel.SavedProperties, &savedPropertyIDs); err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
	}

	// If no saved properties, return empty array
	if len(savedPropertyIDs) == 0 {
		ctx.JSON(iris.Map{"success": true, "properties": []models.Property{}})
		return
	}

	// Fetch properties with their details
	var properties []models.Property
	if err := storage.DB.Where("id IN ?", savedPropertyIDs).Find(&properties).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true, "properties": properties})
}

// AddToUserWishlist - Add property to user's personal wishlist
func AddToUserWishlist(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var input AddToWishlistInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Verify property exists
	var property models.Property
	if err := storage.DB.First(&property, input.PropertyID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Get user's current saved properties
	var userModel models.User
	if err := storage.DB.First(&userModel, user.ID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Parse existing saved properties
	var savedPropertyIDs []uint
	if userModel.SavedProperties != nil {
		if err := json.Unmarshal(userModel.SavedProperties, &savedPropertyIDs); err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
	}

	// Check if property is already saved
	for _, id := range savedPropertyIDs {
		if id == input.PropertyID {
			ctx.JSON(iris.Map{"success": true, "message": "Property already in wishlist"})
			return
		}
	}

	// Add property to saved list
	savedPropertyIDs = append(savedPropertyIDs, input.PropertyID)

	// Marshal the updated property IDs to JSON
	marshaledProperties, err := json.Marshal(savedPropertyIDs)
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Update user's saved properties
	if err := storage.DB.Model(&userModel).Update("saved_properties", marshaledProperties).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": "Property added to wishlist"})
}

// RemoveFromUserWishlist - Remove property from user's personal wishlist
func RemoveFromUserWishlist(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	propertyID, err := ctx.Params().GetUint("propertyID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Get user's current saved properties
	var userModel models.User
	if err := storage.DB.First(&userModel, user.ID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Parse existing saved properties
	var savedPropertyIDs []uint
	if userModel.SavedProperties != nil {
		if err := json.Unmarshal(userModel.SavedProperties, &savedPropertyIDs); err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
	}

	// Remove property from saved list
	var updatedPropertyIDs []uint
	for _, id := range savedPropertyIDs {
		if id != propertyID {
			updatedPropertyIDs = append(updatedPropertyIDs, id)
		}
	}

	// Marshal the updated property IDs to JSON
	marshaledProperties, err := json.Marshal(updatedPropertyIDs)
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Update user's saved properties
	if err := storage.DB.Model(&userModel).Update("saved_properties", marshaledProperties).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": "Property removed from wishlist"})
}
