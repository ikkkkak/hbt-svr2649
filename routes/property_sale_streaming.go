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

// GetPropertySaleVideoStreamingStatus returns transcoding progress for upload UI polling.
func GetPropertySaleVideoStreamingStatus(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	saleVideoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var video models.PropertySaleVideo
	if err := storage.DB.First(&video, saleVideoID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if video.UserID != userID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	ctx.JSON(iris.Map{
		"success":          true,
		"videoID":          video.ID,
		"entityType":       "sale",
		"processingStatus": video.ProcessingStatus,
		"processingError":  video.ProcessingError,
		"hlsURL":           video.HlsURL,
		"mobileVideoURL":   video.MobileVideoURL,
		"previewBlurURL":   video.PreviewBlurURL,
		"preview_blur_url": video.PreviewBlurURL,
		"videoURL":         video.VideoURL,
		"ready":            video.ProcessingStatus == "ready",
	})
}

// PropertySaleVideoProcessingSSE streams transcoding progress (Server-Sent Events).
func PropertySaleVideoProcessingSSE(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	saleVideoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var video models.PropertySaleVideo
	if err := storage.DB.First(&video, saleVideoID).Error; err != nil {
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
		_, _ = fmt.Fprintf(w, "event: progress\ndata: {\"videoID\":%d,\"entityType\":\"sale\",\"processingStatus\":\"%s\",\"progress\":%d,\"ready\":%t,\"hlsURL\":\"%s\",\"mobileVideoURL\":\"%s\",\"processingError\":\"%s\",\"spriteSheetURL\":\"%s\"}\n\n",
			ev.VideoID, ev.ProcessingStatus, ev.Progress, ev.Ready,
			escapeJSON(ev.HlsURL), escapeJSON(ev.MobileVideoURL), escapeJSON(ev.ProcessingError), escapeJSON(ev.SpriteSheetURL),
		)
		flusher.Flush()
	}

	writeSSE(videoprocessing.ProcessingEvent{
		VideoID:          video.ID,
		EntityType:       "sale",
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

	ch := videoprocessing.SubscribeSSE(saleVideoID)
	defer videoprocessing.UnsubscribeSSE(saleVideoID, ch)

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
