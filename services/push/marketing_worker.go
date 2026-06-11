package push

import (
	"log"
	"math/rand"
	"time"

	"apartments-clone-server/discoverpush"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

const marketingReminderInterval = 24 * time.Hour

var marketingMessages = []struct {
	Title string
	Body  string
}{
	{Title: "عقارات جديدة مطابقة لاهتماماتك", Body: "وجدنا عقارات أقرب لميزانيتك وموقعك المفضل."},
	{Title: "فرص جديدة في منطقتك", Body: "تمت إضافة عقارات حديثة قد تناسب بحثك الحالي."},
	{Title: "اختيارات عقارية أفضل لك", Body: "لدينا اقتراحات جديدة بناءً على تفضيلاتك الأخيرة."},
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

		// Prefer localized “discovery spotlight” pushes (papers + deep link) when listings exist.
		sentDiscovery := discoverpush.TrySendMarketingDiscovery(&device, now, func(token, title, body, image string, data map[string]string) error {
			return SendPushWithImage([]string{token}, title, body, image, data)
		})
		if sentDiscovery {
			next := now.Add(marketingReminderInterval)
			if err := storage.DB.Model(&models.MarketingDevice{}).
				Where("id = ?", device.ID).
				Updates(map[string]interface{}{
					"last_sent_at": now,
					"next_send_at": next,
				}).Error; err != nil {
				log.Printf("⚠️ Failed to update marketing device %s schedule: %v", device.DeviceID, err)
			}
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
