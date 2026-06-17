package storage

import (
	"os"
	"strings"
)

// NormalizePublicMediaURL converts stored paths to absolute https URLs for API payloads.
func NormalizePublicMediaURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" || strings.HasPrefix(u, "data:") {
		return ""
	}
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return NormalizePlaybackMediaURL(u)
	}
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	if strings.HasPrefix(u, "/habitat-bucket/") {
		return "https://storage.googleapis.com" + u
	}
	if strings.HasPrefix(u, "habitat-bucket/") {
		return "https://storage.googleapis.com/" + u
	}
	switch ActiveCDN() {
	case CDNCloudinary:
		// Relative paths are not used for Cloudinary; only absolute URLs are stored.
	case CDNAWS, CDNDigitalOcean:
		if base := s3PublicBaseURL(); base != "" && !strings.HasPrefix(base, "/") {
			return NormalizePlaybackMediaURL(strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/"))
		}
		if base := strings.TrimSpace(os.Getenv("AWS_S3_PUBLIC_BASE_URL")); base != "" && !strings.HasPrefix(base, "/") {
			return NormalizePlaybackMediaURL(strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/"))
		}
	}
	base := strings.TrimSpace(os.Getenv("GCS_PUBLIC_BASE_URL"))
	base = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(base, "GCS_PUBLIC_BASE_URL=")), "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		return NormalizePlaybackMediaURL(strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/"))
	}
	return ""
}

// NormalizePlaybackMediaURL fixes CDN hostname and legacy HLS manifest paths for clients.
func NormalizePlaybackMediaURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.Contains(u, ".digitaloceanspaces.com") && !strings.Contains(u, ".cdn.digitaloceanspaces.com") {
		u = strings.Replace(u, ".digitaloceanspaces.com", ".cdn.digitaloceanspaces.com", 1)
	}
	path := u
	if i := strings.Index(u, "?"); i >= 0 {
		path = u[:i]
	}
	if strings.Contains(path, "/hls/") && !strings.Contains(path, ".m3u8") {
		if strings.HasSuffix(path, "/master") || strings.HasSuffix(path, "/master/") {
			path = strings.TrimRight(path, "/") + ".m3u8"
			if i := strings.Index(u, "?"); i >= 0 {
				return path + u[i:]
			}
			return path
		}
	}
	if (strings.HasSuffix(path, "/mobile") || strings.HasSuffix(path, "/mobile/")) && !strings.HasSuffix(path, ".mp4") {
		path = strings.TrimRight(path, "/") + ".mp4"
		if i := strings.Index(u, "?"); i >= 0 {
			return path + u[i:]
		}
		return path
	}
	return u
}

// NormalizePublicMediaURLs normalizes a list and drops invalid entries.
func NormalizePublicMediaURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		if u := NormalizePublicMediaURL(raw); u != "" && len(u) <= 2048 {
			out = append(out, u)
		}
	}
	return out
}
