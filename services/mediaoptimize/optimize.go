package mediaoptimize

import (
	"context"
	"encoding/base64"
	"fmt"
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
	if strings.HasPrefix(dataURL, "data:") {
		if i := strings.Index(dataURL, ","); i >= 0 {
			mime = strings.TrimPrefix(dataURL[:i], "data:")
			mime = strings.Split(mime, ";")[0]
			raw = dataURL[i+1:]
		}
	}
	b, err := decodeBase64Payload(raw)
	if err != nil {
		return "", nil, err
	}
	ext := hintExt
	if ext == "" {
		ext = extFromBytes(b, mime)
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

func decodeBase64Payload(raw string) ([]byte, error) {
	payload := strings.TrimSpace(raw)
	if payload == "" {
		return nil, fmt.Errorf("empty base64 payload")
	}
	payload = strings.ReplaceAll(payload, "\n", "")
	payload = strings.ReplaceAll(payload, "\r", "")
	payload = strings.ReplaceAll(payload, " ", "")
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}
	if b, err := base64.StdEncoding.DecodeString(payload); err == nil {
		if len(b) == 0 {
			return nil, fmt.Errorf("empty decoded payload")
		}
		return b, nil
	}
	b, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 payload")
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("empty decoded payload")
	}
	return b, nil
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
	case "image/heic", "image/heif":
		return ".heic"
	default:
		return ".jpg"
	}
}

func extFromBytes(b []byte, mime string) string {
	if len(b) >= 12 && string(b[4:8]) == "ftyp" {
		brand := string(b[8:12])
		switch brand {
		case "heic", "heif", "mif1", "msf1":
			return ".heic"
		}
	}
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return ".png"
	}
	if len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return ".webp"
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return ".jpg"
	}
	return extFromMIME(mime)
}

// CleanupResult removes temp dirs when optimization produced a new file.
func CleanupResult(r Result, inputPath string) {
	if r.Skipped || r.OutputPath == inputPath {
		return
	}
	_ = os.RemoveAll(TempDirFor(r.OutputPath))
}
