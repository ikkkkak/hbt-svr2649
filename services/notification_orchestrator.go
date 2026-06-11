package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

// Notification types in Meskeny (for mapping / analytics).
// marketing / smart_* → lower default relevance; transactional → higher.
const (
	OrchestratorDefaultDailyLimit = 4
)

// NotificationOrchestratorEnabled turns on candidate queue + scoring for all wired send paths.
// Set MESKENY_NOTIFICATION_ORCHESTRATOR=true
func NotificationOrchestratorEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("MESKENY_NOTIFICATION_ORCHESTRATOR")))
	return v == "1" || v == "true" || v == "yes"
}

// NotificationCandidateInput is the unified ingest shape (internal + HTTP).
type NotificationCandidateInput struct {
	UserID uint `json:"user_id"`

	NotificationType string `json:"notification_type"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	ImageURL         string `json:"image_url"`

	// Data becomes JSON payload (deep links, screens, etc.)
	Data map[string]string `json:"-"`

	// Scoring helpers (optional — defaults inferred from type)
	RelevanceScore *int    `json:"relevance_score"`
	UrgencyLevel   string  `json:"urgency_level"` // critical, high, normal, low
	MatchScore     *int    `json:"match_score"`
	PropertySaleID *uint   `json:"property_sale_id"`
	PayloadRaw     json.RawMessage `json:"payload"` // optional JSON from HTTP API
}

func randomUUIDv4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func defaultRelevanceForType(notificationType string) int {
	typ := strings.TrimSpace(strings.ToLower(notificationType))
	switch typ {
	case "new_message", "chat_message", "direct_message":
		return 88
	case "reservation_request", "reservation_confirmed", "reservation_cancelled", "reservation_expired", "reservation_update":
		return 92
	case "property_offer", "offer", "property_tour", "tour_request":
		return 90
	case "price_drop":
		return 85
	case "new_property", "new_match", "match_suggestion", "host_contact":
		return 82
	case "meskeny_guide", "guide_comment":
		return 88
	case "video_comment", "video_like", "video_interaction":
		return 75
	case "welcome":
		return 60
	case "reengage_digest", "weekly_digest", "smart_weekly_digest":
		return 45
	default:
		if strings.HasPrefix(typ, "smart_") {
			return 58
		}
		return 70
	}
}

func defaultUrgencyForType(notificationType string) string {
	typ := strings.TrimSpace(strings.ToLower(notificationType))
	switch typ {
	case "reservation_request", "property_offer", "offer", "tour_request", "property_tour", "new_message", "chat_message", "price_drop":
		return "high"
	case "meskeny_guide", "guide_comment":
		return "high"
	case "host_contact", "new_match":
		return "high"
	case "welcome", "reengage_digest", "weekly_digest":
		return "low"
	default:
		if strings.HasPrefix(typ, "smart_") {
			return "normal"
		}
		return "normal"
	}
}

func urgencyWeight(urgency string) int {
	switch strings.ToLower(strings.TrimSpace(urgency)) {
	case "critical":
		return 100
	case "high":
		return 80
	case "low":
		return 30
	default:
		return 50
	}
}

// SubmitNotificationCandidate persists a candidate and runs the decision engine asynchronously.
func SubmitNotificationCandidate(in NotificationCandidateInput) (string, error) {
	if storage.DB == nil {
		return "", fmt.Errorf("database not initialized")
	}
	if in.UserID == 0 {
		return "", fmt.Errorf("user_id required")
	}

	var user models.User
	if err := storage.DB.Select("id", "allows_notifications").First(&user, in.UserID).Error; err != nil {
		return "", fmt.Errorf("user not found")
	}
	if user.AllowsNotifications == nil || !*user.AllowsNotifications {
		return "", fmt.Errorf("notifications disabled for user")
	}

	rel := defaultRelevanceForType(in.NotificationType)
	if in.RelevanceScore != nil {
		rel = *in.RelevanceScore
	}
	if rel < 0 {
		rel = 0
	}
	if rel > 100 {
		rel = 100
	}

	urg := strings.TrimSpace(in.UrgencyLevel)
	if urg == "" {
		urg = defaultUrgencyForType(in.NotificationType)
	}

	payloadBytes := []byte(`{}`)
	if len(in.PayloadRaw) > 0 && json.Valid(in.PayloadRaw) {
		payloadBytes = in.PayloadRaw
	} else if in.Data != nil {
		// Merge data + metadata for client deep links
		m := make(map[string]interface{}, len(in.Data)+4)
		for k, v := range in.Data {
			m[k] = v
		}
		m["notification_type"] = in.NotificationType
		if in.PropertySaleID != nil {
			m["property_sale_id"] = *in.PropertySaleID
		}
		var err error
		payloadBytes, err = json.Marshal(m)
		if err != nil {
			payloadBytes = []byte(`{}`)
		}
	}

	id := randomUUIDv4()
	row := models.NotificationCandidate{
		ID:               id,
		UserID:           in.UserID,
		NotificationType: strings.TrimSpace(in.NotificationType),
		Title:            in.Title,
		Body:             in.Body,
		Payload:          datatypes.JSON(payloadBytes),
		ImageURL:         strings.TrimSpace(in.ImageURL),
		RelevanceScore:   rel,
		UrgencyLevel:     urg,
		PropertySaleID:   in.PropertySaleID,
		MatchScore:       in.MatchScore,
		AIDecision:       "pending",
		RequestedAt:      time.Now(),
	}

	if err := storage.DB.Create(&row).Error; err != nil {
		return "", err
	}

	go processNotificationCandidate(id)
	return id, nil
}

func effectiveDailyLimit(userID uint) int {
	defaultLim := OrchestratorDefaultDailyLimit
	var learned models.UserNotificationLearned
	if err := storage.DB.Where("user_id = ?", userID).First(&learned).Error; err == nil && learned.DailyLimitOverride > 0 {
		return learned.DailyLimitOverride
	}
	return defaultLim
}

func countUserSendsLast24h(userID uint, now time.Time) int64 {
	since := now.Add(-24 * time.Hour)
	var n int64
	_ = storage.DB.Model(&models.NotificationCandidate{}).
		Where("user_id = ? AND sent_at IS NOT NULL AND sent_at >= ?", userID, since).
		Where("ai_decision IN ?", []string{"send", "digest_sent"}).
		Count(&n).Error
	return n
}

func loadLearnedProfile(userID uint) models.UserNotificationLearned {
	var learned models.UserNotificationLearned
	if err := storage.DB.Where("user_id = ?", userID).First(&learned).Error; err != nil {
		learned = models.UserNotificationLearned{
			ID:               randomUUIDv4(),
			UserID:           userID,
			OpenRate7d:       0.4,
			DismissRate7d:    0.2,
			QuietHoursStart:  23,
			QuietHoursEnd:    7,
			MatchOpenRate:    0.5,
			MessageOpenRate:  0.5,
		}
		_ = storage.DB.Create(&learned).Error
	}

	// Sync quiet hours from device notification_preferences when AI row is generic
	var pref models.NotificationPreference
	if err := storage.DB.Where("user_id = ? AND enabled = ?", userID, true).Order("updated_at DESC").First(&pref).Error; err == nil {
		if pref.QuietStartHour >= 0 && pref.QuietStartHour <= 23 {
			learned.QuietHoursStart = pref.QuietStartHour
		}
		if pref.QuietEndHour >= 0 && pref.QuietEndHour <= 23 {
			learned.QuietHoursEnd = pref.QuietEndHour
		}
	}
	return learned
}

func localHourForUser(userID uint, now time.Time) (int, *time.Location) {
	var pref models.NotificationPreference
	loc := time.UTC
	if err := storage.DB.Where("user_id = ? AND enabled = ?", userID, true).Order("updated_at DESC").First(&pref).Error; err == nil {
		if tz := strings.TrimSpace(pref.Timezone); tz != "" {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
	}
	return now.In(loc).Hour(), loc
}

func timingFitScore(learned models.UserNotificationLearned, userID uint, now time.Time, urgency string) int {
	if learned.DoNotDisturbEnabled && strings.ToLower(urgency) != "critical" {
		h, _ := localHourForUser(userID, now)
		qs, qe := learned.QuietHoursStart, learned.QuietHoursEnd
		if qs <= qe {
			if h >= qs && h < qe {
				return 0
			}
		} else {
			if h >= qs || h < qe {
				return 0
			}
		}
	}

	h, _ := localHourForUser(userID, now)
	if learned.PeakOpenHour != nil && *learned.PeakOpenHour == h {
		return 100
	}
	if learned.PreferredHourStart != nil && learned.PreferredHourEnd != nil {
		ps, pe := *learned.PreferredHourStart, *learned.PreferredHourEnd
		if ps <= pe {
			if h >= ps && h <= pe {
				return 70
			}
		}
		return 30
	}
	// Default “reasonable daytime” without learned prefs
	if h >= 8 && h <= 22 {
		return 60
	}
	return 35
}

func quotaPressureScore(sentLast24h int64, dailyLimit int) int {
	switch {
	case sentLast24h <= 0:
		return 100
	case sentLast24h == 1:
		return 80
	case sentLast24h == 2:
		return 60
	case sentLast24h == 3:
		return 40
	default:
		return 0
	}
}

func engagementScore(learned models.UserNotificationLearned) int {
	if learned.DismissRate7d > 0.7 {
		return 10
	}
	or := learned.OpenRate7d
	if or > 0.5 {
		return 100
	}
	if or >= 0.3 {
		return 70
	}
	return 40
}

func computeAIScore(baseRel, urgW, timing, quotaP, engage int) int {
	s := float64(baseRel)*0.40 + float64(urgW)*0.25 + float64(timing)*0.15 + float64(quotaP)*0.10 + float64(engage)*0.10
	sc := int(s + 0.5)
	if sc < 0 {
		return 0
	}
	if sc > 100 {
		return 100
	}
	return sc
}

func decideAction(aiScore int, quotaLeft int, urgency string, timingFit int) (decision, reason string) {
	urg := strings.ToLower(strings.TrimSpace(urgency))
	isCritical := urg == "critical"

	// Hard stop: no quota left (spec) — only critical could theoretically bypass; we still enforce cap.
	if quotaLeft <= 0 && !isCritical {
		if timingFit < 40 {
			return "drop", "Daily quota exhausted and timing poor; dropping to protect UX"
		}
		return "batch", "Daily quota exhausted; routing to digest pool"
	}
	if quotaLeft <= 0 && isCritical {
		return "drop", "Daily quota exhausted (hard cap 4/24h); critical event still blocked by policy"
	}

	if aiScore >= 90 && quotaLeft > 0 {
		return "send", fmt.Sprintf("High value score %d with quota remaining", aiScore)
	}
	if aiScore >= 75 && aiScore <= 89 && quotaLeft >= 2 {
		return "send", fmt.Sprintf("Strong score %d, enough quota headroom", aiScore)
	}
	if aiScore >= 75 && aiScore <= 89 && quotaLeft < 2 {
		return "delay", fmt.Sprintf("Score %d acceptable but low quota (%d)", aiScore, quotaLeft)
	}
	if aiScore >= 60 && aiScore <= 74 {
		if quotaLeft >= 2 {
			return "send", fmt.Sprintf("Moderate score %d, sending while quota allows", aiScore)
		}
		return "batch", fmt.Sprintf("Moderate score %d with tight quota — batching", aiScore)
	}
	if aiScore < 60 {
		if quotaLeft >= 2 {
			return "batch", fmt.Sprintf("Lower score %d — batching to reduce interruption", aiScore)
		}
		return "drop", fmt.Sprintf("Low score %d and insufficient quota", aiScore)
	}
	return "send", "default send"
}

func nextScheduledSend(userID uint, now time.Time) time.Time {
	learned := loadLearnedProfile(userID)
	_, loc := localHourForUser(userID, now)
	targetH := 18
	if learned.PeakOpenHour != nil {
		targetH = *learned.PeakOpenHour
	}
	localNow := now.In(loc)
	candidate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), targetH, 5, 0, 0, loc)
	if !candidate.After(localNow) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate.UTC()
}

func processNotificationCandidate(id string) {
	if storage.DB == nil {
		return
	}
	var c models.NotificationCandidate
	if err := storage.DB.Where("id = ?", id).First(&c).Error; err != nil {
		return
	}
	if c.AIDecision != "pending" {
		return
	}

	now := time.Now()
	limit := effectiveDailyLimit(c.UserID)
	sentN := countUserSendsLast24h(c.UserID, now)
	quotaLeft := int(limit) - int(sentN)
	if quotaLeft < 0 {
		quotaLeft = 0
	}

	learned := loadLearnedProfile(c.UserID)
	timing := timingFitScore(learned, c.UserID, now, c.UrgencyLevel)
	urgW := urgencyWeight(c.UrgencyLevel)
	qp := quotaPressureScore(sentN, limit)
	eng := engagementScore(learned)
	ai := computeAIScore(c.RelevanceScore, urgW, timing, qp, eng)

	decision, reason := decideAction(ai, quotaLeft, c.UrgencyLevel, timing)

	c.AIScore = ai
	c.AIReason = reason

	switch decision {
	case "send":
		if timing == 0 && strings.ToLower(c.UrgencyLevel) != "critical" {
			t := nextScheduledSend(c.UserID, now)
			c.AIDecision = "delay"
			c.ScheduledFor = &t
			c.AIReason = "Quiet hours — delayed to next preferred window"
		} else {
			c.AIDecision = "send"
			if deliverCandidate(&c) {
				t := now
				c.SentAt = &t
				c.Delivered = true
			} else {
				c.AIDecision = "drop"
				c.AIReason = "Delivery failed (no tokens or transport error)"
			}
		}
	case "delay":
		t := nextScheduledSend(c.UserID, now)
		c.AIDecision = "delay"
		c.ScheduledFor = &t
	case "batch":
		c.AIDecision = "batch"
		t := nextScheduledSend(c.UserID, now)
		c.ScheduledFor = &t
	case "drop":
		c.AIDecision = "drop"
	}

	_ = storage.DB.Model(&models.NotificationCandidate{}).Where("id = ?", id).Updates(map[string]interface{}{
		"ai_score":      c.AIScore,
		"ai_decision":   c.AIDecision,
		"ai_reason":     c.AIReason,
		"scheduled_for": c.ScheduledFor,
		"sent_at":       c.SentAt,
		"delivered":     c.Delivered,
	}).Error
}

func payloadToDataMap(payload datatypes.JSON) map[string]string {
	out := map[string]string{}
	if len(payload) == 0 {
		return out
	}
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		return out
	}
	for k, v := range m {
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64:
			out[k] = fmt.Sprintf("%.0f", t)
		default:
			b, _ := json.Marshal(v)
			out[k] = string(b)
		}
	}
	return out
}

func deliverCandidate(c *models.NotificationCandidate) bool {
	data := payloadToDataMap(c.Payload)
	if c.NotificationType != "" && data["type"] == "" {
		data["type"] = c.NotificationType
	}
	data["candidate_id"] = c.ID
	ns := NotificationServiceInstance
	ok := ns.DeliverPushDirectToUser(c.UserID, c.Title, c.Body, c.ImageURL, data)
	if ok {
		logOrchestratorLegacyDelivery(c, data)
	}
	return ok
}

func logOrchestratorLegacyDelivery(c *models.NotificationCandidate, data map[string]string) {
	if storage.DB == nil {
		return
	}
	legacy := strings.TrimSpace(data["legacy_event_type"])
	if legacy == "" {
		return
	}
	kind := strings.TrimSpace(data["legacy_property_kind"])
	if kind == "" {
		kind = "sale"
	}
	now := time.Now()
	timeWindow := now.Format("2006-01-02")

	if kind == "rent" {
		var propRent *uint
		if pid := strings.TrimSpace(data["propertyId"]); pid != "" {
			if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
				v := uint(id)
				propRent = &v
			}
		}
		if propRent == nil {
			return
		}
		fp := models.BuildFingerprint(c.UserID, "rent", propRent, nil, legacy, timeWindow)
		logEntry := models.NotificationDeliveryLog{
			UserID:       c.UserID,
			EventType:    legacy,
			PropertyKind: "rent",
			PropertyID:   propRent,
			Fingerprint:  fp,
			ABVariant:    ABVariantForUser(c.UserID, legacy),
		}
		_ = storage.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&logEntry).Error
		return
	}

	var propSale *uint
	if c.PropertySaleID != nil {
		propSale = c.PropertySaleID
	} else if pid := strings.TrimSpace(data["propertyId"]); pid != "" {
		if id, err := strconv.ParseUint(pid, 10, 32); err == nil {
			v := uint(id)
			propSale = &v
		}
	}
	if propSale == nil {
		return
	}
	fp := models.BuildFingerprint(c.UserID, "sale", nil, propSale, legacy, timeWindow)
	logEntry := models.NotificationDeliveryLog{
		UserID:         c.UserID,
		EventType:      legacy,
		PropertyKind:   "sale",
		PropertySaleID: propSale,
		Fingerprint:    fp,
		ABVariant:      ABVariantForUser(c.UserID, legacy),
	}
	_ = storage.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&logEntry).Error
}

// StartNotificationOrchestratorWorkers periodic processors for delayed + batch queues.
func StartNotificationOrchestratorWorkers() {
	if storage.DB == nil {
		return
	}
	log.Printf("🚀 Starting notification orchestrator workers (enabled=%v)", NotificationOrchestratorEnabled())

	go func() {
		t := time.NewTicker(1 * time.Minute)
		defer t.Stop()
		for range t.C {
			if !NotificationOrchestratorEnabled() {
				continue
			}
			processDueDelayedCandidates()
			tryComposeBatchDigests()
		}
	}()
}

func processDueDelayedCandidates() {
	now := time.Now()
	var rows []models.NotificationCandidate
	_ = storage.DB.Where("ai_decision = ? AND scheduled_for IS NOT NULL AND scheduled_for <= ?", "delay", now).
		Limit(100).
		Find(&rows).Error
	for _, r := range rows {
		// Re-queue as pending to re-evaluate quota/timing
		_ = storage.DB.Model(&models.NotificationCandidate{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
			"ai_decision":   "pending",
			"scheduled_for": nil,
		}).Error
		go processNotificationCandidate(r.ID)
	}

	var batchDue []models.NotificationCandidate
	_ = storage.DB.Where("ai_decision = ? AND scheduled_for IS NOT NULL AND scheduled_for <= ?", "batch", now).
		Limit(100).
		Find(&batchDue).Error
	for _, r := range batchDue {
		_ = storage.DB.Model(&models.NotificationCandidate{}).Where("id = ?", r.ID).Updates(map[string]interface{}{
			"ai_decision":   "pending",
			"scheduled_for": nil,
		}).Error
		go processNotificationCandidate(r.ID)
	}
}

func tryComposeBatchDigests() {
	var userIDs []uint
	_ = storage.DB.Raw(`
		SELECT user_id FROM notification_candidates
		WHERE ai_decision = 'batch' AND sent_at IS NULL
		GROUP BY user_id
		HAVING COUNT(*) >= 2
		LIMIT 50
	`).Scan(&userIDs).Error

	for _, uid := range userIDs {
		if uid == 0 {
			continue
		}
		var items []models.NotificationCandidate
		_ = storage.DB.Where("user_id = ? AND ai_decision = ? AND sent_at IS NULL", uid, "batch").
			Order("created_at ASC").
			Limit(8).
			Find(&items).Error
		if len(items) < 2 {
			continue
		}
		if countUserSendsLast24h(uid, time.Now()) >= int64(effectiveDailyLimit(uid)) {
			continue
		}

		batchID := randomUUIDv4()
		parts := make([]string, 0, len(items))
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.ID)
			p := strings.TrimSpace(it.NotificationType)
			if p == "" {
				p = "update"
			}
			parts = append(parts, p)
		}
		title := "Meskeny — plusieurs mises à jour"
		body := fmt.Sprintf("%d notifications: %s", len(items), strings.Join(parts, ", "))
		digestID := randomUUIDv4()
		payload, _ := json.Marshal(map[string]interface{}{
			"type":                "notification_digest",
			"candidate_ids":       ids,
			"batch_id":            batchID,
			"candidate_id":        digestID,
			"screen":              "Inbox",
			"action":              "open_notification_inbox",
			"notification_type":   "suggestion_digest",
		})

		ns := NotificationServiceInstance
		data := payloadToDataMap(datatypes.JSON(payload))
		data["candidate_id"] = digestID
		if !ns.DeliverPushDirectToUser(uid, title, body, "", data) {
			continue
		}

		now := time.Now()
		bid := batchID
		digestRow := models.NotificationCandidate{
			ID:               digestID,
			UserID:           uid,
			NotificationType: "suggestion_digest",
			Title:            title,
			Body:             body,
			Payload:          datatypes.JSON(payload),
			RelevanceScore:   65,
			UrgencyLevel:     "normal",
			AIScore:          72,
			AIDecision:       "digest_sent",
			AIReason:         "Composed batch digest (1 push for multiple candidates)",
			RequestedAt:      now,
			SentAt:           &now,
			Delivered:        true,
			BatchID:          &bid,
		}
		_ = storage.DB.Create(&digestRow).Error

		for _, it := range items {
			_ = storage.DB.Model(&models.NotificationCandidate{}).Where("id = ?", it.ID).Updates(map[string]interface{}{
				"ai_decision": "batched",
				"batch_id":    batchID,
			}).Error
		}
	}
}
