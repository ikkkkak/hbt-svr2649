package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/utils"
	"log"

	"github.com/kataras/iris/v12"
)

// TrackInteraction records an interaction (append-only). Optional auth; DeviceID supported for anonymous.
func TrackInteraction(ctx iris.Context) {
	var input services.InteractionInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Resolve UserID from context if not in body
	if input.UserID == nil || *input.UserID == 0 {
		if uid, ok := ctx.Values().Get("userID").(uint); ok && uid > 0 {
			input.UserID = &uid
		}
	}

	// Require at least one identifier
	if (input.UserID == nil || *input.UserID == 0) && (input.DeviceID == nil || *input.DeviceID == "") {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "userId or deviceId required"})
		return
	}

	// Validate entity and event types
	entityOk := input.EntityType == models.EntityVideo || input.EntityType == models.EntityPropertySaleVideo ||
		input.EntityType == models.EntityProperty || input.EntityType == models.EntityPropertySale
	eventOk := input.EventType == models.EventVideoView || input.EventType == models.EventPropertyView ||
		input.EventType == models.EventLike || input.EventType == models.EventSave ||
		input.EventType == models.EventShare || input.EventType == models.EventMessageOwner ||
		input.EventType == models.EventBookingAttempt
	if !entityOk || !eventOk {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid entityType or eventType"})
		return
	}

	svc := services.InteractionServiceInstance()
	if err := svc.Record(input); err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	log.Printf("✅ Interaction recorded: entity=%s entityId=%d event=%s", input.EntityType, input.EntityID, input.EventType)
	ctx.JSON(iris.Map{"success": true})
}
