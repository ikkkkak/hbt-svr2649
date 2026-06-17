package videoprocessing

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services/mediaoptimize"
	"apartments-clone-server/storage"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/gorm"
)

const saleVideoEntity = "sale"

// ProcessPropertySaleVideo transcodes a property_sale_videos row to HLS (same ladder as rent feed).
func ProcessPropertySaleVideo(ctx context.Context, db *gorm.DB, saleVideoID, userID uint) error {
	var video models.PropertySaleVideo
	if err := db.First(&video, saleVideoID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(video.VideoURL) == "" {
		return failSaleJob(db, saleVideoID, userID, fmt.Errorf("empty source url"))
	}

	emitSale(userID, saleVideoID, "processing", 10, "", "", "", false, "", "")

	_ = db.Model(&video).Updates(map[string]interface{}{
		"processing_status":   "processing",
		"processing_error":    "",
		"processing_progress": 10,
	}).Error

	workDir, err := os.MkdirTemp("", fmt.Sprintf("psvproc_%d_", saleVideoID))
	if err != nil {
		return failSaleJob(db, saleVideoID, userID, err)
	}
	defer os.RemoveAll(workDir)

	sourcePath := filepath.Join(workDir, "source.mp4")
	emitSale(userID, saleVideoID, "processing", 15, "", "", "", false, "", "")
	if err := downloadFile(ctx, video.VideoURL, sourcePath); err != nil {
		return failSaleJob(db, saleVideoID, userID, err)
	}

	if !mediaoptimize.AlreadyOptimizedAtUpload(video.VideoURL) {
		if opt, err := mediaoptimize.OptimizeVideo(ctx, sourcePath, mediaoptimize.LoadConfig()); err == nil {
			if !opt.Skipped && opt.OutputPath != "" && opt.OutputPath != sourcePath {
				defer func() { _ = os.RemoveAll(mediaoptimize.TempDirFor(opt.OutputPath)) }()
				sourcePath = opt.OutputPath
			}
		} else {
			log.Printf("⚠️ sale videoprocessing %d mezzanine optimize: %v", saleVideoID, err)
		}
	}

	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		log.Printf("⚠️ videoprocessing: ffmpeg not found — sale video %d MP4-only", saleVideoID)
		return markSaleReadyMP4Only(db, saleVideoID, userID, video.VideoURL)
	}

	emitSale(userID, saleVideoID, "processing", 25, "", "", "", false, "", "")

	hlsRoot := filepath.Join(workDir, "hls")
	assetBase := fmt.Sprintf("property-sale-videos/%d", saleVideoID)
	var spriteURL, blurURL string
	var assetWg sync.WaitGroup
	assetWg.Add(2)
	go func() {
		defer assetWg.Done()
		spriteURL = generateSpriteSheet(ctx, ffmpeg, sourcePath, workDir, assetBase+"/sprite")
	}()
	go func() {
		defer assetWg.Done()
		blurURL = generateBlurPreview(ctx, ffmpeg, sourcePath, workDir, assetBase+"/preview_blur")
	}()

	type ladderResult struct {
		order    int
		rend     Rendition
		bw       int
		masterLn []string
	}
	results := make(chan ladderResult, len(activeLadder()))
	var wg sync.WaitGroup
	for i, rung := range activeLadder() {
		i, rung := i, rung
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}
			variantDir := filepath.Join(hlsRoot, fmt.Sprintf("%dx%d", rung.Width, rung.Height))
			if err := os.MkdirAll(variantDir, 0755); err != nil {
				return
			}
			playlistLocal := filepath.Join(variantDir, "playlist.m3u8")
			segPattern := filepath.Join(variantDir, "seg_%03d.ts")
			scaleVF := fmt.Sprintf(
				"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1",
				rung.Width, rung.Height, rung.Width, rung.Height,
			)
			args := []string{
				"-y", "-i", sourcePath,
				"-map_metadata", "-1",
				"-vf", scaleVF,
				"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high",
				"-crf", "28",
				"-maxrate", rung.Bitrate, "-bufsize", "2M",
				"-c:a", "aac", "-b:a", "96k", "-ac", "2",
				"-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
				"-hls_time", "4", "-hls_init_time", "2",
				"-hls_playlist_type", "vod",
				"-hls_segment_filename", segPattern,
				playlistLocal,
			}
			if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
				log.Printf("⚠️ skip %dx%d sale video %d: %v %s", rung.Width, rung.Height, saleVideoID, err, string(out))
				return
			}
			cdnPrefix := fmt.Sprintf("hls/property-sale-videos/%d/%dx%d", saleVideoID, rung.Width, rung.Height)
			playlistURL, err := uploadDirectory(variantDir, cdnPrefix)
			if err != nil || playlistURL == "" {
				return
			}
			bw := parseBitrateK(rung.Bitrate) * 1000
			results <- ladderResult{
				order: i,
				rend:  Rendition{Width: rung.Width, Height: rung.Height, Playlist: playlistURL},
				bw:    bw,
				masterLn: []string{
					fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d", bw, rung.Width, rung.Height),
					playlistURL,
				},
			}
		}()
	}
	wg.Wait()
	close(results)
	assetWg.Wait()
	emitSale(userID, saleVideoID, "processing", 35, "", "", "", false, spriteURL, blurURL)

	byOrder := make([]ladderResult, len(activeLadder()))
	for r := range results {
		if r.order >= 0 && r.order < len(byOrder) {
			byOrder[r.order] = r
		}
	}
	var renditions []Rendition
	masterLines := []string{"#EXTM3U", "#EXT-X-VERSION:3"}
	for _, r := range byOrder {
		if r.rend.Playlist == "" {
			continue
		}
		renditions = append(renditions, r.rend)
		masterLines = append(masterLines, r.masterLn...)
		pct := 40 + (len(renditions)*12)
		emitSale(userID, saleVideoID, "processing", pct, "", "", "", false, spriteURL, blurURL)
	}

	if len(renditions) == 0 {
		return markSaleReadyMP4Only(db, saleVideoID, userID, video.VideoURL)
	}

	emitSale(userID, saleVideoID, "processing", 88, "", "", "", false, spriteURL, blurURL)

	masterLocal := filepath.Join(hlsRoot, "master.m3u8")
	if err := os.WriteFile(masterLocal, []byte(strings.Join(masterLines, "\n")+"\n"), 0644); err != nil {
		return failSaleJob(db, saleVideoID, userID, err)
	}
	masterRes := storage.UploadLocalFileObjectKey(masterLocal, fmt.Sprintf("hls/property-sale-videos/%d/master.m3u8", saleVideoID), "application/vnd.apple.mpegurl")
	masterURL := masterRes["url"]
	if masterURL == "" {
		return failSaleJob(db, saleVideoID, userID, fmt.Errorf("master upload: %s", masterRes["error"]))
	}

	mobileURL := ""
	mobilePath := filepath.Join(workDir, "mobile_540.mp4")
	mcfg := mediaoptimize.LoadConfig()
	mobileArgs := []string{
		"-y", "-i", sourcePath,
		"-map_metadata", "-1",
		"-vf", "scale=540:960:force_original_aspect_ratio=decrease,pad=540:960:(ow-iw)/2:(oh-ih)/2",
		"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high",
		"-crf", fmt.Sprintf("%d", mcfg.VideoMobileCRF),
		"-maxrate", "900k", "-bufsize", "1800k",
		"-c:a", "aac", "-b:a", "64k", "-movflags", "+faststart",
		mobilePath,
	}
	if _, err := exec.CommandContext(ctx, ffmpeg, mobileArgs...).CombinedOutput(); err == nil {
		mobileRes := storage.UploadLocalFile(mobilePath, assetBase+"/mobile.mp4", "video/mp4")
		mobileURL = mobileRes["url"]
	}

	rendJSON, _ := json.Marshal(renditions)
	updates := map[string]interface{}{
		"hls_url":             masterURL,
		"mobile_video_url":    mobileURL,
		"processing_status":   "ready",
		"processing_error":    "",
		"processing_progress": 100,
		"renditions_json":     rendJSON,
		"sprite_sheet_url":    spriteURL,
		"preview_blur_url":    blurURL,
	}
	if strings.TrimSpace(video.ThumbnailURL) == "" && blurURL != "" {
		updates["thumbnail_url"] = blurURL
	}
	if err := db.Model(&models.PropertySaleVideo{}).Where("id = ?", saleVideoID).Updates(updates).Error; err != nil {
		return err
	}

	emitSale(userID, saleVideoID, "ready", 100, masterURL, mobileURL, "", true, spriteURL, blurURL)
	log.Printf("✅ sale videoprocessing: video %d ready — %s", saleVideoID, masterURL)
	return nil
}

func markSaleReadyMP4Only(db *gorm.DB, saleVideoID, userID uint, mp4URL string) error {
	_ = db.Model(&models.PropertySaleVideo{}).Where("id = ?", saleVideoID).Updates(map[string]interface{}{
		"processing_status":   "ready",
		"hls_url":             "",
		"mobile_video_url":    mp4URL,
		"processing_error":    "",
		"processing_progress": 100,
	}).Error
	emitSale(userID, saleVideoID, "ready", 100, "", mp4URL, "", true, "", "")
	go func() {
		blur := QuickBlurFromVideoURL(mp4URL, fmt.Sprintf("property-sale-videos/%d/preview_blur", saleVideoID))
		if blur == "" {
			return
		}
		updates := map[string]interface{}{
			"preview_blur_url": blur,
		}
		var row models.PropertySaleVideo
		if db.First(&row, saleVideoID).Error == nil && strings.TrimSpace(row.ThumbnailURL) == "" {
			updates["thumbnail_url"] = blur
		}
		_ = db.Model(&models.PropertySaleVideo{}).Where("id = ?", saleVideoID).Updates(updates).Error
		emitSale(userID, saleVideoID, "ready", 100, "", mp4URL, "", true, "", blur)
		log.Printf("✅ sale video %d preview blur → %s", saleVideoID, blur)
	}()
	return nil
}

func failSaleJob(db *gorm.DB, saleVideoID, userID uint, err error) error {
	msg := err.Error()
	log.Printf("❌ sale videoprocessing video %d: %s", saleVideoID, msg)
	_ = db.Model(&models.PropertySaleVideo{}).Where("id = ?", saleVideoID).Updates(map[string]interface{}{
		"processing_status": "failed",
		"processing_error":  msg,
	}).Error
	emitSale(userID, saleVideoID, "failed", 0, "", "", msg, false, "", "")
	return err
}

func emitSale(userID, saleVideoID uint, status string, progress int, hls, mobile, errMsg string, ready bool, sprite, blur string) {
	if userID == 0 {
		return
	}
	PublishProcessing(userID, ProcessingEvent{
		VideoID:          saleVideoID,
		EntityType:       saleVideoEntity,
		ProcessingStatus: status,
		ProcessingError:  errMsg,
		Progress:         progress,
		HlsURL:           hls,
		MobileVideoURL:   mobile,
		SpriteSheetURL:   sprite,
		PreviewBlurURL:   blur,
		Ready:            ready,
	})
}
