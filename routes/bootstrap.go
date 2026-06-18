package routes

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
)

func optionalUserIDFromContext(ctx iris.Context) uint {
	if v, ok := ctx.Values().Get("userID").(uint); ok && v > 0 {
		return v
	}
	return 0
}

func bootstrapUnreadDirectMessages(userID uint) int64 {
	if userID == 0 {
		return 0
	}
	var n int64
	_ = storage.DB.Model(&models.DirectMessage{}).
		Where("receiver_id = ? AND is_read = ? AND deleted_at IS NULL", userID, false).
		Count(&n).Error
	return n
}

func bootstrapUserSummary(userID uint) iris.Map {
	var u models.User
	if err := storage.DB.Select(
		"id", "first_name", "last_name", "email", "phone_number",
		"avatar_url", "role", "allows_notifications",
	).First(&u, userID).Error; err != nil {
		return nil
	}
	return iris.Map{
		"ID":                  u.ID,
		"firstName":           u.FirstName,
		"lastName":            u.LastName,
		"email":               u.Email,
		"phoneNumber":         u.PhoneNumber,
		"avatarURL":           u.AvatarURL,
		"role":                u.Role,
		"allowsNotifications": u.AllowsNotifications,
	}
}

func bootstrapSaleVideoFeedFromCache(page, limit int) (iris.Map, bool) {
	bgCtx := context.Background()
	cacheSvc := services.NewCacheService(storage.Redis)
	cacheKey := services.FormatKey(services.PropertySaleVideoFeedKey, page, limit)
	var cached struct {
		Videos     []map[string]interface{} `json:"videos"`
		NextCursor string                   `json:"nextCursor"`
		HasMore    bool                     `json:"hasMore"`
	}
	if err := cacheSvc.Get(bgCtx, cacheKey, &cached); err != nil || len(cached.Videos) == 0 {
		return nil, false
	}
	return iris.Map{
		"videos":     cached.Videos,
		"nextCursor": cached.NextCursor,
		"hasMore":    cached.HasMore,
		"source":     "cache",
	}, true
}

func bootstrapPropertySalesFromCache(limit int, lang string) (iris.Map, bool) {
	bgCtx := context.Background()
	cacheSvc := services.NewCacheService(storage.Redis)
	cacheKey := services.FormatKey(services.PropertySalesSmartFeedAnonKey, limit, lang)
	var cached iris.Map
	if err := cacheSvc.Get(bgCtx, cacheKey, &cached); err != nil || cached == nil {
		return nil, false
	}
	if props, ok := cached["properties"].([]interface{}); ok && len(props) > 0 {
		cached["source"] = "cache"
		return cached, true
	}
	return nil, false
}

func bootstrapBuildPropertySales(userID uint, deviceID string, page, limit int, lang string) iris.Map {
	if feed, ok := bootstrapPropertySalesFromCache(limit, lang); ok && userID == 0 {
		return feed
	}

	q := storage.DB.Model(&models.PropertySale{}).
		Where("property_sales.status = ? OR property_sales.is_published = ?", "published", true).
		Where("(property_sales.is_deactivated = ? OR property_sales.is_sold = ?)", false, true).
		Where("property_sales.deleted_at IS NULL").
		Omit(
			"Description", "DescriptionTranslations",
			"FloorPlans", "Neighborhood",
			"Features", "Amenities", "HostPrivateNote", "VerificationNotes", "VirtualTour",
		).
		Preload("Organization", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name", "phone", "email", "website", "banner_image", "logo", "owner_id")
		}).
		Preload("Owner", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "first_name", "last_name", "avatar_url", "phone_number")
		}).
		Preload("Agent", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "user_id", "organization_id")
		})

	if userID > 0 {
		q = q.Joins("LEFT JOIN agents ON agents.id = property_sales.agent_id").
			Joins("LEFT JOIN organizations ON organizations.id = property_sales.organization_id").
			Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagger_id = ? AND uf.status = 'active' AND (uf.flagged_user_id = agents.user_id OR uf.flagged_user_id = organizations.owner_id))", userID).
			Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID).
			Where("NOT EXISTS (SELECT 1 FROM user_blocked_organizations ubo WHERE ubo.user_id = ? AND ubo.organization_id = property_sales.organization_id AND ubo.status = 'active')", userID)
	}

	properties, totalCount, hasMore, nextCursor := buildSmartPropertyFeedPage(q, userID, deviceID, page, limit)
	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
	}
	redactPropertySaleSliceForViewer(properties, userID)
	expandPropertySaleGalleries(properties)
	applyCardFeedTrim(properties)

	out := iris.Map{
		"data":       properties,
		"properties": properties,
		"hasMore":    hasMore,
		"nextCursor": nextCursor,
		"meta":       iris.Map{"total": totalCount, "page": page, "limit": limit},
		"source":     "smart_feed",
	}

	if userID == 0 && page == 1 && len(properties) > 0 {
		go func(payload iris.Map, lim int, language string) {
			bgCtx := context.Background()
			cacheSvc := services.NewCacheService(storage.Redis)
			key := services.FormatKey(services.PropertySalesSmartFeedAnonKey, lim, language)
			_ = cacheSvc.Set(bgCtx, key, payload, 2*time.Minute)
		}(out, limit, lang)
	}

	return out
}

// GetAppBootstrap bundles cold-start data in one round trip (feeds + user + unread).
// GET /api/bootstrap?limit=8&lang=en
func GetAppBootstrap(ctx iris.Context) {
	userID := optionalUserIDFromContext(ctx)
	deviceID := strings.TrimSpace(ctx.GetHeader("X-Device-ID"))
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	limit := ctx.URLParamIntDefault("limit", 8)
	if limit < 1 || limit > 20 {
		limit = 8
	}

	var (
		saleFeed     iris.Map
		propertyFeed iris.Map
		userSummary  iris.Map
		unread       int64
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if cached, ok := bootstrapSaleVideoFeedFromCache(1, limit); ok {
			saleFeed = cached
			return
		}
		vids, nc, hm, err := buildSaleVideoFeedLitePage(userID, limit)
		if err != nil || len(vids) == 0 {
			saleFeed = iris.Map{"videos": []interface{}{}, "hasMore": true, "source": "empty"}
			return
		}
		saleFeed = iris.Map{
			"videos":     vids,
			"nextCursor": nc,
			"hasMore":    hm,
			"source":     "database",
		}
		if userID == 0 {
			go func() {
				bgCtx := context.Background()
				cacheSvc := services.NewCacheService(storage.Redis)
				key := services.FormatKey(services.PropertySaleVideoFeedKey, 1, limit)
				_ = cacheSvc.Set(bgCtx, key, saleFeed, 2*time.Minute)
			}()
		}
	}()

	go func() {
		defer wg.Done()
		propertyFeed = bootstrapBuildPropertySales(userID, deviceID, 1, limit, lang)
	}()

	if userID > 0 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			userSummary = bootstrapUserSummary(userID)
		}()
		go func() {
			defer wg.Done()
			unread = bootstrapUnreadDirectMessages(userID)
		}()
	}

	wg.Wait()

	payload := iris.Map{
		"serverTime":    time.Now().Unix(),
		"saleVideoFeed": saleFeed,
		"propertySales": propertyFeed,
		"unreadMessages": unread,
	}
	if userSummary != nil {
		payload["user"] = userSummary
	}

	utils.RespondJSONWithETag(ctx, http.StatusOK, payload)
}
