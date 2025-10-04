package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

func CreateMessage(ctx iris.Context) {
	var req CreateMessageInput

	err := ctx.ReadJSON(&req)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	claims := jwt.Get(ctx).(*utils.AccessToken)

	if req.SenderID != claims.ID {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}

	message := models.Message{
		ConversationID:  req.ConversationID,
		SenderID:        req.SenderID,
		ReceiverID:      req.ReceiverID,
		Text:            req.Text,
		Type:            req.Type,
		RefType:         req.RefType,
		RefID:           req.RefID,
		PreviewTitle:    req.PreviewTitle,
		PreviewSubtitle: req.PreviewSubtitle,
		PreviewImageURL: req.PreviewImageURL,
	}

	storage.DB.Create(&message)

	// Send push notification to receiver
	var sender models.User
	var receiver models.User
	if err := storage.DB.First(&sender, req.SenderID).Error; err == nil {
		if err := storage.DB.First(&receiver, req.ReceiverID).Error; err == nil {
			senderName := fmt.Sprintf("%s %s", sender.FirstName, sender.LastName)
			propertyTitle := "une propriété"

			// If message is about a property, get property title
			if req.RefType == "property" && req.RefID != nil {
				var property models.Property
				if err := storage.DB.First(&property, *req.RefID).Error; err == nil {
					propertyTitle = property.Title
				}
			}

			notificationService := services.NewNotificationService()
			go notificationService.SendMessageNotificationToHost(
				req.ReceiverID,
				req.SenderID,
				senderName,
				propertyTitle,
			)
		}
	}

	ctx.JSON(message)
}

type CreateMessageInput struct {
	ConversationID  uint   `json:"conversationID" validate:"required"`
	SenderID        uint   `json:"senderID" validate:"required"`
	ReceiverID      uint   `json:"receiverID" validate:"required"`
	Text            string `json:"text" validate:"lt=5000"`
	Type            string `json:"type" validate:"omitempty,oneof=text property_card"`
	RefType         string `json:"refType" validate:"omitempty,oneof=property"`
	RefID           *uint  `json:"refID"`
	PreviewTitle    string `json:"previewTitle"`
	PreviewSubtitle string `json:"previewSubtitle"`
	PreviewImageURL string `json:"previewImageURL"`
}

// ListMessages: GET /api/messages?conversationID=...&cursor=...&limit=...
func ListMessages(ctx iris.Context) {
	convID, err := ctx.URLParamInt("conversationID")
	if err != nil || convID <= 0 {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	limit, _ := ctx.URLParamInt("limit")
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	cursor, _ := ctx.URLParamInt("cursor")

	q := storage.DB.Where("conversation_id = ?", convID)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var msgs []models.Message
	if err := q.Order("id DESC").Limit(limit).Find(&msgs).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	nextCursor := 0
	if len(msgs) > 0 {
		nextCursor = int(msgs[0].ID)
	}
	ctx.JSON(iris.Map{"messages": msgs, "nextCursor": nextCursor})
}

type SetMessageStateInput struct {
	ConversationID uint   `json:"conversationID" validate:"required"`
	MessageIDs     []uint `json:"messageIDs" validate:"required"`
	State          string `json:"state" validate:"required,oneof=delivered seen"`
}

// SetMessageState: POST /api/messages/state
func SetMessageState(ctx iris.Context) {
	var req SetMessageStateInput
	if err := ctx.ReadJSON(&req); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}
	updates := map[string]any{"state": req.State}
	now := time.Now()
	if req.State == "delivered" {
		updates["delivered_at"] = now
	}
	if req.State == "seen" {
		updates["seen_at"] = now
	}
	if err := storage.DB.Model(&models.Message{}).
		Where("conversation_id = ? AND id IN ?", req.ConversationID, req.MessageIDs).
		Updates(updates).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"success": true})
}
