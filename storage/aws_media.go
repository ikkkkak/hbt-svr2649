package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var s3Client *s3.Client

func s3AccessKeyID() string {
	return envFirst(
		"DO_SPACES_ACCESS_KEY_ID",
		"DO_SPACES_KEY",
		"AWS_ACCESS_KEY_ID",
	)
}

func s3SecretAccessKey() string {
	return envFirst(
		"DO_SPACES_SECRET_ACCESS_KEY",
		"DO_SPACES_SECRET",
		"AWS_SECRET_ACCESS_KEY",
	)
}

func s3Bucket() string {
	return envFirst(
		"DO_SPACES_BUCKET",
		"AWS_S3_BUCKET",
		"S3_BUCKET_NAME",
	)
}

func s3Region() string {
	region := envFirst(
		"DO_SPACES_REGION",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
	)
	if region != "" {
		return region
	}
	if ActiveCDN() == CDNDigitalOcean {
		return "sfo3"
	}
	return "us-east-1"
}

func s3APIEndpoint() string {
	if ep := envFirst("DO_SPACES_ENDPOINT", "AWS_S3_ENDPOINT", "S3_ENDPOINT"); ep != "" {
		return strings.TrimRight(ep, "/")
	}
	if ActiveCDN() == CDNDigitalOcean {
		return fmt.Sprintf("https://%s.digitaloceanspaces.com", s3Region())
	}
	return ""
}

func s3PublicBaseURL() string {
	if base := envFirst(
		"DO_SPACES_PUBLIC_BASE_URL",
		"DO_SPACES_CDN_URL",
		"DO_SPACES_ORIGIN_URL",
		"AWS_S3_PUBLIC_BASE_URL",
	); base != "" {
		return strings.TrimRight(base, "/")
	}
	bucket := s3Bucket()
	if bucket == "" {
		return ""
	}
	if ActiveCDN() == CDNDigitalOcean {
		return fmt.Sprintf("https://%s.%s.digitaloceanspaces.com", bucket, s3Region())
	}
	return ""
}

func initS3CompatibleCDN() {
	bucket := s3Bucket()
	accessKey := s3AccessKeyID()
	secretKey := s3SecretAccessKey()
	if bucket == "" || accessKey == "" || secretKey == "" {
		label := "AWS S3"
		if ActiveCDN() == CDNDigitalOcean {
			label = "DigitalOcean Spaces"
		}
		fmt.Printf("⚠️  %s: set bucket + access key + secret (see .env.example).\n", label)
		return
	}

	region := s3Region()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		fmt.Printf("⚠️  S3-compatible config failed: %v\n", err)
		return
	}

	clientOpts := []func(*s3.Options){}
	if ep := s3APIEndpoint(); ep != "" {
		endpoint := ep
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = false
		})
	}
	s3Client = s3.NewFromConfig(cfg, clientOpts...)

	switch ActiveCDN() {
	case CDNDigitalOcean:
		fmt.Printf("✅ DigitalOcean Spaces ready (bucket: %s, region: %s, origin: %s)\n",
			bucket, region, s3PublicBaseURL())
	default:
		fmt.Printf("✅ AWS S3 CDN ready (bucket: %s, region: %s)\n", bucket, region)
	}
}

// initAWSCDN is kept for any legacy callers.
func initAWSCDN() {
	initS3CompatibleCDN()
}

func awsBucket() string {
	return s3Bucket()
}

func awsObjectURL(key string) string {
	if base := s3PublicBaseURL(); base != "" {
		return base + "/" + strings.TrimLeft(key, "/")
	}
	bucket := s3Bucket()
	region := s3Region()
	if bucket == "" {
		return ""
	}
	if region == "us-east-1" {
		return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, strings.TrimLeft(key, "/"))
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, region, strings.TrimLeft(key, "/"))
}

func uploadBase64ToAWS(base64Src, publicID, contentType, folderHint string) map[string]string {
	if s3Client == nil {
		fmt.Println("ERROR: S3-compatible client not initialized.")
		return emptyURL()
	}
	data, err := decodeBase64Media(base64Src)
	if err != nil {
		fmt.Printf("ERROR: S3 base64 decode failed: %v\n", err)
		return emptyURL()
	}
	bucket := s3Bucket()
	if bucket == "" {
		fmt.Println("ERROR: object storage bucket is not set.")
		return emptyURL()
	}
	key := mediaObjectKey(publicID, folderHint)
	ctx, cancel := context.WithTimeout(context.Background(), UploadTimeoutForBytes(int64(len(data))))
	defer cancel()
	cc := MediaCacheControl(contentType)
	putInput := &s3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String(cc),
	}
	if acl := s3ObjectACL(); acl != "" {
		putInput.ACL = acl
	}
	_, err = s3Client.PutObject(ctx, putInput)
	if err != nil {
		fmt.Printf("ERROR: S3 upload %s: %v\n", key, err)
		return emptyURL()
	}
	urlOut := awsObjectURL(key)
	if urlOut == "" {
		return emptyURL()
	}
	fmt.Printf("✅ Uploaded media to object storage: %s\n", urlOut)
	return urlResult(urlOut)
}

func s3ObjectACL() types.ObjectCannedACL {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DO_SPACES_OBJECT_ACL")), "private") {
		return ""
	}
	if ActiveCDN() == CDNDigitalOcean {
		return types.ObjectCannedACLPublicRead
	}
	return ""
}

func uploadBase64ImageAWS(base64ImageSrc, publicID string) map[string]string {
	return uploadBase64ToAWS(base64ImageSrc, publicID, "image/jpeg", "images")
}

func uploadBase64VideoAWS(base64VideoSrc, publicID, mime string) map[string]string {
	ct := mime
	if strings.TrimSpace(ct) == "" {
		ct = "video/mp4"
	}
	return uploadBase64ToAWS(base64VideoSrc, publicID, ct, "videos")
}

func deleteAWSMedia(mediaURL string) bool {
	if s3Client == nil {
		return false
	}
	bucket := s3Bucket()
	key := awsObjectKeyFromURL(mediaURL, bucket)
	if key == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		fmt.Printf("WARNING: object storage delete %s: %v\n", key, err)
		return false
	}
	fmt.Printf("✅ Deleted object storage key: %s\n", key)
	return true
}

func awsObjectKeyFromURL(mediaURL, bucket string) string {
	u := strings.TrimSpace(mediaURL)
	if base := s3PublicBaseURL(); base != "" {
		root := strings.TrimRight(base, "/") + "/"
		if strings.HasPrefix(u, root) {
			return strings.TrimPrefix(u, root)
		}
	}
	if base := strings.TrimSpace(os.Getenv("AWS_S3_PUBLIC_BASE_URL")); base != "" {
		root := strings.TrimRight(base, "/") + "/"
		if strings.HasPrefix(u, root) {
			return strings.TrimPrefix(u, root)
		}
	}

	region := s3Region()
	if bucket != "" && region != "" {
		doPrefix := fmt.Sprintf("https://%s.%s.digitaloceanspaces.com/", bucket, region)
		if strings.HasPrefix(u, doPrefix) {
			return strings.TrimPrefix(u, doPrefix)
		}
		cdnPrefix := fmt.Sprintf("https://%s.%s.cdn.digitaloceanspaces.com/", bucket, region)
		if strings.HasPrefix(u, cdnPrefix) {
			return strings.TrimPrefix(u, cdnPrefix)
		}
	}

	prefix := fmt.Sprintf("https://%s.s3.amazonaws.com/", bucket)
	if strings.HasPrefix(u, prefix) {
		return strings.TrimPrefix(u, prefix)
	}
	prefix2 := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/", bucket, region)
	if strings.HasPrefix(u, prefix2) {
		return strings.TrimPrefix(u, prefix2)
	}
	return ""
}
