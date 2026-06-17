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

func userDisplayName(u models.User) string {
	name := strings.TrimSpace(fmt.Sprintf("%s %s", u.FirstName, u.LastName))
	if name != "" {
		return name
	}
	return strings.TrimSpace(u.Email)
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

	var followerCount int64
	var isFollowing bool
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface != nil {
		userID, ok := userIDInterface.(uint)
		if ok && userID > 0 {
			isFollowing = false
		}
	}

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
			"verified":      false,
		},
		"isFollowing": isFollowing,
		"phone":       org.Phone,
		"website":     org.Website,
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}

// GetUserProfileSheet retrieves public user profile data for ProfileSheet.
// GET /api/users/{userID}/profile-sheet
func GetUserProfileSheet(ctx iris.Context) {
	userIDStr := ctx.Params().Get("userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	var user models.User
	if err := storage.DB.First(&user, uint(userID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.StatusCode(http.StatusNotFound)
			ctx.JSON(iris.Map{"error": "User not found"})
			return
		}
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Database error"})
		return
	}

	var saleCount int64
	storage.DB.Model(&models.PropertySale{}).
		Where("owner_id = ? AND (organization_id IS NULL OR organization_id = 0)", uint(userID)).
		Count(&saleCount)

	var rentCount int64
	storage.DB.Model(&models.Property{}).
		Where("host_id = ?", uint(userID)).
		Where("COALESCE(properties.is_active, ?) = ?", true, true).
		Where("LOWER(properties.status) IN ?", []string{"approved", "live"}).
		Count(&rentCount)

	response := iris.Map{
		"id":        user.ID,
		"name":      userDisplayName(user),
		"email":     user.Email,
		"avatarUrl": user.AvatarURL,
		"bio":       user.Bio,
		"stats": iris.Map{
			"totalListings": saleCount + rentCount,
			"followers":     0,
			"verified":      user.TrueBroker,
		},
		"isFollowing": false,
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
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

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{
		"properties": propertiesData,
		"listing":    listing,
		"pagination": iris.Map{
			"page":      page,
			"limit":     limit,
			"total":     total,
			"totalPage": totalPage,
		},
	})
}

// GetUserPropertiesForSheet retrieves paginated properties for an individual user.
// GET /api/users/{userID}/properties-sheet?page=1&limit=10&listing=sale|rent
func GetUserPropertiesForSheet(ctx iris.Context) {
	userIDStr := ctx.Params().Get("userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
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
		q := storage.DB.Model(&models.Property{}).
			Where("host_id = ?", uint(userID)).
			Where("COALESCE(properties.is_active, ?) = ?", true, true).
			Where("LOWER(properties.status) IN ?", []string{"approved", "live"})

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
		q := storage.DB.Model(&models.PropertySale{}).
			Where("owner_id = ? AND (organization_id IS NULL OR organization_id = 0)", uint(userID))

		if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to count listings"})
			return
		}

		var properties []models.PropertySale
		if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&properties).Error; err != nil {
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

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{
		"properties": propertiesData,
		"listing":    listing,
		"pagination": iris.Map{
			"page":      page,
			"limit":     limit,
			"total":     total,
			"totalPage": totalPage,
		},
	})
}

// GetOrganizationLandmarksForSheet retrieves landmarks for an organization ProfileSheet
// GET /api/organizations/{orgID}/landmarks-sheet
func GetOrganizationLandmarksForSheet(ctx iris.Context) {
	orgIDStr := ctx.Params().Get("orgID")
	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid organization ID"})
		return
	}

	var org models.Organization
	if err := storage.DB.Where("id = ?", uint(orgID)).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.StatusCode(http.StatusNotFound)
			ctx.JSON(iris.Map{"error": "Organization not found"})
			return
		}
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Database error"})
		return
	}

	var landmarks []models.Landmark
	if err := storage.DB.
		Where("organization_id = ?", uint(orgID)).
		Order("created_at DESC").
		Limit(50).
		Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	landmarksData := make([]iris.Map, 0, len(landmarks))
	for _, lm := range landmarks {
		thumb := firstLandmarkImageURL(lm)
		title := strings.TrimSpace(lm.Title)
		landmarksData = append(landmarksData, iris.Map{
			"id":           lm.ID,
			"name":         title,
			"title":        title,
			"thumbnailUrl": thumb,
		})
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{"landmarks": landmarksData})
}

// GetUserLandmarksForSheet retrieves landmarks owned by an individual user.
// GET /api/users/{userID}/landmarks-sheet
func GetUserLandmarksForSheet(ctx iris.Context) {
	userIDStr := ctx.Params().Get("userID")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	var landmarks []models.Landmark
	if err := storage.DB.
		Where("owner_id = ? AND (organization_id IS NULL OR organization_id = 0)", uint(userID)).
		Order("created_at DESC").
		Limit(50).
		Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	landmarksData := make([]iris.Map, 0, len(landmarks))
	for _, lm := range landmarks {
		thumb := firstLandmarkImageURL(lm)
		title := strings.TrimSpace(lm.Title)
		landmarksData = append(landmarksData, iris.Map{
			"id":           lm.ID,
			"name":         title,
			"title":        title,
			"thumbnailUrl": thumb,
		})
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{"landmarks": landmarksData})
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

	response := iris.Map{
		"success":     true,
		"isFollowing": true,
		"timestamp":   time.Now().Unix(),
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}

// TogglePropertyLike toggles like status for property
// PATCH /api/properties/{propertyID}/like
func TogglePropertyLike(ctx iris.Context) {
	propertyIDStr := ctx.Params().Get("propertyID")
	propertyID, err := strconv.ParseUint(propertyIDStr, 10, 32)
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

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

	var property models.PropertySale
	if err := storage.DB.Where("id = ?", uint(propertyID)).
		First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	response := iris.Map{
		"success":   true,
		"isLiked":   true,
		"timestamp": time.Now().Unix(),
	}

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(response)
}
