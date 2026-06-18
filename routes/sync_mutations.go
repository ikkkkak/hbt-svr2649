package routes

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"

	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
)

type syncMutationItem struct {
	ClientMutationID string `json:"clientMutationId"`
	Action           string `json:"action"`
	EntityID         uint   `json:"entityId"`
}

type syncMutationResult struct {
	ClientMutationID string      `json:"clientMutationId"`
	Status           string      `json:"status"` // applied | duplicate | rejected
	Data             interface{} `json:"data,omitempty"`
	Error            string      `json:"error,omitempty"`
}

func applySaleVideoLike(userID, videoID uint) (map[string]interface{}, error) {
	var existing models.PropertySaleVideoLike
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existing).Error; err == nil {
		var likesCount int64
		storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)
		return map[string]interface{}{"success": true, "likesCount": likesCount, "liked": true}, nil
	}
	like := models.PropertySaleVideoLike{PropertySaleVideoID: videoID, UserID: userID}
	if err := storage.DB.Create(&like).Error; err != nil {
		return nil, err
	}
	psID := videoID
	services.InteractionServiceInstance().Record(services.InteractionInput{
		EntityType: models.EntityPropertySale, EntityID: videoID, PropertySaleID: &psID,
		EventType: models.EventLike, UserID: &userID,
	})
	var likesCount int64
	storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)
	return map[string]interface{}{"success": true, "likesCount": likesCount, "liked": true}, nil
}

func applySaleVideoUnlike(userID, videoID uint) (map[string]interface{}, error) {
	_ = storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).
		Delete(&models.PropertySaleVideoLike{}).Error
	var likesCount int64
	storage.DB.Model(&models.PropertySaleVideoLike{}).Where("property_sale_video_id = ?", videoID).Count(&likesCount)
	return map[string]interface{}{"success": true, "likesCount": likesCount, "liked": false}, nil
}

func applySaleVideoSave(userID, videoID uint) (map[string]interface{}, error) {
	var existing models.PropertySaleVideoSave
	if err := storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).First(&existing).Error; err == nil {
		var savesCount int64
		storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)
		return map[string]interface{}{"success": true, "savesCount": savesCount, "saved": true}, nil
	}
	save := models.PropertySaleVideoSave{PropertySaleVideoID: videoID, UserID: userID}
	if err := storage.DB.Create(&save).Error; err != nil {
		return nil, err
	}
	psID := videoID
	services.InteractionServiceInstance().Record(services.InteractionInput{
		EntityType: models.EntityPropertySale, EntityID: videoID, PropertySaleID: &psID,
		EventType: models.EventSave, UserID: &userID,
	})
	var savesCount int64
	storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)
	return map[string]interface{}{"success": true, "savesCount": savesCount, "saved": true}, nil
}

func applySaleVideoUnsave(userID, videoID uint) (map[string]interface{}, error) {
	_ = storage.DB.Where("property_sale_video_id = ? AND user_id = ?", videoID, userID).
		Delete(&models.PropertySaleVideoSave{}).Error
	var savesCount int64
	storage.DB.Model(&models.PropertySaleVideoSave{}).Where("property_sale_video_id = ?", videoID).Count(&savesCount)
	return map[string]interface{}{"success": true, "savesCount": savesCount, "saved": false}, nil
}

func lookupClientMutation(userID uint, clientMutationID string) (*models.ClientMutation, bool) {
	if clientMutationID == "" {
		return nil, false
	}
	var row models.ClientMutation
	if err := storage.DB.Where("client_mutation_id = ? AND user_id = ?", clientMutationID, userID).First(&row).Error; err != nil {
		return nil, false
	}
	return &row, true
}

func storeClientMutation(userID uint, clientMutationID, action string, entityID uint, payload map[string]interface{}) {
	if clientMutationID == "" {
		return
	}
	b, _ := json.Marshal(payload)
	_ = storage.DB.Create(&models.ClientMutation{
		ClientMutationID: clientMutationID,
		UserID:           userID,
		Action:           action,
		EntityID:         entityID,
		ResponseJSON:     string(b),
	}).Error
}

func dispatchSyncMutation(userID uint, item syncMutationItem) syncMutationResult {
	clientID := strings.TrimSpace(item.ClientMutationID)
	action := strings.TrimSpace(strings.ToLower(item.Action))
	if item.EntityID == 0 {
		return syncMutationResult{ClientMutationID: clientID, Status: "rejected", Error: "entityId required"}
	}

	if row, ok := lookupClientMutation(userID, clientID); ok && clientID != "" {
		var data interface{}
		_ = json.Unmarshal([]byte(row.ResponseJSON), &data)
		return syncMutationResult{ClientMutationID: clientID, Status: "duplicate", Data: data}
	}

	var (
		data map[string]interface{}
		err  error
	)
	switch action {
	case "sale_video_like":
		data, err = applySaleVideoLike(userID, item.EntityID)
	case "sale_video_unlike":
		data, err = applySaleVideoUnlike(userID, item.EntityID)
	case "sale_video_save":
		data, err = applySaleVideoSave(userID, item.EntityID)
	case "sale_video_unsave":
		data, err = applySaleVideoUnsave(userID, item.EntityID)
	default:
		return syncMutationResult{ClientMutationID: clientID, Status: "rejected", Error: "unknown action"}
	}
	if err != nil {
		return syncMutationResult{ClientMutationID: clientID, Status: "rejected", Error: err.Error()}
	}

	storeClientMutation(userID, clientID, action, item.EntityID, data)
	return syncMutationResult{ClientMutationID: clientID, Status: "applied", Data: data}
}

// PostSyncMutations applies batched idempotent writes from the mobile offline queue.
// POST /api/sync/mutations
func PostSyncMutations(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var body struct {
		Mutations []syncMutationItem `json:"mutations"`
	}
	if err := ctx.ReadJSON(&body); err != nil || len(body.Mutations) == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "mutations required"})
		return
	}
	if len(body.Mutations) > 25 {
		body.Mutations = body.Mutations[:25]
	}

	results := make([]syncMutationResult, 0, len(body.Mutations))
	for _, m := range body.Mutations {
		results = append(results, dispatchSyncMutation(userID, m))
	}

	utils.RespondJSONWithETag(ctx, http.StatusOK, iris.Map{
		"results": results,
		"count":   len(results),
	})
}
