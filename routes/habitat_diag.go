package routes

import (
	"net/http"
	"os"
	"strconv"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
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
	storage.DB.Model(&models.HabitatPlot{}).Where("sector_id = ?", sectorID).Count(&total)
	storage.DB.Model(&models.HabitatPlot{}).Where("sector_id = ? AND geom IS NOT NULL", sectorID).Count(&withGeom)
	storage.DB.Model(&models.HabitatPlot{}).
		Where("sector_id = ? AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')", sectorID).
		Count(&withGeoJSON)
	storage.DB.Model(&models.HabitatPlot{}).
		Where("sector_id = ? AND centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL", sectorID).
		Count(&withCentroid)
	storage.DB.Model(&models.HabitatPlot{}).
		Where("sector_id = ? AND geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')", sectorID).
		Count(&backfillable)

	diagnosis := "ok — all plots render"
	switch {
	case total == 0:
		diagnosis = "sector has no plots"
	case backfillable > 0:
		diagnosis = "geom backfill incomplete — trigger /habitat/admin/backfill-geom"
	case withGeoJSON < total:
		diagnosis = "source geometry missing (geom_geojson) — data import problem, backfill cannot help these rows"
	case withGeom < total:
		diagnosis = "some rows have malformed geometry (skipped by backfill)"
	}

	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(iris.Map{
		"api_version":       HabitatAPIVersion,
		"sector_id":         sectorID,
		"total_plots":       total,
		"with_geom":         withGeom,     // renders in MVT tiles
		"with_geom_geojson": withGeoJSON,  // backfill source
		"with_centroid":     withCentroid, // drives tilejson bounds
		"backfillable":      backfillable, // geom NULL but geojson present
		"postgis_ready":     storage.HabitatPostGISReady,
		"diagnosis":         diagnosis,
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
