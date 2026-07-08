package routes

import (
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var habitatCentroidBackfillOnce sync.Once

type geoLatLng struct {
	Lat float64
	Lng float64
}

func centroidFromGeoJSON(raw datatypes.JSON) (lat, lng float64, ok bool) {
	if len(raw) == 0 {
		return 0, 0, false
	}
	rings := ringsFromGeoJSONBytes([]byte(raw))
	if len(rings) == 0 {
		return 0, 0, false
	}
	best := rings[0]
	bestArea := ringAreaAbs(best)
	for i := 1; i < len(rings); i++ {
		if a := ringAreaAbs(rings[i]); a > bestArea {
			bestArea = a
			best = rings[i]
		}
	}
	return ringCentroid(best)
}

func ringCentroid(ring []geoLatLng) (lat, lng float64, ok bool) {
	if len(ring) == 0 {
		return 0, 0, false
	}
	var sumLat, sumLng float64
	for _, p := range ring {
		sumLat += p.Lat
		sumLng += p.Lng
	}
	n := float64(len(ring))
	return sumLat / n, sumLng / n, true
}

func ringAreaAbs(ring []geoLatLng) float64 {
	if len(ring) < 3 {
		return 0
	}
	var area float64
	for i := 0; i < len(ring); i++ {
		j := (i + 1) % len(ring)
		area += ring[i].Lng*ring[j].Lat - ring[j].Lng*ring[i].Lat
	}
	return math.Abs(area / 2)
}

func ringsFromGeoJSONBytes(raw []byte) [][]geoLatLng {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return ringsFromGeometry(v)
}

func ringsFromGeometry(v any) [][]geoLatLng {
	m, ok := v.(map[string]any)
	if !ok {
		ring := coordsToRing(v)
		if len(ring) >= 3 {
			return [][]geoLatLng{ring}
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
			var out [][]geoLatLng
			for _, f := range feats {
				out = append(out, ringsFromGeometry(f)...)
			}
			return out
		}
	case "Polygon":
		if coords, ok := m["coordinates"].([]any); ok && len(coords) > 0 {
			ring := coordsToRing(coords[0])
			if len(ring) >= 3 {
				return [][]geoLatLng{ring}
			}
		}
	case "MultiPolygon":
		if polys, ok := m["coordinates"].([]any); ok {
			var out [][]geoLatLng
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

func coordsToRing(v any) []geoLatLng {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]geoLatLng, 0, len(arr))
	for _, item := range arr {
		if pair, ok := item.([]any); ok && len(pair) >= 2 {
			a, b := geoToFloat(pair[0]), geoToFloat(pair[1])
			if pt := pairToLatLng(a, b); pt != nil {
				out = append(out, *pt)
			}
		}
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

func pairToLatLng(a, b float64) *geoLatLng {
	if a == 0 && b == 0 {
		return nil
	}
	aIsLat := a >= 10 && a <= 35
	bIsLat := b >= 10 && b <= 35
	aIsLng := a <= -5 && a >= -25
	bIsLng := b <= -5 && b >= -25
	if aIsLat && bIsLng {
		return &geoLatLng{Lat: a, Lng: b}
	}
	if aIsLng && bIsLat {
		return &geoLatLng{Lat: b, Lng: a}
	}
	if math.Abs(a) <= 180 && math.Abs(b) <= 90 {
		return &geoLatLng{Lat: b, Lng: a}
	}
	return nil
}

func geoToFloat(v any) float64 {
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

type sectorCentroidAcc struct {
	sumLat float64
	sumLng float64
	n      int
}

func accumulateSectorCentroidsFromPlotModels(plots []models.HabitatPlot) map[uint]sectorCentroidAcc {
	out := make(map[uint]sectorCentroidAcc)
	for _, p := range plots {
		if len(p.GeomGeoJSON) == 0 {
			continue
		}
		lat, lng, ok := centroidFromGeoJSON(p.GeomGeoJSON)
		if !ok {
			continue
		}
		acc := out[p.SectorID]
		acc.sumLat += lat
		acc.sumLng += lng
		acc.n++
		out[p.SectorID] = acc
	}
	return out
}

func geoJSONIntersectsBBox(raw datatypes.JSON, minLat, maxLat, minLng, maxLng float64) bool {
	if len(raw) == 0 {
		return false
	}
	if lat, lng, ok := centroidFromGeoJSON(raw); ok {
		if lat >= minLat && lat <= maxLat && lng >= minLng && lng <= maxLng {
			return true
		}
	}
	for _, ring := range ringsFromGeoJSONBytes([]byte(raw)) {
		for _, p := range ring {
			if p.Lat >= minLat && p.Lat <= maxLat && p.Lng >= minLng && p.Lng <= maxLng {
				return true
			}
		}
	}
	return false
}

// sectorIDsInBBoxFromPlotGeom scans plot geometry via GORM (correct column mapping).
func sectorIDsInBBoxFromPlotGeom(
	minLat, maxLat, minLng, maxLng float64,
	planID *uint,
	limit int,
) []uint {
	seen := make(map[uint]struct{})
	var lastID uint

	for len(seen) < limit {
		var batch []models.HabitatPlot
		q := storage.DB.
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(3000)
		if planID != nil {
			q = q.Where("plan_id = ?", *planID)
		}
		if err := q.Find(&batch).Error; err != nil || len(batch) == 0 {
			break
		}

		for _, p := range batch {
			lastID = p.ID
			if !geoJSONIntersectsBBox(p.GeomGeoJSON, minLat, maxLat, minLng, maxLng) {
				continue
			}
			seen[p.SectorID] = struct{}{}
			if len(seen) >= limit {
				break
			}
		}
		if len(batch) < 3000 {
			break
		}
	}

	out := make([]uint, 0, len(seen))
	for id := range seen {
		if id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func backfillSectorCentroidsFromPlotGeom(planID uint, sectors []models.HabitatSector) {
	var lastID uint
	accBySector := make(map[uint]sectorCentroidAcc)

	for {
		var batch []models.HabitatPlot
		if err := storage.DB.Where("plan_id = ? AND id > ?", planID, lastID).
			Order("id ASC").
			Limit(2000).
			Find(&batch).Error; err != nil || len(batch) == 0 {
			break
		}
		for _, p := range batch {
			lastID = p.ID
			if len(p.GeomGeoJSON) == 0 {
				continue
			}
			lat, lng, ok := centroidFromGeoJSON(p.GeomGeoJSON)
			if !ok {
				continue
			}
			acc := accBySector[p.SectorID]
			acc.sumLat += lat
			acc.sumLng += lng
			acc.n++
			accBySector[p.SectorID] = acc
		}
		if len(batch) < 2000 {
			break
		}
	}

	for i := range sectors {
		s := &sectors[i]
		if s.CentroidLat != nil && s.CentroidLng != nil {
			continue
		}
		if acc, ok := accBySector[s.ID]; ok {
			if lat, lng, ok2 := sectorCentroidFromAcc(acc); ok2 {
				s.CentroidLat = &lat
				s.CentroidLng = &lng
			}
		}
	}
}

func applyGeomCentroidsToSectors(sectors []models.HabitatSector, accBySector map[uint]sectorCentroidAcc) {
	for i := range sectors {
		s := &sectors[i]
		if s.CentroidLat != nil && s.CentroidLng != nil {
			continue
		}
		if s.OriginalLat != nil && s.OriginalLng != nil {
			s.CentroidLat = s.OriginalLat
			s.CentroidLng = s.OriginalLng
			continue
		}
		if acc, ok := accBySector[s.ID]; ok {
			if lat, lng, ok2 := sectorCentroidFromAcc(acc); ok2 {
				s.CentroidLat = &lat
				s.CentroidLng = &lng
			}
		}
	}
}

func sectorCentroidFromAcc(acc sectorCentroidAcc) (lat, lng float64, ok bool) {
	if acc.n == 0 {
		return 0, 0, false
	}
	return acc.sumLat / float64(acc.n), acc.sumLng / float64(acc.n), true
}

// EnsureHabitatCentroidsBackfilled persists centroid_lat/lng from geom_geojson (once per process).
func EnsureHabitatCentroidsBackfilled(db *gorm.DB) {
	if db == nil {
		return
	}
	habitatCentroidBackfillOnce.Do(func() {
		var missing int64
		_ = db.Model(&models.HabitatPlot{}).
			Where("centroid_lat IS NULL OR centroid_lng IS NULL").
			Count(&missing)
		if missing == 0 {
			log.Printf("%s Centroid backfill skipped — all plots have lat/lng", habitatGISLogTag)
			return
		}

		log.Printf("%s Centroid backfill starting — %d plots missing lat/lng", habitatGISLogTag, missing)
		result := backfillAllPlotCentroidsFromGeom(db)
		log.Printf(
			"%s Centroid backfill done | updated=%d geom_empty=%d parse_failed=%d batches=%d",
			habitatGISLogTag,
			result.Updated,
			result.GeomEmpty,
			result.ParseFailed,
			result.Batches,
		)
		if result.Updated == 0 && missing > 0 {
			log.Printf(
				"%s WARNING: backfill updated 0 plots but %d still missing — check geom_geojson column / import",
				habitatGISLogTag,
				missing,
			)
			logHabitatDBSnapshot(db, "after failed backfill")
		}
	})
}

type backfillResult struct {
	Updated      int
	GeomEmpty    int
	ParseFailed  int
	Batches      int
}

func backfillAllPlotCentroidsFromGeom(db *gorm.DB) backfillResult {
	var out backfillResult
	var missing int64
	if err := db.Model(&models.HabitatPlot{}).
		Where("centroid_lat IS NULL OR centroid_lng IS NULL").
		Count(&missing).Error; err != nil || missing == 0 {
		return out
	}

	var lastID uint

	for {
		var batch []models.HabitatPlot
		if err := db.Select("id", "sector_id", "geom_geojson", "centroid_lat", "centroid_lng").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(500).
			Find(&batch).Error; err != nil || len(batch) == 0 {
			break
		}
		out.Batches++

		for _, p := range batch {
			lastID = p.ID
			if p.CentroidLat != nil && p.CentroidLng != nil {
				continue
			}
			if len(p.GeomGeoJSON) == 0 {
				out.GeomEmpty++
				continue
			}
			lat, lng, ok := centroidFromGeoJSON(p.GeomGeoJSON)
			if !ok {
				out.ParseFailed++
				continue
			}
			if err := db.Model(&models.HabitatPlot{}).Where("id = ?", p.ID).
				Updates(map[string]interface{}{
					"centroid_lat": lat,
					"centroid_lng": lng,
				}).Error; err != nil {
				continue
			}
			out.Updated++
		}
		if len(batch) < 500 {
			break
		}
	}

	if out.Updated > 0 {
		db.Exec(`
			UPDATE habitat_sectors s SET
				centroid_lat = sub.avg_lat,
				centroid_lng = sub.avg_lng
			FROM (
				SELECT sector_id, AVG(centroid_lat) AS avg_lat, AVG(centroid_lng) AS avg_lng
				FROM habitat_plots
				WHERE centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL
				GROUP BY sector_id
			) sub
			WHERE s.id = sub.sector_id
			  AND (s.centroid_lat IS NULL OR s.centroid_lng IS NULL)
		`)
	}
	return out
}

func sectorIDsInBBoxFromPlotCentroids(
	db *gorm.DB,
	minLat, maxLat, minLng, maxLng float64,
	planID *uint,
	limit int,
) []uint {
	var ids []uint
	q := db.Model(&models.HabitatPlot{}).
		Distinct("sector_id").
		Where("centroid_lat IS NOT NULL AND centroid_lng IS NOT NULL").
		Where("centroid_lat BETWEEN ? AND ?", minLat, maxLat).
		Where("centroid_lng BETWEEN ? AND ?", minLng, maxLng)
	if planID != nil {
		q = q.Where("plan_id = ?", *planID)
	}
	_ = q.Limit(limit).Pluck("sector_id", &ids)
	return ids
}
