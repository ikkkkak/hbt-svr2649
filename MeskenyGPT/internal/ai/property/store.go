package property

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// Store abstracts access to the properties inventory.
type Store interface {
	Find(ctx context.Context, f Filters) ([]Property, error)
	FindLandmarks(ctx context.Context, f Filters) ([]Property, error)
}

type store struct {
	db *gorm.DB
}

// NewStore creates a new property store backed by the given DB handle.
// db is expected to be a *gorm.DB from apartmentscloneserver/storage.
func NewStore(db any) Store {
	gdb, _ := db.(*gorm.DB)
	return &store{db: gdb}
}

// Find queries the real PropertySale table to retrieve matching properties.
func (s *store) Find(ctx context.Context, f Filters) ([]Property, error) {
	if s.db == nil {
		return []Property{}, nil
	}
	relaxTypeForBroadQuery := shouldRelaxTypeForBroadQuery(f.Query)

	// Rent path must query the rent inventory table (properties), not property_sales.
	if strings.EqualFold(strings.TrimSpace(f.Purpose), "rent") {
		q := s.db.WithContext(ctx).Model(&models.Property{}).
			Where("COALESCE(is_active, ?) = ?", true, true).
			Where("LOWER(status) IN (?)", []string{"approved", "live", "published"})

		if f.City != "" {
			c := strings.ToLower(strings.TrimSpace(f.City))
			like := "%" + c + "%"
			q = q.Where("(LOWER(city) = ? OR LOWER(city) LIKE ? OR LOWER(address_line1) LIKE ? OR LOWER(address_line2) LIKE ?)",
				c, like, like, like)
		}

		zone := strings.TrimSpace(f.Zone)
		if zone != "" {
			patterns := strings.Split(zone, "|")
			var ors []string
			var args []interface{}
			for _, raw := range patterns {
				pat := strings.TrimSpace(raw)
				if pat == "" {
					continue
				}
				z := strings.ToLower(pat)
				likeLatin := "%" + z + "%"
				likeRaw := "%" + pat + "%"
				ors = append(ors, "(LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(address_line1) LIKE ? OR LOWER(address_line2) LIKE ? OR title LIKE ? OR description LIKE ?)")
				args = append(args, likeLatin, likeLatin, likeLatin, likeLatin, likeRaw, likeRaw)
			}
			if len(ors) > 0 {
				q = q.Where(strings.Join(ors, " OR "), args...)
			}
		}

		if f.BudgetMin > 0 && f.BudgetMax > f.BudgetMin {
			q = q.Where("nightly_price BETWEEN ? AND ?", f.BudgetMin, f.BudgetMax)
		}

		// For rentals, parsed type can be inferred (often "villa"/house-like) even when
		// user did not explicitly ask for a strict type. Apply type filtering only when
		// the raw query clearly contains a type hint.
		if f.Type != "" && !relaxTypeForBroadQuery && shouldApplyRentTypeFilter(f.Query, f.Type) {
			t := strings.ToLower(strings.TrimSpace(f.Type))
			switch t {
			case "land", "terrain":
				// Rent listings are homes/stays; land should not be searched here.
				q = q.Where("1 = 0")
			case "villa":
				q = q.Where("(LOWER(property_type) = ? OR LOWER(title) LIKE ? OR LOWER(description) LIKE ?)",
					"villa", "%villa%", "%villa%")
			case "house", "maison", "home":
				q = q.Where("(LOWER(property_type) IN (?, ?, ?) OR LOWER(title) LIKE ? OR LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(description) LIKE ?) AND LOWER(property_type) <> ? AND LOWER(title) NOT LIKE ? AND LOWER(description) NOT LIKE ?",
					"house", "maison", "home",
					"%house%", "%maison%", "%house%", "%maison%",
					"villa", "%villa%", "%villa%")
			case "apartment", "appartement", "flat", "studio":
				q = q.Where("(LOWER(title) LIKE ? OR LOWER(title) LIKE ? OR LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(description) LIKE ? OR LOWER(description) LIKE ?)",
					"%apartment%", "%appartement%", "%studio%",
					"%apartment%", "%appartement%", "%studio%")
			case "room", "private_room", "shared_room":
				q = q.Where("(LOWER(property_type) IN (?, ?) OR LOWER(title) LIKE ? OR LOWER(description) LIKE ?)",
					"private_room", "shared_room", "%room%", "%room%")
			default:
				like := "%" + t + "%"
				q = q.Where("(LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(property_type) LIKE ?)",
					like, like, like)
			}
		}

		q = q.Order("created_at DESC").Limit(12)

		var rows []models.Property
		if err := q.Find(&rows).Error; err != nil {
			fmt.Printf("❌ MeskenyGPT rent search DB error: %v\n", err)
			return []Property{}, err
		}

		fmt.Printf("🔍 MeskenyGPT rent search: city=%s zone=%s type=%s budget=[%.0f,%.0f] → %d rows\n",
			f.City, f.Zone, f.Type, f.BudgetMin, f.BudgetMax, len(rows))

		props := make([]Property, 0, len(rows))
		for _, r := range rows {
			props = append(props, Property{
				ID:       r.ID,
				Title:    r.Title,
				Price:    float64(r.NightlyPrice),
				Currency: r.Currency,
				City:     r.City,
				Bedrooms: r.Bedrooms,
				Image:    firstImageFromString(r.Images),
				Type:     "rent",
				Source:   "property_rent",
			})
		}
		return props, nil
	}

	newBase := func() *gorm.DB {
		return s.db.WithContext(ctx).Model(&models.PropertySale{}).
			Where("is_published = ? AND is_deactivated = ? AND is_sold = ?", true, false, false)
	}

	// Helper to apply common filters (city, type, budget) to a query.
	applyCommon := func(q *gorm.DB) *gorm.DB {
		if f.City != "" {
			// Semantic city match:
			// - exact city value when it exists
			// - plus a LIKE fallback for minor casing/spacing differences.
			c := strings.ToLower(strings.TrimSpace(f.City))
			if f.Zone != "" {
				// Some legacy rows store zone in title/address but leave city empty.
				// Keep strict zone behavior while allowing those rows through.
				q = q.Where("(LOWER(city) = ? OR LOWER(city) LIKE ? OR city IS NULL OR TRIM(city) = '')",
					c, "%"+c+"%")
			} else {
				q = q.Where("(LOWER(city) = ? OR LOWER(city) LIKE ? OR LOWER(address) LIKE ? OR LOWER(description) LIKE ?)",
					c, "%"+c+"%", "%"+c+"%", "%"+c+"%")
			}
		}
		if f.Type != "" && !relaxTypeForBroadQuery {
			q = applySaleTypeFilter(q, f.Type)
		}
		if f.BudgetMin > 0 && f.BudgetMax > f.BudgetMin {
			q = q.Where("listing_price BETWEEN ? AND ?", f.BudgetMin, f.BudgetMax)
		}
		return q
	}

	// Raw query fallback: match user words in title/address/description.
	// This catches area names written only in ad text (e.g. "الصحراوي"),
	// even if they were not parsed into structured zone fields.
	applyQuery := func(q *gorm.DB, query string) *gorm.DB {
		query = strings.TrimSpace(query)
		if query == "" {
			return q
		}
		terms := extractSearchTerms(query)
		if len(terms) == 0 {
			return q
		}
		generic := map[string]bool{
			"دار": true, "منزل": true, "بيت": true, "عقار": true, "عقارات": true,
			"للبيع": true, "تنباع": true, "ابيع": true, "فرصة": true, "فرص": true, "استثمار": true, "الاستثمارية": true,
			"property": true, "properties": true, "sale": true, "rent": true, "house": true, "home": true, "villa": true, "land": true,
		}
		var ors []string
		var args []interface{}
		for _, t := range terms {
			tt := strings.TrimSpace(strings.ToLower(t))
			if tt == "" || len([]rune(tt)) < 2 || generic[tt] {
				continue
			}
			like := "%" + tt + "%"
			ors = append(ors, "(LOWER(title) LIKE ? OR LOWER(address) LIKE ? OR LOWER(description) LIKE ?)")
			args = append(args, like, like, like)
		}
		if len(ors) > 0 {
			q = q.Where(strings.Join(ors, " OR "), args...)
		}
		return q
	}

	applyLocationConstraint := func(q *gorm.DB, query string) *gorm.DB {
		// If user mentions a location phrase (road/zone/area), force at least one
		// location token to appear in title/address/description.
		locationTerms := extractLocationTerms(query)
		if len(locationTerms) == 0 {
			return q
		}
		var ors []string
		var args []interface{}
		for _, t := range locationTerms {
			like := "%" + strings.ToLower(strings.TrimSpace(t)) + "%"
			ors = append(ors, "(LOWER(title) LIKE ? OR LOWER(address) LIKE ? OR LOWER(description) LIKE ?)")
			args = append(args, like, like, like)
		}
		if len(ors) > 0 {
			q = q.Where("("+strings.Join(ors, " OR ")+")", args...)
		}
		return q
	}

	// First pass: if we have a specific zone (e.g. Tevragh Zeina), try to
	// constrain title/address to that zone. If nothing is found, we will
	// relax the zone and search the whole city as a fallback.
	// Zone may be pipe-separated OR patterns (e.g. tevragh|صحراوي|sahraoui).
	q := applyCommon(newBase())
	if f.Zone != "" {
		patterns := strings.Split(f.Zone, "|")
		var ors []string
		var args []interface{}
		for _, raw := range patterns {
			pat := strings.TrimSpace(raw)
			if pat == "" {
				continue
			}
			z := strings.ToLower(pat)
			// Arabic / mixed titles: match pattern as-is and lowercased for Latin.
			likeLatin := "%" + z + "%"
			likeRaw := "%" + pat + "%"
			ors = append(ors, "(LOWER(title) LIKE ? OR LOWER(address) LIKE ? OR LOWER(description) LIKE ? OR title LIKE ? OR address LIKE ? OR description LIKE ?)")
			args = append(args, likeLatin, likeLatin, likeLatin, likeRaw, likeRaw, likeRaw)
		}
		if len(ors) > 0 {
			q = q.Where(strings.Join(ors, " OR "), args...)
		}
	} else {
		q = applyQuery(q, f.Query)
		q = applyLocationConstraint(q, f.Query)
	}

	q = q.Order("is_gold DESC, is_featured DESC, created_at DESC").Limit(12)

	var rows []models.PropertySale
	typeDropped := false

	if err := q.Find(&rows).Error; err != nil {
		fmt.Printf("❌ MeskenyGPT property search DB error: %v\n", err)
		return []Property{}, err
	}

	// Strict behavior: when a zone is explicitly requested, do not broaden
	// to city-wide fallbacks. Return only zone-constrained matches.
	//
	// If no zone was explicitly requested, we can still relax type when needed.
	if len(rows) == 0 && f.Zone == "" && f.City != "" && f.Type != "" {
		base2 := s.db.WithContext(ctx).Model(&models.PropertySale{}).
			Where("is_published = ? AND is_deactivated = ? AND is_sold = ?", true, false, false)
		q3 := base2
		c := strings.ToLower(strings.TrimSpace(f.City))
		q3 = q3.Where("(LOWER(city) = ? OR LOWER(city) LIKE ?)", c, "%"+c+"%")
		if f.BudgetMin > 0 && f.BudgetMax > f.BudgetMin {
			q3 = q3.Where("listing_price BETWEEN ? AND ?", f.BudgetMin, f.BudgetMax)
		}
		q3 = applyQuery(q3, f.Query)
		q3 = applyLocationConstraint(q3, f.Query)
		q3 = q3.Order("is_gold DESC, is_featured DESC, created_at DESC").Limit(12)
		if err := q3.Find(&rows).Error; err != nil {
			fmt.Printf("❌ MeskenyGPT second fallback DB error: %v\n", err)
			return []Property{}, err
		}
		typeDropped = true
	}
	// Final city-text fallback for broad or sparse-structured data:
	// some rows have weak/empty city but mention the city in title/address/description.
	if len(rows) == 0 && f.Zone == "" && strings.TrimSpace(f.City) != "" {
		base3 := s.db.WithContext(ctx).Model(&models.PropertySale{}).
			Where("is_published = ? AND is_deactivated = ? AND is_sold = ?", true, false, false)
		cityToken := strings.ToLower(strings.TrimSpace(f.City))
		likeCity := "%" + cityToken + "%"
		q4 := base3.Where("(LOWER(title) LIKE ? OR LOWER(address) LIKE ? OR LOWER(description) LIKE ?)", likeCity, likeCity, likeCity)
		if f.BudgetMin > 0 && f.BudgetMax > f.BudgetMin {
			q4 = q4.Where("listing_price BETWEEN ? AND ?", f.BudgetMin, f.BudgetMax)
		}
		if f.Type != "" && !relaxTypeForBroadQuery {
			q4 = applySaleTypeFilter(q4, f.Type)
		}
		q4 = applyQuery(q4, f.Query).Order("is_gold DESC, is_featured DESC, created_at DESC").Limit(12)
		if err := q4.Find(&rows).Error; err != nil {
			fmt.Printf("❌ MeskenyGPT city-text fallback DB error: %v\n", err)
			return []Property{}, err
		}
	}

	note := ""
	if typeDropped {
		note = " [type filter dropped]"
	}
	fmt.Printf("🔍 MeskenyGPT property search: city=%s zone=%s type=%s budget=[%.0f,%.0f] → %d rows%s\n",
		f.City, f.Zone, f.Type, f.BudgetMin, f.BudgetMax, len(rows), note)

	props := make([]Property, 0, len(rows))
	for _, r := range rows {
		img := ""
		if len(r.Images) > 0 {
			img = r.Images[0]
		}
		props = append(props, Property{
			ID:       r.ID,
			Title:    r.Title,
			Price:    r.ListingPrice,
			Currency: r.Currency,
			City:     r.City,
			Bedrooms: r.Bedrooms,
			Image:    img,
			Type:     "sale",
		})
	}

	return props, nil
}

// maxLandmarkResults caps landmark rows per AI request (never stream 1k rows).
const maxLandmarkResults = 12

// maxLandmarkMapPlots caps how many cadastre polygons we attach per response.
const maxLandmarkMapPlots = 5

func (s *store) FindLandmarks(ctx context.Context, f Filters) ([]Property, error) {
	if s.db == nil {
		return []Property{}, nil
	}

	if f.QuartierID == 0 && strings.TrimSpace(f.Quartier) != "" {
		f.QuartierID = resolveQuartierID(s.db, f.City, f.Zone, f.Quartier)
	}

	base := func() *gorm.DB {
		return s.db.WithContext(ctx).Model(&models.Landmark{}).
			Where("landmarks.is_verified = ? AND landmarks.is_published = ? AND landmarks.status = ?", true, true, "verified")
	}

	q := base()

	// City: structured fields + title/description (legacy listings often only put city in text).
	city := strings.TrimSpace(strings.ToLower(f.City))
	if city != "" {
		like := "%" + city + "%"
		q = q.Where("(LOWER(landmarks.region) LIKE ? OR LOWER(landmarks.district) LIKE ? OR LOWER(landmarks.title) LIKE ? OR LOWER(landmarks.description) LIKE ?)",
			like, like, like, like)
	}

	// Zone: pipe-separated OR patterns (same idea as property sales)
	zone := strings.TrimSpace(f.Zone)
	if zone != "" {
		patterns := strings.Split(zone, "|")
		var ors []string
		var args []interface{}
		for _, raw := range patterns {
			pat := strings.TrimSpace(raw)
			if pat == "" {
				continue
			}
			z := strings.ToLower(pat)
			likeLatin := "%" + z + "%"
			likeRaw := "%" + pat + "%"
			ors = append(ors, "(LOWER(landmarks.district) LIKE ? OR LOWER(landmarks.region) LIKE ? OR LOWER(landmarks.title) LIKE ? OR LOWER(landmarks.description) LIKE ? OR landmarks.title LIKE ? OR landmarks.description LIKE ?)")
			args = append(args, likeLatin, likeLatin, likeLatin, likeLatin, likeRaw, likeRaw)
		}
		if len(ors) > 0 {
			q = q.Where("("+strings.Join(ors, " OR ")+")", args...)
		}
	}

	// Quartier: prefer structured quartier_id, then catalog name match.
	if f.QuartierID > 0 {
		q = q.Where("landmarks.quartier_id = ?", f.QuartierID)
	} else if qName := strings.TrimSpace(f.Quartier); qName != "" {
		qLike := "%" + strings.ToLower(qName) + "%"
		q = q.Where(`landmarks.quartier_id IN (
			SELECT quartiers.id FROM quartiers
			WHERE LOWER(quartiers.name) LIKE ? OR LOWER(quartiers.name_ar) LIKE ?
				OR quartiers.name = ? OR quartiers.name_ar = ?
		)`, qLike, qLike, qName, qName)
	}

	// Cadastre plot number (exact / case-insensitive)
	if pn := strings.TrimSpace(f.PlotNumber); pn != "" {
		q = q.Where("LOWER(landmarks.plot_number) = LOWER(?)", pn)
	}

	// Budget
	if f.BudgetMin > 0 && f.BudgetMax > f.BudgetMin {
		q = q.Where("landmarks.price BETWEEN ? AND ?", f.BudgetMin, f.BudgetMax)
	}

	// When Zone is empty, require each leftover keyword (AND) so free-text search is scoped.
	query := strings.TrimSpace(f.Query)
	if query != "" && zone == "" {
		for _, kw := range extractLandmarkKeywordTerms(query) {
			like := "%" + strings.ToLower(kw) + "%"
			likeRaw := "%" + kw + "%"
			q = q.Where(
				"(LOWER(landmarks.title) LIKE ? OR LOWER(landmarks.description) LIKE ? OR LOWER(landmarks.district) LIKE ? OR LOWER(landmarks.region) LIKE ? OR landmarks.title LIKE ? OR landmarks.description LIKE ?)",
				like, like, like, like, likeRaw, likeRaw,
			)
		}
	}

	// Prefer cadastre-linked listings with plot geometry for map display.
	q = q.Order("CASE WHEN landmarks.habitat_plot_id IS NOT NULL THEN 0 ELSE 1 END, landmarks.created_at DESC").
		Limit(maxLandmarkResults)

	var rows []models.Landmark
	if err := q.Find(&rows).Error; err != nil {
		fmt.Printf("❌ MeskenyGPT landmarks search DB error: %v\n", err)
		return []Property{}, err
	}

	fmt.Printf("🔍 MeskenyGPT landmarks search: city=%s zone=%s quartier=%s quartier_id=%d plot=%s budget=[%.0f,%.0f] → %d rows (cap=%d)\n",
		city, zone, f.Quartier, f.QuartierID, f.PlotNumber, f.BudgetMin, f.BudgetMax, len(rows), maxLandmarkResults)

	plotByID, quartierNames := s.loadLandmarkEnrichment(ctx, rows)

	props := make([]Property, 0, len(rows))
	mapPlots := 0
	for _, r := range rows {
		img := firstImageFromJSON(r.Images)
		cityVal := r.Region
		if cityVal == "" {
			cityVal = r.District
		}
		if cityVal == "" {
			cityVal = "نواكشوط"
		}
		qLabel := ""
		if r.QuartierID != nil {
			qLabel = quartierNames[*r.QuartierID]
		}

		card := Card{Source: "landmark"}
		var habitat *models.HabitatPlot
		if r.HabitatPlotID != nil && *r.HabitatPlotID > 0 {
			if p, ok := plotByID[*r.HabitatPlotID]; ok {
				hp := p
				habitat = &hp
			}
		}
		enrichLandmarkCard(&card, r, habitat, qLabel)

		if len(card.PlotCorners) >= 3 {
			mapPlots++
		}

		props = append(props, Property{
			ID:             r.ID,
			Title:          r.Title,
			Price:          r.Price,
			Currency:       r.Currency,
			City:           cityVal,
			Bedrooms:       0,
			Image:          img,
			Type:           "sale",
			Source:         "landmark",
			Area:           card.SizeM2,
			LocationLabel:  card.LocationLabel,
			Lat:            card.Lat,
			Lng:            card.Lng,
			PlotNumber:     card.PlotNumber,
			PlotCorners:    card.PlotCorners,
			CadastreLinked: card.CadastreLinked,
			QuartierLabel:  card.QuartierLabel,
		})
	}
	_ = mapPlots
	return props, nil
}

func (s *store) loadLandmarkEnrichment(ctx context.Context, rows []models.Landmark) (map[uint]models.HabitatPlot, map[uint]string) {
	plotIDs := make([]uint, 0)
	quartierIDs := make([]uint, 0)
	for _, r := range rows {
		if r.HabitatPlotID != nil && *r.HabitatPlotID > 0 {
			plotIDs = append(plotIDs, *r.HabitatPlotID)
		}
		if r.QuartierID != nil && *r.QuartierID > 0 {
			quartierIDs = append(quartierIDs, *r.QuartierID)
		}
	}

	plotByID := map[uint]models.HabitatPlot{}
	if len(plotIDs) > 0 {
		var plots []models.HabitatPlot
		if err := s.db.WithContext(ctx).Where("id IN ?", plotIDs).Find(&plots).Error; err == nil {
			for _, p := range plots {
				plotByID[p.ID] = p
			}
		}
	}

	qNames := map[uint]string{}
	if len(quartierIDs) > 0 {
		var qs []models.Quartier
		if err := s.db.WithContext(ctx).Where("id IN ?", quartierIDs).Find(&qs).Error; err == nil {
			for _, q := range qs {
				name := strings.TrimSpace(q.Name)
				if strings.TrimSpace(q.NameAr) != "" {
					name = strings.TrimSpace(q.NameAr)
				}
				qNames[q.ID] = name
			}
		}
	}
	return plotByID, qNames
}

func firstImageFromJSON(js []byte) string {
	if len(js) == 0 {
		return ""
	}
	var urls []string
	if err := json.Unmarshal(js, &urls); err != nil || len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func firstImageFromString(js string) string {
	js = strings.TrimSpace(js)
	if js == "" {
		return ""
	}
	var urls []string
	if err := json.Unmarshal([]byte(js), &urls); err != nil || len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func shouldApplyRentTypeFilter(query, parsedType string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	t := strings.ToLower(strings.TrimSpace(parsedType))
	if q == "" || t == "" {
		return false
	}

	// Only enforce type filter when user text explicitly asks for that type.
	switch t {
	case "villa":
		return strings.Contains(q, "villa") || strings.Contains(q, "فيلا")
	case "house", "maison", "home":
		return strings.Contains(q, "house") ||
			strings.Contains(q, "maison") ||
			strings.Contains(q, "home") ||
			strings.Contains(q, "دار") ||
			strings.Contains(q, "منزل") ||
			strings.Contains(q, "بيت")
	case "land", "terrain":
		return strings.Contains(q, "land") ||
			strings.Contains(q, "terrain") ||
			strings.Contains(q, "ارض") ||
			strings.Contains(q, "أرض")
	case "apartment", "appartement", "flat", "studio":
		return strings.Contains(q, "apartment") ||
			strings.Contains(q, "appartement") ||
			strings.Contains(q, "flat") ||
			strings.Contains(q, "studio") ||
			strings.Contains(q, "شقة") ||
			strings.Contains(q, "استوديو")
	case "room", "private_room", "shared_room":
		return strings.Contains(q, "room") ||
			strings.Contains(q, "غرفة")
	default:
		return strings.Contains(q, t)
	}
}

func applySaleTypeFilter(q *gorm.DB, propertyType string) *gorm.DB {
	t := strings.ToLower(strings.TrimSpace(propertyType))
	switch t {
	case "land", "terrain":
		return q.Where("LOWER(property_type) IN (?, ?)", "land", "terrain")
	case "villa":
		return q.Where(`(
			LOWER(property_type) = ?
			OR LOWER(title) LIKE ?
			OR LOWER(description) LIKE ?
		)`, "villa", "%villa%", "%villa%")
	case "house", "maison", "home":
		return q.Where(`(
			LOWER(property_type) IN (?, ?, ?, ?, ?, ?)
			OR LOWER(title) LIKE ?
			OR LOWER(title) LIKE ?
			OR LOWER(description) LIKE ?
			OR LOWER(description) LIKE ?
		) AND LOWER(COALESCE(property_type, '')) <> ?
			AND LOWER(title) NOT LIKE ?
			AND LOWER(description) NOT LIKE ?`,
			"house", "maison", "home", "duplex", "townhouse", "residential",
			"%house%", "%maison%", "%house%", "%maison%",
			"villa", "%villa%", "%villa%")
	case "appartement", "apartment", "flat", "studio":
		return q.Where("(LOWER(property_type) IN (?, ?, ?, ?) OR LOWER(title) LIKE ? OR LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(description) LIKE ?)",
			"apartment", "appartement", "flat", "studio",
			"%apartment%", "%appartement%", "%apartment%", "%appartement%")
	default:
		return q.Where("LOWER(property_type) = ?", t)
	}
}

func shouldRelaxTypeForBroadQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	return strings.Contains(q, "all properties") ||
		strings.Contains(q, "all real estate") ||
		strings.Contains(q, "all houses") ||
		strings.Contains(q, "all listings") ||
		strings.Contains(q, "كل العقارات") ||
		strings.Contains(q, "جميع العقارات") ||
		strings.Contains(q, "كامل العقارات") ||
		strings.Contains(q, "all homes") ||
		strings.Contains(q, "عقارات في") ||
		strings.Contains(q, "real estate in")
}

func extractSearchTerms(s string) []string {
	// Extract meaningful terms (skip very short and common words)
	words := strings.Fields(s)
	var out []string
	stop := map[string]bool{
		"في": true, "على": true, "من": true, "إلى": true, "ال": true, "و": true, "أو": true,
		"the": true, "a": true, "in": true, "for": true,
		"de": true, "la": true, "le": true, "du": true, "et": true, "ou": true,
	}
	for _, w := range words {
		w = strings.TrimSpace(w)
		if len(w) >= 2 && !stop[w] {
			out = append(out, w)
		}
	}
	return out
}

// extractLandmarkKeywordTerms returns tokens for AND search when Zone is not set from intent.
// Prefers multi-word place phrases; strips Hassaniya/generic real-estate words.
func extractLandmarkKeywordTerms(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	low := strings.ToLower(s)
	// Longest locality phrases first (one is enough for a tight AND).
	phrases := []string{
		"المطار القديم", "مطار قديم", "ilo k", "ilô k", "إلو ك", "ايلو ك", "old airport",
	}
	for _, p := range phrases {
		if strings.Contains(s, p) || strings.Contains(low, strings.ToLower(p)) {
			return []string{p}
		}
	}

	stop := map[string]bool{
		"في": true, "على": true, "من": true, "إلى": true, "ال": true, "و": true, "أو": true, "ف": true,
		"the": true, "a": true, "in": true, "for": true,
		"de": true, "la": true, "le": true, "du": true, "et": true, "ou": true,
		"أرض": true, "ارض": true, "تراب": true, "نيمرو": true, "نمرة": true,
		"terrain": true, "land": true,
		"للبيع": true, "تنباع": true, "تنبيع": true, "نبيع": true, "تبيع": true, "ينباع": true, "بيع": true,
		"اندور": true, "أدور": true, "أريد": true, "ابحث": true, "أبحث": true, "عقار": true, "عقارات": true,
		"دار": true, "منزل": true, "بيت": true,
	}
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(s) {
		w = strings.TrimSpace(w)
		t := strings.ToLower(w)
		if len([]rune(t)) < 2 || stop[w] || stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, w)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func extractLocationTerms(s string) []string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return nil
	}
	// Trigger only when the user language suggests a location-qualified request.
	locationCue := strings.Contains(lower, "طريق") ||
		strings.Contains(lower, "حي") ||
		strings.Contains(lower, "منطقة") ||
		strings.Contains(lower, "شارع") ||
		strings.Contains(lower, "سكتور") ||
		strings.Contains(lower, "المطار") ||
		strings.Contains(lower, "road") ||
		strings.Contains(lower, "route") ||
		strings.Contains(lower, "street") ||
		strings.Contains(lower, "avenue") ||
		strings.Contains(lower, "quartier") ||
		strings.Contains(lower, "district")
	if !locationCue {
		return nil
	}

	terms := extractSearchTerms(s)
	block := map[string]bool{
		"دار": true, "منزل": true, "بيت": true, "عقار": true, "عقارات": true,
		"للبيع": true, "تنباع": true, "ابيع": true, "اندور": true, "أدور": true, "ابحث": true, "أبحث": true,
		"الفرص": true, "فرص": true, "فرصة": true, "الاستثمارية": true, "استثمار": true,
		"property": true, "properties": true, "sale": true, "rent": true, "house": true, "home": true, "villa": true, "land": true,
	}
	out := make([]string, 0, len(terms))
	seen := map[string]bool{}
	for _, t := range terms {
		tt := strings.ToLower(strings.TrimSpace(t))
		if tt == "" || block[tt] || len([]rune(tt)) < 2 || seen[tt] {
			continue
		}
		seen[tt] = true
		out = append(out, tt)
	}
	return out
}

