package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// attachApprovedListingVideos sets Property.ListingVideo from the `videos` table (property-linked rent tours).
// Uploads start as status=pending (CreateVideo); we prefer approved but fall back to latest pending so the row
// in `videos` actually surfaces on discovery cards before/without admin action.
// rentDiscoveryFilters holds optional query filters for GET …/criteria/:id/properties (rent listings).
type rentDiscoveryFilters struct {
	PropertyType         string
	PropertyCategoryID   *uint
	CityID               *uint
	ZoneID               *uint
	QuartierID           *uint
	MinPrice             *float64
	MaxPrice             *float64
}

func (f rentDiscoveryFilters) active() bool {
	return f.PropertyType != "" || f.PropertyCategoryID != nil || f.CityID != nil || f.ZoneID != nil || f.QuartierID != nil ||
		f.MinPrice != nil || f.MaxPrice != nil
}

func parseRentDiscoveryFilters(ctx iris.Context) rentDiscoveryFilters {
	var f rentDiscoveryFilters
	pt := strings.TrimSpace(ctx.URLParam("property_type"))
	if pt == "" {
		pt = strings.TrimSpace(ctx.URLParam("propertyType"))
	}
	pt = strings.ToLower(strings.TrimSpace(pt))
	if pt != "" && pt != "all" {
		f.PropertyType = pt
	}
	for _, key := range []string{"property_category_id", "category_id", "categoryId"} {
		if v := strings.TrimSpace(ctx.URLParam(key)); v != "" {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
				u := uint(n)
				f.PropertyCategoryID = &u
				break
			}
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("city_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			u := uint(n)
			f.CityID = &u
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("zone_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			u := uint(n)
			f.ZoneID = &u
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("quartier_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			u := uint(n)
			f.QuartierID = &u
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("min_price")); v != "" {
		if x, err := strconv.ParseFloat(v, 64); err == nil {
			f.MinPrice = &x
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("max_price")); v != "" {
		if x, err := strconv.ParseFloat(v, 64); err == nil {
			f.MaxPrice = &x
		}
	}
	return f
}

func propertyMatchesRentDiscovery(p models.Property, f rentDiscoveryFilters) bool {
	if f.PropertyType != "" {
		pt := strings.ToLower(strings.TrimSpace(p.PropertyType))
		if pt != f.PropertyType {
			return false
		}
	}
	if f.PropertyCategoryID != nil {
		if p.PropertyCategoryID == nil || *p.PropertyCategoryID != *f.PropertyCategoryID {
			return false
		}
	}
	if f.CityID != nil {
		if p.CityID == nil || *p.CityID != *f.CityID {
			return false
		}
	}
	if f.ZoneID != nil {
		if p.ZoneID == nil || *p.ZoneID != *f.ZoneID {
			return false
		}
	}
	if f.QuartierID != nil {
		if p.QuartierID == nil || *p.QuartierID != *f.QuartierID {
			return false
		}
	}
	n := float64(p.NightlyPrice)
	if f.MinPrice != nil && n < *f.MinPrice {
		return false
	}
	if f.MaxPrice != nil && n > *f.MaxPrice {
		return false
	}
	return true
}

func publicRentListingStatuses(ctx iris.Context) []string {
	raw := strings.TrimSpace(ctx.URLParam("status"))
	if raw == "" {
		return []string{"approved", "published"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "approved" && p != "published" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"approved", "published"}
	}
	return out
}

func isBlockedRentListingStatus(status string) bool {
	st := strings.TrimSpace(strings.ToLower(status))
	if st == "" {
		return true
	}
	if strings.Contains(st, "reject") {
		return true
	}
	switch st {
	case "pending", "draft", "denied", "cancelled", "canceled", "suspended", "inactive", "blocked":
		return true
	default:
		return false
	}
}

func isPublicRentListingStatus(status string) bool {
	if isBlockedRentListingStatus(status) {
		return false
	}
	st := strings.TrimSpace(strings.ToLower(status))
	switch st {
	case "approved", "published":
		return true
	default:
		return false
	}
}

func removePropertyFromLocationCriteria(propertyID uint) error {
	return storage.DB.Where("property_id = ?", propertyID).Delete(&models.LocationCriteriaProperty{}).Error
}

func filterPropertiesByRentDiscovery(props []models.Property, f rentDiscoveryFilters) []models.Property {
	out := make([]models.Property, 0, len(props))
	for i := range props {
		if propertyMatchesRentDiscovery(props[i], f) {
			out = append(out, props[i])
		}
	}
	return out
}

func attachApprovedListingVideos(pageItems []models.Property) {
	if len(pageItems) == 0 {
		return
	}
	ids := make([]uint, len(pageItems))
	for i := range pageItems {
		ids[i] = pageItems[i].ID
	}
	var rows []models.Video
	q := storage.DB.Model(&models.Video{}).
		Where("videos.property_id IN ?", ids).
		Where("videos.video_url IS NOT NULL AND TRIM(videos.video_url) <> ''").
		Where("COALESCE(videos.is_promotional, false) = ?", false).
		Where("COALESCE(videos.is_flagged, false) = ?", false).
		Where("LOWER(TRIM(COALESCE(videos.status, ''))) IN ?", []string{"approved", "pending"}).
		Order(`videos.property_id ASC,
			CASE WHEN LOWER(TRIM(COALESCE(videos.status, ''))) = 'approved' THEN 0 ELSE 1 END ASC,
			videos.id DESC`)
	if err := q.Find(&rows).Error; err != nil {
		return
	}
	seen := make(map[uint]struct{})
	for _, v := range rows {
		if v.PropertyID == nil {
			continue
		}
		pid := *v.PropertyID
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		st := strings.ToLower(strings.TrimSpace(v.Status))
		if st == "" {
			st = "pending"
		}
		lv := &models.PropertyListingVideo{
			VideoID:      v.ID,
			PropertyID:   pid,
			VideoURL:     strings.TrimSpace(v.VideoURL),
			ThumbnailURL: v.ThumbnailURL,
			Caption:      v.Caption,
			DurationSec:  v.DurationSec,
			Status:       st,
		}
		for i := range pageItems {
			if pageItems[i].ID == pid {
				pageItems[i].ListingVideo = lv
				break
			}
		}
	}
}

// GetLocationCriteria returns all active location criteria
func GetLocationCriteria(ctx iris.Context) {
	var criteria []models.LocationCriteria

	// Get all active criteria ordered by priority
	if err := storage.DB.Where("is_active = ?", true).
		Order("priority DESC, name ASC").
		Find(&criteria).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch location criteria"})
		return
	}

	// Convert to response format with property counts
	var response []models.GetLocationCriteriaResponse
	for _, criterion := range criteria {
		// Count properties for this criteria
		var propertyCount int64
		storage.DB.Model(&models.LocationCriteriaProperty{}).
			Where("location_criteria_id = ? AND is_active = ?", criterion.ID, true).
			Count(&propertyCount)

		response = append(response, models.GetLocationCriteriaResponse{
			ID:            criterion.ID,
			Name:          criterion.Name,
			DisplayName:   criterion.DisplayName,
			Description:   criterion.Description,
			CenterLat:     criterion.CenterLat,
			CenterLng:     criterion.CenterLng,
			Radius:        criterion.Radius,
			Priority:      criterion.Priority,
			IsActive:      criterion.IsActive,
			Icon:          criterion.Icon,
			Color:         criterion.Color,
			PropertyCount: int(propertyCount),
		})
	}

	// Debug logging
	fmt.Printf("🔍 GetLocationCriteria - Returning %d criteria:\n", len(response))
	for _, criterion := range response {
		fmt.Printf("  - %s (ID: %d, Lat: %.4f, Lng: %.4f, Radius: %.1fkm)\n",
			criterion.Name, criterion.ID, criterion.CenterLat, criterion.CenterLng, criterion.Radius)
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
	})
}

// GetLocationProperties returns properties for a specific location criteria
// Works for both authenticated and unauthenticated users
// Properties are rotated per-request to show fresh content (TikTok-like cycling)
// Properties with images are always prioritized at the top
func GetLocationProperties(ctx iris.Context) {
	criteriaIDStr := ctx.Params().Get("criteriaId")
	criteriaID, err := strconv.ParseUint(criteriaIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid criteria ID"})
		return
	}

	// Get limit from query params
	limitStr := ctx.URLParamDefault("limit", "8")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}
	page := ctx.URLParamIntDefault("page", 1)
	if page < 1 {
		page = 1
	}

	// Get the location criteria
	var criteria models.LocationCriteria
	if err := storage.DB.Where("id = ? AND is_active = ?", criteriaID, true).First(&criteria).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Location criteria not found"})
		return
	}

	// Log what the user clicked on
	fmt.Printf("🔍 USER CLICKED ON LOCATION CRITERIA: %s (ID: %d, DisplayName: %s)\n",
		criteria.Name, criteria.ID, criteria.DisplayName)
	fmt.Printf("🔍 Criteria Details: Lat=%.4f, Lng=%.4f, Radius=%.1fkm\n",
		criteria.CenterLat, criteria.CenterLng, criteria.Radius)

	// Extract userID for user-specific exclusions (optional auth)
	var userID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
				}
			}
		}
	}

	allowedStatuses := publicRentListingStatuses(ctx)

	// Build base query — only link rows whose property is active + publicly listable (approved).
	query := storage.DB.
		Where("location_criteria_id = ? AND is_active = ?", criteriaID, true).
		Where(`property_id IN (
			SELECT id FROM properties
			WHERE COALESCE(is_active, true) = true
			AND COALESCE(is_flagged, false) = false
			AND LOWER(TRIM(COALESCE(status, ''))) IN ?
			AND LOWER(TRIM(COALESCE(status, ''))) NOT IN ?
		)`, allowedStatuses, []string{"rejected", "pending", "draft", "denied", "cancelled", "canceled", "suspended", "inactive", "blocked"})

	// Apply user-specific exclusions if authenticated
	if userID > 0 {
		query = query.Where("property_id NOT IN (SELECT property_id FROM hidden_properties WHERE user_id = ?)", userID)
		query = query.Where("property_id NOT IN (SELECT property_id FROM property_reports WHERE reporter_id = ?)", userID)
		query = query.Where("property_id NOT IN (SELECT p.id FROM properties p JOIN user_flags uf ON p.host_id = uf.flagged_user_id WHERE uf.flagger_id = ? AND uf.status='active')", userID)
	}

	// Get all properties assigned to this criteria (SQL already enforces public status)
	var allCriteriaProperties []models.LocationCriteriaProperty

	if err := query.
		Preload("Property").
		Preload("Property.Host").
		Find(&allCriteriaProperties).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to fetch properties"})
		return
	}

	// Extract all properties first
	var allProperties []models.Property
	for _, cp := range allCriteriaProperties {
		if cp.Property.ID == 0 {
			continue
		}
		if !isPublicRentListingStatus(cp.Property.Status) {
			continue
		}
		if cp.Property.IsActive != nil && !*cp.Property.IsActive {
			continue
		}
		if cp.Property.IsFlagged {
			continue
		}
		allProperties = append(allProperties, cp.Property)
	}

	rdFilters := parseRentDiscoveryFilters(ctx)
	if rdFilters.active() {
		before := len(allProperties)
		allProperties = filterPropertiesByRentDiscovery(allProperties, rdFilters)
		fmt.Printf("🔎 Rent discovery filters applied: %d -> %d properties (criteria=%d)\n",
			before, len(allProperties), criteria.ID)
	}

	fmt.Printf("✅ Found %d properties for criteria '%s' before rotation\n", len(allProperties), criteria.Name)

	// Separate properties with images from those without
	var withImages []models.Property
	var withoutImages []models.Property
	
	for _, prop := range allProperties {
		// Check if property has images (Images is stored as JSON string)
		hasImages := false
		if prop.Images != "" {
			var images []string
			if err := json.Unmarshal([]byte(prop.Images), &images); err == nil {
				// Check if images array contains non-empty strings
				for _, img := range images {
					if strings.TrimSpace(img) != "" {
						hasImages = true
						break
					}
				}
			}
		}
		
		if hasImages {
			withImages = append(withImages, prop)
		} else {
			withoutImages = append(withoutImages, prop)
		}
	}

	fmt.Printf("📸 Properties with images: %d, without images: %d\n", len(withImages), len(withoutImages))

	// Rank each bucket first (fresh + popular content), then rotate with a deterministic seed.
	// This gives variety between requests without unstable randomness or duplicate-heavy order.
	sortBucket := func(props []models.Property) {
		sort.SliceStable(props, func(i, j int) bool {
			pi := props[i]
			pj := props[j]
			score := func(p models.Property) float64 {
				s := 0.0
				age := time.Since(p.CreatedAt)
				if age <= 24*time.Hour {
					s += 18
				} else if age <= 7*24*time.Hour {
					s += 10
				}
				if p.Rating >= 4.7 {
					s += 10
				} else if p.Rating >= 4.0 {
					s += 6
				}
				if p.NightlyPrice > 0 {
					// Slight preference for realistic market prices over outliers
					s += 2
				}
				return s
			}
			si := score(pi)
			sj := score(pj)
			if si != sj {
				return si > sj
			}
			if !pi.CreatedAt.Equal(pj.CreatedAt) {
				return pi.CreatedAt.After(pj.CreatedAt)
			}
			return pi.ID > pj.ID
		})
	}
	rotateBySeed := func(props []models.Property, seed uint64) []models.Property {
		if len(props) <= 1 {
			return props
		}
		offset := int(seed % uint64(len(props)))
		if offset == 0 {
			return props
		}
		rotated := make([]models.Property, 0, len(props))
		rotated = append(rotated, props[offset:]...)
		rotated = append(rotated, props[:offset]...)
		return rotated
	}

	sortBucket(withImages)
	sortBucket(withoutImages)

	timeBucket := time.Now().Unix() / 30 // refresh ordering every ~30s
	seedBase := uint64(criteriaID) ^ uint64(limit*31) ^ uint64(page*131) ^ uint64(timeBucket)
	if userID > 0 {
		seedBase ^= uint64(userID) * 11400714819323198485
	}
	withImages = rotateBySeed(withImages, seedBase)
	withoutImages = rotateBySeed(withoutImages, seedBase^0x9e3779b97f4a7c15)

	// Combine: properties with images first, then without
	var properties []models.Property
	properties = append(properties, withImages...)
	properties = append(properties, withoutImages...)

	totalCount := len(properties)
	offset := (page - 1) * limit
	if offset > totalCount {
		offset = totalCount
	}
	end := offset + limit
	if end > totalCount {
		end = totalCount
	}
	pageItems := properties[offset:end]
	hasMore := end < totalCount

	fmt.Printf("✅ Returning %d properties (criteria=%d page=%d limit=%d total=%d hasMore=%v)\n",
		len(pageItems), criteria.ID, page, limit, totalCount, hasMore)

	// Localize property fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range pageItems {
		p := &pageItems[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
		p.NeighborhoodDescription = utils.ResolveLocalizedText(
			p.NeighborhoodDescription,
			p.NeighborhoodDescriptionTranslations,
			lang,
		)
	}

	attachApprovedListingVideos(pageItems)

	// Log how many properties were found
	fmt.Printf("🔍 FOUND %d PROPERTIES FOR CRITERIA '%s' ON THIS PAGE:\n", len(pageItems), criteria.Name)
	for i, prop := range pageItems {
		fmt.Printf("  %d. ID: %d, Title: '%s', Price: %.2f MRU, Host: %s\n",
			i+1, prop.ID, prop.Title, prop.NightlyPrice, prop.Host.FirstName+" "+prop.Host.LastName)
	}

	// Convert criteria to response format
	criteriaResponse := models.GetLocationCriteriaResponse{
		ID:            criteria.ID,
		Name:          criteria.Name,
		DisplayName:   criteria.DisplayName,
		Description:   criteria.Description,
		CenterLat:     criteria.CenterLat,
		CenterLng:     criteria.CenterLng,
		Radius:        criteria.Radius,
		Priority:      criteria.Priority,
		IsActive:      criteria.IsActive,
		Icon:          criteria.Icon,
		Color:         criteria.Color,
		PropertyCount: totalCount,
	}

	response := models.GetLocationPropertiesResponse{
		LocationCriteria: criteriaResponse,
		Properties:       pageItems,
		TotalCount:       totalCount,
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
		"meta": iris.Map{
			"page":    page,
			"limit":   limit,
			"total":   totalCount,
			"hasMore": hasMore,
		},
	})
}

// InitializeLocationCriteriaEndpoint initializes location criteria via API
func InitializeLocationCriteriaEndpoint(ctx iris.Context) {
	// Initialize location criteria
	if err := InitializeLocationCriteria(); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to initialize location criteria", "error": err.Error()})
		return
	}

	// Assign properties to criteria
	if err := AssignPropertiesToCriteria(); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to assign properties to criteria", "error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Location criteria initialized successfully",
	})
}

// AssignPropertiesToCriteriaEndpoint assigns properties to criteria via API
func AssignPropertiesToCriteriaEndpoint(ctx iris.Context) {
	// Assign properties to criteria
	if err := AssignPropertiesToCriteria(); err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to assign properties to criteria", "error": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Properties assigned to criteria successfully",
	})
}

// AssignSinglePropertyToLocationCriteria assigns a single property to appropriate location criteria
func AssignSinglePropertyToLocationCriteria(propertyID uint) error {
	// Get the property
	var property models.Property
	if err := storage.DB.Where("id = ?", propertyID).First(&property).Error; err != nil {
		return fmt.Errorf("property not found: %v", err)
	}

	if !isPublicRentListingStatus(property.Status) {
		if err := removePropertyFromLocationCriteria(propertyID); err != nil {
			return fmt.Errorf("failed to clear non-public property from criteria: %v", err)
		}
		return nil
	}
	if property.IsActive != nil && !*property.IsActive {
		if err := removePropertyFromLocationCriteria(propertyID); err != nil {
			return fmt.Errorf("failed to clear inactive property from criteria: %v", err)
		}
		return nil
	}
	if property.IsFlagged {
		if err := removePropertyFromLocationCriteria(propertyID); err != nil {
			return fmt.Errorf("failed to clear flagged property from criteria: %v", err)
		}
		return nil
	}

	// Get all active criteria ordered by priority (highest first)
	var criteria []models.LocationCriteria
	if err := storage.DB.Where("is_active = ?", true).Order("priority DESC").Find(&criteria).Error; err != nil {
		return fmt.Errorf("failed to fetch criteria: %v", err)
	}

	// Remove existing assignments for this property
	if err := storage.DB.Where("property_id = ?", propertyID).Delete(&models.LocationCriteriaProperty{}).Error; err != nil {
		return fmt.Errorf("failed to remove existing assignments: %v", err)
	}

	// Calculate distance and assign to appropriate criteria
	propertyAssigned := false
	var closestCriterion *models.LocationCriteria
	var closestDistance float64 = -1

	for _, criterion := range criteria {
		distance := CalculateDistance(
			float64(property.Lat),
			float64(property.Lng),
			criterion.CenterLat,
			criterion.CenterLng,
		)

		if distance <= criterion.Radius {
			// Create assignment
			assignment := models.LocationCriteriaProperty{
				LocationCriteriaID: criterion.ID,
				PropertyID:         property.ID,
				Distance:           distance,
				IsActive:           true,
			}

			if err := storage.DB.Create(&assignment).Error; err != nil {
				return fmt.Errorf("failed to create assignment: %v", err)
			}

			propertyAssigned = true
			fmt.Printf("✅ Property %d assigned to criteria '%s' (distance: %.2fkm)\n",
				property.ID, criterion.Name, distance)
		}

		// Track closest criterion for fallback (approved properties must show somewhere)
		if closestDistance < 0 || distance < closestDistance {
			closestDistance = distance
			c := criterion
			closestCriterion = &c
		}
	}

	// Fallback: if outside all radii, assign to closest criterion so approved property shows
	if !propertyAssigned && closestCriterion != nil {
		assignment := models.LocationCriteriaProperty{
			LocationCriteriaID: closestCriterion.ID,
			PropertyID:         property.ID,
			Distance:           closestDistance,
			IsActive:           true,
		}
		if err := storage.DB.Create(&assignment).Error; err != nil {
			return fmt.Errorf("failed to create fallback assignment: %v", err)
		}
		fmt.Printf("✅ Property %d fallback-assigned to nearest criteria '%s' (distance: %.2fkm, outside radius)\n",
			property.ID, closestCriterion.Name, closestDistance)
	} else if !propertyAssigned {
		fmt.Printf("⚠️ Property %d not assigned (no criteria exist)\n", property.ID)
	}

	return nil
}

// CalculateDistance calculates the distance between two points in kilometers
func CalculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// IsPointInCircle checks if a point is within a circle
func IsPointInCircle(pointLat, pointLng, centerLat, centerLng, radiusKm float64) bool {
	distance := CalculateDistance(pointLat, pointLng, centerLat, centerLng)
	return distance <= radiusKm
}

// AssignPropertiesToCriteria assigns properties to location criteria based on geographic boundaries
func AssignPropertiesToCriteria() error {
	// Get all active criteria ordered by priority (highest first)
	var criteria []models.LocationCriteria
	if err := storage.DB.Where("is_active = ?", true).Order("priority DESC").Find(&criteria).Error; err != nil {
		return err
	}

	// Get all active properties
	var properties []models.Property
	if err := storage.DB.Where("is_active = ?", true).Find(&properties).Error; err != nil {
		return err
	}

	fmt.Printf("Found %d active properties to assign\n", len(properties))
	fmt.Printf("Processing criteria in priority order:\n")
	for i, criterion := range criteria {
		fmt.Printf("  %d. %s (priority: %d, radius: %.1fkm)\n", i+1, criterion.Name, criterion.Priority, criterion.Radius)
	}

	// Clear existing assignments
	storage.DB.Where("1 = 1").Delete(&models.LocationCriteriaProperty{})

	// Track which properties have been assigned to avoid duplicates
	assignedProperties := make(map[uint]bool)

	// Process each criterion
	for _, criterion := range criteria {
		var assignedCount int

		for _, property := range properties {
			// Skip if property already assigned to another criteria
			if assignedProperties[property.ID] {
				continue
			}

			// Debug: Log property coordinates
			fmt.Printf("Checking property %d: lat=%.6f, lng=%.6f against criteria %s (center: %.6f, %.6f, radius: %.1fkm)\n",
				property.ID, property.Lat, property.Lng, criterion.Name, criterion.CenterLat, criterion.CenterLng, criterion.Radius)

			// Check if property is within the circle
			if IsPointInCircle(float64(property.Lat), float64(property.Lng), criterion.CenterLat, criterion.CenterLng, criterion.Radius) {
				// Calculate distance from center
				distance := CalculateDistance(float64(property.Lat), float64(property.Lng), criterion.CenterLat, criterion.CenterLng)

				// Create assignment
				assignment := models.LocationCriteriaProperty{
					LocationCriteriaID: criterion.ID,
					PropertyID:         property.ID,
					Distance:           distance,
					IsActive:           true,
				}

				if err := storage.DB.Create(&assignment).Error; err != nil {
					fmt.Printf("Error assigning property %d to criteria %d: %v\n", property.ID, criterion.ID, err)
					continue
				}

				// Mark property as assigned
				assignedProperties[property.ID] = true
				assignedCount++

				// Limit properties per criteria to avoid overcrowding
				if assignedCount >= 20 {
					break
				}
			}
		}

		fmt.Printf("Assigned %d properties to criteria '%s'\n", assignedCount, criterion.Name)
	}

	return nil
}

// InitializeLocationCriteria creates default location criteria for Nouakchott
func InitializeLocationCriteria() error {
	// Force re-initialization by clearing existing criteria first
	storage.DB.Where("1 = 1").Delete(&models.LocationCriteria{})
	fmt.Println("🔍 Cleared existing location criteria for re-initialization")

	// Define Nouakchott location criteria
	criteria := []models.LocationCriteria{
		{
			Name:        "tevragh_zeina",
			DisplayName: "Properties in Tevragh Zeina",
			Description: "Luxury stays in the diplomatic quarter",
			CenterLat:   18.0861,
			CenterLng:   -15.9753,
			Radius:      2.0, // 2km radius
			Priority:    10,
			IsActive:    true,
			Icon:        "business",
			Color:       "#00A699",
		},
		{
			Name:        "palais_congres",
			DisplayName: "Near Palais des Congrès",
			Description: "Properties near the convention center",
			CenterLat:   18.0844,
			CenterLng:   -15.9789,
			Radius:      1.5, // 1.5km radius
			Priority:    9,
			IsActive:    true,
			Icon:        "place",
			Color:       "#FF5A5F",
		},
		{
			Name:        "presidential_palace",
			DisplayName: "Near Presidential Palace",
			Description: "Properties near the presidential palace",
			CenterLat:   18.0922,
			CenterLng:   -15.9711,
			Radius:      1.8, // 1.8km radius
			Priority:    8,
			IsActive:    true,
			Icon:        "account_balance",
			Color:       "#C0C0C0",
		},
		{
			Name:        "port_nouakchott",
			DisplayName: "Port de Nouakchott",
			Description: "Properties near the port area",
			CenterLat:   18.0956,
			CenterLng:   -15.9889,
			Radius:      4.0, // 4.0km radius (expanded to include Property 8)
			Priority:    7,
			IsActive:    true,
			Icon:        "local_shipping",
			Color:       "#2196F3",
		},
		{
			Name:        "airport_area",
			DisplayName: "Near Airport",
			Description: "Properties near the international airport",
			CenterLat:   18.0978,
			CenterLng:   -15.9567,
			Radius:      3.0, // 3km radius
			Priority:    6,
			IsActive:    true,
			Icon:        "flight",
			Color:       "#4CAF50",
		},
		{
			Name:        "embassy_quarter",
			DisplayName: "Embassy Quarter",
			Description: "Luxury properties in the embassy district",
			CenterLat:   18.0889,
			CenterLng:   -15.9722,
			Radius:      1.2, // 1.2km radius
			Priority:    5,
			IsActive:    true,
			Icon:        "account_balance",
			Color:       "#9C27B0",
		},
		{
			Name:        "city_center",
			DisplayName: "City Center",
			Description: "Properties in the heart of Nouakchott",
			CenterLat:   18.0733,
			CenterLng:   -15.9589,
			Radius:      2.0, // 2km radius
			Priority:    4,
			IsActive:    true,
			Icon:        "location_city",
			Color:       "#FF9800",
		},
		{
			Name:        "beach_area",
			DisplayName: "Beach Area",
			Description: "Properties near the beach",
			CenterLat:   18.0667,
			CenterLng:   -15.9444,
			Radius:      2.2, // 2.2km radius
			Priority:    3,
			IsActive:    true,
			Icon:        "beach_access",
			Color:       "#00BCD4",
		},
		{
			Name:        "centre_emetteur",
			DisplayName: "Centre Emetteur",
			Description: "Properties near Centre Emetteur broadcasting center",
			CenterLat:   18.0850,
			CenterLng:   -15.9650,
			Radius:      1.5, // 1.5km radius
			Priority:    2,
			IsActive:    true,
			Icon:        "broadcast",
			Color:       "#FF5722",
		},
		{
			Name:        "cite_plage",
			DisplayName: "Cite Plage",
			Description: "Properties in Cite Plage area",
			CenterLat:   18.0750,
			CenterLng:   -15.9450,
			Radius:      2.0, // 2km radius
			Priority:    1,
			IsActive:    true,
			Icon:        "beach_access",
			Color:       "#00BCD4",
		},
	}

	// Create criteria
	for _, criterion := range criteria {
		if err := storage.DB.Create(&criterion).Error; err != nil {
			return fmt.Errorf("error creating criteria %s: %v", criterion.Name, err)
		}
	}

	fmt.Println("Location criteria initialized successfully")
	return nil
}

func GetPropertyLocationCriteria(ctx iris.Context) {
	propertyID := ctx.Params().GetUintDefault("propertyId", 0)
	if propertyID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"message": "Invalid property ID"})
		return
	}

	// Get the property
	var property models.Property
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property not found"})
		return
	}

	// Find which criteria this property belongs to
	var criteriaProperty models.LocationCriteriaProperty
	if err := storage.DB.Where("property_id = ?", propertyID).
		Preload("LocationCriteria").
		First(&criteriaProperty).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"message": "Property not assigned to any location criteria"})
		return
	}

	// Calculate distance from property to criteria center
	distance := CalculateDistance(
		float64(property.Lat),
		float64(property.Lng),
		criteriaProperty.LocationCriteria.CenterLat,
		criteriaProperty.LocationCriteria.CenterLng,
	)

	response := iris.Map{
		"id":          criteriaProperty.LocationCriteria.ID,
		"name":        criteriaProperty.LocationCriteria.Name,
		"displayName": criteriaProperty.LocationCriteria.DisplayName,
		"centerLat":   criteriaProperty.LocationCriteria.CenterLat,
		"centerLng":   criteriaProperty.LocationCriteria.CenterLng,
		"radius":      criteriaProperty.LocationCriteria.Radius,
		"distance":    distance,
		"icon":        criteriaProperty.LocationCriteria.Icon,
		"color":       criteriaProperty.LocationCriteria.Color,
	}

	ctx.JSON(iris.Map{
		"success": true,
		"data":    response,
	})
}
