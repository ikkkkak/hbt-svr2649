package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strings"

	"github.com/kataras/iris/v12"
)

// SearchProperties handles property search with multiple filters
func SearchProperties(ctx iris.Context) {
	q := storage.DB.Model(&models.Property{})

	// Text/location filters
	if city := strings.TrimSpace(ctx.URLParam("city")); city != "" {
		q = q.Where("LOWER(city) = LOWER(?)", city)
	}
	if state := strings.TrimSpace(ctx.URLParam("state")); state != "" {
		q = q.Where("LOWER(state) = LOWER(?)", state)
	}
	if country := strings.TrimSpace(ctx.URLParam("country")); country != "" {
		q = q.Where("LOWER(country) = LOWER(?)", country)
	}

	// Property attributes
	if pType := strings.TrimSpace(ctx.URLParam("propertyType")); pType != "" {
		q = q.Where("property_type = ?", pType)
	}
	if minPrice, err := ctx.URLParamInt("minPrice"); err == nil && minPrice > 0 {
		q = q.Where("nightly_price >= ?", minPrice)
	}
	if maxPrice, err := ctx.URLParamInt("maxPrice"); err == nil && maxPrice > 0 {
		q = q.Where("nightly_price <= ?", maxPrice)
	}
	if minBeds, err := ctx.URLParamInt("minBeds"); err == nil && minBeds > 0 {
		q = q.Where("beds >= ?", minBeds)
	}
	if minBedrooms, err := ctx.URLParamInt("minBedrooms"); err == nil && minBedrooms > 0 {
		q = q.Where("bedrooms >= ?", minBedrooms)
	}
	if minBathrooms, err := ctx.URLParamInt("minBathrooms"); err == nil && minBathrooms > 0 {
		q = q.Where("bathrooms >= ?", minBathrooms)
	}
	if minRating, err := ctx.URLParamInt("minRating"); err == nil && minRating > 0 {
		q = q.Where("rating >= ?", minRating)
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
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"message": "Failed to search properties"})
		return
	}

	ctx.JSON(properties)
}
