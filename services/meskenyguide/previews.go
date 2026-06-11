package meskenyguide

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"gorm.io/gorm"
)

// ListingGuidePreview is the latest actionable note for a listing card.
type ListingGuidePreview struct {
	ID           uint   `json:"id"`
	PropertySaleID uint `json:"propertySaleId"`
	Diagnosis    string `json:"diagnosis"`
	Severity     string `json:"severity"`
	Status       string `json:"status"`
	Category     string `json:"category"`
	TriggerEvent string `json:"triggerEvent"`
	Locale       string `json:"locale"`
}

// GetListingPreviews returns the best current guide note per listing (non-dismissed).
func GetListingPreviews(hostID uint, listingIDs []uint) map[uint]ListingGuidePreview {
	out := make(map[uint]ListingGuidePreview)
	if len(listingIDs) == 0 {
		return out
	}

	var comments []models.GuideComment
	storage.DB.Where("host_id = ? AND parent_id IS NULL AND property_sale_id IN ?", hostID, listingIDs).
		Where("status NOT IN ?", []string{models.GuideStatusDismissed, models.GuideStatusResolved}).
		Order(`CASE severity WHEN 'urgent' THEN 0 WHEN 'action' THEN 1 ELSE 2 END,
			CASE status WHEN 'unread' THEN 0 WHEN 'read' THEN 1 ELSE 2 END,
			created_at DESC`).
		Find(&comments)

	seen := make(map[uint]bool)
	for _, c := range comments {
		if c.PropertySaleID == nil || seen[*c.PropertySaleID] {
			continue
		}
		seen[*c.PropertySaleID] = true
		out[*c.PropertySaleID] = ListingGuidePreview{
			ID:             c.ID,
			PropertySaleID: *c.PropertySaleID,
			Diagnosis:      c.Diagnosis,
			Severity:       c.Severity,
			Status:         c.Status,
			Category:       c.Category,
			TriggerEvent:   c.TriggerEvent,
			Locale:         c.Locale,
		}
	}
	return out
}

// GuideCategoryGroup groups comments for dashboard display.
type GuideCategoryGroup struct {
	Category string                   `json:"category"`
	Comments []models.GuideComment    `json:"comments"`
	Count    int                      `json:"count"`
}

// GetGuideGroupedByCategory returns active comments grouped by category.
func GetGuideGroupedByCategory(hostID uint, limitPerCategory int) []GuideCategoryGroup {
	if limitPerCategory < 1 {
		limitPerCategory = 5
	}
	categories := []string{"photo", "price", "seo", "timing", "engagement", "competitive"}
	var groups []GuideCategoryGroup
	for _, cat := range categories {
		var comments []models.GuideComment
		storage.DB.Where("host_id = ? AND parent_id IS NULL AND category = ?", hostID, cat).
			Where("status NOT IN ?", []string{models.GuideStatusDismissed}).
			Order(`CASE status WHEN 'unread' THEN 0 WHEN 'read' THEN 1 ELSE 2 END, created_at DESC`).
			Limit(limitPerCategory).
			Preload("PropertySale", func(db *gorm.DB) *gorm.DB {
				return db.Select("id", "title", "city")
			}).
			Find(&comments)
		if len(comments) == 0 {
			continue
		}
		var total int64
		storage.DB.Model(&models.GuideComment{}).
			Where("host_id = ? AND parent_id IS NULL AND category = ?", hostID, cat).
			Where("status NOT IN ?", []string{models.GuideStatusDismissed}).
			Count(&total)
		groups = append(groups, GuideCategoryGroup{
			Category: cat,
			Comments: comments,
			Count:    int(total),
		})
	}
	return groups
}
