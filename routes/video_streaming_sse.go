package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services/videoprocessing"
	"apartments-clone-server/storage"
	"fmt"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// VideoProcessingSSE streams transcoding progress (Server-Sent Events).
// GET /api/video/:id/streaming/events
func VideoProcessingSSE(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if video.UserID != userID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	ctx.ContentType("text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	w := ctx.ResponseWriter()
	flusher, ok := w.(http.Flusher)
	if !ok {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	writeSSE := func(ev videoprocessing.ProcessingEvent) {
		_, _ = fmt.Fprintf(w, "event: progress\ndata: {\"videoID\":%d,\"processingStatus\":\"%s\",\"progress\":%d,\"ready\":%t,\"hlsURL\":\"%s\",\"mobileVideoURL\":\"%s\",\"processingError\":\"%s\",\"spriteSheetURL\":\"%s\"}\n\n",
			ev.VideoID, ev.ProcessingStatus, ev.Progress, ev.Ready,
			escapeJSON(ev.HlsURL), escapeJSON(ev.MobileVideoURL), escapeJSON(ev.ProcessingError), escapeJSON(ev.SpriteSheetURL),
		)
		flusher.Flush()
	}

	// Initial snapshot
	writeSSE(videoprocessing.ProcessingEvent{
		VideoID:          video.ID,
		ProcessingStatus: video.ProcessingStatus,
		ProcessingError:  video.ProcessingError,
		Progress:         video.ProcessingProgress,
		HlsURL:           video.HlsURL,
		MobileVideoURL:   video.MobileVideoURL,
		SpriteSheetURL:   video.SpriteSheetURL,
		Ready:            video.ProcessingStatus == "ready",
	})
	if video.ProcessingStatus == "ready" || video.ProcessingStatus == "failed" {
		return
	}

	ch := videoprocessing.SubscribeSSE(videoID)
	defer videoprocessing.UnsubscribeSSE(videoID, ch)

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	reqCtx := ctx.Request().Context()
	for {
		select {
		case <-reqCtx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			writeSSE(ev)
			if ev.Ready || ev.ProcessingStatus == "failed" {
				return
			}
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func escapeJSON(s string) string {
	out := ""
	for _, r := range s {
		switch r {
		case '\\':
			out += "\\\\"
		case '"':
			out += "\\\""
		default:
			out += string(r)
		}
	}
	return out
}
