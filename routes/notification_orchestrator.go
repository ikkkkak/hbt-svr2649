package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

func orchestratorInternalKeyOK(ctx iris.Context) bool {
	want := strings.TrimSpace(os.Getenv("ORCHESTRATOR_INTERNAL_KEY"))
	if want == "" {
		ctx.StatusCode(503)
		ctx.JSON(iris.Map{"error": "ORCHESTRATOR_INTERNAL_KEY is not configured"})
		return false
	}
	if ctx.GetHeader("X-Internal-Key") != want {
		ctx.StatusCode(401)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return false
	}
	return true
}

// POST /api/orchestrator/candidates — backend ingest (internal key).
func PostOrchestratorCandidatesInternal(ctx iris.Context) {
	if !orchestratorInternalKeyOK(ctx) {
		return
	}
	var body struct {
		UserID           uint            `json:"user_id"`
		NotificationType string          `json:"notification_type"`
		Title            string          `json:"title"`
		Body             string          `json:"body"`
		ImageURL         string          `json:"image_url"`
		RelevanceScore   *int            `json:"relevance_score"`
		UrgencyLevel     string          `json:"urgency_level"`
		MatchScore       *int            `json:"match_score"`
		PropertySaleID   *uint           `json:"property_sale_id"`
		Payload          json.RawMessage `json:"payload"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "invalid_json"})
		return
	}
	id, err := services.SubmitNotificationCandidate(services.NotificationCandidateInput{
		UserID:           body.UserID,
		NotificationType: body.NotificationType,
		Title:            body.Title,
		Body:             body.Body,
		ImageURL:         body.ImageURL,
		RelevanceScore:   body.RelevanceScore,
		UrgencyLevel:     body.UrgencyLevel,
		MatchScore:       body.MatchScore,
		PropertySaleID:   body.PropertySaleID,
		PayloadRaw:       body.Payload,
	})
	if err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.StatusCode(202)
	ctx.JSON(iris.Map{
		"candidate_id":          id,
		"status":                "queued",
		"estimated_decision_ms": 500,
	})
}

// POST /api/orchestrator/feedback — mobile app.
func PostOrchestratorFeedback(ctx iris.Context) {
	uidAny := ctx.Values().Get("userID")
	uid, ok := uidAny.(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(401)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}
	var body struct {
		CandidateID string `json:"candidate_id"`
		Event       string `json:"event"`
		Timestamp   string `json:"timestamp"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "invalid_json"})
		return
	}
	var c models.NotificationCandidate
	if err := storage.DB.Where("id = ? AND user_id = ?", body.CandidateID, uid).First(&c).Error; err != nil {
		ctx.StatusCode(404)
		ctx.JSON(iris.Map{"error": "candidate_not_found"})
		return
	}
	ts := time.Now().UTC()
	if body.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, body.Timestamp); err == nil {
			ts = t
		}
	}
	ev := strings.ToLower(strings.TrimSpace(body.Event))
	switch ev {
	case "opened":
		_ = storage.DB.Model(&c).Updates(map[string]interface{}{
			"opened":    true,
			"opened_at": ts,
		}).Error
	case "dismissed":
		_ = storage.DB.Model(&c).Updates(map[string]interface{}{
			"dismissed":    true,
			"dismissed_at": ts,
		}).Error
	default:
		ctx.StatusCode(400)
		ctx.JSON(iris.Map{"error": "unknown_event"})
		return
	}
	ctx.JSON(iris.Map{"success": true})
}

// GET /api/user/notification-orchestrator — inbox + quota summary.
func GetMyOrchestratorNotifications(ctx iris.Context) {
	uidAny := ctx.Values().Get("userID")
	uid, ok := uidAny.(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(401)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}
	limit, _ := strconv.Atoi(ctx.URLParamDefault("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, _ := strconv.Atoi(ctx.URLParamDefault("offset", "0"))
	if offset < 0 {
		offset = 0
	}

	var rows []models.NotificationCandidate
	_ = storage.DB.Where("user_id = ?", uid).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error

	now := time.Now()
	since := now.Add(-24 * time.Hour)
	var used int64
	_ = storage.DB.Model(&models.NotificationCandidate{}).
		Where("user_id = ? AND sent_at IS NOT NULL AND sent_at >= ?", uid, since).
		Where("ai_decision IN ?", []string{"send", "digest_sent"}).
		Count(&used).Error

	limitDay := services.OrchestratorDefaultDailyLimit
	var learned models.UserNotificationLearned
	if err := storage.DB.Where("user_id = ?", uid).First(&learned).Error; err == nil && learned.DailyLimitOverride > 0 {
		limitDay = learned.DailyLimitOverride
	}

	out := make([]iris.Map, 0, len(rows))
	for _, r := range rows {
		st := r.AIDecision
		if r.SentAt != nil {
			st = "sent"
		}
		out = append(out, iris.Map{
			"id":                r.ID,
			"type":              r.NotificationType,
			"title":             r.Title,
			"body":              r.Body,
			"status":            st,
			"ai_decision":       r.AIDecision,
			"ai_score":          r.AIScore,
			"sent_at":           r.SentAt,
			"scheduled_for":     r.ScheduledFor,
			"opened":            r.Opened,
			"opened_at":         r.OpenedAt,
			"payload":           string(r.Payload),
		})
	}

	ctx.JSON(iris.Map{
		"notifications": out,
		"quota": iris.Map{
			"used_today": used,
			"limit":      limitDay,
			"resets_at":  since.Add(24 * time.Hour).Format(time.RFC3339),
		},
	})
}

// AdminOrchestratorStats GET /api/admin/orchestrator/stats
func AdminOrchestratorStats(ctx iris.Context) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	var received, sent, batched, dropped int64
	_ = storage.DB.Model(&models.NotificationCandidate{}).Where("created_at >= ?", start).Count(&received).Error
	_ = storage.DB.Model(&models.NotificationCandidate{}).Where("sent_at IS NOT NULL AND sent_at >= ? AND ai_decision = ?", start, "send").Count(&sent).Error
	_ = storage.DB.Model(&models.NotificationCandidate{}).Where("created_at >= ? AND ai_decision = ?", start, "digest_sent").Count(&batched).Error
	_ = storage.DB.Model(&models.NotificationCandidate{}).Where("created_at >= ? AND ai_decision = ?", start, "drop").Count(&dropped).Error

	ctx.JSON(iris.Map{
		"today": iris.Map{
			"candidates_received": received,
			"sent":                sent,
			"batched_digest_rows": batched,
			"dropped":             dropped,
		},
	})
}

// AdminOrchestratorUserLog GET /api/admin/orchestrator/users/{userID:uint}/log
func AdminOrchestratorUserLog(ctx iris.Context) {
	uid := ctx.Params().GetUintDefault("userID", 0)
	if uid == 0 {
		ctx.StatusCode(400)
		return
	}
	var rows []models.NotificationCandidate
	_ = storage.DB.Where("user_id = ?", uid).Order("created_at DESC").Limit(200).Find(&rows).Error
	ctx.JSON(iris.Map{"user_id": uid, "items": rows})
}
