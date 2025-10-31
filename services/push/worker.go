package push

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"apartments-clone-server/storage"
)

const pushQueueKey = "push:queue"

type pushJob struct {
	Tokens []string `json:"tokens"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
}

// EnqueuePush adds a push job to Redis
func EnqueuePush(tokens []string, title, body string) error {
	if len(tokens) == 0 {
		return nil
	}
	job := pushJob{Tokens: tokens, Title: title, Body: body}
	b, _ := json.Marshal(job)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if storage.Redis == nil {
		log.Printf("⚠️ Redis not initialized, sending push synchronously")
		return SendExpoPush(tokens, title, body)
	}
	if err := storage.Redis.LPush(ctx, pushQueueKey, b).Err(); err != nil {
		log.Printf("⚠️ Failed to enqueue push job: %v — falling back to direct send", err)
		return SendExpoPush(tokens, title, body)
	}
	log.Printf("🧾 Enqueued push job for %d tokens", len(tokens))
	return nil
}

// StartPushWorker starts a goroutine that continuously processes push jobs from Redis
func StartPushWorker() {
	go func() {
		ctx := context.Background()
		for {
			// BLPOP blocks until a job is available
			res, err := storage.Redis.BLPop(ctx, 30*time.Second, pushQueueKey).Result()
			if err != nil {
				if err.Error() == "redis: nil" {
					continue
				}
				log.Printf("⚠️ Push worker pop error: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			if len(res) < 2 {
				continue
			}
			var job pushJob
			if err := json.Unmarshal([]byte(res[1]), &job); err != nil {
				log.Printf("⚠️ Invalid push job: %v", err)
				continue
			}
			log.Printf("🚚 Dequeued push job: %d tokens", len(job.Tokens))
			// send pushes
			if err := SendExpoPush(job.Tokens, job.Title, job.Body); err != nil {
				log.Printf("⚠️ Expo push send error: %v", err)
			}
		}
	}()
}
