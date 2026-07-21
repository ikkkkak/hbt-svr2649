package routes

import (
	"log"
	"net/http"
	"os"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// BackfillHabitatCentroidsFromGeoJSON derives centroid_lat/centroid_lng from
// geom_geojson for plots missing a centroid — WITHOUT PostGIS. Parsing is
// done in Go with the same helpers the tile/derived paths use, so it works
// on managed hosts where the PostGIS extension is unavailable.
//
// Why this matters: the TileJSON bounds + plot_count (camera fit + health
// metric) are centroid-based on the non-PostGIS path. Plots imported with
// geometry but no centroid left the quartier reporting plot_count 0 and a
// degenerate bounding box, so the map framed the wrong area even though the
// legacy tile path could render the geometry.
//
// Batched, idempotent (only touches NULL-centroid rows), progress-logged.
func BackfillHabitatCentroidsFromGeoJSON() {
	if storage.DB == nil {
		return
	}
	var pending int64
	storage.DB.Model(&models.HabitatPlot{}).
		Where("(centroid_lat IS NULL OR centroid_lng IS NULL) AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')").
		Count(&pending)
	if pending == 0 {
		return
	}
	log.Printf("🔧 habitat: deriving centroids from geom_geojson for %d plots...", pending)

	const batchSize = 2000
	updated := 0
	for {
		var plots []models.HabitatPlot
		if err := storage.DB.
			Select("id", "geom_geojson", "centroid_lat", "centroid_lng").
			Where("(centroid_lat IS NULL OR centroid_lng IS NULL) AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')").
			Limit(batchSize).
			Find(&plots).Error; err != nil {
			log.Printf("⚠️ habitat: centroid-from-geojson fetch error: %v", err)
			return
		}
		if len(plots) == 0 {
			break
		}

		batchUpdated := 0
		for i := range plots {
			ring := primaryRingFromGeoJSON(plots[i].GeomGeoJSON)
			if len(ring) < 3 {
				// Mark as processed with a sentinel-free skip: set centroid to
				// the ring's first point if any, else leave NULL (won't retry
				// forever because the WHERE also excludes rows we can't fix —
				// but to avoid an infinite loop on unparseable rows, we bump
				// them out of the batch by writing a 0/0 guard only if truly
				// empty). Simpler: skip; the LIMIT+offset-free loop would spin,
				// so we instead break when a full batch yields zero updates.
				continue
			}
			lat, lng, ok := ringCentroid(ring)
			if !ok {
				continue
			}
			lat, lng = normalizeMauritaniaLatLng(lat, lng)
			if err := storage.DB.Model(&models.HabitatPlot{}).
				Where("id = ?", plots[i].ID).
				Updates(map[string]interface{}{"centroid_lat": lat, "centroid_lng": lng}).Error; err == nil {
				batchUpdated++
			}
		}
		updated += batchUpdated
		// If a whole batch produced no updates, the remaining rows are all
		// unparseable — stop instead of looping forever on them.
		if batchUpdated == 0 {
			break
		}
	}
	log.Printf("✅ habitat: centroid-from-geojson backfill complete (%d updated)", updated)
}

// POST /api/habitat/admin/backfill-centroids?secret=...
// Forces the PostGIS-free centroid derivation to run now (background).
func TriggerHabitatCentroidBackfill(ctx iris.Context) {
	secret := os.Getenv("ADMIN_BACKFILL_SECRET")
	if secret == "" || ctx.URLParam("secret") != secret {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden"})
		return
	}
	var pending int64
	storage.DB.Model(&models.HabitatPlot{}).
		Where("(centroid_lat IS NULL OR centroid_lng IS NULL) AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null','{}')").
		Count(&pending)

	go BackfillHabitatCentroidsFromGeoJSON()

	ctx.JSON(iris.Map{
		"api_version":    HabitatAPIVersion,
		"status":         "centroid backfill started (background)",
		"pending_before": pending,
		"check_progress": "GET /api/habitat/sectors/{id}/diag",
	})
}
