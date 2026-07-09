package routes

import (
	"encoding/json"
	"strconv"

	"apartments-clone-server/models"

	"gorm.io/datatypes"
)

func extractHabitatPlotFromCorners(plot *models.HabitatPlot) bool {
	if plot == nil || len(plot.Corners) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(plot.Corners, &v); err != nil {
		return false
	}
	props, ok := v.(map[string]any)
	if !ok || len(props) == 0 {
		return false
	}
	return applyHabitatPropsToPlot(plot, props)
}

func primaryRingFromCornersJSON(raw datatypes.JSON) []geoLatLng {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}

	if arr, ok := v.([]any); ok {
		ring := coordsToRing(arr)
		if len(ring) >= 3 {
			return ring
		}
	}

	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}

	if coords, ok := m["coordinates"]; ok {
		if rings := ringsFromGeometry(map[string]any{"type": "Polygon", "coordinates": coords}); len(rings) > 0 {
			return rings[0]
		}
	}

	points := make([]geoLatLng, 0, 12)
	for i := 1; i <= 12; i++ {
		idx := strconv.Itoa(i)
		lat := firstFloatFromAny(m["point"+idx+"_lat"], m["p"+idx+"_lat"])
		lng := firstFloatFromAny(m["point"+idx+"_lng"], m["p"+idx+"_lng"])
		if lat == nil || lng == nil {
			continue
		}
		points = append(points, geoLatLng{Lat: *lat, Lng: *lng})
	}
	if len(points) >= 3 {
		return points
	}
	return nil
}

func firstFloatFromAny(values ...any) *float64 {
	for _, v := range values {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			f := t
			return &f
		case float32:
			f := float64(t)
			return &f
		case int:
			f := float64(t)
			return &f
		case int64:
			f := float64(t)
			return &f
		case json.Number:
			if f, err := t.Float64(); err == nil {
				return &f
			}
		case string:
			if f, ok := parseFloatString(t); ok {
				return &f
			}
		}
	}
	return nil
}

func parseFloatString(s string) (float64, bool) {
	f, ok := firstFloat(map[string]any{"v": s}, "v")
	return f, ok
}
