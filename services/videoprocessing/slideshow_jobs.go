package videoprocessing

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const redisSlideshowQueueKey = "video:slideshow:queue:v1"

type slideshowQueueJob struct {
	JobID uint `json:"job_id"`
}

var slideshowWorkersOnce sync.Once

// StartSlideshowWorkers launches Redis-backed slideshow generators (idempotent).
func StartSlideshowWorkers(db *gorm.DB) {
	slideshowWorkersOnce.Do(func() {
		n := slideshowWorkerCount()
		for i := 0; i < n; i++ {
			go runSlideshowWorker(db, i)
		}
		log.Printf("✅ slideshow: started %d worker(s)", n)
	})
}

func slideshowWorkerCount() int {
	if v := strings.TrimSpace(os.Getenv("SLIDESHOW_WORKER_COUNT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 8 {
			return n
		}
	}
	return 2
}

func slideshowEnabled() bool {
	if strings.EqualFold(os.Getenv("SLIDESHOW_VIDEO_ENABLED"), "false") {
		return false
	}
	return ffmpegPath() != ""
}

func slideshowMinImages() int {
	if v := strings.TrimSpace(os.Getenv("SLIDESHOW_MIN_IMAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 20 {
			return n
		}
	}
	return 2
}

func slideshowMaxImages() int {
	if v := strings.TrimSpace(os.Getenv("SLIDESHOW_MAX_IMAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 20 {
			return n
		}
	}
	return 10
}

func runSlideshowWorker(db *gorm.DB, id int) {
	for {
		jobID, ok := dequeueSlideshow(context.Background())
		if !ok {
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("🎞️ slideshow-worker-%d job %d", id, jobID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := ProcessSlideshowJob(ctx, db, jobID)
		cancel()
		if err != nil {
			log.Printf("❌ slideshow job %d: %v", jobID, err)
		}
	}
}

// EnqueuePropertySaleSlideshow creates a job when a listing has images but no video.
func EnqueuePropertySaleSlideshow(db *gorm.DB, propertySaleID, userID uint) (*models.PropertyVideoGenerationJob, error) {
	if !slideshowEnabled() {
		return nil, nil
	}
	var sale models.PropertySale
	if err := db.First(&sale, propertySaleID).Error; err != nil {
		return nil, err
	}
	if len(sale.Videos) > 0 {
		return nil, nil
	}
	images := filterHTTPURLs(sale.Images)
	if len(images) < slideshowMinImages() {
		return nil, nil
	}
	if len(images) > slideshowMaxImages() {
		images = images[:slideshowMaxImages()]
	}

	// Skip if a job is already pending/processing for this listing.
	var existing models.PropertyVideoGenerationJob
	err := db.Where("entity_type = ? AND entity_id = ? AND status IN ?",
		"sale", propertySaleID, []string{"pending", "processing"}).
		Order("id DESC").First(&existing).Error
	if err == nil {
		return &existing, nil
	}

	imgJSON, _ := json.Marshal(images)
	job := models.PropertyVideoGenerationJob{
		UserID:          userID,
		EntityType:      "sale",
		EntityID:        propertySaleID,
		Status:          "pending",
		Progress:        0,
		ImageURLs:       imgJSON,
		PropertyType:    strings.TrimSpace(sale.PropertyType),
		OverlayTitle:    strings.TrimSpace(sale.Title),
		OverlayLocation: formatLocation(sale.City, sale.State, sale.Country),
		OverlayArea:     formatArea(sale),
		OverlayPrice:    formatPrice(sale.ListingPrice, sale.Currency),
		OverlayCTA:      "Découvrir sur Meskeny",
	}
	if err := db.Create(&job).Error; err != nil {
		return nil, err
	}

	StartSlideshowWorkers(db)
	if pushSlideshowRedis(job.ID) {
		log.Printf("🎞️ slideshow enqueued job=%d sale=%d images=%d", job.ID, propertySaleID, len(images))
		return &job, nil
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = ProcessSlideshowJob(ctx, db, job.ID)
	}()
	return &job, nil
}

// ProcessSlideshowJob renders MP4, uploads to CDN, attaches to property sale feed.
func ProcessSlideshowJob(ctx context.Context, db *gorm.DB, jobID uint) error {
	var job models.PropertyVideoGenerationJob
	if err := db.First(&job, jobID).Error; err != nil {
		return err
	}
	if job.Status == "completed" {
		return nil
	}

	updateJob := func(status string, progress int, errMsg, videoURL string) {
		up := map[string]interface{}{
			"status":   status,
			"progress": progress,
		}
		if errMsg != "" {
			up["error_message"] = errMsg
		}
		if videoURL != "" {
			up["output_video_url"] = videoURL
		}
		_ = db.Model(&job).Updates(up).Error
	}

	updateJob("processing", 5, "", "")
	log.Printf("🎞️ slideshow job %d: processing (%s #%d)", jobID, job.EntityType, job.EntityID)

	var imageURLs []string
	_ = json.Unmarshal(job.ImageURLs, &imageURLs)
	imageURLs = filterHTTPURLs(imageURLs)
	if len(imageURLs) == 0 {
		updateJob("failed", 0, "no images", "")
		return fmt.Errorf("no images")
	}

	workDir, err := os.MkdirTemp("", fmt.Sprintf("slideshow_%d_", jobID))
	if err != nil {
		updateJob("failed", 0, err.Error(), "")
		return err
	}
	defer os.RemoveAll(workDir)

	updateJob("processing", 15, "", "")
	localImages := make([]string, 0, len(imageURLs))
	for i, u := range imageURLs {
		dest := filepath.Join(workDir, fmt.Sprintf("img_%03d.jpg", i))
		if err := downloadFile(ctx, u, dest); err != nil {
			log.Printf("⚠️ slideshow job %d: skip image %d: %v", jobID, i, err)
			continue
		}
		localImages = append(localImages, dest)
	}
	if len(localImages) < slideshowMinImages() {
		updateJob("failed", 0, "could not download enough images", "")
		return fmt.Errorf("download failed")
	}

	updateJob("processing", 35, "", "")
	musicPath := ""
	if track := pickMusicTrack(db, job.PropertyType); track != nil && strings.TrimSpace(track.FileURL) != "" {
		job.MusicTrackID = &track.ID
		_ = db.Model(&job).Update("music_track_id", track.ID).Error
		mp3 := filepath.Join(workDir, "music.mp3")
		if err := downloadFile(ctx, track.FileURL, mp3); err == nil {
			musicPath = mp3
		}
	}

	outPath := filepath.Join(workDir, "slideshow.mp4")
	updateJob("processing", 50, "", "")
	if err := GenerateSlideshowMP4(ctx, SlideshowInput{
		ImagePaths:  localImages,
		MusicPath:   musicPath,
		OutputPath:  outPath,
		Title:       job.OverlayTitle,
		Location:    job.OverlayLocation,
		Area:        job.OverlayArea,
		Price:       job.OverlayPrice,
		CTA:         job.OverlayCTA,
		SecPerSlide: slideshowSecPerSlide,
	}); err != nil {
		updateJob("failed", 0, err.Error(), "")
		return err
	}

	updateJob("processing", 80, "", "")
	objectKey := fmt.Sprintf("property-sales/%d/auto-slideshow_%d.mp4", job.EntityID, jobID)
	res := storage.UploadLocalFileObjectKey(outPath, objectKey, "video/mp4")
	videoURL := res["url"]
	if videoURL == "" {
		msg := res["error"]
		if msg == "" {
			msg = "upload failed"
		}
		updateJob("failed", 0, msg, "")
		return fmt.Errorf("%s", msg)
	}

	if job.EntityType == "sale" {
		if err := attachSlideshowToPropertySale(db, job.EntityID, job.UserID, videoURL); err != nil {
			updateJob("failed", 90, err.Error(), videoURL)
			return err
		}
	}

	updateJob("completed", 100, "", videoURL)
	log.Printf("✅ slideshow job %d complete → %s", jobID, videoURL)
	return nil
}

func attachSlideshowToPropertySale(db *gorm.DB, saleID, userID uint, videoURL string) error {
	var sale models.PropertySale
	if err := db.First(&sale, saleID).Error; err != nil {
		return err
	}
	videos := append([]string{}, sale.Videos...)
	dup := false
	for _, v := range videos {
		if v == videoURL {
			dup = true
			break
		}
	}
	if !dup {
		videos = append(videos, videoURL)
		if err := db.Model(&sale).Update("videos", videos).Error; err != nil {
			return err
		}
	}

	// Insert feed row + HLS pipeline (same as host-uploaded video).
	var existing models.PropertySaleVideo
	err := db.Where("property_sale_id = ? AND video_url = ?", saleID, videoURL).First(&existing).Error
	if err == nil {
		EnqueuePropertySaleVideo(db, existing.ID, userID)
		return nil
	}

	thumb := storage.ChunkUploadPreviewBlurURL(videoURL)
	row := models.PropertySaleVideo{
		PropertySaleID:   saleID,
		UserID:           userID,
		VideoURL:         videoURL,
		ThumbnailURL:     thumb,
		PreviewBlurURL:   thumb,
		Status:           "approved",
		ProcessingStatus: "pending",
		Caption:          "Auto-generated listing video",
	}
	if err := db.Create(&row).Error; err != nil {
		return err
	}
	EnqueuePropertySaleVideo(db, row.ID, userID)
	return nil
}

func pickMusicTrack(db *gorm.DB, propertyType string) *models.MusicTrack {
	category := musicCategoryForPropertyType(propertyType)
	var track models.MusicTrack
	q := db.Where("is_active = ? AND file_url <> ''", true)
	if err := q.Where("category = ?", category).Order("sort_order ASC, id ASC").First(&track).Error; err == nil {
		return &track
	}
	if err := q.Where("category = ?", "default").Order("sort_order ASC, id ASC").First(&track).Error; err == nil {
		return &track
	}
	if err := db.Where("is_active = ? AND file_url <> ''", true).
		Order("sort_order ASC, id ASC").First(&track).Error; err == nil {
		return &track
	}
	return nil
}

func musicCategoryForPropertyType(propertyType string) string {
	t := strings.ToLower(strings.TrimSpace(propertyType))
	switch {
	case strings.Contains(t, "land"), strings.Contains(t, "terrain"), strings.Contains(t, "plot"):
		return "land"
	case strings.Contains(t, "villa"), strings.Contains(t, "luxury"), strings.Contains(t, "penthouse"):
		return "luxury"
	case strings.Contains(t, "commercial"), strings.Contains(t, "office"), strings.Contains(t, "business"):
		return "business"
	case strings.Contains(t, "apartment"), strings.Contains(t, "flat"):
		return "urban"
	default:
		return "default"
	}
}

func formatLocation(city, state, country string) string {
	parts := []string{}
	for _, p := range []string{city, state, country} {
		p = strings.TrimSpace(p)
		if p != "" && p != "-" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func formatArea(sale models.PropertySale) string {
	sqm := sale.SquareFootage
	if sqm <= 0 && sale.LotSize > 0 {
		sqm = int(sale.LotSize + 0.5)
	}
	if sqm <= 0 {
		return ""
	}
	return fmt.Sprintf("%d m²", sqm)
}

func formatPrice(price float64, currency string) string {
	if price <= 0 {
		return ""
	}
	cur := strings.TrimSpace(currency)
	if cur == "" {
		cur = "MRU"
	}
	return fmt.Sprintf("%.0f %s", price, cur)
}

func filterHTTPURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			out = append(out, u)
		}
	}
	return out
}

func pushSlideshowRedis(jobID uint) bool {
	if storage.Redis == nil {
		return false
	}
	b, err := json.Marshal(slideshowQueueJob{JobID: jobID})
	if err != nil {
		return false
	}
	return storage.Redis.LPush(context.Background(), redisSlideshowQueueKey, b).Err() == nil
}

func dequeueSlideshow(ctx context.Context) (uint, bool) {
	if storage.Redis == nil {
		return 0, false
	}
	res, err := storage.Redis.BRPop(ctx, 5*time.Second, redisSlideshowQueueKey).Result()
	if err != nil || len(res) < 2 {
		return 0, false
	}
	var job slideshowQueueJob
	if json.Unmarshal([]byte(res[1]), &job) != nil || job.JobID == 0 {
		return 0, false
	}
	return job.JobID, true
}

// SeedDefaultMusicTracks inserts placeholder library rows (admin uploads MP3 URLs later).
func SeedDefaultMusicTracks(db *gorm.DB) {
	if db == nil {
		return
	}
	var count int64
	db.Model(&models.MusicTrack{}).Count(&count)
	if count > 0 {
		return
	}
	rows := []models.MusicTrack{
		{Title: "Meskeny Calm Land", Category: "land", SortOrder: 1, IsActive: false, Notes: "Upload MP3 to CDN and set file_url"},
		{Title: "Meskeny Luxury Estate", Category: "luxury", SortOrder: 1, IsActive: false, Notes: "Upload MP3 to CDN and set file_url"},
		{Title: "Meskeny Urban Flow", Category: "urban", SortOrder: 1, IsActive: false, Notes: "Upload MP3 to CDN and set file_url"},
		{Title: "Meskeny Business Pulse", Category: "business", SortOrder: 1, IsActive: false, Notes: "Upload MP3 to CDN and set file_url"},
		{Title: "Meskeny Default Theme", Category: "default", SortOrder: 1, IsActive: false, Notes: "Fallback track — set file_url to enable music"},
	}
	for _, r := range rows {
		_ = db.Create(&r).Error
	}
	log.Printf("✅ music library: seeded %d placeholder tracks (set file_url via admin)", len(rows))
}
