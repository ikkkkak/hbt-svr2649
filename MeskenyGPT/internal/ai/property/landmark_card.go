package property

import (
	"strings"

	"apartments-clone-server/models"
)

// LatLng is a geographic coordinate for map overlays.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func landmarkLocationLabel(r models.Landmark, quartierName string) string {
	parts := make([]string, 0, 3)
	if q := strings.TrimSpace(quartierName); q != "" {
		parts = append(parts, q)
	}
	if d := strings.TrimSpace(r.District); d != "" {
		parts = append(parts, d)
	}
	if reg := strings.TrimSpace(r.Region); reg != "" {
		parts = append(parts, reg)
	}
	return strings.Join(parts, ", ")
}

func landmarkPlotCorners(r models.Landmark) []LatLng {
	pairs := [][2]*float64{
		{r.Point1Lat, r.Point1Lng},
		{r.Point2Lat, r.Point2Lng},
		{r.Point3Lat, r.Point3Lng},
		{r.Point4Lat, r.Point4Lng},
	}
	out := make([]LatLng, 0, 4)
	for _, p := range pairs {
		if p[0] == nil || p[1] == nil {
			continue
		}
		lat, lng := *p[0], *p[1]
		if lat == 0 && lng == 0 {
			continue
		}
		out = append(out, LatLng{Lat: lat, Lng: lng})
	}
	return out
}

func centroidOfCorners(corners []LatLng) (lat, lng float64, ok bool) {
	if len(corners) == 0 {
		return 0, 0, false
	}
	for _, c := range corners {
		lat += c.Lat
		lng += c.Lng
	}
	n := float64(len(corners))
	return lat / n, lng / n, true
}

func enrichLandmarkCard(card *Card, r models.Landmark, habitat *models.HabitatPlot, quartierName string) {
	card.Source = "landmark"
	card.Type = "sale"
	if r.Area > 0 {
		card.SizeM2 = r.Area
	}
	if label := landmarkLocationLabel(r, quartierName); label != "" {
		card.LocationLabel = label
	}
	if q := strings.TrimSpace(quartierName); q != "" {
		card.QuartierLabel = q
	}
	if pn := strings.TrimSpace(r.PlotNumber); pn != "" {
		card.PlotNumber = pn
	}

	var corners []LatLng
	cadastreLinked := false
	if habitat != nil {
		cadastreLinked = true
		corners = plotCornersFromHabitat(*habitat)
		if card.PlotNumber == "" {
			card.PlotNumber = strings.TrimSpace(habitat.PlotNumber)
		}
		if card.SizeM2 == 0 && habitat.AreaM2 != nil && *habitat.AreaM2 > 0 {
			card.SizeM2 = *habitat.AreaM2
		}
	}
	if len(corners) < 3 {
		corners = landmarkPlotCorners(r)
	}
	if len(corners) >= 3 {
		card.PlotCorners = corners
	}
	if lat, lng, ok := centroidOfCorners(corners); ok {
		card.Lat = lat
		card.Lng = lng
	} else if habitat != nil && habitat.CentroidLat != nil && habitat.CentroidLng != nil {
		card.Lat = *habitat.CentroidLat
		card.Lng = *habitat.CentroidLng
	}
	card.CadastreLinked = cadastreLinked
}
