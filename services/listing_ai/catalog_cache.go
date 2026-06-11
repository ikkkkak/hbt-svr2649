package listing_ai

import (
	"sync"
	"time"
)

const catalogCacheTTL = 30 * time.Minute

var (
	catalogMu       sync.RWMutex
	catalogCache    []LocationEntry
	catalogLoadedAt time.Time
)

// GetLocationCatalog returns the location catalog, cached in memory to avoid DB hits per job.
func GetLocationCatalog() ([]LocationEntry, error) {
	catalogMu.RLock()
	if len(catalogCache) > 0 && time.Since(catalogLoadedAt) < catalogCacheTTL {
		out := make([]LocationEntry, len(catalogCache))
		copy(out, catalogCache)
		catalogMu.RUnlock()
		return out, nil
	}
	catalogMu.RUnlock()

	catalogMu.Lock()
	defer catalogMu.Unlock()

	if len(catalogCache) > 0 && time.Since(catalogLoadedAt) < catalogCacheTTL {
		out := make([]LocationEntry, len(catalogCache))
		copy(out, catalogCache)
		return out, nil
	}

	entries, err := LoadLocationCatalog()
	if err != nil {
		return nil, err
	}
	catalogCache = entries
	catalogLoadedAt = time.Now()
	out := make([]LocationEntry, len(entries))
	copy(out, entries)
	return out, nil
}

// FilterCatalogForInput narrows the catalog sent to the LLM when the user gave location hints.
func FilterCatalogForInput(entries []LocationEntry, in GenerateInput) []LocationEntry {
	if len(entries) == 0 {
		return entries
	}
	city := in.CityHint
	zone := in.ZoneHint
	quartier := in.QuartierHint
	if city == "" && zone == "" && quartier == "" {
		// Still trim very large catalogs for speed.
		if len(entries) > 400 {
			return entries[:400]
		}
		return entries
	}

	filtered := make([]LocationEntry, 0, 64)
	for _, e := range entries {
		score := 0
		if city != "" {
			score = max(score, nameScore(city, e.CityName, e.CityNameAr))
		}
		if zone != "" {
			score = max(score, nameScore(zone, e.ZoneName, e.ZoneNameAr))
		}
		if quartier != "" {
			score = max(score, nameScore(quartier, e.QuartierName, e.QuartierAr))
		}
		if score >= 55 {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		if len(entries) > 200 {
			return entries[:200]
		}
		return entries
	}
	if len(filtered) > 120 {
		return filtered[:120]
	}
	return filtered
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
