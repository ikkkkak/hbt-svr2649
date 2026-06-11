package listing_ai

import (
	"sort"
	"strings"
	"unicode"
)

func normalizeLoc(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	for _, noise := range []string{
		"quartier", "quartiers", "حي", "حى", "zone", "arrondissement",
		"secteur", "region", "ville", "city", "منطقة", "مقاطعة",
	} {
		s = strings.ReplaceAll(s, noise, " ")
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), ""))
}

// nameScore returns 0-100 how well hint matches a catalog label (EN or AR).
func nameScore(hint, name, nameAr string) int {
	h := normalizeLoc(hint)
	if h == "" {
		return 0
	}
	n := normalizeLoc(name)
	na := normalizeLoc(nameAr)
	best := 0
	for _, cand := range []string{n, na} {
		if cand == "" {
			continue
		}
		if h == cand {
			if best < 100 {
				best = 100
			}
			continue
		}
		if strings.Contains(cand, h) || strings.Contains(h, cand) {
			if len(h) >= 4 && best < 85 {
				best = 85
			} else if best < 70 {
				best = 70
			}
			continue
		}
		// Prefix match (e.g. tevragh vs tevraghzeina)
		if len(h) >= 4 && (strings.HasPrefix(cand, h) || strings.HasPrefix(h, cand)) {
			if best < 75 {
				best = 75
			}
		}
	}
	return best
}

// findQuartierInText finds the longest catalog quartier name mentioned in free text.
func findQuartierInText(entries []LocationEntry, text string) string {
	body := normalizeLoc(text)
	if body == "" {
		return ""
	}
	type cand struct {
		name string
		len  int
	}
	var hits []cand
	seen := map[string]bool{}
	for _, e := range entries {
		if e.QuartierID == 0 || e.QuartierName == "" {
			continue
		}
		for _, label := range []string{e.QuartierName, e.QuartierAr} {
			n := normalizeLoc(label)
			if len(n) < 3 || seen[n] {
				continue
			}
			if strings.Contains(body, n) {
				seen[n] = true
				hits = append(hits, cand{name: label, len: len(n)})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].len > hits[j].len })
	if len(hits) > 0 {
		return hits[0].name
	}
	return ""
}

// findZoneInText finds zone name in free text when quartier not found.
func findZoneInText(entries []LocationEntry, text string) string {
	body := normalizeLoc(text)
	if body == "" {
		return ""
	}
	type cand struct {
		name string
		len  int
	}
	var hits []cand
	seen := map[string]bool{}
	for _, e := range entries {
		if e.ZoneID == 0 || e.ZoneName == "" {
			continue
		}
		key := normalizeLoc(e.ZoneName)
		if seen[key] {
			continue
		}
		for _, label := range []string{e.ZoneName, e.ZoneNameAr} {
			n := normalizeLoc(label)
			if len(n) < 3 {
				continue
			}
			if strings.Contains(body, n) {
				seen[key] = true
				hits = append(hits, cand{name: e.ZoneName, len: len(n)})
				break
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].len > hits[j].len })
	if len(hits) > 0 {
		return hits[0].name
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// MatchLocation picks the best catalog row using hints + optional description text.
func MatchLocation(entries []LocationEntry, cityHint, zoneHint, quartierHint, detailsText string) (LocationEntry, string) {
	cityHint = firstNonEmpty(cityHint)
	zoneHint = firstNonEmpty(zoneHint)
	quartierHint = firstNonEmpty(quartierHint)

	// Enrich from description when user wrote location inside the paragraph.
	if quartierHint == "" {
		quartierHint = findQuartierInText(entries, detailsText)
	}
	if zoneHint == "" {
		zoneHint = findZoneInText(entries, detailsText)
	}

	if cityHint == "" && zoneHint == "" && quartierHint == "" {
		return LocationEntry{}, "low"
	}

	// Quartier-first: when we know a quartier, only consider rows that match it.
	if quartierHint != "" {
		var qRows []LocationEntry
		for _, e := range entries {
			if e.QuartierID == 0 {
				continue
			}
			if nameScore(quartierHint, e.QuartierName, e.QuartierAr) >= 60 {
				qRows = append(qRows, e)
			}
		}
		if len(qRows) > 0 {
			best, conf := scoreAmongEntries(qRows, cityHint, zoneHint, quartierHint)
			if best.QuartierID > 0 {
				return best, conf
			}
		}
	}

	// Zone-first when quartier unknown but zone known.
	if zoneHint != "" {
		var zRows []LocationEntry
		zoneSeen := map[uint]bool{}
		for _, e := range entries {
			if e.ZoneID == 0 || zoneSeen[e.ZoneID] {
				continue
			}
			if nameScore(zoneHint, e.ZoneName, e.ZoneNameAr) >= 60 {
				zoneSeen[e.ZoneID] = true
				zRows = append(zRows, e)
			}
		}
		if len(zRows) > 0 {
			best, conf := scoreAmongEntries(zRows, cityHint, zoneHint, quartierHint)
			if best.ZoneID > 0 {
				return best, conf
			}
		}
	}

	return scoreAmongEntries(entries, cityHint, zoneHint, quartierHint)
}

func scoreAmongEntries(entries []LocationEntry, cityHint, zoneHint, quartierHint string) (LocationEntry, string) {
	best := LocationEntry{}
	bestScore := -1

	for _, e := range entries {
		score := 0
		cs := nameScore(cityHint, e.CityName, e.CityNameAr)
		zs := nameScore(zoneHint, e.ZoneName, e.ZoneNameAr)
		qs := 0
		if e.QuartierID > 0 {
			qs = nameScore(quartierHint, e.QuartierName, e.QuartierAr)
		}

		if cityHint != "" {
			score += cs * 35 / 100
		} else {
			score += 5
		}
		if zoneHint != "" {
			score += zs * 40 / 100
		}
		if quartierHint != "" && e.QuartierID > 0 {
			score += qs * 50 / 100
			if qs < 50 {
				score -= 40 // penalize wrong quartier when user specified one
			}
		} else if quartierHint != "" && e.QuartierID == 0 {
			score -= 20
		}

		if score > bestScore {
			bestScore = score
			best = e
		}
	}

	conf := "low"
	if bestScore >= 70 {
		conf = "high"
	} else if bestScore >= 45 {
		conf = "medium"
	}
	return best, conf
}

// MatchLocationFromInput uses form hints, description, and optional LLM-extracted names.
func MatchLocationFromInput(entries []LocationEntry, in GenerateInput, llmCity, llmZone, llmQuartier string) (LocationEntry, string) {
	city := firstNonEmpty(in.CityHint, llmCity)
	zone := firstNonEmpty(in.ZoneHint, llmZone)
	quartier := firstNonEmpty(in.QuartierHint, llmQuartier)

	loc, conf := MatchLocation(entries, city, zone, quartier, in.Details)

	// If quartier was explicit but match missed ID, retry description-only extraction.
	if in.QuartierHint != "" && loc.QuartierID == 0 {
		loc2, conf2 := MatchLocation(entries, city, zone, in.QuartierHint, in.Details)
		if loc2.QuartierID > 0 && conf2 != "low" {
			return loc2, conf2
		}
	}
	return loc, conf
}
