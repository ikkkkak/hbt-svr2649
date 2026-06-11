package mediaoptimize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OptimizeImage resizes, strips EXIF, encodes WebP for CDN bandwidth savings.
func OptimizeImage(ctx context.Context, inputPath string, cfg Config) (Result, error) {
	orig := fileSize(inputPath)
	ext := strings.ToLower(filepath.Ext(inputPath))
	res := Result{
		Kind: "image", OriginalBytes: orig, OutputPath: inputPath,
		OutputMIME: mimeFromExt(ext),
	}

	if !cfg.Enabled || ffmpegBin() == "" {
		res.Skipped = true
		res.SkipReason = "disabled or no ffmpeg"
		return res, nil
	}
	if orig == 0 {
		return res, fmt.Errorf("empty image file")
	}

	dir, err := os.MkdirTemp("", "iopt_")
	if err != nil {
		return res, err
	}
	out := filepath.Join(dir, "optimized.webp")

	maxEdge := cfg.ImageMaxEdge
	if maxEdge <= 0 {
		maxEdge = 2048
	}
	q := cfg.ImageWebPQuality
	if q <= 0 {
		q = 82
	}
	if q > 100 {
		q = 100
	}

	scaleVF := fmt.Sprintf(
		"scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease",
		maxEdge, maxEdge,
	)

	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", inputPath,
		"-map_metadata", "-1",
		"-vf", scaleVF,
		"-c:v", "libwebp",
		"-quality", fmt.Sprintf("%d", q),
		"-preset", "picture",
		"-compression_level", "4",
		out,
	}

	if err := runFFmpeg(ctx, args...); err != nil {
		// Fallback: high-quality JPEG if libwebp unavailable
		out = filepath.Join(dir, "optimized.jpg")
		args = []string{
			"-y", "-hide_banner", "-loglevel", "error",
			"-i", inputPath,
			"-map_metadata", "-1",
			"-vf", scaleVF,
			"-q:v", "3",
			out,
		}
		if err2 := runFFmpeg(ctx, args...); err2 != nil {
			_ = os.RemoveAll(dir)
			return res, err
		}
		res.OutputMIME = "image/jpeg"
	} else {
		res.OutputMIME = "image/webp"
	}

	opt := fileSize(out)
	if opt <= 0 {
		_ = os.RemoveAll(dir)
		return res, fmt.Errorf("optimized image empty")
	}
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
	logCompression(res)
	return res, nil
}

// OptimizeThumbnail produces a small WebP/JPEG for posters and feed cards.
func OptimizeThumbnail(ctx context.Context, inputPath string, cfg Config) (Result, error) {
	edge := cfg.ImageThumbEdge
	if edge <= 0 {
		edge = 720
	}
	sub := cfg
	sub.ImageMaxEdge = edge
	sub.ImageWebPQuality = 78
	if sub.ImageWebPQuality > cfg.ImageWebPQuality {
		sub.ImageWebPQuality = cfg.ImageWebPQuality
	}
	return OptimizeImage(ctx, inputPath, sub)
}

func mimeFromExt(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}
