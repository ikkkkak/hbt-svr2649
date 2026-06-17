package storage

import (
	"strings"
)

const chunkUploadBlurSuffix = "/preview_blur.jpg"

// ChunkUploadPreviewBlurURL returns the CDN URL for a blur JPEG generated during chunked upload.
// Video object: videos/chunk_upload_{hex}.mp4 — blur object: images/chunk_upload_{hex}/preview_blur.jpg
func ChunkUploadPreviewBlurURL(videoURL string) string {
	id := extractChunkUploadID(videoURL)
	if id == "" {
		return ""
	}
	key := mediaObjectKey("chunk_upload_"+id+chunkUploadBlurSuffix, "images")
	return awsObjectURL(key)
}

func extractChunkUploadID(videoURL string) string {
	u := strings.TrimSpace(videoURL)
	lower := strings.ToLower(u)
	i := strings.Index(lower, "chunk_upload_")
	if i < 0 {
		return ""
	}
	rest := u[i+len("chunk_upload_"):]
	end := len(rest)
	for j, ch := range rest {
		if ch == '.' || ch == '?' || ch == '/' || ch == '#' {
			end = j
			break
		}
	}
	id := strings.TrimSpace(rest[:end])
	if len(id) < 8 {
		return ""
	}
	return id
}

// ChunkUploadBlurUploadKey is the publicID passed to UploadLocalFile for blur JPEGs.
func ChunkUploadBlurUploadKey(uploadSessionID string) string {
	id := strings.TrimSpace(uploadSessionID)
	if id == "" {
		return ""
	}
	return "chunk_upload_" + id + chunkUploadBlurSuffix
}
