package routes

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/mvt"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/maptile"
	"gorm.io/datatypes"
)

const (
	habitatTileMinZoom = 12
	habitatTileMaxZoom = 20

	// habitatTileMaxFeatures bounds the legacy Go/orb fallback. Raised from
	// 4096 → 20000 because with PostGIS unavailable on the managed host, the
	// legacy geom_geojson path is the PRIMARY renderer, and an 8K-plot
	// quartier's overview tile must not silently truncate. Per-request decode
	// cost is acceptable for completeness (government/enterprise use).
	habitatTileMaxFeatures = 20000

	// habitatPostGISTileMaxFeatures bounds the ST_AsMVT path (GIST-indexed,
	// set-based, computed entirely inside Postgres). The real limiting
	// factor there is the tile's geographic extent, not row count, so this
	// is a generous sanity ceiling — not a functional cap — high enough
	// that no real quartier (7-8K+ plots) ever gets silently truncated.
	habitatPostGISTileMaxFeatures = 20000

	// habitatTileCentroidBufferDeg pads the legacy centroid-in-tile filter so
	// large plots whose centroid lies just outside a tile still render in it.
	// ~0.004° ≈ 440m at Nouakchott's latitude — covers the largest civic
	// parcels. Purely additive; off-tile geometry is clipped.
	habitatTileCentroidBufferDeg = 0.004
)

// habitatForSaleTileJoinSQL mirrors habitatForSaleJoinSQL (habitat_plot_response.go)
// but is written for the raw-SQL PostGIS tile queries below (aliased "p" not
// "habitat_plots", and returns a boolean directly instead of a CASE alias).
const habitatForSaleTileJoinSQL = `
	LEFT JOIN (
		SELECT DISTINCT habitat_plot_id
		FROM landmarks
		WHERE habitat_plot_id IS NOT NULL
		  AND is_verified = TRUE
		  AND is_published = TRUE
		  AND status = 'verified'
	) fs ON fs.habitat_plot_id = p.id
`

// GET /api/habitat/sectors/{sectorId}/tiles.json
// TileJSON descriptor for MapLibre VectorSource.
func GetHabitatSectorTileJSON(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sector id"})
		return
	}

	var sector models.HabitatSector
	if err := storage.DB.Select("id", "name", "name_ar", "centroid_lat", "centroid_lng").
		First(&sector, uint(sectorID)).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "sector not found"})
		return
	}

	bounds, center, plotCount, ok := habitatSectorTileBounds(uint(sectorID), &sector)
	if !ok {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "sector has no plot bounds"})
		return
	}

	scheme := "https"
	if ctx.Request().TLS == nil {
		if h := ctx.GetHeader("X-Forwarded-Proto"); h != "" {
			scheme = h
		} else {
			scheme = "http"
		}
	}
	host := ctx.Host()
	tileTemplate := fmt.Sprintf("%s://%s/api/habitat/sectors/%d/tiles/{z}/{x}/{y}", scheme, host, sectorID)

	name := sector.Name
	if sector.NameAr != "" {
		name = sector.NameAr
	}

	ctx.Header("Cache-Control", "public, max-age=600")
	ctx.JSON(iris.Map{
		"tilejson":    "3.0.0",
		// Deploy fingerprint — if this doesn't match routes.HabitatAPIVersion
		// in the source, the live server is running a stale binary.
		"api_version": HabitatAPIVersion,
		"name":        fmt.Sprintf("Habitat sector %d — %s", sector.ID, name),
		"description": "Cadastre plot polygons (MVT). Feature id = habitat_plots.id; properties: plot_number, sector_id, plan_id, area_m2, area_rounded, is_for_sale.",
		"minzoom":     habitatTileMinZoom,
		"maxzoom":     habitatTileMaxZoom,
		"bounds":      bounds,
		"center":      center,
		"plot_count":  plotCount,
		"tiles":       []string{tileTemplate},
		"vector_layers": []iris.Map{
			{
				"id":          "plots",
				"description": "Cadastre parcel polygons",
				"minzoom":     habitatTileMinZoom,
				"maxzoom":     habitatTileMaxZoom,
				"fields": iris.Map{
					"id":           "Number",
					"plot_number":  "String",
					"sector_id":    "Number",
					"plan_id":      "Number",
					"area_m2":      "Number",
					"area_rounded": "Number",
					"is_for_sale":  "Boolean",
				},
			},
		},
	})
}

// GET /api/habitat/sectors/{sectorId}/tiles/{z}/{x}/{y}
// Mapbox Vector Tile for plots in one sector (GPU-ready; no GeoJSON bulk download).
//
// Primary path: a single ST_AsMVT query in Postgres, GIST-indexed on
// habitat_plots.geom (see storage.HabitatPostGISReady / ensureHabitatPostGIS).
// This is the standard Mapbox/Mapnik tile-serving pattern and is what lets
// this endpoint scale to 10K+ plot sectors and concurrent load.
//
// Fallback: the original Go/orb implementation that decodes geom_geojson
// per row. Used only when PostGIS isn't available/ready, or for the (rare,
// self-healing) rows where the geom backfill hasn't reached yet.
func GetHabitatSectorVectorTile(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sector id"})
		return
	}

	z, x, y, ok := parseHabitatTileCoords(ctx)
	if !ok {
		return
	}

	cacheKey := habitatTileCacheKey(fmt.Sprintf("sector:%d", sectorID), z, x, y, habitatSectorDataVersion(uint(sectorID)))
	if cached, hit := habitatTileCacheGet(cacheKey); hit {
		writeHabitatTile(ctx, cached)
		return
	}

	var data []byte
	usedPostGIS := false
	if storage.HabitatPostGISReady {
		if d, perr := habitatSectorTilePostGIS(uint(sectorID), z, x, y); perr == nil {
			data = d
			usedPostGIS = true
		} else {
			log.Printf("habitat tile: PostGIS query failed for sector=%d z=%d x=%d y=%d, falling back: %v", sectorID, z, x, y, perr)
		}
	}
	if !usedPostGIS {
		d, lerr := habitatSectorTileLegacy(uint(sectorID), z, x, y)
		if lerr != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "failed to encode vector tile"})
			return
		}
		data = d
	}

	if len(data) == 0 {
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	habitatTileCacheSet(cacheKey, data)
	writeHabitatTile(ctx, data)
}

// GET /api/habitat/tiles/{z}/{x}/{y}
// Nationwide vector tile — no sector scoping. PostGIS-only (the unscoped
// query relies entirely on the GIST index; the legacy per-row Go path can't
// safely bound an unscoped fetch). Not wired into the app yet — the product
// flow still picks a quartier first — but this makes nationwide cadastre
// browsing possible later without another migration.
func GetHabitatNationwideVectorTile(ctx iris.Context) {
	z, x, y, ok := parseHabitatTileCoords(ctx)
	if !ok {
		return
	}

	if !storage.HabitatPostGISReady {
		ctx.StatusCode(http.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "nationwide tiles require PostGIS, which is not ready on this server yet"})
		return
	}

	cacheKey := habitatTileCacheKey("nationwide", z, x, y, "")
	if cached, hit := habitatTileCacheGet(cacheKey); hit {
		writeHabitatTile(ctx, cached)
		return
	}

	data, err := habitatNationwideTilePostGIS(z, x, y)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to encode vector tile"})
		return
	}
	if len(data) == 0 {
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	habitatTileCacheSet(cacheKey, data)
	writeHabitatTile(ctx, data)
}

func parseHabitatTileCoords(ctx iris.Context) (z, x, y int, ok bool) {
	var err1, err2, err3 error
	z, err1 = strconv.Atoi(ctx.Params().Get("z"))
	x, err2 = strconv.Atoi(ctx.Params().Get("x"))
	y, err3 = strconv.Atoi(ctx.Params().Get("y"))
	if err1 != nil || err2 != nil || err3 != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid tile coordinates"})
		return 0, 0, 0, false
	}
	if z < habitatTileMinZoom || z > habitatTileMaxZoom {
		ctx.StatusCode(http.StatusNoContent)
		return 0, 0, 0, false
	}
	maxTile := int(math.Pow(2, float64(z))) - 1
	if x < 0 || y < 0 || x > maxTile || y > maxTile {
		ctx.StatusCode(http.StatusNoContent)
		return 0, 0, 0, false
	}
	return z, x, y, true
}

func writeHabitatTile(ctx iris.Context, data []byte) {
	ctx.ContentType("application/vnd.mapbox-vector-tile")
	// Not "immutable": for-sale status flips via the linked landmarks table
	// (no habitat_plots.updated_at change), so the in-process tile cache and
	// this header share the same conservative 5-minute staleness window.
	ctx.Header("Cache-Control", "public, max-age=300")
	ctx.Write(data)
}

// habitatSectorTilePostGIS generates the tile inside Postgres via ST_AsMVT —
// no per-row Go JSON decode or polygon construction. Requires
// storage.HabitatPostGISReady (habitat_plots.geom + GIST index present).
func habitatSectorTilePostGIS(sectorID uint, z, x, y int) ([]byte, error) {
	const sql = `
		WITH bounds AS (
			SELECT ST_TileEnvelope(?, ?, ?) AS tile
		),
		features AS (
			SELECT
				ST_AsMVTGeom(ST_Transform(p.geom, 3857), bounds.tile, 4096, 64, true) AS mvtgeom,
				p.id, p.plot_number, p.sector_id, p.plan_id,
				p.area_m2, p.area_rounded,
				(fs.habitat_plot_id IS NOT NULL) AS is_for_sale
			FROM habitat_plots p
			CROSS JOIN bounds
			` + habitatForSaleTileJoinSQL + `
			WHERE p.sector_id = ?
			  AND p.geom IS NOT NULL
			  AND p.geom && ST_Transform(bounds.tile, 4326)
			ORDER BY p.id
			LIMIT ?
		)
		SELECT ST_AsMVT(features.*, 'plots', 4096, 'mvtgeom') FROM features WHERE mvtgeom IS NOT NULL
	`
	var data []byte
	if err := storage.DB.Raw(sql, z, x, y, sectorID, habitatPostGISTileMaxFeatures).Row().Scan(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// habitatNationwideTilePostGIS is the same query as habitatSectorTilePostGIS
// without the sector_id filter — the geom && bounds check (GIST-indexed) is
// the only filter, which is exactly what a nationwide vector layer needs.
func habitatNationwideTilePostGIS(z, x, y int) ([]byte, error) {
	const sql = `
		WITH bounds AS (
			SELECT ST_TileEnvelope(?, ?, ?) AS tile
		),
		features AS (
			SELECT
				ST_AsMVTGeom(ST_Transform(p.geom, 3857), bounds.tile, 4096, 64, true) AS mvtgeom,
				p.id, p.plot_number, p.sector_id, p.plan_id,
				p.area_m2, p.area_rounded,
				(fs.habitat_plot_id IS NOT NULL) AS is_for_sale
			FROM habitat_plots p
			CROSS JOIN bounds
			` + habitatForSaleTileJoinSQL + `
			WHERE p.geom IS NOT NULL
			  AND p.geom && ST_Transform(bounds.tile, 4326)
			ORDER BY p.id
			LIMIT ?
		)
		SELECT ST_AsMVT(features.*, 'plots', 4096, 'mvtgeom') FROM features WHERE mvtgeom IS NOT NULL
	`
	var data []byte
	if err := storage.DB.Raw(sql, z, x, y, habitatPostGISTileMaxFeatures).Row().Scan(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// habitatSectorTileLegacy is the pre-PostGIS implementation: decodes
// geom_geojson JSONB into orb polygons in Go and encodes MVT by hand. Kept as
// a safety-net fallback for environments without PostGIS and for the rare
// row still missing a backfilled geom column.
func habitatSectorTileLegacy(sectorID uint, z, x, y int) ([]byte, error) {
	tile := maptile.New(uint32(x), uint32(y), maptile.Zoom(z))
	tileBound := tile.Bound()
	minLng, minLat := tileBound.Min.Lon(), tileBound.Min.Lat()
	maxLng, maxLat := tileBound.Max.Lon(), tileBound.Max.Lat()

	type plotRow struct {
		ID          uint           `gorm:"column:id"`
		PlotNumber  string         `gorm:"column:plot_number"`
		SectorID    uint           `gorm:"column:sector_id"`
		PlanID      uint           `gorm:"column:plan_id"`
		AreaM2      *float64       `gorm:"column:area_m2"`
		AreaRounded *int           `gorm:"column:area_rounded"`
		GeomGeoJSON datatypes.JSON `gorm:"column:geom_geojson"`
		IsForSale   bool           `gorm:"column:is_for_sale"`
	}

	forSaleJoin := habitatForSaleJoinSQL
	forSaleExpr := habitatForSaleSelectExpr()

	var rows []plotRow
	q := storage.DB.Model(&models.HabitatPlot{}).
		Joins(forSaleJoin).
		Select(
			"habitat_plots.id",
			"habitat_plots.plot_number",
			"habitat_plots.sector_id",
			"habitat_plots.plan_id",
			"habitat_plots.area_m2",
			"habitat_plots.area_rounded",
			"habitat_plots.geom_geojson",
			forSaleExpr,
		).
		Where("habitat_plots.sector_id = ?", sectorID).
		Where("habitat_plots.geom_geojson IS NOT NULL AND habitat_plots.geom_geojson::text NOT IN ('null', '{}')").
		// Centroid-in-tile is an APPROXIMATE spatial filter (no PostGIS on the
		// host). Buffer the tile bounds by ~0.004° (~440m) so a large plot
		// (mosque, school, market — hundreds of metres across) whose centroid
		// falls in a neighbouring tile still renders in every tile its polygon
		// overlaps, instead of vanishing at high zoom. Off-tile geometry is
		// clipped away below, so the buffer only ADDS the plots that belong.
		Where("habitat_plots.centroid_lat BETWEEN ? AND ?", minLat-habitatTileCentroidBufferDeg, maxLat+habitatTileCentroidBufferDeg).
		Where("habitat_plots.centroid_lng BETWEEN ? AND ?", minLng-habitatTileCentroidBufferDeg, maxLng+habitatTileCentroidBufferDeg).
		Limit(habitatTileMaxFeatures)

	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	features := make([]*geojson.Feature, 0, len(rows))
	for _, row := range rows {
		ring := primaryRingFromGeoJSON(row.GeomGeoJSON)
		if len(ring) < 3 {
			ring = primaryRingFromPlot(&models.HabitatPlot{GeomGeoJSON: row.GeomGeoJSON})
		}
		if len(ring) < 3 {
			continue
		}

		orbRing := make(orb.Ring, 0, len(ring))
		for _, pt := range ring {
			orbRing = append(orbRing, orb.Point{pt.Lng, pt.Lat})
		}
		if len(orbRing) > 0 && !orbRing[0].Equal(orbRing[len(orbRing)-1]) {
			orbRing = append(orbRing, orbRing[0])
		}

		// Normalize winding order. orb's mvt.Marshal does NOT correct ring
		// orientation (only its decoder does), so a source polygon wound the
		// "wrong" way encodes a broken fill that Mapbox GL renders hollow/
		// corrupted — unnoticeable when plots are sub-pixel, obvious when
		// zoomed in. MVT requires CLOCKWISE exterior rings in tile space;
		// ProjectToTile flips Y (inverting orientation), so the exterior must
		// be COUNTER-CLOCKWISE here in geographic space to land clockwise in
		// the tile. Force it.
		if orbRing.Orientation() != orb.CCW {
			orbRing.Reverse()
		}

		f := geojson.NewFeature(orb.Polygon{orbRing})
		f.ID = row.ID
		f.Properties = geojson.Properties{
			"id":          row.ID,
			"plot_number": row.PlotNumber,
			"sector_id":   row.SectorID,
			"plan_id":     row.PlanID,
			"is_for_sale": row.IsForSale,
		}
		if row.AreaM2 != nil {
			f.Properties["area_m2"] = *row.AreaM2
		}
		if row.AreaRounded != nil {
			f.Properties["area_rounded"] = *row.AreaRounded
		}
		features = append(features, f)
	}

	if len(features) == 0 {
		return nil, nil
	}

	collections := map[string]*geojson.FeatureCollection{
		"plots": {Features: features},
	}
	layers := mvt.NewLayers(collections)
	layers.ProjectToTile(tile)
	layers.Clip(mvt.MapboxGLDefaultExtentBound)

	return mvt.Marshal(layers)
}

func habitatSectorTileBounds(sectorID uint, sector *models.HabitatSector) (bounds [4]float64, center [3]float64, plotCount int64, ok bool) {
	type extent struct {
		MinLat    float64 `gorm:"column:min_lat"`
		MaxLat    float64 `gorm:"column:max_lat"`
		MinLng    float64 `gorm:"column:min_lng"`
		MaxLng    float64 `gorm:"column:max_lng"`
		PlotCount int64   `gorm:"column:plot_count"`
	}
	var ex extent
	// Bounds + count reflect RENDER-TRUTH: only plots with a non-NULL geom
	// (PostGIS) actually appear in the MVT tiles, so plot_count here equals
	// the number of plots the map will draw — a meaningful health metric
	// (a quartier whose geom backfill hasn't run shows a low count). Extent
	// comes from the geom itself (ST_Extent), independent of the separate
	// centroid columns.
	geomFilter := "geom IS NOT NULL"
	selects := []string{
		"ST_YMin(ST_Extent(geom)) AS min_lat",
		"ST_YMax(ST_Extent(geom)) AS max_lat",
		"ST_XMin(ST_Extent(geom)) AS min_lng",
		"ST_XMax(ST_Extent(geom)) AS max_lng",
		"COUNT(*) AS plot_count",
	}
	if !storage.HabitatPostGISReady {
		// No PostGIS geom column — fall back to centroid-based extent.
		geomFilter = "centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL"
		selects = []string{
			"MIN(centroid_lat) AS min_lat",
			"MAX(centroid_lat) AS max_lat",
			"MIN(centroid_lng) AS min_lng",
			"MAX(centroid_lng) AS max_lng",
			"COUNT(*) AS plot_count",
		}
	}
	err := storage.DB.Model(&models.HabitatPlot{}).
		Select(selects).
		Where("sector_id = ?", sectorID).
		Where(geomFilter).
		Scan(&ex).Error
	plotCount = ex.PlotCount
	if err != nil || (ex.MinLat == 0 && ex.MaxLat == 0 && ex.MinLng == 0 && ex.MaxLng == 0) {
		if sector.CentroidLat != nil && sector.CentroidLng != nil {
			pad := 0.01
			bounds = [4]float64{
				*sector.CentroidLng - pad,
				*sector.CentroidLat - pad,
				*sector.CentroidLng + pad,
				*sector.CentroidLat + pad,
			}
			center = [3]float64{*sector.CentroidLng, *sector.CentroidLat, 15}
			return bounds, center, plotCount, true
		}
		return bounds, center, plotCount, false
	}

	padLat := math.Max(0.001, (ex.MaxLat-ex.MinLat)*0.05)
	padLng := math.Max(0.001, (ex.MaxLng-ex.MinLng)*0.05)
	bounds = [4]float64{
		ex.MinLng - padLng,
		ex.MinLat - padLat,
		ex.MaxLng + padLng,
		ex.MaxLat + padLat,
	}
	cLng := (ex.MinLng + ex.MaxLng) / 2
	cLat := (ex.MinLat + ex.MaxLat) / 2
	center = [3]float64{cLng, cLat, 15}
	return bounds, center, plotCount, true
}

// --- In-process tile cache -------------------------------------------------
//
// Keyed by scope (sector id or "nationwide") + z/x/y + a data version, so
// repeat/concurrent requests for the same tile (very common — many users
// panning the same popular quartier) skip Postgres entirely. The version is
// derived from MAX(habitat_plots.updated_at) and itself cached for a short
// TTL to avoid an extra query per tile request.
//
// Known limitation: a plot's is_for_sale flag comes from a JOIN against the
// landmarks table and does not touch habitat_plots.updated_at, so verifying/
// publishing a listing can take up to the cache TTL to show up in tiles.
// Acceptable given the 5-minute HTTP Cache-Control the client also honors.

type habitatTileCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// habitatTileCacheTTL matches the client Cache-Control lifetime (see
// writeHabitatRasterTile) — a short in-process TTL was causing the server to
// silently re-render+re-query tiles that had already been generated minutes
// earlier, adding avoidable render latency right when a user is mid-zoom.
const habitatTileCacheTTL = 24 * time.Hour
const habitatSectorVersionTTL = 60 * time.Second
const habitatTileCacheMaxEntries = 20000

var (
	habitatTileCacheMu sync.RWMutex
	habitatTileCache   = map[string]habitatTileCacheEntry{}

	habitatSectorVersionMu sync.RWMutex
	habitatSectorVersion   = map[uint]habitatSectorVersionEntry{}
)

type habitatSectorVersionEntry struct {
	version   string
	fetchedAt time.Time
}

func habitatTileCacheKey(scope string, z, x, y int, version string) string {
	return fmt.Sprintf("%s:%d:%d:%d:%s", scope, z, x, y, version)
}

func habitatSectorDataVersion(sectorID uint) string {
	habitatSectorVersionMu.RLock()
	entry, ok := habitatSectorVersion[sectorID]
	habitatSectorVersionMu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < habitatSectorVersionTTL {
		return entry.version
	}

	var lastUpdated time.Time
	storage.DB.Raw(`SELECT COALESCE(MAX(updated_at), NOW()) FROM habitat_plots WHERE sector_id = ?`, sectorID).Scan(&lastUpdated)
	version := lastUpdated.UTC().Format(time.RFC3339Nano)

	habitatSectorVersionMu.Lock()
	habitatSectorVersion[sectorID] = habitatSectorVersionEntry{version: version, fetchedAt: time.Now()}
	habitatSectorVersionMu.Unlock()
	return version
}

func habitatTileCacheGet(key string) ([]byte, bool) {
	habitatTileCacheMu.RLock()
	entry, ok := habitatTileCache[key]
	habitatTileCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func habitatTileCacheSet(key string, data []byte) {
	habitatTileCacheMu.Lock()
	defer habitatTileCacheMu.Unlock()
	if len(habitatTileCache) > habitatTileCacheMaxEntries {
		// Cheap unbounded-growth guard — drop everything and let hot tiles
		// repopulate; simpler and safer under load than per-entry LRU bookkeeping.
		habitatTileCache = map[string]habitatTileCacheEntry{}
	}
	habitatTileCache[key] = habitatTileCacheEntry{data: data, expiresAt: time.Now().Add(habitatTileCacheTTL)}
}
