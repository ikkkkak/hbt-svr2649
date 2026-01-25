package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"gorm.io/datatypes"
)

// FeedItem for unified feed (video = rent, property_sale = sale listing with videos)
type FeedItem struct {
	Type    string  `json:"type"`    // "video" | "property_sale"
	ID      uint    `json:"id"`      // Video.ID or PropertySale.ID
	ExtraID *uint   `json:"extraId,omitempty"` // Property.ID for video; nil for property_sale
	Score   float64 `json:"score,omitempty"`
}

// RecommendationTTL for cache
const RecommendationTTL = 5 * time.Minute

// RecommendationService implements hybrid feed generation and suggested content
type RecommendationService struct{}

// NewRecommendationService returns a new RecommendationService
func NewRecommendationService() *RecommendationService {
	return &RecommendationService{}
}

// GetFeedOptions for GetFeed
type GetFeedOptions struct {
	UserID   *uint
	DeviceID *string
	Limit    int
	Cursor   string
}

// GetFeed returns a personalized feed. Uses cache when valid; otherwise computes and caches.
func (s *RecommendationService) GetFeed(opt GetFeedOptions) (items []FeedItem, nextCursor string, hasMore bool, err error) {
	if opt.Limit <= 0 {
		opt.Limit = 10
	}
	if opt.Limit > 50 {
		opt.Limit = 50
	}

	// 1) Try cache
	if cached, _ := s.getCached(opt.UserID, opt.DeviceID); len(cached) > 0 {
		start := 0
		if opt.Cursor != "" {
			var cid uint
			if _, e := fmt.Sscanf(opt.Cursor, "%d", &cid); e == nil {
				for i, it := range cached {
					if it.ID == cid {
						start = i + 1
						break
					}
				}
			}
		}
		end := start + opt.Limit
		if end > len(cached) {
			end = len(cached)
			hasMore = false
		} else {
			hasMore = true
			if end < len(cached) {
				nextCursor = fmt.Sprintf("%d", cached[end-1].ID)
			}
		}
		chunk := cached[start:end]
		return chunk, nextCursor, hasMore, nil
	}

	// 2) Compute
	computed, err := s.computeFeed(opt)
	if err != nil {
		return nil, "", false, err
	}

	// 3) Cache full computed set (first 100)
	toCache := computed
	if len(toCache) > 100 {
		toCache = toCache[:100]
	}
	_ = s.setCached(opt.UserID, opt.DeviceID, toCache)

	// 4) Paginate
	start := 0
	if opt.Cursor != "" {
		var cid uint
		if _, e := fmt.Sscanf(opt.Cursor, "%d", &cid); e == nil {
			for i, it := range computed {
				if it.ID == cid {
					start = i + 1
					break
				}
			}
		}
	}
	end := start + opt.Limit
	if end > len(computed) {
		end = len(computed)
		hasMore = false
	} else {
		hasMore = true
		nextCursor = fmt.Sprintf("%d", computed[end-1].ID)
	}
	return computed[start:end], nextCursor, hasMore, nil
}

func (s *RecommendationService) getCached(userID *uint, deviceID *string) ([]FeedItem, bool) {
	var c models.RecommendationCache
	q := storage.DB.Where("computed_at >= ?", time.Now().Add(-RecommendationTTL))
	if userID != nil && *userID > 0 {
		q = q.Where("user_id = ?", *userID)
	} else if deviceID != nil && *deviceID != "" {
		q = q.Where("device_id = ? AND user_id IS NULL", *deviceID)
	} else {
		return nil, false
	}
	if err := q.Order("computed_at DESC").First(&c).Error; err != nil {
		return nil, false
	}
	var list []models.FeedItem
	if c.FeedSnapshot != nil {
		_ = json.Unmarshal(c.FeedSnapshot, &list)
	}
	out := make([]FeedItem, 0, len(list))
	for _, it := range list {
		out = append(out, FeedItem{Type: it.Type, ID: it.ID, ExtraID: it.ExtraID, Score: it.Score})
	}
	return out, true
}

func (s *RecommendationService) setCached(userID *uint, deviceID *string, items []FeedItem) error {
	list := make([]models.FeedItem, 0, len(items))
	for _, it := range items {
		list = append(list, models.FeedItem{Type: it.Type, ID: it.ID, ExtraID: it.ExtraID, Score: it.Score})
	}
	raw, _ := json.Marshal(list)
	rec := models.RecommendationCache{
		UserID:       userID,
		DeviceID:     deviceID,
		FeedSnapshot: datatypes.JSON(raw),
		ComputedAt:   time.Now(),
	}
	return storage.DB.Create(&rec).Error
}

// computeFeed: cold start = trending + recency; warm = behavioral boost + property similarity + freshness.
func (s *RecommendationService) computeFeed(opt GetFeedOptions) ([]FeedItem, error) {
	// Exclude recently seen (repetition fatigue)
	excludeVideoIDs, excludePropertySaleIDs := s.recentlySeen(opt.UserID, opt.DeviceID, 2*time.Hour, 3)

	// 1) Rent videos (Video with Property)
	var videos []struct {
		ID         uint
		PropertyID *uint
		Likes      int64
		Saves      int64
		Views      int64
		CreatedAt  time.Time
	}
	vq := storage.DB.Model(&models.Video{}).
		Select("videos.id, videos.property_id, videos.likes_count as likes, videos.saves_count as saves, videos.view_count as views, videos.created_at").
		Joins("LEFT JOIN properties ON videos.property_id = properties.id").
		Where("properties.id IS NOT NULL AND COALESCE(properties.is_active,?) = ? AND properties.status IN (?)", true, true, []string{"approved", "live"}).
		Where("(videos.status IS NULL OR LOWER(videos.status) <> ?)", "rejected").
		Where("COALESCE(videos.is_promotional,?) = ?", false, false)
	if len(excludeVideoIDs) > 0 {
		vq = vq.Where("videos.id NOT IN ?", excludeVideoIDs)
	}
	if err := vq.Find(&videos).Error; err != nil {
		log.Printf("recommendation.videos err: %v", err)
	}

	// 2) Sale listings with videos (PropertySale)
	var sales []struct {
		ID        uint
		Likes     int64
		Saves     int64
		Views     int64
		CreatedAt time.Time
	}
	sq := storage.DB.Model(&models.PropertySale{}).
		Select("property_sales.id, property_sales.view_count as views, property_sales.created_at").
		Where("(status = ? OR is_published = ?) AND (videos IS NOT NULL AND videos::text != '[]' AND videos::text != 'null')", "published", true).
		Where("COALESCE(is_deactivated,?) = ? AND COALESCE(is_sold,?) = ?", false, false, false)
	if len(excludePropertySaleIDs) > 0 {
		sq = sq.Where("property_sales.id NOT IN ?", excludePropertySaleIDs)
	}
	if err := sq.Find(&sales).Error; err != nil {
		log.Printf("recommendation.sales err: %v", err)
	}
	// Enrich sales with likes/saves from PropertySaleVideoLike/Save (keyed by property_sale_id in this codebase)
	for i := range sales {
		storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", sales[i].ID).Count(&sales[i].Likes)
		storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", sales[i].ID).Count(&sales[i].Saves)
	}

	// 3) User signals for warm start
	boostVideoIDs, boostPropertySaleIDs := s.userBoost(opt.UserID, opt.DeviceID)

	// 4) Score and merge
	type scored struct {
		Type string
		ID   uint
		Ext  *uint
		S    float64
	}
	var merged []scored

	for _, v := range videos {
		sc := s.scoreVideo(v.Likes, v.Saves, v.Views, v.CreatedAt, boostVideoIDs[v.ID])
		merged = append(merged, scored{Type: "video", ID: v.ID, Ext: v.PropertyID, S: sc})
	}
	for _, sa := range sales {
		sc := s.scorePropertySale(sa.Likes, sa.Saves, sa.Views, sa.CreatedAt, boostPropertySaleIDs[sa.ID])
		merged = append(merged, scored{Type: "property_sale", ID: sa.ID, Ext: nil, S: sc})
	}

	// 5) Sort by score desc, then by id for stability
	for i := 0; i < len(merged)-1; i++ {
		for j := i + 1; j < len(merged); j++ {
			if merged[j].S > merged[i].S || (merged[j].S == merged[i].S && merged[j].ID > merged[i].ID) {
				merged[i], merged[j] = merged[j], merged[i]
			}
		}
	}

	// 6) Convert to FeedItem
	out := make([]FeedItem, 0, len(merged))
	for _, m := range merged {
		out = append(out, FeedItem{Type: m.Type, ID: m.ID, ExtraID: m.Ext, Score: m.S})
	}
	return out, nil
}

func (s *RecommendationService) scoreVideo(likes, saves, views int64, created time.Time, boost float64) float64 {
	eng := float64(likes)*2.5 + float64(saves)*2.5 + math.Log1p(float64(views))
	recency := math.Max(0, 1 - time.Since(created).Hours()/168) // decay over 1 week
	return eng*0.7 + recency*0.3 + boost
}

func (s *RecommendationService) scorePropertySale(likes, saves, views int64, created time.Time, boost float64) float64 {
	eng := float64(likes)*2.5 + float64(saves)*2.5 + math.Log1p(float64(views))
	recency := math.Max(0, 1 - time.Since(created).Hours()/168)
	return eng*0.7 + recency*0.3 + boost
}

func (s *RecommendationService) recentlySeen(userID *uint, deviceID *string, within time.Duration, maxTimes int) (videoIDs []uint, propertySaleIDs []uint) {
	cut := time.Now().Add(-within)
	if userID != nil && *userID > 0 {
		storage.DB.Model(&models.VideoFeedHistory{}).Where("user_id = ? AND last_delivered_at >= ? AND seen_count >= ?", *userID, cut, maxTimes).
			Pluck("video_id", &videoIDs)
	}
	// Property sale: exclude IDs seen >= maxTimes in the window
	var counts []struct {
		PropertySaleID uint `gorm:"column:property_sale_id"`
	}
	base := storage.DB.Model(&models.Interaction{}).
		Select("property_sale_id").
		Where("event_type IN (?) AND created_at >= ? AND property_sale_id IS NOT NULL",
			[]string{models.EventVideoView, models.EventPropertyView}, cut)
	if userID != nil && *userID > 0 {
		base = base.Where("user_id = ?", *userID)
	} else if deviceID != nil && *deviceID != "" {
		base = base.Where("device_id = ?", *deviceID)
	} else {
		return videoIDs, nil
	}
	base.Group("property_sale_id").Having("COUNT(*) >= ?", maxTimes).Scan(&counts)
	for _, c := range counts {
		propertySaleIDs = append(propertySaleIDs, c.PropertySaleID)
	}
	return videoIDs, propertySaleIDs
}

func (s *RecommendationService) userBoost(userID *uint, deviceID *string) (videoIDs map[uint]float64, propertySaleIDs map[uint]float64) {
	videoIDs = make(map[uint]float64)
	propertySaleIDs = make(map[uint]float64)
	q := storage.DB.Model(&models.Interaction{}).Where("event_type IN (?) AND created_at >= ?",
		[]string{models.EventLike, models.EventSave, models.EventVideoView, models.EventPropertyView}, time.Now().Add(-7*24*time.Hour))
	if userID != nil && *userID > 0 {
		q = q.Where("user_id = ?", *userID)
	} else if deviceID != nil && *deviceID != "" {
		q = q.Where("device_id = ?", *deviceID)
	} else {
		return
	}
	var rows []models.Interaction
	if err := q.Find(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		switch r.EntityType {
		case models.EntityVideo:
			videoIDs[r.EntityID] += r.Weight * 0.5
		case models.EntityPropertySale, models.EntityPropertySaleVideo:
			id := r.EntityID
			if r.PropertySaleID != nil {
				id = *r.PropertySaleID
			}
			propertySaleIDs[id] += r.Weight * 0.5
		}
	}
	return videoIDs, propertySaleIDs
}

// GetSuggestedProperties returns properties similar to those the user viewed (after 1–2 videos or property views).
func (s *RecommendationService) GetSuggestedProperties(userID *uint, deviceID *string, limit int) ([]models.PropertySale, []models.Property, error) {
	if limit <= 0 {
		limit = 5
	}
	// From interactions, get recent property_id and property_sale_id
	var pid []uint
	var psid []uint
	q := storage.DB.Model(&models.Interaction{}).Where("event_type IN (?) AND created_at >= ?",
		[]string{models.EventVideoView, models.EventPropertyView}, time.Now().Add(-7*24*time.Hour))
	if userID != nil && *userID > 0 {
		q = q.Where("user_id = ?", *userID)
	} else if deviceID != nil && *deviceID != "" {
		q = q.Where("device_id = ?", *deviceID)
	} else {
		return nil, nil, nil
	}
	var rows []models.Interaction
	if err := q.Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		if r.PropertyID != nil {
			pid = append(pid, *r.PropertyID)
		}
		if r.PropertySaleID != nil {
			psid = append(psid, *r.PropertySaleID)
		}
	}
	// Similar: same city, similar type. For brevity we return recent PropertySale and Property matching city/type of viewed.
	var sales []models.PropertySale
	var props []models.Property
	if len(psid) > 0 {
		var ref models.PropertySale
		if err := storage.DB.First(&ref, psid[0]).Error; err == nil {
			storage.DB.Where("(status = ? OR is_published = ?) AND city_id = ? AND id NOT IN ?",
				"published", true, ref.CityID, psid).Limit(limit).Find(&sales)
		}
	}
	if len(pid) > 0 {
		var ref models.Property
		if err := storage.DB.First(&ref, pid[0]).Error; err == nil {
			storage.DB.Where("(is_active IS NULL OR is_active = ?) AND city_id = ? AND id NOT IN ?",
				true, ref.CityID, pid).Limit(limit).Find(&props)
		}
	}
	return sales, props, nil
}

// GetSuggestedVideosForProperty returns other videos for the same property (rent: Video; sale: PropertySaleVideo or PropertySale.Videos).
func (s *RecommendationService) GetSuggestedVideosForProperty(propertyKind string, propertyID, propertySaleID uint, limit int) (rent []models.Video, sale []models.PropertySaleVideo, saleListings []models.PropertySale) {
	if limit <= 0 {
		limit = 5
	}
	if propertyKind == "rent" && propertyID > 0 {
		storage.DB.Where("property_id = ?", propertyID).Limit(limit).Find(&rent)
		return rent, nil, nil
	}
	if propertyKind == "sale" && propertySaleID > 0 {
		storage.DB.Where("property_sale_id = ?", propertySaleID).Limit(limit).Find(&sale)
		var ps models.PropertySale
		if err := storage.DB.Where("id = ?", propertySaleID).First(&ps).Error; err == nil && len(ps.Videos) > 0 {
			saleListings = []models.PropertySale{ps}
		}
		return nil, sale, saleListings
	}
	return nil, nil, nil
}
