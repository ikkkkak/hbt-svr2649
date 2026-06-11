package mediaoptimize

import "strings"

// AlreadyOptimizedAtUpload is true when ingest-time FFmpeg normalize already ran.
func AlreadyOptimizedAtUpload(cdnURL string) bool {
	u := strings.ToLower(cdnURL)
	return strings.Contains(u, "chunk_upload_") ||
		strings.Contains(u, "/videos/") && strings.Contains(u, "/mobile")
}
