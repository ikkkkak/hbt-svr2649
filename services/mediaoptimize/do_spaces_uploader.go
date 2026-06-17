package mediaoptimize

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// DOSpacesUploader handles uploads to DigitalOcean Spaces (S3-compatible)
type DOSpacesUploader struct {
	client      *s3.Client
	bucket      string
	region      string
	cdnBaseURL  string // public CDN URL like https://meskeny-media.sfo3.cdn.digitaloceanspaces.com
	concurrency int    // max concurrent file uploads (default 4)
}

// NewDOSpacesUploader creates a new uploader for DigitalOcean Spaces
// cdnBaseURL should be like: https://bucket-name.region.cdn.digitaloceanspaces.com
func NewDOSpacesUploader(
	accessKey string,
	secretKey string,
	bucket string,
	region string,
	endpoint string,
	cdnBaseURL string,
) (*DOSpacesUploader, error) {
	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("missing DO Spaces credentials")
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		endpoint := strings.TrimRight(endpoint, "/")
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = false
		})
	}

	client := s3.NewFromConfig(cfg, clientOpts...)

	uploader := &DOSpacesUploader{
		client:      client,
		bucket:      bucket,
		region:      region,
		cdnBaseURL:  strings.TrimRight(cdnBaseURL, "/"),
		concurrency: 4,
	}

	log.Printf("✅ DigitalOcean Spaces uploader ready\n  Bucket: %s\n  Region: %s\n  CDN: %s\n",
		bucket, region, cdnBaseURL)

	return uploader, nil
}

// UploadHLSStream uploads all HLS files from localDir to DO Spaces under /videos/videoID/
// Returns the public master.m3u8 CDN URL and thumbnail URL
func (u *DOSpacesUploader) UploadHLSStream(
	ctx context.Context,
	localDir string,
	videoID string,
	userID int,
) (string, string, error) {
	// List all files in localDir (master.m3u8, 360p.m3u8, 720p.m3u8, 1080p.m3u8, .ts segments, thumbnail)
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return "", "", fmt.Errorf("readdir: %w", err)
	}

	if len(entries) == 0 {
		return "", "", fmt.Errorf("no HLS files in output directory")
	}

	// Upload files concurrently
	var wg sync.WaitGroup
	var uploadMutex sync.Mutex
	var uploadedFiles []string
	var uploadErr error
	var masterURL, thumbnailURL string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		wg.Add(1)
		go func(filename string) {
			defer wg.Done()

			localPath := filepath.Join(localDir, filename)
			file, err := os.Open(localPath)
			if err != nil {
				uploadMutex.Lock()
				if uploadErr == nil {
					uploadErr = fmt.Errorf("open %s: %w", filename, err)
				}
				uploadMutex.Unlock()
				return
			}
			defer file.Close()

			// Determine S3 key path and content type
			var s3Key string
			var contentType string

			switch {
			case strings.HasSuffix(filename, ".m3u8"):
				// playlist
				s3Key = fmt.Sprintf("videos/%s/%s", videoID, filename)
				contentType = "application/vnd.apple.mpegurl; charset=utf-8"
			case strings.HasSuffix(filename, ".ts"):
				// video segment
				s3Key = fmt.Sprintf("videos/%s/%s", videoID, filename)
				contentType = "video/mp2t"
			case strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg"):
				// thumbnail
				s3Key = fmt.Sprintf("videos/%s/%s", videoID, filename)
				contentType = "image/jpeg"
			default:
				// skip unknown files
				return
			}

			// Upload to S3
			fileInfo, _ := file.Stat()
			_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      aws.String(u.bucket),
				Key:         aws.String(s3Key),
				Body:        file,
				ContentType: aws.String(contentType),
				// Cache HLS assets (immutable once uploaded, so use long TTL)
				CacheControl: aws.String("public, max-age=2592000, immutable"), // 30 days
				ACL:          types.ObjectCannedACLPublicRead,
			})
			if err != nil {
				uploadMutex.Lock()
				if uploadErr == nil {
					uploadErr = fmt.Errorf("upload %s: %w", filename, err)
				}
				uploadMutex.Unlock()
				return
			}

			// Build public CDN URL
			publicURL := fmt.Sprintf("%s/%s", u.cdnBaseURL, s3Key)

			uploadMutex.Lock()
			uploadedFiles = append(uploadedFiles, filename)
			if filename == "master.m3u8" {
				masterURL = publicURL
			} else if filename == "thumb.jpg" {
				thumbnailURL = publicURL
			}
			uploadMutex.Unlock()

			log.Printf("  ✅ Uploaded: %s (%d bytes) → %s\n", filename, fileInfo.Size(), s3Key)
		}(entry.Name())
	}

	wg.Wait()

	if uploadErr != nil {
		return "", "", uploadErr
	}

	if masterURL == "" {
		return "", "", fmt.Errorf("master.m3u8 was not uploaded")
	}

	log.Printf("☁️  Uploaded %d HLS files to DO Spaces (/videos/%s/)\n", len(uploadedFiles), videoID)
	return masterURL, thumbnailURL, nil
}

// DeleteHLSStream removes all HLS files for a video from DO Spaces
func (u *DOSpacesUploader) DeleteHLSStream(ctx context.Context, videoID string) error {
	// List all objects with prefix /videos/videoID/
	prefix := fmt.Sprintf("videos/%s/", videoID)

	// Use ListObjectsV2 to get all files
	paginator := s3.NewListObjectsV2Paginator(u.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(u.bucket),
		Prefix: aws.String(prefix),
	})

	var toDelete []types.ObjectIdentifier
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				toDelete = append(toDelete, types.ObjectIdentifier{Key: obj.Key})
			}
		}
	}

	if len(toDelete) == 0 {
		return nil // nothing to delete
	}

	// Delete all objects in one batch (max 1000 per API call)
	for i := 0; i < len(toDelete); i += 1000 {
		end := i + 1000
		if end > len(toDelete) {
			end = len(toDelete)
		}

		_, err := u.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(u.bucket),
			Delete: &types.Delete{
				Objects: toDelete[i:end],
			},
		})
		if err != nil {
			log.Printf("⚠️  DeleteHLSStream warning: %v\n", err)
			// Non-fatal; continue trying other batches
		}
	}

	log.Printf("🗑️  Deleted %d HLS files for video %s\n", len(toDelete), videoID)
	return nil
}

// UploadImageResized uploads a pre-processed image to DO Spaces with variants (card, display, thumb)
// Returns map of { "card": URL, "display": URL, "thumb": URL }
func (u *DOSpacesUploader) UploadImageResized(
	ctx context.Context,
	propertyID string,
	imageID string,
	variants map[string][]byte, // "card" -> bytes, "display" -> bytes, etc.
) (map[string]string, error) {
	urls := make(map[string]string)

	for sizeKey, data := range variants {
		s3Key := fmt.Sprintf("images/%s/%s_%s.jpg", propertyID, imageID, sizeKey)

		_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(u.bucket),
			Key:         aws.String(s3Key),
			Body:        strings.NewReader(string(data)),
			ContentType: aws.String("image/jpeg"),
			// Images are immutable, so cache long-term
			CacheControl: aws.String("public, max-age=2592000, immutable"), // 30 days
			ACL:          types.ObjectCannedACLPublicRead,
		})
		if err != nil {
			return nil, fmt.Errorf("upload %s image: %w", sizeKey, err)
		}

		publicURL := fmt.Sprintf("%s/%s", u.cdnBaseURL, s3Key)
		urls[sizeKey] = publicURL
	}

	return urls, nil
}
