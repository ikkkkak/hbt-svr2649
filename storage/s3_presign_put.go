package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const singlePutPresignTTL = 2 * time.Hour

// PresignSinglePutUpload returns a presigned PUT URL and the exact headers the client must send.
func PresignSinglePutUpload(publicID, contentType string) (putURL, objectKey string, putHeaders map[string]string, err error) {
	if !CanDirectMultipartUpload() {
		return "", "", nil, fmt.Errorf("presigned upload not available")
	}
	if s3Client == nil {
		return "", "", nil, fmt.Errorf("s3 client unavailable")
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	presigner := s3.NewPresignClient(s3Client)
	putIn := &s3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(objectKey),
		ContentType:  aws.String(mime),
		CacheControl: aws.String(MediaCacheControl(mime)),
	}
	if acl := s3ObjectACL(); acl != "" {
		putIn.ACL = acl
	}
	presigned, err := presigner.PresignPutObject(ctx, putIn, func(opts *s3.PresignOptions) {
		opts.Expires = singlePutPresignTTL
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("presign put: %w", err)
	}

	putHeaders = signedPutHeaders(presigned.SignedHeader, mime)
	fmt.Printf("✅ Presigned single PUT: key=%s headers=%v\n", objectKey, headerKeys(putHeaders))
	return presigned.URL, objectKey, putHeaders, nil
}

func signedPutHeaders(signed http.Header, mime string) map[string]string {
	out := make(map[string]string)
	for k, vals := range signed {
		if len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			out[k] = vals[0]
		}
	}
	// DO Spaces requires every header listed in X-Amz-SignedHeaders.
	if _, ok := out["Content-Type"]; !ok {
		out["Content-Type"] = mime
	}
	if _, ok := out["content-type"]; !ok {
		out["content-type"] = mime
	}
	return out
}

func headerKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// PublicURLForObjectKey builds the CDN URL for an uploaded object key.
func PublicURLForObjectKey(objectKey string) string {
	return awsObjectURL(objectKey)
}
