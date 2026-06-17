package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// GetCities returns active cities (optional ?country_id= filter).
func GetCities(ctx iris.Context) {
	var cities []models.City
	db := storage.DB.Where("is_active = ?", true)
	if v := strings.TrimSpace(ctx.URLParam("country_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			db = applyCityCountryFilter(db, uint(n))
		}
	}

	if err := db.Order("name ASC").Find(&cities).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch cities"})
		return
	}
	if cities == nil {
		cities = []models.City{}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    cities,
	})
}


// GetZonesByCity returns zones for a city. For Nouakchott, zones are sourced from
// habitat_plans (cadastre districts) and synced to the listings zones table.
func GetZonesByCity(ctx iris.Context) {
	cityIDStr := ctx.Params().Get("cityId")
	cityID, err := strconv.ParseUint(cityIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid city ID"})
		return
	}

	var city models.City
	if err := storage.DB.First(&city, uint(cityID)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "City not found"})
		return
	}

	q := strings.TrimSpace(ctx.URLParam("q"))

	if isHabitatCatalogCity(&city) {
		zones, err := ensureHabitatPlansAsZones(storage.DB, city.ID, q)
		if err != nil {
			ctx.StatusCode(500)
			ctx.JSON(iris.Map{"error": "Failed to fetch habitat zones"})
			return
		}
		ctx.JSON(iris.Map{"success": true, "data": zones, "source": "habitat_plans"})
		return
	}

	db := storage.DB.Where("city_id = ? AND is_active = ?", city.ID, true)
	if q != "" {
		like := "%" + q + "%"
		db = db.Where("name ILIKE ? OR name_ar ILIKE ?", like, like)
	}
	var zones []models.Zone
	if err := db.Order("name ASC").Find(&zones).Error; err != nil {
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
		CountryID *uint  `json:"country_id"`
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
	if input.CountryID != nil && *input.CountryID > 0 {
		applyCountryToCity(&city, *input.CountryID)
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
		CountryID *uint  `json:"country_id"`
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
	if input.CountryID != nil && *input.CountryID > 0 {
		applyCountryToCity(&city, *input.CountryID)
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
	if err := storage.DB.Preload("Zones").Preload("CountryRef").Find(&cities).Error; err != nil {
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

// GetQuartiersByZone returns quartiers for a zone. When the zone is linked to a
// habitat_plan, quartiers are sourced from habitat_sectors and synced to listings.
func GetQuartiersByZone(ctx iris.Context) {
	zoneIDStr := ctx.Params().Get("zoneId")
	zoneID, err := strconv.ParseUint(zoneIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid zone ID"})
		return
	}

	var zone models.Zone
	if err := storage.DB.First(&zone, uint(zoneID)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "Zone not found"})
		return
	}

	q := strings.TrimSpace(ctx.URLParam("q"))

	planID := resolveZoneHabitatPlanID(storage.DB, &zone)
	if planID > 0 {
		pid := planID
		zone.HabitatPlanID = &pid
		quartiers, err := ensureHabitatSectorsAsQuartiers(storage.DB, &zone, q)
		if err != nil {
			ctx.StatusCode(500)
			ctx.JSON(iris.Map{"error": "Failed to fetch habitat sectors"})
			return
		}
		ctx.JSON(iris.Map{"success": true, "data": quartiers, "source": "habitat_sectors"})
		return
	}

	db := storage.DB.Where("zone_id = ? AND is_active = ? AND parent_quartier_id IS NULL", zone.ID, true)
	if q != "" {
		like := "%" + q + "%"
		db = db.Where("name ILIKE ? OR name_ar ILIKE ?", like, like)
	}
	var quartiers []models.Quartier
	if err := db.Preload("SubQuartiers", "is_active = ?", true).Order("name ASC").Find(&quartiers).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch quartiers"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    quartiers,
	})
}

// Admin: Create Quartier
func AdminCreateQuartier(ctx iris.Context) {
	var input struct {
		ZoneID          uint   `json:"zone_id"`
		ParentQuartierID *uint  `json:"parent_quartier_id"`
		Name            string `json:"name"`
		NameAr          string `json:"name_ar"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	if input.ZoneID == 0 || input.Name == "" || input.NameAr == "" {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "ZoneID, Name and NameAr are required"})
		return
	}

	// Verify zone exists
	var zone models.Zone
	if err := storage.DB.First(&zone, input.ZoneID).Error; err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Zone not found"})
		return
	}

	// If parent_quartier_id is provided, verify it exists and belongs to the same zone
	if input.ParentQuartierID != nil && *input.ParentQuartierID > 0 {
		var parentQuartier models.Quartier
		if err := storage.DB.First(&parentQuartier, *input.ParentQuartierID).Error; err != nil {
			ctx.StatusCode(400)
			ctx.JSON(iris.Map{"error": "Parent quartier not found"})
			return
		}
		if parentQuartier.ZoneID != input.ZoneID {
			ctx.StatusCode(400)
			ctx.JSON(iris.Map{"error": "Parent quartier must belong to the same zone"})
			return
		}
	}

	quartier := models.Quartier{
		ZoneID:           input.ZoneID,
		ParentQuartierID: input.ParentQuartierID,
		Name:             input.Name,
		NameAr:           input.NameAr,
		IsActive:         true,
	}

	if err := storage.DB.Create(&quartier).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to create quartier"})
		return
	}

	// Load relationships
	storage.DB.Preload("Zone").Preload("ParentQuartier").First(&quartier, quartier.ID)

	ctx.JSON(iris.Map{
		"success": true,
		"data":    quartier,
	})
}

// Admin: Update Quartier
func AdminUpdateQuartier(ctx iris.Context) {
	quartierIDStr := ctx.Params().Get("id")
	quartierID, err := strconv.ParseUint(quartierIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid quartier ID"})
		return
	}

	var input struct {
		ZoneID          *uint  `json:"zone_id"`
		ParentQuartierID *uint  `json:"parent_quartier_id"`
		Name            string `json:"name"`
		NameAr          string `json:"name_ar"`
		IsActive        *bool  `json:"is_active"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var quartier models.Quartier
	if err := storage.DB.First(&quartier, uint(quartierID)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "Quartier not found"})
		return
	}

	// Update fields
	if input.ZoneID != nil {
		// Verify new zone exists
		var zone models.Zone
		if err := storage.DB.First(&zone, *input.ZoneID).Error; err != nil {
			ctx.StatusCode(400)
			ctx.JSON(iris.Map{"error": "Zone not found"})
			return
		}
		quartier.ZoneID = *input.ZoneID
	}
	if input.ParentQuartierID != nil {
		if *input.ParentQuartierID == 0 {
			quartier.ParentQuartierID = nil
		} else {
			// Verify parent quartier exists
			var parentQuartier models.Quartier
			if err := storage.DB.First(&parentQuartier, *input.ParentQuartierID).Error; err != nil {
				ctx.StatusCode(400)
				ctx.JSON(iris.Map{"error": "Parent quartier not found"})
				return
			}
			if parentQuartier.ZoneID != quartier.ZoneID {
				ctx.StatusCode(400)
				ctx.JSON(iris.Map{"error": "Parent quartier must belong to the same zone"})
				return
			}
			quartier.ParentQuartierID = input.ParentQuartierID
		}
	}
	if input.Name != "" {
		quartier.Name = input.Name
	}
	if input.NameAr != "" {
		quartier.NameAr = input.NameAr
	}
	if input.IsActive != nil {
		quartier.IsActive = *input.IsActive
	}

	if err := storage.DB.Save(&quartier).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to update quartier"})
		return
	}

	// Load relationships
	storage.DB.Preload("Zone").Preload("ParentQuartier").First(&quartier, quartier.ID)

	ctx.JSON(iris.Map{
		"success": true,
		"data":    quartier,
	})
}

// Admin: Delete Quartier
func AdminDeleteQuartier(ctx iris.Context) {
	quartierIDStr := ctx.Params().Get("id")
	quartierID, err := strconv.ParseUint(quartierIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid quartier ID"})
		return
	}

	if err := storage.DB.Delete(&models.Quartier{}, uint(quartierID)).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to delete quartier"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Quartier deleted successfully",
	})
}

// Admin: Get All Quartiers (including inactive)
func AdminGetAllQuartiers(ctx iris.Context) {
	var quartiers []models.Quartier
	if err := storage.DB.Preload("Zone").Preload("Zone.City").Preload("ParentQuartier").Preload("SubQuartiers").Find(&quartiers).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch quartiers"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    quartiers,
	})
}

// applyCityCountryFilter matches cities by country_id and legacy rows where country_id was never set.
func applyCityCountryFilter(db *gorm.DB, countryID uint) *gorm.DB {
	if countryID == 0 {
		return db
	}
	var country models.Country
	if err := storage.DB.First(&country, countryID).Error; err != nil {
		return db.Where("country_id = ?", countryID)
	}
	name := strings.TrimSpace(country.Name)
	nameAr := strings.TrimSpace(country.NameAr)
	if name == "" && nameAr == "" {
		return db.Where("country_id = ?", countryID)
	}
	return db.Where(
		"country_id = ? OR ((country_id IS NULL OR country_id = 0) AND (LOWER(TRIM(country)) = LOWER(?) OR TRIM(country_ar) = ?))",
		countryID, name, nameAr,
	)
}

// isHabitatCatalogCity is true for Nouakchott — zones/quartiers come from cadastre tables.
func isHabitatCatalogCity(city *models.City) bool {
	if city == nil {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(city.Name))
	ar := strings.TrimSpace(city.NameAr)
	return n == "nouakchott" ||
		strings.Contains(n, "nouakchott") ||
		ar == "نواكشوط" ||
		strings.Contains(ar, "نواكشوط")
}

// ensureHabitatPlansAsZones maps habitat_plans → listings zones for a city.
func ensureHabitatPlansAsZones(db *gorm.DB, cityID uint, searchQ string) ([]models.Zone, error) {
	var plans []models.HabitatPlan
	if err := db.Where("is_active = ?", true).Order("name ASC").Find(&plans).Error; err != nil {
		return nil, err
	}
	syncResult := &habitatBulkResult{}
	for _, p := range plans {
		nameAr := strings.TrimSpace(p.NameAr)
		if nameAr == "" {
			nameAr = p.Name
		}
		syncPlanToListingZone(db, cityID, p.ID, p.Name, nameAr, syncResult)
	}

	dbq := db.Where("city_id = ? AND is_active = ? AND habitat_plan_id IS NOT NULL", cityID, true)
	if searchQ != "" {
		like := "%" + searchQ + "%"
		dbq = dbq.Where("name ILIKE ? OR name_ar ILIKE ?", like, like)
	}
	var zones []models.Zone
	if err := dbq.Order("name ASC").Find(&zones).Error; err != nil {
		return nil, err
	}
	return zones, nil
}

// ensureHabitatSectorsAsQuartiers maps habitat_sectors → listings quartiers for a zone's plan.
func ensureHabitatSectorsAsQuartiers(db *gorm.DB, zone *models.Zone, searchQ string) ([]models.Quartier, error) {
	if zone == nil || zone.HabitatPlanID == nil || *zone.HabitatPlanID == 0 {
		return nil, nil
	}
	planID := *zone.HabitatPlanID
	var sectors []models.HabitatSector
	if err := db.Where("plan_id = ?", planID).Order("name ASC").Find(&sectors).Error; err != nil {
		return nil, err
	}
	syncResult := &habitatBulkResult{}
	for _, s := range sectors {
		nameAr := strings.TrimSpace(s.NameAr)
		if nameAr == "" {
			nameAr = s.Name
		}
		syncSectorToListingQuartier(db, zone.ID, s.ID, s.Name, nameAr, syncResult)
	}

	dbq := db.Where("zone_id = ? AND is_active = ? AND parent_quartier_id IS NULL", zone.ID, true)
	if searchQ != "" {
		like := "%" + searchQ + "%"
		dbq = dbq.Where("name ILIKE ? OR name_ar ILIKE ?", like, like)
	}
	var quartiers []models.Quartier
	if err := dbq.Preload("SubQuartiers", "is_active = ?", true).Order("name ASC").Find(&quartiers).Error; err != nil {
		return nil, err
	}
	return quartiers, nil
}

// resolveZoneHabitatPlanID links a listings zone to habitat_plans when possible.
func resolveZoneHabitatPlanID(db *gorm.DB, zone *models.Zone) uint {
	if zone == nil {
		return 0
	}
	if zone.HabitatPlanID != nil && *zone.HabitatPlanID > 0 {
		return *zone.HabitatPlanID
	}
	var city models.City
	if err := db.First(&city, zone.CityID).Error; err != nil || !isHabitatCatalogCity(&city) {
		return 0
	}
	var plan models.HabitatPlan
	err := db.Where(
		"is_active = ? AND (LOWER(TRIM(name)) = LOWER(TRIM(?)) OR LOWER(TRIM(name_ar)) = LOWER(TRIM(?)) OR LOWER(TRIM(code)) = LOWER(TRIM(?)))",
		true, zone.Name, zone.NameAr, zone.Name,
	).First(&plan).Error
	if err != nil {
		return 0
	}
	persistZoneHabitatPlan(db, zone.ID, plan.ID)
	return plan.ID
}