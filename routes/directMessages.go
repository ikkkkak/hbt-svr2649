package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"fmt"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
)

// BlockUserForDM - Block a user for direct messages
func BlockUserForDM(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	blockedUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	if uid == blockedUserID {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "cannot block yourself"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	ctx.ReadJSON(&body)

	// Check if already blocked
	var existingBlock models.UserBlock
	if err := storage.DB.Where("blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		uid, blockedUserID).First(&existingBlock).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "user is already blocked"})
		return
	}

	// Create block record
	blockRecord := models.UserBlock{
		BlockerID: uid,
		BlockedID: blockedUserID,
		Reason:    body.Reason,
	}
	if err := storage.DB.Create(&blockRecord).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to block user"})
		return
	}

	ctx.JSON(iris.Map{
		"message":         "User blocked successfully",
		"blocked_user_id": blockedUserID,
	})
}

// UnblockUserForDM - Unblock a user for direct messages
func UnblockUserForDM(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	blockedUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	// Soft delete the block record
	if err := storage.DB.Where("blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		uid, blockedUserID).Delete(&models.UserBlock{}).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "block record not found"})
		return
	}

	ctx.JSON(iris.Map{
		"message":           "User unblocked successfully",
		"unblocked_user_id": blockedUserID,
	})
}

// GetBlockedUsersForDM - Get users blocked by current user for direct messages
func GetBlockedUsersForDM(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	var blockedUsers []struct {
		ID        uint      `json:"id"`
		BlockedID uint      `json:"blocked_id"`
		UserName  string    `json:"user_name"`
		UserEmail string    `json:"user_email"`
		Reason    string    `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}

	storage.DB.Table("user_blocks").
		Select("user_blocks.id, user_blocks.blocked_id, users.name as user_name, users.email as user_email, user_blocks.reason, user_blocks.created_at").
		Joins("JOIN users ON users.id = user_blocks.blocked_id").
		Where("user_blocks.blocker_id = ? AND user_blocks.deleted_at IS NULL", uid).
		Order("user_blocks.created_at DESC").
		Scan(&blockedUsers)

	ctx.JSON(iris.Map{
		"blocked_users": blockedUsers,
		"total":         len(blockedUsers),
	})
}

// SendDirectMessage - Send a direct message
func SendDirectMessage(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	var body struct {
		ReceiverID uint   `json:"receiver_id"`
		Content    string `json:"content"`
		Type       string `json:"type"`
		RefType    string `json:"ref_type"`
		RefID      *uint  `json:"ref_id"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid json"})
		return
	}

	if body.Content == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "content is required"})
		return
	}

	// Check if receiver has blocked the sender
	var blockCheck models.UserBlock
	if err := storage.DB.Where("blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		body.ReceiverID, uid).First(&blockCheck).Error; err == nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "cannot send message to this user"})
		return
	}

	// Create direct message
	message := models.DirectMessage{
		SenderID:   uid,
		ReceiverID: body.ReceiverID,
		Content:    body.Content,
		Type:       body.Type,
		RefType:    body.RefType,
		RefID:      body.RefID,
	}
	if err := storage.DB.Create(&message).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to send message"})
		return
	}

	// Send push notification to receiver
	var sender models.User
	var receiver models.User
	if err := storage.DB.First(&sender, uid).Error; err == nil {
		if err := storage.DB.First(&receiver, body.ReceiverID).Error; err == nil {
			senderName := fmt.Sprintf("%s %s", sender.FirstName, sender.LastName)
			propertyTitle := "عقار"

			// If message is about a property, get property title
			if body.RefType == "property" && body.RefID != nil {
				var property models.Property
				if err := storage.DB.First(&property, *body.RefID).Error; err == nil {
					propertyTitle = property.Title
				}
			}

			notificationService := services.NewNotificationService()
			go notificationService.SendMessageNotificationToHost(
				body.ReceiverID,
				uid,
				senderName,
				propertyTitle,
			)
		}
	}

	ctx.JSON(iris.Map{
		"message":    "Message sent successfully",
		"message_id": message.ID,
	})
}

// GetDirectMessages - Get direct messages between two users
func GetDirectMessages(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	otherUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	// Check if either user has blocked the other
	var blockCheck models.UserBlock
	if err := storage.DB.Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?) AND deleted_at IS NULL",
		uid, otherUserID, otherUserID, uid).First(&blockCheck).Error; err == nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "cannot access messages with this user"})
		return
	}

	var messages []models.DirectMessage
	storage.DB.Where("((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)) AND deleted_at IS NULL",
		uid, otherUserID, otherUserID, uid).
		Order("created_at ASC").
		Find(&messages)

	ctx.JSON(iris.Map{
		"messages": messages,
		"total":    len(messages),
	})
}

// MarkDirectMessageRead - Mark a direct message as read
func MarkDirectMessageRead(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	messageID, err := ctx.Params().GetUint("messageID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid message id"})
		return
	}

	// Update message as read
	now := time.Now()
	if err := storage.DB.Model(&models.DirectMessage{}).
		Where("id = ? AND receiver_id = ?", messageID, uid).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": &now,
		}).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "message not found"})
		return
	}

	ctx.JSON(iris.Map{"message": "Message marked as read"})
}
