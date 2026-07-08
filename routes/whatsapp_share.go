package routes

import (
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

var allowedWhatsAppShareEvents = map[string]bool{
	"sheet_opened":    true,
	"share_started":   true,
	"share_completed": true,
	"share_failed":    true,
	"share_dismissed": true,
}

// PostWhatsAppShareEvent records WhatsApp share card funnel steps from the mobile app.
// POST /api/whatsapp-share/events
func PostWhatsAppShareEvent(ctx iris.Context) {
	var body struct {
		PropertySaleID uint   `json:"property_sale_id"`
		Event          string `json:"event"`
		Platform       string `json:"platform"`
		PropertyTitle  string `json:"property_title"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	if body.PropertySaleID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "property_sale_id is required"})
		return
	}
	event := strings.TrimSpace(strings.ToLower(body.Event))
	if !allowedWhatsAppShareEvents[event] {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid event"})
		return
	}

	userID := OptionalAuthUserID(ctx)
	platform := strings.TrimSpace(strings.ToLower(body.Platform))
	if len(platform) > 16 {
		platform = platform[:16]
	}
	title := strings.TrimSpace(body.PropertyTitle)
	if len(title) > 255 {
		title = title[:255]
	}

	row := models.WhatsAppShareUsageEvent{
		UserID:         userID,
		PropertySaleID: body.PropertySaleID,
		Event:          event,
		Platform:       platform,
		PropertyTitle:  title,
	}
	if err := storage.DB.Create(&row).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_record_event"})
		return
	}

	ctx.JSON(iris.Map{"data": iris.Map{"ok": true, "id": row.ID}})
}
