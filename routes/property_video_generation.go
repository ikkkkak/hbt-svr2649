package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
)

const maxMusicUploadBytes = 20 * 1024 * 1024

// GET /api/property-video-jobs/:id
func GetPropertyVideoGenerationJob(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		utils.CreateError(http.StatusUnauthorized, "Unauthorized", "Sign in required", ctx)
		return
	}
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "Invalid job id")
		return
	}
	var job models.PropertyVideoGenerationJob
	if err := storage.DB.First(&job, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "Job not found")
		return
	}
	if job.UserID != userID {
		utils.JSONError(ctx, http.StatusForbidden, "forbidden", "Not your job")
		return
	}
	ctx.JSON(iris.Map{"data": job})
}

// GET /api/property-video-jobs/by-listing?sale_id=123
func GetPropertyVideoJobByListing(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		utils.CreateError(http.StatusUnauthorized, "Unauthorized", "Sign in required", ctx)
		return
	}
	saleID, _ := strconv.ParseUint(strings.TrimSpace(ctx.URLParam("sale_id")), 10, 64)
	if saleID == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_sale_id", "sale_id required")
		return
	}
	var job models.PropertyVideoGenerationJob
	q := storage.DB.Where("entity_type = ? AND entity_id = ?", "sale", saleID).
		Order("id DESC")
	if err := q.First(&job).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "No video job for this listing")
		return
	}
	if job.UserID != userID {
		utils.JSONError(ctx, http.StatusForbidden, "forbidden", "Not your job")
		return
	}
	ctx.JSON(iris.Map{"data": job})
}

// --- Admin music library ---

// GET /api/admin/music-tracks
func AdminListMusicTracks(ctx iris.Context) {
	var tracks []models.MusicTrack
	if err := storage.DB.Order("category ASC, sort_order ASC, id ASC").Find(&tracks).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": tracks})
}

// POST /api/admin/music-tracks
func AdminCreateMusicTrack(ctx iris.Context) {
	var in struct {
		Title       string  `json:"title"`
		Category    string  `json:"category"`
		FileURL     string  `json:"file_url"`
		DurationSec float64 `json:"duration_sec"`
		IsActive    *bool   `json:"is_active"`
		SortOrder   int     `json:"sort_order"`
		Notes       string  `json:"notes"`
	}
	if err := ctx.ReadJSON(&in); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "Invalid JSON")
		return
	}
	title := strings.TrimSpace(in.Title)
	fileURL := strings.TrimSpace(in.FileURL)
	if title == "" || fileURL == "" {
		utils.JSONError(ctx, http.StatusBadRequest, "validation", "title and file_url are required")
		return
	}
	cat := strings.TrimSpace(in.Category)
	if cat == "" {
		cat = "default"
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	track := models.MusicTrack{
		Title: title, Category: cat, FileURL: fileURL,
		DurationSec: in.DurationSec, IsActive: active,
		SortOrder: in.SortOrder, Notes: strings.TrimSpace(in.Notes),
	}
	if err := storage.DB.Create(&track).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": track})
}

// PATCH /api/admin/music-tracks/:id
func AdminUpdateMusicTrack(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	var track models.MusicTrack
	if err := storage.DB.First(&track, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "Track not found")
		return
	}
	var in map[string]interface{}
	if err := ctx.ReadJSON(&in); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "Invalid JSON")
		return
	}
	allowed := map[string]bool{
		"title": true, "category": true, "file_url": true, "duration_sec": true,
		"is_active": true, "sort_order": true, "notes": true,
	}
	up := map[string]interface{}{}
	for k, v := range in {
		if allowed[k] {
			up[k] = v
		}
	}
	if len(up) == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "nothing_to_update", "No valid fields")
		return
	}
	if err := storage.DB.Model(&track).Updates(up).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	_ = storage.DB.First(&track, id)
	ctx.JSON(iris.Map{"data": track})
}

// POST /api/admin/music-tracks/upload — multipart field "audio" (mp3/m4a/aac/wav)
func AdminUploadMusicFile(ctx iris.Context) {
	file, header, err := ctx.FormFile("audio")
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "missing audio field (multipart form)"})
		return
	}
	defer file.Close()

	if header.Size > maxMusicUploadBytes {
		ctx.StopWithJSON(http.StatusRequestEntityTooLarge, iris.Map{
			"error": fmt.Sprintf("audio too large (max %dMB)", maxMusicUploadBytes/(1024*1024)),
		})
		return
	}

	mime := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if mime == "" || mime == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".mp3":
			mime = "audio/mpeg"
		case ".m4a":
			mime = "audio/mp4"
		case ".aac":
			mime = "audio/aac"
		case ".wav":
			mime = "audio/wav"
		default:
			mime = "audio/mpeg"
		}
	}
	if !strings.HasPrefix(mime, "audio/") {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "only audio files are allowed"})
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".mp3"
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	publicID := fmt.Sprintf("music/library_%s%s", hex.EncodeToString(b), ext)

	tmpDir, err := os.MkdirTemp("", "musicup_")
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, "upload"+ext)

	out, err := os.Create(tmpPath)
	if err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	written, err := io.Copy(out, io.LimitReader(file, maxMusicUploadBytes))
	out.Close()
	if err != nil || written <= 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "failed to read audio file"})
		return
	}

	res := storage.UploadLocalFile(tmpPath, publicID, mime)
	url := strings.TrimSpace(res["url"])
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "upload failed"
		}
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": msg})
		return
	}

	ctx.JSON(iris.Map{
		"url":   url,
		"bytes": written,
		"mime":  mime,
	})
}

// DELETE /api/admin/music-tracks/:id
func AdminDeleteMusicTrack(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "Invalid id")
		return
	}
	if err := storage.DB.Delete(&models.MusicTrack{}, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.StatusCode(http.StatusNoContent)
}
