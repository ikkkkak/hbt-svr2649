package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"apartments-clone-server/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const habitatGISLogTag = "[HabitatGIS]"

// habitatPlotAPIDebug — temporary verbose payload for prod plot positioning investigation.
type habitatPlotAPIDebug struct {
	Endpoint           string   `json:"endpoint"`
	PlotID             uint     `json:"plot_id"`
	PlotNumber         string   `json:"plot_number"`
	SectorID           uint     `json:"sector_id"`
	PlanID             uint     `json:"plan_id"`
	DbCentroidLat      *float64 `json:"db_centroid_lat,omitempty"`
	DbCentroidLng      *float64 `json:"db_centroid_lng,omitempty"`
	ResponseCentroidLat *float64 `json:"response_centroid_lat,omitempty"`
	ResponseCentroidLng *float64 `json:"response_centroid_lng,omitempty"`
	GeomFromCentroidLat float64 `json:"geom_centroid_lat,omitempty"`
	GeomFromCentroidLng float64 `json:"geom_centroid_lng,omitempty"`
	GeomBytes           int     `json:"geom_bytes"`
	CornersBytes        int     `json:"corners_bytes"`
	RawPropsBytes       int     `json:"raw_properties_bytes"`
	FirstCoordA         float64 `json:"first_coord_a,omitempty"`
	FirstCoordB         float64 `json:"first_coord_b,omitempty"`
	FirstRingPoints     int     `json:"first_ring_points"`
	AreaM2              *float64 `json:"area_m2,omitempty"`
	DimensionsString    string  `json:"dimensions_string,omitempty"`
	SidesM              []float64 `json:"sides_m,omitempty"`
	ELValue             *float64 `json:"el_value,omitempty"`
	ILValue             string  `json:"il_value,omitempty"`
	RESValue            *float64 `json:"res_value,omitempty"`
	RawLat              *float64 `json:"raw_props_lat,omitempty"`
	RawLng              *float64 `json:"raw_props_lng,omitempty"`
	Note                string  `json:"note,omitempty"`
}

func walkFirstCoordPair(v any) (a, b float64, ok bool) {
	switch t := v.(type) {
	case []any:
		if len(t) >= 2 {
			a, b = geoToFloat(t[0]), geoToFloat(t[1])
			if a != 0 || b != 0 {
				return a, b, true
			}
		}
		for _, item := range t {
			if a, b, ok = walkFirstCoordPair(item); ok {
				return a, b, true
			}
		}
	case map[string]any:
		if coords, exists := t["coordinates"]; exists {
			return walkFirstCoordPair(coords)
		}
		if geom, exists := t["geometry"]; exists {
			return walkFirstCoordPair(geom)
		}
	}
	return 0, 0, false
}

func firstCoordPairFromGeoJSON(raw datatypes.JSON) (a, b float64, ok bool) {
	if len(raw) == 0 {
		return 0, 0, false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, 0, false
	}
	return walkFirstCoordPair(v)
}

func rawCentroidFromProps(raw datatypes.JSON) (lat, lng *float64) {
	if len(raw) == 0 {
		return nil, nil
	}
	var props map[string]any
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, nil
	}
	if v, ok := firstFloat(props, "centroid_lat", "lat", "LAT", "latitude"); ok {
		lat = &v
	}
	if v, ok := firstFloat(props, "centroid_lng", "lng", "LNG", "longitude"); ok {
		lng = &v
	}
	return lat, lng
}

func buildHabitatPlotAPIDebug(endpoint string, before, after *models.HabitatPlot) habitatPlotAPIDebug {
	if after == nil {
		return habitatPlotAPIDebug{Endpoint: endpoint, Note: "plot_nil"}
	}
	out := habitatPlotAPIDebug{
		Endpoint:            endpoint,
		PlotID:              after.ID,
		PlotNumber:          after.PlotNumber,
		SectorID:            after.SectorID,
		PlanID:              after.PlanID,
		ResponseCentroidLat: after.CentroidLat,
		ResponseCentroidLng: after.CentroidLng,
		GeomBytes:           len(after.GeomGeoJSON),
		CornersBytes:        len(after.Corners),
		RawPropsBytes:       len(after.RawProperties),
		AreaM2:              after.AreaM2,
		DimensionsString:    after.DimensionsString,
		SidesM:              []float64(after.SidesM),
		ELValue:             after.ELValue,
		ILValue:             after.ILValue,
		RESValue:            after.RESValue,
	}
	if before != nil {
		out.DbCentroidLat = before.CentroidLat
		out.DbCentroidLng = before.CentroidLng
	}
	if lat, lng, ok := centroidFromGeoJSON(after.GeomGeoJSON); ok {
		out.GeomFromCentroidLat = lat
		out.GeomFromCentroidLng = lng
	}
	if a, b, ok := firstCoordPairFromGeoJSON(after.GeomGeoJSON); ok {
		out.FirstCoordA = a
		out.FirstCoordB = b
	}
	if ring := primaryRingFromPlot(after); len(ring) > 0 {
		out.FirstRingPoints = len(ring)
	}
	if rawLat, rawLng := rawCentroidFromProps(after.RawProperties); rawLat != nil || rawLng != nil {
		out.RawLat = rawLat
		out.RawLng = rawLng
	}
	if before != nil && before.CentroidLat != nil && before.CentroidLng != nil &&
		after.CentroidLat != nil && after.CentroidLng != nil {
		dLat := *after.CentroidLat - *before.CentroidLat
		dLng := *after.CentroidLng - *before.CentroidLng
		if dLat*dLat+dLng*dLng > 1e-10 {
			out.Note = fmt.Sprintf("centroid_changed_by_derived dLat=%.8f dLng=%.8f", dLat, dLng)
		}
	}
	return out
}

func logHabitatPlotAPI(endpoint string, before, after *models.HabitatPlot) {
	d := buildHabitatPlotAPIDebug(endpoint, before, after)
	log.Printf(
		"%s PLOT_API %s id=%d plot=%q sector=%d plan=%d db_centroid=(%v,%v) response_centroid=(%v,%v) geom_centroid=(%.8f,%.8f) first_coord=(%.8f,%.8f) ring_pts=%d geom_bytes=%d raw_bytes=%d area=%v dims=%q sides=%v el=%v il=%v res=%v raw_centroid=(%v,%v) note=%q",
		habitatGISLogTag,
		d.Endpoint,
		d.PlotID,
		d.PlotNumber,
		d.SectorID,
		d.PlanID,
		ptrFloat(d.DbCentroidLat),
		ptrFloat(d.DbCentroidLng),
		ptrFloat(d.ResponseCentroidLat),
		ptrFloat(d.ResponseCentroidLng),
		d.GeomFromCentroidLat,
		d.GeomFromCentroidLng,
		d.FirstCoordA,
		d.FirstCoordB,
		d.FirstRingPoints,
		d.GeomBytes,
		d.RawPropsBytes,
		ptrFloat(d.AreaM2),
		d.DimensionsString,
		d.SidesM,
		ptrFloat(d.ELValue),
		d.ILValue,
		ptrFloat(d.RESValue),
		ptrFloat(d.RawLat),
		ptrFloat(d.RawLng),
		d.Note,
	)
}

func logHabitatPlotAPIBatch(endpoint string, plots []models.HabitatPlot, maxLog int) {
	if maxLog <= 0 {
		maxLog = 3
	}
	log.Printf("%s PLOT_API %s batch_count=%d (logging first %d)", habitatGISLogTag, endpoint, len(plots), minInt(len(plots), maxLog))
	for i := range plots {
		if i >= maxLog {
			break
		}
		p := plots[i]
		logHabitatPlotAPI(endpoint, &p, &p)
	}
}

func ptrFloat(v *float64) any {
	if v == nil {
		return "nil"
	}
	return *v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// habitatDBSnapshot — quick DB health for map label debugging.
type habitatDBSnapshot struct {
	TotalPlots            int64 `json:"total_plots"`
	PlotsWithCentroid     int64 `json:"plots_with_centroid"`
	PlotsMissingCentroid  int64 `json:"plots_missing_centroid"`
	SectorsWithCentroid   int64 `json:"sectors_with_centroid"`
	SectorsMissingCentroid int64 `json:"sectors_missing_centroid"`
	SamplePlotID          uint  `json:"sample_plot_id,omitempty"`
	SamplePlotGeomBytes   int   `json:"sample_plot_geom_bytes,omitempty"`
	SamplePlotCentroidOK  bool  `json:"sample_plot_centroid_ok,omitempty"`
	SamplePlotLat         float64 `json:"sample_plot_lat,omitempty"`
	SamplePlotLng         float64 `json:"sample_plot_lng,omitempty"`
}

func habitatDBSnapshotNow(db *gorm.DB) habitatDBSnapshot {
	var snap habitatDBSnapshot
	if db == nil {
		return snap
	}
	_ = db.Model(&models.HabitatPlot{}).Count(&snap.TotalPlots)
	_ = db.Model(&models.HabitatPlot{}).
		Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").
		Count(&snap.PlotsWithCentroid)
	snap.PlotsMissingCentroid = snap.TotalPlots - snap.PlotsWithCentroid
	_ = db.Model(&models.HabitatSector{}).
		Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").
		Count(&snap.SectorsWithCentroid)
	var totalSectors int64
	_ = db.Model(&models.HabitatSector{}).Count(&totalSectors)
	snap.SectorsMissingCentroid = totalSectors - snap.SectorsWithCentroid

	var sample models.HabitatPlot
	if err := db.Order("id ASC").First(&sample).Error; err == nil {
		snap.SamplePlotID = sample.ID
		snap.SamplePlotGeomBytes = len(sample.GeomGeoJSON)
		if lat, lng, ok := centroidFromGeoJSON(sample.GeomGeoJSON); ok {
			snap.SamplePlotCentroidOK = true
			snap.SamplePlotLat = lat
			snap.SamplePlotLng = lng
		}
	}
	return snap
}

func logHabitatDBSnapshot(db *gorm.DB, context string) habitatDBSnapshot {
	snap := habitatDBSnapshotNow(db)
	log.Printf(
		"%s %s | plots total=%d with_centroid=%d missing=%d | sectors with_centroid=%d missing=%d | sample_plot id=%d geom_bytes=%d centroid_ok=%v lat=%.5f lng=%.5f",
		habitatGISLogTag,
		context,
		snap.TotalPlots,
		snap.PlotsWithCentroid,
		snap.PlotsMissingCentroid,
		snap.SectorsWithCentroid,
		snap.SectorsMissingCentroid,
		snap.SamplePlotID,
		snap.SamplePlotGeomBytes,
		snap.SamplePlotCentroidOK,
		snap.SamplePlotLat,
		snap.SamplePlotLng,
	)
	return snap
}

func sectorNamesForLog(sectors []models.HabitatSector, max int) string {
	if len(sectors) == 0 {
		return "(none)"
	}
	if max <= 0 {
		max = 8
	}
	names := make([]string, 0, max)
	for i, s := range sectors {
		if i >= max {
			names = append(names, fmt.Sprintf("+%d more", len(sectors)-max))
			break
		}
		label := strings.TrimSpace(s.NameAr)
		if label == "" {
			label = strings.TrimSpace(s.Name)
		}
		if label == "" {
			label = fmt.Sprintf("#%d", s.ID)
		}
		names = append(names, label)
	}
	return strings.Join(names, ", ")
}
