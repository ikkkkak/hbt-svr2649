package routes

import (
	"apartments-clone-server/storage"
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

const maxChunkBytes = 5 * 1024 * 1024 // 5MB per chunk

type chunkSession struct {
	ID          string
	UserID      uint
	Mime        string
	TotalSize   int64
	TotalChunks int
	ChunkSize   int
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
	}
	if err := ctx.ReadJSON(&in); err != nil || in.TotalChunks < 1 || in.TotalChunks > 5000 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid upload metadata"})
		return
	}
	if in.TotalSize <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "totalSize required"})
		return
	}
	mime := strings.TrimSpace(in.Mime)
	if mime == "" {
		mime = "video/mp4"
	}
	if !strings.HasPrefix(strings.ToLower(mime), "video/") {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "only video uploads are supported via chunked endpoint"})
		return
	}

	chunkSize := in.ChunkSize
	if chunkSize <= 0 || chunkSize > maxChunkBytes {
		chunkSize = maxChunkBytes
	}
	expectedChunks := int((in.TotalSize + int64(chunkSize) - 1) / int64(chunkSize))
	if in.TotalChunks != expectedChunks {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":           "totalChunks does not match totalSize/chunkSize",
			"expectedChunks":  expectedChunks,
			"receivedChunks":  in.TotalChunks,
		})
		return
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	dir, err := os.MkdirTemp("", "vupload_"+id+"_")
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	chunkMu.Lock()
	chunkSess[id] = &chunkSession{
		ID: id, UserID: userID, Mime: mime,
		TotalSize: in.TotalSize, TotalChunks: in.TotalChunks, ChunkSize: chunkSize,
		Received: make(map[int]bool), ChunkSizes: make(map[int]int),
		TempDir: dir, CreatedAt: time.Now(),
	}
	chunkMu.Unlock()
	log.Printf("📦 chunk upload init user=%d id=%s chunks=%d chunkSize=%d size=%d mime=%s", userID, id, in.TotalChunks, chunkSize, in.TotalSize, mime)
	ctx.JSON(iris.Map{
		"success":     true,
		"uploadId":    id,
		"chunkSize":   chunkSize,
		"totalChunks": in.TotalChunks,
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
	if index >= sess.TotalChunks {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "chunk index out of range"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, maxChunkBytes+1))
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "failed to read chunk"})
		return
	}
	if len(body) == 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "empty chunk"})
		return
	}
	if len(body) > maxChunkBytes {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "chunk too large"})
		return
	}

	expected := expectedChunkPayloadSize(sess, index)
	if expected <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid chunk index"})
		return
	}
	if len(body) != expected {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":    "chunk size mismatch",
			"expected": expected,
			"got":      len(body),
			"index":    index,
		})
		return
	}

	path := filepath.Join(sess.TempDir, fmt.Sprintf("part_%05d", index))
	if err := os.WriteFile(path, body, 0644); err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	chunkMu.Lock()
	sess.Received[index] = true
	sess.ChunkSizes[index] = len(body)
	received := len(sess.Received)
	total := sess.TotalChunks
	chunkMu.Unlock()
	ctx.JSON(iris.Map{"success": true, "index": index, "received": received, "total": total, "bytes": len(body)})
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
	if ok {
		delete(chunkSess, uploadID)
	}
	chunkMu.Unlock()
	if !ok || sess.UserID != userID {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	defer os.RemoveAll(sess.TempDir)

	if len(sess.Received) != sess.TotalChunks {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{
			"error":     "missing chunks",
			"received":  len(sess.Received),
			"expected":  sess.TotalChunks,
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
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	var mergedBytes int64
	for i := 0; i < sess.TotalChunks; i++ {
		part := filepath.Join(sess.TempDir, fmt.Sprintf("part_%05d", i))
		f, err := os.Open(part)
		if err != nil {
			out.Close()
			ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing chunk file", "index": i})
			return
		}
		n, err := io.Copy(out, f)
		f.Close()
		if err != nil {
			out.Close()
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
		mergedBytes += n
	}
	out.Close()

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
	log.Printf("✅ chunk upload complete user=%d id=%s bytes=%d → %s", userID, uploadID, mergedBytes, url)
	ctx.JSON(iris.Map{"success": true, "url": url, "bytes": mergedBytes})
}
