package routes

import (
	"net/http"
	"os"
	"strconv"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// GET /api/habitat/sectors/{sectorId}/diag
// Read-only geometry health for a quartier — answers WHY plots aren't
// rendering. plot rendering (MVT) needs geom IS NOT NULL; geom is backfilled
// from geom_geojson. This breaks the numbers apart so the cause is obvious:
//   - backfillable > 0        → backfill hasn't finished; trigger it
//   - with_geojson < total    → source geometry missing (import problem)
//   - with_geom == total      → fully rendering
func GetHabitatSectorDiag(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sector id"})
		return
	}

	var total, withGeom, withGeoJSON, withCentroid, backfillable int64
	var withCorners, withRawProps, cornersRecoverable int64
	sec := func() *gorm.DB { return storage.DB.Model(&models.HabitatPlot{}).Where("sector_id = ?", sectorID) }
	sec().Count(&total)
	sec().Where("geom IS NOT NULL").Count(&withGeom)
	sec().Where("geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')").Count(&withGeoJSON)
	sec().Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").Count(&withCentroid)
	sec().Where("geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')").Count(&backfillable)
	// Recovery sources for plots missing geom_geojson.
	sec().Where("corners IS NOT NULL AND corners::text NOT IN ('null','{}','[]')").Count(&withCorners)
	sec().Where("raw_properties IS NOT NULL AND raw_properties::text NOT IN ('null','{}')").Count(&withRawProps)
	// Plots with NO usable geom_geojson but WITH corners → reconstructable.
	sec().
		Where("(geom_geojson IS NULL OR geom_geojson::text IN ('null','{}'))").
		Where("corners IS NOT NULL AND corners::text NOT IN ('null','{}','[]')").
		Count(&cornersRecoverable)

	diagnosis := "ok — all plots render"
	switch {
	case total == 0:
		diagnosis = "sector has no plots"
	case backfillable > 0:
		diagnosis = "geom backfill incomplete — trigger /habitat/admin/backfill-geom"
	case withGeoJSON >= total && withGeom < total:
		diagnosis = "some rows have malformed geometry (skipped by backfill)"
	case withGeoJSON < total && cornersRecoverable > 0:
		diagnosis = "geom_geojson missing but corners present — geometry is RECOVERABLE from corners without re-import"
	case withGeoJSON < total:
		diagnosis = "geometry absent from DB entirely (no geom_geojson, no corners) — must re-import source data for these plots"
	}

	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(iris.Map{
		"api_version":         HabitatAPIVersion,
		"sector_id":           sectorID,
		"total_plots":         total,
		"with_geom":           withGeom,           // renders in MVT tiles (PostGIS path)
		"with_geom_geojson":   withGeoJSON,        // renders in legacy path + backfill source
		"with_centroid":       withCentroid,       // drives tilejson bounds
		"with_corners":        withCorners,        // alt geometry source
		"with_raw_properties": withRawProps,       // last-resort source
		"corners_recoverable": cornersRecoverable, // missing geojson but has corners
		"backfillable":        backfillable,       // geom NULL but geojson present
		"postgis_ready":       storage.HabitatPostGISReady,
		"diagnosis":           diagnosis,
	})
}

// POST /api/habitat/admin/backfill-geom?secret=...
// Forces the geom+centroid backfill to run to COMPLETION now (loops all
// batches), instead of waiting on the hourly ticker / 300k-per-run cap.
// Protected by ADMIN_BACKFILL_SECRET (must be set in env; empty disables).
func TriggerHabitatGeomBackfill(ctx iris.Context) {
	secret := os.Getenv("ADMIN_BACKFILL_SECRET")
	if secret == "" || ctx.URLParam("secret") != secret {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden"})
		return
	}

	var pendingBefore int64
	storage.DB.Model(&models.HabitatPlot{}).
		Where("geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')").
		Count(&pendingBefore)

	// Run to completion in the background so the request returns immediately
	// (backfilling hundreds of thousands of rows can take minutes).
	go storage.BackfillHabitatPlotGeomToCompletion()

	ctx.JSON(iris.Map{
		"api_version":    HabitatAPIVersion,
		"status":         "backfill started (running to completion in background)",
		"pending_before": pendingBefore,
		"check_progress": "GET /api/habitat/sectors/{id}/diag",
	})
}
