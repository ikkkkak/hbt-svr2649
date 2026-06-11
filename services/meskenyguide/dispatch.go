package meskenyguide

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
)

func deepLink(propertySaleID, commentID uint) string {
	return fmt.Sprintf("meskeny.mr/listing/%d/guide/%d", propertySaleID, commentID)
}

func dispatchNotifications(c *models.GuideComment, listingTitle string) {
	now := time.Now()
	link := deepLink(derefUint(c.PropertySaleID), c.ID)

	gn := models.GuideNotification{
		CommentID: c.ID,
		HostID:    c.HostID,
		Channel:   models.GuideChannelInApp,
		Status:    "sent",
		DeepLink:  link,
		SentAt:    &now,
	}
	storage.DB.Create(&gn)

	preview := c.Diagnosis
	if len(preview) > 120 {
		preview = preview[:117] + "..."
	}
	title := "Meskeny Guide"
	if listingTitle != "" {
		short := listingTitle
		if len(short) > 28 {
			short = short[:25] + "..."
		}
		title = fmt.Sprintf("Meskeny Guide • %s", short)
	}

	params, _ := json.Marshal(map[string]interface{}{
		"propertySaleId": c.PropertySaleID,
		"commentId":      c.ID,
		"deepLink":       link,
	})
	storage.DB.Create(&models.Notification{
		UserID:  c.HostID,
		Type:    "meskeny_guide",
		Title:   title,
		Message: preview,
		RefType: "guide_comment",
		RefID:   c.ID,
	})

	// Phase 2: push for action + urgent only
	if c.Severity == models.GuideSeverityAction || c.Severity == models.GuideSeverityUrgent {
		pushBody := preview
		if len(pushBody) > 160 {
			pushBody = pushBody[:157] + "..."
		}
		ns := services.NewNotificationService()
		data := services.NotificationData{
			Type:       "meskeny_guide",
			ID:         fmt.Sprintf("%d", c.ID),
			PropertyID: fmt.Sprintf("%d", derefUint(c.PropertySaleID)),
			HostID:     fmt.Sprintf("%d", c.HostID),
			Screen:     "ListingGuide",
			Params:     string(params),
			Action:     "view_comment",
		}
		if err := ns.SendNotificationToUser(c.HostID, title, pushBody, data); err != nil {
			log.Printf("meskeny guide push: %v", err)
		} else {
			pushNow := time.Now()
			storage.DB.Create(&models.GuideNotification{
				CommentID: c.ID,
				HostID:    c.HostID,
				Channel:   models.GuideChannelPush,
				Status:    "sent",
				DeepLink:  link,
				SentAt:    &pushNow,
			})
		}
	}
}

func derefUint(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

func hostActionForCategory(category string) string {
	switch strings.ToLower(category) {
	case "photo":
		return "photo_added"
	case "price":
		return "price_changed"
	case "seo":
		return "description_updated"
	default:
		return "description_updated"
	}
}
