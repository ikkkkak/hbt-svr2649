package routes

import (
	"net/http"
	"strconv"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// AdminGetLocationBulkExample returns the canonical JSON schema example for admins.
// GET /api/admin/locations/bulk/example
func AdminGetLocationBulkExample(ctx iris.Context) {
	ctx.JSON(iris.Map{
		"data": locationBulkExampleDocument(),
		"schema": iris.Map{
			"version":        "Required. Must be 1.",
			"skip_existing":  "Optional. If true, skip rows that already exist (match by name + name_ar within parent).",
			"cities":         "Optional. Array of cities with nested zones and quartiers.",
			"city_id":        "Optional. Add zones to an existing city by database id.",
			"city_name":      "Optional. Add zones to an existing city by English name (used with zones only).",
			"zone_id":        "Optional. Add quartiers to an existing zone by database id.",
			"zones":          "Optional. Zones array (use with city_id or city_name, or inside a city object).",
			"quartiers":      "Optional. Quartiers array (use with zone_id or inside a zone object).",
			"quartier_fields": iris.Map{
				"name":            "Required string (English).",
				"name_ar":         "Required string (Arabic).",
				"key":             "Optional string. Unique per zone; used with parent_key.",
				"parent_key":      "Optional string. References another quartier's key in the same zone.",
				"parent_index":    "Optional number. 1-based index in the same quartiers array (legacy).",
				"sub_quartiers":   "Optional array. Nested child quartiers (alternative to parent_key).",
			},
		},
	})
}

func locationBulkExampleDocument() iris.Map {
	return iris.Map{
		"version":       1,
		"skip_existing": true,
		"cities": []iris.Map{
			{
				"name":       "Nouakchott",
				"name_ar":    "نواكشوط",
				"country":    "Mauritania",
				"country_ar": "موريتانيا",
				"zones": []iris.Map{
					{
						"name":            "Tevragh-Zeina",
						"name_ar":         "تفرغ زينة",
						"description":     "Central district",
						"description_ar":  "الحي المركزي",
						"quartiers": []iris.Map{
							{
								"key":         "center",
								"name":        "Center",
								"name_ar":     "الوسط",
								"parent_key":  nil,
								"sub_quartiers": []iris.Map{
									{
										"name":    "Main Market",
										"name_ar": "السوق الرئيسي",
									},
								},
							},
							{
								"key":        "coast",
								"name":       "Coastal strip",
								"name_ar":    "الساحل",
								"parent_key": nil,
							},
						},
					},
				},
			},
		},
	}
}

type locationBulkPayload struct {
	Version      int                  `json:"version"`
	SkipExisting bool                 `json:"skip_existing"`
	Cities       []locationBulkCity   `json:"cities"`
	CityID       uint                 `json:"city_id"`
	CityName     string               `json:"city_name"`
	ZoneID       uint                 `json:"zone_id"`
	Zones        []locationBulkZone   `json:"zones"`
	Quartiers    []locationBulkQuartier `json:"quartiers"`
}

type locationBulkCity struct {
	Name      string               `json:"name"`
	NameAr    string               `json:"name_ar"`
	Country   string               `json:"country"`
	CountryAr string               `json:"country_ar"`
	Zones     []locationBulkZone   `json:"zones"`
}

type locationBulkZone struct {
	Name          string                 `json:"name"`
	NameAr        string                 `json:"name_ar"`
	Description   string                 `json:"description"`
	DescriptionAr string                 `json:"description_ar"`
	Quartiers     []locationBulkQuartier `json:"quartiers"`
}

type locationBulkQuartier struct {
	Key           string                 `json:"key"`
	Name          string                 `json:"name"`
	NameAr        string                 `json:"name_ar"`
	ParentKey     *string                `json:"parent_key"`
	ParentIndex   *int                   `json:"parent_index"`
	SubQuartiers  []locationBulkQuartier `json:"sub_quartiers"`
}

type locationBulkResult struct {
	CitiesCreated    int      `json:"cities_created"`
	CitiesSkipped    int      `json:"cities_skipped"`
	ZonesCreated     int      `json:"zones_created"`
	ZonesSkipped     int      `json:"zones_skipped"`
	QuartiersCreated int      `json:"quartiers_created"`
	QuartiersSkipped int      `json:"quartiers_skipped"`
	Errors           []string `json:"errors"`
}

// AdminBulkImportLocations imports cities, zones, and quartiers from JSON.
// POST /api/admin/locations/bulk
func AdminBulkImportLocations(ctx iris.Context) {
	var payload locationBulkPayload
	if err := ctx.ReadJSON(&payload); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_json", "message": err.Error()})
		return
	}
	if payload.Version != 1 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{
			"error":   "invalid_version",
			"message": "version must be 1",
		})
		return
	}

	result := locationBulkResult{Errors: []string{}}

	// Mode: quartiers only for existing zone
	if payload.ZoneID > 0 && len(payload.Quartiers) > 0 {
		if err := importQuartiersForZone(storage.DB, payload.ZoneID, payload.Quartiers, payload.SkipExisting, &result); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "import_failed", "message": err.Error()})
			return
		}
		ctx.JSON(iris.Map{"success": true, "data": result})
		return
	}

	// Mode: zones (+ quartiers) for existing city
	if (payload.CityID > 0 || strings.TrimSpace(payload.CityName) != "") && len(payload.Zones) > 0 {
		cityID, err := resolveCityID(payload.CityID, payload.CityName)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "city_not_found", "message": err.Error()})
			return
		}
		for _, z := range payload.Zones {
			importZoneTree(storage.DB, cityID, z, payload.SkipExisting, &result)
		}
		ctx.JSON(iris.Map{"success": true, "data": result})
		return
	}

	// Mode: full nested cities
	if len(payload.Cities) == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{
			"error":   "empty_payload",
			"message": "Provide cities[], or city_id/city_name with zones[], or zone_id with quartiers[]",
		})
		return
	}

	for _, c := range payload.Cities {
		importCityTree(storage.DB, c, payload.SkipExisting, &result)
	}

	ctx.JSON(iris.Map{"success": true, "data": result})
}

func trimLoc(s string) string {
	return strings.TrimSpace(s)
}

func resolveCityID(cityID uint, cityName string) (uint, error) {
	if cityID > 0 {
		var city models.City
		if err := storage.DB.First(&city, cityID).Error; err != nil {
			return 0, err
		}
		return city.ID, nil
	}
	name := trimLoc(cityName)
	var city models.City
	if err := storage.DB.Where("LOWER(name) = ?", strings.ToLower(name)).First(&city).Error; err != nil {
		return 0, err
	}
	return city.ID, nil
}

func findOrCreateCity(db *gorm.DB, name, nameAr, country, countryAr string, skipExisting bool, result *locationBulkResult) (uint, error) {
	name = trimLoc(name)
	nameAr = trimLoc(nameAr)
	if name == "" || nameAr == "" {
		result.Errors = append(result.Errors, "city missing name or name_ar")
		return 0, gorm.ErrInvalidData
	}
	if country == "" {
		country = "Mauritania"
	}
	if countryAr == "" {
		countryAr = "موريتانيا"
	}

	var existing models.City
	err := db.Where("LOWER(name) = ? AND LOWER(name_ar) = ?", strings.ToLower(name), strings.ToLower(nameAr)).First(&existing).Error
	if err == nil {
		if skipExisting {
			result.CitiesSkipped++
			return existing.ID, nil
		}
		return existing.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}

	city := models.City{
		Name:      name,
		NameAr:    nameAr,
		Country:   country,
		CountryAr: countryAr,
		IsActive:  true,
	}
	if err := db.Create(&city).Error; err != nil {
		result.Errors = append(result.Errors, "create city "+name+": "+err.Error())
		return 0, err
	}
	result.CitiesCreated++
	return city.ID, nil
}

func findOrCreateZone(db *gorm.DB, cityID uint, z locationBulkZone, skipExisting bool, result *locationBulkResult) (uint, error) {
	name := trimLoc(z.Name)
	nameAr := trimLoc(z.NameAr)
	if name == "" || nameAr == "" {
		result.Errors = append(result.Errors, "zone missing name or name_ar")
		return 0, gorm.ErrInvalidData
	}

	var existing models.Zone
	err := db.Where("city_id = ? AND LOWER(name) = ? AND LOWER(name_ar) = ?", cityID, strings.ToLower(name), strings.ToLower(nameAr)).First(&existing).Error
	if err == nil {
		if skipExisting {
			result.ZonesSkipped++
			return existing.ID, nil
		}
		return existing.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}

	zone := models.Zone{
		CityID:        cityID,
		Name:          name,
		NameAr:        nameAr,
		Description:   trimLoc(z.Description),
		DescriptionAr: trimLoc(z.DescriptionAr),
		IsActive:      true,
	}
	if err := db.Create(&zone).Error; err != nil {
		result.Errors = append(result.Errors, "create zone "+name+": "+err.Error())
		return 0, err
	}
	result.ZonesCreated++
	return zone.ID, nil
}

func importCityTree(db *gorm.DB, c locationBulkCity, skipExisting bool, result *locationBulkResult) {
	cityID, err := findOrCreateCity(db, c.Name, c.NameAr, c.Country, c.CountryAr, skipExisting, result)
	if err != nil || cityID == 0 {
		return
	}
	for _, z := range c.Zones {
		importZoneTree(db, cityID, z, skipExisting, result)
	}
}

func importZoneTree(db *gorm.DB, cityID uint, z locationBulkZone, skipExisting bool, result *locationBulkResult) {
	zoneID, err := findOrCreateZone(db, cityID, z, skipExisting, result)
	if err != nil || zoneID == 0 {
		return
	}
	if len(z.Quartiers) > 0 {
		_ = importQuartiersForZone(db, zoneID, z.Quartiers, skipExisting, result)
	}
}

func importQuartiersForZone(db *gorm.DB, zoneID uint, quartiers []locationBulkQuartier, skipExisting bool, result *locationBulkResult) error {
	var zone models.Zone
	if err := db.First(&zone, zoneID).Error; err != nil {
		return err
	}

	// Flat quartiers with parent_index only (legacy)
	if allUseParentIndex(quartiers) {
		return importQuartiersFlatIndex(db, zoneID, quartiers, skipExisting, result)
	}

	keyToID := map[string]uint{}

	// Pass 1: top-level and nested trees without parent_key deferral
	var deferred []locationBulkQuartier
	for _, q := range quartiers {
		if hasParentKeyRef(q) && trimLoc(q.Name) != "" {
			deferred = append(deferred, q)
			continue
		}
		createQuartierTree(db, zoneID, nil, q, skipExisting, result, keyToID)
	}

	// Pass 2: parent_key references
	for _, q := range deferred {
		createQuartierWithParentKey(db, zoneID, q, skipExisting, result, keyToID)
	}

	return nil
}

func allUseParentIndex(quartiers []locationBulkQuartier) bool {
	if len(quartiers) == 0 {
		return false
	}
	for _, q := range quartiers {
		if len(q.SubQuartiers) > 0 {
			return false
		}
		if q.ParentKey != nil && trimLoc(*q.ParentKey) != "" {
			return false
		}
	}
	hasIndex := false
	for _, q := range quartiers {
		if q.ParentIndex != nil && *q.ParentIndex > 0 {
			hasIndex = true
		}
	}
	return hasIndex
}

func hasParentKeyRef(q locationBulkQuartier) bool {
	if q.ParentKey == nil {
		return false
	}
	return trimLoc(*q.ParentKey) != ""
}

func importQuartiersFlatIndex(db *gorm.DB, zoneID uint, quartiers []locationBulkQuartier, skipExisting bool, result *locationBulkResult) error {
	created := make([]uint, 0, len(quartiers))
	for i, q := range quartiers {
		name := trimLoc(q.Name)
		nameAr := trimLoc(q.NameAr)
		if name == "" || nameAr == "" {
			result.Errors = append(result.Errors, "quartier at index "+itoa(i)+" missing name or name_ar")
			created = append(created, 0)
			continue
		}

		var parentID *uint
		if q.ParentIndex != nil && *q.ParentIndex > 0 {
			idx := *q.ParentIndex - 1
			if idx >= 0 && idx < len(created) && created[idx] > 0 {
				pid := created[idx]
				parentID = &pid
			} else {
				result.Errors = append(result.Errors, "quartier "+name+": invalid parent_index "+itoa(*q.ParentIndex))
			}
		}

		id, skipped := findOrCreateQuartier(db, zoneID, parentID, name, nameAr, skipExisting, result)
		created = append(created, id)
		if skipped {
			continue
		}
	}
	return nil
}

func createQuartierWithParentKey(db *gorm.DB, zoneID uint, q locationBulkQuartier, skipExisting bool, result *locationBulkResult, keyToID map[string]uint) {
	name := trimLoc(q.Name)
	nameAr := trimLoc(q.NameAr)
	if name == "" || nameAr == "" {
		result.Errors = append(result.Errors, "quartier missing name or name_ar (parent_key)")
		return
	}
	var parentID *uint
	if q.ParentKey != nil {
		pk := trimLoc(*q.ParentKey)
		if pk != "" {
			if id, ok := keyToID[pk]; ok && id > 0 {
				parentID = &id
			} else {
				result.Errors = append(result.Errors, "quartier "+name+": unknown parent_key "+pk)
				return
			}
		}
	}
	id, _ := findOrCreateQuartier(db, zoneID, parentID, name, nameAr, skipExisting, result)
	if q.Key != "" {
		keyToID[trimLoc(q.Key)] = id
	}
	for _, sub := range q.SubQuartiers {
		createQuartierTree(db, zoneID, &id, sub, skipExisting, result, keyToID)
	}
}

func createQuartierTree(db *gorm.DB, zoneID uint, parentID *uint, q locationBulkQuartier, skipExisting bool, result *locationBulkResult, keyToID map[string]uint) {
	name := trimLoc(q.Name)
	nameAr := trimLoc(q.NameAr)
	if name == "" || nameAr == "" {
		return
	}
	id, _ := findOrCreateQuartier(db, zoneID, parentID, name, nameAr, skipExisting, result)
	if q.Key != "" {
		keyToID[trimLoc(q.Key)] = id
	}
	for _, sub := range q.SubQuartiers {
		pid := id
		createQuartierTree(db, zoneID, &pid, sub, skipExisting, result, keyToID)
	}
}

func findOrCreateQuartier(db *gorm.DB, zoneID uint, parentID *uint, name, nameAr string, skipExisting bool, result *locationBulkResult) (uint, bool) {
	q := db.Where("zone_id = ? AND LOWER(name) = ? AND LOWER(name_ar) = ?", zoneID, strings.ToLower(name), strings.ToLower(nameAr))
	if parentID == nil {
		q = q.Where("parent_quartier_id IS NULL")
	} else {
		q = q.Where("parent_quartier_id = ?", *parentID)
	}
	var existing models.Quartier
	if err := q.First(&existing).Error; err == nil {
		if skipExisting {
			result.QuartiersSkipped++
			return existing.ID, true
		}
		return existing.ID, false
	}

	quartier := models.Quartier{
		ZoneID:           zoneID,
		ParentQuartierID: parentID,
		Name:             name,
		NameAr:           nameAr,
		IsActive:         true,
	}
	if err := db.Create(&quartier).Error; err != nil {
		result.Errors = append(result.Errors, "create quartier "+name+": "+err.Error())
		return 0, false
	}
	result.QuartiersCreated++
	return quartier.ID, false
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
