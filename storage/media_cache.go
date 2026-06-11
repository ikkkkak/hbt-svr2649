package storage

import "strings"

// MediaCacheControl returns CDN Cache-Control for streaming assets.
// Manifests: short TTL so players pick up new renditions.
// Segments / MP4 / images: long immutable cache.
func MediaCacheControl(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u8") {
		return "public, max-age=5, must-revalidate"
	}
	return "public, max-age=31536000, immutable"
}
