package mediaoptimize

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

var defaultCfg = LoadConfig()

// Enabled reports whether optimization runs before CDN upload.
func Enabled() bool {
	return defaultCfg.Enabled && ffmpegBin() != ""
}

// OptimizeLocalFile runs video or image optimization based on MIME/extension.
// Caller must os.RemoveAll(mediaoptimize.TempDirFor(result.OutputPath)) when result changed paths.
func OptimizeLocalFile(ctx context.Context, inputPath, contentType string) (Result, error) {
	cfg := defaultCfg
	if isVideoMIME(contentType) || isVideoExt(filepath.Ext(inputPath)) {
		return OptimizeVideoForUpload(ctx, inputPath)
	}
	return OptimizeImage(ctx, inputPath, cfg)
}

// WriteBase64ToTemp decodes a data URL or raw base64 payload to a temp file.
func WriteBase64ToTemp(dataURL, hintExt string) (path string, cleanup func(), err error) {
	raw := dataURL
	mime := ""
	if i := strings.Index(dataURL, ","); i >= 0 && strings.HasPrefix(dataURL, "data:") {
		mime = strings.TrimPrefix(dataURL[:i], "data:")
		mime = strings.Split(mime, ";")[0]
		raw = dataURL[i+1:]
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", nil, err
	}
	ext := hintExt
	if ext == "" {
		ext = extFromMIME(mime)
	}
	dir, err := os.MkdirTemp("", "b64in_")
	if err != nil {
		return "", nil, err
	}
	path = filepath.Join(dir, "input"+ext)
	if err := os.WriteFile(path, b, 0644); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func isVideoMIME(m string) bool {
	return strings.HasPrefix(strings.ToLower(m), "video/")
}

func isVideoExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".mov", ".m4v", ".webm", ".mkv":
		return true
	default:
		return false
	}
}

func extFromMIME(mime string) string {
	switch strings.ToLower(mime) {
	case "video/mp4", "video/quicktime":
		return ".mp4"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// CleanupResult removes temp dirs when optimization produced a new file.
func CleanupResult(r Result, inputPath string) {
	if r.Skipped || r.OutputPath == inputPath {
		return
	}
	_ = os.RemoveAll(TempDirFor(r.OutputPath))
}
