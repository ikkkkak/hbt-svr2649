package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func uploadLocalFileS3ExactKey(localPath, objectKey, contentType string) map[string]string {
	if s3Client == nil {
		return uploadError("object storage client not initialized")
	}
	bucket := s3Bucket()
	if bucket == "" {
		return uploadError("object storage bucket is not set")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return uploadError(err.Error())
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return uploadError(err.Error())
	}
	size := info.Size()
	if size <= 0 {
		return uploadError("empty file")
	}

	mime := ResolveContentType(localPath, contentType)
	ctx, cancel := context.WithTimeout(context.Background(), UploadTimeoutForBytes(size))
	defer cancel()

	putInput := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(objectKey),
		Body:          f,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(mime),
		CacheControl:  aws.String(MediaCacheControl(mime)),
	}
	if acl := s3ObjectACL(); acl != "" {
		putInput.ACL = acl
	}

	if _, err = s3Client.PutObject(ctx, putInput); err != nil {
		fmt.Printf("ERROR: S3 stream upload %s (%d bytes): %v\n", objectKey, size, err)
		return uploadError(fmt.Sprintf("upload failed: %v", err))
	}

	urlOut := awsObjectURL(objectKey)
	if urlOut == "" {
		return uploadError("could not build public URL")
	}
	fmt.Printf("✅ Streamed media to object storage: %s (%d bytes)\n", urlOut, size)
	return urlResult(urlOut)
}

// uploadLocalFileS3 streams a file to S3-compatible storage (no base64 — reliable for large videos).
func uploadLocalFileS3(localPath, publicID, contentType, folderHint string) map[string]string {
	if s3Client == nil {
		return uploadError("object storage client not initialized")
	}
	bucket := s3Bucket()
	if bucket == "" {
		return uploadError("object storage bucket is not set")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return uploadError(err.Error())
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return uploadError(err.Error())
	}
	size := info.Size()
	if size <= 0 {
		return uploadError("empty file")
	}

	mime := ResolveContentType(localPath, contentType)
	folder := strings.TrimSpace(folderHint)
	if folder == "" {
		folder = MediaFolderForMIME(mime)
	}
	key := mediaObjectKey(publicID, folder)

	ctx, cancel := context.WithTimeout(context.Background(), UploadTimeoutForBytes(size))
	defer cancel()

	putInput := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(mime),
		CacheControl:  aws.String(MediaCacheControl(mime)),
	}
	if acl := s3ObjectACL(); acl != "" {
		putInput.ACL = acl
	}

	if _, err = s3Client.PutObject(ctx, putInput); err != nil {
		fmt.Printf("ERROR: S3 stream upload %s (%d bytes): %v\n", key, size, err)
		return uploadError(fmt.Sprintf("upload failed: %v", err))
	}

	urlOut := awsObjectURL(key)
	if urlOut == "" {
		return uploadError("could not build public URL")
	}
	fmt.Printf("✅ Streamed media to object storage: %s (%d bytes)\n", urlOut, size)
	return urlResult(urlOut)
}

// uploadLocalFileGCS streams a file to Google Cloud Storage.
func uploadLocalFileGCS(localPath, publicID, contentType, folderHint string) map[string]string {
	bucketName := strings.TrimSpace(os.Getenv("GCS_BUCKET_NAME"))
	if bucketName == "" {
		return uploadError("GCS_BUCKET_NAME is not set")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return uploadError(err.Error())
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return uploadError(err.Error())
	}
	size := info.Size()
	if size <= 0 {
		return uploadError("empty file")
	}

	mime := ResolveContentType(localPath, contentType)
	folder := strings.TrimSpace(folderHint)
	if folder == "" {
		folder = MediaFolderForMIME(mime)
	}
	objName := mediaObjectKey(publicID, folder)

	ctx, cancel := context.WithTimeout(context.Background(), UploadTimeoutForBytes(size))
	defer cancel()

	client, err := gcsClient(ctx)
	if err != nil {
		return uploadError(err.Error())
	}
	defer client.Close()

	w := client.Bucket(bucketName).Object(objName).NewWriter(ctx)
	w.ContentType = mime
	w.CacheControl = MediaCacheControl(mime)
	if _, err = io.Copy(w, f); err != nil {
		_ = w.Close()
		return uploadError(err.Error())
	}
	if err = w.Close(); err != nil {
		return uploadError(err.Error())
	}

	urlOut := gcsObjectURL(objName)
	if urlOut == "" {
		return uploadError("could not build public URL")
	}
	fmt.Printf("✅ Streamed media to GCS: %s (%d bytes)\n", urlOut, size)
	return urlResult(urlOut)
}

func uploadLocalFileGCSExactKey(localPath, objectKey, contentType string) map[string]string {
	bucketName := strings.TrimSpace(os.Getenv("GCS_BUCKET_NAME"))
	if bucketName == "" {
		return uploadError("GCS_BUCKET_NAME is not set")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return uploadError(err.Error())
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return uploadError(err.Error())
	}
	size := info.Size()
	if size <= 0 {
		return uploadError("empty file")
	}

	mime := ResolveContentType(localPath, contentType)
	ctx, cancel := context.WithTimeout(context.Background(), UploadTimeoutForBytes(size))
	defer cancel()

	client, err := gcsClient(ctx)
	if err != nil {
		return uploadError(err.Error())
	}
	defer client.Close()

	w := client.Bucket(bucketName).Object(objectKey).NewWriter(ctx)
	w.ContentType = mime
	w.CacheControl = MediaCacheControl(mime)
	if _, err = io.Copy(w, f); err != nil {
		_ = w.Close()
		return uploadError(err.Error())
	}
	if err = w.Close(); err != nil {
		return uploadError(err.Error())
	}

	urlOut := gcsObjectURL(objectKey)
	if urlOut == "" {
		return uploadError("could not build public URL")
	}
	fmt.Printf("✅ Streamed media to GCS: %s (%d bytes)\n", urlOut, size)
	return urlResult(urlOut)
}
