package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"net/http"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

type sharePropertyInput struct {
	PropertyID uint `json:"propertyID"`
}

// Share a property to a group's chat as a rich preview message
func SharePropertyToGroup(ctx iris.Context) {
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

	// membership check
	var m models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, user.ID).First(&m).Error; err != nil {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var input sharePropertyInput
	if err := ctx.ReadJSON(&input); err != nil || input.PropertyID == 0 {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Build preview from property
	var p models.Property
	if err := storage.DB.First(&p, input.PropertyID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	previewTitle := p.Title
	previewSubtitle := p.City
	previewImage := ""
	if p.Images != "" {
		var imgs []string
		if jsonErr := json.Unmarshal([]byte(p.Images), &imgs); jsonErr == nil && len(imgs) > 0 {
			previewImage = imgs[0]
		}
	}
	previewDesc := p.Description

	msg := models.ChatMessage{
		GroupID:            groupID,
		SenderID:           user.ID,
		Content:            "shared a property",
		Type:               "share",
		RefType:            "property",
		RefID:              &input.PropertyID,
		PreviewTitle:       previewTitle,
		PreviewSubtitle:    previewSubtitle,
		PreviewImageURL:    previewImage,
		PreviewDescription: previewDesc,
	}
	if err := storage.DB.Create(&msg).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true, "message": msg})
}
