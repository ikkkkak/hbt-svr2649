package routes

import (
	"apartments-clone-server/storage"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
)

const maxLegacyVideoUploadBytes = 12 * 1024 * 1024 // 12MB JSON body cap

type uploadInput struct {
	Data     string `json:"data"`      // base64 data URL or raw base64
	PublicID string `json:"public_id"` // optional
	Mime     string `json:"mime"`      // for video
}

// UploadImage handles base64 image upload (CDN from MEDIA_CDN env).
func UploadImage(ctx iris.Context) {
	var in uploadInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid payload"})
		return
	}
	res := storage.UploadBase64ImageOptimized(in.Data, in.PublicID)
	url := res["url"]
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "upload failed"
		}
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": msg})
		return
	}
	ctx.JSON(iris.Map{"url": url})
}

// UploadVideo handles base64 video upload (CDN from MEDIA_CDN env).
// Prefer chunked upload: POST /upload/video/init + PUT chunks + POST complete.
func UploadVideo(ctx iris.Context) {
	if cl := ctx.GetHeader("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > maxLegacyVideoUploadBytes {
			log.Printf("⚠️ legacy POST /upload/video rejected: %d bytes (use chunked upload)", n)
			ctx.StopWithJSON(http.StatusRequestEntityTooLarge, iris.Map{
				"error": "Video too large for single upload. Update the app and use resumable chunked upload (/upload/video/init).",
				"code":  "USE_CHUNKED_UPLOAD",
			})
			return
		}
	}
	log.Printf("⚠️ legacy POST /upload/video (base64) — prefer /upload/video/init")
	var in uploadInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid payload"})
		return
	}
	res := storage.UploadBase64VideoOptimized(in.Data, in.PublicID, in.Mime)
	url := res["url"]
	if url == "" {
		msg := res["error"]
		if msg == "" {
			msg = "upload failed"
		}
		fmt.Printf("video upload failed: %s\n", msg)
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": msg})
		return
	}
	ctx.JSON(iris.Map{"url": url})
}
