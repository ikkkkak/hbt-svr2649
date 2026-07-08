package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	meskenyai "apartments-clone-server/MeskenyGPT/ai"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"gorm.io/datatypes"
)

var semanticIndexHook func(source string, id uint)

// RegisterSemanticIndexHook registers an async listing indexer (wired from main.go).
func RegisterSemanticIndexHook(fn func(source string, id uint)) {
	semanticIndexHook = fn
}

// QueueSemanticIndex indexes a listing asynchronously when semantic search is enabled.
func QueueSemanticIndex(source string, id uint) {
	if semanticIndexHook == nil || id == 0 {
		return
	}
	go semanticIndexHook(source, id)
}

// SendModelX46Notification persists an in-app AI notification and sends push.
func SendModelX46Notification(
	ctx context.Context,
	userID uint,
	typ, title, body string,
	payload map[string]any,
	relevance float64,
	urgency string,
) (*models.AINotification, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user_id required")
	}
	lang := ResolveUserNotificationLang(userID)
	title, body = EnsureNotificationCopy(lang, typ, title, body)

	payloadJSON, _ := json.Marshal(payload)
	row := &models.AINotification{
		UserID:         userID,
		Type:           typ,
		Title:          title,
		Body:           body,
		RelevanceScore: relevance,
		ActionType:     actionTypeFromPayload(payload),
		ActionPayload:  datatypes.JSON(payloadJSON),
		Status:         "sent",
	}
	now := time.Now()
	row.SentAt = &now
	if err := storage.DB.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}

	data := map[string]string{
		"screen":             "AIChat",
		"notification_type":  typ,
		"ai_notification_id": fmt.Sprintf("%d", row.ID),
		"urgency":            urgency,
	}
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			data["params"] = string(b)
		}
	}
	img := ""
	if psID, ok := payload["property_sale_id"].(uint); ok && psID > 0 {
		var sale models.PropertySale
		if storage.DB.First(&sale, psID).Error == nil && len(sale.Images) > 0 {
			img = sale.Images[0]
		}
	}
	ns := NewNotificationService()
	_ = ns.sendToUserWithImage(userID, title, body, img, data)
	return row, nil
}

func actionTypeFromPayload(payload map[string]any) string {
	if payload == nil {
		return "open_chat"
	}
	if _, ok := payload["property_sale_id"]; ok {
		return "view_property"
	}
	if _, ok := payload["escalation_id"]; ok {
		return "open_escalation"
	}
	return "open_chat"
}

// NotifyAgentsEscalation alerts admin users about a new escalation.
func NotifyAgentsEscalation(ctx context.Context, escalationID uint, sessionID string, userID *uint, title, body, urgency string) {
	var admins []models.User
	storage.DB.WithContext(ctx).Where("role IN ?", []string{"admin", "agent"}).Limit(20).Find(&admins)
	payload := map[string]any{
		"escalation_id": escalationID,
		"session_id":    sessionID,
		"screen":        "AIChat",
	}
	if userID != nil {
		payload["user_id"] = *userID
	}
	for _, admin := range admins {
		_, _ = SendModelX46Notification(ctx, admin.ID, "escalation_update", title, body, payload, 0.95, urgency)
	}
}

// EvaluatePropertyMatchNotification sends a smart property-match notification.
func EvaluatePropertyMatchNotification(ctx context.Context, userID uint, sale models.PropertySale) {
	if !smartNotificationsEnabled() || userID == 0 {
		return
	}
	lang := ResolveUserNotificationLang(userID)
	title, body := ModelX46PropertyMatchFallback(lang, sale.Title, sale.City)
	if MeskenyGPTService != nil {
		langHint := "Respond in English."
		switch lang {
		case "ar":
			langHint = "Respond in Arabic."
		case "fr":
			langHint = "Respond in French."
		}
		prompt := langHint + "\nReturn ONLY JSON {title, body}. Property: " + sale.Title +
			fmt.Sprintf(", %s, %.0f %s", sale.City, sale.ListingPrice, sale.Currency)
		out, err := MeskenyGPTService.HandleChatTurn(ctx, meskenyai.ChatInput{
			UserID:    userID,
			SessionID: fmt.Sprintf("notify_%d_%d", userID, time.Now().UnixNano()),
			Text:      prompt,
		})
		if err == nil {
			var c aiPushCopy
			raw := strings.TrimSpace(out.Message.Content)
			if i := strings.Index(raw, "{"); i >= 0 {
				if j := strings.LastIndex(raw, "}"); j > i {
					raw = raw[i : j+1]
				}
			}
			if json.Unmarshal([]byte(raw), &c) == nil && c.Title != "" && c.Body != "" {
				title, body = c.Title, c.Body
			}
		}
	}
	relevance := 0.72
	if sale.IsInvestmentOpportunity {
		relevance = 0.82
	}
	_, _ = SendModelX46Notification(ctx, userID, "property_match", title, body, map[string]any{
		"property_sale_id": sale.ID,
		"screen":           "PropertySaleDetails",
	}, relevance, "normal")
}

func smartNotificationsEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ENABLE_SMART_NOTIFICATIONS")))
	return v == "" || v == "1" || v == "true" || v == "yes"
}

// BatchNotifyInterestedUsers notifies users with location prefs matching a new listing city.
func BatchNotifyInterestedUsers(ctx context.Context, sale models.PropertySale) {
	if !smartNotificationsEnabled() || !sale.IsPublished {
		return
	}
	city := strings.TrimSpace(sale.City)
	if city == "" {
		return
	}
	var prefs []models.NotificationPreference
	storage.DB.WithContext(ctx).
		Where("enabled = ? AND LOWER(location) LIKE ?", true, "%"+strings.ToLower(city)+"%").
		Limit(25).
		Find(&prefs)
	for _, pref := range prefs {
		if pref.UserID == nil || *pref.UserID == 0 {
			continue
		}
		uid := *pref.UserID
		var count int64
		storage.DB.Model(&models.AINotification{}).
			Where("user_id = ? AND created_at >= ?", uid, time.Now().Add(-24*time.Hour)).
			Count(&count)
		if count >= 5 {
			continue
		}
		EvaluatePropertyMatchNotification(ctx, uid, sale)
	}
}
