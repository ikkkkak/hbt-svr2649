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
		if ff := ffmpegPath(); ff == "" {
			log.Printf("❌ slideshow DISABLED: ffmpeg not found — set FFMPEG_PATH or install ffmpeg (land/sale auto-videos will not generate)")
		} else {
			log.Printf("✅ slideshow: ffmpeg=%s font=%s", ff, slideshowFontPath())
		}
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

func slideshowDisabledReason() string {
	if strings.EqualFold(os.Getenv("SLIDESHOW_VIDEO_ENABLED"), "false") {
		return "SLIDESHOW_VIDEO_ENABLED=false"
	}
	if ff := ffmpegPath(); ff == "" {
		if p := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); p != "" {
			return fmt.Sprintf("FFMPEG_PATH=%q is set but file not found", p)
		}
		return "ffmpeg not installed (apk add ffmpeg or set FFMPEG_PATH=/usr/bin/ffmpeg)"
	}
	return ""
}

// LogSlideshowStartupStatus prints one clear line so production logs show why slideshow is off.
func LogSlideshowStartupStatus() {
	if reason := slideshowDisabledReason(); reason != "" {
		log.Printf("❌ SLIDESHOW DISABLED: %s — land/sale auto-videos will NOT generate until fixed", reason)
		return
	}
	log.Printf("✅ SLIDESHOW READY: ffmpeg=%s font=%s backfill=%v workers=%d",
		ffmpegPath(), slideshowFontPath(), slideshowBackfillEnabled(), slideshowWorkerCount())
}

func slideshowMinImages() int {
	if v := strings.TrimSpace(os.Getenv("SLIDESHOW_MIN_IMAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 20 {
			return n
		}
	}
	return 1
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
		reason := slideshowDisabledReason()
		if reason == "" {
			reason = "slideshow disabled"
		}
		log.Printf("⚠️ slideshow skipped sale=%d: %s", propertySaleID, reason)
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
	images = slideshowEligibleImages(images)
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
	go func(jid uint) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := ProcessSlideshowJob(ctx, db, jid); err != nil {
			log.Printf("❌ slideshow job %d: %v", jid, err)
		}
	}(job.ID)
	return &job, nil
}

// EnqueueLandmarkSlideshow creates a job when a land listing has photos but no video.
func EnqueueLandmarkSlideshow(db *gorm.DB, landmarkID, userID uint) (*models.PropertyVideoGenerationJob, error) {
	if !slideshowEnabled() {
		reason := slideshowDisabledReason()
		if reason == "" {
			reason = "slideshow disabled"
		}
		log.Printf("⚠️ slideshow skipped land=%d: %s", landmarkID, reason)
		return nil, nil
	}
	var lm models.Landmark
	if err := db.First(&lm, landmarkID).Error; err != nil {
		return nil, err
	}
	if lm.VideoURL != nil && strings.TrimSpace(*lm.VideoURL) != "" {
		return nil, nil
	}
	images := landmarkImageURLs(lm.Images)
	if len(images) < slideshowMinImages() {
		return nil, nil
	}
	if len(images) > slideshowMaxImages() {
		images = images[:slideshowMaxImages()]
	}

	// Skip if a job is already pending/processing, or a completed job already produced a video.
	var existing models.PropertyVideoGenerationJob
	err := db.Where("entity_type = ? AND entity_id = ? AND status IN ?",
		"land", landmarkID, []string{"pending", "processing"}).
		Order("id DESC").First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	var done models.PropertyVideoGenerationJob
	if err := db.Where("entity_type = ? AND entity_id = ? AND status = ? AND output_video_url <> ''",
		"land", landmarkID, "completed").Order("id DESC").First(&done).Error; err == nil {
		_ = attachSlideshowToLandmark(db, landmarkID, done.OutputVideoURL)
		return &done, nil
	}

	imgJSON, _ := json.Marshal(images)
	job := models.PropertyVideoGenerationJob{
		UserID:          userID,
		EntityType:      "land",
		EntityID:        landmarkID,
		Status:          "pending",
		Progress:        0,
		ImageURLs:       imgJSON,
		PropertyType:    strings.TrimSpace(lm.LandType),
		OverlayTitle:    strings.TrimSpace(lm.Title),
		OverlayLocation: formatLandmarkLocation(lm),
		OverlayArea:     formatLandmarkArea(lm),
		OverlayPrice:      formatPrice(lm.Price, lm.Currency),
		OverlayPlotNumber: formatLandmarkPlotNumber(lm),
		OverlayCTA:        "Découvrir sur Meskeny",
	}
	if err := db.Create(&job).Error; err != nil {
		return nil, err
	}

	StartSlideshowWorkers(db)
	if pushSlideshowRedis(job.ID) {
		log.Printf("🎞️ slideshow enqueued job=%d land=%d images=%d", job.ID, landmarkID, len(images))
		return &job, nil
	}
	go func(jid uint) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := ProcessSlideshowJob(ctx, db, jid); err != nil {
			log.Printf("❌ slideshow job %d: %v", jid, err)
		}
	}(job.ID)
	return &job, nil
}

// ProcessSlideshowJob renders MP4, uploads to CDN, attaches to listing feed.
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

	imageURLs := freshSlideshowImageURLs(db, &job)
	if len(imageURLs) == 0 {
		updateJob("failed", 0, "no spaces images", "")
		return fmt.Errorf("no eligible images")
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
		ext := strings.ToLower(filepath.Ext(strings.Split(u, "?")[0]))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif":
		default:
			ext = ".jpg"
		}
		dest := filepath.Join(workDir, fmt.Sprintf("img_%03d%s", i, ext))
		if err := downloadFile(ctx, u, dest); err != nil {
			log.Printf("⚠️ slideshow job %d: skip image %d (%s): %v", jobID, i, truncateMediaURL(u), err)
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
	log.Printf("🎞️ slideshow job %d: rendering %d image(s) with ffmpeg…", jobID, len(localImages))
	if err := GenerateSlideshowMP4(ctx, SlideshowInput{
		ImagePaths:  localImages,
		MusicPath:   musicPath,
		OutputPath:  outPath,
		Title:       job.OverlayTitle,
		Location:    job.OverlayLocation,
		Area:        job.OverlayArea,
		Price:       job.OverlayPrice,
		PlotNumber:  job.OverlayPlotNumber,
		CTA:         job.OverlayCTA,
		SecPerSlide: slideshowSecPerSlide,
	}); err != nil {
		updateJob("failed", 0, err.Error(), "")
		return err
	}

	updateJob("processing", 80, "", "")
	objectKey := fmt.Sprintf("property-sales/%d/auto-slideshow_%d.mp4", job.EntityID, jobID)
	if job.EntityType == "land" {
		objectKey = fmt.Sprintf("landmarks/%d/auto-slideshow_%d.mp4", job.EntityID, jobID)
	}
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
	} else if job.EntityType == "land" {
		if err := attachSlideshowToLandmark(db, job.EntityID, videoURL); err != nil {
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

func attachSlideshowToLandmark(db *gorm.DB, landmarkID uint, videoURL string) error {
	var lm models.Landmark
	if err := db.First(&lm, landmarkID).Error; err != nil {
		return err
	}
	if lm.VideoURL != nil && strings.TrimSpace(*lm.VideoURL) == videoURL {
		return nil
	}
	mediaType := "video"
	if len(landmarkImageURLs(lm.Images)) > 0 {
		mediaType = "both"
	}
	return db.Model(&lm).Updates(map[string]interface{}{
		"video_url":  videoURL,
		"media_type": mediaType,
	}).Error
}

func landmarkImageURLs(raw []byte) []string {
	return slideshowEligibleImages(parseJSONStringList(raw))
}

func formatLandmarkLocation(lm models.Landmark) string {
	parts := []string{}
	for _, p := range []string{lm.District, lm.Region} {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func formatLandmarkArea(lm models.Landmark) string {
	if lm.Area <= 0 {
		return ""
	}
	unit := strings.TrimSpace(lm.AreaUnit)
	if unit == "" || unit == "sqm" {
		return fmt.Sprintf("%.0f m²", lm.Area)
	}
	return fmt.Sprintf("%.0f %s", lm.Area, unit)
}

func formatLandmarkPlotNumber(lm models.Landmark) string {
	p := strings.TrimSpace(lm.PlotNumber)
	if p == "" {
		return ""
	}
	return "Plot #" + p
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
	return storage.NormalizePublicMediaURLs(urls)
}

// slideshowEligibleImages prefers DigitalOcean Spaces URLs; skips dead Cloudinary legacy links.
func slideshowEligibleImages(urls []string) []string {
	normalized := storage.NormalizePublicMediaURLs(urls)
	if len(normalized) == 0 {
		return nil
	}
	spaces := make([]string, 0, len(normalized))
	other := make([]string, 0, len(normalized))
	for _, u := range normalized {
		lower := strings.ToLower(u)
		if strings.Contains(lower, "digitaloceanspaces.com") {
			spaces = append(spaces, u)
		} else if strings.Contains(lower, "res.cloudinary.com") {
			continue
		} else {
			other = append(other, u)
		}
	}
	if len(spaces) > 0 {
		return spaces
	}
	return other
}

func countImageSources(urls []string) (spaces, cloudinary, other int) {
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		lower := strings.ToLower(u)
		switch {
		case strings.Contains(lower, "digitaloceanspaces.com"):
			spaces++
		case strings.Contains(lower, "res.cloudinary.com"):
			cloudinary++
		default:
			other++
		}
	}
	return
}

func parseJSONStringList(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var urls []string
	if json.Unmarshal(raw, &urls) != nil {
		return nil
	}
	return urls
}

func freshSlideshowImageURLs(db *gorm.DB, job *models.PropertyVideoGenerationJob) []string {
	switch job.EntityType {
	case "land":
		var lm models.Landmark
		if err := db.First(&lm, job.EntityID).Error; err == nil {
			return landmarkImageURLs(lm.Images)
		}
	case "sale":
		var sale models.PropertySale
		if err := db.First(&sale, job.EntityID).Error; err == nil {
			return slideshowEligibleImages(sale.Images)
		}
	}
	var stored []string
	_ = json.Unmarshal(job.ImageURLs, &stored)
	return slideshowEligibleImages(stored)
}

func truncateMediaURL(u string) string {
	u = strings.TrimSpace(u)
	if len(u) <= 96 {
		return u
	}
	return u[:48] + "…" + u[len(u)-40:]
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// RepairLandmarkVideoFromJob copies output_video_url from a completed slideshow job onto the landmark when missing.
func RepairLandmarkVideoFromJob(db *gorm.DB, landmarkID uint) bool {
	return repairLandmarkVideoFromJob(db, landmarkID)
}

// LatestCompletedLandSlideshowURL returns the newest completed slideshow MP4 URL for a land listing.
func LatestCompletedLandSlideshowURL(db *gorm.DB, landmarkID uint) string {
	var job models.PropertyVideoGenerationJob
	err := db.Where("entity_type = ? AND entity_id = ? AND status = ? AND TRIM(output_video_url) <> ''",
		"land", landmarkID, "completed").
		Order("id DESC").First(&job).Error
	if err != nil {
		return ""
	}
	return strings.TrimSpace(job.OutputVideoURL)
}

func repairLandmarkVideoFromJob(db *gorm.DB, landmarkID uint) bool {
	var lm models.Landmark
	if err := db.First(&lm, landmarkID).Error; err != nil {
		return false
	}
	if lm.VideoURL != nil && strings.TrimSpace(*lm.VideoURL) != "" {
		return true
	}
	var job models.PropertyVideoGenerationJob
	err := db.Where("entity_type = ? AND entity_id = ? AND status = ? AND output_video_url <> ''",
		"land", landmarkID, "completed").
		Order("id DESC").First(&job).Error
	if err != nil {
		return false
	}
	if err := attachSlideshowToLandmark(db, landmarkID, job.OutputVideoURL); err != nil {
		log.Printf("⚠️ slideshow repair land=%d from job %d: %v", landmarkID, job.ID, err)
		return false
	}
	log.Printf("✅ slideshow repaired land=%d video_url from completed job %d", landmarkID, job.ID)
	return true
}

func sanitizeLandmarkImageRecords(db *gorm.DB) int {
	var landmarks []models.Landmark
	if err := db.Find(&landmarks).Error; err != nil {
		return 0
	}
	ctx := context.Background()
	fixed := 0
	migrated := 0
	for _, lm := range landmarks {
		if len(lm.Images) == 0 {
			continue
		}
		var raw []string
		if json.Unmarshal(lm.Images, &raw) != nil {
			continue
		}
		changed := false
		for i, u := range raw {
			if !strings.Contains(u, "res.cloudinary.com") {
				continue
			}
			newURL, err := storage.MigrateCloudinaryURLToSpaces(ctx, u)
			if err != nil {
				log.Printf("ℹ️ slideshow migrate land=%d %q: removing dead cloudinary link", lm.ID, truncateMediaURL(u))
				raw[i] = ""
				changed = true
				continue
			}
			if newURL != u && newURL != "" {
				raw[i] = newURL
				changed = true
				migrated++
			}
		}
		cleaned := slideshowEligibleImages(compactStrings(raw))
		if len(cleaned) == 0 {
			if len(raw) > 0 {
				sp, cl, ot := countImageSources(raw)
				log.Printf("ℹ️ slideshow land=%d %q: no Spaces images (spaces=%d cloudinary=%d other=%d) — re-upload photos",
					lm.ID, strings.TrimSpace(lm.Title), sp, cl, ot)
			}
			continue
		}
		if !changed {
			same := len(cleaned) == len(raw)
			if same {
				for i := range cleaned {
					if cleaned[i] != strings.TrimSpace(raw[i]) {
						same = false
						break
					}
				}
			}
			if same {
				continue
			}
		}
		b, err := json.Marshal(cleaned)
		if err != nil {
			continue
		}
		if err := db.Model(&models.Landmark{}).Where("id = ?", lm.ID).Update("images", b).Error; err != nil {
			log.Printf("⚠️ slideshow sanitize land=%d images: %v", lm.ID, err)
			continue
		}
		fixed++
	}
	if migrated > 0 {
		log.Printf("🎞️ slideshow: migrated %d cloudinary image(s) to Spaces", migrated)
	}
	return fixed
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

func slideshowBackfillEnabled() bool {
	if strings.EqualFold(os.Getenv("SLIDESHOW_BACKFILL_ENABLED"), "false") {
		return false
	}
	return true
}

func slideshowBackfillLimit() int {
	if v := strings.TrimSpace(os.Getenv("SLIDESHOW_BACKFILL_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 500
}

func landmarkOwnerUserID(db *gorm.DB, lm *models.Landmark) uint {
	if lm.OwnerID != nil && *lm.OwnerID > 0 {
		return *lm.OwnerID
	}
	if lm.OrganizationID != nil && *lm.OrganizationID > 0 {
		var org models.Organization
		if err := db.Select("owner_id").First(&org, *lm.OrganizationID).Error; err == nil && org.OwnerID > 0 {
			return org.OwnerID
		}
	}
	if lm.VerifiedBy != nil && *lm.VerifiedBy > 0 {
		return *lm.VerifiedBy
	}
	return 0
}

// BackfillSlideshowVideosOnStart scans all lands and property sales with Spaces photos but no video.
func BackfillSlideshowVideosOnStart(db *gorm.DB) {
	reconcileLandmarkSlideshowJobs(db)
	backfillLandmarksWithoutVideos(db)
	backfillPropertySalesWithoutVideos(db)
}

// ReconcileLandmarkSlideshowJobs repairs existing lands, unstucks jobs, and re-queues pending work.
// Call on deploy so verified lands created before slideshow rollout get videos generated.
func ReconcileLandmarkSlideshowJobs(db *gorm.DB) {
	reconcileLandmarkSlideshowJobs(db)
}

func reconcileLandmarkSlideshowJobs(db *gorm.DB) {
	if db == nil {
		return
	}
	if !slideshowEnabled() {
		log.Printf("⚠️ slideshow reconcile skipped: ffmpeg not available")
		return
	}

	staleProcessing := time.Now().Add(-45 * time.Minute)
	if res := db.Model(&models.PropertyVideoGenerationJob{}).
		Where("entity_type = ? AND status = ? AND updated_at < ?", "land", "processing", staleProcessing).
		Updates(map[string]interface{}{"status": "pending", "progress": 0, "error_message": ""}); res.RowsAffected > 0 {
		log.Printf("🔄 slideshow reconcile: reset %d stale land processing job(s)", res.RowsAffected)
	}

	stalePending := time.Now().Add(-6 * time.Hour)
	if res := db.Model(&models.PropertyVideoGenerationJob{}).
		Where("entity_type = ? AND status = ? AND progress = 0 AND created_at < ?", "land", "pending", stalePending).
		Update("status", "failed"); res.RowsAffected > 0 {
		log.Printf("🔄 slideshow reconcile: expired %d stale pending land job(s)", res.RowsAffected)
	}

	var missingVideoIDs []uint
	if err := db.Model(&models.Landmark{}).
		Where("(video_url IS NULL OR TRIM(video_url) = '')").
		Where("status <> ?", "inactive").
		Pluck("id", &missingVideoIDs).Error; err != nil {
		log.Printf("❌ slideshow reconcile: list lands missing video: %v", err)
	} else {
		repaired := 0
		for _, id := range missingVideoIDs {
			if repairLandmarkVideoFromJob(db, id) {
				repaired++
			}
		}
		log.Printf("🔄 slideshow reconcile: repaired %d/%d land(s) from completed jobs", repaired, len(missingVideoIDs))
	}

	var pending []models.PropertyVideoGenerationJob
	if err := db.Where("entity_type = ? AND status = ?", "land", "pending").Order("id ASC").Find(&pending).Error; err == nil && len(pending) > 0 {
		log.Printf("🔄 slideshow reconcile: re-queueing %d pending land job(s)", len(pending))
		for _, job := range pending {
			jid := job.ID
			if pushSlideshowRedis(jid) {
				continue
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				defer cancel()
				if err := ProcessSlideshowJob(ctx, db, jid); err != nil {
					log.Printf("❌ slideshow reconcile job %d: %v", jid, err)
				}
			}()
			time.Sleep(50 * time.Millisecond)
		}
	}

	var failed []models.PropertyVideoGenerationJob
	if err := db.Where("entity_type = ? AND status = ?", "land", "failed").Order("id DESC").Find(&failed).Error; err != nil {
		return
	}
	seenLand := map[uint]bool{}
	for _, job := range failed {
		if seenLand[job.EntityID] {
			continue
		}
		seenLand[job.EntityID] = true

		var lm models.Landmark
		if err := db.First(&lm, job.EntityID).Error; err != nil {
			continue
		}
		if lm.VideoURL != nil && strings.TrimSpace(*lm.VideoURL) != "" {
			continue
		}
		if len(slideshowEligibleImages(landmarkImageURLs(lm.Images))) < slideshowMinImages() {
			continue
		}
		uid := landmarkOwnerUserID(db, &lm)
		if uid == 0 {
			continue
		}
		var active models.PropertyVideoGenerationJob
		if err := db.Where("entity_type = ? AND entity_id = ? AND status IN ?",
			"land", job.EntityID, []string{"pending", "processing"}).First(&active).Error; err == nil {
			continue
		}
		if _, err := EnqueueLandmarkSlideshow(db, job.EntityID, uid); err != nil {
			log.Printf("⚠️ slideshow reconcile retry land=%d: %v", job.EntityID, err)
			continue
		}
		log.Printf("🔄 slideshow reconcile: re-enqueued land=%d after failed job", job.EntityID)
		time.Sleep(100 * time.Millisecond)
	}

	EnqueueMissingLandmarkSlideshowsBatch(db, 100)
}

// EnqueueMissingLandmarkSlideshowsBatch queues slideshow generation for verified lands without video.
func EnqueueMissingLandmarkSlideshowsBatch(db *gorm.DB, limit int) {
	if db == nil || limit <= 0 || !slideshowEnabled() {
		return
	}
	minImg := slideshowMinImages()
	var landmarks []models.Landmark
	q := db.Where("(video_url IS NULL OR TRIM(video_url) = '')").
		Where("is_verified = ? AND is_published = ?", true, true).
		Where("status <> ?", "inactive").
		Order("id ASC").
		Limit(limit)
	if err := q.Find(&landmarks).Error; err != nil {
		log.Printf("❌ slideshow enqueue batch: %v", err)
		return
	}
	if len(landmarks) == 0 {
		return
	}
	var enqueued int
	for _, lm := range landmarks {
		if repairLandmarkVideoFromJob(db, lm.ID) {
			continue
		}
		if len(slideshowEligibleImages(landmarkImageURLs(lm.Images))) < minImg {
			continue
		}
		uid := landmarkOwnerUserID(db, &lm)
		if uid == 0 {
			continue
		}
		job, err := EnqueueLandmarkSlideshow(db, lm.ID, uid)
		if err != nil || job == nil {
			continue
		}
		enqueued++
		time.Sleep(80 * time.Millisecond)
	}
	if enqueued > 0 {
		log.Printf("🎞️ slideshow enqueue batch: queued %d verified land(s)", enqueued)
	}
}

// BackfillLandmarksWithoutVideos runs on server startup for land listings.
func BackfillLandmarksWithoutVideos(db *gorm.DB) {
	backfillLandmarksWithoutVideos(db)
}

func backfillLandmarksWithoutVideos(db *gorm.DB) {
	if db == nil {
		return
	}
	if !slideshowBackfillEnabled() {
		log.Printf("ℹ️ slideshow backfill disabled (SLIDESHOW_BACKFILL_ENABLED=false)")
		return
	}
	if !slideshowEnabled() {
		log.Printf("⚠️ slideshow backfill skipped: ffmpeg not available")
		return
	}

	if fixed := sanitizeLandmarkImageRecords(db); fixed > 0 {
		log.Printf("🧹 slideshow: cleaned invalid/local image paths on %d landmark(s)", fixed)
	}

	minImg := slideshowMinImages()
	var landmarks []models.Landmark
	q := db.Where("(video_url IS NULL OR TRIM(video_url) = '')").
		Where("status <> ?", "inactive").
		Order("is_verified DESC, is_published DESC, id ASC")
	if limit := slideshowBackfillLimit(); limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&landmarks).Error; err != nil {
		log.Printf("❌ slideshow backfill: query failed: %v", err)
		return
	}

	log.Printf("🎞️ slideshow backfill — %d land listing(s) without video:", len(landmarks))
	for _, lm := range landmarks {
		raw := parseJSONStringList(lm.Images)
		eligible := slideshowEligibleImages(raw)
		sp, cl, ot := countImageSources(raw)
		log.Printf("   • land #%d %q — eligible=%d (spaces=%d cloudinary=%d other=%d)",
			lm.ID, strings.TrimSpace(lm.Title), len(eligible), sp, cl, ot)
	}

	var enqueued, skippedNoImages, skippedTooFew, skippedNoOwner, skippedAlready, skippedRepaired, failed int
	for _, lm := range landmarks {
		if repairLandmarkVideoFromJob(db, lm.ID) {
			skippedRepaired++
			continue
		}
		raw := parseJSONStringList(lm.Images)
		eligible := slideshowEligibleImages(raw)
		if len(eligible) == 0 {
			skippedNoImages++
			continue
		}
		if len(eligible) < minImg {
			skippedTooFew++
			continue
		}
		uid := landmarkOwnerUserID(db, &lm)
		if uid == 0 {
			skippedNoOwner++
			continue
		}
		job, err := EnqueueLandmarkSlideshow(db, lm.ID, uid)
		if err != nil {
			failed++
			log.Printf("⚠️ slideshow backfill land=%d: %v", lm.ID, err)
			continue
		}
		if job == nil {
			skippedAlready++
			continue
		}
		enqueued++
		time.Sleep(150 * time.Millisecond)
	}

	log.Printf("🎞️ slideshow land backfill done: scanned=%d enqueued=%d repaired=%d skip_no_spaces=%d skip_too_few=%d skip_no_owner=%d skip_already=%d failed=%d (min_images=%d)",
		len(landmarks), enqueued, skippedRepaired, skippedNoImages, skippedTooFew, skippedNoOwner, skippedAlready, failed, minImg)
}

func propertySaleUserID(db *gorm.DB, sale *models.PropertySale) uint {
	if sale.OwnerID != nil && *sale.OwnerID > 0 {
		return *sale.OwnerID
	}
	if sale.OrganizationID != nil && *sale.OrganizationID > 0 {
		var org models.Organization
		if err := db.Select("owner_id").First(&org, *sale.OrganizationID).Error; err == nil && org.OwnerID > 0 {
			return org.OwnerID
		}
	}
	if sale.AgentID != nil && *sale.AgentID > 0 {
		var agent models.Agent
		if err := db.Select("user_id").First(&agent, *sale.AgentID).Error; err == nil && agent.UserID > 0 {
			return agent.UserID
		}
	}
	return 0
}

func backfillPropertySalesWithoutVideos(db *gorm.DB) {
	if db == nil || !slideshowBackfillEnabled() || !slideshowEnabled() {
		return
	}

	minImg := slideshowMinImages()
	var sales []models.PropertySale
	q := db.Where("deleted_at IS NULL").
		Where("status NOT IN ?", []string{"withdrawn", "sold"}).
		Order("id ASC")
	if limit := slideshowBackfillLimit(); limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&sales).Error; err != nil {
		log.Printf("❌ slideshow sale backfill: query failed: %v", err)
		return
	}

	var noVideo []models.PropertySale
	var candidates []models.PropertySale
	for _, s := range sales {
		if len(s.Videos) > 0 {
			continue
		}
		noVideo = append(noVideo, s)
		if len(slideshowEligibleImages(s.Images)) >= minImg {
			candidates = append(candidates, s)
		}
	}

	log.Printf("🎞️ slideshow backfill — %d property sale(s) without video:", len(noVideo))
	for _, s := range noVideo {
		raw := s.Images
		eligible := slideshowEligibleImages(raw)
		sp, cl, ot := countImageSources(raw)
		log.Printf("   • sale #%d %q — eligible=%d (spaces=%d cloudinary=%d other=%d)",
			s.ID, strings.TrimSpace(s.Title), len(eligible), sp, cl, ot)
	}
	if len(candidates) == 0 && len(noVideo) > 0 {
		log.Printf("ℹ️ slideshow sale backfill: none queued (need Spaces images + no existing videos JSON)")
	} else if len(noVideo) == 0 {
		log.Printf("ℹ️ slideshow sale backfill: all property sales already have videos[] set")
	}

	var enqueued, skippedNoOwner, skippedAlready, failed int
	for _, s := range candidates {
		uid := propertySaleUserID(db, &s)
		if uid == 0 {
			skippedNoOwner++
			continue
		}
		job, err := EnqueuePropertySaleSlideshow(db, s.ID, uid)
		if err != nil {
			failed++
			log.Printf("⚠️ slideshow backfill sale=%d: %v", s.ID, err)
			continue
		}
		if job == nil {
			skippedAlready++
			continue
		}
		enqueued++
		time.Sleep(150 * time.Millisecond)
	}

	log.Printf("🎞️ slideshow sale backfill done: candidates=%d enqueued=%d skip_no_owner=%d skip_already=%d failed=%d",
		len(candidates), enqueued, skippedNoOwner, skippedAlready, failed)
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
