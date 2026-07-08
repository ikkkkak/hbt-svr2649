package routes

import (
	"fmt"
	"log"
	"strings"

	"apartments-clone-server/models"

	"gorm.io/gorm"
)

const habitatGISLogTag = "[HabitatGIS]"

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
