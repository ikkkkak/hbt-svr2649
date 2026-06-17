package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
)

// GetCountries returns active countries for selectors and filters.
func GetCountries(ctx iris.Context) {
	var countries []models.Country
	q := storage.DB.Where("is_active = ?", true).Order("sort_order ASC, name ASC")
	if err := q.Find(&countries).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch countries"})
		return
	}
	if countries == nil {
		countries = []models.Country{}
	}
	ctx.JSON(iris.Map{"success": true, "data": countries})
}

// GetCitiesByCountry returns cities for a country (location hierarchy step 2).
func GetCitiesByCountry(ctx iris.Context) {
	countryIDStr := ctx.Params().Get("countryId")
	countryID, err := strconv.ParseUint(countryIDStr, 10, 32)
	if err != nil || countryID == 0 {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid country ID"})
		return
	}

	var cities []models.City
	db := applyCityCountryFilter(storage.DB.Where("is_active = ?", true), uint(countryID))
	if q := strings.TrimSpace(ctx.URLParam("q")); q != "" {
		like := "%" + q + "%"
		db = db.Where("name ILIKE ? OR name_ar ILIKE ?", like, like)
	}
	if err := db.Order("name ASC").Find(&cities).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch cities"})
		return
	}
	if cities == nil {
		cities = []models.City{}
	}
	ctx.JSON(iris.Map{"success": true, "data": cities})
}
