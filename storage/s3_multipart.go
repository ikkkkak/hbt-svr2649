package storage

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	multipartCreateTimeout  = 5 * time.Second
	multipartPresignPerPart = 1500 * time.Millisecond
	multipartPresignMax     = 20 * time.Second
)

// CompletedPartInfo is returned by the client after a direct multipart part upload.
type CompletedPartInfo struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

// CanDirectMultipartUpload is true when object storage supports presigned multipart.
func CanDirectMultipartUpload() bool {
	return UsesS3CompatibleStorage() && s3Client != nil && s3Bucket() != ""
}

// ShouldTryDirectMultipartUpload gates the presigned CDN path (skip when relay-only or S3 unreachable).
func ShouldTryDirectMultipartUpload() bool {
	if !CanDirectMultipartUpload() {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MESKENY_UPLOAD_DIRECT_MULTIPART")))
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv("UPLOAD_RELAY_ONLY"))) == "true" {
		return false
	}
	return true
}

// BeginDirectMultipartUpload starts S3 multipart and returns presigned PUT URLs per part.
// Uses a short timeout + single retry so init fails fast and the API can fall back to relay.
func BeginDirectMultipartUpload(
	publicID, contentType string,
	partCount int,
) (uploadID string, partURLs []string, objectKey string, err error) {
	if !CanDirectMultipartUpload() {
		return "", nil, "", fmt.Errorf("direct multipart not available")
	}
	probe := S3ProbeClient()
	if probe == nil {
		return "", nil, "", fmt.Errorf("direct multipart probe client unavailable")
	}
	if partCount < 1 || partCount > 10000 {
		return "", nil, "", fmt.Errorf("invalid part count")
	}
	bucket := s3Bucket()
	mime := ResolveContentType(publicID, contentType)
	if mime == "" {
		mime = "video/mp4"
	}
	pid := strings.TrimSpace(publicID)
	if pid != "" && !strings.HasSuffix(strings.ToLower(pid), ".mp4") {
		pid = pid + ".mp4"
	}
	objectKey = mediaObjectKey(pid, "videos")

	createCtx, createCancel := context.WithTimeout(context.Background(), multipartCreateTimeout)
	defer createCancel()

	createIn := &s3.CreateMultipartUploadInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(objectKey),
		ContentType:  aws.String(mime),
		CacheControl: aws.String(MediaCacheControl(mime)),
	}
	if acl := s3ObjectACL(); acl != "" {
		createIn.ACL = acl
	}

	created, err := probe.CreateMultipartUpload(createCtx, createIn)
	if err != nil {
		return "", nil, "", fmt.Errorf("create multipart: %w", err)
	}
	if created.UploadId == nil || *created.UploadId == "" {
		return "", nil, "", fmt.Errorf("empty multipart upload id")
	}

	presignBudget := time.Duration(partCount) * multipartPresignPerPart
	if presignBudget > multipartPresignMax {
		presignBudget = multipartPresignMax
	}
	presignCtx, presignCancel := context.WithTimeout(context.Background(), presignBudget)
	defer presignCancel()

	presigner := s3.NewPresignClient(probe)
	partURLs = make([]string, partCount)
	presignTTL := 2 * time.Hour
	for i := 1; i <= partCount; i++ {
		partNum := int32(i)
		presigned, perr := presigner.PresignUploadPart(presignCtx, &s3.UploadPartInput{
			Bucket:     created.Bucket,
			Key:        created.Key,
			UploadId:   created.UploadId,
			PartNumber: aws.Int32(partNum),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = presignTTL
		})
		if perr != nil {
			abortCtx, abortCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = probe.AbortMultipartUpload(abortCtx, &s3.AbortMultipartUploadInput{
				Bucket:   created.Bucket,
				Key:      created.Key,
				UploadId: created.UploadId,
			})
			abortCancel()
			return "", nil, "", fmt.Errorf("presign part %d: %w", i, perr)
		}
		partURLs[i-1] = presigned.URL
	}

	fmt.Printf("✅ Direct multipart started: key=%s parts=%d\n", objectKey, partCount)
	return *created.UploadId, partURLs, objectKey, nil
}

// ListDirectMultipartParts returns parts already uploaded to CDN for resume.
func ListDirectMultipartParts(objectKey, uploadID string) ([]CompletedPartInfo, error) {
	if !CanDirectMultipartUpload() {
		return nil, fmt.Errorf("direct multipart not available")
	}
	if strings.TrimSpace(objectKey) == "" || strings.TrimSpace(uploadID) == "" {
		return nil, fmt.Errorf("missing multipart metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := s3Client.ListParts(ctx, &s3.ListPartsInput{
		Bucket:   aws.String(s3Bucket()),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	parts := make([]CompletedPartInfo, 0, len(out.Parts))
	for _, p := range out.Parts {
		if p.PartNumber == nil || p.ETag == nil {
			continue
		}
		parts = append(parts, CompletedPartInfo{
			PartNumber: int(*p.PartNumber),
			ETag:       strings.TrimSpace(*p.ETag),
		})
	}
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})
	return parts, nil
}

// CompleteDirectMultipartUpload finalizes a client-side multipart upload.
func CompleteDirectMultipartUpload(
	objectKey, uploadID string,
	parts []CompletedPartInfo,
) (string, error) {
	if !CanDirectMultipartUpload() {
		return "", fmt.Errorf("direct multipart not available")
	}
	if strings.TrimSpace(objectKey) == "" || strings.TrimSpace(uploadID) == "" {
		return "", fmt.Errorf("missing multipart metadata")
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("no parts provided")
	}

	completed := make([]types.CompletedPart, 0, len(parts))
	for _, p := range parts {
		if p.PartNumber < 1 || strings.TrimSpace(p.ETag) == "" {
			return "", fmt.Errorf("invalid part metadata at index %d", p.PartNumber)
		}
		completed = append(completed, types.CompletedPart{
			PartNumber: aws.Int32(int32(p.PartNumber)),
			ETag:       aws.String(strings.TrimSpace(p.ETag)),
		})
	}
	sort.Slice(completed, func(i, j int) bool {
		return *completed[i].PartNumber < *completed[j].PartNumber
	})

	bucket := s3Bucket()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	_, err := s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completed,
		},
	})
	if err != nil {
		return "", fmt.Errorf("complete multipart: %w", err)
	}

	urlOut := awsObjectURL(objectKey)
	if urlOut == "" {
		return "", fmt.Errorf("could not build public URL")
	}
	fmt.Printf("✅ Direct multipart complete: %s\n", urlOut)
	return urlOut, nil
}

// AbortDirectMultipartUpload cancels an in-progress multipart upload.
func AbortDirectMultipartUpload(objectKey, uploadID string) {
	if !CanDirectMultipartUpload() || objectKey == "" || uploadID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s3Bucket()),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
	})
}

// VerifyS3ObjectSize confirms an object exists on CDN and meets minimum byte count.
func VerifyS3ObjectSize(objectKey string, minBytes int64) error {
	if s3Client == nil {
		return fmt.Errorf("object storage not configured")
	}
	key := strings.TrimPrefix(strings.TrimSpace(objectKey), "/")
	if key == "" {
		return fmt.Errorf("empty object key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s3Bucket()),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("object not uploaded: %w", err)
	}
	if out.ContentLength == nil || *out.ContentLength < minBytes {
		got := int64(0)
		if out.ContentLength != nil {
			got = *out.ContentLength
		}
		return fmt.Errorf("object too small: got %d, need %d", got, minBytes)
	}
	return nil
}
