package property

import (
	"strings"

	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// EnrichFiltersFromCatalog resolves structured zone_id values from parsed zone text.
func EnrichFiltersFromCatalog(db *gorm.DB, f *Filters) {
	if db == nil || f == nil {
		return
	}
	f.ZoneIDs = resolveZoneIDs(db, f.City, f.Zone)
}

func resolveZoneIDs(db *gorm.DB, city, zonePipe string) []uint {
	zonePipe = strings.TrimSpace(zonePipe)
	if db == nil || zonePipe == "" {
		return nil
	}
	patterns := strings.Split(zonePipe, "|")
	seen := map[uint]bool{}
	var ids []uint

	for _, raw := range patterns {
		pat := strings.TrimSpace(raw)
		if pat == "" {
			continue
		}
		for _, id := range lookupZoneIDs(db, city, pat) {
			if id == 0 || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func lookupZoneIDs(db *gorm.DB, city, pattern string) []uint {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}

	type row struct {
		ID uint
	}
	var rows []row

	q := db.Model(&models.Zone{}).
		Select("zones.id").
		Joins("JOIN cities ON cities.id = zones.city_id").
		Where("zones.is_active = ? AND cities.is_active = ?", true, true)

	if cityName := strings.TrimSpace(city); cityName != "" {
		cLike := "%" + strings.ToLower(cityName) + "%"
		q = q.Where("(LOWER(cities.name) LIKE ? OR LOWER(cities.name_ar) LIKE ?)", cLike, cLike)
	}

	like := "%" + strings.ToLower(pattern) + "%"
	likeRaw := "%" + pattern + "%"
	compact := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(pattern), "-", ""), " ", "")

	q = q.Where(`(
		LOWER(zones.name) LIKE ? OR LOWER(zones.name_ar) LIKE ?
		OR zones.name LIKE ? OR zones.name_ar LIKE ?
		OR REPLACE(REPLACE(LOWER(zones.name), '-', ''), ' ', '') LIKE ?
		OR REPLACE(REPLACE(LOWER(zones.name_ar), '-', ''), ' ', '') LIKE ?
	)`, like, like, likeRaw, likeRaw, "%"+compact+"%", "%"+compact+"%")

	if err := q.Limit(8).Scan(&rows).Error; err != nil {
		return nil
	}
	out := make([]uint, 0, len(rows))
	for _, r := range rows {
		if r.ID > 0 {
			out = append(out, r.ID)
		}
	}
	return out
}
