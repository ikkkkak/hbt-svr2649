// Package goldproperty holds analytics counters for admin-assigned Gold listings.
// It intentionally does not import routes, discoverpush, or notification services:
// callers record events (feed impression, detail view, notification) after those layers run.
package goldproperty

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func upsertIncr(stat models.GoldPropertyStat, field string, delta int64) {
	if storage.DB == nil || stat.PropertySaleID == 0 || delta == 0 {
		return
	}
	qualified := "gold_property_stats." + field
	assign := map[string]interface{}{
		field:        gorm.Expr(qualified+" + ?", delta),
		"updated_at": time.Now(),
	}

	err := storage.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "property_sale_id"}},
		DoUpdates: clause.Assignments(assign),
	}).Create(&stat).Error
	if err != nil {
		log.Printf("goldproperty: upsert property_sale_id=%d field=%s: %v", stat.PropertySaleID, field, err)
	}
}

// RecordFeedImpressionsBatch increments feed_impressions for Gold properties in ids.
func RecordFeedImpressionsBatch(propertyIDs []uint) {
	if storage.DB == nil || len(propertyIDs) == 0 {
		return
	}
	var goldIDs []uint
	_ = storage.DB.Model(&models.PropertySale{}).
		Where("id IN ? AND is_gold = ?", propertyIDs, true).
		Pluck("id", &goldIDs).Error
	for _, id := range goldIDs {
		upsertIncr(models.GoldPropertyStat{
			PropertySaleID:  id,
			FeedImpressions: 1,
		}, "feed_impressions", 1)
	}
}

// RecordDetailView increments detail_views when a Gold listing earns a counted detail view.
func RecordDetailView(propertySaleID uint) {
	if propertySaleID == 0 {
		return
	}
	var n int64
	if err := storage.DB.Model(&models.PropertySale{}).
		Where("id = ? AND is_gold = ?", propertySaleID, true).
		Count(&n).Error; err != nil || n == 0 {
		return
	}
	upsertIncr(models.GoldPropertyStat{
		PropertySaleID: propertySaleID,
		DetailViews:    1,
	}, "detail_views", 1)
}

// RecordNotificationSent increments notifications_sent for a Gold property sale push.
func RecordNotificationSent(propertySaleID uint) {
	if propertySaleID == 0 {
		return
	}
	var n int64
	if err := storage.DB.Model(&models.PropertySale{}).
		Where("id = ? AND is_gold = ?", propertySaleID, true).
		Count(&n).Error; err != nil || n == 0 {
		return
	}
	upsertIncr(models.GoldPropertyStat{
		PropertySaleID:    propertySaleID,
		NotificationsSent: 1,
	}, "notifications_sent", 1)
}
