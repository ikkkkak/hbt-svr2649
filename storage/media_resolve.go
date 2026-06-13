package storage

import (
	"path/filepath"
	"strings"
	"time"
)

// ResolveContentType picks a reliable MIME from header + file extension.
func ResolveContentType(localPath, contentType string) string {
	mime := strings.TrimSpace(contentType)
	if mime != "" && !strings.EqualFold(mime, "application/octet-stream") {
		return mime
	}
	ext := strings.ToLower(filepath.Ext(localPath))
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".heic", ".heif":
		return "image/heic"
	default:
		if mime != "" {
			return mime
		}
		return "application/octet-stream"
	}
}

// IsVideoMedia classifies uploads for video vs image pipelines.
func IsVideoMedia(localPath, contentType string) bool {
	mime := strings.ToLower(ResolveContentType(localPath, contentType))
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(localPath))
	switch ext {
	case ".mp4", ".mov", ".m4v", ".webm", ".mkv":
		return true
	default:
		return false
	}
}

// MediaFolderForMIME returns the CDN object prefix (images | videos).
func MediaFolderForMIME(contentType string) string {
	mime := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(mime, "video/") {
		return "videos"
	}
	if strings.HasPrefix(mime, "image/") {
		return "images"
	}
	return ""
}

// UploadTimeoutForBytes scales HTTP/object-store deadlines with payload size.
func UploadTimeoutForBytes(size int64) time.Duration {
	if size <= 0 {
		return 2 * time.Minute
	}
	mb := size / (1024 * 1024)
	timeout := 2*time.Minute + time.Duration(mb)*45*time.Second
	if timeout > 30*time.Minute {
		return 30 * time.Minute
	}
	return timeout
}
