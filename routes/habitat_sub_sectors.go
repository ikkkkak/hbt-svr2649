package routes

import (
	"net/http"
	"strconv"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// GET /api/habitat/sectors/{sectorId}/sub-sectors
// Only sub-sectors with at least one plot are returned — a sector with no
// Ilot subdivisions in the source cadastre data simply returns an empty
// list, and the frontend falls back straight from sector to plots.
func GetHabitatSubSectorsBySector(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sector id"})
		return
	}

	var subSectors []models.HabitatSubSector
	if err := storage.DB.
		Where("sector_id = ? AND plot_count > 0", uint(sectorID)).
		Order("name ASC").
		Find(&subSectors).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch sub-sectors"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "data": subSectors})
}

// GET /api/habitat/sub-sectors/{subSectorId}
func GetHabitatSubSector(ctx iris.Context) {
	subSectorID, err := strconv.ParseUint(ctx.Params().Get("subSectorId"), 10, 32)
	if err != nil || subSectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sub-sector id"})
		return
	}

	var subSector models.HabitatSubSector
	if err := storage.DB.
		Preload("Sector").
		Preload("Sector.Plan").
		First(&subSector, uint(subSectorID)).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Sub-sector not found"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "data": subSector})
}

// GET /api/habitat/sub-sectors/{subSectorId}/plots
// Same query shape as GetHabitatPlotsBySector (page/limit, all=true, map=true, lite=true)
// so the frontend can reuse its existing plot-list handling for this tier.
func GetHabitatPlotsBySubSector(ctx iris.Context) {
	subSectorID, err := strconv.ParseUint(ctx.Params().Get("subSectorId"), 10, 32)
	if err != nil || subSectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sub-sector id"})
		return
	}

	fetchAll := ctx.URLParam("all") == "true" || ctx.URLParam("all") == "1"
	mapMode := ctx.URLParam("map") == "true" || ctx.URLParam("map") == "1"
	liteMode := ctx.URLParam("lite") == "true" || ctx.URLParam("lite") == "1"

	var total int64
	storage.DB.Model(&models.HabitatPlot{}).Where("sub_sector_id = ?", uint(subSectorID)).Count(&total)

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
			Where("sub_sector_id = ?", uint(subSectorID)).
			Select(plotSelect).
			Order(habitatPlotNaturalOrderSQL)
		if total > int64(maxAll) {
			q = q.Limit(maxAll)
		}
		if err := q.Find(&plots).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to fetch plots"})
			return
		}
		fillHabitatPlotsDerivedFields(storage.DB, plots)
		enrichHabitatPlotsWithSubSector(plots, uint(subSectorID))
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
		Where("sub_sector_id = ?", uint(subSectorID)).
		Select(plotSelect).
		Order(habitatPlotNaturalOrderSQL).
		Offset(offset).Limit(limit).
		Find(&plots).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch plots"})
		return
	}
	fillHabitatPlotsDerivedFields(storage.DB, plots)
	enrichHabitatPlotsWithSubSector(plots, uint(subSectorID))
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

func enrichHabitatPlotsWithSubSector(plots []models.HabitatPlot, subSectorID uint) {
	if len(plots) == 0 || subSectorID == 0 {
		return
	}

	var subSector models.HabitatSubSector
	if err := storage.DB.First(&subSector, subSectorID).Error; err != nil {
		return
	}
	enrichHabitatPlotsWithPlanSector(plots, subSector.SectorID)

	for i := range plots {
		ss := subSector
		plots[i].SubSector = &ss
	}
}
