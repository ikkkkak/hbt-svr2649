package routes

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/realtime"
	"apartments-clone-server/services"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

// AdminContactUser POST /admin/users/{id:uint}/contact — send an official Meskeny Team DM to a user.
func AdminContactUser(ctx iris.Context) {
	adminID, ok := ctx.Values().Get("userID").(uint)
	if !ok || adminID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	targetID, err := ctx.Params().GetUint("id")
	if err != nil || targetID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	var body struct {
		Content string `json:"content"`
		Subject string `json:"subject"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid json"})
		return
	}

	content := strings.TrimSpace(body.Content)
	if content == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "content is required"})
		return
	}
	if len(content) > 4000 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "content too long"})
		return
	}

	teamID, err := services.EnsureMeskenyTeamUser()
	if err != nil {
		log.Printf("❌ AdminContactUser: team user: %v", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "meskeny team account unavailable"})
		return
	}

	if targetID == teamID {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "cannot message system account"})
		return
	}

	var target models.User
	if err := storage.DB.First(&target, targetID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "user not found"})
		return
	}

	message := models.DirectMessage{
		SenderID:   teamID,
		ReceiverID: targetID,
		Content:    content,
		Type:       "text",
		RefType:    "meskeny_team",
	}
	if err := storage.DB.Create(&message).Error; err != nil {
		log.Printf("❌ AdminContactUser: create message admin=%d target=%d: %v", adminID, targetID, err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to send message"})
		return
	}

	log.Printf("✅ AdminContactUser: admin=%d sent Meskeny Team message %d → user %d", adminID, message.ID, targetID)

	func() {
		out := map[string]any{
			"type": "dm:new_message",
			"data": message,
		}
		b, err := json.Marshal(out)
		if err != nil {
			return
		}
		realtime.UserHubInstance().BroadcastToUsers([]uint{teamID, targetID}, b)
		realtime.PublishUserEvent(context.Background(), []uint{teamID, targetID}, out)
	}()

	if target.AllowsNotifications == nil || *target.AllowsNotifications {
		title := services.MeskenyTeamDisplayName()
		bodyText := content
		if len(bodyText) > 120 {
			bodyText = bodyText[:120] + "…"
		}
		avatar := services.MeskenyTeamAvatarURL()
		tokens := pushsvc.GetUserPushTokens(targetID)
		if len(tokens) > 0 {
			go func() {
				if err := pushsvc.SendPushWithImage(tokens, title, bodyText, avatar, nil); err != nil {
					log.Printf("⚠️ AdminContactUser push failed user=%d: %v", targetID, err)
				}
			}()
		}
	}

	ctx.JSON(iris.Map{
		"message":         "Message sent",
		"message_id":      message.ID,
		"meskeny_team_id": teamID,
		"receiver_id":     targetID,
		"sent_by_admin_id": adminID,
	})
}

// AdminGetMeskenyTeamInfo GET /admin/meskeny-team — system account metadata for dashboard.
func AdminGetMeskenyTeamInfo(ctx iris.Context) {
	teamID, err := services.EnsureMeskenyTeamUser()
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "meskeny team account unavailable"})
		return
	}
	ctx.JSON(iris.Map{
		"team_user_id": teamID,
		"display_name": services.MeskenyTeamDisplayName(),
		"avatar_url":   services.MeskenyTeamAvatarURL(),
	})
}
