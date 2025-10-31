package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strconv"

	"github.com/kataras/iris/v12"
)

// GetCities returns all active cities
func GetCities(ctx iris.Context) {
	var cities []models.City
	if err := storage.DB.Where("is_active = ?", true).Preload("Zones", "is_active = ?", true).Find(&cities).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch cities"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    cities,
	})
}

// GetZonesByCity returns all zones for a specific city
func GetZonesByCity(ctx iris.Context) {
	cityIDStr := ctx.Params().Get("cityId")
	cityID, err := strconv.ParseUint(cityIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid city ID"})
		return
	}

	var zones []models.Zone
	if err := storage.DB.Where("city_id = ? AND is_active = ?", uint(cityID), true).Find(&zones).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch zones"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    zones,
	})
}

// Admin: Create City
func AdminCreateCity(ctx iris.Context) {
	var input struct {
		Name      string `json:"name"`
		NameAr    string `json:"name_ar"`
		Country   string `json:"country"`
		CountryAr string `json:"country_ar"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	if input.Name == "" || input.NameAr == "" {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Name and NameAr are required"})
		return
	}

	city := models.City{
		Name:      input.Name,
		NameAr:    input.NameAr,
		Country:   input.Country,
		CountryAr: input.CountryAr,
		IsActive:  true,
	}

	if err := storage.DB.Create(&city).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to create city"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    city,
	})
}

// Admin: Update City
func AdminUpdateCity(ctx iris.Context) {
	cityIDStr := ctx.Params().Get("id")
	cityID, err := strconv.ParseUint(cityIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid city ID"})
		return
	}

	var input struct {
		Name      string `json:"name"`
		NameAr    string `json:"name_ar"`
		Country   string `json:"country"`
		CountryAr string `json:"country_ar"`
		IsActive  *bool  `json:"is_active"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var city models.City
	if err := storage.DB.First(&city, uint(cityID)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "City not found"})
		return
	}

	// Update fields
	if input.Name != "" {
		city.Name = input.Name
	}
	if input.NameAr != "" {
		city.NameAr = input.NameAr
	}
	if input.Country != "" {
		city.Country = input.Country
	}
	if input.CountryAr != "" {
		city.CountryAr = input.CountryAr
	}
	if input.IsActive != nil {
		city.IsActive = *input.IsActive
	}

	if err := storage.DB.Save(&city).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to update city"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    city,
	})
}

// Admin: Delete City
func AdminDeleteCity(ctx iris.Context) {
	cityIDStr := ctx.Params().Get("id")
	cityID, err := strconv.ParseUint(cityIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid city ID"})
		return
	}

	if err := storage.DB.Delete(&models.City{}, uint(cityID)).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to delete city"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "City deleted successfully",
	})
}

// Admin: Create Zone
func AdminCreateZone(ctx iris.Context) {
	var input struct {
		CityID        uint   `json:"city_id"`
		Name          string `json:"name"`
		NameAr        string `json:"name_ar"`
		Description   string `json:"description"`
		DescriptionAr string `json:"description_ar"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	if input.CityID == 0 || input.Name == "" || input.NameAr == "" {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "CityID, Name and NameAr are required"})
		return
	}

	// Verify city exists
	var city models.City
	if err := storage.DB.First(&city, input.CityID).Error; err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "City not found"})
		return
	}

	zone := models.Zone{
		CityID:        input.CityID,
		Name:          input.Name,
		NameAr:        input.NameAr,
		Description:   input.Description,
		DescriptionAr: input.DescriptionAr,
		IsActive:      true,
	}

	if err := storage.DB.Create(&zone).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to create zone"})
		return
	}

	// Load city relationship
	storage.DB.Preload("City").First(&zone, zone.ID)

	ctx.JSON(iris.Map{
		"success": true,
		"data":    zone,
	})
}

// Admin: Update Zone
func AdminUpdateZone(ctx iris.Context) {
	zoneIDStr := ctx.Params().Get("id")
	zoneID, err := strconv.ParseUint(zoneIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid zone ID"})
		return
	}

	var input struct {
		CityID        *uint  `json:"city_id"`
		Name          string `json:"name"`
		NameAr        string `json:"name_ar"`
		Description   string `json:"description"`
		DescriptionAr string `json:"description_ar"`
		IsActive      *bool  `json:"is_active"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var zone models.Zone
	if err := storage.DB.First(&zone, uint(zoneID)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "Zone not found"})
		return
	}

	// Update fields
	if input.CityID != nil {
		// Verify new city exists
		var city models.City
		if err := storage.DB.First(&city, *input.CityID).Error; err != nil {
			ctx.StatusCode(400)
			ctx.JSON(iris.Map{"error": "City not found"})
			return
		}
		zone.CityID = *input.CityID
	}
	if input.Name != "" {
		zone.Name = input.Name
	}
	if input.NameAr != "" {
		zone.NameAr = input.NameAr
	}
	if input.Description != "" {
		zone.Description = input.Description
	}
	if input.DescriptionAr != "" {
		zone.DescriptionAr = input.DescriptionAr
	}
	if input.IsActive != nil {
		zone.IsActive = *input.IsActive
	}

	if err := storage.DB.Save(&zone).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to update zone"})
		return
	}

	// Load city relationship
	storage.DB.Preload("City").First(&zone, zone.ID)

	ctx.JSON(iris.Map{
		"success": true,
		"data":    zone,
	})
}

// Admin: Delete Zone
func AdminDeleteZone(ctx iris.Context) {
	zoneIDStr := ctx.Params().Get("id")
	zoneID, err := strconv.ParseUint(zoneIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid zone ID"})
		return
	}

	if err := storage.DB.Delete(&models.Zone{}, uint(zoneID)).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to delete zone"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Zone deleted successfully",
	})
}

// Admin: Get All Cities (including inactive)
func AdminGetAllCities(ctx iris.Context) {
	var cities []models.City
	if err := storage.DB.Preload("Zones").Find(&cities).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch cities"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    cities,
	})
}

// Admin: Get All Zones (including inactive)
func AdminGetAllZones(ctx iris.Context) {
	var zones []models.Zone
	if err := storage.DB.Preload("City").Find(&zones).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch zones"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    zones,
	})
}
