package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"strconv"

	"github.com/kataras/iris/v12"
)

// Get properties near a specific location
func GetPropertiesNearLocation(ctx iris.Context) {
	locationKey := ctx.Params().Get("location")
	limitStr := ctx.URLParamDefault("limit", "8")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 8
	}

	// Get location info
	location, exists := services.GetLocationInfo(locationKey)
	if !exists {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Location not found",
		})
		return
	}

	// Get all active properties
	var properties []models.Property
	result := storage.DB.Where("is_active = ?", true).Find(&properties)
	if result.Error != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Failed to fetch properties",
		})
		return
	}

	// Filter properties near the location
	nearbyProperties := services.GetPropertiesNearLocation(properties, locationKey)

	// Limit results
	if len(nearbyProperties) > limit {
		nearbyProperties = nearbyProperties[:limit]
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"properties": nearbyProperties,
		"location":   location,
		"count":      len(nearbyProperties),
	})
}

// Get all available locations
func GetAvailableLocations(ctx iris.Context) {
	locations := make(map[string]services.Location)
	for key, location := range services.MauritaniaLocations {
		locations[key] = location
	}

	ctx.JSON(iris.Map{
		"success":   true,
		"locations": locations,
	})
}

// Get properties by coordinates with radius
func GetPropertiesByCoordinates(ctx iris.Context) {
	latStr := ctx.URLParam("lat")
	lngStr := ctx.URLParam("lng")
	radiusStr := ctx.URLParamDefault("radius", "5")
	limitStr := ctx.URLParamDefault("limit", "20")

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Invalid latitude",
		})
		return
	}

	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Invalid longitude",
		})
		return
	}

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil {
		radius = 5.0
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 20
	}

	// Get all active properties
	var properties []models.Property
	result := storage.DB.Where("is_active = ?", true).Find(&properties)
	if result.Error != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Failed to fetch properties",
		})
		return
	}

	// Filter properties within radius
	var nearbyProperties []models.Property
	for _, property := range properties {
		distance := services.CalculateDistance(
			float64(property.Lat),
			float64(property.Lng),
			lat,
			lng,
		)
		if distance <= radius {
			nearbyProperties = append(nearbyProperties, property)
		}
	}

	// Limit results
	if len(nearbyProperties) > limit {
		nearbyProperties = nearbyProperties[:limit]
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"properties": nearbyProperties,
		"count":      len(nearbyProperties),
		"center": iris.Map{
			"lat": lat,
			"lng": lng,
		},
		"radius": radius,
	})
}

// Get properties with advanced filtering
func GetPropertiesWithFilters(ctx iris.Context) {
	// Parse query parameters
	latStr := ctx.URLParam("lat")
	lngStr := ctx.URLParam("lng")
	radiusStr := ctx.URLParamDefault("radius", "10")
	limitStr := ctx.URLParamDefault("limit", "20")
	propertyType := ctx.URLParam("property_type")
	minPriceStr := ctx.URLParam("min_price")
	maxPriceStr := ctx.URLParam("max_price")
	bedroomsStr := ctx.URLParam("bedrooms")
	bathroomsStr := ctx.URLParam("bathrooms")
	amenitiesStr := ctx.URLParam("amenities")

	// Parse coordinates
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Invalid latitude",
		})
		return
	}

	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Invalid longitude",
		})
		return
	}

	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil {
		radius = 10.0
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 20
	}

	// Build query
	query := storage.DB.Where("is_active = ?", true)

	// Property type filter
	if propertyType != "" {
		query = query.Where("property_type = ?", propertyType)
	}

	// Price filters
	if minPriceStr != "" {
		if minPrice, err := strconv.ParseFloat(minPriceStr, 64); err == nil {
			query = query.Where("nightly_price >= ?", minPrice)
		}
	}
	if maxPriceStr != "" {
		if maxPrice, err := strconv.ParseFloat(maxPriceStr, 64); err == nil {
			query = query.Where("nightly_price <= ?", maxPrice)
		}
	}

	// Bedrooms filter
	if bedroomsStr != "" {
		if bedrooms, err := strconv.Atoi(bedroomsStr); err == nil {
			query = query.Where("bedrooms >= ?", bedrooms)
		}
	}

	// Bathrooms filter
	if bathroomsStr != "" {
		if bathrooms, err := strconv.ParseFloat(bathroomsStr, 64); err == nil {
			query = query.Where("bathrooms >= ?", bathrooms)
		}
	}

	// Execute query
	var properties []models.Property
	result := query.Find(&properties)
	if result.Error != nil {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "Failed to fetch properties",
		})
		return
	}

	// Filter by distance
	var nearbyProperties []models.Property
	for _, property := range properties {
		distance := services.CalculateDistance(
			float64(property.Lat),
			float64(property.Lng),
			lat,
			lng,
		)
		if distance <= radius {
			nearbyProperties = append(nearbyProperties, property)
		}
	}

	// Filter by amenities if provided
	if amenitiesStr != "" {
		var requestedAmenities []string
		if err := json.Unmarshal([]byte(amenitiesStr), &requestedAmenities); err == nil {
			var filteredProperties []models.Property
			for _, property := range nearbyProperties {
				var propertyAmenities []string
				if property.Amenities != "" {
					json.Unmarshal([]byte(property.Amenities), &propertyAmenities)
				}

				// Check if property has all requested amenities
				hasAllAmenities := true
				for _, requested := range requestedAmenities {
					found := false
					for _, propertyAmenity := range propertyAmenities {
						if propertyAmenity == requested {
							found = true
							break
						}
					}
					if !found {
						hasAllAmenities = false
						break
					}
				}

				if hasAllAmenities {
					filteredProperties = append(filteredProperties, property)
				}
			}
			nearbyProperties = filteredProperties
		}
	}

	// Limit results
	if len(nearbyProperties) > limit {
		nearbyProperties = nearbyProperties[:limit]
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"properties": nearbyProperties,
		"count":      len(nearbyProperties),
		"filters": iris.Map{
			"center": iris.Map{
				"lat": lat,
				"lng": lng,
			},
			"radius":        radius,
			"property_type": propertyType,
			"min_price":     minPriceStr,
			"max_price":     maxPriceStr,
			"bedrooms":      bedroomsStr,
			"bathrooms":     bathroomsStr,
			"amenities":     amenitiesStr,
		},
	})
}
