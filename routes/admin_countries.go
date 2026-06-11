package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"errors"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

func normalizeCountryCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// AdminGetAllCountries returns all countries (including inactive) for the admin dashboard.
func AdminGetAllCountries(ctx iris.Context) {
	var countries []models.Country
	if err := storage.DB.Order("sort_order ASC, name ASC").Find(&countries).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch countries"})
		return
	}
	if countries == nil {
		countries = []models.Country{}
	}
	ctx.JSON(iris.Map{"success": true, "data": countries})
}

// AdminCreateCountry adds a country to the location hierarchy.
func AdminCreateCountry(ctx iris.Context) {
	var input struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		NameAr    string `json:"name_ar"`
		NameFr    string `json:"name_fr"`
		IsActive  *bool  `json:"is_active"`
		SortOrder *int   `json:"sort_order"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	code := normalizeCountryCode(input.Code)
	name := strings.TrimSpace(input.Name)
	nameAr := strings.TrimSpace(input.NameAr)
	if code == "" || len(code) > 8 {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Code is required (max 8 characters, e.g. MR, SN)"})
		return
	}
	if name == "" || nameAr == "" {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Name and name_ar are required"})
		return
	}

	var existing models.Country
	if err := storage.DB.Where("code = ?", code).First(&existing).Error; err == nil {
		ctx.StatusCode(409)
		ctx.JSON(iris.Map{"error": "A country with this code already exists"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to validate country code"})
		return
	}

	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	}

	country := models.Country{
		Code:      code,
		Name:      name,
		NameAr:    nameAr,
		NameFr:    strings.TrimSpace(input.NameFr),
		IsActive:  active,
		SortOrder: sortOrder,
	}
	if err := storage.DB.Create(&country).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to create country"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "data": country})
}

// AdminUpdateCountry updates country fields.
func AdminUpdateCountry(ctx iris.Context) {
	id, err := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	if err != nil || id == 0 {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid country ID"})
		return
	}

	var input struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		NameAr    string `json:"name_ar"`
		NameFr    string `json:"name_fr"`
		IsActive  *bool  `json:"is_active"`
		SortOrder *int   `json:"sort_order"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var country models.Country
	if err := storage.DB.First(&country, uint(id)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "Country not found"})
		return
	}

	if c := normalizeCountryCode(input.Code); c != "" && c != country.Code {
		var dup models.Country
		if err := storage.DB.Where("code = ? AND id <> ?", c, country.ID).First(&dup).Error; err == nil {
			ctx.StatusCode(409)
			ctx.JSON(iris.Map{"error": "A country with this code already exists"})
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.StatusCode(500)
			ctx.JSON(iris.Map{"error": "Failed to validate country code"})
			return
		}
		country.Code = c
	}
	if s := strings.TrimSpace(input.Name); s != "" {
		country.Name = s
	}
	if s := strings.TrimSpace(input.NameAr); s != "" {
		country.NameAr = s
	}
	if input.NameFr != "" {
		country.NameFr = strings.TrimSpace(input.NameFr)
	}
	if input.IsActive != nil {
		country.IsActive = *input.IsActive
	}
	if input.SortOrder != nil {
		country.SortOrder = *input.SortOrder
	}

	if err := storage.DB.Save(&country).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to update country"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "data": country})
}

// AdminDeleteCountry soft-deletes a country when it has no cities.
func AdminDeleteCountry(ctx iris.Context) {
	id, err := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	if err != nil || id == 0 {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid country ID"})
		return
	}

	var count int64
	if err := storage.DB.Model(&models.City{}).Where("country_id = ?", uint(id)).Count(&count).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to check cities"})
		return
	}
	if count > 0 {
		ctx.StatusCode(409)
		ctx.JSON(iris.Map{
			"error": "Cannot delete country: remove or reassign its cities first",
		})
		return
	}

	if err := storage.DB.Delete(&models.Country{}, uint(id)).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to delete country"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": "Country deleted successfully"})
}

// applyCountryToCity copies FK + legacy display strings from a Country row onto a City.
func applyCountryToCity(city *models.City, countryID uint) {
	if countryID == 0 {
		return
	}
	var c models.Country
	if err := storage.DB.First(&c, countryID).Error; err != nil {
		return
	}
	cid := countryID
	city.CountryID = &cid
	city.Country = c.Name
	city.CountryAr = c.NameAr
}
