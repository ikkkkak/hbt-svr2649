package meskenyguide

import (
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// ListingMetrics holds computed signals for trigger evaluation.
type ListingMetrics struct {
	PropertySaleID uint
	HostID         uint
	Title          string
	PhotoCount     int
	ViewCount      int64
	ViewsLast6h    int64
	ViewsPrev6h    int64
	ViewsDeltaPct  float64
	InquiryCount   int64
	HoursSincePublish float64
	HoursSinceUpdate  float64
	PublishedAt    time.Time
}

func loadActiveSaleListings() ([]models.PropertySale, error) {
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	var rows []models.PropertySale
	err := storage.DB.
		Where("is_published = ? AND is_deactivated = ? AND is_sold = ? AND deleted_at IS NULL", true, false, false).
		Where("updated_at >= ?", cutoff).
		Find(&rows).Error
	return rows, err
}

func buildMetrics(ps models.PropertySale, now time.Time) ListingMetrics {
	photoCount := len(ps.Images)
	for _, cp := range ps.ClassifiedPhotos {
		photoCount += len(cp.Photos)
	}
	m := ListingMetrics{
		PropertySaleID: ps.ID,
		Title:          ps.Title,
		PhotoCount:     photoCount,
		ViewCount:      ps.ViewCount,
		PublishedAt:    ps.CreatedAt,
	}
	if ps.OwnerID != nil && *ps.OwnerID > 0 {
		m.HostID = *ps.OwnerID
	} else if ps.OrganizationID != nil {
		var org models.Organization
		if storage.DB.Select("owner_id").First(&org, *ps.OrganizationID).Error == nil {
			m.HostID = org.OwnerID
		}
	}
	m.HoursSincePublish = now.Sub(ps.CreatedAt).Hours()
	m.HoursSinceUpdate = now.Sub(ps.UpdatedAt).Hours()

	windowEnd := now
	windowMid := now.Add(-6 * time.Hour)
	windowStart := now.Add(-12 * time.Hour)

	storage.DB.Model(&models.Interaction{}).
		Where("property_sale_id = ? AND event_type = ? AND created_at >= ? AND created_at < ?",
			ps.ID, models.EventPropertyView, windowMid, windowEnd).
		Count(&m.ViewsLast6h)

	storage.DB.Model(&models.Interaction{}).
		Where("property_sale_id = ? AND event_type = ? AND created_at >= ? AND created_at < ?",
			ps.ID, models.EventPropertyView, windowStart, windowMid).
		Count(&m.ViewsPrev6h)

	if m.ViewsPrev6h > 0 {
		m.ViewsDeltaPct = (float64(m.ViewsLast6h-m.ViewsPrev6h) / float64(m.ViewsPrev6h)) * 100
	} else if m.ViewsLast6h > 0 {
		m.ViewsDeltaPct = 100
	}

	storage.DB.Model(&models.PropertyInquiry{}).
		Where("property_sale_id = ? AND deleted_at IS NULL", ps.ID).
		Count(&m.InquiryCount)

	return m
}

func signalsFromMetrics(m ListingMetrics, trigger string) models.JSONMap {
	return models.JSONMap{
		"trigger_event":        trigger,
		"photo_count":          m.PhotoCount,
		"view_count":           m.ViewCount,
		"views_last_6h":        m.ViewsLast6h,
		"views_prev_6h":        m.ViewsPrev6h,
		"views_delta_pct":      m.ViewsDeltaPct,
		"inquiry_count":        m.InquiryCount,
		"hours_since_publish":  m.HoursSincePublish,
		"hours_since_update":   m.HoursSinceUpdate,
		"listing_title":        m.Title,
	}
}
