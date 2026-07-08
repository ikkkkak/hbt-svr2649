package routes

import (
	"fmt"
	"math"

	"apartments-clone-server/models"

	"gorm.io/datatypes"
)

const earthRadiusM = 6378137.0

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

func ringSideLengthsM(ring []geoLatLng) []float64 {
	if len(ring) < 2 {
		return nil
	}
	n := len(ring)
	// Drop duplicate closing vertex when present.
	if n > 3 {
		first, last := ring[0], ring[n-1]
		if math.Abs(first.Lat-last.Lat) < 1e-9 && math.Abs(first.Lng-last.Lng) < 1e-9 {
			n--
		}
	}
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		d := haversineMeters(ring[i], ring[j])
		if d > 0.05 {
			out = append(out, d)
		}
	}
	return out
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
	if plot == nil || len(plot.GeomGeoJSON) == 0 {
		return
	}

	ring := primaryRingFromGeoJSON(plot.GeomGeoJSON)
	if len(ring) < 3 {
		return
	}

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

	if plot.DimensionsString == "" && (plot.LengthM == nil || plot.WidthM == nil) {
		sides := ringSideLengthsM(ring)
		if len(sides) >= 2 {
			minSide, maxSide := sides[0], sides[0]
			for _, s := range sides[1:] {
				if s < minSide {
					minSide = s
				}
				if s > maxSide {
					maxSide = s
				}
			}
			length := maxSide
			width := minSide
			if length > 0 && width > 0 {
				plot.LengthM = &length
				plot.WidthM = &width
				plot.DimensionsString = fmt.Sprintf("%.1fm %.1fm", length, width)
			}
		}
	}
}

func fillHabitatPlotsDerivedFields(plots []models.HabitatPlot) {
	for i := range plots {
		fillHabitatPlotDerivedFields(&plots[i])
	}
}
