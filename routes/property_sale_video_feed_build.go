package routes

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// buildSaleVideoFeedLitePage builds page-1 sale video feed (fast path, lite JSON).
func buildSaleVideoFeedLitePage(userID uint, limit int) ([]map[string]interface{}, string, bool, error) {
	if limit < 1 || limit > 20 {
		limit = 8
	}

	query := storage.DB.Model(&models.PropertySale{}).
		Where("(status = ? OR is_published = ? OR status IS NULL) AND COALESCE(is_deactivated, false) = ?", "published", true, false).
		Where(`(
			(property_sales.videos IS NOT NULL AND property_sales.videos::text NOT IN ('[]','null',''))
			OR EXISTS (SELECT 1 FROM property_sale_videos psv WHERE psv.property_sale_id = property_sales.id AND psv.deleted_at IS NULL)
		)`).
		Omit(
			"Description", "DescriptionTranslations",
			"FloorPlans", "Neighborhood",
			"Features", "Amenities", "HostPrivateNote", "VerificationNotes", "VirtualTour",
		).
		Preload("Organization", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name", "logo", "banner_image", "owner_id")
		}).
		Preload("Owner", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "first_name", "last_name", "avatar_url")
		})

	if userID > 0 {
		query = query.Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID)
	}

	query = query.Order("property_sales.created_at DESC, property_sales.id DESC")

	var propertySales []models.PropertySale
	if err := query.Limit(limit).Find(&propertySales).Error; err != nil {
		return nil, "", false, err
	}

	propertyIDs := make([]uint, 0, len(propertySales))
	for _, ps := range propertySales {
		propertyIDs = append(propertyIDs, ps.ID)
	}

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

	var videos []map[string]interface{}
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

		for i, videoURL := range videoURLs {
			videoID := fmt.Sprintf("%d_%d", ps.ID, i)
			orgLogo, orgName, orgID := "", "", uint(0)
			orgOwnerID := uint(0)
			if ps.OrganizationID != nil && *ps.OrganizationID > 0 && ps.Organization != nil {
				orgName = ps.Organization.Name
				orgLogo = ps.Organization.Logo
				orgOwnerID = ps.Organization.OwnerID
				orgID = ps.Organization.ID
			}
			profileUserID := orgOwnerID
			if profileUserID == 0 && ps.OwnerID != nil {
				profileUserID = *ps.OwnerID
			}
			if profileUserID == 0 && ps.Owner != nil {
				profileUserID = ps.Owner.ID
			}
			thumb := ""
			if i < len(tableThumbs) {
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

			video := map[string]interface{}{
				"ID":             videoID,
				"propertySaleID": ps.ID,
				"propertySale":   propertySaleFeedLite(ps),
				"userID":         profileUserID,
				"videoURL":       videoURL,
				"thumbnailURL":   thumb,
				"durationSec":    0,
				"caption":        ps.Title,
				"likesCount":     likesCountByProperty[ps.ID],
				"commentsCount":  commentsCountByProperty[ps.ID],
				"savesCount":     savesCountByProperty[ps.ID],
				"viewCount":      0,
				"isFlagged":      false,
				"status":         "approved",
				"liked":          userLikedByProperty[ps.ID],
				"saved":          userSavedByProperty[ps.ID],
				"organization": map[string]interface{}{
					"id": orgID, "name": orgName, "logoURL": orgLogo,
				},
				"CreatedAt": ps.CreatedAt,
				"UpdatedAt": ps.UpdatedAt,
			}
			if tv != nil {
				applyPropertySaleVideoStreamFields(video, tv)
			}
			videos = append(videos, video)
		}
	}

	hasMore := len(propertySales) >= limit
	nextCursor := ""
	if hasMore {
		nextCursor = "2"
	}
	return videos, nextCursor, hasMore, nil
}
