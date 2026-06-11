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
		return u
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
			return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/")
		}
		if base := strings.TrimSpace(os.Getenv("AWS_S3_PUBLIC_BASE_URL")); base != "" && !strings.HasPrefix(base, "/") {
			return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/")
		}
	}
	base := strings.TrimSpace(os.Getenv("GCS_PUBLIC_BASE_URL"))
	base = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(base, "GCS_PUBLIC_BASE_URL=")), "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u, "/")
	}
	return ""
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
