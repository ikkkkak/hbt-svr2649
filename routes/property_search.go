package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// parsePublicRentStatusParam — comma-separated status values (e.g. approved,published).
func parsePublicRentStatusParam(status string) []string {
	defaults := []string{"approved", "published"}
	if status == "" {
		return defaults
	}
	parts := strings.Split(status, ",")
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
		return defaults
	}
	return out
}

// SearchProperties handles property search with multiple filters
// Works for both authenticated and unauthenticated users
// Properties are rotated per-request to show fresh content (TikTok-like cycling)
// Properties with images are always prioritized at the top
func SearchProperties(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Debug: Log all received parameters
	fmt.Printf("🔍 SearchProperties called with parameters:\n")
	fmt.Printf("  city: %s\n", ctx.URLParam("city"))
	fmt.Printf("  propertyType: %s\n", ctx.URLParam("propertyType"))
	fmt.Printf("  type: %s\n", ctx.URLParam("type"))
	fmt.Printf("  categoryId: %s\n", ctx.URLParam("categoryId"))
	fmt.Printf("  minPrice: %s\n", ctx.URLParam("minPrice"))
	fmt.Printf("  maxPrice: %s\n", ctx.URLParam("maxPrice"))
	fmt.Printf("  minBedrooms: %s\n", ctx.URLParam("minBedrooms"))
	fmt.Printf("  minBathrooms: %s\n", ctx.URLParam("minBathrooms"))
	fmt.Printf("  amenities: %s\n", ctx.URLParam("amenities"))
	fmt.Printf("  Full URL: %s\n", ctx.Request().URL.String())

	q := storage.DB.Model(&models.Property{})

	// Exclusions based on user moderation (optional auth)
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
	if userID > 0 {
		// Use NOT EXISTS to allow index-only lookups and avoid IN list pitfalls
		q = q.Where("NOT EXISTS (SELECT 1 FROM hidden_properties hp WHERE hp.property_id = properties.id AND hp.user_id = ?)", userID)
		q = q.Where("NOT EXISTS (SELECT 1 FROM property_reports pr WHERE pr.property_id = properties.id AND pr.reporter_id = ?)", userID)
		q = q.Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagged_user_id = properties.host_id AND uf.flagger_id = ? AND uf.status='active')", userID)
	}

	// Text/location filters
	if city := strings.TrimSpace(ctx.URLParam("city")); city != "" {
		fmt.Printf("🔍 Applying city filter: %s\n", city)
		q = q.Where("LOWER(city) = LOWER(?)", city)
	}
	if v := strings.TrimSpace(ctx.URLParam("city_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			q = q.Where("properties.city_id = ?", uint(n))
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("zone_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			q = q.Where("properties.zone_id = ?", uint(n))
		}
	}
	if v := strings.TrimSpace(ctx.URLParam("quartier_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			q = q.Where("properties.quartier_id = ?", uint(n))
		}
	}
	if state := strings.TrimSpace(ctx.URLParam("state")); state != "" {
		fmt.Printf("🔍 Applying state filter: %s\n", state)
		q = q.Where("LOWER(state) = LOWER(?)", state)
	}
	if country := strings.TrimSpace(ctx.URLParam("country")); country != "" {
		fmt.Printf("🔍 Applying country filter: %s\n", country)
		q = q.Where("LOWER(country) = LOWER(?)", country)
	}
	if v := strings.TrimSpace(ctx.URLParam("country_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			q = q.Where("properties.country_id = ?", uint(n))
		}
	}

	// Property attributes
	// Support both propertyType and type as aliases
	pTypeParam := strings.TrimSpace(ctx.URLParam("propertyType"))
	if pTypeParam == "" {
		pTypeParam = strings.TrimSpace(ctx.URLParam("type"))
	}
	if pTypeParam != "" {
		fmt.Printf("🔍 Applying property type filter: %s\n", pTypeParam)
		q = q.Where("property_type = ?", pTypeParam)
	}

	// Property category filter (column + legacy property_categories join)
	categoryID := strings.TrimSpace(ctx.URLParam("categoryId"))
	if categoryID == "" {
		categoryID = strings.TrimSpace(ctx.URLParam("category_id"))
	}
	if categoryID == "" {
		categoryID = strings.TrimSpace(ctx.URLParam("property_category_id"))
	}
	if categoryID != "" {
		fmt.Printf("🔍 Applying category filter: %s\n", categoryID)
		q = q.Where(
			"properties.property_category_id = ? OR properties.id IN (SELECT property_id FROM property_categories WHERE category_id = ?)",
			categoryID, categoryID,
		)
	}
	if minPrice, err := ctx.URLParamInt("minPrice"); err == nil && minPrice > 0 {
		fmt.Printf("🔍 Applying min price filter: %d\n", minPrice)
		q = q.Where("nightly_price >= ?", minPrice)
	}
	if maxPrice, err := ctx.URLParamInt("maxPrice"); err == nil && maxPrice > 0 {
		fmt.Printf("🔍 Applying max price filter: %d\n", maxPrice)
		q = q.Where("nightly_price <= ?", maxPrice)
	}
	if minBeds, err := ctx.URLParamInt("minBeds"); err == nil && minBeds > 0 {
		fmt.Printf("🔍 Applying min beds filter: %d\n", minBeds)
		q = q.Where("beds >= ?", minBeds)
	}
	if minBedrooms, err := ctx.URLParamInt("minBedrooms"); err == nil && minBedrooms > 0 {
		fmt.Printf("🔍 Applying min bedrooms filter: %d\n", minBedrooms)
		q = q.Where("bedrooms >= ?", minBedrooms)
	}
	if minBathrooms, err := ctx.URLParamInt("minBathrooms"); err == nil && minBathrooms > 0 {
		fmt.Printf("🔍 Applying min bathrooms filter: %d\n", minBathrooms)
		q = q.Where("bathrooms >= ?", minBathrooms)
	}
	// Optional minimum year built
	if minYearBuilt, err := ctx.URLParamInt("minYearBuilt"); err == nil && minYearBuilt > 0 {
		fmt.Printf("🔍 Applying min year built filter: %d\n", minYearBuilt)
		q = q.Where("year_built >= ?", minYearBuilt)
	}
	if minRating, err := ctx.URLParamInt("minRating"); err == nil && minRating > 0 {
		fmt.Printf("🔍 Applying min rating filter: %d\n", minRating)
		q = q.Where("rating >= ?", minRating)
	}

	// Amenities filter - check if property has specific amenities
	if amenities := strings.TrimSpace(ctx.URLParam("amenities")); amenities != "" {
		fmt.Printf("🔍 Applying amenities filter: %s\n", amenities)
		// Split comma-separated amenities
		amenityList := strings.Split(amenities, ",")
		for _, amenity := range amenityList {
			amenity = strings.TrimSpace(amenity)
			if amenity != "" {
				// Use JSON contains to check if amenity exists in the JSON array
				q = q.Where("JSON_CONTAINS(amenities, ?)", fmt.Sprintf(`"%s"`, amenity))
			}
		}
	}

	// Public rent search: never return rejected/pending/draft or flagged listings.
	q = q.Where("LOWER(TRIM(COALESCE(status, ''))) NOT IN (?)",
		[]string{"rejected", "pending", "draft", "denied", "cancelled", "canceled", "suspended", "inactive", "blocked"})
	q = q.Where("COALESCE(is_flagged, false) = ?", false)

	allowedStatuses := parsePublicRentStatusParam(strings.TrimSpace(ctx.URLParam("status")))
	q = q.Where("LOWER(TRIM(COALESCE(status, ''))) IN ?", allowedStatuses)

	// Active flag additionally required
	q = q.Where("COALESCE(is_active, ?) = ?", true, true)

	// Debug: Log the final query conditions
	fmt.Printf("🔍 Final query conditions applied\n")

	// Get all properties first for rotation and image prioritization
	var allProperties []models.Property
	if err := q.Find(&allProperties).Error; err != nil {
		fmt.Printf("❌ Database query error: %v\n", err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to search properties"})
		return
	}

	fmt.Printf("✅ Found %d properties before rotation\n", len(allProperties))

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

	// Apply time-based rotation for TikTok-like cycling
	// Rotation changes per-request with high precision + random component for maximum variety
	// This ensures users see different properties on each visit/reload
	now := time.Now()
	// High-precision rotation: includes seconds, nanoseconds, Unix timestamp, and random component
	// Random component ensures different results even if requests happen at exact same time
	rand.Seed(now.UnixNano()) // Seed random with current nanosecond for true randomness
	randomComponent := int64(rand.Intn(10000)) // Random 0-9999
	rotationSeed := int64(now.Second()) + 
		int64(now.Minute())*60 + 
		int64(now.Hour())*3600 + 
		int64(now.Day())*86400 + 
		int64(now.Month())*2678400 + 
		(now.UnixNano() % 1000000) + // Use nanoseconds for microsecond-level variation
		randomComponent // Add random component for guaranteed variation
	
	// Rotate both lists with different offsets for maximum variety
	// Uses multiple rotation passes for better distribution
	rotateProperties := func(props []models.Property, seed int64) []models.Property {
		if len(props) == 0 {
			return props
		}
		if len(props) == 1 {
			return props
		}
		
		// Calculate offset with multiple components for better distribution
		offset1 := int(seed % int64(len(props)))
		offset2 := int((seed * 7) % int64(len(props))) // Different multiplier for variety
		
		// Use the larger offset, but ensure it's not 0
		offset := offset1
		if offset2 > offset1 {
			offset = offset2
		}
		if offset == 0 {
			offset = int((seed * 13) % int64(len(props)-1)) + 1 // Force non-zero with different multiplier
		}
		
		// Perform rotation
		rotated := make([]models.Property, len(props))
		copy(rotated, props[offset:])
		copy(rotated[len(props)-offset:], props[:offset])
		return rotated
	}

	withImages = rotateProperties(withImages, rotationSeed)
	withoutImages = rotateProperties(withoutImages, rotationSeed+5000) // Different offset for variety

	// Apply custom sorting if requested (but still prioritize images)
	sort := strings.ToLower(strings.TrimSpace(ctx.URLParam("sort")))
	if sort != "" {
		switch sort {
		case "price_low":
			// Sort within each group (with/without images)
			sortByPriceLow := func(props []models.Property) {
				for i := 0; i < len(props)-1; i++ {
					for j := i + 1; j < len(props); j++ {
						if props[i].NightlyPrice > props[j].NightlyPrice {
							props[i], props[j] = props[j], props[i]
						}
					}
				}
			}
			sortByPriceLow(withImages)
			sortByPriceLow(withoutImages)
		case "price_high":
			sortByPriceHigh := func(props []models.Property) {
				for i := 0; i < len(props)-1; i++ {
					for j := i + 1; j < len(props); j++ {
						if props[i].NightlyPrice < props[j].NightlyPrice {
							props[i], props[j] = props[j], props[i]
						}
					}
				}
			}
			sortByPriceHigh(withImages)
			sortByPriceHigh(withoutImages)
		case "rating":
			sortByRating := func(props []models.Property) {
				for i := 0; i < len(props)-1; i++ {
					for j := i + 1; j < len(props); j++ {
						if props[i].Rating < props[j].Rating {
							props[i], props[j] = props[j], props[i]
						}
					}
				}
			}
			sortByRating(withImages)
			sortByRating(withoutImages)
		}
	}

	// Combine: properties with images first, then without
	var properties []models.Property
	properties = append(properties, withImages...)
	properties = append(properties, withoutImages...)

	total := len(properties)
	offset := (page - 1) * limit
	if offset >= total {
		properties = []models.Property{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		properties = properties[offset:end]
	}

	fmt.Printf("✅ Returning %d properties (rotated, images prioritized, page=%d, limit=%d, total=%d)\n", len(properties), page, limit, total)

	// Debug: Show sample property data to understand the structure
	if len(properties) > 0 {
		fmt.Printf("🔍 Sample property data:\n")
		for i, prop := range properties {
			if i >= 3 { // Show first 3 properties
				break
			}
			fmt.Printf("  Property %d: ID=%d, Type=%s, Price=%.0f, Beds=%d, Baths=%.0f, City=%s\n",
				i+1, prop.ID, prop.PropertyType, float64(prop.NightlyPrice), prop.Bedrooms, float64(prop.Bathrooms), prop.City)
		}
	}

	ctx.Header("X-Page", fmt.Sprintf("%d", page))
	ctx.Header("X-Limit", fmt.Sprintf("%d", limit))
	ctx.Header("X-Total", fmt.Sprintf("%d", total))
	ctx.JSON(properties)
}

// ListProperties handles GET /api/properties - returns paginated approved properties
// Format: { data: Property[], meta: { page, limit, total } }
func ListProperties(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page < 1 {
		page = 1
	}

	// Exclusions based on user moderation (optional auth)
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

	q := storage.DB.Model(&models.Property{})

	// Apply user-specific exclusions if authenticated
	if userID > 0 {
		// Use NOT EXISTS to allow index-only lookups and avoid IN list pitfalls
		q = q.Where("NOT EXISTS (SELECT 1 FROM hidden_properties hp WHERE hp.property_id = properties.id AND hp.user_id = ?)", userID)
		q = q.Where("NOT EXISTS (SELECT 1 FROM property_reports pr WHERE pr.property_id = properties.id AND pr.reporter_id = ?)", userID)
		q = q.Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagged_user_id = properties.host_id AND uf.flagger_id = ? AND uf.status='active')", userID)
	}

	// Only show approved/live properties (case-insensitive)
	q = q.Where("LOWER(status) IN (?)", []string{"approved", "live", "published"})
	
	// Active flag additionally required
	q = q.Where("COALESCE(is_active, ?) = ?", true, true)

	// Count total matching properties
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fmt.Printf("❌ Error counting properties: %v\n", err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to count properties"})
		return
	}

	// Fetch paginated properties
	var properties []models.Property
	offset := (page - 1) * limit
	if err := q.Preload("Host").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&properties).Error; err != nil {
		fmt.Printf("❌ Error fetching properties: %v\n", err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	fmt.Printf("✅ ListProperties: page=%d, limit=%d, total=%d, returned=%d\n", page, limit, total, len(properties))

	// Return in the format expected by frontend: { data: Property[], meta: { page, limit, total } }
	ctx.JSON(iris.Map{
		"data": properties,
		"meta": iris.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}
