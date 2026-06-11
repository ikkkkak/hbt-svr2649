package services

import (
	"apartments-clone-server/discoverpush"
	"apartments-clone-server/goldproperty"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"time"
)

// SendDiscoverySpotlightForUsers sends up to maxUsers localized discovery pushes
// (papers + investment copy + deep links). Uses discoverpush for selection and copy.
func (ns *NotificationService) SendDiscoverySpotlightForUsers(now time.Time, maxUsers int) {
	if storage.DB == nil || maxUsers <= 0 {
		return
	}

	var users []models.User
	if err := storage.DB.Where("allows_notifications = ?", true).Limit(maxUsers * 3).Find(&users).Error; err != nil {
		return
	}

	sent := 0
	for i := range users {
		if sent >= maxUsers {
			break
		}
		u := &users[i]
		title, body, img, data, saleID, landmarkID, ok := discoverpush.PlanUserSpotlight(u, now)
		if !ok {
			continue
		}
		if ns.sendToUserWithImage(u.ID, title, body, img, data) {
			_ = discoverpush.LogDiscoverySend(&u.ID, "", saleID, landmarkID)
			if saleID != nil {
				goldproperty.RecordNotificationSent(*saleID)
			}
			sent++
			if saleID != nil {
				log.Printf("discovery: user=%d sale=%d", u.ID, *saleID)
			} else if landmarkID != nil {
				log.Printf("discovery: user=%d landmark=%d", u.ID, *landmarkID)
			}
		}
	}
}
