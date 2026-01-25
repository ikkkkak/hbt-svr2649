package routes

import (
	"apartments-clone-server/services"
	"apartments-clone-server/utils"
	"strconv"

	"github.com/kataras/iris/v12"
)

// GetRecommendedFeed returns a personalized video/property_sale feed (TikTok-style).
func GetRecommendedFeed(ctx iris.Context) {
	var userID *uint
	var deviceID *string
	if u, ok := ctx.Values().Get("userID").(uint); ok && u > 0 {
		userID = &u
	}
	if d := ctx.URLParam("device_id"); d != "" {
		deviceID = &d
	}

	limit := ctx.URLParamIntDefault("limit", 10)
	cursor := ctx.URLParam("cursor")

	svc := services.NewRecommendationService()
	items, nextCursor, hasMore, err := svc.GetFeed(services.GetFeedOptions{
		UserID:   userID,
		DeviceID: deviceID,
		Limit:    limit,
		Cursor:   cursor,
	})
	if err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":     true,
		"items":       items,
		"nextCursor":  nextCursor,
		"hasMore":     hasMore,
	})
}

// GetSuggestedProperties returns properties similar to those the user has viewed.
func GetSuggestedProperties(ctx iris.Context) {
	var userID *uint
	var deviceID *string
	if u, ok := ctx.Values().Get("userID").(uint); ok && u > 0 {
		userID = &u
	}
	if d := ctx.URLParam("device_id"); d != "" {
		deviceID = &d
	}
	limit := ctx.URLParamIntDefault("limit", 5)

	svc := services.NewRecommendationService()
	sales, props, err := svc.GetSuggestedProperties(userID, deviceID, limit)
	if err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":          true,
		"propertySales":    sales,
		"properties":       props,
	})
}

// GetSuggestedVideosForProperty returns other videos for the same property.
func GetSuggestedVideosForProperty(ctx iris.Context) {
	kind := ctx.URLParamDefault("property_kind", "rent") // rent | sale
	limit := ctx.URLParamIntDefault("limit", 5)

	var propertyID, propertySaleID uint
	if kind == "rent" {
		propertyID, _ = ctx.Params().GetUint("propertyId")
		if propertyID == 0 {
			if v := ctx.URLParam("property_id"); v != "" {
				u, _ := strconv.ParseUint(v, 10, 0)
				propertyID = uint(u)
			}
		}
	} else {
		propertySaleID, _ = ctx.Params().GetUint("propertySaleId")
		if propertySaleID == 0 {
			if v := ctx.URLParam("property_sale_id"); v != "" {
				u, _ := strconv.ParseUint(v, 10, 0)
				propertySaleID = uint(u)
			}
		}
	}

	svc := services.NewRecommendationService()
	rent, sale, saleListings := svc.GetSuggestedVideosForProperty(kind, propertyID, propertySaleID, limit)

	ctx.JSON(iris.Map{
		"success":        true,
		"videos":         rent,
		"saleVideos":     sale,
		"saleListings":   saleListings,
	})
}
