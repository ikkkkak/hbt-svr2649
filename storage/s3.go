// Package storage media uploads are configured via MEDIA_CDN
// (google | cloudinary | aws | digitalocean). Implementations live in
// gcs_media.go, cloudinary_media.go, and aws_media.go (S3-compatible, incl. DO Spaces).// This file is kept so existing imports and InitializeS3() calls remain valid.

package storage
