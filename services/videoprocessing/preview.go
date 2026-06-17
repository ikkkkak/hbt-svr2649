package videoprocessing

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QuickBlurFromLocalFile extracts a blurred first-frame JPEG from a local video and uploads it.
func QuickBlurFromLocalFile(ctx context.Context, localVideoPath, uploadKey string) string {
	ffmpeg := ffmpegPath()
	if ffmpeg == "" || strings.TrimSpace(localVideoPath) == "" {
		return ""
	}
	workDir := filepath.Dir(localVideoPath)
	if workDir == "" {
		var err error
		workDir, err = os.MkdirTemp("", "vblur_")
		if err != nil {
			return ""
		}
		defer os.RemoveAll(workDir)
	}
	return generateBlurPreview(ctx, ffmpeg, localVideoPath, workDir, uploadKey)
}

// QuickBlurFromVideoURL downloads a remote MP4 and uploads a blurred preview (best-effort).
func QuickBlurFromVideoURL(videoURL string, uploadKey string) string {
	if strings.TrimSpace(videoURL) == "" {
		return ""
	}
	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		log.Printf("⚠️ QuickBlurFromVideoURL: ffmpeg not found for %s", uploadKey)
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	workDir, err := os.MkdirTemp("", "vblur_dl_")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(workDir)

	localPath := filepath.Join(workDir, "source.mp4")
	if err := downloadFile(ctx, videoURL, localPath); err != nil {
		log.Printf("⚠️ QuickBlurFromVideoURL download %s: %v", uploadKey, err)
		return ""
	}
	return generateBlurPreview(ctx, ffmpeg, localPath, workDir, uploadKey)
}
