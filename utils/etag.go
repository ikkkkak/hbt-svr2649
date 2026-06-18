package utils

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/kataras/iris/v12"
)

// RespondJSONWithETag writes JSON with a weak ETag. Returns true if 304 Not Modified was sent.
func RespondJSONWithETag(ctx iris.Context, status int, payload interface{}) bool {
	b, err := json.Marshal(payload)
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to encode response"})
		return true
	}
	sum := sha256.Sum256(b)
	etag := fmt.Sprintf(`"W/%x"`, sum[:8])
	ctx.Header("ETag", etag)
	if ctx.GetHeader("If-None-Match") == etag {
		ctx.StatusCode(iris.StatusNotModified)
		return true
	}
	ctx.StatusCode(status)
	ctx.ContentType("application/json")
	_, _ = ctx.Write(b)
	return false
}
