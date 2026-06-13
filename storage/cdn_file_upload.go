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
	mime := ResolveContentType(localPath, contentType)

	switch ActiveCDN() {
	case CDNAWS, CDNDigitalOcean:
		if s3Client != nil {
			return uploadLocalFileS3(localPath, publicID, mime, "")
		}
	case CDNGoogle:
		return uploadLocalFileGCS(localPath, publicID, mime, "")
	}

	// Cloudinary + fallback: legacy in-memory path (small assets only).
	data, err := os.ReadFile(localPath)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	return UploadBytes(publicID, data, mime)
}

// UploadBytes uploads raw bytes to the configured CDN.
func UploadBytes(publicID string, data []byte, contentType string) map[string]string {
	if len(data) == 0 {
		return map[string]string{"error": "empty payload"}
	}
	mime := strings.TrimSpace(contentType)
	if mime == "" {
		mime = "application/octet-stream"
	}

	switch ActiveCDN() {
	case CDNAWS, CDNDigitalOcean:
		if s3Client != nil {
			// Small payloads: write temp file and stream (avoids giant base64 strings).
			dir, err := os.MkdirTemp("", "bytes_up_")
			if err != nil {
				return uploadError(err.Error())
			}
			ext := filepath.Ext(publicID)
			if ext == "" {
				ext = extFromMIMEForUpload(mime)
			}
			tmp := filepath.Join(dir, "payload"+ext)
			if err := os.WriteFile(tmp, data, 0644); err != nil {
				_ = os.RemoveAll(dir)
				return uploadError(err.Error())
			}
			res := uploadLocalFileS3(tmp, publicID, mime, "")
			_ = os.RemoveAll(dir)
			return res
		}
	case CDNGoogle:
		dir, err := os.MkdirTemp("", "bytes_up_")
		if err != nil {
			return uploadError(err.Error())
		}
		ext := filepath.Ext(publicID)
		if ext == "" {
			ext = extFromMIMEForUpload(mime)
		}
		tmp := filepath.Join(dir, "payload"+ext)
		if err := os.WriteFile(tmp, data, 0644); err != nil {
			_ = os.RemoveAll(dir)
			return uploadError(err.Error())
		}
		res := uploadLocalFileGCS(tmp, publicID, mime, "")
		_ = os.RemoveAll(dir)
		return res
	}

	b64 := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	if strings.HasPrefix(mime, "video/") {
		return UploadBase64Video(b64, publicID, mime)
	}
	return UploadBase64Image(b64, publicID)
}

func extFromMIMEForUpload(mime string) string {
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
