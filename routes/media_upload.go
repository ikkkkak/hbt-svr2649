package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services/mediaoptimize"
	"apartments-clone-server/storage"
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// UploadSession tracks progress of a chunked video upload before it becomes a videoId
type UploadSession struct {
	UploadID      string
	UserID        int
	TotalChunks   int64
	ReceivedChunks int64
	TotalSize     int64
	ReceivedSize  int64
	Status        string // "uploading", "assembling", "transcoding", "completed", "failed"
	ErrorMsg      string
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
}

// Global upload session tracking
var (
	uploadSessions = make(map[string]*UploadSession)
	uploadMutex    sync.RWMutex
	hlsWorkerPool  *mediaoptimize.TranscodeWorkerPool
	spacesUploader *mediaoptimize.DOSpacesUploader
)

// InitializeMediaUploadServices sets up HLS transcoding and CDN upload infrastructure
// Call this from main.go after initializing database and S3/DO Spaces credentials
func InitializeMediaUploadServices(
	workerCount int,
	uploader *mediaoptimize.DOSpacesUploader,
) {
	spacesUploader = uploader
	hlsWorkerPool = mediaoptimize.NewTranscodeWorkerPool(workerCount, uploader)

	// Start background cleanup of expired upload sessions
	go cleanupExpiredUploadSessions()
}

// cleanupExpiredUploadSessions removes upload sessions that haven't been updated in 24 hours
func cleanupExpiredUploadSessions() {
	ticker := time.NewTicker(1 * time.Hour) // Run cleanup every hour
	defer ticker.Stop()

	for range ticker.C {
		uploadMutex.Lock()
		now := time.Now()
		for uploadID, session := range uploadSessions {
			// Remove sessions older than 24 hours
			if now.Sub(session.CreatedAt) > 24*time.Hour {
				delete(uploadSessions, uploadID)
				log.Printf("🗑️ cleaned up expired upload session: %s\n", uploadID)
			}
		}
		uploadMutex.Unlock()
	}
}

// UploadVideoInit initiates a chunked video upload
// Returns uploadId used for subsequent chunk uploads
// POST /api/v1/media/upload/video/init
func UploadVideoInit(ctx iris.Context) {
	var req struct {
		TotalChunks int64  `json:"totalChunks"`
		TotalSize   int64  `json:"totalSize"`
		PropertyID  string `json:"propertyId"` // optional, for property sales video
		Mime        string `json:"mime"`       // optional, default video/mp4
	}

	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error": "invalid request body",
		})
		return
	}

	// Validate
	if req.TotalSize <= 0 || req.TotalChunks <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error": "totalSize and totalChunks must be positive",
		})
		return
	}

	const maxVideoSize = 500 * 1024 * 1024 // 500MB max
	if req.TotalSize > maxVideoSize {
		ctx.StopWithJSON(http.StatusRequestEntityTooLarge, iris.Map{
			"error": fmt.Sprintf("video too large (max %d MB)", maxVideoSize/1024/1024),
		})
		return
	}

	uploadId := uuid.New().String()
	userID, _ := strconv.Atoi(ctx.GetHeader("X-User-ID"))

	// Create upload session for progress tracking
	uploadMutex.Lock()
	uploadSessions[uploadId] = &UploadSession{
		UploadID:     uploadId,
		UserID:       userID,
		TotalChunks:  req.TotalChunks,
		TotalSize:    req.TotalSize,
		Status:       "uploading",
		CreatedAt:    time.Now(),
	}
	uploadMutex.Unlock()

	log.Printf("📦 chunk upload init: uploadId=%s, totalChunks=%d, totalSize=%d MB, user=%d\n",
		uploadId, req.TotalChunks, req.TotalSize/1024/1024, userID)

	ctx.JSON(iris.Map{
		"uploadId":   uploadId,
		"chunkSize":  5 * 1024 * 1024, // 5MB chunks recommended
		"expiresAt":  time.Now().Add(24 * time.Hour),
	})
}

// UploadVideoChunk receives a single chunk of a multipart video upload
// PUT /api/v1/media/upload/video/:uploadId/chunk/:chunkIndex
// NOTE: This is a different endpoint from video_upload_chunked.go - this is for HLS transcoding pipeline
func UploadVideoChunkHLS(ctx iris.Context) {
	uploadID := ctx.Params().Get("uploadId")
	chunkIndexStr := ctx.Params().Get("chunkIndex")

	if uploadID == "" || chunkIndexStr == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing uploadId or chunkIndex"})
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid chunkIndex"})
		return
	}

	userID, _ := strconv.Atoi(ctx.GetHeader("X-User-ID"))

	// Read chunk data directly from request body (streaming, not base64)
	// Limit to 100MB per chunk
	limitedReader := io.LimitReader(ctx.Request().Body, 100*1024*1024)
	defer ctx.Request().Body.Close()

	// Create temp directory for this upload session
	uploadDir := filepath.Join(os.TempDir(), "video_uploads", uploadID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to create upload directory"})
		return
	}

	// Write chunk to disk
	chunkPath := filepath.Join(uploadDir, fmt.Sprintf("chunk_%d", chunkIndex))
	f, err := os.Create(chunkPath)
	if err != nil {
		log.Printf("❌ failed to create chunk file for upload %s index %d: %v", uploadID, chunkIndex, err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write chunk"})
		return
	}
	defer f.Close()

	// Use buffered writer for better performance
	bufWriter := bufio.NewWriterSize(f, 256*1024) // 256KB buffer

	// Stream chunk to disk (not in memory)
	written, copyErr := io.Copy(bufWriter, limitedReader)
	if copyErr != nil && copyErr != io.EOF {
		log.Printf("❌ failed to stream chunk for upload %s index %d: %v", uploadID, chunkIndex, copyErr)
		os.Remove(chunkPath)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to stream chunk"})
		return
	}
	if err := bufWriter.Flush(); err != nil {
		log.Printf("❌ failed to flush chunk buffer for upload %s index %d: %v", uploadID, chunkIndex, err)
		os.Remove(chunkPath)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write chunk data"})
		return
	}

	// Verify chunk file integrity
	fileInfo, err := os.Stat(chunkPath)
	if err != nil {
		log.Printf("❌ failed to stat chunk file for upload %s index %d: %v", uploadID, chunkIndex, err)
		os.Remove(chunkPath)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify chunk file"})
		return
	}
	if fileInfo.Size() != written {
		log.Printf("❌ chunk file size mismatch for upload %s index %d: expected %d, got %d", uploadID, chunkIndex, written, fileInfo.Size())
		os.Remove(chunkPath)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "chunk file corruption detected"})
		return
	}

	log.Printf("📦 chunk %d received: %d MB, uploadId=%s, user=%d\n",
		chunkIndex, written/1024/1024, uploadID, userID)

	// Update upload session progress
	uploadMutex.Lock()
	if session, exists := uploadSessions[uploadID]; exists {
		session.ReceivedChunks++
		session.ReceivedSize += written
	}
	uploadMutex.Unlock()

	ctx.JSON(iris.Map{
		"uploadId":   uploadID,
		"chunkIndex": chunkIndex,
		"bytesWrite": written,
	})
}

// UploadVideoComplete assembles chunks and enqueues for HLS transcoding
// POST /api/v1/media/upload/video/:uploadId/complete
func UploadVideoComplete(ctx iris.Context) {
	uploadID := ctx.Params().Get("uploadId")
	if uploadID == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing uploadId"})
		return
	}

	var req struct {
		TotalChunks  int    `json:"totalChunks"`
		VideoTitle   string `json:"videoTitle"`   // optional
		PropertyID   string `json:"propertyId"`   // optional, for property sale video
		PropertyType string `json:"propertyType"` // "rental" or "sale"
	}

	if err := ctx.ReadJSON(&req); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid request"})
		return
	}

	userID, _ := strconv.Atoi(ctx.GetHeader("X-User-ID"))
	uploadDir := filepath.Join(os.TempDir(), "video_uploads", uploadID)

	// Assemble chunks into single MP4 file
	finalPath := filepath.Join(uploadDir, "video.mp4")
	finalFile, err := os.Create(finalPath)
	if err != nil {
		log.Printf("❌ failed to create final video file for upload %s: %v", uploadID, err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to create final video"})
		return
	}
	defer finalFile.Close()

	// Use buffered writer for better performance during merge
	bufWriter := bufio.NewWriterSize(finalFile, 1024*1024) // 1MB buffer for merge

	var mergedBytes int64
	for i := 0; i < req.TotalChunks; i++ {
		chunkPath := filepath.Join(uploadDir, fmt.Sprintf("chunk_%d", i))
		chunk, err := os.Open(chunkPath)
		if err != nil {
			log.Printf("❌ failed to open chunk %d for upload %s: %v", i, uploadID, err)
			bufWriter.Flush()
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
				"error": fmt.Sprintf("missing chunk %d", i),
			})
			return
		}

		// Verify chunk file size before merging
		_, err = chunk.Stat()
		if err != nil {
			log.Printf("❌ failed to stat chunk %d for upload %s: %v", i, uploadID, err)
			chunk.Close()
			bufWriter.Flush()
			ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify chunk file"})
			return
		}

		n, err := io.Copy(bufWriter, chunk)
		chunk.Close()
		if err != nil {
			log.Printf("❌ failed to copy chunk %d for upload %s: %v", i, uploadID, err)
			bufWriter.Flush()
			ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to assemble chunks"})
			return
		}
		mergedBytes += n
		os.Remove(chunkPath) // clean up individual chunks
	}

	if err := bufWriter.Flush(); err != nil {
		log.Printf("❌ failed to flush merged file buffer for upload %s: %v", uploadID, err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write merged file"})
		return
	}
	finalFile.Close()

	// Verify merged file integrity
	mergedInfo, err := os.Stat(finalPath)
	if err != nil {
		log.Printf("❌ failed to stat merged file for upload %s: %v", uploadID, err)
		os.RemoveAll(uploadDir)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify merged file"})
		return
	}
	if mergedInfo.Size() != mergedBytes {
		log.Printf("❌ merged file size mismatch for upload %s: expected %d, got %d", uploadID, mergedBytes, mergedInfo.Size())
		os.RemoveAll(uploadDir)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "merged file corruption detected"})
		return
	}

	// Verify final file
	fileInfo, err := os.Stat(finalPath)
	if err != nil || fileInfo.Size() == 0 {
		log.Printf("❌ final video verification failed for upload %s: %v", uploadID, err)
		os.RemoveAll(uploadDir)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "final video empty or invalid"})
		return
	}

	log.Printf("✅ chunk upload COMPLETE: assembled %d MB, uploadId=%s, user=%d\n",
		fileInfo.Size()/1024/1024, uploadID, userID)

	// Create video record in DB (status = "processing")
	db := storage.DB
	if db == nil {
		log.Printf("❌ Database not initialized for upload %s", uploadID)
		os.RemoveAll(uploadDir)
		ctx.StopWithJSON(http.StatusServiceUnavailable, iris.Map{"error": "database not initialized"})
		return
	}

	videoID := uploadID
	propertySaleID := uint(0)
	if req.PropertyID != "" {
		if pid, err := strconv.ParseUint(req.PropertyID, 10, 32); err == nil {
			propertySaleID = uint(pid)
		}
	}

	video := models.PropertySaleVideo{
		PropertySaleID: propertySaleID,
		UserID:         uint(userID),
		VideoURL:       finalPath, // Temporary path, will be updated after transcoding
		ProcessingStatus: "processing",
	}

	if err := db.Create(&video).Error; err != nil {
		log.Printf("⚠️ Failed to create video record: %v\n", err)
		// Non-fatal; continue with transcoding
	}

	// Enqueue HLS transcoding job
	if hlsWorkerPool == nil {
		log.Printf("❌ HLS worker pool not initialized for upload %s", uploadID)
		os.RemoveAll(uploadDir)
		ctx.StopWithJSON(http.StatusServiceUnavailable, iris.Map{"error": "transcoding service not initialized"})
		return
	}

	job := mediaoptimize.TranscodeJob{
		JobID:     uploadID,
		VideoID:   videoID,
		InputPath: finalPath,
		UserID:    userID,
		CreatedAt: time.Now(),
		OnSuccess: func(hlsURL, thumbnailURL string) {
			// Update video record with HLS URLs
			if err := db.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Updates(map[string]interface{}{
				"processing_status": "ready",
				"hls_url":           hlsURL,
				"thumbnail_url":     thumbnailURL,
			}).Error; err != nil {
				log.Printf("❌ Failed to update video HLS URLs: %v\n", err)
			}
		},
		OnError: func(err error) {
			// Mark as failed
			if dbErr := db.Model(&models.PropertySaleVideo{}).Where("id = ?", videoID).Updates(map[string]interface{}{
				"processing_status": "failed",
				"processing_error":   err.Error(),
			}).Error; dbErr != nil {
				log.Printf("❌ Failed to update video error status: %v\n", dbErr)
			}
		},
	}

	if err := hlsWorkerPool.Submit(job); err != nil {
		log.Printf("❌ failed to enqueue transcoding job for upload %s: %v", uploadID, err)
		os.RemoveAll(uploadDir)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{
			"error": "failed to enqueue transcoding: " + err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"videoId":   videoID,
		"status":    "processing",
		"message":   "Video enqueued for HLS transcoding",
		"statusUrl": fmt.Sprintf("/api/v1/media/status/%s", videoID),
	})
}

// GetMediaStatus returns the current transcoding or upload status
// GET /api/v1/media/status/:videoId (or :uploadId for uploads in progress)
func GetMediaStatus(ctx iris.Context) {
	idParam := ctx.Params().Get("videoId")
	if idParam == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing videoId or uploadId"})
		return
	}

	// Check if this is an active upload session (uploadId format is UUID)
	uploadMutex.RLock()
	if session, exists := uploadSessions[idParam]; exists {
		uploadMutex.RUnlock()
		progress := 0
		if session.TotalSize > 0 {
			progress = int((session.ReceivedSize * 100) / session.TotalSize)
		}
		ctx.JSON(iris.Map{
			"uploadId":      session.UploadID,
			"status":        session.Status,
			"progress":      progress,
			"receivedChunks": session.ReceivedChunks,
			"totalChunks":   session.TotalChunks,
			"receivedSize":  session.ReceivedSize,
			"totalSize":     session.TotalSize,
			"error":         session.ErrorMsg,
			"createdAt":     session.CreatedAt,
		})
		return
	}
	uploadMutex.RUnlock()

	// Check HLS worker pool status (videoId format)
	if hlsWorkerPool == nil {
		ctx.StopWithJSON(http.StatusServiceUnavailable, iris.Map{"error": "transcoding service not initialized"})
		return
	}
	jobStatus := hlsWorkerPool.GetStatus(idParam)
	if jobStatus != nil {
		ctx.JSON(iris.Map{
			"videoId":    idParam,
			"status":     jobStatus.Status,    // pending, transcoding, uploading, completed, failed
			"progress":   jobStatus.Progress,  // 0-100
			"error":      jobStatus.ErrorMsg,
			"createdAt":  jobStatus.CreatedAt,
			"startedAt":  jobStatus.StartedAt,
			"completedAt": jobStatus.CompletedAt,
		})
		return
	}

	// Check database for final URLs
	db := storage.DB
	if db == nil {
		ctx.StopWithJSON(http.StatusServiceUnavailable, iris.Map{"error": "database not initialized"})
		return
	}

	var video models.PropertySaleVideo
	if err := db.Where("id = ?", idParam).First(&video).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "video not found"})
		} else {
			ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "database error"})
		}
		return
	}

	ctx.JSON(iris.Map{
		"videoId":          idParam,
		"status":           video.ProcessingStatus,
		"hlsUrl":           video.HlsURL,
		"thumbnailUrl":     video.ThumbnailURL,
		"processingError":  video.ProcessingError,
		"processingProgress": video.ProcessingProgress,
	})
}

// UploadImageBinary handles direct binary image upload (not base64)
// POST /api/v1/media/upload/image/binary
// Content-Type: multipart/form-data or image/jpeg, image/heic, etc.
// NOTE: This is a different endpoint from upload_stream.go - this is for HLS transcoding pipeline
func UploadImageBinaryHLS(ctx iris.Context) {
	userID, _ := strconv.Atoi(ctx.GetHeader("X-User-ID"))
	propertyID := ctx.URLParam("propertyId")

	// Read image from request body (streaming)
	limitedReader := io.LimitReader(ctx.Request().Body, 10*1024*1024) // 10MB max
	defer ctx.Request().Body.Close()

	// Process and resize image (handles HEIC, auto-orientation, EXIF stripping)
	variants, err := mediaoptimize.ProcessAndResizeImage(limitedReader, 0)
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error": "failed to process image: " + err.Error(),
		})
		return
	}

	// Validate all variants
	for _, variant := range variants {
		if err := mediaoptimize.ValidateImageSize(variant.Data); err != nil {
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
				"error": "image validation failed: " + err.Error(),
			})
			return
		}
	}

	// Prepare upload variants
	uploadVariants := make(map[string][]byte)
	for _, variant := range variants {
		uploadVariants[variant.SizeName] = variant.Data
	}

	// Upload to DO Spaces
	if spacesUploader == nil {
		log.Printf("❌ Spaces uploader not initialized for image upload")
		ctx.StopWithJSON(http.StatusServiceUnavailable, iris.Map{"error": "CDN service not initialized"})
		return
	}

	imageID := uuid.New().String()
	if propertyID == "" {
		propertyID = fmt.Sprintf("user_%d", userID)
	}

	urls, err := spacesUploader.UploadImageResized(ctx.Request().Context(), propertyID, imageID, uploadVariants)
	if err != nil {
		log.Printf("❌ failed to upload image to CDN: %v", err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{
			"error": "failed to upload to CDN: " + err.Error(),
		})
		return
	}

	log.Printf("✅ binary image upload user=%d bytes=%d → %s (card: %s)\n",
		userID, len(uploadVariants["original"]), propertyID, urls["card"])

	ctx.JSON(iris.Map{
		"imageId": imageID,
		"urls":    urls, // { "original": URL, "display": URL, "card": URL, "thumb": URL }
	})
}
