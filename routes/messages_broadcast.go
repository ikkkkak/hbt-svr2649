package routes

import (
    "encoding/json"
    "strconv"

    "github.com/kataras/iris/v12"
    "apartments-clone-server/realtime"
    pushsvc "apartments-clone-server/services/push"
)

// Call this after persisting a message to DB
func BroadcastGroupMessageHandler(ctx iris.Context) {
    gidStr := ctx.Params().Get("groupID")
    gid64, _ := strconv.ParseUint(gidStr, 10, 64)
    groupID := uint(gid64)

    // Parse minimal payload
    var dto struct {
        ID       uint   `json:"id"`
        SenderID uint   `json:"senderId"`
        Content  string `json:"content"`
        Sender   string `json:"senderName"`
    }
    if err := ctx.ReadJSON(&dto); err != nil { ctx.StatusCode(400); return }

    evt := map[string]interface{}{
        "type": "message",
        "groupId": groupID,
        "senderId": dto.SenderID,
        "message": map[string]interface{}{
            "id": dto.ID,
            "senderId": dto.SenderID,
            "content": dto.Content,
        },
    }
    b, _ := json.Marshal(evt)
    realtime.HubInstance().Broadcast(groupID, b, dto.SenderID)

    // Push notifications
    tokens := pushsvc.GetGroupPushTokens(groupID, dto.SenderID)
    if len(tokens) > 0 {
        body := dto.Content
        if len(body) > 120 { body = body[:120] + "…" }
        _ = pushsvc.SendExpoPush(tokens, dto.Sender, body)
    }

    ctx.StatusCode(204)
}



