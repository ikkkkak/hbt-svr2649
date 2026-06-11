package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"net/http"

	"github.com/kataras/iris/v12"
)

// GetVideoStreamingStatus returns transcoding progress for upload UI polling.
func GetVideoStreamingStatus(ctx iris.Context) {
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
	ctx.JSON(iris.Map{
		"success":          true,
		"videoID":          video.ID,
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
