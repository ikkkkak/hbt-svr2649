package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const habitatMaxPlotsPerImport = 2000

// GET /api/admin/habitat/bulk/example
func AdminGetHabitatBulkExample(ctx iris.Context) {
	ctx.JSON(iris.Map{
		"data":    habitatBulkExampleDocument(),
		"mapping": habitatListingMappingDoc(),
	})
}

func habitatListingMappingDoc() iris.Map {
	return iris.Map{
		"habitat_to_listings": iris.Map{
			"plan":    "Maps to Zone under listings city (default Nouakchott)",
			"sector":  "Maps to Quartier under that zone",
			"plot":    "Cadastre only (not auto-created as listing); link in app later",
		},
		"nouakchott_plans": []string{
			"Tevergh Zeina", "Arafat", "Dar Naim", "El Mina", "Riyad",
			"Sebkha", "Teyarett", "Toujounine", "Ksar",
		},
	}
}

func habitatBulkExampleDocument() iris.Map {
	return iris.Map{
		"version":            1,
		"skip_existing":      true,
		"sync_to_listings":   true,
		"listings_city_name": "Nouakchott",
		"plans": []iris.Map{
			{
				"code":     "TEV",
				"name":     "Tevergh Zeina",
				"name_ar":  "ØªÙØ±Øº Ø²ÙŠÙ†Ø©",
				"color":    "#2a5298",
				"sectors": []iris.Map{
					{
						"name":    "Aghnewdert",
						"name_ar": "Ø£ØºÙ†ÙˆÙŠØ¯Ø±Øª",
						"plots": []iris.Map{
							{
								"plot_number":        "501",
								"area_m2":            250.5,
								"area_rounded":       251,
								"dimensions_string":  "12.5 x 20",
								"il_value":           "1.5",
								"el_value":           2.0,
								"res_value":          0.6,
								"centroid_lat":       18.085,
								"centroid_lng":       -15.978,
								"geom_geojson": iris.Map{
									"type": "Polygon",
									"coordinates": []interface{}{
										[]interface{}{
											[]float64{-15.978, 18.085},
											[]float64{-15.977, 18.085},
											[]float64{-15.977, 18.086},
											[]float64{-15.978, 18.086},
											[]float64{-15.978, 18.085},
										},
									},
								},
							},
						},
					},
				},
			},
			{"code": "ARF", "name": "Arafat", "name_ar": "Ø¹Ø±ÙØ§Øª"},
			{"code": "DNM", "name": "Dar Naim", "name_ar": "Ø¯Ø§Ø± Ø§Ù„Ù†Ø¹ÙŠÙ…"},
			{"code": "MNA", "name": "El Mina", "name_ar": "Ø§Ù„Ù…ÙŠÙ†Ø§Ø¡"},
			{"code": "RYD", "name": "Riyad", "name_ar": "Ø§Ù„Ø±ÙŠØ§Ø¶"},
			{"code": "SBK", "name": "Sebkha", "name_ar": "Ø§Ù„Ø³Ø¨Ø®Ø©"},
			{"code": "TYR", "name": "Teyarett", "name_ar": "ØªÙŠØ§Ø±Øª"},
			{"code": "TJN", "name": "Toujounine", "name_ar": "ØªØ¬ÙƒØ¬Ø©"},
			{"code": "KSR", "name": "Ksar", "name_ar": "Ù„ÙƒØµØ±"},
		},
	}
}

type habitatBulkPayload struct {
	Version          int                 `json:"version"`
	SkipExisting     bool                `json:"skip_existing"`
	SyncToListings   bool                `json:"sync_to_listings"`
	ListingsCityName string              `json:"listings_city_name"`
	Plans            []habitatBulkPlan   `json:"plans"`
	PlanID           uint                `json:"plan_id"`
	PlanCode         string              `json:"plan_code"`
	Sectors          []habitatBulkSector `json:"sectors"`
	SectorID         uint                `json:"sector_id"`
	Plots            []habitatBulkPlot   `json:"plots"`
}

type habitatBulkPlan struct {
	Code          string              `json:"code"`
	Name          string              `json:"name"`
	NameAr        string              `json:"name_ar"`
	Color         string              `json:"color"`
	BoundsGeoJSON json.RawMessage     `json:"bounds_geojson"`
	CentroidLat   *float64            `json:"centroid_lat"`
	CentroidLng   *float64            `json:"centroid_lng"`
	Metadata      json.RawMessage     `json:"metadata"`
	Sectors       []habitatBulkSector `json:"sectors"`
}

type habitatBulkSector struct {
	Name          string            `json:"name"`
	NameAr        string            `json:"name_ar"`
	Code          string            `json:"code"`
	BoundsGeoJSON json.RawMessage   `json:"bounds_geojson"`
	CentroidLat   *float64          `json:"centroid_lat"`
	CentroidLng   *float64          `json:"centroid_lng"`
	OriginalSize  *int              `json:"original_size"`
	OriginalLat   *float64          `json:"original_lat"`
	OriginalLng   *float64          `json:"original_lng"`
	Metadata      json.RawMessage   `json:"metadata"`
	Plots         []habitatBulkPlot `json:"plots"`
}

type habitatBulkPlot struct {
	PlotNumber       string          `json:"plot_number"`
	LValue           string          `json:"l_value"`
	IValue           *int            `json:"i_value"`
	AreaM2           *float64        `json:"area_m2"`
	AreaRounded      *int            `json:"area_rounded"`
	AreaHectares     *float64        `json:"area_hectares"`
	PerimeterM       *float64        `json:"perimeter_m"`
	SidesM           json.RawMessage `json:"sides_m"`
	DimensionsString string          `json:"dimensions_string"`
	LengthM          *float64        `json:"length_m"`
	WidthM           *float64        `json:"width_m"`
	ILValue          string          `json:"il_value"`
	ELValue          *float64        `json:"el_value"`
	RESValue         *float64        `json:"res_value"`
	GeomGeoJSON      json.RawMessage `json:"geom_geojson"`
	CentroidLat      *float64        `json:"centroid_lat"`
	CentroidLng      *float64        `json:"centroid_lng"`
	Corners          json.RawMessage `json:"corners"`
	RawProperties    json.RawMessage `json:"raw_properties"`
	DataFilePath     string          `json:"data_file_path"`
}

type habitatBulkResult struct {
	PlansCreated      int      `json:"plans_created"`
	PlansSkipped      int      `json:"plans_skipped"`
	SectorsCreated    int      `json:"sectors_created"`
	SectorsSkipped    int      `json:"sectors_skipped"`
	PlotsCreated      int      `json:"plots_created"`
	PlotsSkipped      int      `json:"plots_skipped"`
	ListingsZonesSync int      `json:"listings_zones_synced"`
	ListingsQuartSync int      `json:"listings_quartiers_synced"`
	Errors            []string `json:"errors"`
}

// POST /api/admin/habitat/bulk
func AdminHabitatBulkImport(ctx iris.Context) {
	var payload habitatBulkPayload
	if err := ctx.ReadJSON(&payload); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_json", "message": err.Error()})
		return
	}
	if payload.Version != 1 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_version", "message": "version must be 1"})
		return
	}

	result := habitatBulkResult{Errors: []string{}}
	var listingsCityID uint
	if payload.SyncToListings {
		cityName := payload.ListingsCityName
		if cityName == "" {
			cityName = "Nouakchott"
		}
		id, err := resolveCityID(0, cityName)
		if err != nil {
			id, err = findOrCreateCityOnly(cityName, "Ù†ÙˆØ§ÙƒØ´ÙˆØ·")
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				ctx.JSON(iris.Map{"error": "listings_city", "message": err.Error()})
				return
			}
		}
		listingsCityID = id
	}

	plotBudget := habitatMaxPlotsPerImport

	// Plots-only for existing sector
	if payload.SectorID > 0 && len(payload.Plots) > 0 {
		if len(payload.Plots) > plotBudget {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{
				"error":   "too_many_plots",
				"message": fmt.Sprintf("max %d plots per request", plotBudget),
			})
			return
		}
		var sec models.HabitatSector
		if err := storage.DB.Select("id", "plan_id").First(&sec, payload.SectorID).Error; err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "sector_not_found", "message": err.Error()})
			return
		}
		importHabitatPlots(storage.DB, sec.PlanID, payload.SectorID, payload.Plots, payload.SkipExisting, &result, nil)
		ctx.JSON(iris.Map{"success": true, "data": result})
		return
	}

	// Sectors (+plots) for existing plan
	if (payload.PlanID > 0 || payload.PlanCode != "") && len(payload.Sectors) > 0 {
		planID, err := resolvePlanID(payload.PlanID, payload.PlanCode)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "plan_not_found", "message": err.Error()})
			return
		}
		for _, s := range payload.Sectors {
			if plotBudget <= 0 {
				result.Errors = append(result.Errors, "plot import limit reached for this request")
				break
			}
			n := importHabitatSectorTree(storage.DB, planID, s, payload.SkipExisting, listingsCityID, payload.SyncToListings, &result, &plotBudget)
			_ = n
		}
		ctx.JSON(iris.Map{"success": true, "data": result})
		return
	}

	if len(payload.Plans) == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{
			"error":   "empty_payload",
			"message": "Provide plans[], or plan_id/plan_code with sectors[], or sector_id with plots[]",
		})
		return
	}

	for _, p := range payload.Plans {
		if plotBudget <= 0 {
			result.Errors = append(result.Errors, "plot import limit reached; send remaining plots in another request")
			break
		}
		importHabitatPlanTree(storage.DB, p, payload.SkipExisting, listingsCityID, payload.SyncToListings, &result, &plotBudget)
	}

	ctx.JSON(iris.Map{"success": true, "data": result})
}

func findOrCreateCityOnly(name, nameAr string) (uint, error) {
	var c models.City
	err := storage.DB.Where("LOWER(name) = ?", strings.ToLower(trimLoc(name))).First(&c).Error
	if err == nil {
		return c.ID, nil
	}
	c = models.City{Name: name, NameAr: nameAr, Country: "Mauritania", CountryAr: "Ù…ÙˆØ±ÙŠØªØ§Ù†ÙŠØ§", IsActive: true}
	if err := storage.DB.Create(&c).Error; err != nil {
		return 0, err
	}
	return c.ID, nil
}

func resolvePlanID(planID uint, planCode string) (uint, error) {
	if planID > 0 {
		var p models.HabitatPlan
		if err := storage.DB.First(&p, planID).Error; err != nil {
			return 0, err
		}
		return p.ID, nil
	}
	code := strings.ToUpper(trimLoc(planCode))
	var p models.HabitatPlan
	if err := storage.DB.Where("UPPER(code) = ?", code).First(&p).Error; err != nil {
		return 0, err
	}
	return p.ID, nil
}

func planCodeFromName(name string) string {
	parts := strings.Fields(strings.ToUpper(trimLoc(name)))
	if len(parts) == 0 {
		return "PLAN"
	}
	if len(parts) == 1 {
		if len(parts[0]) >= 3 {
			return parts[0][:3]
		}
		return parts[0]
	}
	return string([]byte{parts[0][0], parts[1][0], parts[len(parts)-1][0]})
}

func importHabitatPlanTree(db *gorm.DB, p habitatBulkPlan, skipExisting bool, listingsCityID uint, syncListings bool, result *habitatBulkResult, plotBudget *int) {
	code := strings.ToUpper(trimLoc(p.Code))
	if code == "" {
		code = planCodeFromName(p.Name)
	}
	name := trimLoc(p.Name)
	nameAr := trimLoc(p.NameAr)
	if name == "" || nameAr == "" {
		result.Errors = append(result.Errors, "plan missing name or name_ar")
		return
	}

	var existing models.HabitatPlan
	err := db.Where("UPPER(code) = ?", code).First(&existing).Error
	var planID uint
	if err == nil {
		planID = existing.ID
		result.PlansSkipped++
	} else {
		plan := models.HabitatPlan{
			Code:          code,
			Name:          name,
			NameAr:        nameAr,
			Color:         defaultStr(p.Color, "#2a5298"),
			BoundsGeoJSON: rawJSON(p.BoundsGeoJSON),
			CentroidLat:   p.CentroidLat,
			CentroidLng:   p.CentroidLng,
			Metadata:      rawJSON(p.Metadata),
			IsActive:      true,
		}
		if err := db.Create(&plan).Error; err != nil {
			result.Errors = append(result.Errors, "plan "+name+": "+err.Error())
			return
		}
		planID = plan.ID
		result.PlansCreated++
	}

	var zoneID uint
	if syncListings && listingsCityID > 0 {
		zid, synced := syncPlanToListingZone(db, listingsCityID, planID, name, nameAr, result)
		zoneID = zid
		_ = synced
	}

	for _, s := range p.Sectors {
		if *plotBudget <= 0 {
			break
		}
		importHabitatSectorTree(db, planID, s, skipExisting, zoneID, syncListings, result, plotBudget)
	}
	refreshPlanStats(db, planID)
}

func importHabitatSectorTree(db *gorm.DB, planID uint, s habitatBulkSector, skipExisting bool, listingsZoneID uint, syncListings bool, result *habitatBulkResult, plotBudget *int) uint {
	name := trimLoc(s.Name)
	nameAr := trimLoc(s.NameAr)
	if nameAr == "" {
		nameAr = name
	}
	if name == "" {
		result.Errors = append(result.Errors, "sector missing name")
		return 0
	}

	var existing models.HabitatSector
	err := db.Where("plan_id = ? AND LOWER(name) = ?", planID, strings.ToLower(name)).First(&existing).Error
	var sectorID uint
	if err == nil {
		sectorID = existing.ID
		result.SectorsSkipped++
	} else {
		sec := models.HabitatSector{
			PlanID:        planID,
			Name:          name,
			NameAr:        nameAr,
			Code:          trimLoc(s.Code),
			BoundsGeoJSON: rawJSON(s.BoundsGeoJSON),
			CentroidLat:   s.CentroidLat,
			CentroidLng:   s.CentroidLng,
			OriginalSize:  s.OriginalSize,
			OriginalLat:   s.OriginalLat,
			OriginalLng:   s.OriginalLng,
			Metadata:      rawJSON(s.Metadata),
		}
		if err := db.Create(&sec).Error; err != nil {
			result.Errors = append(result.Errors, "sector "+name+": "+err.Error())
			return 0
		}
		sectorID = sec.ID
		result.SectorsCreated++
	}

	if syncListings && listingsZoneID > 0 {
		syncSectorToListingQuartier(db, listingsZoneID, sectorID, name, nameAr, result)
	}

	if len(s.Plots) > 0 {
		importHabitatPlots(db, planID, sectorID, s.Plots, skipExisting, result, plotBudget)
	}
	refreshSectorStats(db, sectorID)
	return sectorID
}

func importHabitatPlots(db *gorm.DB, planID, sectorID uint, plots []habitatBulkPlot, skipExisting bool, result *habitatBulkResult, budget *int) {
	if planID == 0 {
		var sec models.HabitatSector
		if err := db.Select("plan_id").First(&sec, sectorID).Error; err == nil {
			planID = sec.PlanID
		}
	}
	limit := len(plots)
	if budget != nil && *budget < limit {
		limit = *budget
	}
	for i := 0; i < limit; i++ {
		p := plots[i]
		pn := trimLoc(p.PlotNumber)
		if pn == "" {
			result.Errors = append(result.Errors, "plot missing plot_number")
			continue
		}
		var existing models.HabitatPlot
		err := db.Where("sector_id = ? AND plot_number = ?", sectorID, pn).First(&existing).Error
		if err == nil {
			result.PlotsSkipped++
			continue
		}
		row := models.HabitatPlot{
			PlanID:           planID,
			SectorID:         sectorID,
			PlotNumber:       pn,
			LValue:           trimLoc(p.LValue),
			IValue:           p.IValue,
			AreaM2:           p.AreaM2,
			AreaRounded:      p.AreaRounded,
			AreaHectares:     p.AreaHectares,
			PerimeterM:       p.PerimeterM,
			SidesM:           parseSidesM(p.SidesM),
			DimensionsString: trimLoc(p.DimensionsString),
			LengthM:          p.LengthM,
			WidthM:           p.WidthM,
			ILValue:          p.ILValue,
			ELValue:          p.ELValue,
			RESValue:         p.RESValue,
			GeomGeoJSON:      rawJSON(p.GeomGeoJSON),
			CentroidLat:      p.CentroidLat,
			CentroidLng:      p.CentroidLng,
			Corners:          rawJSON(p.Corners),
			RawProperties:    rawJSON(p.RawProperties),
			DataFilePath:     trimLoc(p.DataFilePath),
			Version:          1,
		}
		if err := db.Create(&row).Error; err != nil {
			result.Errors = append(result.Errors, "plot "+pn+": "+err.Error())
			continue
		}
		result.PlotsCreated++
		if budget != nil {
			*budget--
		}
	}
	if budget != nil && len(plots) > limit {
		result.Errors = append(result.Errors, fmt.Sprintf("%d plots deferred (per-request limit); use sector_id + plots[] in another request", len(plots)-limit))
	}
	refreshSectorStats(db, sectorID)
}

func syncPlanToListingZone(db *gorm.DB, cityID, planID uint, name, nameAr string, result *habitatBulkResult) (uint, bool) {
	r := &locationBulkResult{}
	zid, err := findOrCreateZone(db, cityID, locationBulkZone{Name: name, NameAr: nameAr}, true, r)
	if err != nil {
		return 0, false
	}
	if planID > 0 && zid > 0 {
		persistZoneHabitatPlan(db, zid, planID)
	}
	if r.ZonesCreated > 0 {
		result.ListingsZonesSync++
	}
	return zid, true
}

func syncSectorToListingQuartier(db *gorm.DB, zoneID, sectorID uint, name, nameAr string, result *habitatBulkResult) {
	r := &locationBulkResult{}
	qid, skipped := findOrCreateQuartier(db, zoneID, nil, name, nameAr, true, r)
	if sectorID > 0 && qid > 0 {
		persistQuartierHabitatSector(db, qid, sectorID)
	}
	if !skipped && r.QuartiersCreated > 0 {
		result.ListingsQuartSync++
	} else if r.QuartiersCreated > 0 {
		result.ListingsQuartSync++
	}
}

func persistZoneHabitatPlan(db *gorm.DB, zoneID, planID uint) {
	if zoneID == 0 || planID == 0 {
		return
	}
	db.Model(&models.Zone{}).
		Where("id = ? AND (habitat_plan_id IS NULL OR habitat_plan_id = 0)", zoneID).
		Update("habitat_plan_id", planID)
}

func persistQuartierHabitatSector(db *gorm.DB, quartierID, sectorID uint) {
	if quartierID == 0 || sectorID == 0 {
		return
	}
	db.Model(&models.Quartier{}).
		Where("id = ? AND (habitat_sector_id IS NULL OR habitat_sector_id = 0)", quartierID).
		Update("habitat_sector_id", sectorID)
}

func refreshSectorStats(db *gorm.DB, sectorID uint) {
	var count int64
	var sumArea float64
	db.Model(&models.HabitatPlot{}).Where("sector_id = ?", sectorID).Count(&count)
	db.Model(&models.HabitatPlot{}).Where("sector_id = ?", sectorID).Select("COALESCE(SUM(area_m2),0)").Scan(&sumArea)
	db.Model(&models.HabitatSector{}).Where("id = ?", sectorID).Updates(map[string]interface{}{
		"plot_count":    count,
		"total_area_m2": sumArea,
	})
	var planID uint
	db.Model(&models.HabitatSector{}).Where("id = ?", sectorID).Select("plan_id").Scan(&planID)
	if planID > 0 {
		refreshPlanStats(db, planID)
	}
}

func refreshPlanStats(db *gorm.DB, planID uint) {
	var sectorCount int64
	var plotCount int64
	var sumArea float64
	db.Model(&models.HabitatSector{}).Where("plan_id = ?", planID).Count(&sectorCount)
	db.Raw(`
		SELECT COUNT(p.id), COALESCE(SUM(p.area_m2),0)
		FROM habitat_plots p
		JOIN habitat_sectors s ON s.id = p.sector_id
		WHERE s.plan_id = ?
	`, planID).Row().Scan(&plotCount, &sumArea)
	db.Model(&models.HabitatPlan{}).Where("id = ?", planID).Updates(map[string]interface{}{
		"sector_count":  sectorCount,
		"plot_count":    plotCount,
		"total_area_m2": sumArea,
	})
}

func parseSidesM(raw json.RawMessage) models.Float64Slice {
	if len(raw) == 0 {
		return nil
	}
	var arr []float64
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	return models.Float64Slice(arr)
}

func rawJSON(b json.RawMessage) datatypes.JSON {
	if len(b) == 0 {
		return nil
	}
	return datatypes.JSON(b)
}

func defaultStr(s, def string) string {
	if trimLoc(s) == "" {
		return def
	}
	return s
}

// GET /api/habitat/plans
func GetHabitatPlans(ctx iris.Context) {
	var plans []models.HabitatPlan
	q := storage.DB.Where("is_active = ?", true).Order("name ASC")
	if err := q.Find(&plans).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch plans"})
		return
	}
	ctx.JSON(iris.Map{"success": true, "data": plans})
}

// GET /api/habitat/plans/{planId}/sectors
func GetHabitatSectorsByPlan(ctx iris.Context) {
	planID, _ := strconv.ParseUint(ctx.Params().Get("planId"), 10, 32)
	var sectors []models.HabitatSector
	if err := storage.DB.Where("plan_id = ?", uint(planID)).Order("name ASC").Find(&sectors).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch sectors"})
		return
	}
	// Backfill centroid from plot averages when import omitted lat/lng
	type centroidRow struct {
		SectorID uint
		AvgLat   float64
		AvgLng   float64
	}
	var centroids []centroidRow
	storage.DB.Raw(`
		SELECT sector_id, AVG(centroid_lat) AS avg_lat, AVG(centroid_lng) AS avg_lng
		FROM habitat_plots
		WHERE plan_id = ? AND centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL
		GROUP BY sector_id
	`, uint(planID)).Scan(&centroids)
	centroidBySector := make(map[uint]centroidRow, len(centroids))
	for _, c := range centroids {
		centroidBySector[c.SectorID] = c
	}
	for i := range sectors {
		s := &sectors[i]
		if s.CentroidLat != nil && s.CentroidLng != nil {
			continue
		}
		if s.OriginalLat != nil && s.OriginalLng != nil {
			s.CentroidLat = s.OriginalLat
			s.CentroidLng = s.OriginalLng
			continue
		}
		if c, ok := centroidBySector[s.ID]; ok {
			lat, lng := c.AvgLat, c.AvgLng
			s.CentroidLat = &lat
			s.CentroidLng = &lng
		}
	}
	ctx.JSON(iris.Map{"success": true, "data": sectors})
}

// GET /api/habitat/sectors/{sectorId}/plots
// Query: page, limit (max 1000), or all=true to return every plot in the sector (max 20000).
// map=true omits raw_properties/sides_m but includes all fields needed for the plot callout card.
// lite=true omits geometry — use for bulk quartier index; fetch geometry separately for visible plots.
func GetHabitatPlotsBySector(ctx iris.Context) {
	sectorID, _ := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	fetchAll := ctx.URLParam("all") == "true" || ctx.URLParam("all") == "1"
	mapMode := ctx.URLParam("map") == "true" || ctx.URLParam("map") == "1"
	liteMode := ctx.URLParam("lite") == "true" || ctx.URLParam("lite") == "1"

	var total int64
	storage.DB.Model(&models.HabitatPlot{}).Where("sector_id = ?", uint(sectorID)).Count(&total)

	forSaleJoin := habitatForSaleJoinSQL
	forSaleExpr := habitatForSaleSelectExpr()
	plotSelect := "habitat_plots.*, " + forSaleExpr
	switch {
	case liteMode:
		plotSelect = habitatPlotLiteSelect(forSaleExpr)
	case mapMode:
		plotSelect = habitatPlotMapModeSelect(forSaleExpr)
	}

	if fetchAll {
		const maxAll = 20000
		var plots []models.HabitatPlot
		q := storage.DB.Model(&models.HabitatPlot{}).
			Joins(forSaleJoin).
			Where("sector_id = ?", uint(sectorID)).
			Select(plotSelect).
			Order(habitatPlotNaturalOrderSQL)
		if total > int64(maxAll) {
			q = q.Limit(maxAll)
		}
		if err := q.Find(&plots).Error; err != nil {
			ctx.StatusCode(500)
			ctx.JSON(iris.Map{"error": "Failed to fetch plots"})
			return
		}
		fillHabitatPlotsDerivedFields(storage.DB, plots)
		enrichHabitatPlotsWithPlanSector(plots, uint(sectorID))
		ctx.JSON(iris.Map{
			"success": true,
			"data":    plots,
			"meta": iris.Map{
				"total":     total,
				"truncated": int64(len(plots)) < total,
				"all":       true,
			},
		})
		return
	}

	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 100)
	if limit > 1000 {
		limit = 1000
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	var plots []models.HabitatPlot
	if err := storage.DB.Model(&models.HabitatPlot{}).
		Joins(forSaleJoin).
		Where("sector_id = ?", uint(sectorID)).
		Select(plotSelect).
			Order(habitatPlotNaturalOrderSQL).
		Offset(offset).Limit(limit).
		Find(&plots).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch plots"})
		return
	}
	fillHabitatPlotsDerivedFields(storage.DB, plots)
	enrichHabitatPlotsWithPlanSector(plots, uint(sectorID))
	ctx.JSON(iris.Map{
		"success": true,
		"data":    plots,
		"pagination": iris.Map{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (int(total) + limit - 1) / limit,
		},
	})
}


// GET /api/habitat/plots/bbox?min_lng=&min_lat=&max_lng=&max_lat=&zoom=&plan_id=&sector_id=
func GetHabitatPlotsInBBox(ctx iris.Context) {
	minLng, err1 := strconv.ParseFloat(ctx.URLParam("min_lng"), 64)
	minLat, err2 := strconv.ParseFloat(ctx.URLParam("min_lat"), 64)
	maxLng, err3 := strconv.ParseFloat(ctx.URLParam("max_lng"), 64)
	maxLat, err4 := strconv.ParseFloat(ctx.URLParam("max_lat"), 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Invalid bbox parameters"})
		return
	}
	if minLat > maxLat {
		minLat, maxLat = maxLat, minLat
	}
	if minLng > maxLng {
		minLng, maxLng = maxLng, minLng
	}

	zoom := ctx.URLParamIntDefault("zoom", 14)
	limit := 200
	switch {
	case zoom >= 17:
		limit = 150
	case zoom >= 16:
		limit = 120
	case zoom >= 15:
		limit = 80
	case zoom >= 14:
		limit = 50
	default:
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}

	type plotRow struct {
		models.HabitatPlot
		IsForSale bool `json:"is_for_sale" gorm:"column:is_for_sale"`
	}

	// Landmarks (land for sale) — we only mark plots that are verified + published and linked
	// by habitat_plot_id (host-confirmed cadastre plot).
	forSaleJoin := habitatForSaleJoinSQL
	forSaleExpr := habitatForSaleSelectExpr()

	q := storage.DB.Model(&models.HabitatPlot{}).
		Joins(forSaleJoin).
		Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").
		Where("centroid_lat BETWEEN ? AND ?", minLat, maxLat).
		Where("centroid_lng BETWEEN ? AND ?", minLng, maxLng)

	var sectorTotal int64
	if planIDStr := strings.TrimSpace(ctx.URLParam("plan_id")); planIDStr != "" {
		if planID, err := strconv.ParseUint(planIDStr, 10, 32); err == nil {
			q = q.Where("plan_id = ?", uint(planID))
		}
	}

	if sectorIDStr := strings.TrimSpace(ctx.URLParam("sector_id")); sectorIDStr != "" {
		if sectorID, err := strconv.ParseUint(sectorIDStr, 10, 32); err == nil {
			q = q.Where("sector_id = ?", uint(sectorID))
			storage.DB.Model(&models.HabitatPlot{}).
				Where("sector_id = ?", uint(sectorID)).
				Count(&sectorTotal)
		}
	}

	var total int64
	q.Count(&total)

	includeGeom := zoom >= 15
	var plots []plotRow
	query := q.Order(habitatPlotNaturalOrderSQL).Limit(limit)
	if !includeGeom {
		query = query.Select(habitatPlotCardSelect(forSaleExpr))
	} else {
		query = query.Select(habitatPlotMapModeSelect(forSaleExpr))
	}
	if err := query.Find(&plots).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch plots in viewport"})
		return
	}

	out := make([]models.HabitatPlot, len(plots))
	for i := range plots {
		out[i] = plots[i].HabitatPlot
		out[i].IsForSale = plots[i].IsForSale
	}
	fillHabitatPlotsDerivedFields(storage.DB, out)
	if sectorTotal > 0 && len(out) > 0 {
		if sectorID, err := strconv.ParseUint(strings.TrimSpace(ctx.URLParam("sector_id")), 10, 32); err == nil && sectorID > 0 {
			enrichHabitatPlotsWithPlanSector(out, uint(sectorID))
		}
	}

	meta := iris.Map{
		"count":         len(out),
		"total_in_bbox": total,
		"truncated":     int64(len(out)) < total,
		"include_geom":  includeGeom,
	}
	if sectorTotal > 0 {
		meta["sector_total"] = sectorTotal
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    out,
		"meta":    meta,
	})
}

// GET /api/habitat/plots/lookup?quartier_id=&plot_number=
// Resolves listings quartier â†’ habitat sector by name, then finds one plot (fast).
func LookupHabitatPlotForListing(ctx iris.Context) {
	quartierID, err := strconv.ParseUint(strings.TrimSpace(ctx.URLParam("quartier_id")), 10, 32)
	if err != nil || quartierID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "quartier_id is required"})
		return
	}
	plotNumber := strings.TrimSpace(ctx.URLParam("plot_number"))
	if plotNumber == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "plot_number is required"})
		return
	}

	sectorID, resolveMeta, resolveErr := resolveHabitatSectorIDForQuartier(uint(quartierID))
	if resolveErr != nil {
		ctx.JSON(iris.Map{
			"success": true,
			"data":    nil,
			"meta": iris.Map{
				"quartier_id": quartierID,
				"plot_number": plotNumber,
				"resolved":   false,
				"reason":     resolveErr.Error(),
				"resolve":    resolveMeta,
			},
		})
		return
	}

	plot, matchKind, findErr := findHabitatPlotInSector(sectorID, plotNumber, nil)
	if findErr != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to look up plot"})
		return
	}

	meta := iris.Map{
		"quartier_id":       quartierID,
		"habitat_sector_id": sectorID,
		"plot_number":       plotNumber,
		"resolved":          plot != nil,
		"match_kind":        matchKind,
		"resolve":           resolveMeta,
	}
	if plot == nil {
		meta["reason"] = "no plot with this number in the matched cadastre sector"
	}

	ctx.JSON(iris.Map{"success": true, "data": plot, "meta": meta})
}

// GET /api/habitat/plots/lookup_in_sector?sector_id=&plot_number=
// Fast plot lookup within one cadastre sector (used by the cadastre filter sheet).
func LookupHabitatPlotInSector(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(strings.TrimSpace(ctx.URLParam("sector_id")), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "sector_id is required"})
		return
	}
	plotNumber := strings.TrimSpace(ctx.URLParam("plot_number"))
	if plotNumber == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "plot_number is required"})
		return
	}

	var subSectorID *uint
	if raw := strings.TrimSpace(ctx.URLParam("sub_sector_id")); raw != "" {
		if id, err := strconv.ParseUint(raw, 10, 32); err == nil && id > 0 {
			v := uint(id)
			subSectorID = &v
		}
	}

	plot, matchKind, findErr := findHabitatPlotInSector(uint(sectorID), plotNumber, subSectorID)
	if findErr != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to look up plot"})
		return
	}

	if plot != nil {
		logHabitatPlotAPI("GET /api/habitat/plots/lookup_in_sector", plot, plot)
	}

	reason := "no plot with this number in this sector"
	if subSectorID != nil {
		reason = "no plot with this number in this sub-sector"
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    plot,
		"meta": iris.Map{
			"sector_id":     sectorID,
			"sub_sector_id": subSectorID,
			"plot_number":   plotNumber,
			"resolved":      plot != nil,
			"match_kind":    matchKind,
			"reason":        reason,
		},
	})
}

// GET /api/habitat/plots/{plotId}/for_sale_landmark
// Returns the verified+published landmark id linked to this plot (if any).
func GetForSaleLandmarkByPlot(ctx iris.Context) {
	plotID, err := strconv.ParseUint(ctx.Params().Get("plotId"), 10, 32)
	if err != nil || plotID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid plot id"})
		return
	}

	type row struct {
		ID uint `gorm:"column:id"`
	}
	var r row
	q := storage.DB.Table("landmarks").
		Select("id").
		Where("habitat_plot_id = ?", uint(plotID)).
		Where("is_verified = TRUE AND is_published = TRUE AND status = 'verified'").
		Order("updated_at DESC, id DESC").
		Limit(1)

	if err := q.Scan(&r).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch linked landmark"})
		return
	}
	if r.ID == 0 {
		ctx.JSON(iris.Map{"success": true, "data": nil})
		return
	}
	ctx.JSON(iris.Map{
		"success": true,
		"data": iris.Map{
			"landmark_id": r.ID,
		},
	})
}

// resolveHabitatSectorIDForQuartier maps a listings quartier to a habitat_sectors row.
// Listings: City â†’ Zone â†’ Quartier. Cadastre: Plan â†’ Sector â†’ Plot.
// IDs are not shared; matching is by zone/plan name and quartier/sector name.
func resolveHabitatSectorIDForQuartier(quartierID uint) (sectorID uint, meta iris.Map, err error) {
	meta = iris.Map{}

	var q models.Quartier
	if err = storage.DB.First(&q, quartierID).Error; err != nil {
		return 0, meta, fmt.Errorf("quartier not found")
	}
	meta["quartier_name"] = q.Name

	var zone models.Zone
	if err = storage.DB.First(&zone, q.ZoneID).Error; err != nil {
		return 0, meta, fmt.Errorf("zone not found for quartier")
	}
	meta["zone_name"] = zone.Name

	// Stored link from cadastre import or prior successful match.
	if q.HabitatSectorID != nil && *q.HabitatSectorID > 0 {
		var linked models.HabitatSector
		if e := storage.DB.First(&linked, *q.HabitatSectorID).Error; e == nil {
			if zone.HabitatPlanID == nil || *zone.HabitatPlanID == 0 || linked.PlanID == *zone.HabitatPlanID {
				meta["match_method"] = "stored_link"
				meta["habitat_sector_id"] = linked.ID
				meta["habitat_sector_name"] = linked.Name
				if zone.HabitatPlanID != nil {
					meta["habitat_plan_id"] = *zone.HabitatPlanID
				}
				return linked.ID, meta, nil
			}
			meta["link_plan_mismatch"] = true
		}
	}

	// Fast path: same numeric id only when names align (legacy imports).
	var direct models.HabitatSector
	if e := storage.DB.Where("id = ?", quartierID).First(&direct).Error; e == nil {
		if locationNamesMatch(q.Name, q.NameAr, direct.Name, direct.NameAr) {
			meta["match_method"] = "id_and_name"
			persistQuartierHabitatSector(storage.DB, quartierID, direct.ID)
			persistZoneHabitatPlan(storage.DB, zone.ID, direct.PlanID)
			return direct.ID, meta, nil
		}
	}

	var plan models.HabitatPlan
	if zone.HabitatPlanID != nil && *zone.HabitatPlanID > 0 {
		if e := storage.DB.First(&plan, *zone.HabitatPlanID).Error; e != nil {
			return 0, meta, fmt.Errorf("linked cadastre plan not found for zone")
		}
		meta["habitat_plan_id"] = plan.ID
		meta["habitat_plan_code"] = plan.Code
		meta["match_method"] = "zone_stored_plan"
	} else {
	planDB := storage.DB.Where("is_active = ?", true)
	zoneNorm := normalizeLocationName(zone.Name)
	zoneNormAr := normalizeLocationName(zone.NameAr)
	if e := planDB.Where(
		"LOWER(TRIM(code)) = LOWER(TRIM(?)) OR LOWER(TRIM(name)) = ? OR LOWER(TRIM(name_ar)) = ?",
		zone.Name, zoneNorm, zoneNormAr,
	).First(&plan).Error; e != nil {
		if e := planDB.Where(
			"name ILIKE ? OR name_ar ILIKE ? OR code ILIKE ?",
			"%"+strings.TrimSpace(zone.Name)+"%",
			"%"+strings.TrimSpace(zone.NameAr)+"%",
			"%"+strings.TrimSpace(zone.Name)+"%",
		).First(&plan).Error; e != nil {
			return 0, meta, fmt.Errorf("no cadastre zone (plan) matches '%s'", zone.Name)
		}
	}
	meta["habitat_plan_id"] = plan.ID
	meta["habitat_plan_code"] = plan.Code
	persistZoneHabitatPlan(storage.DB, zone.ID, plan.ID)
	}

	qNorm := normalizeLocationName(q.Name)
	qNormAr := normalizeLocationName(q.NameAr)
	var sector models.HabitatSector
	sectorDB := storage.DB.Where("plan_id = ?", plan.ID)
	if e := sectorDB.Where(
		"LOWER(TRIM(name)) = ? OR LOWER(TRIM(name_ar)) = ? OR LOWER(TRIM(code)) = ?",
		qNorm, qNormAr, qNorm,
	).First(&sector).Error; e != nil {
		if e := sectorDB.Where(
			"name ILIKE ? OR name_ar ILIKE ? OR code ILIKE ?",
			"%"+strings.TrimSpace(q.Name)+"%",
			"%"+strings.TrimSpace(q.NameAr)+"%",
			"%"+strings.TrimSpace(q.Name)+"%",
		).First(&sector).Error; e != nil {
			return 0, meta, fmt.Errorf("no cadastre sector matches quartier '%s' in plan %s", q.Name, plan.Code)
		}
		meta["match_method"] = "plan_and_name_fuzzy"
	} else {
		meta["match_method"] = "plan_and_name_exact"
	}
	meta["habitat_sector_name"] = sector.Name
	persistQuartierHabitatSector(storage.DB, quartierID, sector.ID)
	persistZoneHabitatPlan(storage.DB, zone.ID, plan.ID)
	return sector.ID, meta, nil
}

// findHabitatPlotInSector looks up a plot by number within a sector. When
// subSectorID is non-nil, the match is additionally scoped to that
// sub-sector — without it, a sector that has Ilot subdivisions can return a
// plot number that exists in a *different* sub-sector than the one the user
// is actually browsing, which reads as a random/wrong result.
func findHabitatPlotInSector(sectorID uint, plotNumber string, subSectorID *uint) (*models.HabitatPlot, string, error) {
	pn := strings.TrimSpace(plotNumber)
	if pn == "" {
		return nil, "", nil
	}
	norm := normalizePlotNumber(pn)

	var plot models.HabitatPlot
	forSaleJoin := `
		LEFT JOIN (
			SELECT DISTINCT habitat_plot_id
			FROM landmarks
			WHERE habitat_plot_id IS NOT NULL
			  AND is_verified = TRUE
			  AND is_published = TRUE
			  AND status = 'verified'
		) lm ON lm.habitat_plot_id = habitat_plots.id
	`

	applySubSectorScope := func(q *gorm.DB) *gorm.DB {
		if subSectorID != nil && *subSectorID > 0 {
			return q.Where("sub_sector_id = ?", *subSectorID)
		}
		return q
	}

	err := applySubSectorScope(storage.DB.Model(&models.HabitatPlot{}).
		Joins(forSaleJoin).
		Select(
			"habitat_plots.*",
			"CASE WHEN lm.habitat_plot_id IS NULL THEN FALSE ELSE TRUE END AS is_for_sale",
		).
		Preload("Sector").Preload("Plan").Preload("SubSector").
		Where("sector_id = ?", sectorID).
		Where("LOWER(REPLACE(TRIM(plot_number), ' ', '')) = ?", norm)).
		First(&plot).Error
	if err == nil {
		ensureHabitatPlotPlanSector(&plot)
		plots := []models.HabitatPlot{plot}
		fillHabitatPlotsDerivedFields(storage.DB, plots)
		plot = plots[0]
		return &plot, "exact", nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, "", err
	}

	err = applySubSectorScope(storage.DB.Model(&models.HabitatPlot{}).
		Joins(forSaleJoin).
		Select(
			"habitat_plots.*",
			"CASE WHEN lm.habitat_plot_id IS NULL THEN FALSE ELSE TRUE END AS is_for_sale",
		).
		Preload("Sector").Preload("Plan").Preload("SubSector").
		Where("sector_id = ?", sectorID).
		Where("LOWER(TRIM(plot_number)) = LOWER(TRIM(?))", pn)).
		First(&plot).Error
	if err == nil {
		ensureHabitatPlotPlanSector(&plot)
		plots := []models.HabitatPlot{plot}
		fillHabitatPlotsDerivedFields(storage.DB, plots)
		plot = plots[0]
		return &plot, "exact_trim", nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, "", nil
	}
	return nil, "", err
}

func normalizeLocationName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizePlotNumber(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	return strings.ToLower(s)
}

func locationNamesMatch(a, aAr, b, bAr string) bool {
	na := normalizeLocationName(a)
	nb := normalizeLocationName(b)
	if na != "" && nb != "" && na == nb {
		return true
	}
	naAr := normalizeLocationName(aAr)
	nbAr := normalizeLocationName(bAr)
	if naAr != "" && nbAr != "" && naAr == nbAr {
		return true
	}
	if na != "" && nbAr != "" && na == nbAr {
		return true
	}
	if naAr != "" && nb != "" && naAr == nb {
		return true
	}
	return false
}

// GET /api/habitat/plots/{plotId}
func GetHabitatPlot(ctx iris.Context) {
	plotID, _ := strconv.ParseUint(ctx.Params().Get("plotId"), 10, 32)
	var plot models.HabitatPlot
	forSaleJoin := habitatForSaleJoinSQL
	forSaleExpr := habitatForSaleSelectExpr()
	if err := storage.DB.Model(&models.HabitatPlot{}).
		Joins(forSaleJoin).
		Select(
			"habitat_plots.*",
			forSaleExpr,
		).
		Preload("Sector").
		Preload("Plan").
		Preload("SubSector").
		First(&plot, uint(plotID)).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "Plot not found"})
		return
	}
	ensureHabitatPlotPlanSector(&plot)
	before := plot
	plots := []models.HabitatPlot{plot}
	fillHabitatPlotsDerivedFields(storage.DB, plots)
	plot = plots[0]
	logHabitatPlotAPI("GET /api/habitat/plots/{plotId}", &before, &plot)
	ctx.JSON(iris.Map{"success": true, "data": plot})
}

// GET /api/habitat/plots/geometry?ids=1,2,3 — batch geometry for map rendering (max 150 ids).
func GetHabitatPlotGeometryBatch(ctx iris.Context) {
	idsParam := strings.TrimSpace(ctx.URLParam("ids"))
	if idsParam == "" {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "ids is required"})
		return
	}

	const maxIDs = 150
	parts := strings.Split(idsParam, ",")
	ids := make([]uint, 0, len(parts))
	seen := make(map[uint]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id64, err := strconv.ParseUint(part, 10, 32)
		if err != nil || id64 == 0 {
			continue
		}
		id := uint(id64)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) >= maxIDs {
			break
		}
	}
	if len(ids) == 0 {
		ctx.JSON(iris.Map{"success": true, "data": []models.HabitatPlot{}})
		return
	}

	var plots []models.HabitatPlot
	if err := storage.DB.Model(&models.HabitatPlot{}).
		Select(habitatPlotGeometrySelect()).
		Where("id IN ?", ids).
		Find(&plots).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Failed to fetch plot geometry"})
		return
	}

	fillHabitatPlotsDerivedFields(storage.DB, plots)

	beforePlots := append([]models.HabitatPlot(nil), plots...)
	logHabitatPlotAPIBatch("GET /api/habitat/plots/geometry", beforePlots, 5)
	for i := range plots {
		p := &plots[i]
		if p.ID == 396474 || strings.TrimSpace(p.PlotNumber) == "501" {
			logHabitatPlotAPI("GET /api/habitat/plots/geometry [watch]", &beforePlots[i], p)
		}
	}

	ctx.JSON(iris.Map{"success": true, "data": plots})
}

// GET /api/habitat/search?q= â€” plans, sectors (quartiers), and plots
func SearchHabitatPlots(ctx iris.Context) {
	q := strings.TrimSpace(ctx.URLParam("q"))
	if len(q) < 2 {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "Search query too short (min 2 chars)"})
		return
	}
	like := "%" + q + "%"
	codeLike := "%" + strings.ToUpper(strings.TrimSpace(q)) + "%"

	var plans []models.HabitatPlan
	storage.DB.
		Where("is_active = ?", true).
		Where("name ILIKE ? OR name_ar ILIKE ? OR code ILIKE ?", like, like, codeLike).
		Order("code ASC").
		Limit(15).
		Find(&plans)

	var sectors []models.HabitatSector
	storage.DB.
		Preload("Plan").
		Where("name ILIKE ? OR name_ar ILIKE ? OR code ILIKE ?", like, like, like).
		Order("name ASC").
		Limit(40).
		Find(&sectors)

	var subSectors []models.HabitatSubSector
	storage.DB.
		Preload("Sector").
		Preload("Sector.Plan").
		Where("plot_count > 0 AND (name ILIKE ? OR name_ar ILIKE ? OR code ILIKE ?)", like, like, like).
		Order("name ASC").
		Limit(40).
		Find(&subSectors)

	var plots []models.HabitatPlot
	plotDB := storage.DB.
		Preload("Sector").
		Preload("Plan").
		Preload("SubSector").
		Where("plot_number ILIKE ? OR l_value ILIKE ?", like, like)
	// Optional scope — narrows a plot-number search to the area the user is
	// actually browsing instead of matching that number anywhere
	// nationwide (a sector with Ilot subdivisions can otherwise return a
	// plot from a different sub-sector than the one being searched).
	if sectorIDStr := strings.TrimSpace(ctx.URLParam("sector_id")); sectorIDStr != "" {
		if sectorID, err := strconv.ParseUint(sectorIDStr, 10, 32); err == nil && sectorID > 0 {
			plotDB = plotDB.Where("sector_id = ?", uint(sectorID))
		}
	}
	if subSectorIDStr := strings.TrimSpace(ctx.URLParam("sub_sector_id")); subSectorIDStr != "" {
		if subSectorID, err := strconv.ParseUint(subSectorIDStr, 10, 32); err == nil && subSectorID > 0 {
			plotDB = plotDB.Where("sub_sector_id = ?", uint(subSectorID))
		}
	}
	exact := strings.TrimSpace(q)
	plotDB = plotDB.Order(gorm.Expr(
		"CASE WHEN plot_number ILIKE ? THEN 0 WHEN plot_number = ? THEN 1 ELSE 2 END, plot_number ASC",
		exact, exact,
	))
	plotDB.Limit(40).Find(&plots)
	fillHabitatPlotsDerivedFields(storage.DB, plots)
	logHabitatPlotAPIBatch("GET /api/habitat/search", plots, 5)
	for i := range plots {
		p := &plots[i]
		if p.ID == 396474 || strings.TrimSpace(p.PlotNumber) == "501" {
			logHabitatPlotAPI("GET /api/habitat/search [watch]", p, p)
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data": iris.Map{
			"plans":       plans,
			"sectors":     sectors,
			"sub_sectors": subSectors,
			"plots":       plots,
		},
	})
}

// POST /api/admin/habitat/backfill-plot-fields
// Copies el_value, dimensions, sides_m, area, etc. from raw_properties, corners, and geometry.
func AdminHabitatBackfillPlotFields(ctx iris.Context) {
	updated, err := backfillHabitatPlotColumnsFromRaw(storage.DB, 500)
	if err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "Backfill failed: " + err.Error()})
		return
	}
	ctx.JSON(iris.Map{
		"success": true,
		"updated": updated,
		"message": fmt.Sprintf("Backfilled %d plots from raw_properties, corners, and geometry", updated),
	})
}
