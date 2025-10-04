package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// GetCategories returns all categories for a specific type (property or experience)
func GetCategories(ctx iris.Context) {
	categoryType := ctx.URLParamDefault("type", "property")

	if categoryType != "property" && categoryType != "experience" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid category type. Must be 'property' or 'experience'"})
		return
	}

	var categories []models.Category
	err := storage.DB.Where("type = ? AND is_active = ?", categoryType, true).
		Order("sort_order ASC").
		Find(&categories).Error
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch categories"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    categories,
		"count":   len(categories),
	})
}

// GetAmenities returns all amenities, optionally filtered by category
func GetAmenities(ctx iris.Context) {
	category := ctx.URLParam("category")

	var amenities []models.Amenity
	var err error

	if category != "" {
		err = storage.DB.Where("category = ? AND is_active = ?", category, true).
			Order("sort_order ASC").
			Find(&amenities).Error
	} else {
		err = storage.DB.Where("is_active = ?", true).
			Order("category ASC, sort_order ASC").
			Find(&amenities).Error
	}

	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch amenities"})
		return
	}

	// Group amenities by category
	amenitiesByCategory := make(map[string][]models.Amenity)
	for _, amenity := range amenities {
		amenitiesByCategory[amenity.Category] = append(amenitiesByCategory[amenity.Category], amenity)
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    amenities,
		"grouped": amenitiesByCategory,
		"count":   len(amenities),
	})
}

// GetAmenityCategories returns all amenity categories
func GetAmenityCategories(ctx iris.Context) {
	var categories []string
	if err := storage.DB.Model(&models.Amenity{}).
		Distinct("category").
		Where("is_active = ?", true).
		Order("category ASC").
		Pluck("category", &categories).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch amenity categories"})
		return
	}

	var categoryList []iris.Map
	for _, cat := range categories {
		categoryList = append(categoryList, iris.Map{
			"id": cat,
			"name": iris.Map{
				"en": strings.Title(strings.Replace(cat, "_", " ", -1)),
				"fr": strings.Title(strings.Replace(cat, "_", " ", -1)),
				"ar": strings.Title(strings.Replace(cat, "_", " ", -1)),
			},
		})
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    categoryList,
		"count":   len(categoryList),
	})
}

// GetPropertyCategories returns categories for a specific property
func GetPropertyCategories(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("id")
	propertyID, err := strconv.Atoi(propertyIDStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	var categories []models.Category
	if err := storage.DB.Raw(`
        SELECT c.id, c.type, c.name, c.icon, c.description, c.is_active, c.sort_order, c.created_at, c.updated_at
        FROM categories c
        INNER JOIN property_categories pc ON c.id = pc.category_id
        WHERE pc.property_id = ? AND c.is_active = true
        ORDER BY c.sort_order ASC
    `, propertyID).Scan(&categories).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch property categories"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    categories,
		"count":   len(categories),
	})
}

// GetPropertyAmenities returns amenities for a specific property
func GetPropertyAmenities(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("id")
	propertyID, err := strconv.Atoi(propertyIDStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	var amenities []models.Amenity
	if err := storage.DB.Raw(`
        SELECT a.id, a.name, a.icon, a.category, a.description, a.is_active, a.sort_order, a.created_at, a.updated_at
        FROM amenities a
        INNER JOIN property_amenities pa ON a.id = pa.amenity_id
        WHERE pa.property_id = ? AND a.is_active = true
        ORDER BY a.category ASC, a.sort_order ASC
    `, propertyID).Scan(&amenities).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch property amenities"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    amenities,
		"count":   len(amenities),
	})
}

// UpdatePropertyCategories updates categories for a specific property
func UpdatePropertyCategories(ctx iris.Context) {
	claims := jwt.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	propertyIDStr := ctx.Params().Get("id")
	propertyID, err := strconv.Atoi(propertyIDStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	// Verify property ownership
	var prop models.Property
	if err := storage.DB.First(&prop, propertyID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property not found"})
		return
	}
	if uint64(prop.HostID) != uint64(userID) {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "You can only update your own properties"})
		return
	}

	var request struct {
		CategoryIDs []int `json:"category_ids"`
	}
	if err := ctx.ReadJSON(&request); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	tx := storage.DB.Begin()
	if err := tx.Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to start transaction"})
		return
	}
	defer func() { _ = tx.Rollback().Error }()

	if err := tx.Exec("DELETE FROM property_categories WHERE property_id = ?", propertyID).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to remove existing categories"})
		return
	}
	for _, categoryID := range request.CategoryIDs {
		if err := tx.Exec("INSERT INTO property_categories (property_id, category_id) VALUES (?, ?)", propertyID, categoryID).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"message": "Failed to add category"})
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to save changes"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": "Property categories updated successfully"})
}

// UpdatePropertyAmenities updates amenities for a specific property
func UpdatePropertyAmenities(ctx iris.Context) {
	claims := jwt.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	propertyIDStr := ctx.Params().Get("id")
	propertyID, err := strconv.Atoi(propertyIDStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	// Verify property ownership
	var prop models.Property
	if err := storage.DB.First(&prop, propertyID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property not found"})
		return
	}
	if uint64(prop.HostID) != uint64(userID) {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"message": "You can only update your own properties"})
		return
	}

	var request struct {
		AmenityIDs []int `json:"amenity_ids"`
	}
	if err := ctx.ReadJSON(&request); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	tx := storage.DB.Begin()
	if err := tx.Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to start transaction"})
		return
	}
	defer func() { _ = tx.Rollback().Error }()

	if err := tx.Exec("DELETE FROM property_amenities WHERE property_id = ?", propertyID).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to remove existing amenities"})
		return
	}
	for _, amenityID := range request.AmenityIDs {
		if err := tx.Exec("INSERT INTO property_amenities (property_id, amenity_id) VALUES (?, ?)", propertyID, amenityID).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"message": "Failed to add amenity"})
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to save changes"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": "Property amenities updated successfully"})
}
