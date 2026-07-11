package routes

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

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
	habitatTileMinZoom     = 12
	habitatTileMaxZoom     = 20
	habitatTileMaxFeatures = 4096
)

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
	tileTemplate := fmt.Sprintf("%s://%s/api/habitat/sectors/%d/tiles/{z}/{x}/{y}.pbf", scheme, host, sectorID)

	name := sector.Name
	if sector.NameAr != "" {
		name = sector.NameAr
	}

	ctx.Header("Cache-Control", "public, max-age=600")
	ctx.JSON(iris.Map{
		"tilejson":    "3.0.0",
		"name":        fmt.Sprintf("Habitat sector %d — %s", sector.ID, name),
		"description": "Cadastre plot polygons (MVT). Feature id = habitat_plots.id; properties: plot_number, sector_id, plan_id, is_for_sale.",
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
					"id":          "Number",
					"plot_number": "String",
					"sector_id":   "Number",
					"plan_id":     "Number",
					"is_for_sale": "Boolean",
				},
			},
		},
	})
}

// GET /api/habitat/sectors/{sectorId}/tiles/{z}/{x}/{y}.pbf
// Mapbox Vector Tile for plots in one sector (GPU-ready; no GeoJSON bulk download).
func GetHabitatSectorVectorTile(ctx iris.Context) {
	sectorID, err := strconv.ParseUint(ctx.Params().Get("sectorId"), 10, 32)
	if err != nil || sectorID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid sector id"})
		return
	}

	z, err1 := strconv.Atoi(ctx.Params().Get("z"))
	x, err2 := strconv.Atoi(ctx.Params().Get("x"))
	y, err3 := strconv.Atoi(ctx.Params().Get("y"))
	if err1 != nil || err2 != nil || err3 != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid tile coordinates"})
		return
	}
	if z < habitatTileMinZoom || z > habitatTileMaxZoom {
		ctx.StatusCode(http.StatusNoContent)
		return
	}
	maxTile := int(math.Pow(2, float64(z))) - 1
	if x < 0 || y < 0 || x > maxTile || y > maxTile {
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	tile := maptile.New(uint32(x), uint32(y), maptile.Zoom(z))
	tileBound := tile.Bound()
	minLng, minLat := tileBound.Min.Lon(), tileBound.Min.Lat()
	maxLng, maxLat := tileBound.Max.Lon(), tileBound.Max.Lat()

	type plotRow struct {
		ID          uint           `gorm:"column:id"`
		PlotNumber  string         `gorm:"column:plot_number"`
		SectorID    uint           `gorm:"column:sector_id"`
		PlanID      uint           `gorm:"column:plan_id"`
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
			"habitat_plots.geom_geojson",
			forSaleExpr+" AS is_for_sale",
		).
		Where("habitat_plots.sector_id = ?", uint(sectorID)).
		Where("habitat_plots.geom_geojson IS NOT NULL AND habitat_plots.geom_geojson::text NOT IN ('null', '{}')").
		Where("habitat_plots.centroid_lat BETWEEN ? AND ?", minLat, maxLat).
		Where("habitat_plots.centroid_lng BETWEEN ? AND ?", minLng, maxLng).
		Limit(habitatTileMaxFeatures)

	if err := q.Find(&rows).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to load tile plots"})
		return
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

		f := geojson.NewFeature(orb.Polygon{orbRing})
		f.ID = row.ID
		f.Properties = geojson.Properties{
			"id":          row.ID,
			"plot_number": row.PlotNumber,
			"sector_id":   row.SectorID,
			"plan_id":     row.PlanID,
			"is_for_sale": row.IsForSale,
		}
		features = append(features, f)
	}

	if len(features) == 0 {
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	collections := map[string]*geojson.FeatureCollection{
		"plots": {Features: features},
	}
	layers := mvt.NewLayers(collections)
	layers.ProjectToTile(tile)
	layers.Clip(mvt.MapboxGLDefaultExtentBound)

	data, err := mvt.Marshal(layers)
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to encode vector tile"})
		return
	}

	ctx.ContentType("application/vnd.mapbox-vector-tile")
	ctx.Header("Cache-Control", "public, max-age=300")
	ctx.Write(data)
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
	err := storage.DB.Model(&models.HabitatPlot{}).
		Select(
			"MIN(centroid_lat) AS min_lat",
			"MAX(centroid_lat) AS max_lat",
			"MIN(centroid_lng) AS min_lng",
			"MAX(centroid_lng) AS max_lng",
			"COUNT(*) AS plot_count",
		).
		Where("sector_id = ?", sectorID).
		Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").
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
