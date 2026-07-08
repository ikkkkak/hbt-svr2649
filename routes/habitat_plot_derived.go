package routes

import (
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

func fillHabitatPlotDerivedFields(plot *models.HabitatPlot) {
	if plot == nil {
		return
	}

	extractHabitatPlotFromRawProperties(plot)

	if len(plot.GeomGeoJSON) == 0 {
		return
	}

	ring := primaryRingFromGeoJSON(plot.GeomGeoJSON)
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
}

func fillHabitatPlotsDerivedFields(db *gorm.DB, plots []models.HabitatPlot) {
	hydrateHabitatPlotsFromRawProperties(db, plots)
	for i := range plots {
		fillHabitatPlotDerivedFields(&plots[i])
	}
}

const habitatPlotNaturalOrderSQL = `
	CASE WHEN plot_number ~ '^[0-9]+$' THEN plot_number::bigint END ASC NULLS LAST,
	plot_number ASC
`
