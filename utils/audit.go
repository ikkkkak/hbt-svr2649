package utils

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"net"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

func Audit(ctx iris.Context, action, resourceType string, resourceID uint, before interface{}, after interface{}) {
	var beforeStr, afterStr string
	if before != nil {
		if b, err := json.Marshal(before); err == nil {
			beforeStr = string(b)
		}
	}
	if after != nil {
		if a, err := json.Marshal(after); err == nil {
			afterStr = string(a)
		}
	}
	var adminID uint
	if tok := jsonWT.Get(ctx); tok != nil {
		if at, ok := tok.(*AccessToken); ok {
			adminID = at.ID
		}
	}
	ip := clientIP(ctx)
	log := models.AuditLog{AdminUserID: adminID, Action: action, ResourceType: resourceType, ResourceID: resourceID, BeforeJSON: beforeStr, AfterJSON: afterStr, IPAddress: ip}
	storage.DB.Create(&log)
}

func GetAccessToken(ctx iris.Context) *AccessToken { return nil }

func GetJWT(ctx iris.Context) interface{} { return nil }

func clientIP(ctx iris.Context) string {
	if ip := ctx.GetHeader("X-Forwarded-For"); ip != "" {
		return ip
	}
	ip, _, _ := net.SplitHostPort(ctx.RemoteAddr())
	return ip
}
