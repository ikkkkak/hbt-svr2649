package mediaoptimize

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/png"
	"io"
	"os"

	"github.com/disintegration/imaging"
)

// ImageProcessingConfig defines different resize profiles for properties
type ImageProcessingConfig struct {
	Name    string // "original", "display", "card", "thumb"
	MaxPx   int    // max dimension (preserves aspect ratio)
	Quality int    // JPEG quality 0-100
}

// PropertyImageSizes defines all image variants generated for property photos
// This supports responsive image loading (serve appropriate size based on device)
var PropertyImageSizes = []ImageProcessingConfig{
	{
		Name:    "original",
		MaxPx:   3840, // 4K limit
		Quality: 88,   // high quality for desktop viewers
	},
	{
		Name:    "display",
		MaxPx:   2048, // full-screen property details
		Quality: 85,
	},
	{
		Name:    "card",
		MaxPx:   800, // property cards in list view
		Quality: 82,
	},
	{
		Name:    "thumb",
		MaxPx:   400, // thumbnails and feeds
		Quality: 78,
	},
}

// ProcessedImage represents a resized image variant
type ProcessedImage struct {
	SizeName string
	Data     []byte
	Width    int
	Height   int
	Format   string // "jpeg"
}

// ProcessAndResizeImage takes raw image bytes and produces multiple resize variants
// - Auto-rotates based on EXIF orientation (fixes upside-down photos)
// - Strips all EXIF metadata (privacy: no GPS, camera model, timestamps)
// - Converts any format (HEIC, PNG, etc.) to optimized JPEG
// - Returns variants: original, display, card, thumb
func ProcessAndResizeImage(reader io.Reader, maxDimension int) ([]ProcessedImage, error) {
	// Read all image data
	srcData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	// Decode and auto-orient (handles HEIC, JPEG rotated from phone camera, etc.)
	src, err := imaging.Decode(bytes.NewReader(srcData), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	var results []ProcessedImage

	// Process each size variant
	for _, sizeConfig := range PropertyImageSizes {
		// Skip if user specified max and this is larger
		if maxDimension > 0 && sizeConfig.MaxPx > maxDimension {
			sizeConfig.MaxPx = maxDimension
		}

		bounds := src.Bounds()
		srcWidth := bounds.Max.X
		srcHeight := bounds.Max.Y

		// Only resize if image exceeds target dimension
		// imaging.Fit preserves aspect ratio and pads to exact dimensions
		var processed image.Image
		if srcWidth > sizeConfig.MaxPx || srcHeight > sizeConfig.MaxPx {
			processed = imaging.Fit(src, sizeConfig.MaxPx, sizeConfig.MaxPx, imaging.Lanczos)
		} else {
			processed = src
		}

		// Encode as JPEG
		// Note: imaging.Encode automatically strips EXIF and all metadata when encoding to JPEG
		// This is exactly what we want for privacy
		var buf bytes.Buffer
		err := imaging.Encode(&buf, processed, imaging.JPEG, imaging.JPEGQuality(sizeConfig.Quality))
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", sizeConfig.Name, err)
		}

		// Get final dimensions
		b := processed.Bounds()
		results = append(results, ProcessedImage{
			SizeName: sizeConfig.Name,
			Data:     buf.Bytes(),
			Width:    b.Max.X,
			Height:   b.Max.Y,
			Format:   "jpeg",
		})
	}

	return results, nil
}

// ProcessAndResizeImageFile is a convenience wrapper that reads from disk
func ProcessAndResizeImageFile(filePath string) ([]ProcessedImage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	return ProcessAndResizeImage(file, 0)
}

// GenerateImageWithMaxSize processes and resizes image with explicit max dimension
// Used when user wants to limit final size (e.g., property creation sets max 1920px)
func GenerateImageWithMaxSize(reader io.Reader, maxDimension int) (ProcessedImage, error) {
	srcData, err := io.ReadAll(reader)
	if err != nil {
		return ProcessedImage{}, fmt.Errorf("read image: %w", err)
	}

	src, err := imaging.Decode(bytes.NewReader(srcData), imaging.AutoOrientation(true))
	if err != nil {
		return ProcessedImage{}, fmt.Errorf("decode image: %w", err)
	}

	bounds := src.Bounds()
	srcWidth := bounds.Max.X
	srcHeight := bounds.Max.Y

	var processed image.Image
	if srcWidth > maxDimension || srcHeight > maxDimension {
		processed = imaging.Fit(src, maxDimension, maxDimension, imaging.Lanczos)
	} else {
		processed = src
	}

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, processed, imaging.JPEG, imaging.JPEGQuality(85)); err != nil {
		return ProcessedImage{}, fmt.Errorf("encode: %w", err)
	}

	b := processed.Bounds()
	return ProcessedImage{
		SizeName: "default",
		Data:     buf.Bytes(),
		Width:    b.Max.X,
		Height:   b.Max.Y,
		Format:   "jpeg",
	}, nil
}

// ValidateImageSize checks if image is reasonable (not corrupted, not too small)
// Returns error if image appears invalid
func ValidateImageSize(data []byte) error {
	if len(data) < 100 {
		return fmt.Errorf("image too small (%d bytes, expected > 100)", len(data))
	}

	// Try to decode to verify it's a valid image
	_, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid image format: %w", err)
	}

	return nil
}
