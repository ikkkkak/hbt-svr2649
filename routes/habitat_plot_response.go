package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

const habitatForSaleJoinSQL = `
		LEFT JOIN (
			SELECT DISTINCT habitat_plot_id
			FROM landmarks
			WHERE habitat_plot_id IS NOT NULL
			  AND is_verified = TRUE
			  AND is_published = TRUE
			  AND status = 'verified'
		) lm ON lm.habitat_plot_id = habitat_plots.id
	`

func habitatForSaleSelectExpr() string {
	return "CASE WHEN lm.habitat_plot_id IS NULL THEN FALSE ELSE TRUE END AS is_for_sale"
}

// habitatPlotMapModeSelect returns plot columns needed for map rendering and the plot callout card.
// Omits raw_properties to keep payloads smaller.
func habitatPlotMapModeSelect(forSaleExpr string) string {
	return `habitat_plots.id, habitat_plots.plan_id, habitat_plots.sector_id, habitat_plots.plot_number,
		habitat_plots.l_value, habitat_plots.i_value,
		habitat_plots.area_m2, habitat_plots.area_rounded, habitat_plots.area_hectares,
		habitat_plots.perimeter_m, habitat_plots.sides_m,
		habitat_plots.dimensions_string, habitat_plots.length_m, habitat_plots.width_m,
		habitat_plots.il_value, habitat_plots.el_value, habitat_plots.res_value,
		habitat_plots.geom_geojson, habitat_plots.centroid_lat, habitat_plots.centroid_lng, habitat_plots.corners, ` + forSaleExpr
}

// habitatPlotLiteSelect returns metadata only — no geometry (bulk quartier index).
func habitatPlotLiteSelect(forSaleExpr string) string {
	return `habitat_plots.id, habitat_plots.plan_id, habitat_plots.sector_id, habitat_plots.plot_number,
		habitat_plots.area_m2, habitat_plots.area_rounded,
		habitat_plots.dimensions_string, habitat_plots.length_m, habitat_plots.width_m,
		habitat_plots.il_value, habitat_plots.el_value, habitat_plots.res_value,
		habitat_plots.centroid_lat, habitat_plots.centroid_lng, ` + forSaleExpr
}

// habitatPlotGeometrySelect returns only fields needed to render parcel polygons.
func habitatPlotGeometrySelect() string {
	return "habitat_plots.id, habitat_plots.geom_geojson, habitat_plots.corners, habitat_plots.centroid_lat, habitat_plots.centroid_lng"
}

// habitatPlotCardSelect returns plot columns for low-zoom bbox tiles (no geometry).
func habitatPlotCardSelect(forSaleExpr string) string {
	return `habitat_plots.id, habitat_plots.plan_id, habitat_plots.sector_id, habitat_plots.plot_number,
		habitat_plots.area_m2, habitat_plots.area_rounded, habitat_plots.sides_m,
		habitat_plots.dimensions_string, habitat_plots.length_m, habitat_plots.width_m,
		habitat_plots.il_value, habitat_plots.el_value, habitat_plots.res_value,
		habitat_plots.centroid_lat, habitat_plots.centroid_lng, habitat_plots.corners, ` + forSaleExpr
}

func enrichHabitatPlotsWithPlanSector(plots []models.HabitatPlot, sectorID uint) {
	if len(plots) == 0 || sectorID == 0 {
		return
	}

	var sector models.HabitatSector
	if err := storage.DB.First(&sector, sectorID).Error; err != nil {
		return
	}

	var plan models.HabitatPlan
	if sector.PlanID > 0 {
		_ = storage.DB.First(&plan, sector.PlanID).Error
	}

	sectorForJSON := sector
	sectorForJSON.Plan = models.HabitatPlan{}

	for i := range plots {
		plots[i].Sector = sectorForJSON
		if plan.ID > 0 && (plots[i].PlanID == 0 || plots[i].PlanID == plan.ID) {
			plots[i].Plan = plan
		}
	}
}

func ensureHabitatPlotPlanSector(plot *models.HabitatPlot) {
	if plot == nil {
		return
	}
	if plot.Sector.ID == 0 && plot.SectorID > 0 {
		if err := storage.DB.First(&plot.Sector, plot.SectorID).Error; err != nil {
			plot.Sector = models.HabitatSector{}
		}
	}
	if plot.Plan.ID == 0 && plot.PlanID > 0 {
		if err := storage.DB.First(&plot.Plan, plot.PlanID).Error; err != nil {
			plot.Plan = models.HabitatPlan{}
		}
	}
}
