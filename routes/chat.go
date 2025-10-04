package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

type sendMessageInput struct {
	Content string `json:"content"`
	Color   string `json:"color"`
	TTLSec  int    `json:"ttlSec"`
}

// List recent messages for a group (last 100)
func ListGroupMessages(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	// Ensure membership
	var membership models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&membership).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	var msgs []models.ChatMessage
	storage.DB.Where("group_id = ?", groupID).
		Preload("Sender").
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("id DESC").Limit(100).Find(&msgs)
	// reverse to chronological
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	ctx.JSON(iris.Map{"success": true, "messages": msgs})
}

// Send a message
func SendGroupMessage(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	// Ensure membership
	var membership models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&membership).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var input sendMessageInput
	if err := ctx.ReadJSON(&input); err != nil || input.Content == "" {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var expires *time.Time
	if input.TTLSec > 0 {
		t := time.Now().Add(time.Duration(input.TTLSec) * time.Second)
		expires = &t
	}
	msg := models.ChatMessage{
		GroupID:   groupID,
		SenderID:  user.ID,
		Content:   input.Content,
		Color:     input.Color,
		ExpiresAt: expires,
	}
	if err := storage.DB.Create(&msg).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	// preload sender for client display
	storage.DB.Preload("Sender").First(&msg, msg.ID)
	ctx.JSON(iris.Map{"success": true, "message": msg})
}

type startDirectInput struct {
	HostID     uint   `json:"hostID" validate:"required"`
	PropertyID uint   `json:"propertyID" validate:"required"`
	Message    string `json:"message"`
}

// StartDirectConversation creates or reuses a direct chat and sends a property card message
func StartDirectConversation(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var input startDirectInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Find or create a direct group using ExperienceGroup as chat room
	var group models.ExperienceGroup
	storage.DB.
		Joins("JOIN experience_group_members m1 ON m1.group_id = experience_groups.id").
		Joins("JOIN experience_group_members m2 ON m2.group_id = experience_groups.id").
		Where("m1.user_id = ? AND m2.user_id = ? AND experience_groups.privacy = ?", user.ID, input.HostID, "direct").
		First(&group)

	if group.ID == 0 {
		group = models.ExperienceGroup{Name: "Conversation", OwnerID: user.ID, Privacy: "direct", Status: "active"}
		if err := storage.DB.Create(&group).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
		if err := storage.DB.Create(&models.ExperienceGroupMember{GroupID: group.ID, UserID: user.ID, Role: "member", State: "joined"}).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
		if err := storage.DB.Create(&models.ExperienceGroupMember{GroupID: group.ID, UserID: input.HostID, Role: "member", State: "joined"}).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
	}

	// Load property to include a preview
	var property models.Property
	storage.DB.First(&property, input.PropertyID)

	previewTitle := property.Title
	previewSubtitle := property.City
	previewImage := ""
	if property.Images != "" {
		var imgs []string
		if err := json.Unmarshal([]byte(property.Images), &imgs); err == nil && len(imgs) > 0 {
			previewImage = imgs[0]
		}
	}

	msg := models.ChatMessage{
		GroupID:         group.ID,
		SenderID:        user.ID,
		Content:         input.Message,
		PreviewTitle:    previewTitle,
		PreviewSubtitle: previewSubtitle,
		PreviewImageURL: previewImage,
		Color:           "#222222",
	}
	if err := storage.DB.Create(&msg).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true, "groupID": group.ID})
}

// Typing indicator: set a short-lived key in Redis for 5 seconds
func Typing(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	// Ensure membership
	var membership models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&membership).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	key := typingKey(groupID, user.ID)
	storage.Redis.Set(ctx, key, "1", 5*time.Second)
	ctx.JSON(iris.Map{"success": true})
}

// List who is typing by scanning known group members and checking Redis keys
func ListTyping(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	// Ensure membership
	var members []models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ?", groupID).Preload("User").Find(&members).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	typing := []iris.Map{}
	for _, m := range members {
		if m.UserID == user.ID {
			continue
		}
		key := typingKey(groupID, m.UserID)
		if val, err := storage.Redis.Get(ctx, key).Result(); err == nil && val == "1" {
			typing = append(typing, iris.Map{
				"userID": m.UserID,
				"name":   m.User.FirstName + " " + m.User.LastName,
			})
		}
	}
	ctx.JSON(iris.Map{"success": true, "typing": typing})
}

func typingKey(groupID uint, userID uint) string {
	return fmt.Sprintf("typing:grp:%d:user:%d", groupID, userID)
}
