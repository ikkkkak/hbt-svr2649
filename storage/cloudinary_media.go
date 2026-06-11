package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

var (
	cloudinaryClient    *cloudinary.Cloudinary
	cloudinaryCloudName string
)

func envTrim(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	return strings.Trim(v, "\"'")
}

func initCloudinaryCDN() {
	if cldURL := envTrim("CLOUDINARY_URL"); cldURL != "" {
		cld, err := cloudinary.NewFromURL(cldURL)
		if err != nil {
			fmt.Printf("⚠️  Cloudinary CLOUDINARY_URL invalid: %v\n", err)
		} else {
			cloudinaryClient = cld
			cloudinaryCloudName = cld.Config.Cloud.CloudName
			fmt.Printf("✅ Cloudinary CDN ready (cloud: %s, from CLOUDINARY_URL)\n", cloudinaryCloudName)
			return
		}
	}

	cloudName := envTrim("CLOUDINARY_CLOUD_NAME")
	apiKey := envTrim("CLOUDINARY_API_KEY")
	apiSecret := envTrim("CLOUDINARY_API_SECRET")
	if cloudName == "" || apiKey == "" || apiSecret == "" {
		fmt.Println("⚠️  Set CLOUDINARY_URL or CLOUDINARY_CLOUD_NAME + CLOUDINARY_API_KEY + CLOUDINARY_API_SECRET")
		return
	}
	cld, err := cloudinary.NewFromParams(cloudName, apiKey, apiSecret)
	if err != nil {
		fmt.Printf("⚠️  Cloudinary init failed: %v\n", err)
		return
	}
	cloudinaryClient = cld
	cloudinaryCloudName = cloudName
	fmt.Printf("✅ Cloudinary CDN ready (cloud: %s)\n", cloudName)
	if preset := envTrim("CLOUDINARY_UPLOAD_PRESET"); preset != "" {
		fmt.Printf("   unsigned preset configured: %s\n", preset)
	}
}

func cloudinaryFolder() string {
	if f := envTrim("CLOUDINARY_FOLDER"); f != "" {
		return f
	}
	return envTrim("CLOUDINARY_UPLOAD_FOLDER")
}

var cloudinaryPublicIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_\-/]+`)

func sanitizeCloudinaryPublicID(publicID string) string {
	pid := strings.TrimSpace(publicID)
	pid = strings.ReplaceAll(pid, "\\", "/")
	pid = cloudinaryPublicIDSanitizer.ReplaceAllString(pid, "_")
	pid = strings.Trim(pid, "/")
	if pid == "" {
		pid = fmt.Sprintf("media_%d", time.Now().UnixNano())
	}
	return pid
}

func cloudinaryDataURI(base64Src, resourceType string) string {
	s := strings.TrimSpace(base64Src)
	if strings.HasPrefix(s, "data:") {
		return s
	}
	if resourceType == "video" {
		return "data:video/mp4;base64," + s
	}
	return "data:image/jpeg;base64," + s
}

func cloudinaryDeliveryURL(resp *uploader.UploadResult) string {
	if resp == nil {
		return ""
	}
	if resp.SecureURL != "" {
		return resp.SecureURL
	}
	if resp.URL != "" {
		return resp.URL
	}
	if resp.PlaybackURL != "" {
		return resp.PlaybackURL
	}
	if len(resp.Eager) > 0 && resp.Eager[0].SecureURL != "" {
		return resp.Eager[0].SecureURL
	}
	if resp.PublicID != "" && cloudinaryCloudName != "" {
		rt := resp.ResourceType
		if rt == "" {
			rt = "image"
		}
		pid := resp.PublicID
		if resp.Format != "" && !strings.HasSuffix(pid, "."+resp.Format) {
			return fmt.Sprintf("https://res.cloudinary.com/%s/%s/upload/%s.%s",
				cloudinaryCloudName, rt, pid, resp.Format)
		}
		return fmt.Sprintf("https://res.cloudinary.com/%s/%s/upload/%s",
			cloudinaryCloudName, rt, pid)
	}
	return ""
}

func cloudinaryAPIError(resp *uploader.UploadResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if resp != nil && strings.TrimSpace(resp.Error.Message) != "" {
		return strings.TrimSpace(resp.Error.Message)
	}
	return "cloudinary upload failed"
}

func cloudinaryUploadParams(publicID, resourceType, folderHint string, unsigned bool) uploader.UploadParams {
	params := uploader.UploadParams{
		Overwrite: api.Bool(true),
	}
	if resourceType == "video" {
		params.ResourceType = "video"
	} else if resourceType == "image" {
		params.ResourceType = "image"
	} else {
		params.ResourceType = "auto"
	}
	if unsigned {
		return params
	}
	pid := sanitizeCloudinaryPublicID(publicID)
	params.PublicID = pid
	folder := cloudinaryFolder()
	if folder == "" {
		folder = folderHint
	}
	if folder != "" {
		params.Folder = folder
	}
	return params
}

func tryCloudinaryUpload(ctx context.Context, payload interface{}, params uploader.UploadParams) (*uploader.UploadResult, error) {
	return cloudinaryClient.Upload.Upload(ctx, payload, params)
}

func uploadBase64Cloudinary(base64Src, publicID, resourceType, folderHint string) map[string]string {
	if cloudinaryClient == nil {
		return uploadError("cloudinary client not initialized — check CLOUDINARY_* env vars")
	}

	dataURI := cloudinaryDataURI(base64Src, resourceType)
	data, decodeErr := decodeBase64Media(base64Src)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var lastErr string
	useUnsignedFirst := strings.EqualFold(envTrim("CLOUDINARY_UPLOAD_MODE"), "unsigned")
	preset := envTrim("CLOUDINARY_UPLOAD_PRESET")

	attemptUpload := func(unsigned bool, file interface{}, params uploader.UploadParams) map[string]string {
		var resp *uploader.UploadResult
		var err error
		if unsigned && preset != "" {
			resp, err = cloudinaryClient.Upload.UnsignedUpload(ctx, file, preset, params)
		} else if unsigned {
			lastErr = "CLOUDINARY_UPLOAD_PRESET is required for unsigned uploads"
			return nil
		} else {
			resp, err = tryCloudinaryUpload(ctx, file, params)
		}
		if u := cloudinaryDeliveryURL(resp); u != "" && strings.TrimSpace(resp.Error.Message) == "" && err == nil {
			fmt.Printf("✅ Uploaded media to Cloudinary: %s\n", u)
			return urlResult(u)
		}
		lastErr = cloudinaryAPIError(resp, err)
		return nil
	}

	signedParams := cloudinaryUploadParams(publicID, resourceType, folderHint, false)
	unsignedParams := cloudinaryUploadParams(publicID, resourceType, folderHint, true)

	if useUnsignedFirst && preset != "" {
		if res := attemptUpload(true, dataURI, unsignedParams); res != nil {
			return res
		}
	} else {
		// Signed upload (API key must have upload/create permission).
		if res := attemptUpload(false, dataURI, signedParams); res != nil {
			return res
		}
		// Retry with raw bytes (some payloads parse better than data URIs).
		if decodeErr == nil && len(data) > 0 {
			if res := attemptUpload(false, bytes.NewReader(data), signedParams); res != nil {
				return res
			}
		}
		// Fallback: unsigned preset (e.g. when API key is read-only).
		if preset != "" {
			if res := attemptUpload(true, dataURI, unsignedParams); res != nil {
				return res
			}
			if decodeErr == nil && len(data) > 0 {
				if res := attemptUpload(true, bytes.NewReader(data), unsignedParams); res != nil {
					return res
				}
			}
		}
	}

	fmt.Printf("ERROR: Cloudinary: %s\n", lastErr)
	return uploadError(lastErr)
}

func uploadBase64ImageCloudinary(base64ImageSrc, publicID string) map[string]string {
	return uploadBase64Cloudinary(base64ImageSrc, publicID, "image", "images")
}

func uploadBase64VideoCloudinary(base64VideoSrc, publicID, mime string) map[string]string {
	_ = mime
	return uploadBase64Cloudinary(base64VideoSrc, publicID, "video", "videos")
}

func deleteCloudinaryMedia(mediaURL string) bool {
	if cloudinaryClient == nil || !strings.Contains(mediaURL, "res.cloudinary.com") {
		return false
	}
	publicID, resourceType := cloudinaryPublicIDFromURL(mediaURL)
	if publicID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := cloudinaryClient.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID:     publicID,
		ResourceType: resourceType,
	})
	if err != nil {
		fmt.Printf("WARNING: Cloudinary delete %s: %v\n", publicID, err)
		return false
	}
	fmt.Printf("✅ Deleted Cloudinary asset: %s\n", publicID)
	return true
}

func cloudinaryPublicIDFromURL(mediaURL string) (publicID, resourceType string) {
	resourceType = "image"
	if strings.Contains(mediaURL, "/video/") {
		resourceType = "video"
	}
	idx := strings.Index(mediaURL, "/upload/")
	if idx < 0 {
		return "", resourceType
	}
	rest := mediaURL[idx+len("/upload/"):]
	if strings.HasPrefix(rest, "v") {
		if j := strings.Index(rest, "/"); j > 0 {
			rest = rest[j+1:]
		}
	}
	if dot := strings.LastIndex(rest, "."); dot > 0 {
		rest = rest[:dot]
	}
	return rest, resourceType
}
