package routes

import (
	"apartments-clone-server/services/videoprocessing"
	"apartments-clone-server/storage"
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kataras/iris/v12"
)

const maxStreamVideoBytes = 500 * 1024 * 1024
const maxStreamImageBytes = 25 * 1024 * 1024

// UploadImageBinary accepts multipart field "image" — streams binary to CDN (no base64 JSON).
// FIXED: Proper streaming with buffer management and error handling to prevent corruption.
func UploadImageBinary(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	file, header, err := ctx.FormFile("image")
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing image field (multipart form)"})
		return
	}
	defer file.Close()

	// Get content length for validation
	contentLength := header.Size
	if contentLength <= 0 {
		contentLength = maxStreamImageBytes // Unknown size, allow up to max
	}

	// Validate size before streaming
	if contentLength > maxStreamImageBytes {
		ctx.StopWithJSON(http.StatusRequestEntityTooLarge, iris.Map{
			"error": fmt.Sprintf("image too large (max %dMB)", maxStreamImageBytes/(1024*1024)),
		})
		return
	}
	if contentLength <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "empty image"})
		return
	}

	tmpDir, err := os.MkdirTemp("", "imgup_")
	if err != nil {
		log.Printf("❌ failed to create temp dir for image upload: %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}
	tmpPath := filepath.Join(tmpDir, "upload"+ext)

	out, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("❌ failed to create temp file for image upload: %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	bufWriter := bufio.NewWriterSize(out, 512*1024)

	written, err := io.Copy(bufWriter, io.LimitReader(file, maxStreamImageBytes))
	if err != nil {
		log.Printf("❌ failed to read image stream: %v", err)
		out.Close()
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "failed to read image data"})
		return
	}
	if err := bufWriter.Flush(); err != nil {
		log.Printf("❌ failed to flush image buffer: %v", err)
		out.Close()
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write image data"})
		return
	}
	out.Close()

	if written <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "empty image after upload"})
		return
	}
	if header.Size > 0 && written != header.Size {
		log.Printf("❌ image size mismatch: expected %d, got %d", header.Size, written)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "incomplete image upload"})
		return
	}

	fileInfo, err := os.Stat(tmpPath)
	if err != nil {
		log.Printf("❌ failed to stat uploaded image file: %v", err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify uploaded file"})
		return
	}
	if fileInfo.Size() != written {
		log.Printf("❌ image file size mismatch: expected %d, got %d", written, fileInfo.Size())
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "file corruption detected"})
		return
	}

	if err := storage.ValidateImageFile(tmpPath); err != nil {
		log.Printf("❌ image validation failed user=%d: %v", userID, err)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid image: " + err.Error()})
		return
	}

	b := make([]byte, 4)
	_, _ = rand.Read(b)
	publicID := fmt.Sprintf("img_%s%s", hex.EncodeToString(b), ext)
	mime := storage.ResolveContentType(tmpPath, header.Header.Get("Content-Type"))

	res := storage.UploadBase64ImageOptimizedFromFile(tmpPath, publicID, mime)
	url := res["url"]
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "upload failed"
		}
		log.Printf("❌ image CDN upload failed user=%d: %s", userID, msg)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": msg})
		return
	}

	log.Printf("✅ binary image upload user=%d bytes=%d → %s", userID, written, url)
	ctx.JSON(iris.Map{"url": url, "bytes": written})
}

// UploadVideoStream accepts multipart field "video" — streams to temp, validates MP4, uploads to CDN.
// FIXED: Proper streaming with buffer management and error handling to prevent corruption.
func UploadVideoStream(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}

	file, header, err := ctx.FormFile("video")
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing video field (multipart form)"})
		return
	}
	defer file.Close()

	// Get content length for validation
	contentLength := header.Size
	if contentLength <= 0 {
		contentLength = maxStreamVideoBytes // Unknown size, allow up to max
	}

	// Validate size before streaming
	if contentLength > maxStreamVideoBytes {
		ctx.StopWithJSON(http.StatusRequestEntityTooLarge, iris.Map{
			"error": fmt.Sprintf("video too large (max %dMB)", maxStreamVideoBytes/(1024*1024)),
		})
		return
	}
	if contentLength <= 1024 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "video file too small"})
		return
	}

	tmpDir, err := os.MkdirTemp("", "vstream_")
	if err != nil {
		log.Printf("❌ failed to create temp dir for video upload: %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	merged := filepath.Join(tmpDir, "upload.mp4")
	out, err := os.Create(merged)
	if err != nil {
		log.Printf("❌ failed to create temp file for video upload: %v", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Use buffered writer for better performance
	bufWriter := bufio.NewWriterSize(out, 256*1024) // 256KB buffer

	// Stream with proper limit and tracking
	written, err := io.Copy(bufWriter, io.LimitReader(file, maxStreamVideoBytes))
	if err != nil {
		log.Printf("❌ failed to read video stream: %v", err)
		out.Close()
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "failed to read video data"})
		return
	}
	if err := bufWriter.Flush(); err != nil {
		log.Printf("❌ failed to flush video buffer: %v", err)
		out.Close()
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write video data"})
		return
	}
	out.Close()

	if written <= 1024 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "video file too small after upload"})
		return
	}
	if header.Size > 0 && written != header.Size {
		log.Printf("❌ video size mismatch: expected %d, got %d", header.Size, written)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "incomplete video upload"})
		return
	}

	// Verify file integrity by checking if file exists and has correct size
	fileInfo, err := os.Stat(merged)
	if err != nil {
		log.Printf("❌ failed to stat uploaded video file: %v", err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify uploaded file"})
		return
	}
	if fileInfo.Size() != written {
		log.Printf("❌ video file size mismatch: expected %d, got %d", written, fileInfo.Size())
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "file corruption detected"})
		return
	}

	if err := storage.ValidateMP4Container(merged); err != nil {
		log.Printf("❌ stream video invalid container user=%d: %v", userID, err)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid or corrupted video file: " + err.Error()})
		return
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	uploadID := hex.EncodeToString(b)
	publicID := fmt.Sprintf("chunk_upload_%s.mp4", uploadID)

	mime := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mime == "" || !strings.HasPrefix(strings.ToLower(mime), "video/") {
		mime = "video/mp4"
	}

	// CDN first — blur is best-effort background (FFmpeg was adding 60–90s to every upload).
	res := storage.UploadLocalVideoPreserve(merged, publicID, mime)
	url := res["url"]
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "cdn upload failed"
		}
		log.Printf("❌ video CDN upload failed user=%d: %s", userID, msg)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": msg})
		return
	}

	blurKey := storage.ChunkUploadBlurUploadKey(uploadID)
	videoURL := url
	go func() {
		if blur := videoprocessing.QuickBlurFromVideoURL(videoURL, blurKey); blur != "" {
			log.Printf("✅ stream video blur async id=%s → %s", uploadID, blur)
		}
	}()

	log.Printf("✅ stream video upload user=%d bytes=%d → %s (blur async)", userID, written, url)
	ctx.JSON(iris.Map{
		"success": true,
		"url":     url,
		"bytes":   written,
		"mode":    "stream",
	})
}
