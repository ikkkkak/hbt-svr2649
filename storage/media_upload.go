package storage



import (

	"context"

	"fmt"

	"path/filepath"

	"strings"

	"time"



	"apartments-clone-server/services/mediaoptimize"

)



// UploadLocalFileOptimized compresses then uploads. Broker uploads use the fast path when enabled.

func UploadLocalFileOptimized(localPath, publicID, contentType string) map[string]string {

	mime := ResolveContentType(localPath, contentType)

	if IsVideoMedia(localPath, mime) {

		return uploadLocalVideoFast(localPath, publicID, mime)

	}

	return uploadLocalImageFast(localPath, publicID, mime)

}



func imageNeedsTranscode(localPath, mime string) bool {

	ext := strings.ToLower(filepath.Ext(localPath))

	m := strings.ToLower(strings.TrimSpace(mime))

	switch ext {

	case ".heic", ".heif", ".avif":

		return true

	}

	return strings.Contains(m, "heic") ||

		strings.Contains(m, "heif") ||

		strings.Contains(m, "avif")

}



func uploadLocalImageFast(localPath, publicID, contentType string) map[string]string {

	mime := ResolveContentType(localPath, contentType)

	pid := ensureImagePublicID(publicID, mime)

	cfg := mediaoptimize.LoadConfig()



	if imageNeedsTranscode(localPath, mime) && cfg.Enabled && mediaoptimize.FFmpegAvailable() {

		return uploadLocalImageOptimizedWithTimeout(localPath, pid, contentType, 12*time.Second)

	}

	if cfg.UploadFastPath || !cfg.Enabled {

		return UploadLocalFile(localPath, pid, mime)

	}

	return uploadLocalImageOptimized(localPath, pid, contentType)

}



func uploadLocalImageOptimizedWithTimeout(localPath, publicID, contentType string, timeout time.Duration) map[string]string {

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	defer cancel()



	r, err := mediaoptimize.OptimizeImage(ctx, localPath, mediaoptimize.LoadConfig())

	if err != nil {

		mime := ResolveContentType(localPath, contentType)

		pid := ensureImagePublicID(publicID, mime)

		return UploadLocalFile(localPath, pid, mime)

	}

	defer mediaoptimize.CleanupResult(r, localPath)



	if r.Skipped || r.OutputPath == localPath {

		mime := ResolveContentType(localPath, contentType)

		pid := ensureImagePublicID(publicID, mime)

		return UploadLocalFile(localPath, pid, mime)

	}



	mime := "image/jpeg"

	if r.OutputMIME != "" {

		mime = r.OutputMIME

	}

	pid := ensureImagePublicID(publicID, mime)

	return UploadLocalFile(r.OutputPath, pid, mime)

}



func uploadLocalImageOptimized(localPath, publicID, contentType string) map[string]string {

	return uploadLocalImageOptimizedWithTimeout(localPath, publicID, contentType, 5*time.Minute)

}



func ensureImagePublicID(publicID, mime string) string {

	pid := strings.TrimSpace(publicID)

	if pid == "" {

		pid = fmt.Sprintf("img_%d", time.Now().UnixNano())

	}

	m := strings.ToLower(strings.TrimSpace(mime))

	if strings.Contains(m, "jpeg") || m == "image/jpg" {

		return replaceImageExt(pid, ".jpg")

	}

	if strings.Contains(m, "png") {

		return replaceImageExt(pid, ".png")

	}

	if strings.Contains(m, "webp") {

		return replaceImageExt(pid, ".webp")

	}

	if strings.Contains(m, "gif") {

		return replaceImageExt(pid, ".gif")

	}

	if strings.Contains(pid, ".") {

		return pid

	}

	return pid + ".jpg"

}



func replaceImageExt(path, newExt string) string {

	ext := filepath.Ext(path)

	if ext == "" {

		return path + newExt

	}

	return strings.TrimSuffix(path, ext) + newExt

}



func uploadLocalVideoFast(localPath, publicID, mime string) map[string]string {

	cfg := mediaoptimize.LoadConfig()

	if cfg.UploadFastPath {

		return UploadLocalVideoPreserve(localPath, publicID, mime)

	}



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



// UploadBase64ImageOptimized decodes, optionally compresses (JPEG), uploads original on any doubt.

func UploadBase64ImageOptimized(base64ImageSrc, publicID string) map[string]string {

	path, cleanup, err := mediaoptimize.WriteBase64ToTemp(base64ImageSrc, "")

	if err != nil {

		return map[string]string{"error": err.Error()}

	}

	defer cleanup()

	mime := ResolveContentType(path, "")

	return UploadBase64ImageOptimizedFromFile(path, publicID, mime)

}



// UploadBase64ImageOptimizedFromFile validates and uploads a local image file (multipart/binary path).

func UploadBase64ImageOptimizedFromFile(localPath, publicID, contentType string) map[string]string {

	if err := ValidateImageFile(localPath); err != nil {

		return map[string]string{"error": "invalid image: " + err.Error()}

	}

	pid := strings.TrimSpace(publicID)

	if pid == "" {

		pid = fmt.Sprintf("img_%d.jpg", time.Now().UnixNano())

	}

	return uploadLocalImageFast(localPath, pid, contentType)

}



// UploadBase64VideoOptimized decodes and uploads without re-encoding when fast path is enabled.

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



// UploadLocalVideoPreserve streams a merged upload to CDN without FFmpeg re-encoding (byte-perfect).

func UploadLocalVideoPreserve(localPath, publicID, contentType string) map[string]string {

	mime := ResolveContentType(localPath, contentType)

	if mime == "" {

		mime = "video/mp4"

	}

	pid := strings.TrimSpace(publicID)

	if pid != "" && !strings.HasSuffix(strings.ToLower(pid), ".mp4") {

		pid = pid + ".mp4"

	}

	return UploadLocalFile(localPath, pid, mime)

}


