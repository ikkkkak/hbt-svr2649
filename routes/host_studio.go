package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

const hostStudioTrendDays = 14

type studioDayMetrics map[string]int64 // views, saves, likes

// GET /api/host/studio — host-safe listing performance (views, saves, likes; no admin/ML data).
func GetHostStudio(ctx iris.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ GetHostStudio panic: %v", r)
			ctx.StatusCode(500)
			ctx.JSON(iris.Map{"error": "host_studio_failed", "message": fmt.Sprint(r)})
		}
	}()

	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(401)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	log.Printf("📊 GetHostStudio userID=%d", userID)

	dayLabels := buildStudioDayLabels(hostStudioTrendDays)
	since := time.Now().UTC().AddDate(0, 0, -(hostStudioTrendDays - 1)).Truncate(24 * time.Hour)

	var rentProps []models.Property
	if err := storage.DB.
		Select("id", "title", "city", "status", "images", "host_id", "updated_at").
		Where("host_id = ? AND deleted_at IS NULL", userID).
		Where("LOWER(status) IN ?", []string{"approved", "live", "published", "pending"}).
		Order("updated_at DESC").Limit(50).
		Find(&rentProps).Error; err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "failed to load listings"})
		return
	}

	saleProps, err := loadHostPropertySales(userID)
	if err != nil {
		ctx.StatusCode(500)
		ctx.JSON(iris.Map{"error": "failed to load sale listings"})
		return
	}

	rentIDs := make([]uint, 0, len(rentProps))
	for _, p := range rentProps {
		rentIDs = append(rentIDs, p.ID)
	}
	saleIDs := make([]uint, 0, len(saleProps))
	for _, p := range saleProps {
		saleIDs = append(saleIDs, p.ID)
	}

	resByProperty := loadReservationCountsByProperty(rentIDs)
	videoByProperty := loadVideoAggregatesByProperty(rentIDs)
	rentTrends, saleTrends := loadStudioInteractionTrends(since, rentIDs, saleIDs)

	listings := make([]iris.Map, 0, len(rentProps)+len(saleProps))
	var sumViews, sumSaves, sumLikes int64
	summaryViews := make([]int64, hostStudioTrendDays)

	for _, p := range rentProps {
		key := studioRentKey(p.ID)
		trend := rentTrends[key]
		va := videoByProperty[p.ID]
		views := studioSumMetric(trend, "views") + va.Views
		saves := studioSumMetric(trend, "saves") + va.Saves
		likes := studioSumMetric(trend, "likes") + va.Likes
		sumViews += views
		sumSaves += saves
		sumLikes += likes
		mergeTrendIntoSummary(summaryViews, dayLabels, trend)

		listings = append(listings, iris.Map{
			"kind":      "rent",
			"id":        p.ID,
			"title":     strings.TrimSpace(p.Title),
			"city":      strings.TrimSpace(p.City),
			"status":    strings.ToLower(strings.TrimSpace(p.Status)),
			"image_url": firstRentPropertyImage(p.Images),
			"metrics": iris.Map{
				"views":        views,
				"saves":        saves,
				"likes":        likes,
				"comments":     va.Comments,
				"reservations": resByProperty[p.ID],
			},
			"trend": studioTrendPayload(dayLabels, trend),
		})
	}

	for _, p := range saleProps {
		key := studioSaleKey(p.ID)
		trend := saleTrends[key]
		// ViewCount is the authoritative all-time total; daily trend comes from interactions.
		views := int64(p.ViewCount)
		saves := studioSumMetric(trend, "saves")
		likes := studioSumMetric(trend, "likes")
		sumViews += views
		sumSaves += saves
		sumLikes += likes
		mergeTrendIntoSummary(summaryViews, dayLabels, trend)

		listings = append(listings, iris.Map{
			"kind":      "sale",
			"id":        p.ID,
			"title":     strings.TrimSpace(p.Title),
			"city":      strings.TrimSpace(p.City),
			"status":    strings.ToLower(strings.TrimSpace(p.Status)),
			"image_url": firstSaleImage(p.Images),
			"metrics": iris.Map{
				"views": views,
				"saves": saves,
				"likes": likes,
			},
			"trend": studioTrendPayload(dayLabels, trend),
		})
	}

	var pendingRes int64
	if len(rentIDs) > 0 {
		storage.DB.Model(&models.Reservation{}).
			Where("property_id IN ? AND LOWER(status) = ?", rentIDs, "pending").
			Count(&pendingRes)
	}

	hostVideos, videoSummary := loadHostStudioVideos(userID, rentIDs, rentProps, saleProps)

	var hostUser models.User
	_ = storage.DB.Select(
		"id", "broker_id", "broker_status", "broker_submitted_at", "broker_verified_at",
		"true_broker", "avatar_url",
	).First(&hostUser, userID).Error
	brokerBlock := brokerStatusPayload(&hostUser)

	log.Printf("📊 GetHostStudio ok userID=%d listings=%d videos=%d", userID, len(listings), len(hostVideos))

	ctx.Header("Cache-Control", "private, max-age=5, stale-while-revalidate=15")
	ctx.JSON(iris.Map{
		"data": iris.Map{
			"summary": iris.Map{
				"active_listings":      len(listings),
				"total_views":          sumViews,
				"total_saves":          sumSaves,
				"total_likes":          sumLikes,
				"pending_reservations": pendingRes,
				"trend": iris.Map{
					"days":  dayLabels,
					"views": summaryViews,
				},
			},
			"listings":      listings,
			"videos":        hostVideos,
			"video_summary": videoSummary,
			"broker_verification": brokerBlock,
		},
	})
}

func studioMaxMetric(counts ...int64) int64 {
	var max int64
	for _, c := range counts {
		if c > max {
			max = c
		}
	}
	return max
}

// loadHostStudioVideos returns each uploaded video (rent + sale) with per-video visibility metrics.
func loadHostStudioVideos(userID uint, rentIDs []uint, rentProps []models.Property, saleProps []models.PropertySale) ([]iris.Map, iris.Map) {
	out := make([]iris.Map, 0)
	var sumRentViews, sumRentLikes, sumSaleViews, sumSaleLikes int64
	var rentCount, saleCount int

	rentTitleByID := map[uint]string{}
	for _, p := range rentProps {
		rentTitleByID[p.ID] = strings.TrimSpace(p.Title)
	}

	var rentVideos []models.Video
	rentQ := storage.DB.
		Select("id", "property_id", "user_id", "caption", "video_url", "thumbnail_url", "status", "duration_sec", "view_count", "likes_count", "saves_count", "comments_count", "created_at").
		Where("deleted_at IS NULL").
		Where("property_id IS NOT NULL").
		Where("COALESCE(is_promotional, false) = ?", false)
	if len(rentIDs) > 0 {
		rentQ = rentQ.Where("user_id = ? OR property_id IN ?", userID, rentIDs)
	} else {
		rentQ = rentQ.Where("user_id = ?", userID)
	}
	if err := rentQ.Order("created_at DESC").Limit(100).Find(&rentVideos).Error; err != nil {
		log.Printf("⚠️ GetHostStudio rent videos: %v", err)
	}

	type metricRow struct {
		ID  uint
		Cnt int64
	}
	rentVideoIDs := make([]uint, 0, len(rentVideos))
	for _, v := range rentVideos {
		rentVideoIDs = append(rentVideoIDs, v.ID)
	}
	rentInteractionViews := map[uint]int64{}
	if len(rentVideoIDs) > 0 {
		var viewRows []metricRow
		_ = storage.DB.Model(&models.Interaction{}).
			Select("entity_id as id, COUNT(*) as cnt").
			Where("entity_type = ? AND entity_id IN ? AND event_type = ?", models.EntityVideo, rentVideoIDs, models.EventVideoView).
			Group("entity_id").
			Scan(&viewRows).Error
		for _, r := range viewRows {
			rentInteractionViews[r.ID] = r.Cnt
		}
	}

	for _, v := range rentVideos {
		listingTitle := ""
		if v.PropertyID != nil {
			listingTitle = rentTitleByID[*v.PropertyID]
		}
		caption := strings.TrimSpace(v.Caption)
		if caption == "" {
			caption = listingTitle
		}
		views := studioMaxMetric(v.ViewCount, rentInteractionViews[v.ID])
		likes := v.LikesCount
		saves := v.SavesCount
		comments := v.CommentsCount
		rentCount++
		sumRentViews += views
		sumRentLikes += likes

		out = append(out, iris.Map{
			"kind":          "rent",
			"id":            v.ID,
			"property_id":   v.PropertyID,
			"listing_title": listingTitle,
			"caption":       caption,
			"video_url":     strings.TrimSpace(v.VideoURL),
			"thumbnail_url": strings.TrimSpace(v.ThumbnailURL),
			"status":        strings.ToLower(strings.TrimSpace(v.Status)),
			"duration_sec":  v.DurationSec,
			"created_at":    v.CreatedAt.Format(time.RFC3339),
			"metrics": iris.Map{
				"views":    views,
				"likes":    likes,
				"saves":    saves,
				"comments": comments,
			},
		})
	}

	saleIDs := make([]uint, 0, len(saleProps))
	saleTitleByID := map[uint]string{}
	saleViewCountByID := map[uint]int64{}
	for _, ps := range saleProps {
		saleIDs = append(saleIDs, ps.ID)
		saleTitleByID[ps.ID] = strings.TrimSpace(ps.Title)
		saleViewCountByID[ps.ID] = ps.ViewCount
	}

	if len(saleIDs) > 0 {
		type countRow struct {
			PropertySaleVideoID uint
			Cnt                 int64
		}
		likesBy := map[uint]int64{}
		savesBy := map[uint]int64{}
		commentsBy := map[uint]int64{}
		saleViewsByListing := map[uint]int64{}
		var likeRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoLike{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ?", saleIDs).
			Group("property_sale_video_id").Scan(&likeRows).Error
		for _, r := range likeRows {
			likesBy[r.PropertySaleVideoID] = r.Cnt
		}
		var saveRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoSave{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ?", saleIDs).
			Group("property_sale_video_id").Scan(&saveRows).Error
		for _, r := range saveRows {
			savesBy[r.PropertySaleVideoID] = r.Cnt
		}
		var commentRows []countRow
		_ = storage.DB.Model(&models.PropertySaleVideoComment{}).
			Select("property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_video_id IN ? AND parent_id IS NULL", saleIDs).
			Group("property_sale_video_id").Scan(&commentRows).Error
		for _, r := range commentRows {
			commentsBy[r.PropertySaleVideoID] = r.Cnt
		}
		var saleViewRows []countRow
		_ = storage.DB.Model(&models.Interaction{}).
			Select("property_sale_id as property_sale_video_id, COUNT(*) as cnt").
			Where("property_sale_id IN ? AND event_type = ?", saleIDs, models.EventVideoView).
			Group("property_sale_id").
			Scan(&saleViewRows).Error
		for _, r := range saleViewRows {
			saleViewsByListing[r.PropertySaleVideoID] = r.Cnt
		}

		var tableVideos []models.PropertySaleVideo
		_ = storage.DB.
			Where("property_sale_id IN ? AND deleted_at IS NULL", saleIDs).
			Order("created_at DESC").
			Find(&tableVideos).Error

		seenSaleURL := map[uint]map[string]bool{}

		for _, tv := range tableVideos {
			url := strings.TrimSpace(tv.VideoURL)
			if url == "" {
				continue
			}
			if seenSaleURL[tv.PropertySaleID] == nil {
				seenSaleURL[tv.PropertySaleID] = map[string]bool{}
			}
			seenSaleURL[tv.PropertySaleID][url] = true

			title := saleTitleByID[tv.PropertySaleID]
			caption := strings.TrimSpace(tv.Caption)
			if caption == "" {
				caption = title
			}
			listingViews := studioMaxMetric(saleViewCountByID[tv.PropertySaleID], saleViewsByListing[tv.PropertySaleID])
			views := studioMaxMetric(tv.ViewCount, listingViews)
			likes := studioMaxMetric(tv.LikesCount, likesBy[tv.PropertySaleID])
			saves := studioMaxMetric(tv.SavesCount, savesBy[tv.PropertySaleID])
			comments := studioMaxMetric(tv.CommentsCount, commentsBy[tv.PropertySaleID])
			saleCount++
			sumSaleViews += views
			sumSaleLikes += likes

			out = append(out, iris.Map{
				"kind":             "sale",
				"id":               tv.ID,
				"property_sale_id": tv.PropertySaleID,
				"listing_title":    title,
				"caption":          caption,
				"video_url":        url,
				"thumbnail_url":    strings.TrimSpace(tv.ThumbnailURL),
				"status":           strings.ToLower(strings.TrimSpace(tv.Status)),
				"duration_sec":     tv.DurationSec,
				"created_at":       tv.CreatedAt.Format(time.RFC3339),
				"metrics": iris.Map{
					"views":    views,
					"likes":    likes,
					"saves":    saves,
					"comments": comments,
				},
			})
		}

		for _, ps := range saleProps {
			if len(ps.Videos) == 0 {
				continue
			}
			listingViews := studioMaxMetric(ps.ViewCount, saleViewsByListing[ps.ID])
			likes := likesBy[ps.ID]
			saves := savesBy[ps.ID]
			comments := commentsBy[ps.ID]
			title := saleTitleByID[ps.ID]
			for i, rawURL := range ps.Videos {
				url := strings.TrimSpace(rawURL)
				if url == "" {
					continue
				}
				if seenSaleURL[ps.ID] != nil && seenSaleURL[ps.ID][url] {
					continue
				}
				saleCount++
				sumSaleViews += listingViews
				sumSaleLikes += likes

				out = append(out, iris.Map{
					"kind":             "sale",
					"id":               ps.ID*1000 + uint(i),
					"property_sale_id": ps.ID,
					"listing_title":    title,
					"caption":          title,
					"video_url":        url,
					"thumbnail_url":    url,
					"status":           strings.ToLower(strings.TrimSpace(ps.Status)),
					"metrics": iris.Map{
						"views":    listingViews,
						"likes":    likes,
						"saves":    saves,
						"comments": comments,
					},
				})
			}
		}
	}

	videoSummary := iris.Map{
		"rent_count":      rentCount,
		"sale_count":      saleCount,
		"rent_views":      sumRentViews,
		"rent_likes":      sumRentLikes,
		"sale_views":      sumSaleViews,
		"sale_likes":      sumSaleLikes,
		"total_views":     sumRentViews + sumSaleViews,
		"total_likes":     sumRentLikes + sumSaleLikes,
		"total_videos":    rentCount + saleCount,
	}

	return out, videoSummary
}

func loadHostPropertySales(userID uint) ([]models.PropertySale, error) {
	var organization models.Organization
	var hasOrg bool
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		hasOrg = true
	} else {
		var member models.OrganizationMember
		if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
			Preload("Organization").First(&member).Error; err == nil {
			organization = member.Organization
			hasOrg = true
		}
	}

	var properties []models.PropertySale
	q := storage.DB.Where("deleted_at IS NULL")
	if hasOrg {
		q = q.Where(
			"organization_id = ? OR (organization_id IS NULL AND owner_id = ?)",
			organization.ID, userID,
		)
	} else {
		q = q.Where("organization_id IS NULL AND owner_id = ?", userID)
	}
	err := q.
		Select("id", "title", "city", "status", "images", "view_count", "owner_id", "organization_id", "updated_at").
		Order("updated_at DESC").Limit(50).
		Find(&properties).Error
	return properties, err
}

func loadReservationCountsByProperty(rentIDs []uint) map[uint]int64 {
	out := map[uint]int64{}
	if len(rentIDs) == 0 {
		return out
	}
	type row struct {
		PropertyID uint
		Cnt        int64
	}
	var rows []row
	storage.DB.Model(&models.Reservation{}).
		Select("property_id, COUNT(*) as cnt").
		Where("property_id IN ? AND LOWER(status) IN ?", rentIDs, []string{"pending", "confirmed"}).
		Group("property_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.PropertyID] = r.Cnt
	}
	return out
}

type videoAggRow struct {
	PropertyID uint
	Views      int64
	Likes      int64
	Saves      int64
	Comments   int64
}

func loadVideoAggregatesByProperty(rentIDs []uint) map[uint]videoAggRow {
	out := map[uint]videoAggRow{}
	if len(rentIDs) == 0 {
		return out
	}
	var rows []videoAggRow
	storage.DB.Model(&models.Video{}).
		Select(`property_id,
			COALESCE(SUM(view_count),0) as views,
			COALESCE(SUM(likes_count),0) as likes,
			COALESCE(SUM(saves_count),0) as saves,
			COALESCE(SUM(comments_count),0) as comments`).
		Where("property_id IN ? AND deleted_at IS NULL", rentIDs).
		Group("property_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.PropertyID] = r
	}
	return out
}

func loadStudioInteractionTrends(since time.Time, rentIDs, saleIDs []uint) (map[string]studioDayMetrics, map[string]studioDayMetrics) {
	rentOut := map[string]studioDayMetrics{}
	saleOut := map[string]studioDayMetrics{}
	if len(rentIDs) == 0 && len(saleIDs) == 0 {
		return rentOut, saleOut
	}

	type row struct {
		Day            time.Time
		PropertyID     *uint
		PropertySaleID *uint
		EventType      string
		Cnt            int64
	}
	var rows []row

	const studioDayExpr = `(created_at AT TIME ZONE 'UTC')::date`

	q := storage.DB.Model(&models.Interaction{}).
		Select(studioDayExpr+` as day, property_id, property_sale_id, event_type, COUNT(*) as cnt`).
		Where("created_at >= ?", since).
		Where("event_type IN ?", []string{
			models.EventPropertyView,
			models.EventVideoView,
			models.EventSave,
			models.EventLike,
		})

	switch {
	case len(rentIDs) > 0 && len(saleIDs) > 0:
		q = q.Where("(property_id IN ? OR property_sale_id IN ?)", rentIDs, saleIDs)
	case len(rentIDs) > 0:
		q = q.Where("property_id IN ?", rentIDs)
	default:
		q = q.Where("property_sale_id IN ?", saleIDs)
	}

	if err := q.Group(studioDayExpr + ", property_id, property_sale_id, event_type").Scan(&rows).Error; err != nil {
		log.Printf("⚠️ GetHostStudio interaction trends: %v", err)
		return rentOut, saleOut
	}

	for _, r := range rows {
		metric := studioEventMetric(r.EventType)
		if metric == "" {
			continue
		}
		day := r.Day.Format("2006-01-02")
		cell := studioDayMetricKey(day, metric)
		if r.PropertySaleID != nil && *r.PropertySaleID > 0 {
			key := studioSaleKey(*r.PropertySaleID)
			if saleOut[key] == nil {
				saleOut[key] = studioDayMetrics{}
			}
			saleOut[key][cell] += r.Cnt
		} else if r.PropertyID != nil && *r.PropertyID > 0 {
			key := studioRentKey(*r.PropertyID)
			if rentOut[key] == nil {
				rentOut[key] = studioDayMetrics{}
			}
			rentOut[key][cell] += r.Cnt
		}
	}
	return rentOut, saleOut
}

func studioDayMetricKey(day, metric string) string {
	return day + "|" + metric
}

func studioSumMetric(trend studioDayMetrics, metric string) int64 {
	var n int64
	for k, v := range trend {
		if strings.HasSuffix(k, "|"+metric) {
			n += v
		}
	}
	return n
}

func mergeTrendIntoSummary(summary []int64, days []string, trend studioDayMetrics) {
	for i, day := range days {
		summary[i] += trend[studioDayMetricKey(day, "views")]
	}
}

func studioTrendPayload(days []string, trend studioDayMetrics) iris.Map {
	views := make([]int64, len(days))
	saves := make([]int64, len(days))
	likes := make([]int64, len(days))
	for i, day := range days {
		views[i] = trend[studioDayMetricKey(day, "views")]
		saves[i] = trend[studioDayMetricKey(day, "saves")]
		likes[i] = trend[studioDayMetricKey(day, "likes")]
	}
	return iris.Map{"days": days, "views": views, "saves": saves, "likes": likes}
}

func buildStudioDayLabels(n int) []string {
	out := make([]string, n)
	now := time.Now().UTC().Truncate(24 * time.Hour)
	for i := 0; i < n; i++ {
		out[i] = now.AddDate(0, 0, -(n - 1 - i)).Format("2006-01-02")
	}
	return out
}

func studioRentKey(id uint) string {
	return "rent:" + itoaU(id)
}

func studioSaleKey(id uint) string {
	return "sale:" + itoaU(id)
}

func studioEventMetric(eventType string) string {
	switch eventType {
	case models.EventPropertyView, models.EventVideoView:
		return "views"
	case models.EventSave:
		return "saves"
	case models.EventLike:
		return "likes"
	default:
		return ""
	}
}

func itoaU(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func firstRentPropertyImage(imagesJSON string) string {
	if imagesJSON == "" {
		return ""
	}
	var urls []string
	if err := json.Unmarshal([]byte(imagesJSON), &urls); err == nil && len(urls) > 0 {
		return strings.TrimSpace(urls[0])
	}
	return ""
}

func firstSaleImage(images []string) string {
	if len(images) == 0 {
		return ""
	}
	return strings.TrimSpace(images[0])
}
