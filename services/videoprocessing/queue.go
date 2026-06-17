package videoprocessing

import (
	"apartments-clone-server/models"
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
	VideoID uint   `json:"video_id"`
	UserID  uint   `json:"user_id"`
	Kind    string `json:"kind,omitempty"` // "" or "rent", "sale"
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
		log.Printf("🎬 worker-%d processing %s video %d", id, jobKindLabel(job.Kind), job.VideoID)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		var err error
		if job.Kind == saleVideoEntity {
			err = ProcessPropertySaleVideo(ctx, db, job.VideoID, job.UserID)
		} else {
			err = ProcessVideo(ctx, db, job.VideoID, job.UserID)
		}
		cancel()
		if err != nil {
			log.Printf("❌ worker-%d video %d: %v", id, job.VideoID, err)
		}
	}
}

// Enqueue schedules rent video transcoding via Redis queue (falls back to in-process goroutine).
func Enqueue(db *gorm.DB, videoID, userID uint) {
	enqueueJob(db, queueJob{VideoID: videoID, UserID: userID, Kind: "rent"}, func() {
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
				VideoID: videoID, EntityType: "rent", ProcessingStatus: "ready", Progress: 100, Ready: true,
				MobileVideoURL: v.VideoURL,
			})
		}
	})
}

// EnqueuePropertySaleVideo schedules HLS transcoding for a property_sale_videos row.
func EnqueuePropertySaleVideo(db *gorm.DB, saleVideoID, userID uint) {
	enqueueJob(db, queueJob{VideoID: saleVideoID, UserID: userID, Kind: saleVideoEntity}, func() {
		var v struct {
			ID       uint
			VideoURL string
			UserID   uint
		}
		if err := db.Table("property_sale_videos").Select("id", "video_url", "user_id").First(&v, saleVideoID).Error; err == nil {
			uid := userID
			if uid == 0 {
				uid = v.UserID
			}
			_ = markSaleReadyMP4Only(db, saleVideoID, uid, v.VideoURL)
			PublishProcessing(userID, ProcessingEvent{
				VideoID: saleVideoID, EntityType: saleVideoEntity, ProcessingStatus: "ready", Progress: 100, Ready: true,
				MobileVideoURL: v.VideoURL,
			})
		}
	})
}

func enqueueJob(db *gorm.DB, job queueJob, mp4OnlyFallback func()) {
	if strings.EqualFold(os.Getenv("VIDEO_PROCESSING_ENABLED"), "false") {
		mp4OnlyFallback()
		return
	}

	StartWorkers(db)

	if pushRedis(job) {
		PublishProcessing(job.UserID, ProcessingEvent{
			VideoID: job.VideoID, EntityType: jobKindLabel(job.Kind), ProcessingStatus: "pending", Progress: 5,
		})
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if job.Kind == saleVideoEntity {
			_ = ProcessPropertySaleVideo(ctx, db, job.VideoID, job.UserID)
		} else {
			_ = ProcessVideo(ctx, db, job.VideoID, job.UserID)
		}
	}()
}

func jobKindLabel(kind string) string {
	if kind == saleVideoEntity {
		return saleVideoEntity
	}
	return "rent"
}

// BackfillPendingPropertySaleVideos enqueues HLS jobs for rows created before the pipeline existed.
// Only runs when VIDEO_BACKFILL_ENABLED=true (off by default — avoids hammering the server on boot).
func BackfillPendingPropertySaleVideos(db *gorm.DB, limit int) {
	if os.Getenv("VIDEO_BACKFILL_ENABLED") != "true" {
		return
	}
	if limit <= 0 {
		limit = 50
	}
	maxAgeDays := 14
	if v := strings.TrimSpace(os.Getenv("VIDEO_BACKFILL_MAX_AGE_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			maxAgeDays = n
		}
	}
	cutoff := time.Now().Add(-time.Duration(maxAgeDays) * 24 * time.Hour)
	var rows []models.PropertySaleVideo
	q := db.Where("deleted_at IS NULL").
		Where("created_at >= ?", cutoff).
		Where("(hls_url IS NULL OR hls_url = '')").
		Where("processing_status IN ? OR processing_status IS NULL OR processing_status = ''",
			[]string{"pending", "failed", ""}).
		Order("id DESC").
		Limit(limit)
	if err := q.Find(&rows).Error; err != nil || len(rows) == 0 {
		return
	}
	log.Printf("🎬 backfill: enqueueing %d pending property sale video(s) for HLS", len(rows))
	for _, row := range rows {
		_ = db.Model(&models.PropertySaleVideo{}).Where("id = ?", row.ID).
			Update("processing_status", "pending").Error
		EnqueuePropertySaleVideo(db, row.ID, row.UserID)
	}
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
