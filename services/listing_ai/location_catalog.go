package listing_ai

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strings"
)

// LocationEntry is one row in the catalog sent to the matcher + LLM.
type LocationEntry struct {
	CityID       uint
	CityName     string
	CityNameAr   string
	ZoneID       uint
	ZoneName     string
	ZoneNameAr   string
	QuartierID   uint
	QuartierName string
	QuartierAr   string
}

// LoadLocationCatalog returns flat city → zone → quartier rows for active records.
func LoadLocationCatalog() ([]LocationEntry, error) {
	var cities []models.City
	if err := storage.DB.Where("is_active = ?", true).
		Preload("Zones", "is_active = ?", true).
		Find(&cities).Error; err != nil {
		return nil, err
	}

	out := make([]LocationEntry, 0, 256)
	for _, c := range cities {
		for _, z := range c.Zones {
			var quartiers []models.Quartier
			_ = storage.DB.Where("zone_id = ? AND is_active = ?", z.ID, true).Find(&quartiers).Error
			if len(quartiers) == 0 {
				out = append(out, LocationEntry{
					CityID:     c.ID,
					CityName:   c.Name,
					CityNameAr: c.NameAr,
					ZoneID:     z.ID,
					ZoneName:   z.Name,
					ZoneNameAr: z.NameAr,
				})
				continue
			}
			for _, q := range quartiers {
				out = append(out, LocationEntry{
					CityID:       c.ID,
					CityName:     c.Name,
					CityNameAr:   c.NameAr,
					ZoneID:       z.ID,
					ZoneName:     z.Name,
					ZoneNameAr:   z.NameAr,
					QuartierID:   q.ID,
					QuartierName: q.Name,
					QuartierAr:   q.NameAr,
				})
			}
		}
	}
	return out, nil
}

// BuildCatalogSummary compresses the catalog for the LLM prompt (unique cities + sample zones).
func BuildCatalogSummary(entries []LocationEntry, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 80
	}
	seen := make(map[string]bool)
	var b strings.Builder
	n := 0
	for _, e := range entries {
		line := e.CityName
		if e.ZoneName != "" {
			line += " > " + e.ZoneName
		}
		if e.QuartierName != "" {
			line += " > " + e.QuartierName
		}
		key := strings.ToLower(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
		n++
		if n >= maxLines {
			b.WriteString("…\n")
			break
		}
	}
	return b.String()
}
