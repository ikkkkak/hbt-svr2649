package property

import (
	"strings"

	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// resolveQuartierID finds a catalog quartier from city/zone/quartier hints.
func resolveQuartierID(db *gorm.DB, city, zone, quartier string) uint {
	if db == nil {
		return 0
	}
	qName := strings.TrimSpace(quartier)
	if qName == "" {
		return 0
	}
	qLower := strings.ToLower(qName)

	type row struct {
		ID uint
	}
	var match row

	q := db.Model(&models.Quartier{}).
		Select("quartiers.id").
		Joins("JOIN zones ON zones.id = quartiers.zone_id").
		Joins("JOIN cities ON cities.id = zones.city_id").
		Where("quartiers.is_active = ?", true)

	if cityName := strings.TrimSpace(city); cityName != "" {
		cLike := "%" + strings.ToLower(cityName) + "%"
		q = q.Where("(LOWER(cities.name) LIKE ? OR LOWER(cities.name_ar) LIKE ?)", cLike, cLike)
	}
	if zoneName := strings.TrimSpace(zone); zoneName != "" {
		zLike := "%" + strings.ToLower(zoneName) + "%"
		q = q.Where("(LOWER(zones.name) LIKE ? OR LOWER(zones.name_ar) LIKE ?)", zLike, zLike)
	}

	qLike := "%" + qLower + "%"
	err := q.Where(
		"(LOWER(quartiers.name) LIKE ? OR LOWER(quartiers.name_ar) LIKE ? OR quartiers.name = ? OR quartiers.name_ar = ?)",
		qLike, qLike, qName, qName,
	).Limit(1).Scan(&match).Error
	if err != nil || match.ID == 0 {
		return 0
	}
	return match.ID
}
