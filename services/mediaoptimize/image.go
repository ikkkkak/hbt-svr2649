package mediaoptimize



import (

	"context"

	"fmt"

	"image"

	_ "image/gif"

	_ "image/jpeg"

	_ "image/png"

	"os"

	"path/filepath"

	"strings"

)



// OptimizeImage resizes and encodes JPEG for CDN (reliable for phone HEIC/JPEG/PNG).

// WebP/libwebp can produce tiny black frames on some inputs — JPEG + validation avoids that.

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



	maxEdge := cfg.ImageMaxEdge

	if maxEdge <= 0 {

		maxEdge = 2048

	}



	scaleVF := fmt.Sprintf(

		"scale=%d:%d:force_original_aspect_ratio=decrease:flags=lanczos",

		maxEdge, maxEdge,

	)



	out := filepath.Join(dir, "optimized.jpg")

	args := []string{

		"-y", "-hide_banner", "-loglevel", "error",

		"-i", inputPath,

		"-map_metadata", "-1",

		"-vf", scaleVF,

		"-pix_fmt", "yuvj420p",

		"-q:v", "4",

		out,

	}



	if err := runFFmpeg(ctx, args...); err != nil {

		_ = os.RemoveAll(dir)

		res.Skipped = true

		res.SkipReason = "jpeg encode failed"

		logCompression(res)

		return res, nil

	}



	opt := fileSize(out)

	if !imageOutputLooksValid(orig, opt) || !jpegFileLooksValid(out) || !jpegImageLooksReal(out) {

		_ = os.RemoveAll(dir)

		res.Skipped = true

		res.SkipReason = "optimized output invalid (too small, corrupt, or blank)"

		logCompression(res)

		return res, nil

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

	res.OutputMIME = "image/jpeg"

	res.OptimizedBytes = opt

	logCompression(res)

	return res, nil

}



// OptimizeThumbnail produces a small JPEG for posters and feed cards.

func OptimizeThumbnail(ctx context.Context, inputPath string, cfg Config) (Result, error) {

	edge := cfg.ImageThumbEdge

	if edge <= 0 {

		edge = 720

	}

	sub := cfg

	sub.ImageMaxEdge = edge

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



// imageOutputLooksValid rejects ffmpeg outputs that are absurdly small vs source (black/corrupt frames).

func imageOutputLooksValid(originalBytes, optimizedBytes int64) bool {

	if optimizedBytes < 1024 {

		return false

	}

	if originalBytes > 500_000 && optimizedBytes < 25_000 {

		return false

	}

	if originalBytes > 200_000 && optimizedBytes < 12_000 {

		return false

	}

	if originalBytes > 50_000 && optimizedBytes < 4_000 {

		return false

	}

	return true

}



func jpegFileLooksValid(path string) bool {

	f, err := os.Open(path)

	if err != nil {

		return false

	}

	defer f.Close()

	hdr := make([]byte, 3)

	if _, err := f.Read(hdr); err != nil {

		return false

	}

	return hdr[0] == 0xFF && hdr[1] == 0xD8 && hdr[2] == 0xFF

}



// jpegImageLooksReal decodes the JPEG and rejects near-black or tiny outputs (common ffmpeg failure mode).

func jpegImageLooksReal(path string) bool {

	f, err := os.Open(path)

	if err != nil {

		return false

	}

	defer f.Close()



	cfg, _, err := image.DecodeConfig(f)

	if err != nil {

		return false

	}

	if cfg.Width < 32 || cfg.Height < 32 {

		return false

	}



	if _, err := f.Seek(0, 0); err != nil {

		return false

	}

	img, _, err := image.Decode(f)

	if err != nil {

		return false

	}



	bounds := img.Bounds()

	w, h := bounds.Dx(), bounds.Dy()

	if w < 32 || h < 32 {

		return false

	}



	stepX := w / 12

	stepY := h / 12

	if stepX < 1 {

		stepX = 1

	}

	if stepY < 1 {

		stepY = 1

	}



	var sum float64

	var n int

	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {

		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {

			r, g, b, _ := img.At(x, y).RGBA()

			lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)

			sum += lum

			n++

		}

	}

	if n == 0 {

		return false

	}

	mean := sum / float64(n)

	// Reject frames that are essentially black (ffmpeg HEIC/color-space failures).

	return mean >= 8.0

}



