package routes

import (
    "log"
    "strconv"
    "strings"
    "github.com/kataras/iris/v12"
    pushsvc "apartments-clone-server/services/push"
)

type pushTokenReq struct {
    Token string `json:"token"`
}

func RegisterPushToken(ctx iris.Context) {
    var req pushTokenReq
    if err := ctx.ReadJSON(&req); err != nil { ctx.StatusCode(400); return }
    if req.Token == "" { ctx.StatusCode(400); return }
    // In production, get from JWT middleware
    var userID uint
    if id := ctx.GetHeader("X-User-ID"); id != "" {
        // tolerate whitespace
        trimmed := strings.TrimSpace(id)
        if u64, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
            userID = uint(u64)
        } else {
            ctx.StatusCode(400)
            return
        }
    } else {
        // fallback: try value set by optional or strict JWT middleware
        if v := ctx.Values().Get("userID"); v != nil {
            if n, ok := v.(int); ok { userID = uint(n) }
            if n, ok := v.(uint); ok { userID = n }
            if n, ok := v.(int64); ok { userID = uint(n) }
            if n, ok := v.(uint64); ok { userID = uint(n) }
        }
        if userID == 0 {
            ctx.StatusCode(401)
            return
        }
    }
    log.Printf("🔔 Registering token for user %d: length=%d prefix=%s...", userID, len(req.Token), func() string { if len(req.Token) > 12 { return req.Token[:12] } ; return req.Token }())
    pushsvc.SetUserPushToken(userID, req.Token)
    log.Printf("🔔 Token saved successfully (length: %d)", len(req.Token))
    ctx.StatusCode(204)
}




