package push

import (
	"log"
	"math/rand"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

const marketingReminderInterval = 6 * time.Hour

var marketingMessages = []struct {
	Title string
	Body  string
}{
	{Title: "عقارات جديدة في انتظارك", Body: "عد إلى Habitat وتصفح أحدث العقارات المضافة اليوم."},
	{Title: "لا تفوت العروض الجديدة", Body: "شاهد الآن العقارات المميزة التي قد تناسب احتياجاتك."},
	{Title: "فرص مميزة بالقرب منك", Body: "قم بفتح التطبيق لاكتشاف عقارات جديدة في موقعك المفضل."},
}

// StartMarketingReminderWorker launches a background worker to send marketing reminders
func StartMarketingReminderWorker() {
	if storage.DB == nil {
		log.Printf("⚠️ Database not initialized, marketing worker disabled")
		return
	}

	rand.Seed(time.Now().UnixNano())

	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for {
			sendMarketingReminders()
			<-ticker.C
		}
	}()
}

func sendMarketingReminders() {
	if storage.DB == nil {
		return
	}

	now := time.Now()
	var devices []models.MarketingDevice

	if err := storage.DB.Where("marketing_opt_in = ? AND fcm_token <> '' AND (next_send_at IS NULL OR next_send_at <= ?)", true, now).
		Limit(200).
		Find(&devices).Error; err != nil {
		log.Printf("⚠️ Failed to load marketing devices: %v", err)
		return
	}

	if len(devices) == 0 {
		return
	}

	for _, device := range devices {
		if device.FCMToken == "" {
			continue
		}

		message := marketingMessages[rand.Intn(len(marketingMessages))]
		tokens := []string{device.FCMToken}

		if err := EnqueuePush(tokens, message.Title, message.Body); err != nil {
			log.Printf("⚠️ Failed to enqueue marketing push for device %s: %v", device.DeviceID, err)
			continue
		}

		next := now.Add(marketingReminderInterval)
		if err := storage.DB.Model(&models.MarketingDevice{}).
			Where("id = ?", device.ID).
			Updates(map[string]interface{}{
				"last_sent_at": now,
				"next_send_at": next,
			}).Error; err != nil {
			log.Printf("⚠️ Failed to update marketing device %s schedule: %v", device.DeviceID, err)
		}
	}
}
