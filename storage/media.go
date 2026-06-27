package storage

import (
	"fmt"
	"strings"
)

// InitializeMediaCDN selects the upload backend from MEDIA_CDN / CDN_PROVIDER.
func InitializeMediaCDN() {
	activeCDN = ResolveCDNProvider()
	switch activeCDN {
	case CDNCloudinary:
		initCloudinaryCDN()
	case CDNAWS, CDNDigitalOcean:
		initS3CompatibleCDN()
	default:
		initGCSCDN()
	}
	// Legacy listing images may still live on Cloudinary while new uploads use Spaces/S3.
	ensureCloudinaryClients()
	fmt.Printf("✅ Media CDN provider: %s\n", activeCDN)
}

// InitializeS3 is kept for backward compatibility with main.go.
func InitializeS3() {
	InitializeMediaCDN()
}

// UploadBase64Image uploads a base64 image using the configured CDN.
func UploadBase64Image(base64ImageSrc, publicID string) map[string]string {
	switch ActiveCDN() {
	case CDNCloudinary:
		return uploadBase64ImageCloudinary(base64ImageSrc, publicID)
	case CDNAWS, CDNDigitalOcean:
		return uploadBase64ImageAWS(base64ImageSrc, publicID)
	default:
		return uploadBase64ImageGCS(base64ImageSrc, publicID)
	}
}

// UploadBase64Video uploads a base64 video using the configured CDN.
func UploadBase64Video(base64VideoSrc, publicID, mime string) map[string]string {
	switch ActiveCDN() {
	case CDNCloudinary:
		return uploadBase64VideoCloudinary(base64VideoSrc, publicID, mime)
	case CDNAWS, CDNDigitalOcean:
		return uploadBase64VideoAWS(base64VideoSrc, publicID, mime)
	default:
		return uploadBase64VideoGCS(base64VideoSrc, publicID, mime)
	}
}

// DeleteMedia removes a hosted asset when possible (best-effort by URL host).
func DeleteMedia(mediaURL string) bool {
	u := strings.TrimSpace(mediaURL)
	if u == "" {
		return false
	}
	if strings.Contains(u, "res.cloudinary.com") {
		return deleteCloudinaryMedia(u)
	}
	if strings.Contains(u, "storage.googleapis.com") {
		return deleteGCSMedia(u)
	}
	bucket := s3Bucket()
	if bucket != "" && (strings.Contains(u, ".s3.") || strings.Contains(u, "s3.amazonaws.com") || strings.Contains(u, "digitaloceanspaces.com")) {
		return deleteAWSMedia(u)
	}
	return false
}

// DeleteImageFromCloudinary deletes Cloudinary assets; also routes GCS/S3 when URL matches.
func DeleteImageFromCloudinary(imageURL string) bool {
	return DeleteMedia(imageURL)
}
