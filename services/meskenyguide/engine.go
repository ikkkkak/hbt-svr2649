package meskenyguide

import (
	"errors"
	"log"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"gorm.io/gorm"
)

var errForbidden = errors.New("forbidden")

// StartScheduler runs trigger + follow-up ticks every 15 minutes.
func StartScheduler() {
	if storage.DB == nil {
		log.Printf("⚠️ Meskeny Guide scheduler disabled: DB nil")
		return
	}
	log.Printf("🧭 Starting Meskeny Guide scheduler (15m)")
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		RunTriggerTick(time.Now())
		RunFollowUpTick(time.Now())
		for now := range ticker.C {
			RunTriggerTick(now)
			RunFollowUpTick(now)
		}
	}()
}

// RunTriggerTick evaluates active listings (Phase 1: views_drop, first_inquiry).
func RunTriggerTick(now time.Time) {
	listings, err := loadActiveSaleListings()
	if err != nil {
		log.Printf("meskeny guide: load listings: %v", err)
		return
	}
	for _, ps := range listings {
		m := buildMetrics(ps, now)
		if m.HostID == 0 {
			continue
		}
		if isHostPaused(m.HostID, ps.ID, now) {
			continue
		}

		// views_drop: −25% vs prior 6h
		if m.ViewsPrev6h >= 3 && m.ViewsDeltaPct <= -25 {
			if canCreateComment(m.HostID, ps.ID, models.GuideTriggerViewsDrop, now) &&
				!isCategorySuppressed(m.HostID, ps.ID, "engagement", now) {
				createGuideComment(ps.ID, m.HostID, models.GuideTriggerViewsDrop, models.GuideSeverityAction, m, now)
			}
		}

		// first_inquiry: 48h live, zero inquiries
		if m.HoursSincePublish >= 48 && m.InquiryCount == 0 {
			if canCreateComment(m.HostID, ps.ID, models.GuideTriggerFirstInquiry, now) &&
				!isCategorySuppressed(m.HostID, ps.ID, "photo", now) {
				createGuideComment(ps.ID, m.HostID, models.GuideTriggerFirstInquiry, models.GuideSeverityUrgent, m, now)
			}
		}
	}
}

// RunFollowUpTick generates impact reports 48h after implementation.
func RunFollowUpTick(now time.Time) {
	var due []models.GuideComment
	storage.DB.Where("status = ? AND parent_id IS NULL AND follow_up_scheduled_at IS NOT NULL AND follow_up_scheduled_at <= ?",
		models.GuideStatusImplemented, now).
		Find(&due)

	for _, parent := range due {
		if parent.PropertySaleID == nil {
			continue
		}
		var ps models.PropertySale
		if storage.DB.First(&ps, *parent.PropertySaleID).Error != nil {
			continue
		}
		m := buildMetrics(ps, now)
		if !canCreateComment(parent.HostID, *parent.PropertySaleID, models.GuideTriggerActionImpact, now) {
			storage.DB.Model(&parent).Update("follow_up_scheduled_at", nil)
			continue
		}
		signals := signalsFromMetrics(m, models.GuideTriggerActionImpact)
		signals["parent_comment_id"] = parent.ID
		if parent.HostAction != nil {
			signals["host_action"] = *parent.HostAction
		}
		createGuideCommentWithSignals(*parent.PropertySaleID, parent.HostID, models.GuideTriggerActionImpact,
			models.GuideSeverityInfo, m, signals, now)
		storage.DB.Model(&parent).Updates(map[string]interface{}{
			"follow_up_scheduled_at": nil,
			"status":                 models.GuideStatusResolved,
		})
	}
}

func createGuideComment(propertySaleID, hostID uint, trigger, severity string, m ListingMetrics, now time.Time) {
	signals := signalsFromMetrics(m, trigger)
	createGuideCommentWithSignals(propertySaleID, hostID, trigger, severity, m, signals, now)
}

func createGuideCommentWithSignals(propertySaleID, hostID uint, trigger, severity string, m ListingMetrics, signals models.JSONMap, now time.Time) {
	lang := ResolveHostLocale(hostID)
	if signals == nil {
		signals = models.JSONMap{}
	}
	signals["locale"] = lang

	diag, root, rx, impact, cat, tone := generateCommentContent(trigger, severity, lang, m, signals)

	saleID := propertySaleID
	c := models.GuideComment{
		ListingKind:      models.GuideListingSale,
		PropertySaleID:   &saleID,
		HostID:           hostID,
		Locale:           lang,
		TriggerEvent:     trigger,
		Severity:         severity,
		Category:         cat,
		Tone:             tone,
		Diagnosis:        diag,
		RootCause:        root,
		Prescription:     rx,
		ImpactForecast:   impact + disclaimerForLocale(lang),
		AlgorithmSignals: signals,
		Status:           models.GuideStatusUnread,
	}
	if err := storage.DB.Create(&c).Error; err != nil {
		log.Printf("meskeny guide create: %v", err)
		return
	}
	dispatchNotifications(&c, m.Title)
}

// MarkImplemented sets status and schedules impact follow-up.
func MarkImplemented(commentID, hostID uint) error {
	var c models.GuideComment
	if err := storage.DB.First(&c, commentID).Error; err != nil {
		return err
	}
	if c.HostID != hostID {
		return errForbidden
	}
	if c.ParentID != nil {
		return errForbidden
	}
	action := hostActionForCategory(c.Category)
	follow := time.Now().Add(48 * time.Hour)
	updates := map[string]interface{}{
		"status":                  models.GuideStatusImplemented,
		"host_action":             action,
		"follow_up_scheduled_at":  follow,
	}
	if err := storage.DB.Model(&c).Updates(updates).Error; err != nil {
		return err
	}
	if c.PropertySaleID != nil {
		resetDismissStreak(hostID, *c.PropertySaleID, time.Now())
	}
	return nil
}

// MarkDismissed dismisses a comment and applies category cooldown.
func MarkDismissed(commentID, hostID uint) error {
	var c models.GuideComment
	if err := storage.DB.First(&c, commentID).Error; err != nil {
		return err
	}
	if c.HostID != hostID || c.ParentID != nil {
		return errForbidden
	}
	if err := storage.DB.Model(&c).Update("status", models.GuideStatusDismissed).Error; err != nil {
		return err
	}
	if c.PropertySaleID != nil {
		recordDismiss(hostID, *c.PropertySaleID, c.Category, time.Now())
	}
	return nil
}

// AddHostReply creates a child comment (Ask Question).
func AddHostReply(parentID, hostID uint, body string) (*models.GuideComment, error) {
	var parent models.GuideComment
	if err := storage.DB.First(&parent, parentID).Error; err != nil {
		return nil, err
	}
	if parent.HostID != hostID {
		return nil, errForbidden
	}
	pid := parentID
	reply := models.GuideComment{
		ParentID:       &pid,
		HostID:         hostID,
		ListingKind:    parent.ListingKind,
		PropertySaleID: parent.PropertySaleID,
		PropertyID:     parent.PropertyID,
		LandmarkID:     parent.LandmarkID,
		TriggerEvent:   "host_question",
		Severity:       models.GuideSeverityInfo,
		Category:       parent.Category,
		Tone:           "clinical",
		Body:           body,
		Status:         models.GuideStatusRead,
	}
	if err := storage.DB.Create(&reply).Error; err != nil {
		return nil, err
	}
	return &reply, nil
}

// ManualTriggerListing creates a guide comment for QA (bypasses threshold checks except daily cap).
func ManualTriggerListing(propertySaleID uint, trigger string) error {
	var ps models.PropertySale
	if err := storage.DB.First(&ps, propertySaleID).Error; err != nil {
		return err
	}
	now := time.Now()
	m := buildMetrics(ps, now)
	if m.HostID == 0 {
		return errForbidden
	}
	severity := models.GuideSeverityAction
	if trigger == models.GuideTriggerFirstInquiry {
		severity = models.GuideSeverityUrgent
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var hostToday int64
	storage.DB.Model(&models.GuideComment{}).
		Where("host_id = ? AND parent_id IS NULL AND created_at >= ?", m.HostID, dayStart).
		Count(&hostToday)
	if hostToday >= maxCommentsPerHostPerDay {
		return errors.New("host daily guide cap reached")
	}
	createGuideComment(propertySaleID, m.HostID, trigger, severity, m, now)
	return nil
}

// MarkListingCommentsRead marks all guide comments on a listing as read for the host.
func MarkListingCommentsRead(hostID, propertySaleID uint) {
	now := time.Now()
	storage.DB.Model(&models.GuideComment{}).
		Where("host_id = ? AND property_sale_id = ? AND status = ? AND parent_id IS NULL",
			hostID, propertySaleID, models.GuideStatusUnread).
		Update("status", models.GuideStatusRead)

	var ids []uint
	storage.DB.Model(&models.GuideComment{}).Where("property_sale_id = ?", propertySaleID).Pluck("id", &ids)
	if len(ids) > 0 {
		storage.DB.Model(&models.GuideNotification{}).
			Where("host_id = ? AND read_at IS NULL AND comment_id IN ?", hostID, ids).
			Update("read_at", now)
	}
}

// ErrForbidden is returned when a host accesses another host's comment.
func ErrForbidden() error { return errForbidden }

// IsNotFound reports gorm not found.
func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
