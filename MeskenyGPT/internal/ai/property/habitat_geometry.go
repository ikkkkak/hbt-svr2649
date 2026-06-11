package property

import (
	"encoding/json"
	"math"
	"strings"

	"apartments-clone-server/models"
)

func plotCornersFromHabitat(p models.HabitatPlot) []LatLng {
	if rings := ringsFromGeoJSON(p.GeomGeoJSON); len(rings) > 0 {
		return rings[0]
	}
	if rings := ringsFromCornersJSON(p.Corners); len(rings) > 0 {
		return rings[0]
	}
	lat, lng := p.CentroidLat, p.CentroidLng
	if lat == nil || lng == nil {
		return nil
	}
	length, width := 0.0, 0.0
	if p.LengthM != nil && p.WidthM != nil && *p.LengthM > 0 && *p.WidthM > 0 {
		length, width = *p.LengthM, *p.WidthM
	} else if len(p.SidesM) >= 2 {
		length, width = p.SidesM[0], p.SidesM[1]
	} else if p.AreaM2 != nil && *p.AreaM2 > 0 {
		side := math.Sqrt(*p.AreaM2)
		length, width = side, side
	}
	if length > 0 && width > 0 {
		return rectangleFromCentroid(*lat, *lng, length, width)
	}
	return nil
}

func ringsFromGeoJSON(raw []byte) [][]LatLng {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return ringsFromGeometry(v)
}

func ringsFromCornersJSON(raw []byte) [][]LatLng {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	ring := coordsToRing(v)
	if len(ring) >= 3 {
		return [][]LatLng{ring}
	}
	return nil
}

func ringsFromGeometry(v any) [][]LatLng {
	m, ok := v.(map[string]any)
	if !ok {
		ring := coordsToRing(v)
		if len(ring) >= 3 {
			return [][]LatLng{ring}
		}
		return nil
	}
	typ, _ := m["type"].(string)
	switch strings.TrimSpace(typ) {
	case "Feature":
		if geom, ok := m["geometry"]; ok {
			return ringsFromGeometry(geom)
		}
	case "FeatureCollection":
		if feats, ok := m["features"].([]any); ok {
			var out [][]LatLng
			for _, f := range feats {
				out = append(out, ringsFromGeometry(f)...)
			}
			return out
		}
	case "Polygon":
		if coords, ok := m["coordinates"].([]any); ok && len(coords) > 0 {
			ring := coordsToRing(coords[0])
			if len(ring) >= 3 {
				return [][]LatLng{ring}
			}
		}
	case "MultiPolygon":
		if polys, ok := m["coordinates"].([]any); ok {
			var out [][]LatLng
			for _, poly := range polys {
				if pArr, ok := poly.([]any); ok && len(pArr) > 0 {
					ring := coordsToRing(pArr[0])
					if len(ring) >= 3 {
						out = append(out, ring)
					}
				}
			}
			return out
		}
	}
	return nil
}

func coordsToRing(v any) []LatLng {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]LatLng, 0, len(arr))
	for _, item := range arr {
		if pair, ok := item.([]any); ok && len(pair) >= 2 {
			a, b := toFloat(pair[0]), toFloat(pair[1])
			if pt := pairToLatLng(a, b); pt != nil {
				out = append(out, *pt)
			}
			continue
		}
		if obj, ok := item.(map[string]any); ok {
			lat := toFloat(obj["lat"])
			if lat == 0 {
				lat = toFloat(obj["latitude"])
			}
			lng := toFloat(obj["lng"])
			if lng == 0 {
				lng = toFloat(obj["longitude"])
			}
			if lat != 0 || lng != 0 {
				out = append(out, LatLng{Lat: lat, Lng: lng})
			}
		}
	}
	if len(out) < 3 {
		return nil
	}
	return closeRing(out)
}

func pairToLatLng(a, b float64) *LatLng {
	if a == 0 && b == 0 {
		return nil
	}
	aIsLat := a >= 10 && a <= 35
	bIsLat := b >= 10 && b <= 35
	aIsLng := a <= -5 && a >= -25
	bIsLng := b <= -5 && b >= -25
	if aIsLat && bIsLng {
		return &LatLng{Lat: a, Lng: b}
	}
	if aIsLng && bIsLat {
		return &LatLng{Lat: b, Lng: a}
	}
	if math.Abs(a) <= 180 && math.Abs(b) <= 90 {
		return &LatLng{Lat: b, Lng: a}
	}
	return nil
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func closeRing(ring []LatLng) []LatLng {
	if len(ring) < 3 {
		return ring
	}
	first, last := ring[0], ring[len(ring)-1]
	if math.Abs(first.Lat-last.Lat) < 1e-9 && math.Abs(first.Lng-last.Lng) < 1e-9 {
		return ring
	}
	return append(ring, first)
}

func rectangleFromCentroid(lat, lng, lengthM, widthM float64) []LatLng {
	if lengthM <= 0 || widthM <= 0 {
		return nil
	}
	dLat := lengthM / 2 / 111_320
	dLng := widthM / 2 / (111_320 * math.Cos(lat*math.Pi/180))
	return closeRing([]LatLng{
		{Lat: lat - dLat, Lng: lng - dLng},
		{Lat: lat - dLat, Lng: lng + dLng},
		{Lat: lat + dLat, Lng: lng + dLng},
		{Lat: lat + dLat, Lng: lng - dLng},
	})
}
