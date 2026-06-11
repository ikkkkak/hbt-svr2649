package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// GetOrganizationProfileSheet retrieves public organization profile data for ProfileSheet
// GET /api/organizations/{orgID}/profile-sheet
func GetOrganizationProfileSheet(ctx iris.Context) {
	orgIDStr := ctx.Params().Get("orgID")
	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid organization ID"})
		return
	}

	var org models.Organization

	// Get organization with owner info
	if err := storage.DB.Where("id = ?", uint(orgID)).
		Preload("Owner").
		First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.StatusCode(http.StatusNotFound)
			ctx.JSON(iris.Map{"error": "Organization not found"})
			return
		}
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Database error"})
		return
	}

	// Get follower count (placeholder - adjust based on actual follower model)
	var followerCount int64
	followerCount = 0 // TODO: Implement follower count from actual model

	// Check if current user is following (if authenticated)
	var isFollowing bool
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface != nil {
		userID, ok := userIDInterface.(uint)
		if ok && userID > 0 {
			// TODO: Check follower relationship from actual model
			isFollowing = false
		}
	}

	// Count total listings
	var totalListings int64
	storage.DB.Model(&models.PropertySale{}).
		Where("organization_id = ?", orgID).
		Count(&totalListings)

	response := iris.Map{
		"id":        org.ID,
		"name":      org.Name,
		"email":     org.Email,
		"avatarUrl": org.Logo,
		"bio":       org.Description,
		"stats": iris.Map{
			"totalListings": totalListings,
			"followers":     followerCount,
			"verified":      false, // TODO: Add verification field to Organization model
		},
		"isFollowing": isFollowing,
		"phone":       org.Phone,
		"website":     org.Website,
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}

// firstRentalImageURL parses the JSON images array on models.Property.
func firstRentalImageURL(imagesJSON string) string {
	s := strings.TrimSpace(imagesJSON)
	if s == "" {
		return ""
	}
	var urls []string
	if err := json.Unmarshal([]byte(s), &urls); err != nil {
		return ""
	}
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

// GetOrganizationPropertiesForSheet retrieves paginated properties for ProfileSheet
// GET /api/organizations/{orgID}/properties-sheet?page=1&limit=10&listing=sale|rent
func GetOrganizationPropertiesForSheet(ctx iris.Context) {
	orgIDStr := ctx.Params().Get("orgID")
	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid organization ID"})
		return
	}

	page, err := strconv.Atoi(ctx.URLParamDefault("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(ctx.URLParamDefault("limit", "10"))
	if err != nil || limit < 1 || limit > 50 {
		limit = 10
	}

	offset := (page - 1) * limit

	listing := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("listing", "sale")))
	if listing != "rent" {
		listing = "sale"
	}

	var propertiesData []iris.Map
	var total int64

	if listing == "rent" {
		var org models.Organization
		if err := storage.DB.Select("id", "owner_id").First(&org, uint(orgID)).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				ctx.StatusCode(http.StatusNotFound)
				ctx.JSON(iris.Map{"error": "Organization not found"})
				return
			}
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to load organization"})
			return
		}

		var agentUIDs []uint
		_ = storage.DB.Model(&models.Agent{}).
			Where("organization_id = ?", uint(orgID)).
			Pluck("user_id", &agentUIDs)

		q := storage.DB.Model(&models.Property{}).
			Where("COALESCE(properties.is_active, ?) = ?", true, true).
			Where("LOWER(properties.status) IN ?", []string{"approved", "live"})

		if len(agentUIDs) == 0 {
			q = q.Where("properties.host_id = ?", org.OwnerID)
		} else {
			q = q.Where("(properties.host_id = ? OR properties.host_id IN ?)", org.OwnerID, agentUIDs)
		}

		if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to count rental listings"})
			return
		}

		var rentals []models.Property
		if err := q.Order("properties.created_at DESC").Offset(offset).Limit(limit).Find(&rentals).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
			return
		}

		for _, prop := range rentals {
			thumb := firstRentalImageURL(prop.Images)
			loc := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(prop.City)+", "+strings.TrimSpace(prop.State), ", "))
			propertiesData = append(propertiesData, iris.Map{
				"id":           prop.ID,
				"title":        prop.Title,
				"price":        prop.NightlyPrice,
				"thumbnailUrl": thumb,
				"location":     loc,
				"bedrooms":     prop.Bedrooms,
				"bathrooms":    prop.Bathrooms,
				"listing":      "rent",
			})
		}
	} else {
		if err := storage.DB.Model(&models.PropertySale{}).
			Where("organization_id = ?", uint(orgID)).
			Count(&total).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to count listings"})
			return
		}

		var properties []models.PropertySale
		if err := storage.DB.
			Where("organization_id = ?", uint(orgID)).
			Order("created_at DESC").
			Offset(offset).
			Limit(limit).
			Find(&properties).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
			return
		}

		for _, prop := range properties {
			thumbnailURL := ""
			if len(prop.Images) > 0 {
				thumbnailURL = prop.Images[0]
			}

			propertiesData = append(propertiesData, iris.Map{
				"id":           prop.ID,
				"title":        prop.Title,
				"price":        prop.ListingPrice,
				"thumbnailUrl": thumbnailURL,
				"location":     fmt.Sprintf("%s, %s", prop.City, prop.State),
				"bedrooms":     prop.Bedrooms,
				"bathrooms":    prop.Bathrooms,
				"listing":      "sale",
			})
		}
	}

	totalPage := int64(1)
	if limit > 0 {
		totalPage = (total + int64(limit) - 1) / int64(limit)
	}
	if totalPage < 1 {
		totalPage = 1
	}

	response := iris.Map{
		"properties": propertiesData,
		"listing":    listing,
		"pagination": iris.Map{
			"page":      page,
			"limit":     limit,
			"total":     total,
			"totalPage": totalPage,
		},
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}

// GetOrganizationLandmarksForSheet retrieves nearby landmarks for ProfileSheet
// GET /api/organizations/{orgID}/landmarks-sheet
func GetOrganizationLandmarksForSheet(ctx iris.Context) {
	orgIDStr := ctx.Params().Get("orgID")
	_, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid organization ID"})
		return
	}

	var org models.Organization
	if err := storage.DB.Where("id = ?", orgIDStr).First(&org).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Organization not found"})
		return
	}

	// TODO: Implement actual landmark search based on organization location
	// For now, return empty landmarks list
	var landmarks []iris.Map

	response := iris.Map{
		"landmarks": landmarks,
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}

// ToggleOrganizationFollow toggles follow status for organization
// POST /api/organizations/{orgID}/follow
func ToggleOrganizationFollow(ctx iris.Context) {
	orgIDStr := ctx.Params().Get("orgID")
	_, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid organization ID"})
		return
	}

	// Get user ID from context (set by auth middleware)
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Not authenticated"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Invalid user context"})
		return
	}

	// TODO: Implement follow/unfollow logic with proper model
	// This requires a FollowRelation or similar model to exist
	// For now, return success with placeholder status

	response := iris.Map{
		"success":     true,
		"isFollowing": true, // Placeholder
		"timestamp":   time.Now().Unix(),
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}

// TogglePropertyLike toggles like status for property
// POST /api/properties/{propertyID}/like
func TogglePropertyLike(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("propertyID")
	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	// Get user ID from context (set by auth middleware)
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Not authenticated"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Invalid user context"})
		return
	}

	// Verify property exists
	var property models.PropertySale
	if err := storage.DB.Where("id = ?", uint(propertyID)).
		First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// TODO: Implement like/unlike logic with proper model
	// This requires a UserFavorite or PropertyLike model to exist
	// For now, return success with placeholder status

	response := iris.Map{
		"success":   true,
		"isLiked":   true, // Placeholder
		"timestamp": time.Now().Unix(),
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}
