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
	TotalChunks int
	Received    map[int]bool
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
	}
	if err := ctx.ReadJSON(&in); err != nil || in.TotalChunks < 1 || in.TotalChunks > 5000 {
		ctx.StopWithStatus(http.StatusBadRequest)
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
		ID: id, UserID: userID, Mime: in.Mime,
		TotalChunks: in.TotalChunks, Received: make(map[int]bool),
		TempDir: dir, CreatedAt: time.Now(),
	}
	chunkMu.Unlock()
	log.Printf("📦 chunk upload init user=%d id=%s chunks=%d size=%d", userID, id, in.TotalChunks, in.TotalSize)
	ctx.JSON(iris.Map{
		"success":     true,
		"uploadId":    id,
		"chunkSize":   maxChunkBytes,
		"totalChunks": in.TotalChunks,
	})
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
	body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, maxChunkBytes+1))
	if err != nil || len(body) > maxChunkBytes {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	path := filepath.Join(sess.TempDir, fmt.Sprintf("part_%05d", index))
	if err := os.WriteFile(path, body, 0644); err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	chunkMu.Lock()
	sess.Received[index] = true
	received := len(sess.Received)
	total := sess.TotalChunks
	chunkMu.Unlock()
	ctx.JSON(iris.Map{"success": true, "index": index, "received": received, "total": total})
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
		ctx.StopWithStatus(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "missing chunks"})
		return
	}

	merged := filepath.Join(sess.TempDir, "merged.mp4")
	out, err := os.Create(merged)
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	for i := 0; i < sess.TotalChunks; i++ {
		part := filepath.Join(sess.TempDir, fmt.Sprintf("part_%05d", i))
		f, err := os.Open(part)
		if err != nil {
			out.Close()
			ctx.StopWithStatus(http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(out, f)
		f.Close()
	}
	out.Close()

	mime := sess.Mime
	if mime == "" {
		mime = "video/mp4"
	}
	res := storage.UploadLocalFileOptimized(merged, fmt.Sprintf("chunk_upload_%s", uploadID), mime)
	url := res["url"]
	if url == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": res["error"]})
		return
	}
	log.Printf("✅ chunk upload complete user=%d id=%s → %s", userID, uploadID, url)
	ctx.JSON(iris.Map{"success": true, "url": url})
}
