package routes

import (
    "apartments-clone-server/models"
    "apartments-clone-server/storage"
    "log"
    "strconv"
    "strings"
    "time"
    "github.com/kataras/iris/v12"
    pushsvc "apartments-clone-server/services/push"
)

type pushTokenReq struct {
    Token string `json:"token"`
    DeviceID string `json:"deviceId"`
}

func RegisterPushToken(ctx iris.Context) {
    var req pushTokenReq
    if err := ctx.ReadJSON(&req); err != nil { ctx.StatusCode(400); return }
    if req.Token == "" { ctx.StatusCode(400); return }
    req.DeviceID = strings.TrimSpace(req.DeviceID)
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
        if userID == 0 && req.DeviceID == "" {
            ctx.StatusCode(401)
            return
        }
    }

    // Persist against user when authenticated.
    if userID > 0 {
        log.Printf("🔔 Registering token for user %d: length=%d prefix=%s...", userID, len(req.Token), func() string { if len(req.Token) > 12 { return req.Token[:12] } ; return req.Token }())
        pushsvc.SetUserPushToken(userID, req.Token)
    }

    // Persist device-level fallback so pushes still work when JWT expires / user is logged out.
    if req.DeviceID != "" {
        var device models.MarketingDevice
        q := storage.DB.Where("device_id = ?", req.DeviceID).First(&device)
        if q.Error != nil {
            next := time.Now().Add(6 * time.Hour)
            device = models.MarketingDevice{
                DeviceID: req.DeviceID,
                FCMToken: req.Token,
                MarketingOptIn: true,
                NextSendAt: &next,
            }
            if userID > 0 {
                device.UserID = &userID
            }
            _ = storage.DB.Create(&device).Error
        } else {
            updates := map[string]interface{}{
                "fcm_token": req.Token,
                "marketing_opt_in": true,
            }
            if userID > 0 {
                updates["user_id"] = userID
            }
            _ = storage.DB.Model(&device).Updates(updates).Error
        }
    }

    log.Printf("🔔 Token saved successfully (length: %d)", len(req.Token))
    ctx.StatusCode(204)
}




