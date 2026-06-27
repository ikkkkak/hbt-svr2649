package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/services/videoprocessing"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
)

// propertySaleFeedLite trims nested listing JSON for scroll feed (full detail on open).
// Images: keep full gallery URLs — they are lightweight strings; client prefetches progressively.
func propertySaleFeedLite(ps models.PropertySale) map[string]interface{} {
	images := propertySaleGalleryURLs(&ps)
	if len(images) == 0 {
		images = ps.Images
	}
	out := map[string]interface{}{
		"id":            ps.ID,
		"title":         ps.Title,
		"city":          ps.City,
		"bedrooms":      ps.Bedrooms,
		"bathrooms":     ps.Bathrooms,
		"listing_price": ps.ListingPrice,
		"images":        images,
	}
	if len(ps.Videos) > 0 {
		out["videos"] = ps.Videos
	}
	if ps.Organization != nil && ps.Organization.ID > 0 {
		out["organization"] = map[string]interface{}{
			"id":   ps.Organization.ID,
			"name": ps.Organization.Name,
			"logo": ps.Organization.Logo,
		}
	}
	if ps.Owner != nil && ps.Owner.ID > 0 {
		out["owner"] = map[string]interface{}{
			"id":        ps.Owner.ID,
			"firstName": ps.Owner.FirstName,
			"lastName":  ps.Owner.LastName,
			"avatarURL": ps.Owner.AvatarURL,
		}
	}
	return out
}

// chunkUploadPreviewBlurURL derives the CDN URL for a blur preview generated during chunked upload.
func chunkUploadPreviewBlurURL(videoURL string) string {
	return storage.ChunkUploadPreviewBlurURL(videoURL)
}

// SyncPropertySaleVideoRows replaces rows in property_sale_videos for this listing so the video feed
// serves the exact URLs uploaded by the host (same order as property_sales.videos / create payload).
// Call after property_sales.videos JSON is saved. Idempotent per full replace.
func SyncPropertySaleVideoRows(propertySaleID uint, userID uint, urls []string) error {
	if propertySaleID == 0 || userID == 0 {
		return nil
	}
	// Hard-delete previous rows for this listing so we never show stale duplicate URLs.
	if err := storage.DB.Unscoped().Where("property_sale_id = ?", propertySaleID).Delete(&models.PropertySaleVideo{}).Error; err != nil {
		return err
	}
	for _, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		thumb := chunkUploadPreviewBlurURL(u)
		row := models.PropertySaleVideo{
			PropertySaleID:   propertySaleID,
			UserID:           userID,
			VideoURL:         u,
			ThumbnailURL:     thumb,
			PreviewBlurURL:   thumb,
			Status:           "approved",
			ProcessingStatus: "pending",
		}
		if err := storage.DB.Create(&row).Error; err != nil {
			return err
		}
		videoprocessing.EnqueuePropertySaleVideo(storage.DB, row.ID, userID)
	}
	return nil
}

// RecordPropertySaleVideoView records a view for a property sale video (supports both authenticated and anonymous users)
func RecordPropertySaleVideoView(ctx iris.Context) {
	propertySaleID, err := ctx.Params().GetUint("id")
	if err != nil || propertySaleID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property sale ID"})
		return
	}

	// Get user ID if authenticated (optional)
	var userID *uint
	var deviceID *string

	// Try to get userID from context values (set by optionalAuthMiddleware)
	if ctxUserID, ok := ctx.Values().Get("userID").(uint); ok && ctxUserID > 0 {
		userID = &ctxUserID
	}

	// Get deviceID and watchDurationSec from request body (optional)
	var input struct {
		DeviceID         *string  `json:"deviceID"`
		WatchDurationSec *float64 `json:"watchDurationSec"`
	}
	if err := ctx.ReadJSON(&input); err == nil {
		if input.DeviceID != nil {
			deviceID = input.DeviceID
		}
	}

	// Append-only interaction for recommendations (entity=property_sale when viewing listing's video)
	psID := propertySaleID
	services.InteractionServiceInstance().Record(services.InteractionInput{
		EntityType:       models.EntityPropertySale,
		EntityID:         propertySaleID,
		PropertySaleID:   &psID,
		EventType:        models.EventVideoView,
		WatchDurationSec: input.WatchDurationSec,
		UserID:           userID,
		DeviceID:         deviceID,
	})

	// Persist visibility for hosts (org tab + studio) — interactions alone are not enough for display counts.
	storage.DB.Model(&models.PropertySale{}).
		Where("id = ?", propertySaleID).
		UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))

	var canonicalRows int64
	storage.DB.Model(&models.PropertySaleVideo{}).
		Where("property_sale_id = ? AND deleted_at IS NULL", propertySaleID).
		Count(&canonicalRows)
	if canonicalRows == 1 {
		storage.DB.Model(&models.PropertySaleVideo{}).
			Where("property_sale_id = ? AND deleted_at IS NULL", propertySaleID).
			UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))
	} else if canonicalRows > 1 {
		// Multiple clips on one listing: bump the row with the highest view_count (best-effort attribution).
		storage.DB.Model(&models.PropertySaleVideo{}).
			Where("property_sale_id = ? AND deleted_at IS NULL", propertySaleID).
			Order("view_count DESC, id ASC").
			Limit(1).
			UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))
	}

	log.Printf("📹 Property Sale Video View: propertySaleID=%d, userID=%v, deviceID=%v",
		propertySaleID, userID, deviceID)

	ctx.JSON(iris.Map{
		"success":        true,
		"propertySaleID": propertySaleID,
		"viewed":         true,
	})
}

// CreatePropertySaleVideo stores a new property sale video record after upload is completed
func CreatePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ CreatePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var input struct {
		PropertySaleID uint    `json:"propertySaleID" validate:"required"`
		VideoURL       string  `json:"videoURL" validate:"required,url"`
		ThumbnailURL   string  `json:"thumbnailURL"`
		DurationSec    float64 `json:"durationSec"`
		Caption        string  `json:"caption"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Ensure property sale exists
	var propertySale models.PropertySale
	if err := storage.DB.Where("id = ?", input.PropertySaleID).First(&propertySale).Error; err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property sale not found"})
		return
	}

	video := models.PropertySaleVideo{
		PropertySaleID:   input.PropertySaleID,
		UserID:           userID,
		VideoURL:         input.VideoURL,
		ThumbnailURL:     input.ThumbnailURL,
		DurationSec:      input.DurationSec,
		Caption:          input.Caption,
		ProcessingStatus: "pending",
	}

	if err := storage.DB.Create(&video).Error; err != nil {
		fmt.Printf("Error creating property sale video: %v\n", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	videoprocessing.EnqueuePropertySaleVideo(storage.DB, video.ID, userID)

	// Notify users who viewed this property or its videos (TikTok-style, dedup/cooldown)
	psID := video.PropertySaleID
	go func() {
		services.NotificationServiceInstance.NotifyNewVideoForProperty("sale", nil, &psID, video.ID, video.Caption, video.ThumbnailURL)
	}()

	ctx.JSON(iris.Map{"success": true, "video": video})
}

// GetPropertySaleVideoFeed returns paginated property sale videos extracted from property sales table
func GetPropertySaleVideoFeed(ctx iris.Context) {
	start := time.Now()
	// Get user ID if authenticated, otherwise use 0 for public access
	// Try context values first (set by optionalAuthMiddleware), then JWT claims
	var userID uint = 0

	// Method 1: Get from context values (set by optionalAuthMiddleware)
	if ctxUserID, ok := ctx.Values().Get("userID").(uint); ok && ctxUserID > 0 {
		userID = ctxUserID
	} else {
		// Method 2: Try to get from JWT claims in context values
		if jwtClaims := ctx.Values().Get("jwt.claims"); jwtClaims != nil {
			if accessToken, ok := jwtClaims.(*utils.AccessToken); ok && accessToken.ID > 0 {
				userID = accessToken.ID
			}
		}

		// Method 3: Fallback to jsonWT.Get (for backward compatibility)
		if userID == 0 {
			if claims := jsonWT.Get(ctx); claims != nil {
				if accessToken, ok := claims.(*utils.AccessToken); ok && accessToken.ID > 0 {
					userID = accessToken.ID
				}
			}
		}
	}

	// Simple pagination
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	// Default lite payload for scroll feed; pass ?full=1 for legacy fat JSON.
	feedLite := strings.TrimSpace(ctx.URLParam("full")) != "1"
	if feedLite && limit > 8 {
		limit = 8
	}
	offset := (page - 1) * limit

	hasGeoFilters := strings.TrimSpace(ctx.URLParam("city")) != "" ||
		ctx.URLParamIntDefault("country_id", 0) > 0 ||
		ctx.URLParamIntDefault("city_id", 0) > 0 ||
		ctx.URLParamIntDefault("quartier_id", 0) > 0 ||
		ctx.URLParamIntDefault("min_area", 0) > 0 ||
		ctx.URLParamIntDefault("max_area", 0) > 0 ||
		ctx.URLParamFloat64Default("min_price", 0) > 0 ||
		ctx.URLParamFloat64Default("max_price", 0) > 0
	fastFirstPage := page == 1 && !hasGeoFilters

	// Redis cache: anonymous first page, no filters — instant cold open on weak networks.
	if userID == 0 && fastFirstPage && feedLite {
		bgCtx := context.Background()
		cacheSvc := services.NewCacheService(storage.Redis)
		cacheKey := services.FormatKey(services.PropertySaleVideoFeedKey, page, limit)
		var cached struct {
			Videos     []map[string]interface{} `json:"videos"`
			NextCursor string                   `json:"nextCursor"`
			HasMore    bool                     `json:"hasMore"`
		}
		if err := cacheSvc.Get(bgCtx, cacheKey, &cached); err == nil && len(cached.Videos) > 0 {
			ctx.JSON(iris.Map{
				"videos":     cached.Videos,
				"nextCursor": cached.NextCursor,
				"hasMore":    cached.HasMore,
				"pagination": iris.Map{"page": page, "limit": limit, "total": len(cached.Videos)},
				"source":     "cache",
			})
			return
		}
	}

	// Base: property sales that have videos
	query := storage.DB.Model(&models.PropertySale{}).
		Where("(status = ? OR is_published = ? OR status IS NULL) AND COALESCE(is_deactivated, false) = ?", "published", true, false).
		Where(`(
			(property_sales.videos IS NOT NULL AND property_sales.videos::text NOT IN ('[]','null',''))
			OR EXISTS (SELECT 1 FROM property_sale_videos psv WHERE psv.property_sale_id = property_sales.id AND psv.deleted_at IS NULL)
		)`)
	if feedLite {
		query = query.Omit(
			"Description", "DescriptionTranslations",
			"FloorPlans", "Neighborhood",
			"Features", "Amenities", "HostPrivateNote", "VerificationNotes", "VirtualTour",
		)
		query = query.Preload("Organization", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name", "logo", "banner_image", "owner_id")
		})
		query = query.Preload("Owner", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "first_name", "last_name", "avatar_url")
		})
	} else {
		query = query.Preload("Organization").Preload("Owner")
	}

	if userID > 0 {
		query = query.Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID)
	}

	// Safe filter params (sanitized, no SQL injection)
	if city := strings.TrimSpace(ctx.URLParam("city")); city != "" {
		query = query.Where("LOWER(property_sales.city) = LOWER(?)", city)
	}
	if countryID := ctx.URLParamIntDefault("country_id", 0); countryID > 0 {
		query = query.Where("property_sales.country_id = ?", countryID)
	}
	if cityID := ctx.URLParamIntDefault("city_id", 0); cityID > 0 {
		query = query.Where("property_sales.city_id = ?", cityID)
	}
	if quartierID := ctx.URLParamIntDefault("quartier_id", 0); quartierID > 0 {
		query = query.Where("property_sales.quartier_id = ?", quartierID)
	}
	if minArea := ctx.URLParamIntDefault("min_area", 0); minArea > 0 {
		query = query.Where("property_sales.square_footage >= ?", minArea)
	}
	if maxArea := ctx.URLParamIntDefault("max_area", 0); maxArea > 0 {
		query = query.Where("property_sales.square_footage <= ?", maxArea)
	}
	if minPrice := ctx.URLParamFloat64Default("min_price", 0); minPrice > 0 {
		query = query.Where("property_sales.listing_price >= ?", minPrice)
	}
	if maxPrice := ctx.URLParamFloat64Default("max_price", 0); maxPrice > 0 {
		query = query.Where("property_sales.listing_price <= ?", maxPrice)
	}

	if fastFirstPage {
		// Fast path: recent listings first — no heavy aggregate joins on scroll open.
		query = query.Order("property_sales.created_at DESC, property_sales.id DESC")
	} else {
		// Hybrid ranking for pagination / filtered feeds
		query = query.Select("property_sales.*").
			Joins(`LEFT JOIN (
			SELECT property_sale_video_id AS ps_id, COALESCE(COUNT(*), 0)::float AS like_cnt
			FROM property_sale_video_likes WHERE deleted_at IS NULL GROUP BY property_sale_video_id
		) likes ON likes.ps_id = property_sales.id`).
			Joins(`LEFT JOIN (
			SELECT property_sale_video_id AS ps_id, COALESCE(COUNT(*), 0)::float AS save_cnt
			FROM property_sale_video_saves WHERE deleted_at IS NULL GROUP BY property_sale_video_id
		) saves ON saves.ps_id = property_sales.id`).
			Joins(`LEFT JOIN (
			SELECT property_sale_video_id AS ps_id, COALESCE(COUNT(*), 0)::float AS comment_cnt
			FROM property_sale_video_comments WHERE deleted_at IS NULL AND parent_id IS NULL GROUP BY property_sale_video_id
		) comments ON comments.ps_id = property_sales.id`).
			Order(`(
			0.4 * GREATEST(0, 1.0 - EXTRACT(EPOCH FROM (NOW() - property_sales.created_at)) / 86400.0 / 30.0) +
			0.4 * LEAST(1.0, (COALESCE(likes.like_cnt, 0) * 2 + COALESCE(comments.comment_cnt, 0) * 3 + COALESCE(saves.save_cnt, 0)) / 100.0) +
			0.2 * (random() + 0.5)
		) DESC`)
	}

	var propertySales []models.PropertySale
	if err := query.Limit(limit).Offset(offset).Find(&propertySales).Error; err != nil {
		log.Printf("❌ property-sale-videos/feed query error: %v", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch property sales with videos"})
		return
	}

	propertyIDs := make([]uint, 0, len(propertySales))
	for _, ps := range propertySales {
		propertyIDs = append(propertyIDs, ps.ID)
	}

	// Preload canonical property_sale_videos rows in one query (avoids N+1).
	tableVideosByProperty := map[uint][]models.PropertySaleVideo{}
	if len(propertyIDs) > 0 {
		var allTableVideos []models.PropertySaleVideo
		_ = storage.DB.Where("property_sale_id IN ?", propertyIDs).Order("property_sale_id ASC, id ASC").Find(&allTableVideos).Error
		for _, tv := range allTableVideos {
			tableVideosByProperty[tv.PropertySaleID] = append(tableVideosByProperty[tv.PropertySaleID], tv)
		}
	}

	type countRow struct {
		PropertySaleVideoID uint
		Cnt                 int64
	}
	likesCountByProperty := map[uint]int64{}
	savesCountByProperty := map[uint]int64{}
	commentsCountByProperty := map[uint]int64{}
	if len(propertyIDs) > 0 {
		var likeRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoLike{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ?", propertyIDs).
			Group("property_sale_video_id").
			Scan(&likeRows).Error
		for _, r := range likeRows {
			likesCountByProperty[r.PropertySaleVideoID] = r.Cnt
		}

		var saveRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoSave{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ?", propertyIDs).
			Group("property_sale_video_id").
			Scan(&saveRows).Error
		for _, r := range saveRows {
			savesCountByProperty[r.PropertySaleVideoID] = r.Cnt
		}

		var commentRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoComment{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ? AND parent_id IS NULL", propertyIDs).
			Group("property_sale_video_id").
			Scan(&commentRows).Error
		for _, r := range commentRows {
			commentsCountByProperty[r.PropertySaleVideoID] = r.Cnt
		}
	}

	userLikedByProperty := map[uint]bool{}
	userSavedByProperty := map[uint]bool{}
	if userID > 0 && len(propertyIDs) > 0 {
		var myLikes []models.PropertySaleVideoLike
		_ = storage.DB.Select("property_sale_video_id").
			Where("property_sale_video_id IN ? AND user_id = ?", propertyIDs, userID).
			Find(&myLikes).Error
		for _, l := range myLikes {
			userLikedByProperty[l.PropertySaleVideoID] = true
		}

		var mySaves []models.PropertySaleVideoSave
		_ = storage.DB.Select("property_sale_video_id").
			Where("property_sale_video_id IN ? AND user_id = ?", propertyIDs, userID).
			Find(&mySaves).Error
		for _, s := range mySaves {
			userSavedByProperty[s.PropertySaleVideoID] = true
		}
	}

	// Convert property sales to video format
	var videos []map[string]interface{}
	for _, ps := range propertySales {
		// Prefer canonical rows from property_sale_videos (exact upload URLs + thumbnails);
		// fall back to property_sales.videos JSON for older listings.
		tableVideos := tableVideosByProperty[ps.ID]

		var videoURLs []string
		var tableThumbs []string
		if len(tableVideos) > 0 {
			for _, tv := range tableVideos {
				u := strings.TrimSpace(tv.VideoURL)
				if u == "" {
					continue
				}
				videoURLs = append(videoURLs, u)
				tableThumbs = append(tableThumbs, strings.TrimSpace(tv.ThumbnailURL))
			}
		}
		if len(videoURLs) == 0 {
			videoURLs = ps.Videos
		}

		// Skip if no videos
		if len(videoURLs) == 0 {
			continue
		}

		// Create a video entry for each video URL
		for i, videoURL := range videoURLs {
			videoID := fmt.Sprintf("%d_%d", ps.ID, i)

			likesCount := likesCountByProperty[ps.ID]
			savesCount := savesCountByProperty[ps.ID]
			commentsCount := commentsCountByProperty[ps.ID]
			liked := userLikedByProperty[ps.ID]
			saved := userSavedByProperty[ps.ID]

			// Get organization branding (handle empty organization gracefully)
			orgLogo := ""
			orgName := ""
			orgOwnerID := uint(0)
			orgID := uint(0)
			if ps.OrganizationID != nil && *ps.OrganizationID > 0 && ps.Organization != nil && ps.Organization.ID > 0 {
				orgName = ps.Organization.Name
				if ps.Organization.Logo != "" {
					orgLogo = ps.Organization.Logo
				}
				orgOwnerID = ps.Organization.OwnerID
				orgID = ps.Organization.ID
			}

			profileUserID := orgOwnerID
			if profileUserID == 0 && ps.OwnerID != nil && *ps.OwnerID > 0 {
				profileUserID = *ps.OwnerID
			}
			if profileUserID == 0 && ps.Owner != nil && ps.Owner.ID > 0 {
				profileUserID = ps.Owner.ID
			}

			thumb := ""
			if i < len(tableThumbs) && tableThumbs[i] != "" {
				thumb = tableThumbs[i]
			}

			var tv *models.PropertySaleVideo
			if i < len(tableVideos) {
				tv = &tableVideos[i]
				if thumb == "" && tv.ThumbnailURL != "" {
					thumb = strings.TrimSpace(tv.ThumbnailURL)
				}
				if thumb == "" && tv.PreviewBlurURL != "" {
					thumb = strings.TrimSpace(tv.PreviewBlurURL)
				}
			}

			psPayload := any(ps)
			if feedLite {
				psPayload = propertySaleFeedLite(ps)
			}

			video := map[string]interface{}{
				"ID":             videoID, // Unique ID for each video
				"propertySaleID": ps.ID,
				"propertySale":   psPayload,
				"userID":         profileUserID,
				"videoURL":       videoURL,
				"thumbnailURL":   thumb,
				"durationSec":    0, // Will be calculated
				"caption":        ps.Title,
				"likesCount":     likesCount,
				"commentsCount":  commentsCount,
				"savesCount":     savesCount,
				"viewCount":      0,
				"isFlagged":      false,
				"status":         "approved",
				"liked":          liked,
				"saved":          saved,
				"organization": map[string]interface{}{
					"id":      orgID,
					"name":    orgName,
					"logoURL": orgLogo,
				},
				"CreatedAt": ps.CreatedAt,
				"UpdatedAt": ps.UpdatedAt,
			}
			if tv != nil {
				applyPropertySaleVideoStreamFields(video, tv)
				if strings.Contains(strings.ToLower(strings.TrimSpace(tv.Caption)), "auto-generated") {
					video["isAutoSlideshow"] = true
				}
			}

			videos = append(videos, video)
		}
	}

	fmt.Printf("📹 Property Sale Video Feed - Returning %d videos for user ID: %d (page: %d, limit: %d, fast=%v)\n", len(videos), userID, page, limit, fastFirstPage)

	hasMore := len(propertySales) >= limit
	nextCursor := ""
	if hasMore {
		nextCursor = fmt.Sprintf("%d", page+1)
	}

	if userID == 0 && fastFirstPage && feedLite && len(videos) > 0 {
		bgCtx := context.Background()
		cacheSvc := services.NewCacheService(storage.Redis)
		cacheKey := services.FormatKey(services.PropertySaleVideoFeedKey, page, limit)
		_ = cacheSvc.Set(bgCtx, cacheKey, map[string]interface{}{
			"videos":     videos,
			"nextCursor": nextCursor,
			"hasMore":    hasMore,
		}, 2*time.Minute)
	}

	if time.Since(start) >= 300*time.Millisecond {
		log.Printf("📹 Property Sale Video Feed slow: %v (user=%d page=%d)", time.Since(start), userID, page)
	}

	ctx.JSON(iris.Map{
		"videos": videos,
		"nextCursor": nextCursor,
		"hasMore":    hasMore,
		"pagination": iris.Map{
			"page":  page,
			"limit": limit,
			"total": len(videos),
		},
	})
}

// LikePropertySaleVideo handles liking a property sale video
func LikePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ LikePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Check if already liked
	var existingLike models.PropertySaleVideoLike
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existingLike).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "message": "Already liked"})
		return
	}

	// Create like
	like := models.PropertySaleVideoLike{
		PropertySaleVideoID: videoID,
		UserID:              userID,
	}

	if err := storage.DB.Create(&like).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Record interaction for recommendations (id can be property_sale_id for synthetic or PropertySaleVideo.ID)
	psID := videoID
	services.InteractionServiceInstance().Record(services.InteractionInput{
		EntityType: models.EntityPropertySale, EntityID: videoID, PropertySaleID: &psID,
		EventType: models.EventLike, UserID: &userID, DeviceID: nil,
	})

	// Count total likes for this property sale video (synthetic videos, so count from likes table)
	var likesCount int64
	storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)

	log.Printf("✅ LikePropertySaleVideo: User %d liked video %d, new count: %d", userID, videoID, likesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video liked", "likesCount": likesCount})
}

// UnlikePropertySaleVideo handles unliking a property sale video
func UnlikePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnlikePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Delete like
	result := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).Delete(&models.PropertySaleVideoLike{})
	if result.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Count total likes for this property sale video (synthetic videos, so count from likes table)
	var likesCount int64
	storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)

	log.Printf("✅ UnlikePropertySaleVideo: User %d unliked video %d, new count: %d", userID, videoID, likesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video unliked", "likesCount": likesCount})
}

// SavePropertySaleVideo handles saving a property sale video
func SavePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ SavePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Check if already saved
	var existingSave models.PropertySaleVideoSave
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existingSave).Error; err == nil {
		// Already saved, return current count
		var savesCount int64
		storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Already saved", "savesCount": savesCount})
		return
	}

	// Create save
	save := models.PropertySaleVideoSave{
		PropertySaleVideoID: videoID,
		UserID:              userID,
	}

	if err := storage.DB.Create(&save).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Record interaction for recommendations
	psID := videoID
	services.InteractionServiceInstance().Record(services.InteractionInput{
		EntityType: models.EntityPropertySale, EntityID: videoID, PropertySaleID: &psID,
		EventType: models.EventSave, UserID: &userID, DeviceID: nil,
	})

	// Count total saves for this property sale video (synthetic videos, so count from saves table)
	var savesCount int64
	storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)

	log.Printf("✅ SavePropertySaleVideo: User %d saved video %d, new count: %d", userID, videoID, savesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video saved", "savesCount": savesCount})
}

// UnsavePropertySaleVideo handles unsaving a property sale video
func UnsavePropertySaleVideo(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnsavePropertySaleVideo: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Delete save
	result := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).Delete(&models.PropertySaleVideoSave{})
	if result.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Count total saves for this property sale video (synthetic videos, so count from saves table)
	var savesCount int64
	storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)

	log.Printf("✅ UnsavePropertySaleVideo: User %d unsaved video %d, new count: %d", userID, videoID, savesCount)
	ctx.JSON(iris.Map{"success": true, "message": "Video unsaved", "savesCount": savesCount})
}

// GetPropertySaleVideoComments gets comments for a property sale video with nested replies
func GetPropertySaleVideoComments(ctx iris.Context) {
	userID := OptionalAuthUserID(ctx)

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	// Get top-level comments (no parent)
	var comments []models.PropertySaleVideoComment
	err = storage.DB.Where("property_sale_video_id = ? AND parent_id IS NULL", videoID).
		Preload("User").
		Preload("Replies.User").
		Order("created_at DESC").
		Find(&comments).Error
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch comments"})
		return
	}

	// Get user's liked comment IDs (only if authenticated)
	var likedMap map[uint]bool = make(map[uint]bool)
	if userID > 0 {
		var commentIDs []uint
		for _, comment := range comments {
			commentIDs = append(commentIDs, comment.ID)
			for _, reply := range comment.Replies {
				commentIDs = append(commentIDs, reply.ID)
			}
		}

		var likedCommentIDs []uint
		if len(commentIDs) > 0 {
			storage.DB.Model(&models.PropertySaleVideoCommentLike{}).Where("comment_id IN ? AND user_id = ?", commentIDs, userID).Pluck("comment_id", &likedCommentIDs)
		}

		for _, id := range likedCommentIDs {
			likedMap[id] = true
		}
	}

	// Add isLiked to comments and replies
	type CommentWithUserState struct {
		models.PropertySaleVideoComment
		IsLiked bool                   `json:"isLiked"`
		Replies []CommentWithUserState `json:"replies"`
	}

	var commentsWithState []CommentWithUserState
	for _, comment := range comments {
		// Create replies with IsLiked
		var repliesWithState []CommentWithUserState
		for _, reply := range comment.Replies {
			repliesWithState = append(repliesWithState, CommentWithUserState{
				PropertySaleVideoComment: reply,
				IsLiked:                  likedMap[reply.ID],
			})
		}

		commentWithState := CommentWithUserState{
			PropertySaleVideoComment: comment,
			IsLiked:                  likedMap[comment.ID],
		}
		commentWithState.Replies = repliesWithState
		commentsWithState = append(commentsWithState, commentWithState)
	}

	ctx.JSON(iris.Map{"success": true, "comments": commentsWithState})
}

// CreatePropertySaleVideoComment creates a comment on a property sale video
func CreatePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ CreatePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	var input struct {
		Content  string `json:"content" validate:"required"`
		ParentID *uint  `json:"parentID"` // Optional parent ID for replies
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate parent comment exists if parentID is provided
	if input.ParentID != nil {
		var parentComment models.PropertySaleVideoComment
		if err := storage.DB.Where("id = ? AND property_sale_video_id = ?", *input.ParentID, videoID).First(&parentComment).Error; err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Parent comment not found"})
			return
		}
	}

	comment := models.PropertySaleVideoComment{
		PropertySaleVideoID: videoID,
		UserID:              userID,
		Content:             input.Content,
		ParentID:            input.ParentID,
	}

	if err := storage.DB.Create(&comment).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Note: For synthetic property sale videos, we don't update a PropertySaleVideo record
	// The comments count is calculated dynamically in the feed endpoint

	// Load the comment with user data
	storage.DB.Preload("User").First(&comment, comment.ID)

	ctx.JSON(iris.Map{"success": true, "comment": comment})
}

// UpdatePropertySaleVideoComment updates a property sale video comment
func UpdatePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UpdatePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	var input struct {
		Content string `json:"content" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	var comment models.PropertySaleVideoComment
	if err := storage.DB.Where("id = ? AND user_id = ?", id, userID).First(&comment).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Comment not found"})
		return
	}

	comment.Content = input.Content
	comment.Edited = true
	if err := storage.DB.Save(&comment).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	storage.DB.Preload("User").First(&comment, comment.ID)
	ctx.JSON(iris.Map{"success": true, "comment": comment})
}

// DeletePropertySaleVideoComment deletes a property sale video comment
func DeletePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ DeletePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	var comment models.PropertySaleVideoComment
	if err := storage.DB.Where("id = ? AND user_id = ?", id, userID).First(&comment).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Comment not found"})
		return
	}

	// Delete replies first
	storage.DB.Where("parent_id = ?", comment.ID).Delete(&models.PropertySaleVideoComment{})

	// Delete comment likes
	storage.DB.Where("comment_id = ?", comment.ID).Delete(&models.PropertySaleVideoCommentLike{})

	// Get video ID before deleting (for logging)
	videoID := comment.PropertySaleVideoID

	// Delete the comment
	storage.DB.Delete(&comment)

	// Note: We use dynamic counting in the feed endpoint, so no need to update a stored count
	// The feed always counts comments dynamically: WHERE property_sale_video_id = ? AND parent_id IS NULL

	log.Printf("✅ DeletePropertySaleVideoComment: User %d deleted comment %d for video %d", userID, id, videoID)
	ctx.JSON(iris.Map{"success": true})
}

// LikePropertySaleVideoComment handles liking a property sale video comment
func LikePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ LikePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	// Check if already liked
	var existingLike models.PropertySaleVideoCommentLike
	if err := storage.DB.Where("comment_id = ? AND user_id = ?", id, userID).First(&existingLike).Error; err == nil {
		// Already liked, return current count
		var comment models.PropertySaleVideoComment
		if err := storage.DB.First(&comment, id).Error; err == nil {
			ctx.JSON(iris.Map{"success": true, "message": "Already liked", "likesCount": comment.LikesCount})
		} else {
			ctx.JSON(iris.Map{"success": true, "message": "Already liked"})
		}
		return
	}

	// Create like
	like := models.PropertySaleVideoCommentLike{
		CommentID: id,
		UserID:    userID,
	}

	if err := storage.DB.Create(&like).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Update likes count
	storage.DB.Model(&models.PropertySaleVideoComment{}).Where("id = ?", id).Update("likes_count", gorm.Expr("likes_count + 1"))

	// Get updated likes count
	var comment models.PropertySaleVideoComment
	if err := storage.DB.First(&comment, id).Error; err == nil {
		log.Printf("✅ LikePropertySaleVideoComment: User %d liked comment %d, new count: %d", userID, id, comment.LikesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Comment liked", "likesCount": comment.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true, "message": "Comment liked"})
	}
}

// UnlikePropertySaleVideoComment handles unliking a property sale video comment
func UnlikePropertySaleVideoComment(ctx iris.Context) {
	// Get user ID from middleware context
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ UnlikePropertySaleVideoComment: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid comment ID"})
		return
	}

	// Delete like
	result := storage.DB.Where("comment_id = ? AND user_id = ?", id, userID).Delete(&models.PropertySaleVideoCommentLike{})
	if result.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if result.RowsAffected > 0 {
		// Update likes count
		storage.DB.Model(&models.PropertySaleVideoComment{}).Where("id = ?", id).Update("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)"))
	}

	// Get updated likes count
	var comment models.PropertySaleVideoComment
	if err := storage.DB.First(&comment, id).Error; err == nil {
		log.Printf("✅ UnlikePropertySaleVideoComment: User %d unliked comment %d, new count: %d", userID, id, comment.LikesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Comment unliked", "likesCount": comment.LikesCount})
	} else {
		ctx.JSON(iris.Map{"success": true, "message": "Comment unliked"})
	}
}

// hostSaleVideoDisplayViews picks the best available stored view count (avoid double-counting).
func hostSaleVideoDisplayViews(counts ...int64) int64 {
	var max int64
	for _, c := range counts {
		if c > max {
			max = c
		}
	}
	return max
}

// GetPropertySaleVideosByOrganizationOrHost returns property sale videos with stats for admin interface
// Supports filtering by organization_id or owner_id (for individual hosts)
func GetPropertySaleVideosByOrganizationOrHost(ctx iris.Context) {
	// Get user ID from middleware context (must be authenticated)
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Println("❌ GetPropertySaleVideosByOrganizationOrHost: Unauthorized")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	// Get query parameters
	organizationID := ctx.URLParamDefault("organization_id", "")
	ownerID := ctx.URLParamDefault("owner_id", "")

	// Build query for property sales with videos
	// The videos field is stored as json/jsonb array, check if it's not empty
	// Use text comparison to avoid type casting issues
	query := storage.DB.Model(&models.PropertySale{}).
		Where("videos IS NOT NULL AND videos::text != '[]' AND videos::text != 'null'").
		Preload("Organization").
		Preload("Owner")

	// Filter by organization or owner
	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	} else if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	} else {
		// If no filter, check if user is admin of any organization or is an owner
		var userOrgs []models.OrganizationMember
		storage.DB.Where("user_id = ? AND role = ?", userID, "admin").Find(&userOrgs)

		var orgIDs []uint
		for _, member := range userOrgs {
			orgIDs = append(orgIDs, member.OrganizationID)
		}

		if len(orgIDs) > 0 {
			query = query.Where("organization_id IN ? OR owner_id = ?", orgIDs, userID)
		} else {
			// User is not admin, only show their own properties
			query = query.Where("owner_id = ?", userID)
		}
	}

	// Get property sales
	var propertySales []models.PropertySale
	if err := query.Order("created_at DESC").Find(&propertySales).Error; err != nil {
		log.Printf("❌ Error fetching property sales: %v", err)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Extract videos with stats (batched — same keys as the public video feed)
	type VideoStats struct {
		PropertySaleID   uint   `json:"propertySaleID"`
		PropertyTitle    string `json:"propertyTitle"`
		VideoURL         string `json:"videoURL"`
		ThumbnailURL     string `json:"thumbnailURL"`
		LikesCount       int64  `json:"likesCount"`
		CommentsCount    int64  `json:"commentsCount"`
		SavesCount       int64  `json:"savesCount"`
		ViewCount        int64  `json:"viewCount"`
		OrganizationName string `json:"organizationName,omitempty"`
		OwnerName        string `json:"ownerName,omitempty"`
		CreatedAt        string `json:"createdAt"`
	}

	propertyIDs := make([]uint, 0, len(propertySales))
	for _, ps := range propertySales {
		propertyIDs = append(propertyIDs, ps.ID)
	}

	type countRow struct {
		PropertySaleVideoID uint
		Cnt                 int64
	}
	likesByProperty := map[uint]int64{}
	savesByProperty := map[uint]int64{}
	commentsByProperty := map[uint]int64{}
	videoViewsByProperty := map[uint]int64{}

	if len(propertyIDs) > 0 {
		var likeRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoLike{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ?", propertyIDs).
			Group("property_sale_video_id").
			Scan(&likeRows).Error
		for _, r := range likeRows {
			likesByProperty[r.PropertySaleVideoID] = r.Cnt
		}

		var saveRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoSave{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ?", propertyIDs).
			Group("property_sale_video_id").
			Scan(&saveRows).Error
		for _, r := range saveRows {
			savesByProperty[r.PropertySaleVideoID] = r.Cnt
		}

		var commentRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoComment{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ? AND parent_id IS NULL", propertyIDs).
			Group("property_sale_video_id").
			Scan(&commentRows).Error
		for _, r := range commentRows {
			commentsByProperty[r.PropertySaleVideoID] = r.Cnt
		}

		var interactionViewRows []countRow
		_ = storage.DB.Model(&models.Interaction{}).
			Select("property_sale_id as property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_id IN ? AND event_type = ?", propertyIDs, models.EventVideoView).
			Group("property_sale_id").
			Scan(&interactionViewRows).Error
		for _, r := range interactionViewRows {
			videoViewsByProperty[r.PropertySaleVideoID] = r.Cnt
		}
	}

	tableVideosByProperty := map[uint][]models.PropertySaleVideo{}
	if len(propertyIDs) > 0 {
		var allTableVideos []models.PropertySaleVideo
		_ = storage.DB.Where("property_sale_id IN ?", propertyIDs).
			Order("property_sale_id ASC, id ASC").
			Find(&allTableVideos).Error
		for _, tv := range allTableVideos {
			tableVideosByProperty[tv.PropertySaleID] = append(tableVideosByProperty[tv.PropertySaleID], tv)
		}
	}

	var videos []VideoStats

	for _, ps := range propertySales {
		tableVideos := tableVideosByProperty[ps.ID]
		var videoURLs []string
		var tableThumbs []string
		if len(tableVideos) > 0 {
			for _, tv := range tableVideos {
				u := strings.TrimSpace(tv.VideoURL)
				if u == "" {
					continue
				}
				videoURLs = append(videoURLs, u)
				tableThumbs = append(tableThumbs, strings.TrimSpace(tv.ThumbnailURL))
			}
		}
		if len(videoURLs) == 0 {
			videoURLs = ps.Videos
		}
		if len(videoURLs) == 0 {
			continue
		}

		likesCount := likesByProperty[ps.ID]
		commentsCount := commentsByProperty[ps.ID]
		savesCount := savesByProperty[ps.ID]
		listingViews := hostSaleVideoDisplayViews(ps.ViewCount, videoViewsByProperty[ps.ID])

		for i, videoURL := range videoURLs {
			thumb := videoURL
			if i < len(tableThumbs) && tableThumbs[i] != "" {
				thumb = tableThumbs[i]
			}
			rowViews := int64(0)
			if i < len(tableVideos) {
				rowViews = tableVideos[i].ViewCount
			}
			viewCount := hostSaleVideoDisplayViews(listingViews, rowViews)

			video := VideoStats{
				PropertySaleID: ps.ID,
				PropertyTitle:  ps.Title,
				VideoURL:       videoURL,
				ThumbnailURL:   thumb,
				LikesCount:     likesCount,
				CommentsCount:  commentsCount,
				SavesCount:     savesCount,
				ViewCount:      viewCount,
				CreatedAt:      ps.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}

			if ps.Organization != nil {
				video.OrganizationName = ps.Organization.Name
			}
			if ps.Owner != nil {
				ownerName := strings.TrimSpace(fmt.Sprintf("%s %s", ps.Owner.FirstName, ps.Owner.LastName))
				if ownerName == "" {
					ownerName = ps.Owner.Email
				}
				video.OwnerName = ownerName
			}

			videos = append(videos, video)
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"videos":  videos,
		"total":   len(videos),
	})
}

// applyPropertySaleVideoStreamFields adds HLS playback fields to feed/API maps.
func applyPropertySaleVideoStreamFields(out map[string]interface{}, tv *models.PropertySaleVideo) {
	if out == nil || tv == nil {
		return
	}
	out["propertySaleVideoID"] = tv.ID
	if tv.HlsURL != "" {
		out["hlsURL"] = storage.NormalizePlaybackMediaURL(tv.HlsURL)
	}
	if tv.MobileVideoURL != "" {
		mobile := storage.NormalizePlaybackMediaURL(tv.MobileVideoURL)
		out["mobileVideoURL"] = mobile
		out["mobile_video_url"] = mobile
	}
	if tv.PreviewBlurURL != "" {
		out["previewBlurURL"] = tv.PreviewBlurURL
		out["preview_blur_url"] = tv.PreviewBlurURL
	}
	if tv.SpriteSheetURL != "" {
		out["spriteSheetURL"] = tv.SpriteSheetURL
	}
	if tv.ProcessingStatus != "" {
		out["processingStatus"] = tv.ProcessingStatus
	}
	if tv.DurationSec > 0 {
		out["durationSec"] = tv.DurationSec
	}
}

// expandPropertySaleVideoRows attaches canonical saleVideos (with HLS URLs) to listings.
func expandPropertySaleVideoRows(properties []models.PropertySale) {
	if len(properties) == 0 {
		return
	}
	ids := make([]uint, 0, len(properties))
	for _, ps := range properties {
		if ps.ID > 0 {
			ids = append(ids, ps.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var rows []models.PropertySaleVideo
	_ = storage.DB.Where("property_sale_id IN ? AND deleted_at IS NULL", ids).
		Order("property_sale_id ASC, id ASC").
		Find(&rows).Error
	byProperty := map[uint][]models.PropertySaleVideo{}
	for _, row := range rows {
		byProperty[row.PropertySaleID] = append(byProperty[row.PropertySaleID], row)
	}
	for i := range properties {
		if vids, ok := byProperty[properties[i].ID]; ok && len(vids) > 0 {
			properties[i].SaleVideos = vids
		}
	}
}
