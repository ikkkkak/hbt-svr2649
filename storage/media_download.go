package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DownloadMediaFile saves remote media to destPath.
// For private DigitalOcean Spaces / S3 buckets, uses authenticated GetObject instead of public CDN GET.
func DownloadMediaFile(ctx context.Context, mediaURL, destPath string) error {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return fmt.Errorf("download: empty url")
	}
	if IsLocalMediaReference(mediaURL) {
		return fmt.Errorf("download: local device uri (re-upload image to CDN)")
	}

	if strings.Contains(mediaURL, "res.cloudinary.com") {
		ensureCloudinaryClients()
		if err := downloadHTTP(ctx, mediaURL, destPath); err == nil {
			return nil
		}
		if err := DownloadCloudinaryMedia(ctx, mediaURL, destPath); err == nil {
			return nil
		} else if cloudinaryClientForURL(mediaURL) == nil {
			cloud := cloudinaryCloudNameFromURL(mediaURL)
			return fmt.Errorf("download: cloudinary cloud %q — set CLOUDINARY_LEGACY_ACCOUNTS=cloud:apiKey:apiSecret", cloud)
		} else {
			return fmt.Errorf("download: cloudinary: %w", err)
		}
	}

	key := MediaObjectKeyFromURL(mediaURL)
	if UsesS3CompatibleStorage() && s3Client != nil && key != "" {
		if err := downloadS3Object(ctx, key, destPath); err == nil {
			return nil
		} else if err2 := downloadHTTP(ctx, mediaURL, destPath); err2 == nil {
			return nil
		} else {
			return fmt.Errorf("s3(%s): %v; http: %w", key, err, err2)
		}
	}

	return downloadHTTP(ctx, mediaURL, destPath)
}

func downloadHTTP(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("download: %s", res.Status)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func downloadS3Object(ctx context.Context, key, destPath string) error {
	bucket := s3Bucket()
	if bucket == "" {
		return fmt.Errorf("s3 bucket not configured")
	}
	out, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer out.Body.Close()
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, out.Body); err != nil {
		return err
	}
	return nil
}
