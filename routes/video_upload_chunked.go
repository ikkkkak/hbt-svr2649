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
	"sync"
	"time"

	"github.com/kataras/iris/v12"
)

const maxChunkBytes = 9 * 1024 * 1024 // 9MB — matches client (base64-safe, still above S3 5MB min)
const singlePutMaxBytes = 30 * 1024 * 1024 // one presigned PUT when source is small enough
const maxChunkUploadBytes = 500 * 1024 * 1024

type chunkSession struct {
	Mode        string // "relay" (via API) or "direct" (presigned → CDN)
	ID          string
	UserID      uint
	Mime        string
	TotalSize   int64
	TotalChunks int
	ChunkSize   int
	S3UploadID  string
	S3Key       string
	Received    map[int]bool
	ChunkSizes  map[int]int
	TempDir     string
	CreatedAt   time.Time
}

var (
	chunkMu   sync.Mutex
	chunkSess = map[string]*chunkSession{}
)

func InitChunkUpload(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	var in struct {
		Filename    string `json:"filename"`
		Mime        string `json:"mime"`
		TotalSize   int64  `json:"totalSize"`
		TotalChunks int    `json:"totalChunks"`
		ChunkSize   int    `json:"chunkSize"`
		PreferRelay bool   `json:"preferRelay"`
	}
	if err := ctx.ReadJSON(&in); err != nil || in.TotalChunks < 1 || in.TotalChunks > 5000 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid upload metadata"})
		return
	}
	if in.TotalSize <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "totalSize required"})
		return
	}
	if in.TotalSize > maxChunkUploadBytes {
		ctx.StopWithJSON(http.StatusRequestEntityTooLarge, iris.Map{
			"error": fmt.Sprintf("video too large (max %dMB)", maxChunkUploadBytes/(1024*1024)),
		})
		return
	}
	mime := strings.TrimSpace(in.Mime)
	if mime == "" {
		mime = "video/mp4"
	}
	lower := strings.ToLower(mime)
	if lower == "video" || lower == "pairedvideo" || lower == "livephoto" {
		mime = "video/mp4"
	}
	if !strings.HasPrefix(lower, "video/") {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "only video uploads are supported via chunked endpoint"})
		return
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	publicID := fmt.Sprintf("chunk_upload_%s.mp4", id)
	initStart := time.Now()

	// Fastest path: one presigned PUT (after on-device compression). Skip when client prefers relay (Expo Go).
	if in.TotalSize <= singlePutMaxBytes && storage.ShouldTryDirectMultipartUpload() && !in.PreferRelay {
		putURL, key, putHeaders, err := storage.PresignSinglePutUpload(publicID, mime)
		if err == nil {
			chunkMu.Lock()
			chunkSess[id] = &chunkSession{
				Mode: "single", ID: id, UserID: userID, Mime: mime,
				TotalSize: in.TotalSize, S3Key: key, CreatedAt: time.Now(),
			}
			chunkMu.Unlock()
			log.Printf("📦 chunk upload init SINGLE user=%d id=%s size=%d took=%v",
				userID, id, in.TotalSize, time.Since(initStart))
			ctx.JSON(iris.Map{
				"success":    true,
				"mode":       "single",
				"uploadId":   id,
				"putUrl":     putURL,
				"putHeaders": putHeaders,
				"totalSize":  in.TotalSize,
			})
			return
		}
		log.Printf("⚠️ single PUT presign unavailable (%v), trying multipart/relay", err)
	}

	chunkSize := in.ChunkSize
	// Expo Go relay: one HTTP chunk carries the whole file (may exceed 9MB part size).
	if in.PreferRelay && in.TotalChunks == 1 {
		chunkSize = int(in.TotalSize)
	} else if chunkSize <= 0 || chunkSize > maxChunkBytes {
		chunkSize = maxChunkBytes
	}
	expectedChunks := int((in.TotalSize + int64(chunkSize) - 1) / int64(chunkSize))
	if in.TotalChunks != expectedChunks {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":          "totalChunks does not match totalSize/chunkSize",
			"expectedChunks": expectedChunks,
			"receivedChunks": in.TotalChunks,
		})
		return
	}

	// Multipart: presigned parts in parallel straight to CDN (skip when client asks for relay, e.g. Expo Go).
	if storage.ShouldTryDirectMultipartUpload() && !in.PreferRelay {
		s3UpID, partURLs, key, err := storage.BeginDirectMultipartUpload(publicID, mime, in.TotalChunks)
		if err == nil {
			chunkMu.Lock()
			chunkSess[id] = &chunkSession{
				Mode: "direct", ID: id, UserID: userID, Mime: mime,
				TotalSize: in.TotalSize, TotalChunks: in.TotalChunks, ChunkSize: chunkSize,
				S3UploadID: s3UpID, S3Key: key, CreatedAt: time.Now(),
			}
			chunkMu.Unlock()
			log.Printf("📦 chunk upload init DIRECT user=%d id=%s parts=%d chunkSize=%d size=%d took=%v",
				userID, id, in.TotalChunks, chunkSize, in.TotalSize, time.Since(initStart))
			ctx.JSON(iris.Map{
				"success":     true,
				"mode":        "direct",
				"uploadId":    id,
				"chunkSize":   chunkSize,
				"totalChunks": in.TotalChunks,
				"partUrls":    partURLs,
			})
			return
		}
		log.Printf("⚠️ direct multipart unavailable (%v), relay fallback", err)
	}

	dir, err := os.MkdirTemp("", "vupload_"+id+"_")
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	chunkMu.Lock()
	chunkSess[id] = &chunkSession{
		Mode: "relay", ID: id, UserID: userID, Mime: mime,
		TotalSize: in.TotalSize, TotalChunks: in.TotalChunks, ChunkSize: chunkSize,
		Received: make(map[int]bool), ChunkSizes: make(map[int]int),
		TempDir: dir, CreatedAt: time.Now(),
	}
	chunkMu.Unlock()
	log.Printf("📦 chunk upload init RELAY user=%d id=%s chunks=%d chunkSize=%d size=%d mime=%s took=%v",
		userID, id, in.TotalChunks, chunkSize, in.TotalSize, mime, time.Since(initStart))
	ctx.JSON(iris.Map{
		"success":     true,
		"mode":        "relay",
		"uploadId":    id,
		"chunkSize":   chunkSize,
		"totalChunks": in.TotalChunks,
	})
}

// GetChunkUploadStatus reports relay progress or lists CDN parts already uploaded (direct resume).
func GetChunkUploadStatus(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	uploadID := strings.TrimSpace(ctx.Params().Get("uploadId"))
	chunkMu.Lock()
	sess, ok := chunkSess[uploadID]
	chunkMu.Unlock()
	if !ok || sess.UserID != userID {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	if sess.Mode == "direct" {
		parts, err := storage.ListDirectMultipartParts(sess.S3Key, sess.S3UploadID)
		if err != nil {
			log.Printf("⚠️ chunk status list parts id=%s: %v", uploadID, err)
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": err.Error()})
			return
		}
		ctx.JSON(iris.Map{
			"success":     true,
			"uploadId":    uploadID,
			"mode":        "direct",
			"totalChunks": sess.TotalChunks,
			"parts":       parts,
			"uploaded":    len(parts),
		})
		return
	}

	if sess.Mode == "single" {
		ctx.JSON(iris.Map{
			"success":  true,
			"uploadId": uploadID,
			"mode":     "single",
			"status":   "awaiting_complete",
			"bytes":    sess.TotalSize,
		})
		return
	}

	received := make([]int, 0, len(sess.Received))
	for i := 0; i < sess.TotalChunks; i++ {
		if sess.Received[i] {
			received = append(received, i)
		}
	}
	ctx.JSON(iris.Map{
		"success":     true,
		"uploadId":    uploadID,
		"mode":        "relay",
		"totalChunks": sess.TotalChunks,
		"received":    received,
		"uploaded":    len(received),
	})
}

func expectedChunkPayloadSize(sess *chunkSession, index int) int {
	if index < 0 || index >= sess.TotalChunks {
		return 0
	}
	if index == sess.TotalChunks-1 {
		remainder := int(sess.TotalSize % int64(sess.ChunkSize))
		if remainder > 0 {
			return remainder
		}
		return sess.ChunkSize
	}
	return sess.ChunkSize
}

func UploadVideoChunk(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	uploadID := strings.TrimSpace(ctx.Params().Get("uploadId"))
	index := ctx.URLParamIntDefault("index", -1)
	if uploadID == "" || index < 0 {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	chunkMu.Lock()
	sess, ok := chunkSess[uploadID]
	chunkMu.Unlock()
	if !ok || sess.UserID != userID {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if sess.Mode == "direct" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "direct upload session — PUT parts to presigned URLs"})
		return
	}
	if index >= sess.TotalChunks {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "chunk index out of range"})
		return
	}

	maxRead := maxChunkBytes
	if sess.TotalChunks == 1 {
		maxRead = int(sess.TotalSize)
		if sess.Mode == "relay" {
			const relaySingleMax = 500 * 1024 * 1024
			if maxRead > relaySingleMax {
				maxRead = relaySingleMax
			}
		} else if maxRead > singlePutMaxBytes {
			maxRead = singlePutMaxBytes
		}
	}

	expected := expectedChunkPayloadSize(sess, index)
	if expected <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid chunk index", "index": index})
		return
	}
	if expected > maxRead {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":    "chunk larger than allowed read size",
			"expected": expected,
			"maxRead":  maxRead,
		})
		return
	}

	path := filepath.Join(sess.TempDir, fmt.Sprintf("part_%05d", index))
	out, err := os.Create(path)
	if err != nil {
		log.Printf("❌ failed to create chunk file for upload %s index %d: %v", uploadID, index, err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Use buffered writer for better performance
	bufWriter := bufio.NewWriterSize(out, 256*1024) // 256KB buffer

	// Stream chunk with proper error handling
	written, copyErr := io.Copy(bufWriter, io.LimitReader(ctx.Request().Body, int64(expected)))
	if copyErr != nil && copyErr != io.EOF {
		log.Printf("❌ failed to read chunk for upload %s index %d: %v", uploadID, index, copyErr)
		out.Close()
		_ = os.Remove(path)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "failed to read chunk"})
		return
	}
	if err := bufWriter.Flush(); err != nil {
		log.Printf("❌ failed to flush chunk buffer for upload %s index %d: %v", uploadID, index, err)
		out.Close()
		_ = os.Remove(path)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write chunk data"})
		return
	}
	out.Close()

	if written != int64(expected) {
		log.Printf("❌ chunk size mismatch for upload %s index %d: expected %d, got %d", uploadID, index, expected, written)
		_ = os.Remove(path)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":    "chunk size mismatch",
			"expected": expected,
			"got":      written,
			"index":    index,
		})
		return
	}

	// Verify file integrity
	fileInfo, err := os.Stat(path)
	if err != nil {
		log.Printf("❌ failed to stat chunk file for upload %s index %d: %v", uploadID, index, err)
		_ = os.Remove(path)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify chunk file"})
		return
	}
	if fileInfo.Size() != written {
		log.Printf("❌ chunk file size mismatch for upload %s index %d: expected %d, got %d", uploadID, index, written, fileInfo.Size())
		_ = os.Remove(path)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "chunk file corruption detected"})
		return
	}

	chunkMu.Lock()
	sess.Received[index] = true
	sess.ChunkSizes[index] = int(written)
	received := len(sess.Received)
	total := sess.TotalChunks
	chunkMu.Unlock()
	log.Printf("📦 chunk relay user=%d id=%s index=%d bytes=%d (%d/%d)", userID, uploadID, index, written, received, total)
	ctx.JSON(iris.Map{"success": true, "index": index, "received": received, "total": total, "bytes": written})
}

func CompleteChunkUpload(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	uploadID := strings.TrimSpace(ctx.Params().Get("uploadId"))
	chunkMu.Lock()
	sess, ok := chunkSess[uploadID]
	chunkMu.Unlock()
	if !ok || sess.UserID != userID {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	if sess.Mode == "single" {
		if err := storage.VerifyS3ObjectSize(sess.S3Key, 1024); err != nil {
			log.Printf("❌ chunk upload SINGLE verify failed user=%d id=%s: %v", userID, uploadID, err)
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "video not uploaded to CDN yet"})
			return
		}
		url := storage.PublicURLForObjectKey(sess.S3Key)
		if url == "" {
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "could not resolve CDN URL"})
			return
		}
		chunkMu.Lock()
		delete(chunkSess, uploadID)
		chunkMu.Unlock()
		go func(videoURL, uploadID string) {
			blur := videoprocessing.QuickBlurFromVideoURL(videoURL, storage.ChunkUploadBlurUploadKey(uploadID))
			if blur != "" {
				log.Printf("✅ chunk upload SINGLE preview blur id=%s → %s", uploadID, blur)
			}
		}(url, uploadID)
		log.Printf("✅ chunk upload SINGLE complete user=%d id=%s bytes=%d → %s", userID, uploadID, sess.TotalSize, url)
		ctx.JSON(iris.Map{
			"success":        true,
			"url":            url,
			"bytes":          sess.TotalSize,
			"mode":           "single",
			"previewBlurUrl": storage.ChunkUploadPreviewBlurURL(url),
		})
		return
	}

	if sess.Mode == "direct" {
		var in struct {
			Parts []storage.CompletedPartInfo `json:"parts"`
		}
		if err := ctx.ReadJSON(&in); err != nil {
			storage.AbortDirectMultipartUpload(sess.S3Key, sess.S3UploadID)
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid complete payload"})
			return
		}
		if len(in.Parts) != sess.TotalChunks {
			storage.AbortDirectMultipartUpload(sess.S3Key, sess.S3UploadID)
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
				"error":    "missing parts",
				"expected": sess.TotalChunks,
				"received": len(in.Parts),
			})
			return
		}
		if err := validateMultipartParts(in.Parts, sess.TotalChunks); err != nil {
			storage.AbortDirectMultipartUpload(sess.S3Key, sess.S3UploadID)
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": err.Error()})
			return
		}
		url, err := storage.CompleteDirectMultipartUpload(sess.S3Key, sess.S3UploadID, in.Parts)
		if err != nil {
			storage.AbortDirectMultipartUpload(sess.S3Key, sess.S3UploadID)
			log.Printf("❌ direct multipart complete failed user=%d id=%s: %v", userID, uploadID, err)
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": err.Error()})
			return
		}
		chunkMu.Lock()
		delete(chunkSess, uploadID)
		chunkMu.Unlock()
		go func(videoURL, uploadID string) {
			blur := videoprocessing.QuickBlurFromVideoURL(videoURL, storage.ChunkUploadBlurUploadKey(uploadID))
			if blur != "" {
				log.Printf("✅ chunk upload DIRECT preview blur id=%s → %s", uploadID, blur)
			}
		}(url, uploadID)
		log.Printf("✅ chunk upload DIRECT complete user=%d id=%s bytes=%d → %s", userID, uploadID, sess.TotalSize, url)
		ctx.JSON(iris.Map{
			"success":        true,
			"url":            url,
			"bytes":          sess.TotalSize,
			"mode":           "direct",
			"previewBlurUrl": storage.ChunkUploadPreviewBlurURL(url),
		})
		return
	}

	if sess.TempDir != "" {
		defer os.RemoveAll(sess.TempDir)
	}

	if len(sess.Received) != sess.TotalChunks {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":    "missing chunks",
			"received": len(sess.Received),
			"expected": sess.TotalChunks,
		})
		return
	}
	for i := 0; i < sess.TotalChunks; i++ {
		if !sess.Received[i] {
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing chunk", "index": i})
			return
		}
	}

	merged := filepath.Join(sess.TempDir, "merged.mp4")
	out, err := os.Create(merged)
	if err != nil {
		log.Printf("❌ failed to create merged file for upload %s: %v", uploadID, err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Use buffered writer for better performance during merge
	bufWriter := bufio.NewWriterSize(out, 1024*1024) // 1MB buffer for merge

	var mergedBytes int64
	for i := 0; i < sess.TotalChunks; i++ {
		part := filepath.Join(sess.TempDir, fmt.Sprintf("part_%05d", i))
		f, err := os.Open(part)
		if err != nil {
			log.Printf("❌ failed to open chunk part %d for upload %s: %v", i, uploadID, err)
			bufWriter.Flush()
			out.Close()
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing chunk file", "index": i})
			return
		}

		// Verify chunk file size before merging
		partInfo, err := f.Stat()
		if err != nil {
			log.Printf("❌ failed to stat chunk part %d for upload %s: %v", i, uploadID, err)
			f.Close()
			bufWriter.Flush()
			out.Close()
			ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify chunk file"})
			return
		}
		expectedSize := int64(sess.ChunkSizes[i])
		if partInfo.Size() != expectedSize {
			log.Printf("❌ chunk part %d size mismatch for upload %s: expected %d, got %d", i, uploadID, expectedSize, partInfo.Size())
			f.Close()
			bufWriter.Flush()
			out.Close()
			ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "chunk file corruption detected during merge"})
			return
		}
		n, err := io.Copy(bufWriter, f)
		f.Close()
		if err != nil {
			log.Printf("❌ failed to copy chunk part %d for upload %s: %v", i, uploadID, err)
			bufWriter.Flush()
			out.Close()
			ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to merge chunks"})
			return
		}
		mergedBytes += n
	}

	if err := bufWriter.Flush(); err != nil {
		log.Printf("❌ failed to flush merged file buffer for upload %s: %v", uploadID, err)
		out.Close()
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to write merged file"})
		return
	}
	out.Close()

	// Verify merged file integrity
	mergedInfo, err := os.Stat(merged)
	if err != nil {
		log.Printf("❌ failed to stat merged file for upload %s: %v", uploadID, err)
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to verify merged file"})
		return
	}
	if mergedInfo.Size() != mergedBytes {
		log.Printf("❌ merged file size mismatch for upload %s: expected %d, got %d", uploadID, mergedBytes, mergedInfo.Size())
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "merged file corruption detected"})
		return
	}

	if sess.TotalSize > 0 && mergedBytes != sess.TotalSize {
		log.Printf("⚠️ chunk merge size mismatch id=%s expected=%d merged=%d", uploadID, sess.TotalSize, mergedBytes)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":    "merged file size mismatch",
			"expected": sess.TotalSize,
			"merged":   mergedBytes,
		})
		return
	}

	if err := storage.ValidateMP4Container(merged); err != nil {
		log.Printf("❌ chunk upload invalid container id=%s: %v", uploadID, err)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid or corrupted video file"})
		return
	}

	mime := sess.Mime
	if mime == "" {
		mime = "video/mp4"
	}
	publicID := fmt.Sprintf("chunk_upload_%s.mp4", uploadID)

	res := storage.UploadLocalVideoPreserve(merged, publicID, mime)
	url := res["url"]
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "cdn upload failed"
		}
		log.Printf("❌ chunk upload CDN failed user=%d id=%s: %s", userID, uploadID, msg)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": msg})
		return
	}

	blurKey := storage.ChunkUploadBlurUploadKey(uploadID)
	videoURL := url
	go func() {
		if blur := videoprocessing.QuickBlurFromVideoURL(videoURL, blurKey); blur != "" {
			log.Printf("✅ chunk relay blur async id=%s → %s", uploadID, blur)
		}
	}()

	log.Printf("✅ chunk upload RELAY complete user=%d id=%s bytes=%d → %s", userID, uploadID, mergedBytes, url)
	chunkMu.Lock()
	delete(chunkSess, uploadID)
	chunkMu.Unlock()
	ctx.JSON(iris.Map{"success": true, "url": url, "bytes": mergedBytes, "mode": "relay"})
}

func validateMultipartParts(parts []storage.CompletedPartInfo, totalChunks int) error {
	if len(parts) != totalChunks {
		return fmt.Errorf("part count mismatch")
	}
	seen := make(map[int]bool, totalChunks)
	for _, p := range parts {
		if p.PartNumber < 1 || p.PartNumber > totalChunks {
			return fmt.Errorf("invalid part number %d", p.PartNumber)
		}
		if strings.TrimSpace(p.ETag) == "" {
			return fmt.Errorf("missing etag for part %d", p.PartNumber)
		}
		if seen[p.PartNumber] {
			return fmt.Errorf("duplicate part number %d", p.PartNumber)
		}
		seen[p.PartNumber] = true
	}
	for i := 1; i <= totalChunks; i++ {
		if !seen[i] {
			return fmt.Errorf("missing part number %d", i)
		}
	}
	return nil
}
