package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrateCloudinaryURLToSpaces downloads a legacy Cloudinary asset and re-uploads to the active Spaces/S3 CDN.
func MigrateCloudinaryURLToSpaces(ctx context.Context, mediaURL string) (string, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if !strings.Contains(mediaURL, "res.cloudinary.com") {
		return mediaURL, nil
	}
	if !UsesS3CompatibleStorage() {
		return mediaURL, nil
	}

	tmpDir, err := os.MkdirTemp("", "cldmig_")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	ext := filepath.Ext(strings.Split(mediaURL, "?")[0])
	if ext == "" || len(ext) > 6 {
		ext = ".jpg"
	}
	tmpPath := filepath.Join(tmpDir, "img"+ext)
	if err := DownloadMediaFile(ctx, mediaURL, tmpPath); err != nil {
		return "", err
	}

	mime := "image/jpeg"
	switch strings.ToLower(ext) {
	case ".png":
		mime = "image/png"
	case ".webp":
		mime = "image/webp"
	case ".gif":
		mime = "image/gif"
	}

	objectKey := fmt.Sprintf("images/migrated_%d%s", time.Now().UnixNano(), ext)
	res := UploadLocalFileObjectKey(tmpPath, objectKey, mime)
	url := strings.TrimSpace(res["url"])
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "upload failed"
		}
		return "", fmt.Errorf("%s", msg)
	}
	return url, nil
}
