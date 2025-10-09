package storage

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Cloudinary configuration via environment variables
// CLOUDINARY_CLOUD_NAME, CLOUDINARY_API_KEY, CLOUDINARY_API_SECRET, CLOUDINARY_FOLDER (optional)

func InitializeS3() {}

func UploadBase64Image(base64ImageSrc string, publicID string) map[string]string {
	// Check if base64 is valid
	if base64ImageSrc == "" {
		fmt.Printf("ERROR: Empty base64 image\n")
		return map[string]string{"url": ""}
	}

	i := strings.Index(base64ImageSrc, ",")
	payload := base64ImageSrc
	if i != -1 {
		payload = base64ImageSrc[i+1:]
	}

	// Check environment variables
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")
	folder := os.Getenv("CLOUDINARY_FOLDER")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		fmt.Printf("ERROR: Missing Cloudinary env vars - cloudName: %s, apiKey: %s, apiSecret: %s\n",
			cloudName, apiKey, apiSecret)
		return map[string]string{"url": ""}
	}

	fmt.Printf("Cloudinary config - cloudName: %s, apiKey: %s, folder: %s\n", cloudName, apiKey, folder)

	endpoint := "https://api.cloudinary.com/v1_1/" + cloudName + "/image/upload"

	// Build form data for signed upload
	form := url.Values{}
	form.Add("file", "data:image/jpeg;base64,"+payload)
	form.Add("api_key", apiKey)

	// Handle folder/public_id correctly
	// If publicID provided, we can prefix with folder. If not, use Cloudinary 'folder' param instead.
	finalPublicID := ""
	if strings.TrimSpace(publicID) != "" {
		if folder != "" {
			finalPublicID = folder + "/" + publicID
		} else {
			finalPublicID = publicID
		}
		form.Add("public_id", finalPublicID)
	} else if folder != "" {
		form.Add("folder", folder)
	}

	// Generate signature for signed upload
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	form.Add("timestamp", timestamp)

	// Create signature string for Cloudinary (must be SHA1) — include only params we sent, ordered by key
	// Possible keys: folder, public_id, timestamp
	var parts []string
	if form.Has("folder") {
		parts = append(parts, fmt.Sprintf("folder=%s", form.Get("folder")))
	}
	if form.Has("public_id") {
		parts = append(parts, fmt.Sprintf("public_id=%s", form.Get("public_id")))
	}
	parts = append(parts, fmt.Sprintf("timestamp=%s", timestamp))
	signatureString := strings.Join(parts, "&") + apiSecret
	signature := fmt.Sprintf("%x", sha1.Sum([]byte(signatureString)))
	form.Add("signature", signature)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		fmt.Printf("ERROR: Failed to create request: %v\n", err)
		return map[string]string{"url": ""}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("ERROR: HTTP request failed: %v\n", err)
		return map[string]string{"url": ""}
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read response: %v\n", err)
		return map[string]string{"url": ""}
	}

	fmt.Printf("Cloudinary response status: %d\n", res.StatusCode)

	if res.StatusCode != 200 {
		fmt.Printf("ERROR: HTTP request failed with status: %d\n", res.StatusCode)
		fmt.Printf("ERROR: Response body: %s\n", string(body))
		if res.StatusCode == 403 {
			fmt.Printf("ERROR: 403 Forbidden - Check Cloudinary API credentials and permissions\n")
		}
		return map[string]string{"url": ""}
	}

	var cloudRes struct {
		SecureURL string `json:"secure_url"`
		URL       string `json:"url"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	err = json.Unmarshal(body, &cloudRes)
	if err != nil {
		fmt.Printf("ERROR: Failed to parse JSON: %v\n", err)
		return map[string]string{"url": ""}
	}

	if cloudRes.Error.Message != "" {
		fmt.Printf("ERROR: Cloudinary error: %s\n", cloudRes.Error.Message)
		return map[string]string{"url": ""}
	}

	urlOut := cloudRes.SecureURL
	if urlOut == "" {
		urlOut = cloudRes.URL
	}

	if urlOut == "" {
		fmt.Printf("ERROR: No URL returned from Cloudinary\n")
		return map[string]string{"url": ""}
	}

	fmt.Printf("SUCCESS: Uploaded to %s\n", urlOut)
	return map[string]string{"url": urlOut}
}

// UploadBase64Video uploads a base64 video to Cloudinary and returns {"url": secure_url}
func UploadBase64Video(base64VideoSrc string, publicID string, mime string) map[string]string {
	if base64VideoSrc == "" {
		fmt.Printf("ERROR: Empty base64 video\n")
		return map[string]string{"url": ""}
	}
	i := strings.Index(base64VideoSrc, ",")
	payload := base64VideoSrc
	if i != -1 {
		payload = base64VideoSrc[i+1:]
	}
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")
	folder := os.Getenv("CLOUDINARY_FOLDER")
	if cloudName == "" || apiKey == "" || apiSecret == "" {
		fmt.Printf("ERROR: Missing Cloudinary env vars - cloudName: %s, apiKey: %s, apiSecret: %s\n", cloudName, apiKey, apiSecret)
		return map[string]string{"url": ""}
	}
	endpoint := "https://api.cloudinary.com/v1_1/" + cloudName + "/video/upload"
	form := url.Values{}
	m := mime
	if m == "" {
		m = "video/mp4"
	}
	form.Add("file", "data:"+m+";base64,"+payload)
	form.Add("api_key", apiKey)

	// Handle folder/public_id like images: if publicID provided -> prefix with folder; else use folder param
	finalPublicID := ""
	if strings.TrimSpace(publicID) != "" {
		if folder != "" {
			finalPublicID = folder + "/" + publicID
		} else {
			finalPublicID = publicID
		}
		form.Add("public_id", finalPublicID)
	} else if folder != "" {
		form.Add("folder", folder)
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	form.Add("timestamp", timestamp)

	// Build signature only with provided params, ordered by key
	var parts []string
	if form.Has("folder") {
		parts = append(parts, fmt.Sprintf("folder=%s", form.Get("folder")))
	}
	if form.Has("public_id") {
		parts = append(parts, fmt.Sprintf("public_id=%s", form.Get("public_id")))
	}
	parts = append(parts, fmt.Sprintf("timestamp=%s", timestamp))
	signatureString := strings.Join(parts, "&") + apiSecret
	signature := fmt.Sprintf("%x", sha1.Sum([]byte(signatureString)))
	form.Add("signature", signature)
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		fmt.Printf("ERROR: Failed to create request: %v\n", err)
		return map[string]string{"url": ""}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("ERROR: HTTP request failed: %v\n", err)
		return map[string]string{"url": ""}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read response: %v\n", err)
		return map[string]string{"url": ""}
	}
	if res.StatusCode != 200 {
		fmt.Printf("ERROR: HTTP request failed with status: %d\n", res.StatusCode)
		fmt.Printf("ERROR: Response body: %s\n", string(body))
		return map[string]string{"url": ""}
	}
	var cloudRes struct {
		SecureURL string `json:"secure_url"`
		URL       string `json:"url"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &cloudRes); err != nil {
		fmt.Printf("ERROR: Failed to parse JSON: %v\n", err)
		return map[string]string{"url": ""}
	}
	if cloudRes.Error.Message != "" {
		fmt.Printf("ERROR: Cloudinary error: %s\n", cloudRes.Error.Message)
		return map[string]string{"url": ""}
	}
	urlOut := cloudRes.SecureURL
	if urlOut == "" {
		urlOut = cloudRes.URL
	}
	if urlOut == "" {
		fmt.Printf("ERROR: No URL returned from Cloudinary (video)\n")
		return map[string]string{"url": ""}
	}
	fmt.Printf("SUCCESS: Uploaded video to %s\n", urlOut)
	return map[string]string{"url": urlOut}
}

// DeleteImageFromCloudinary deletes an image from Cloudinary using its public ID
func DeleteImageFromCloudinary(imageURL string) bool {
	// Extract public ID from Cloudinary URL
	// URL format: https://res.cloudinary.com/{cloud_name}/image/upload/v{version}/{public_id}.{format}
	if !strings.Contains(imageURL, "res.cloudinary.com") {
		fmt.Printf("ERROR: Not a Cloudinary URL: %s\n", imageURL)
		return false
	}

	// Extract public ID from URL
	parts := strings.Split(imageURL, "/")
	if len(parts) < 2 {
		fmt.Printf("ERROR: Invalid Cloudinary URL format: %s\n", imageURL)
		return false
	}

	// Get the last part and remove file extension
	lastPart := parts[len(parts)-1]
	publicID := strings.Split(lastPart, ".")[0]

	// Get environment variables
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")
	folder := os.Getenv("CLOUDINARY_FOLDER")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		fmt.Printf("ERROR: Missing Cloudinary env vars\n")
		return false
	}

	// Build public ID with folder
	finalPublicID := publicID
	if folder != "" {
		finalPublicID = folder + "/" + publicID
	}

	// Generate signature for deletion
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signatureString := fmt.Sprintf("public_id=%s&timestamp=%s%s", finalPublicID, timestamp, apiSecret)
	signature := fmt.Sprintf("%x", sha1.Sum([]byte(signatureString)))

	// Build form data
	form := url.Values{}
	form.Add("public_id", finalPublicID)
	form.Add("api_key", apiKey)
	form.Add("timestamp", timestamp)
	form.Add("signature", signature)

	// Make deletion request
	endpoint := "https://api.cloudinary.com/v1_1/" + cloudName + "/image/destroy"
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		fmt.Printf("ERROR: Failed to create deletion request: %v\n", err)
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("ERROR: Deletion request failed: %v\n", err)
		return false
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read deletion response: %v\n", err)
		return false
	}

	if res.StatusCode != 200 {
		fmt.Printf("ERROR: Deletion failed with status: %d\n", res.StatusCode)
		fmt.Printf("ERROR: Response body: %s\n", string(body))
		return false
	}

	var deleteRes struct {
		Result string `json:"result"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	err = json.Unmarshal(body, &deleteRes)
	if err != nil {
		fmt.Printf("ERROR: Failed to parse deletion response: %v\n", err)
		return false
	}

	if deleteRes.Error.Message != "" {
		fmt.Printf("ERROR: Cloudinary deletion error: %s\n", deleteRes.Error.Message)
		return false
	}

	if deleteRes.Result != "ok" {
		fmt.Printf("ERROR: Deletion result not ok: %s\n", deleteRes.Result)
		return false
	}

	fmt.Printf("SUCCESS: Deleted image from Cloudinary: %s\n", finalPublicID)
	return true
}
