package storage

import (
	"context"
	"time"

	"apartments-clone-server/services/mediaoptimize"
)

// UploadLocalFileOptimized compresses then uploads. Videos use the fast upload profile.
func UploadLocalFileOptimized(localPath, publicID, contentType string) map[string]string {
	mime := ResolveContentType(localPath, contentType)
	if IsVideoMedia(localPath, mime) {
		return uploadLocalVideoFast(localPath, publicID, mime)
	}
	return uploadLocalImageOptimized(localPath, publicID, mime)
}

func uploadLocalImageOptimized(localPath, publicID, contentType string) map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	r, err := mediaoptimize.OptimizeImage(ctx, localPath, mediaoptimize.LoadConfig())
	if err != nil {
		return UploadLocalFile(localPath, publicID, contentType)
	}
	defer mediaoptimize.CleanupResult(r, localPath)
	mime := contentType
	if r.OutputMIME != "" {
		mime = r.OutputMIME
	}
	return UploadLocalFile(r.OutputPath, publicID, mime)
}

func uploadLocalVideoFast(localPath, publicID, mime string) map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	r, err := mediaoptimize.OptimizeVideoForUpload(ctx, localPath)
	if err != nil {
		return UploadLocalFile(localPath, publicID, mime)
	}
	defer mediaoptimize.CleanupResult(r, localPath)
	if mime == "" {
		mime = "video/mp4"
	}
	return UploadLocalFile(r.OutputPath, publicID, mime)
}

// UploadBase64ImageOptimized decodes, compresses (WebP), uploads.
func UploadBase64ImageOptimized(base64ImageSrc, publicID string) map[string]string {
	path, cleanup, err := mediaoptimize.WriteBase64ToTemp(base64ImageSrc, "")
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	defer cleanup()
	return uploadLocalImageOptimized(path, publicID, "image/jpeg")
}

// UploadBase64VideoOptimized decodes, compresses (H.264 CRF MP4), uploads.
func UploadBase64VideoOptimized(base64VideoSrc, publicID, mime string) map[string]string {
	path, cleanup, err := mediaoptimize.WriteBase64ToTemp(base64VideoSrc, ".mp4")
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	defer cleanup()
	if mime == "" {
		mime = "video/mp4"
	}
	return uploadLocalVideoFast(path, publicID, mime)
}
