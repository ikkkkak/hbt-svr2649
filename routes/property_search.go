package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"os"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// SearchProperties handles property search with multiple filters
func SearchProperties(ctx iris.Context) {
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
	if state := strings.TrimSpace(ctx.URLParam("state")); state != "" {
		fmt.Printf("🔍 Applying state filter: %s\n", state)
		q = q.Where("LOWER(state) = LOWER(?)", state)
	}
	if country := strings.TrimSpace(ctx.URLParam("country")); country != "" {
		fmt.Printf("🔍 Applying country filter: %s\n", country)
		q = q.Where("LOWER(country) = LOWER(?)", country)
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

	// Property category filter
	if categoryID := strings.TrimSpace(ctx.URLParam("categoryId")); categoryID != "" {
		fmt.Printf("🔍 Applying category filter: %s\n", categoryID)
		q = q.Where("property_category_id = ?", categoryID)
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

	// Enforce only approved/live properties by default for safety
	status := strings.TrimSpace(ctx.URLParam("status"))
	if status == "" {
		q = q.Where("status IN (?)", []string{"approved", "live"})
	} else {
		// If status provided, still prevent unsafe values by intersecting
		// Only allow approved/live explicitly; others will return empty
		if strings.EqualFold(status, "approved") || strings.EqualFold(status, "live") {
			q = q.Where("status = ?", status)
		} else {
			q = q.Where("1 = 0") // block other statuses
		}
	}

	// Active flag additionally required
	q = q.Where("COALESCE(is_active, ?) = ?", true, true)

	// Debug: Log the final query conditions
	fmt.Printf("🔍 Final query conditions applied\n")

	// Sorting
	sort := strings.ToLower(strings.TrimSpace(ctx.URLParam("sort")))
	switch sort {
	case "price_low":
		q = q.Order("nightly_price ASC").Order("id DESC")
	case "price_high":
		q = q.Order("nightly_price DESC").Order("id DESC")
	case "rating":
		q = q.Order("rating DESC").Order("id DESC")
	default:
		q = q.Order("created_at DESC")
	}

	var properties []models.Property
	if err := q.Find(&properties).Error; err != nil {
		fmt.Printf("❌ Database query error: %v\n", err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to search properties"})
		return
	}

	fmt.Printf("✅ Found %d properties\n", len(properties))

	// Debug: Show sample property data to understand the structure
	if len(properties) > 0 {
		fmt.Printf("🔍 Sample property data:\n")
		for i, prop := range properties {
			if i >= 3 { // Show first 3 properties
				break
			}
			fmt.Printf("  Property %d: ID=%d, Type=%s, Price=%d, Beds=%d, Baths=%d, City=%s\n",
				i+1, prop.ID, prop.PropertyType, prop.NightlyPrice, prop.Bedrooms, prop.Bathrooms, prop.City)
		}
	} else {
		fmt.Printf("🔍 No properties found - checking database structure...\n")
		// Let's check what property types exist
		var propertyTypes []string
		storage.DB.Model(&models.Property{}).Distinct("property_type").Pluck("property_type", &propertyTypes)
		fmt.Printf("🔍 Available property types in database: %v\n", propertyTypes)

		// Check price ranges
		var minPrice, maxPrice int
		storage.DB.Model(&models.Property{}).Select("MIN(nightly_price)").Scan(&minPrice)
		storage.DB.Model(&models.Property{}).Select("MAX(nightly_price)").Scan(&maxPrice)
		fmt.Printf("🔍 Price range in database: %d - %d\n", minPrice, maxPrice)

		// Check bedroom ranges
		var minBeds, maxBeds int
		storage.DB.Model(&models.Property{}).Select("MIN(bedrooms)").Scan(&minBeds)
		storage.DB.Model(&models.Property{}).Select("MAX(bedrooms)").Scan(&maxBeds)
		fmt.Printf("🔍 Bedroom range in database: %d - %d\n", minBeds, maxBeds)

		// Check bathroom ranges
		var minBaths, maxBaths int
		storage.DB.Model(&models.Property{}).Select("MIN(bathrooms)").Scan(&minBaths)
		storage.DB.Model(&models.Property{}).Select("MAX(bathrooms)").Scan(&maxBaths)
		fmt.Printf("🔍 Bathroom range in database: %d - %d\n", minBaths, maxBaths)

		// Check how many properties match the current filters
		var count int64
		testQuery := storage.DB.Model(&models.Property{})
		testQuery = testQuery.Where("status IN (?)", []string{"approved", "live"})
		testQuery = testQuery.Where("COALESCE(is_active, ?) = ?", true, true)
		testQuery.Count(&count)
		fmt.Printf("🔍 Total active properties in database: %d\n", count)

		// Test with just property type filter
		if pType := strings.TrimSpace(ctx.URLParam("propertyType")); pType != "" {
			var typeCount int64
			storage.DB.Model(&models.Property{}).Where("property_type = ? AND status IN (?) AND COALESCE(is_active, ?) = ?", pType, []string{"approved", "live"}, true, true).Count(&typeCount)
			fmt.Printf("🔍 Properties with type '%s': %d\n", pType, typeCount)
		}
	}

	ctx.JSON(properties)
}
