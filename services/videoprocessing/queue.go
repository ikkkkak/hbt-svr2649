package videoprocessing

import (
	"apartments-clone-server/storage"
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const redisQueueKey = "video:transcode:queue:v1"

type queueJob struct {
	VideoID uint `json:"video_id"`
	UserID  uint `json:"user_id"`
}

var (
	workersOnce sync.Once
)

// StartWorkers launches Redis-backed transcode consumers (idempotent).
func StartWorkers(db *gorm.DB) {
	workersOnce.Do(func() {
		n := workerCount()
		for i := 0; i < n; i++ {
			go runWorker(db, i)
		}
		log.Printf("✅ videoprocessing: started %d worker(s)", n)
	})
}

func workerCount() int {
	if v := strings.TrimSpace(os.Getenv("VIDEO_WORKER_COUNT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 32 {
			return n
		}
	}
	return 3
}

func runWorker(db *gorm.DB, id int) {
	for {
		job, ok := dequeue(context.Background())
		if !ok {
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("🎬 worker-%d processing video %d", id, job.VideoID)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		err := ProcessVideo(ctx, db, job.VideoID, job.UserID)
		cancel()
		if err != nil {
			log.Printf("❌ worker-%d video %d: %v", id, job.VideoID, err)
		}
	}
}

// Enqueue schedules transcoding via Redis queue (falls back to in-process goroutine).
func Enqueue(db *gorm.DB, videoID, userID uint) {
	if strings.EqualFold(os.Getenv("VIDEO_PROCESSING_ENABLED"), "false") {
		var v struct {
			ID       uint
			VideoURL string
			UserID   uint
		}
		if err := db.Table("videos").Select("id", "video_url", "user_id").First(&v, videoID).Error; err == nil {
			uid := userID
			if uid == 0 {
				uid = v.UserID
			}
			_ = markReadyMP4Only(db, videoID, uid, v.VideoURL)
			PublishProcessing(userID, ProcessingEvent{
				VideoID: videoID, ProcessingStatus: "ready", Progress: 100, Ready: true,
				MobileVideoURL: v.VideoURL,
			})
		}
		return
	}

	StartWorkers(db)

	if pushRedis(queueJob{VideoID: videoID, UserID: userID}) {
		PublishProcessing(userID, ProcessingEvent{
			VideoID: videoID, ProcessingStatus: "pending", Progress: 5,
		})
		return
	}

	// Fallback: single-process goroutine when Redis unavailable
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		_ = ProcessVideo(ctx, db, videoID, userID)
	}()
}

func pushRedis(job queueJob) bool {
	if storage.Redis == nil {
		return false
	}
	b, err := json.Marshal(job)
	if err != nil {
		return false
	}
	return storage.Redis.LPush(context.Background(), redisQueueKey, b).Err() == nil
}

func dequeue(ctx context.Context) (queueJob, bool) {
	if storage.Redis == nil {
		return queueJob{}, false
	}
	res, err := storage.Redis.BRPop(ctx, 5*time.Second, redisQueueKey).Result()
	if err != nil || len(res) < 2 {
		return queueJob{}, false
	}
	var job queueJob
	if json.Unmarshal([]byte(res[1]), &job) != nil || job.VideoID == 0 {
		return queueJob{}, false
	}
	return job, true
}
