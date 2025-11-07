package routes

import (
    "net/url"
    "strings"

    "github.com/kataras/iris/v12"
)

// buildCloudinaryWatermarkedURL returns a URL with a static text watermark using Cloudinary transforms
func buildCloudinaryWatermarkedURL(raw string) string {
    if raw == "" || !strings.Contains(raw, "res.cloudinary.com") || !strings.Contains(raw, "/upload/") {
        return raw
    }
    parts := strings.Split(raw, "/upload/")
    if len(parts) != 2 {
        return raw
    }
    overlayText := url.QueryEscape("Habitat")
    transform := "l_text:Arial_32_bold:" + overlayText + ",co_white,g_south_west,x_20,y_20,o_70"
    return parts[0] + "/upload/" + transform + "/" + parts[1]
}

// GetWatermarkedVideo returns a watermarked variant URL for a given source (Cloudinary only)
func GetWatermarkedVideo(ctx iris.Context) {
    src := strings.TrimSpace(ctx.URLParam("url"))
    if src == "" {
        ctx.StatusCode(iris.StatusBadRequest)
        ctx.JSON(iris.Map{"success": false, "error": "missing url"})
        return
    }
    out := buildCloudinaryWatermarkedURL(src)
    ctx.JSON(iris.Map{"success": true, "url": out})
}


