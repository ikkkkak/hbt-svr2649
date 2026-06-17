package mediaoptimize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HLSProfile defines a video bitrate variant for adaptive streaming
type HLSProfile struct {
	Name       string // "360p", "720p", "1080p"
	Width      int
	Height     int
	Bitrate    string // "500k", "1500k", "3000k"
	MaxBitrate string
	BufSize    string
	CRF        int
}

var DefaultHLSProfiles = []HLSProfile{
	{
		Name:       "360p",
		Width:      640,
		Height:     360,
		Bitrate:    "500k",
		MaxBitrate: "600k",
		BufSize:    "1200k",
		CRF:        28,
	},
	{
		Name:       "720p",
		Width:      1280,
		Height:     720,
		Bitrate:    "1500k",
		MaxBitrate: "1800k",
		BufSize:    "3600k",
		CRF:        24,
	},
	{
		Name:       "1080p",
		Width:      1920,
		Height:     1080,
		Bitrate:    "3000k",
		MaxBitrate: "3500k",
		BufSize:    "7000k",
		CRF:        22,
	},
}

type HLSTranscodeResult struct {
	// Output directory containing master.m3u8 and variant playlists
	OutputDir string
	// Relative paths to generated files
	MasterPlaylist string // master.m3u8
	Variants       map[string]string // "360p" -> "360p.m3u8", etc.
	Segments       []string // all .ts files
	// Duration in seconds
	Duration float64
	// Completion time
	CompletedAt time.Time
}

// TranscodeToHLS converts an MP4 video to multi-bitrate HLS.
// Generates master.m3u8 with 360p, 720p, 1080p variants, each with 6-second segments.
// IMPORTANT: Caller must clean up outputDir when done (temp directory).
// Context timeout should be at least 5-10 minutes for typical 10-50MB videos.
func TranscodeToHLS(ctx context.Context, inputPath string, outputDir string) (*HLSTranscodeResult, error) {
	// Verify input exists
	inputInfo, err := os.Stat(inputPath)
	if err != nil || inputInfo.IsDir() {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir output: %w", err)
	}

	// Build complex FFmpeg filter graph for 3 renditions in one pass
	// This is more efficient than 3 separate passes
	filterComplex := buildHLSFilterGraph(inputPath, outputDir)

	args := []string{
		"-y",
		"-hide_banner", "-loglevel", "warning",
		"-i", inputPath,
		"-filter_complex", filterComplex,
	}

	// Add 3 video outputs (360p, 720p, 1080p)
	for i, profile := range DefaultHLSProfiles {
		args = append(args,
			"-map", fmt.Sprintf("[v%d]", i),
			"-c:v:"+fmt.Sprintf("%d", i), "libx264",
			"-preset", "fast",
			"-crf", fmt.Sprintf("%d", profile.CRF),
			"-b:v:"+fmt.Sprintf("%d", i), profile.Bitrate,
			"-maxrate:v:"+fmt.Sprintf("%d", i), profile.MaxBitrate,
			"-bufsize:v:"+fmt.Sprintf("%d", i), profile.BufSize,
		)

		// Set H.264 profile based on resolution
		if i == 0 {
			args = append(args,
				"-profile:v:"+fmt.Sprintf("%d", i), "baseline",
				"-level:v:"+fmt.Sprintf("%d", i), "3.0",
			)
		} else if i == 1 {
			args = append(args,
				"-profile:v:"+fmt.Sprintf("%d", i), "main",
				"-level:v:"+fmt.Sprintf("%d", i), "3.1",
			)
		} else {
			args = append(args,
				"-profile:v:"+fmt.Sprintf("%d", i), "high",
				"-level:v:"+fmt.Sprintf("%d", i), "4.0",
			)
		}
	}

	// Add 3 audio outputs (same audio for all, reduces encoding time)
	for i := 0; i < 3; i++ {
		args = append(args,
			"-map", "0:a:0?",
			"-c:a:"+fmt.Sprintf("%d", i), "aac",
			"-b:a:"+fmt.Sprintf("%d", i), "128k",
			"-ac", "2",
		)
	}

	// Global flags
	args = append(args,
		"-map_metadata", "-1", // Strip all metadata (privacy + smaller file)
		"-f", "hls",
		"-hls_time", "6", // 6-second segments (balance between seeking granularity and player simplicity)
		"-hls_playlist_type", "vod", // VOD = supports seeking across entire playlist
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", filepath.Join(outputDir, "%v_%03d.ts"),
		"-hls_flags", "independent_segments", // Each segment can be decoded independently
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", "v:0,a:0,name:360p v:1,a:1,name:720p v:2,a:2,name:1080p",
		filepath.Join(outputDir, "%v.m3u8"),
	)

	// Run transcoding
	if err := runFFmpeg(ctx, args...); err != nil {
		return nil, fmt.Errorf("hls transcode: %w", err)
	}

	// Verify master.m3u8 was created
	masterPath := filepath.Join(outputDir, "master.m3u8")
	if _, err := os.Stat(masterPath); err != nil {
		return nil, fmt.Errorf("master.m3u8 not created: %w", err)
	}

	result := &HLSTranscodeResult{
		OutputDir:      outputDir,
		MasterPlaylist: "master.m3u8",
		Variants:       make(map[string]string),
		CompletedAt:    time.Now(),
	}

	// Collect variants and segments
	for _, profile := range DefaultHLSProfiles {
		variantPath := filepath.Join(outputDir, profile.Name+".m3u8")
		if _, err := os.Stat(variantPath); err == nil {
			result.Variants[profile.Name] = profile.Name + ".m3u8"
		}
	}

	// List all .ts segment files
	files, err := os.ReadDir(outputDir)
	if err == nil {
		for _, f := range files {
			if filepath.Ext(f.Name()) == ".ts" {
				result.Segments = append(result.Segments, f.Name())
			}
		}
	}

	return result, nil
}

// buildHLSFilterGraph constructs the complex FFmpeg filter chain for 3 renditions
func buildHLSFilterGraph(inputPath string, outputDir string) string {
	// [v:0] = video input
	// Split into 3 streams, scale each to different resolution, label outputs as [v0], [v1], [v2]
	// Each stream maintains aspect ratio and is padded to exact dimensions
	return "[v:0]split=3[vtemp001][vtemp002][vtemp003];" +
		"[vtemp001]scale=640:360:force_original_aspect_ratio=decrease,pad=640:360:(ow-iw)/2:(oh-ih)/2[v0];" +
		"[vtemp002]scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2[v1];" +
		"[vtemp003]scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2[v2]"
}

// ExtractThumbnailFromVideo extracts a single frame as JPEG at 1 second mark
// Resizes to 640x360 for feed cards
func ExtractThumbnailFromVideo(ctx context.Context, videoPath, outputPath string) error {
	args := []string{
		"-y",
		"-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-ss", "00:00:01", // seek to 1 second
		"-vframes", "1", // extract 1 frame only
		"-vf", "scale=640:360:force_original_aspect_ratio=decrease,pad=640:360:(ow-iw)/2:(oh-ih)/2",
		"-q:v", "2", // JPEG quality (1-5, lower=better)
		outputPath,
	}
	return runFFmpeg(ctx, args...)
}

// GeneratePreviewGIF creates a short 3-second looping GIF preview (used in property cards)
// Useful for showing video preview without autoplay
func GeneratePreviewGIF(ctx context.Context, videoPath, outputPath string) error {
	// First pass: generate palette (optimizes colors)
	paletteFilter := "fps=12,scale=320:180:force_original_aspect_ratio=decrease"
	args := []string{
		"-y",
		"-hide_banner", "-loglevel", "error",
		"-t", "3", // 3 seconds
		"-i", videoPath,
		"-vf", paletteFilter + "[s0]split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse",
		"-loop", "0",
		outputPath,
	}
	return runFFmpeg(ctx, args...)
}

// ValidateHLSOutput checks if HLS transcoding produced valid output
func ValidateHLSOutput(outputDir string) error {
	// Check master.m3u8
	masterPath := filepath.Join(outputDir, "master.m3u8")
	if _, err := os.Stat(masterPath); err != nil {
		return fmt.Errorf("master.m3u8 not found: %w", err)
	}

	// Check at least one variant exists
	hasVariant := false
	for _, profile := range DefaultHLSProfiles {
		variantPath := filepath.Join(outputDir, profile.Name+".m3u8")
		if _, err := os.Stat(variantPath); err == nil {
			hasVariant = true
			break
		}
	}
	if !hasVariant {
		return fmt.Errorf("no HLS variants found")
	}

	// Check .ts segments exist
	files, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("cannot read output dir: %w", err)
	}

	segmentCount := 0
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".ts" {
			segmentCount++
		}
	}
	if segmentCount == 0 {
		return fmt.Errorf("no HLS segments (.ts files) found")
	}

	return nil
}
