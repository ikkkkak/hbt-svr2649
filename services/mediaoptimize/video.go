package mediaoptimize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// OptimizeVideoForUpload is the host-facing path: veryfast preset, optional skip on small files.
func OptimizeVideoForUpload(ctx context.Context, inputPath string) (Result, error) {
	cfg := LoadConfig()
	if cfg.UploadSkipBelowMB > 0 {
		limit := int64(cfg.UploadSkipBelowMB) * 1024 * 1024
		if fileSize(inputPath) > 0 && fileSize(inputPath) < limit {
			r := Result{
				Kind: "video", OriginalBytes: fileSize(inputPath),
				OutputPath: inputPath, OutputMIME: "video/mp4",
				Skipped: true, SkipReason: "already small",
			}
			logCompression(r)
			return r, nil
		}
	}
	cfg.VideoPreset = "veryfast"
	if cfg.VideoFastCRF > 0 {
		cfg.VideoCRF = cfg.VideoFastCRF
	}
	return OptimizeVideo(ctx, inputPath, cfg)
}

// OptimizeVideo normalizes uploads for mobile feed: H.264 CRF, 30fps, metadata stripped, faststart.
// Output is always MP4. If optimized size >= original, returns original path (Skipped).
func OptimizeVideo(ctx context.Context, inputPath string, cfg Config) (Result, error) {
	orig := fileSize(inputPath)
	res := Result{Kind: "video", OriginalBytes: orig, OutputPath: inputPath, OutputMIME: "video/mp4"}

	if !cfg.Enabled || ffmpegBin() == "" {
		res.Skipped = true
		res.SkipReason = "disabled or no ffmpeg"
		return res, nil
	}
	if orig == 0 {
		return res, fmt.Errorf("empty video file")
	}

	dir, err := os.MkdirTemp("", "vopt_")
	if err != nil {
		return res, err
	}
	out := filepath.Join(dir, "optimized.mp4")

	// Fit inside mobile portrait box (e.g. 1080×1920); preserve aspect ratio.
	scaleVF := fmt.Sprintf(
		"fps=%d,scale=%d:%d:force_original_aspect_ratio=decrease",
		cfg.VideoFPS, cfg.VideoMaxWidth, cfg.VideoMaxHeight,
	)
	audioK := cfg.VideoAudioKbps
	if audioK <= 0 {
		audioK = 96
	}

	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", inputPath,
		"-map", "0:v:0?", "-map", "0:a:0?",
		"-map_metadata", "-1",
		"-vf", scaleVF,
		"-c:v", "libx264",
		"-preset", cfg.VideoPreset,
		"-profile:v", "high",
		"-crf", fmt.Sprintf("%d", cfg.VideoCRF),
		"-maxrate", "5M",
		"-bufsize", "10M",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", fmt.Sprintf("%dk", audioK),
		"-ac", "2",
		"-ar", "44100",
		"-movflags", "+faststart",
		"-shortest",
		out,
	}

	if err := runFFmpeg(ctx, args...); err != nil {
		_ = os.RemoveAll(dir)
		return res, err
	}

	opt := fileSize(out)
	if opt <= 0 {
		_ = os.RemoveAll(dir)
		return res, fmt.Errorf("optimized video empty")
	}

	// Keep original if compression did not help (rare with CRF on already-compressed sources).
	if opt >= orig {
		_ = os.RemoveAll(dir)
		res.Skipped = true
		res.SkipReason = "no size win"
		res.OptimizedBytes = orig
		logCompression(res)
		return res, nil
	}

	res.OutputPath = out
	res.OptimizedBytes = opt
	res.OutputMIME = "video/mp4"
	// Caller must RemoveAll(dir) via cleanup — store dir in path parent
	logCompression(res)
	return res, nil
}

// TempDirFor removes the temp directory containing an optimized output path.
func TempDirFor(optimizedPath string) string {
	return filepath.Dir(optimizedPath)
}
