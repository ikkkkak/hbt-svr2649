package storage

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

func initGCSCDN() {
	bucket := strings.TrimSpace(os.Getenv("GCS_BUCKET_NAME"))
	if bucket == "" {
		fmt.Println("⚠️  GCS_BUCKET_NAME not set — google CDN uploads will fail until configured.")
		return
	}
	fmt.Printf("✅ Google Cloud Storage CDN ready (bucket: %s)\n", bucket)
}

func gcsClient(ctx context.Context) (*storage.Client, error) {
	return storage.NewClient(ctx)
}

func gcsObjectURL(objectName string) string {
	base := strings.TrimSpace(os.Getenv("GCS_PUBLIC_BASE_URL"))
	base = strings.TrimSpace(strings.TrimPrefix(base, "GCS_PUBLIC_BASE_URL="))
	bucket := strings.TrimSpace(os.Getenv("GCS_BUCKET_NAME"))
	if base != "" {
		if strings.HasPrefix(base, "/") && !strings.HasPrefix(base, "//") {
			base = ""
		}
		if base != "" {
			return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(objectName, "/")
		}
	}
	if bucket == "" {
		return ""
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucket, strings.TrimLeft(objectName, "/"))
}

func uploadBase64ToGCS(base64Src, publicID, contentType, folderHint string) map[string]string {
	data, err := decodeBase64Media(base64Src)
	if err != nil {
		fmt.Printf("ERROR: GCS base64 decode failed: %v\n", err)
		return emptyURL()
	}

	bucketName := strings.TrimSpace(os.Getenv("GCS_BUCKET_NAME"))
	folder := strings.TrimSpace(os.Getenv("GCS_FOLDER"))
	if folderHint != "" {
		folder = folderHint
	}
	if bucketName == "" {
		fmt.Println("ERROR: GCS_BUCKET_NAME is not set — cannot upload media.")
		return emptyURL()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := gcsClient(ctx)
	if err != nil {
		fmt.Printf("ERROR: Failed to create GCS client: %v\n", err)
		return emptyURL()
	}
	defer client.Close()

	objName := mediaObjectKey(publicID, "")
	if folder != "" {
		objName = path.Join(folder, objName)
	}

	w := client.Bucket(bucketName).Object(objName).NewWriter(ctx)
	if contentType != "" {
		w.ContentType = contentType
	}
	w.CacheControl = MediaCacheControl(contentType)

	if _, err := w.Write(data); err != nil {
		fmt.Printf("ERROR: Failed to write GCS object %s: %v\n", objName, err)
		_ = w.Close()
		return emptyURL()
	}
	if err := w.Close(); err != nil {
		fmt.Printf("ERROR: Failed to finalize GCS upload %s: %v\n", objName, err)
		return emptyURL()
	}

	urlOut := gcsObjectURL(objName)
	if urlOut == "" {
		fmt.Printf("ERROR: Could not build public URL for GCS object: %s\n", objName)
		return emptyURL()
	}
	fmt.Printf("✅ Uploaded media to GCS: %s\n", urlOut)
	return urlResult(urlOut)
}

func uploadBase64ImageGCS(base64ImageSrc, publicID string) map[string]string {
	return uploadBase64ToGCS(base64ImageSrc, publicID, "image/jpeg", "images")
}

func uploadBase64VideoGCS(base64VideoSrc, publicID, mime string) map[string]string {
	ct := mime
	if strings.TrimSpace(ct) == "" {
		ct = "video/mp4"
	}
	return uploadBase64ToGCS(base64VideoSrc, publicID, ct, "videos")
}

func deleteGCSMedia(mediaURL string) bool {
	if !strings.Contains(mediaURL, "storage.googleapis.com") {
		return false
	}
	bucketName := strings.TrimSpace(os.Getenv("GCS_BUCKET_NAME"))
	if bucketName == "" {
		return false
	}
	objName := gcsObjectNameFromURL(mediaURL, bucketName)
	if objName == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := gcsClient(ctx)
	if err != nil {
		fmt.Printf("ERROR: GCS delete client: %v\n", err)
		return false
	}
	defer client.Close()
	if err := client.Bucket(bucketName).Object(objName).Delete(ctx); err != nil {
		fmt.Printf("WARNING: GCS delete %s: %v\n", objName, err)
		return false
	}
	fmt.Printf("✅ Deleted GCS object: %s\n", objName)
	return true
}

func gcsObjectNameFromURL(mediaURL, bucketName string) string {
	u := strings.TrimSpace(mediaURL)
	prefix := fmt.Sprintf("https://storage.googleapis.com/%s/", bucketName)
	if strings.HasPrefix(u, prefix) {
		return strings.TrimPrefix(u, prefix)
	}
	base := strings.TrimSpace(os.Getenv("GCS_PUBLIC_BASE_URL"))
	base = strings.TrimSpace(strings.TrimPrefix(base, "GCS_PUBLIC_BASE_URL="))
	if base != "" {
		root := strings.TrimRight(base, "/") + "/"
		if strings.HasPrefix(u, root) {
			return strings.TrimPrefix(u, root)
		}
	}
	return ""
}
