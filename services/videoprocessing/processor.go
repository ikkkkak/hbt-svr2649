package videoprocessing

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services/mediaoptimize"
	"apartments-clone-server/storage"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/gorm"
)

type Rendition struct {
	Height   int    `json:"height"`
	Width    int    `json:"width"`
	Playlist string `json:"playlist_url"`
}

// Portrait ladder (9:16) — width x height
var portraitLadder = []struct {
	Width   int
	Height  int
	Bitrate string
}{
	{360, 640, "450k"},
	{540, 960, "900k"},
	{720, 1280, "1800k"},
	{1080, 1920, "4500k"},
}

func ProcessVideo(ctx context.Context, db *gorm.DB, videoID, userID uint) error {
	var video models.Video
	if err := db.First(&video, videoID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(video.VideoURL) == "" {
		return failJob(db, videoID, userID, fmt.Errorf("empty source url"))
	}

	emit(userID, videoID, "processing", 10, "", "", "", false, "", "")

	_ = db.Model(&video).Updates(map[string]interface{}{
		"processing_status":   "processing",
		"processing_error":    "",
		"processing_progress": 10,
	}).Error

	workDir, err := os.MkdirTemp("", fmt.Sprintf("vproc_%d_", videoID))
	if err != nil {
		return failJob(db, videoID, userID, err)
	}
	defer os.RemoveAll(workDir)

	sourcePath := filepath.Join(workDir, "source.mp4")
	emit(userID, videoID, "processing", 15, "", "", "", false, "", "")
	if err := downloadFile(ctx, video.VideoURL, sourcePath); err != nil {
		return failJob(db, videoID, userID, err)
	}

	// Skip second FFmpeg pass when upload path already normalized (saves minutes on host uploads).
	if !mediaoptimize.AlreadyOptimizedAtUpload(video.VideoURL) {
		if opt, err := mediaoptimize.OptimizeVideo(ctx, sourcePath, mediaoptimize.LoadConfig()); err == nil {
			if !opt.Skipped && opt.OutputPath != "" && opt.OutputPath != sourcePath {
				defer func() { _ = os.RemoveAll(mediaoptimize.TempDirFor(opt.OutputPath)) }()
				sourcePath = opt.OutputPath
			}
		} else {
			log.Printf("⚠️ videoprocessing %d mezzanine optimize: %v", videoID, err)
		}
	} else {
		log.Printf("⏩ videoprocessing %d: skip mezzanine (already optimized at upload)", videoID)
	}

	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		log.Printf("⚠️ videoprocessing: ffmpeg not found — video %d MP4-only", videoID)
		return markReadyMP4Only(db, videoID, userID, video.VideoURL)
	}

	emit(userID, videoID, "processing", 25, "", "", "", false, "", "")

	hlsRoot := filepath.Join(workDir, "hls")
	var spriteURL, blurURL string
	var assetWg sync.WaitGroup
	assetWg.Add(2)
	go func() {
		defer assetWg.Done()
		spriteURL = generateSpriteSheet(ctx, ffmpeg, sourcePath, workDir, fmt.Sprintf("videos/%d/sprite", videoID))
	}()
	go func() {
		defer assetWg.Done()
		blurURL = generateBlurPreview(ctx, ffmpeg, sourcePath, workDir, fmt.Sprintf("videos/%d/preview_blur", videoID))
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
				log.Printf("⚠️ skip %dx%d video %d: %v %s", rung.Width, rung.Height, videoID, err, string(out))
				return
			}
			cdnPrefix := fmt.Sprintf("hls/videos/%d/%dx%d", videoID, rung.Width, rung.Height)
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
	emit(userID, videoID, "processing", 35, "", "", "", false, spriteURL, blurURL)

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
		emit(userID, videoID, "processing", pct, "", "", "", false, spriteURL, blurURL)
	}

	if len(renditions) == 0 {
		return markReadyMP4Only(db, videoID, userID, video.VideoURL)
	}

	emit(userID, videoID, "processing", 88, "", "", "", false, spriteURL, blurURL)

	masterLocal := filepath.Join(hlsRoot, "master.m3u8")
	if err := os.WriteFile(masterLocal, []byte(strings.Join(masterLines, "\n")+"\n"), 0644); err != nil {
		return failJob(db, videoID, userID, err)
	}
	masterRes := storage.UploadLocalFileObjectKey(masterLocal, fmt.Sprintf("hls/videos/%d/master.m3u8", videoID), "application/vnd.apple.mpegurl")
	masterURL := masterRes["url"]
	if masterURL == "" {
		return failJob(db, videoID, userID, fmt.Errorf("master upload: %s", masterRes["error"]))
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
		mobileRes := storage.UploadLocalFile(mobilePath, fmt.Sprintf("videos/%d/mobile.mp4", videoID), "video/mp4")
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
	}
	if err := db.Model(&models.Video{}).Where("id = ?", videoID).Updates(updates).Error; err != nil {
		return err
	}

	emit(userID, videoID, "ready", 100, masterURL, mobileURL, "", true, spriteURL, blurURL)
	log.Printf("✅ videoprocessing: video %d ready — %s", videoID, masterURL)
	return nil
}

// generateBlurPreview builds a tiny blurred first-frame JPEG for feed placeholders.
func generateBlurPreview(ctx context.Context, ffmpeg, sourcePath, workDir, uploadKey string) string {
	outPath := filepath.Join(workDir, "preview_blur.jpg")
	args := []string{
		"-y", "-ss", "0", "-i", sourcePath,
		"-vf", "scale=320:-1,boxblur=10:1",
		"-frames:v", "1",
		outPath,
	}
	if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		log.Printf("⚠️ blur preview %s: %v %s", uploadKey, err, string(out))
		return ""
	}
	res := storage.UploadLocalFile(outPath, uploadKey, "image/jpeg")
	return res["url"]
}

func generateSpriteSheet(ctx context.Context, ffmpeg, sourcePath, workDir, uploadKey string) string {
	outDir := filepath.Join(workDir, "sprites")
	_ = os.MkdirAll(outDir, 0755)
	// Single tiled sprite image (5x5 grid, 1 frame every 2s)
	args := []string{
		"-y", "-i", sourcePath,
		"-vf", "fps=0.5,scale=160:-1,tile=5x5:padding=2:color=black",
		"-frames:v", "1",
		filepath.Join(outDir, "sheet.jpg"),
	}
	if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		log.Printf("⚠️ sprite sheet %s: %v %s", uploadKey, err, string(out))
		return ""
	}
	sheetPath := filepath.Join(outDir, "sheet.jpg")
	res := storage.UploadLocalFile(sheetPath, uploadKey, "image/jpeg")
	return res["url"]
}

func markReadyMP4Only(db *gorm.DB, videoID, userID uint, mp4URL string) error {
	_ = db.Model(&models.Video{}).Where("id = ?", videoID).Updates(map[string]interface{}{
		"processing_status":   "ready",
		"hls_url":             "",
		"mobile_video_url":    mp4URL,
		"processing_error":    "",
		"processing_progress": 100,
	}).Error
	emit(userID, videoID, "ready", 100, "", mp4URL, "", true, "", "")
	return nil
}

func failJob(db *gorm.DB, videoID, userID uint, err error) error {
	msg := err.Error()
	log.Printf("❌ videoprocessing video %d: %s", videoID, msg)
	_ = db.Model(&models.Video{}).Where("id = ?", videoID).Updates(map[string]interface{}{
		"processing_status": "failed",
		"processing_error":  msg,
	}).Error
	emit(userID, videoID, "failed", 0, "", "", msg, false, "", "")
	return err
}

func emit(userID, videoID uint, status string, progress int, hls, mobile, errMsg string, ready bool, sprite, blur string) {
	if userID == 0 {
		return
	}
	PublishProcessing(userID, ProcessingEvent{
		VideoID:          videoID,
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

func ffmpegPath() string {
	if p := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	return p
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("download: %s", res.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func uploadDirectory(dir, cdnPrefix string) (playlistURL string, err error) {
	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(dir, path)
		rel = strings.ReplaceAll(rel, "\\", "/")
		objectKey := cdnPrefix + "/" + rel
		res := storage.UploadLocalFileObjectKey(path, objectKey, "")
		if res["error"] != "" {
			return fmt.Errorf("%s: %s", rel, res["error"])
		}
		if strings.HasSuffix(path, "playlist.m3u8") {
			playlistURL = res["url"]
		}
		return nil
	})
	return playlistURL, err
}

// activeLadder returns rungs to encode. Fast mode (default) skips 1080p for ~40% faster transcode.
func activeLadder() []struct {
	Width   int
	Height  int
	Bitrate string
} {
	fast := strings.TrimSpace(os.Getenv("VIDEO_LADDER_FAST"))
	if fast == "" || fast == "1" || strings.EqualFold(fast, "true") {
		return portraitLadder[:3] // 360, 540, 720
	}
	return portraitLadder
}

func parseBitrateK(s string) int {
	s = strings.TrimSuffix(strings.TrimSpace(s), "k")
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return 800
	}
	return n
}
