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

	// Set public_id with folder
	finalPublicID := publicID
	if folder != "" {
		finalPublicID = folder + "/" + publicID
	}
	if finalPublicID != "" {
		form.Add("public_id", finalPublicID)
	}

	// Generate signature for signed upload
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	form.Add("timestamp", timestamp)

	// Create signature string for Cloudinary (must be SHA1)
	signatureString := fmt.Sprintf("public_id=%s&timestamp=%s%s", finalPublicID, timestamp, apiSecret)
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
