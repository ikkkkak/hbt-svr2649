package routes

import (
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"github.com/kataras/iris/v12"
)

// GET /api/notifications
func ListUserNotifications(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	limit, _ := ctx.URLParamInt("limit")
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var rows []models.Notification
	storage.DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows)
	ctx.JSON(iris.Map{"notifications": rows})
}

// PATCH /api/notifications/:id/read
func MarkUserNotificationRead(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		return
	}
	now := time.Now()
	res := storage.DB.Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})
	if res.RowsAffected == 0 {
		ctx.StatusCode(iris.StatusNotFound)
		return
	}
	ctx.JSON(iris.Map{"ok": true})
}

// PATCH /api/notifications/read-all
func MarkAllUserNotificationsRead(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	now := time.Now()
	storage.DB.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})
	ctx.JSON(iris.Map{"ok": true})
}
