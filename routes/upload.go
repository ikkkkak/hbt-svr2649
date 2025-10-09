package routes

import (
	"apartments-clone-server/storage"
	"fmt"
	"net/http"

	"github.com/kataras/iris/v12"
)

type uploadInput struct {
	Data     string `json:"data"`      // base64 data URL or raw base64
	PublicID string `json:"public_id"` // optional
	Mime     string `json:"mime"`      // for video
}

// UploadImage handles base64 image upload to Cloudinary
func UploadImage(ctx iris.Context) {
	var in uploadInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid payload"})
		return
	}
	res := storage.UploadBase64Image(in.Data, in.PublicID)
	url := res["url"]
	if url == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "upload failed"})
		return
	}
	ctx.JSON(iris.Map{"url": url})
}

// UploadVideo handles base64 video upload to Cloudinary
func UploadVideo(ctx iris.Context) {
	var in uploadInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid payload"})
		return
	}
	res := storage.UploadBase64Video(in.Data, in.PublicID, in.Mime)
	url := res["url"]
	if url == "" {
		fmt.Println("video upload failed")
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "upload failed"})
		return
	}
	ctx.JSON(iris.Map{"url": url})
}
