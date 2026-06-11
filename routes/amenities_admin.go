package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"strconv"

	"github.com/kataras/iris/v12"
)

// AdminListAmenities returns all amenities (admin only), optionally filtered by category
func AdminListAmenities(ctx iris.Context) {
	category := ctx.URLParam("category")

	query := storage.DB.Model(&models.Amenity{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	query = query.Order("category ASC, sort_order ASC")

	var amenities []models.Amenity
	if err := query.Find(&amenities).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch amenities", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    amenities,
		"count":   len(amenities),
	})
}

// AdminCreateAmenity creates a new amenity (admin only)
func AdminCreateAmenity(ctx iris.Context) {
	var input struct {
		NameEn        string `json:"name_en" validate:"required"`
		NameFr        string `json:"name_fr"`
		NameAr        string `json:"name_ar"`
		Icon          string `json:"icon" validate:"required"`
		Category      string `json:"category" validate:"required"`
		DescriptionEn string `json:"description_en"`
		DescriptionFr string `json:"description_fr"`
		DescriptionAr string `json:"description_ar"`
		SortOrder     int    `json:"sort_order"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON", "message": err.Error()})
		return
	}

	name := models.AmenityNames{En: input.NameEn, Fr: input.NameFr, Ar: input.NameAr}
	if name.Fr == "" {
		name.Fr = input.NameEn
	}
	if name.Ar == "" {
		name.Ar = input.NameEn
	}
	desc := models.AmenityNames{En: input.DescriptionEn, Fr: input.DescriptionFr, Ar: input.DescriptionAr}
	if desc.Fr == "" {
		desc.Fr = input.DescriptionEn
	}
	if desc.Ar == "" {
		desc.Ar = input.DescriptionEn
	}

	amenity := models.Amenity{
		Name:        name,
		Icon:        input.Icon,
		Category:    input.Category,
		Description: desc,
		IsActive:    true,
		SortOrder:   input.SortOrder,
	}
	if err := storage.DB.Create(&amenity).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create amenity", "message": err.Error()})
		return
	}

	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(iris.Map{"success": true, "data": amenity})
}

// AdminUpdateAmenity updates an amenity (admin only)
func AdminUpdateAmenity(ctx iris.Context) {
	idStr := ctx.Params().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid amenity ID"})
		return
	}

	var amenity models.Amenity
	if err := storage.DB.First(&amenity, id).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Amenity not found"})
		return
	}

	var input struct {
		NameEn        *string `json:"name_en"`
		NameFr        *string `json:"name_fr"`
		NameAr        *string `json:"name_ar"`
		Icon          *string `json:"icon"`
		Category      *string `json:"category"`
		DescriptionEn *string `json:"description_en"`
		DescriptionFr *string `json:"description_fr"`
		DescriptionAr *string `json:"description_ar"`
		IsActive      *bool   `json:"is_active"`
		SortOrder     *int    `json:"sort_order"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON", "message": err.Error()})
		return
	}

	if input.NameEn != nil {
		amenity.Name.En = *input.NameEn
	}
	if input.NameFr != nil {
		amenity.Name.Fr = *input.NameFr
	}
	if input.NameAr != nil {
		amenity.Name.Ar = *input.NameAr
	}
	if input.Icon != nil {
		amenity.Icon = *input.Icon
	}
	if input.Category != nil {
		amenity.Category = *input.Category
	}
	if input.DescriptionEn != nil {
		amenity.Description.En = *input.DescriptionEn
	}
	if input.DescriptionFr != nil {
		amenity.Description.Fr = *input.DescriptionFr
	}
	if input.DescriptionAr != nil {
		amenity.Description.Ar = *input.DescriptionAr
	}
	if input.IsActive != nil {
		amenity.IsActive = *input.IsActive
	}
	if input.SortOrder != nil {
		amenity.SortOrder = *input.SortOrder
	}

	if err := storage.DB.Save(&amenity).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update amenity", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{"success": true, "data": amenity})
}

// AdminDeleteAmenity deactivates or deletes an amenity (admin only)
func AdminDeleteAmenity(ctx iris.Context) {
	idStr := ctx.Params().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid amenity ID"})
		return
	}

	hard := ctx.URLParam("hard") == "true"

	var amenity models.Amenity
	if err := storage.DB.First(&amenity, id).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Amenity not found"})
		return
	}

	if hard {
		if err := storage.DB.Delete(&amenity).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to delete amenity", "message": err.Error()})
			return
		}
		ctx.JSON(iris.Map{"success": true, "message": "Amenity deleted permanently"})
	} else {
		amenity.IsActive = false
		if err := storage.DB.Save(&amenity).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to deactivate amenity", "message": err.Error()})
			return
		}
		ctx.JSON(iris.Map{"success": true, "message": "Amenity deactivated"})
	}
}

// AdminSeedAmenities seeds default amenities if none exist (admin only)
func AdminSeedAmenities(ctx iris.Context) {
	var count int64
	if err := storage.DB.Model(&models.Amenity{}).Count(&count).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to check amenities", "message": err.Error()})
		return
	}

	if count > 0 {
		ctx.JSON(iris.Map{
			"success": true,
			"message": "Amenities already exist",
			"count":   count,
		})
		return
	}

	// Seed a minimal set - full seed is in migration 003
	seeds := []struct {
		NameEn, NameFr, NameAr string
		Icon, Category         string
		DescEn, DescFr, DescAr string
		SortOrder              int
	}{
		{"WiFi", "WiFi", "واي فاي", "WifiHigh", "essential", "High-speed internet", "Connexion internet haut débit", "اتصال إنترنت عالي السرعة", 1},
		{"Air Conditioning", "Climatisation", "تكييف هواء", "Snowflake", "essential", "Air conditioning", "Climatisation", "تكييف هواء", 2},
		{"Kitchen", "Cuisine", "مطبخ", "CookingPot", "kitchen", "Fully equipped kitchen", "Cuisine équipée", "مطبخ مجهز", 3},
		{"Free Parking", "Parking gratuit", "موقف سيارات مجاني", "Car", "essential", "Free parking", "Parking gratuit", "موقف سيارات مجاني", 4},
		{"TV", "Télévision", "تلفزيون", "Television", "entertainment", "Television", "Télévision", "تلفزيون", 5},
	}

	for i, s := range seeds {
		a := models.Amenity{
			Name:        models.AmenityNames{En: s.NameEn, Fr: s.NameFr, Ar: s.NameAr},
			Icon:        s.Icon,
			Category:    s.Category,
			Description: models.AmenityNames{En: s.DescEn, Fr: s.DescFr, Ar: s.DescAr},
			IsActive:    true,
			SortOrder:   i + 1,
		}
		if err := storage.DB.Create(&a).Error; err != nil {
			log.Printf("[AdminSeedAmenities] Failed: %v", err)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to seed amenity " + s.NameEn, "message": err.Error()})
			return
		}
	}

	log.Printf("[AdminSeedAmenities] Seeded %d amenities", len(seeds))
	ctx.JSON(iris.Map{"success": true, "message": "Amenities seeded successfully", "count": len(seeds)})
}
