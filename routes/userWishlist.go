package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"log"
	"net/http"

	"github.com/kataras/iris/v12"
)

type AddToWishlistInput struct {
	PropertyID uint `json:"propertyID"`
}

// GetUserWishlist - Get user's personal wishlist
func GetUserWishlist(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetUserWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	// Get user's saved properties
	var userModel models.User
	if err := storage.DB.First(&userModel, userID).Error; err != nil {
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
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ AddToUserWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

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
	if err := storage.DB.First(&userModel, userID).Error; err != nil {
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
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ RemoveFromUserWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	propertyID, err := ctx.Params().GetUint("propertyID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Get user's current saved properties
	var userModel models.User
	if err := storage.DB.First(&userModel, userID).Error; err != nil {
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

// ============================================
// PROPERTY SALE WISHLIST ENDPOINTS
// ============================================

type AddPropertySaleToWishlistInput struct {
	PropertySaleID uint `json:"propertySaleID"`
}

// GetUserPropertySaleWishlist - Get user's saved property sales
func GetUserPropertySaleWishlist(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetUserPropertySaleWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	// Get user's saved property sales
	var userModel models.User
	if err := storage.DB.First(&userModel, userID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Parse saved property sale IDs
	var savedPropertySaleIDs []uint
	if userModel.SavedPropertySales != nil {
		if err := json.Unmarshal(userModel.SavedPropertySales, &savedPropertySaleIDs); err != nil {
			log.Printf("⚠️ Failed to parse SavedPropertySales: %v", err)
		}
	}

	// If no saved property sales, return empty array
	if len(savedPropertySaleIDs) == 0 {
		ctx.JSON(iris.Map{"success": true, "properties": []models.PropertySale{}})
		return
	}

	// Fetch property sales with their details
	var propertySales []models.PropertySale
	if err := storage.DB.Where("id IN ?", savedPropertySaleIDs).
		Preload("ZoneRef").
		Preload("Organization").
		Find(&propertySales).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true, "properties": propertySales})
}

// AddPropertySaleToWishlist - Add property sale to user's wishlist
func AddPropertySaleToWishlist(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ AddPropertySaleToWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	var input AddPropertySaleToWishlistInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Verify property sale exists
	var propertySale models.PropertySale
	if err := storage.DB.First(&propertySale, input.PropertySaleID).Error; err != nil {
		log.Printf("❌ AddPropertySaleToWishlist: Property sale %d not found", input.PropertySaleID)
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Get user's current saved property sales
	var userModel models.User
	if err := storage.DB.First(&userModel, userID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Parse existing saved property sales
	var savedPropertySaleIDs []uint
	if userModel.SavedPropertySales != nil {
		if err := json.Unmarshal(userModel.SavedPropertySales, &savedPropertySaleIDs); err != nil {
			// Initialize empty array if unmarshal fails
			savedPropertySaleIDs = []uint{}
		}
	}

	// Check if property sale is already saved
	for _, id := range savedPropertySaleIDs {
		if id == input.PropertySaleID {
			ctx.JSON(iris.Map{"success": true, "message": "Property sale already in wishlist"})
			return
		}
	}

	// Add property sale to saved list
	savedPropertySaleIDs = append(savedPropertySaleIDs, input.PropertySaleID)

	// Marshal the updated property sale IDs to JSON
	marshaledPropertySales, err := json.Marshal(savedPropertySaleIDs)
	if err != nil {
		log.Printf("❌ AddPropertySaleToWishlist: Failed to marshal - %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Update user's saved property sales
	if err := storage.DB.Model(&userModel).Update("saved_property_sales", marshaledPropertySales).Error; err != nil {
		log.Printf("❌ AddPropertySaleToWishlist: Failed to update - %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	log.Printf("✅ AddPropertySaleToWishlist: User %d saved property sale %d", userID, input.PropertySaleID)
	ctx.JSON(iris.Map{"success": true, "message": "Property sale added to wishlist"})
}

// RemovePropertySaleFromWishlist - Remove property sale from user's wishlist
func RemovePropertySaleFromWishlist(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ RemovePropertySaleFromWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	propertySaleID, err := ctx.Params().GetUint("propertySaleID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Get user's current saved property sales
	var userModel models.User
	if err := storage.DB.First(&userModel, userID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Parse existing saved property sales
	var savedPropertySaleIDs []uint
	if userModel.SavedPropertySales != nil {
		if err := json.Unmarshal(userModel.SavedPropertySales, &savedPropertySaleIDs); err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
	}

	// Remove property sale from saved list
	var updatedPropertySaleIDs []uint
	for _, id := range savedPropertySaleIDs {
		if id != propertySaleID {
			updatedPropertySaleIDs = append(updatedPropertySaleIDs, id)
		}
	}

	// Marshal the updated property sale IDs to JSON
	marshaledPropertySales, err := json.Marshal(updatedPropertySaleIDs)
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Update user's saved property sales
	if err := storage.DB.Model(&userModel).Update("saved_property_sales", marshaledPropertySales).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	log.Printf("✅ RemovePropertySaleFromWishlist: User %d removed property sale %d", userID, propertySaleID)
	ctx.JSON(iris.Map{"success": true, "message": "Property sale removed from wishlist"})
}

// ============================================
// LANDMARK WISHLIST (landmark_video_saves)
// ============================================

// GetUserLandmarkWishlist returns landmarks the user saved (wishlist).
func GetUserLandmarkWishlist(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetUserLandmarkWishlist: Unauthorized - no userID in context")
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	var saves []models.LandmarkVideoSave
	if err := storage.DB.
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Find(&saves).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	if len(saves) == 0 {
		ctx.JSON(iris.Map{"success": true, "landmarks": []models.Landmark{}})
		return
	}

	landmarkIDs := make([]uint, 0, len(saves))
	seen := make(map[uint]struct{}, len(saves))
	for _, s := range saves {
		if _, dup := seen[s.LandmarkID]; dup {
			continue
		}
		seen[s.LandmarkID] = struct{}{}
		landmarkIDs = append(landmarkIDs, s.LandmarkID)
	}

	var landmarks []models.Landmark
	if err := storage.DB.Where("id IN ?", landmarkIDs).Find(&landmarks).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Preserve wishlist order (most recently saved first).
	byID := make(map[uint]models.Landmark, len(landmarks))
	for _, lm := range landmarks {
		byID[lm.ID] = lm
	}
	ordered := make([]models.Landmark, 0, len(landmarkIDs))
	for _, id := range landmarkIDs {
		if lm, ok := byID[id]; ok {
			ordered = append(ordered, lm)
		}
	}

	ctx.JSON(iris.Map{"success": true, "landmarks": ordered})
}