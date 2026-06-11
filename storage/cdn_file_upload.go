package storage

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UploadLocalFile uploads a file from disk to the active CDN (GCS / S3 / Cloudinary).
// For user-facing uploads prefer UploadLocalFileOptimized (CRF/WebP pipeline).
func UploadLocalFile(localPath, publicID, contentType string) map[string]string {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	mime := contentType
	if mime == "" {
		ext := strings.ToLower(filepath.Ext(localPath))
		switch ext {
		case ".m3u8":
			mime = "application/vnd.apple.mpegurl"
		case ".ts":
			mime = "video/mp2t"
		case ".mp4":
			mime = "video/mp4"
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		default:
			mime = "application/octet-stream"
		}
	}
	return UploadBytes(publicID, data, mime)
}

// UploadBytes uploads raw bytes to the configured CDN.
func UploadBytes(publicID string, data []byte, contentType string) map[string]string {
	if len(data) == 0 {
		return map[string]string{"error": "empty payload"}
	}
	b64 := fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(data))
	if strings.HasPrefix(contentType, "video/") {
		return UploadBase64Video(b64, publicID, contentType)
	}
	return UploadBase64Image(b64, publicID)
}
