package routes

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"apartments-clone-server/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const earthRadiusM = 6378137.0

var dimensionNumberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

func haversineMeters(a, b geoLatLng) float64 {
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(h)))
}

func latLngToLocalMeters(p, origin geoLatLng) (x, y float64) {
	latRad := (p.Lat - origin.Lat) * math.Pi / 180
	lngRad := (p.Lng - origin.Lng) * math.Pi / 180
	y = latRad * earthRadiusM
	x = lngRad * earthRadiusM * math.Cos(origin.Lat*math.Pi/180)
	return x, y
}

func ringAreaM2(ring []geoLatLng) float64 {
	if len(ring) < 3 {
		return 0
	}
	origin := ring[0]
	var area float64
	for i := 0; i < len(ring); i++ {
		j := (i + 1) % len(ring)
		xi, yi := latLngToLocalMeters(ring[i], origin)
		xj, yj := latLngToLocalMeters(ring[j], origin)
		area += xi*yj - xj*yi
	}
	return math.Abs(area / 2)
}

func formatSideLengths(sides []float64) string {
	if len(sides) == 0 {
		return ""
	}
	parts := make([]string, len(sides))
	for i, s := range sides {
		parts[i] = fmt.Sprintf("%.1fm", s)
	}
	return strings.Join(parts, " ")
}

func dimensionPartCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return len(dimensionNumberRe.FindAllString(s, -1))
}

func primaryRingFromGeoJSON(raw datatypes.JSON) []geoLatLng {
	rings := ringsFromGeoJSONBytes([]byte(raw))
	if len(rings) == 0 {
		return nil
	}
	best := rings[0]
	bestArea := ringAreaAbs(best)
	for i := 1; i < len(rings); i++ {
		if a := ringAreaAbs(rings[i]); a > bestArea {
			bestArea = a
			best = rings[i]
		}
	}
	return best
}

func plotAreaMissing(area *float64, rounded *int) bool {
	if area != nil && *area > 0 {
		return false
	}
	if rounded != nil && *rounded > 0 {
		return false
	}
	return true
}

func ringNearlyEqual(a, b geoLatLng, eps float64) bool {
	return math.Abs(a.Lat-b.Lat) <= eps && math.Abs(a.Lng-b.Lng) <= eps
}

func sideLengthsFromRing(ring []geoLatLng) []float64 {
	if len(ring) < 2 {
		return nil
	}
	pts := ring
	if len(ring) > 3 && ringNearlyEqual(ring[0], ring[len(ring)-1], 1e-9) {
		pts = ring[:len(ring)-1]
	}
	if len(pts) < 2 {
		return nil
	}
	sides := make([]float64, 0, len(pts))
	for i := range pts {
		j := (i + 1) % len(pts)
		d := haversineMeters(pts[i], pts[j])
		if d > 0.01 {
			sides = append(sides, d)
		}
	}
	return sides
}

func primaryRingFromPlot(plot *models.HabitatPlot) []geoLatLng {
	if plot == nil {
		return nil
	}
	if len(plot.GeomGeoJSON) > 0 {
		if ring := primaryRingFromGeoJSON(plot.GeomGeoJSON); len(ring) >= 3 {
			return ring
		}
	}
	if len(plot.Corners) > 0 {
		if ring := primaryRingFromCornersJSON(plot.Corners); len(ring) >= 3 {
			return ring
		}
	}
	return nil
}

func fillSidesFromRing(plot *models.HabitatPlot, ring []geoLatLng) {
	if plot == nil || len(ring) < 3 {
		return
	}
	if len(plot.SidesM) > 0 && dimensionPartCount(plot.DimensionsString) >= 3 {
		return
	}
	sides := sideLengthsFromRing(ring)
	if len(sides) < 3 {
		return
	}
	plot.SidesM = models.Float64Slice(sides)
	if plot.DimensionsString == "" || dimensionPartCount(plot.DimensionsString) < 3 {
		plot.DimensionsString = formatSideLengths(sides)
	}
	if plot.LengthM == nil || plot.WidthM == nil {
		minSide, maxSide := sides[0], sides[0]
		for _, s := range sides[1:] {
			if s < minSide {
				minSide = s
			}
			if s > maxSide {
				maxSide = s
			}
		}
		if plot.LengthM == nil && maxSide > 0 {
			plot.LengthM = &maxSide
		}
		if plot.WidthM == nil && minSide > 0 {
			plot.WidthM = &minSide
		}
	}
	if plot.PerimeterM == nil {
		var perimeter float64
		for _, s := range sides {
			perimeter += s
		}
		if perimeter > 0 {
			plot.PerimeterM = &perimeter
		}
	}
}

func fillHabitatPlotDerivedFields(plot *models.HabitatPlot) {
	if plot == nil {
		return
	}

	extractHabitatPlotFromRawProperties(plot)
	extractHabitatPlotFromCorners(plot)

	ring := primaryRingFromPlot(plot)
	if len(ring) < 3 {
		return
	}

	// Area from geometry only when cadastre/raw data has no area.
	if plotAreaMissing(plot.AreaM2, plot.AreaRounded) {
		area := ringAreaM2(ring)
		if area > 0 {
			plot.AreaM2 = &area
			rounded := int(math.Round(area))
			if rounded > 0 {
				plot.AreaRounded = &rounded
			}
		}
	}

	fillSidesFromRing(plot, ring)
	syncCentroidFromGeometry(plot, ring)
}

func centroidValidForMauritania(lat, lng *float64) bool {
	if lat == nil || lng == nil {
		return false
	}
	nlat, nlng := normalizeMauritaniaLatLng(*lat, *lng)
	return nlat >= 10 && nlat <= 35 && nlng <= -5 && nlng >= -25
}

func syncCentroidFromGeometry(plot *models.HabitatPlot, ring []geoLatLng) {
	if plot == nil || len(ring) < 3 {
		return
	}
	// Prefer cadastre DB centroid when already valid — imported columns beat derived geometry.
	if centroidValidForMauritania(plot.CentroidLat, plot.CentroidLng) {
		lat, lng := normalizeMauritaniaLatLng(*plot.CentroidLat, *plot.CentroidLng)
		plot.CentroidLat = &lat
		plot.CentroidLng = &lng
		return
	}
	lat, lng, ok := ringCentroid(ring)
	if !ok {
		return
	}
	lat, lng = normalizeMauritaniaLatLng(lat, lng)
	plot.CentroidLat = &lat
	plot.CentroidLng = &lng
}

// normalizeMauritaniaLatLng fixes common lat/lng column swaps (Nouakchott ~18°N, ~−16°W).
func normalizeMauritaniaLatLng(lat, lng float64) (float64, float64) {
	latOk := lat >= 10 && lat <= 35
	lngOk := lng <= -5 && lng >= -25
	if latOk && lngOk {
		return lat, lng
	}
	if lng >= 10 && lng <= 35 && lat <= -5 && lat >= -25 {
		return lng, lat
	}
	return lat, lng
}

func fillHabitatPlotsDerivedFields(db *gorm.DB, plots []models.HabitatPlot) {
	hydrateHabitatPlotsFromRawProperties(db, plots)
	for i := range plots {
		fillHabitatPlotDerivedFields(&plots[i])
	}
}

// ensureHabitatPlotGeometryPayload guarantees every plot in a geometry API
// response carries a drawable polygon. Plots whose geom_geojson is NULL or
// empty in the DB get one synthesized into the RESPONSE (never written back)
// from the same fallback chain the raster tiles already use: corners first,
// then a rectangle at the true centroid with the plot's real length/width.
// Without this, a quartier of 1,840 plots renders only the ~1,750 that have
// stored GeoJSON — the rest silently vanish from the client map.
func ensureHabitatPlotGeometryPayload(plots []models.HabitatPlot) {
	for i := range plots {
		p := &plots[i]
		if geomJSONLooksUsable(p.GeomGeoJSON) {
			continue
		}
		ring := habitatPlotMapRing(p)
		if len(ring) < 3 {
			continue
		}
		coords := make([][]float64, 0, len(ring)+1)
		for _, c := range ring {
			coords = append(coords, []float64{c.Lng, c.Lat})
		}
		first := coords[0]
		last := coords[len(coords)-1]
		if first[0] != last[0] || first[1] != last[1] {
			coords = append(coords, []float64{first[0], first[1]})
		}
		payload, err := json.Marshal(map[string]any{
			"type":        "Polygon",
			"coordinates": [][][]float64{coords},
		})
		if err != nil {
			continue
		}
		p.GeomGeoJSON = datatypes.JSON(payload)
	}
}

// geomJSONLooksUsable filters out NULL/empty/degenerate jsonb values that
// deserialize fine but render nothing.
func geomJSONLooksUsable(raw datatypes.JSON) bool {
	if len(raw) == 0 {
		return false
	}
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "{}" && s != "[]"
}

// habitatPlotMapRing returns the best polygon ring for map overlays (geom → corners → rectangle).
func habitatPlotMapRing(plot *models.HabitatPlot) []geoLatLng {
	if plot == nil {
		return nil
	}
	if ring := primaryRingFromGeoJSON(plot.GeomGeoJSON); len(ring) >= 3 {
		return ring
	}
	if ring := primaryRingFromCornersJSON(plot.Corners); len(ring) >= 3 {
		return ring
	}
	lat, lng := plot.CentroidLat, plot.CentroidLng
	if lat == nil || lng == nil {
		return nil
	}

	lengthM, widthM := 0.0, 0.0
	if plot.LengthM != nil && plot.WidthM != nil && *plot.LengthM > 0 && *plot.WidthM > 0 {
		lengthM, widthM = *plot.LengthM, *plot.WidthM
	} else if len(plot.SidesM) >= 2 && plot.SidesM[0] > 0 && plot.SidesM[1] > 0 {
		lengthM, widthM = plot.SidesM[0], plot.SidesM[1]
	} else if plot.AreaM2 != nil && *plot.AreaM2 > 0 {
		side := math.Sqrt(*plot.AreaM2)
		lengthM, widthM = side, side
	} else if plot.AreaRounded != nil && *plot.AreaRounded > 0 {
		side := math.Sqrt(float64(*plot.AreaRounded))
		lengthM, widthM = side, side
	}
	if lengthM <= 0 || widthM <= 0 {
		return nil
	}
	return rectangleRingFromCentroid(*lat, *lng, lengthM, widthM)
}

func rectangleRingFromCentroid(lat, lng, lengthM, widthM float64) []geoLatLng {
	if lengthM <= 0 || widthM <= 0 {
		return nil
	}
	dLat := lengthM / 2 / 111_320
	cosLat := math.Cos(lat * math.Pi / 180)
	if math.Abs(cosLat) < 1e-6 {
		return nil
	}
	dLng := widthM / 2 / (111_320 * cosLat)
	return []geoLatLng{
		{Lat: lat - dLat, Lng: lng - dLng},
		{Lat: lat - dLat, Lng: lng + dLng},
		{Lat: lat + dLat, Lng: lng + dLng},
		{Lat: lat + dLat, Lng: lng - dLng},
		{Lat: lat - dLat, Lng: lng - dLng},
	}
}

const habitatPlotNaturalOrderSQL = `
	CASE WHEN plot_number ~ '^[0-9]+$' THEN plot_number::bigint END ASC NULLS LAST,
	plot_number ASC
`
