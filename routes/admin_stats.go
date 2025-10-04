package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"time"

	"github.com/kataras/iris/v12"
)

// GET /admin/stats
func AdminStats(ctx iris.Context) {
	var pendingProperties int64
	storage.DB.Model(&models.Property{}).Where("status = ?", "pending").Count(&pendingProperties)
	var pendingVerifications int64
	storage.DB.Model(&models.IdentityVerification{}).Where("status = ?", "pending").Count(&pendingVerifications)
	var pendingVideos int64
	storage.DB.Model(&models.Video{}).Where("status = ?", "pending").Count(&pendingVideos)

	since7 := time.Now().AddDate(0, 0, -7)
	since30 := time.Now().AddDate(0, 0, -30)
	var newRes7, newRes30 int64
	storage.DB.Model(&models.Reservation{}).Where("created_at >= ?", since7).Count(&newRes7)
	storage.DB.Model(&models.Reservation{}).Where("created_at >= ?", since30).Count(&newRes30)

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"pending_properties":    pendingProperties,
			"pending_verifications": pendingVerifications,
			"pending_videos":        pendingVideos,
			"new_reservations_7d":   newRes7,
			"new_reservations_30d":  newRes30,
		},
		"meta":  iris.Map{},
		"links": iris.Map{},
	})
}

// GET /admin/activity
func AdminActivity(ctx iris.Context) {
	var logs []models.AuditLog
	storage.DB.Order("created_at DESC").Limit(100).Find(&logs)
	ctx.JSON(iris.Map{"data": logs, "meta": iris.Map{}, "links": iris.Map{}})
}
