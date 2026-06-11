package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"strconv"

	"github.com/kataras/iris/v12"
)

// AdminListCategories returns all categories (admin only), optionally filtered by type
func AdminListCategories(ctx iris.Context) {
	categoryType := ctx.URLParam("type") // "property", "experience", or empty for all

	query := storage.DB.Model(&models.Category{})
	if categoryType == "property" || categoryType == "experience" {
		query = query.Where("type = ?", categoryType)
	}
	query = query.Order("type ASC, sort_order ASC")

	var categories []models.Category
	if err := query.Find(&categories).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch categories", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    categories,
		"count":   len(categories),
	})
}

// AdminCreateCategory creates a new property or experience category (admin only)
func AdminCreateCategory(ctx iris.Context) {
	var input struct {
		Type        string `json:"type" validate:"required,oneof=property experience"`
		NameEn      string `json:"name_en" validate:"required"`
		NameFr      string `json:"name_fr"`
		NameAr      string `json:"name_ar"`
		Icon        string `json:"icon" validate:"required"`
		DescriptionEn string `json:"description_en"`
		DescriptionFr string `json:"description_fr"`
		DescriptionAr string `json:"description_ar"`
		SortOrder   int    `json:"sort_order"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON", "message": err.Error()})
		return
	}

	name := models.CategoryNames{En: input.NameEn, Fr: input.NameFr, Ar: input.NameAr}
	if name.Fr == "" {
		name.Fr = input.NameEn
	}
	if name.Ar == "" {
		name.Ar = input.NameEn
	}
	desc := models.CategoryNames{En: input.DescriptionEn, Fr: input.DescriptionFr, Ar: input.DescriptionAr}
	if desc.Fr == "" {
		desc.Fr = input.DescriptionEn
	}
	if desc.Ar == "" {
		desc.Ar = input.DescriptionEn
	}

	cat := models.Category{
		Type:        input.Type,
		Name:        name,
		Icon:        input.Icon,
		Description: desc,
		IsActive:    true,
		SortOrder:   input.SortOrder,
	}
	if err := storage.DB.Create(&cat).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create category", "message": err.Error()})
		return
	}

	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(iris.Map{"success": true, "data": cat})
}

// AdminUpdateCategory updates a category (admin only)
func AdminUpdateCategory(ctx iris.Context) {
	idStr := ctx.Params().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid category ID"})
		return
	}

	var cat models.Category
	if err := storage.DB.First(&cat, id).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Category not found"})
		return
	}

	var input struct {
		NameEn        *string `json:"name_en"`
		NameFr        *string `json:"name_fr"`
		NameAr        *string `json:"name_ar"`
		Icon          *string `json:"icon"`
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
		cat.Name.En = *input.NameEn
	}
	if input.NameFr != nil {
		cat.Name.Fr = *input.NameFr
	}
	if input.NameAr != nil {
		cat.Name.Ar = *input.NameAr
	}
	if input.Icon != nil {
		cat.Icon = *input.Icon
	}
	if input.DescriptionEn != nil {
		cat.Description.En = *input.DescriptionEn
	}
	if input.DescriptionFr != nil {
		cat.Description.Fr = *input.DescriptionFr
	}
	if input.DescriptionAr != nil {
		cat.Description.Ar = *input.DescriptionAr
	}
	if input.IsActive != nil {
		cat.IsActive = *input.IsActive
	}
	if input.SortOrder != nil {
		cat.SortOrder = *input.SortOrder
	}

	if err := storage.DB.Save(&cat).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update category", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{"success": true, "data": cat})
}

// AdminDeleteCategory deactivates (soft) or deletes a category (admin only)
func AdminDeleteCategory(ctx iris.Context) {
	idStr := ctx.Params().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid category ID"})
		return
	}

	hard := ctx.URLParam("hard") == "true"

	var cat models.Category
	if err := storage.DB.First(&cat, id).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Category not found"})
		return
	}

	if hard {
		if err := storage.DB.Delete(&cat).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to delete category", "message": err.Error()})
			return
		}
		ctx.JSON(iris.Map{"success": true, "message": "Category deleted permanently"})
	} else {
		cat.IsActive = false
		if err := storage.DB.Save(&cat).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to deactivate category", "message": err.Error()})
			return
		}
		ctx.JSON(iris.Map{"success": true, "message": "Category deactivated"})
	}
}

// defaultPropertyCategories matches migration 003 seed data
var defaultPropertyCategories = []struct {
	Name        string
	Icon        string
	Description string
}{
	{"Apartment", "Buildings", "Modern apartments in Nouakchott and other cities"},
	{"House", "House", "Traditional and modern houses"},
	{"Villa", "HouseLine", "Luxury villas with gardens and pools"},
	{"Riyad", "Tree", "Traditional Mauritanian courtyard houses"},
	{"Guest House", "Users", "Traditional guest houses and family homes"},
	{"Hotel", "Buildings", "Hotels and business accommodations"},
	{"Beach House", "Waves", "Beachfront properties in Nouadhibou and coastal areas"},
	{"Desert Camp", "Tent", "Traditional desert camps and nomadic accommodations"},
	{"Business Space", "Briefcase", "Office spaces and business accommodations"},
	{"Student Housing", "GraduationCap", "Student accommodations near universities"},
}

// AdminSeedPropertyCategories seeds default property types if none exist (admin only)
func AdminSeedPropertyCategories(ctx iris.Context) {
	var count int64
	if err := storage.DB.Model(&models.Category{}).Where("type = ?", "property").Count(&count).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to check categories", "message": err.Error()})
		return
	}

	if count > 0 {
		ctx.JSON(iris.Map{
			"success": true,
			"message": "Property categories already exist",
			"count":   count,
		})
		return
	}

	for i, d := range defaultPropertyCategories {
		name := models.CategoryNames{En: d.Name, Fr: d.Name, Ar: d.Name}
		desc := models.CategoryNames{En: d.Description, Fr: d.Description, Ar: d.Description}
		cat := models.Category{
			Type:        "property",
			Name:        name,
			Icon:        d.Icon,
			Description: desc,
			IsActive:    true,
			SortOrder:   i + 1,
		}
		if err := storage.DB.Create(&cat).Error; err != nil {
			log.Printf("[AdminSeedPropertyCategories] Failed to create %s: %v", d.Name, err)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to seed category " + d.Name, "message": err.Error()})
			return
		}
	}

	log.Printf("[AdminSeedPropertyCategories] Seeded %d property categories", len(defaultPropertyCategories))
	ctx.JSON(iris.Map{
		"success": true,
		"message": "Property categories seeded successfully",
		"count":   len(defaultPropertyCategories),
	})
}
