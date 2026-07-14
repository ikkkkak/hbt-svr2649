package routes

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/fogleman/gg"
	"github.com/kataras/iris/v12"
	"github.com/paulmach/orb/maptile"
	"gorm.io/datatypes"
)

// rasterTileSize matches the standard 256x256 web map tile convention.
const rasterTileSize = 256

// rasterTilePadding — plots are rendered onto a canvas larger than the tile
// itself, then cropped back down to rasterTileSize. Without this, any plot
// polygon crossing a tile boundary gets its stroke/fill hard-clipped exactly
// at the pixel edge (a bare 256x256 canvas has no room to draw the part of
// a boundary stroke that would fall "outside" it), which is what produced
// plots that looked half-cut/missing right at tile seams. This is the
// standard technique production tile servers (Mapnik et al.) use.
const rasterTilePadding = 24
const rasterCanvasSize = rasterTileSize + 2*rasterTilePadding

var (
	rasterPlotFill      = color.RGBA{59, 130, 246, 70}
	rasterPlotStroke    = color.RGBA{37, 99, 235, 190}
	rasterForSaleFill   = color.RGBA{239, 68, 68, 110}
	rasterForSaleStroke = color.RGBA{220, 38, 38, 220}
)

// habitatRasterRenderVersion must be bumped any time a change is made to how
// a tile is drawn (colors, stroke width, clipping/projection math, etc.) —
// NOT when plot data changes (habitatSectorDataVersion already covers that).
// Both the server-side in-process cache (24h TTL) and the client's own HTTP
// cache (Cache-Control: max-age=86400 below) are keyed off values that only
// change with the underlying data, never with the rendering code itself —
// confirmed in production: a tile fetched fresh from the live server still
// came back with stroke colors that don't exist anywhere in this file
// anymore (found via direct pixel inspection of the PNG), meaning a
// rendering change shipped at some point and every cache layer kept serving
// the pre-change tile indefinitely, with no way for it to ever self-correct.
// Bumping this string forces a new cache key server-side AND — because the
// frontend embeds it in the tile URL query string (habitatRasterOverlay.ts)
// — a genuinely new URL client-side, so old cached bitmaps are abandoned
// instead of being served forever.
const habitatRasterRenderVersion = "2"

// habitatRasterCacheVersion combines the data version (bumps whenever a
// sector's plot rows change) with habitatRasterRenderVersion (bumped by hand
// whenever this file's drawing logic changes) so either kind of change
// invalidates the server-side cache — data-only versioning was the gap that
// let a stale render survive indefinitely.
func habitatRasterCacheVersion(sectorID uint) string {
	return habitatSectorDataVersion(sectorID) + ":" + habitatRasterRenderVersion
}

// lngLatToGlobalPixel converts a geographic coordinate to Web Mercator pixel
// space at the given zoom — same projection every XYZ tile scheme (Google/
// Apple/OSM/Esri) uses, just carried through to pixels instead of tile
// indices. Matches the tile addressing scheme react-native-maps' <UrlTile>
// requests with (standard {z}/{x}/{y}, origin at top-left / north-west).
func lngLatToGlobalPixel(lng, lat float64, z int) (float64, float64) {
	scale := float64(uint64(1)<<uint(z)) * rasterTileSize
	x := (lng + 180.0) / 360.0 * scale
	sinLat := math.Sin(lat * math.Pi / 180.0)
	// Clamp to avoid log(0)/log(negative) at the poles.
	sinLat = math.Max(-0.9999, math.Min(0.9999, sinLat))
	y := (0.5 - math.Log((1+sinLat)/(1-sinLat))/(4*math.Pi)) * scale
	return x, y
}

// lngLatToTileXY returns the standard slippy-map tile index containing a
// coordinate at zoom z — used by the prewarm sweep below to turn a sector's
// plot bounding box into a range of z/x/y tiles to render ahead of time.
func lngLatToTileXY(lng, lat float64, z int) (int, int) {
	px, py := lngLatToGlobalPixel(lng, lat, z)
	return int(math.Floor(px / rasterTileSize)), int(math.Floor(py / rasterTileSize))
}

// webMercatorMetersPerPixel — for buffering the PostGIS query bounds (which
// are in EPSG:3857 meters) by rasterTilePadding pixels' worth of distance.
func webMercatorMetersPerPixel(z int) float64 {
	const earthCircumferenceM = 40075016.68557849
	return earthCircumferenceM / (float64(uint64(1)<<uint(z)) * rasterTileSize)
}

// degreesPerPixelApprox — same idea for the legacy (no-PostGIS) fallback,
// which filters on plain lat/lng degrees. Not latitude-corrected (that
// would need cos(lat)), but this is only sizing a generous inclusion
// margin, not doing precise geometry — a rough approximation is fine.
func degreesPerPixelApprox(z int) float64 {
	return 360.0 / (float64(uint64(1)<<uint(z)) * rasterTileSize)
}

// rasterPlotRing is the shared shape fed to renderPlotRingsToTile regardless
// of which query path (PostGIS or legacy) produced it.
type rasterPlotRing struct {
	ring      []geoLatLng
	isForSale bool
}

// renderPlotRingsToTile draws every ring onto a padded canvas, then crops
// back to the true tile size — shared by both query paths so the
// padding/crop logic only exists once.
func renderPlotRingsToTile(rings []rasterPlotRing, z, x, y int) ([]byte, error) {
	dc := gg.NewContext(rasterCanvasSize, rasterCanvasSize)
	dc.SetColor(color.RGBA{0, 0, 0, 0})
	dc.Clear()

	originX := float64(x)*rasterTileSize - rasterTilePadding
	originY := float64(y)*rasterTileSize - rasterTilePadding

	for _, r := range rings {
		drawPlotRing(dc, r.ring, z, originX, originY, r.isForSale)
	}

	// gg.NewContext always backs onto an *image.RGBA (see fogleman/gg's
	// NewContext -> NewContextForRGBA), so this assertion is safe.
	full, ok := dc.Image().(*image.RGBA)
	if !ok {
		return nil, errors.New("habitat raster tile: unexpected image type from gg.Context")
	}
	cropped := full.SubImage(image.Rect(
		rasterTilePadding,
		rasterTilePadding,
		rasterTilePadding+rasterTileSize,
		rasterTilePadding+rasterTileSize,
	))

	var buf bytes.Buffer
	if err := png.Encode(&buf, cropped); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pixelPt is a local (tile-relative) pixel coordinate.
type pixelPt struct{ x, y float64 }

// clipPolygonToRect clips a (closed, simple) polygon against an axis-aligned
// rectangle using Sutherland–Hodgman. This is the piece that was missing
// from the original padding/crop approach: that technique stopped hard
// pixel-clipping at the tile edge, but still handed the rasterizer whole
// plot rings whose vertices can legitimately sit hundreds of pixels outside
// the canvas — a real plot spanning several tiles at high zoom projects to
// coordinates far off-canvas at z18+ (verified against production data: one
// ~5,000m² plot's ring goes from y≈67 at z16 to y≈-49 at z20). Relying on
// the software rasterizer to clip those extreme coordinates correctly on
// its own is exactly the kind of edge case that produces distorted/
// incomplete-looking shapes. Clipping explicitly, first, guarantees every
// coordinate handed to gg is small and sane, and — because two adjacent
// tiles clip the very same source polygon against two rectangles that
// overlap through the padding — guarantees the visible edges reconstruct
// seamlessly across the tile seam. Same technique production tile
// renderers (Mapnik, tippecanoe) use.
func clipPolygonToRect(points []pixelPt, minX, minY, maxX, maxY float64) []pixelPt {
	if len(points) < 3 {
		return nil
	}
	points = clipAgainstEdge(points, func(p pixelPt) bool { return p.x >= minX }, func(a, b pixelPt) pixelPt {
		t := (minX - a.x) / (b.x - a.x)
		return pixelPt{minX, a.y + t*(b.y-a.y)}
	})
	if len(points) < 3 {
		return nil
	}
	points = clipAgainstEdge(points, func(p pixelPt) bool { return p.x <= maxX }, func(a, b pixelPt) pixelPt {
		t := (maxX - a.x) / (b.x - a.x)
		return pixelPt{maxX, a.y + t*(b.y-a.y)}
	})
	if len(points) < 3 {
		return nil
	}
	points = clipAgainstEdge(points, func(p pixelPt) bool { return p.y >= minY }, func(a, b pixelPt) pixelPt {
		t := (minY - a.y) / (b.y - a.y)
		return pixelPt{a.x + t*(b.x-a.x), minY}
	})
	if len(points) < 3 {
		return nil
	}
	points = clipAgainstEdge(points, func(p pixelPt) bool { return p.y <= maxY }, func(a, b pixelPt) pixelPt {
		t := (maxY - a.y) / (b.y - a.y)
		return pixelPt{a.x + t*(b.x-a.x), maxY}
	})
	if len(points) < 3 {
		return nil
	}
	return points
}

func clipAgainstEdge(points []pixelPt, inside func(pixelPt) bool, intersect func(a, b pixelPt) pixelPt) []pixelPt {
	n := len(points)
	out := make([]pixelPt, 0, n+2)
	for i := 0; i < n; i++ {
		curr := points[i]
		prev := points[(i-1+n)%n]
		currIn := inside(curr)
		prevIn := inside(prev)
		if currIn {
			if !prevIn {
				out = append(out, intersect(prev, curr))
			}
			out = append(out, curr)
		} else if prevIn {
			out = append(out, intersect(prev, curr))
		}
	}
	return out
}

// drawPlotRing rasterizes one plot polygon ring onto the (padded) tile
// canvas. The ring is clipped to the padded canvas rectangle first (see
// clipPolygonToRect) so the rasterizer only ever sees small, in-range
// coordinates — never the raw ring, which for a plot spanning multiple
// tiles at high zoom can have vertices hundreds of pixels off-canvas.
func drawPlotRing(dc *gg.Context, ring []geoLatLng, z int, originX, originY float64, isForSale bool) {
	if len(ring) < 3 {
		return
	}
	local := make([]pixelPt, len(ring))
	for i, pt := range ring {
		px, py := lngLatToGlobalPixel(pt.Lng, pt.Lat, z)
		local[i] = pixelPt{px - originX, py - originY}
	}

	const clipMargin = 4 // small extra slack so stroke width at the padding edge never gets a hairline clip
	clipped := clipPolygonToRect(
		local,
		-clipMargin, -clipMargin,
		rasterCanvasSize+clipMargin, rasterCanvasSize+clipMargin,
	)
	if len(clipped) < 3 {
		return
	}

	dc.NewSubPath()
	for i, p := range clipped {
		if i == 0 {
			dc.MoveTo(p.x, p.y)
		} else {
			dc.LineTo(p.x, p.y)
		}
	}
	dc.ClosePath()

	if isForSale {
		dc.SetColor(rasterForSaleFill)
	} else {
		dc.SetColor(rasterPlotFill)
	}
	dc.FillPreserve()

	if isForSale {
		dc.SetColor(rasterForSaleStroke)
	} else {
		dc.SetColor(rasterPlotStroke)
	}
	dc.SetLineWidth(1.4)
	dc.Stroke()
}

// --- Tile prewarming --------------------------------------------------------
//
// Each raster tile is rendered on demand: a DB query + a software (CPU)
// PNG rasterization per request. That's fast for a single tile, but a pinch
// zoom on a native map fires off dozens of NEW z/x/y tile requests at once
// (a zoom level change invalidates the entire previous tile grid — nothing
// from the old zoom is reusable). If those all hit the render path cold at
// the same moment, some resolve fast and some lag behind by a few hundred
// ms, and a native <UrlTile> overlay does not crossfade — a slow tile just
// renders blank until it's ready. Visually that's exactly "plots disappear /
// half the polygon is missing" during zoom, even though every individual
// tile is correct once rendered. This is the same reason "zoomed out is
// fine" (few, already-cached tiles) but "zoomed in breaks" (many never-
// before-seen tiles rendered under time pressure).
//
// The fix is to render likely tiles *before* the user reaches them: as soon
// as a sector's first raster tile is requested, sweep its plot bounding box
// across the zoom range real usage covers (14–20, matching the parcel-level
// zoom the frontend flies to on quartier selection) and populate the same
// in-process cache the request path reads from. By the time a pinch zoom
// actually lands on a given z/x/y, it's very likely already cached.
const (
	habitatPrewarmMinZoom     = 14
	habitatPrewarmMaxZoom     = 20
	habitatPrewarmMaxTiles    = 1000
	habitatPrewarmConcurrency = 8
)

// habitatPrewarmInFlight dedupes concurrent prewarm sweeps for the same
// sector+data-version — every tile request for a sector triggers a prewarm
// call, but only the first one per version actually does the work.
var habitatPrewarmInFlight sync.Map

func prewarmSectorRasterTiles(sectorID uint) {
	if !storage.HabitatPostGISReady {
		// The legacy fallback does an unindexed centroid-bbox scan per tile;
		// sweeping hundreds of tiles through it would add DB load without a
		// GIST index to make it cheap. Skip — legacy sectors just render
		// on-demand as before.
		return
	}

	version := habitatRasterCacheVersion(sectorID)
	dedupeKey := fmt.Sprintf("%d:%s", sectorID, version)
	if _, alreadyRunning := habitatPrewarmInFlight.LoadOrStore(dedupeKey, struct{}{}); alreadyRunning {
		return
	}
	defer habitatPrewarmInFlight.Delete(dedupeKey)

	var ex struct {
		MinLat float64 `gorm:"column:min_lat"`
		MaxLat float64 `gorm:"column:max_lat"`
		MinLng float64 `gorm:"column:min_lng"`
		MaxLng float64 `gorm:"column:max_lng"`
	}
	err := storage.DB.Model(&models.HabitatPlot{}).
		Select(
			"MIN(centroid_lat) AS min_lat",
			"MAX(centroid_lat) AS max_lat",
			"MIN(centroid_lng) AS min_lng",
			"MAX(centroid_lng) AS max_lng",
		).
		Where("sector_id = ?", sectorID).
		Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").
		Scan(&ex).Error
	if err != nil || (ex.MinLat == 0 && ex.MaxLat == 0 && ex.MinLng == 0 && ex.MaxLng == 0) {
		return
	}

	type tileCoord struct{ z, x, y int }
	var tiles []tileCoord
sweep:
	for z := habitatPrewarmMinZoom; z <= habitatPrewarmMaxZoom; z++ {
		x0, y0 := lngLatToTileXY(ex.MinLng, ex.MaxLat, z) // NW corner
		x1, y1 := lngLatToTileXY(ex.MaxLng, ex.MinLat, z) // SE corner
		for x := x0; x <= x1; x++ {
			for y := y0; y <= y1; y++ {
				tiles = append(tiles, tileCoord{z, x, y})
				if len(tiles) >= habitatPrewarmMaxTiles {
					break sweep
				}
			}
		}
	}

	sem := make(chan struct{}, habitatPrewarmConcurrency)
	var wg sync.WaitGroup
	rendered := 0
	for _, t := range tiles {
		cacheKey := habitatTileCacheKey("raster-sector:"+strconv.FormatUint(uint64(sectorID), 10), t.z, t.x, t.y, version)
		if _, hit := habitatTileCacheGet(cacheKey); hit {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		rendered++
		go func(t tileCoord, cacheKey string) {
			defer wg.Done()
			defer func() { <-sem }()
			data, err := habitatSectorRasterTilePostGIS(sectorID, t.z, t.x, t.y)
			if err != nil || data == nil {
				return
			}
			habitatTileCacheSet(cacheKey, data)
		}(t, cacheKey)
	}
	wg.Wait()
	if rendered > 0 {
		log.Printf("habitat raster tile: prewarmed %d/%d tiles for sector=%d version=%s", rendered, len(tiles), sectorID, version)
	}
}

// GET /api/habitat/sectors/{sectorId}/raster-tiles/{z}/{x}/{y}.png
//
// Renders plot boundaries as a pre-rasterized PNG image tile instead of MVT
// vector data or native <Polygon> components — for use as a <UrlTile>
// overlay on top of a native MapView (Apple Maps on iOS, Google Maps on
// Android via react-native-maps), which cannot render our MVT vector layer
// (that's a MapLibre-only capability) or safely handle thousands of native
// Polygon components for a large quartier. This gets native map imagery
// without reintroducing that crash.
//
// Primary path: PostGIS (ST_TileEnvelope + GIST). Falls back to the same
// geom_geojson/centroid-bbox approach the legacy MVT handler uses when
// PostGIS isn't ready on this server — this endpoint must not just 503,
// since it has no other renderer to fall back to on the client side.
func GetHabitatSectorRasterTile(ctx iris.Context) {
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

	// Fire-and-forget: renders the sector's likely-needed tile range in the
	// background so a later pinch zoom mostly hits warm cache instead of
	// racing live DB queries + PNG rendering (see prewarmSectorRasterTiles
	// doc comment). Deduped internally, safe to call on every request.
	go prewarmSectorRasterTiles(uint(sectorID))

	cacheKey := habitatTileCacheKey(
		"raster-sector:"+strconv.FormatUint(sectorID, 10),
		z, x, y,
		habitatRasterCacheVersion(uint(sectorID)),
	)
	if cached, hit := habitatTileCacheGet(cacheKey); hit {
		writeHabitatRasterTile(ctx, cached)
		return
	}

	var data []byte
	if storage.HabitatPostGISReady {
		data, err = habitatSectorRasterTilePostGIS(uint(sectorID), z, x, y)
		if err != nil {
			log.Printf("habitat raster tile: PostGIS query failed for sector=%d z=%d x=%d y=%d, falling back: %v", sectorID, z, x, y, err)
			data = nil
		}
	}
	if data == nil {
		data, err = habitatSectorRasterTileLegacy(uint(sectorID), z, x, y)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "failed to render raster tile"})
			return
		}
	}

	habitatTileCacheSet(cacheKey, data)
	writeHabitatRasterTile(ctx, data)
}

func writeHabitatRasterTile(ctx iris.Context, data []byte) {
	ctx.ContentType("image/png")
	// Tiles are version-keyed (habitatSectorDataVersion) and re-cached
	// server-side the moment the underlying data changes, so a long
	// client-side cache lifetime is safe and cuts the re-fetch-on-every-
	// zoom-level flicker significantly. Not "immutable" — is_for_sale can
	// change without bumping habitat_plots.updated_at (same caveat as the
	// MVT tile cache).
	ctx.Header("Cache-Control", "public, max-age=86400")
	ctx.Write(data)
}

func habitatSectorRasterTilePostGIS(sectorID uint, z, x, y int) ([]byte, error) {
	bufferMeters := float64(rasterTilePadding) * webMercatorMetersPerPixel(z)

	const sql = `
		WITH bounds AS (
			SELECT ST_Expand(ST_TileEnvelope(?, ?, ?), ?) AS tile
		)
		SELECT
			p.id,
			ST_AsGeoJSON(p.geom) AS geojson,
			(fs.habitat_plot_id IS NOT NULL) AS is_for_sale
		FROM habitat_plots p
		CROSS JOIN bounds
		` + habitatForSaleTileJoinSQL + `
		WHERE p.sector_id = ?
		  AND p.geom IS NOT NULL
		  AND p.geom && ST_Transform(bounds.tile, 4326)
		ORDER BY p.id
		LIMIT ?
	`

	type plotGeomRow struct {
		ID        uint   `gorm:"column:id"`
		GeoJSON   string `gorm:"column:geojson"`
		IsForSale bool   `gorm:"column:is_for_sale"`
	}

	var rows []plotGeomRow
	if err := storage.DB.Raw(sql, z, x, y, bufferMeters, sectorID, habitatPostGISTileMaxFeatures).Scan(&rows).Error; err != nil {
		return nil, err
	}

	rings := make([]rasterPlotRing, 0, len(rows))
	for _, row := range rows {
		parsed := ringsFromGeoJSONBytes([]byte(row.GeoJSON))
		if len(parsed) == 0 {
			continue
		}
		rings = append(rings, rasterPlotRing{ring: parsed[0], isForSale: row.IsForSale})
	}
	return renderPlotRingsToTile(rings, z, x, y)
}

// habitatSectorRasterTileLegacy mirrors habitatSectorTileLegacy in
// habitat_tiles.go — same centroid-bbox filter over geom_geojson, no
// PostGIS required. Kept as a true fallback, not the primary path (a
// sequential scan over centroid_lat/lng doesn't scale to nationwide data,
// but is fine for one sector's worth of plots).
func habitatSectorRasterTileLegacy(sectorID uint, z, x, y int) ([]byte, error) {
	tile := maptile.New(uint32(x), uint32(y), maptile.Zoom(z))
	b := tile.Bound()
	margin := float64(rasterTilePadding) * degreesPerPixelApprox(z)
	minLng, minLat := b.Min.Lon()-margin, b.Min.Lat()-margin
	maxLng, maxLat := b.Max.Lon()+margin, b.Max.Lat()+margin

	type plotRow struct {
		ID          uint           `gorm:"column:id"`
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
			"habitat_plots.geom_geojson",
			forSaleExpr,
		).
		Where("habitat_plots.sector_id = ?", sectorID).
		Where("habitat_plots.geom_geojson IS NOT NULL AND habitat_plots.geom_geojson::text NOT IN ('null', '{}')").
		Where("habitat_plots.centroid_lat BETWEEN ? AND ?", minLat, maxLat).
		Where("habitat_plots.centroid_lng BETWEEN ? AND ?", minLng, maxLng).
		Limit(habitatTileMaxFeatures)

	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	rings := make([]rasterPlotRing, 0, len(rows))
	for _, row := range rows {
		ring := primaryRingFromGeoJSON(row.GeomGeoJSON)
		if len(ring) < 3 {
			ring = primaryRingFromPlot(&models.HabitatPlot{GeomGeoJSON: row.GeomGeoJSON})
		}
		if len(ring) < 3 {
			continue
		}
		rings = append(rings, rasterPlotRing{ring: ring, isForSale: row.IsForSale})
	}
	return renderPlotRingsToTile(rings, z, x, y)
}

// pointInRing is a standard ray-casting point-in-polygon test — used by the
// non-PostGIS point lookup fallback below (ST_Contains isn't available).
func pointInRing(lat, lng float64, ring []geoLatLng) bool {
	n := len(ring)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		yi, xi := ring[i].Lat, ring[i].Lng
		yj, xj := ring[j].Lat, ring[j].Lng
		if (yi > lat) != (yj > lat) &&
			lng < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}

// GET /api/habitat/sectors/{sectorId}/plot-at-point?lat=&lng=
//
// Point-in-polygon lookup — the raster overlay is a flat image with no
// built-in tap detection the way the MVT vector layer has (onPress with
// feature hit-testing built into MapLibre). This gives the native-map path
// an equivalent: tap the map, find which plot polygon contains that point.
//
// Primary path: PostGIS ST_Contains (GIST-indexed). Falls back to a Go-side
// ray-casting test over geom_geojson within a small bbox around the tapped
// point when PostGIS isn't ready — same reasoning as the tile fallback
// above, this endpoint has no other way to answer the tap.
func GetHabitatPlotAtPoint(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sector id"})
		return
	}
	lat, err1 := strconv.ParseFloat(ctx.URLParam("lat"), 64)
	lng, err2 := strconv.ParseFloat(ctx.URLParam("lng"), 64)
	if err1 != nil || err2 != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "lat and lng are required"})
		return
	}

	var plotID uint
	if storage.HabitatPostGISReady {
		plotID, err = habitatPlotAtPointPostGIS(uint(sectorID), lat, lng)
		if err != nil {
			log.Printf("habitat plot-at-point: PostGIS query failed for sector=%d, falling back: %v", sectorID, err)
			plotID = 0
		}
	}
	if plotID == 0 {
		plotID, err = habitatPlotAtPointLegacy(uint(sectorID), lat, lng)
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "failed to look up plot"})
			return
		}
	}

	if plotID == 0 {
		ctx.JSON(iris.Map{"success": true, "data": nil})
		return
	}
	ctx.JSON(iris.Map{"success": true, "data": iris.Map{"plot_id": plotID}})
}

func habitatPlotAtPointPostGIS(sectorID uint, lat, lng float64) (uint, error) {
	var plotID uint
	err := storage.DB.Raw(`
		SELECT p.id
		FROM habitat_plots p
		WHERE p.sector_id = ?
		  AND p.geom IS NOT NULL
		  AND ST_Contains(p.geom, ST_SetSRID(ST_MakePoint(?, ?), 4326))
		LIMIT 1
	`, sectorID, lng, lat).Scan(&plotID).Error
	return plotID, err
}

// habitatPlotAtPointLegacy scans plots within ~300m of the tapped point
// (generous for any real cadastre parcel) and tests each ring directly —
// no spatial index, but bounded to a handful of candidates per tap.
func habitatPlotAtPointLegacy(sectorID uint, lat, lng float64) (uint, error) {
	const marginDeg = 0.003

	type plotRow struct {
		ID          uint           `gorm:"column:id"`
		GeomGeoJSON datatypes.JSON `gorm:"column:geom_geojson"`
	}

	var rows []plotRow
	err := storage.DB.Model(&models.HabitatPlot{}).
		Select("id", "geom_geojson").
		Where("sector_id = ?", sectorID).
		Where("geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')").
		Where("centroid_lat BETWEEN ? AND ?", lat-marginDeg, lat+marginDeg).
		Where("centroid_lng BETWEEN ? AND ?", lng-marginDeg, lng+marginDeg).
		Limit(200).
		Find(&rows).Error
	if err != nil {
		return 0, err
	}

	for _, row := range rows {
		ring := primaryRingFromGeoJSON(row.GeomGeoJSON)
		if len(ring) < 3 {
			ring = primaryRingFromPlot(&models.HabitatPlot{GeomGeoJSON: row.GeomGeoJSON})
		}
		if len(ring) >= 3 && pointInRing(lat, lng, ring) {
			return row.ID, nil
		}
	}
	return 0, nil
}
