package storage

import (
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

// CDNProvider identifies which media backend handles uploads.
type CDNProvider string

const (
	CDNGoogle       CDNProvider = "google"
	CDNCloudinary   CDNProvider = "cloudinary"
	CDNAWS          CDNProvider = "aws"
	CDNDigitalOcean CDNProvider = "digitalocean"
)

var activeCDN CDNProvider

// ResolveCDNProvider reads MEDIA_CDN (or CDN_PROVIDER) from the environment.
// Supported values: google|gcs|gcp, cloudinary|cld, aws|s3, digitalocean|spaces|do.
// Defaults to google when unset.
func ResolveCDNProvider() CDNProvider {
	raw := strings.TrimSpace(os.Getenv("MEDIA_CDN"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("CDN_PROVIDER"))
	}
	if raw == "" {
		return CDNGoogle
	}
	switch strings.ToLower(raw) {
	case "cloudinary", "cld":
		return CDNCloudinary
	case "aws", "s3", "amazon":
		return CDNAWS
	case "digitalocean", "spaces", "do", "dos", "digital_ocean":
		return CDNDigitalOcean
	case "google", "gcs", "gcp":
		return CDNGoogle
	default:
		fmt.Printf("⚠️  Unknown MEDIA_CDN=%q — defaulting to google\n", raw)
		return CDNGoogle
	}
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// UsesS3CompatibleStorage is true for AWS S3 and DigitalOcean Spaces backends.
func UsesS3CompatibleStorage() bool {
	switch ActiveCDN() {
	case CDNAWS, CDNDigitalOcean:
		return true
	default:
		return false
	}
}

// ActiveCDN returns the configured provider (resolved once per process).
func ActiveCDN() CDNProvider {
	if activeCDN == "" {
		activeCDN = ResolveCDNProvider()
	}
	return activeCDN
}

func decodeBase64Media(base64Src string) ([]byte, error) {
	if strings.TrimSpace(base64Src) == "" {
		return nil, fmt.Errorf("empty base64 payload")
	}
	payload := base64Src
	if i := strings.Index(base64Src, ","); i != -1 {
		payload = base64Src[i+1:]
	}
	return base64.StdEncoding.DecodeString(payload)
}

func mediaObjectKey(publicID, folderHint string) string {
	objName := strings.TrimSpace(publicID)
	if objName == "" {
		objName = fmt.Sprintf("media_%d", time.Now().UnixNano())
	}
	folder := strings.TrimSpace(folderHint)
	if folder != "" {
		objName = path.Join(folder, objName)
	}
	return objName
}

func emptyURL() map[string]string {
	return map[string]string{"url": ""}
}

func uploadError(message string) map[string]string {
	return map[string]string{"url": "", "error": message}
}

func urlResult(u string) map[string]string {
	return map[string]string{"url": u}
}
