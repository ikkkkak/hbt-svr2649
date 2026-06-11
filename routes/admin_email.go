package routes

import (
	"net/http"
	"os"
	"strings"

	"apartments-clone-server/services"
	"apartments-clone-server/utils"

	"github.com/kataras/iris/v12"
)

type adminEmailTestRequest struct {
	To          string `json:"to"`
	ListingKind string `json:"listing_kind"`
	ListingID   uint   `json:"listing_id"`
}

// POST /admin/email/test
func AdminSendTestListingEmail(ctx iris.Context) {
	if !services.EmailConfigured() {
		utils.JSONError(ctx, http.StatusServiceUnavailable, "email_not_configured", "Set EMAIL_FROM and GMAIL_APP_PASSWORD on the server")
		return
	}

	var req adminEmailTestRequest
	if err := ctx.ReadJSON(&req); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_body", "invalid request body")
		return
	}

	kind := services.ListingKind(strings.TrimSpace(req.ListingKind))
	if req.ListingID == 0 {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_listing_id", "listing_id is required")
		return
	}
	switch kind {
	case services.ListingKindPropertySale, services.ListingKindRent, services.ListingKindLand:
	default:
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_listing_kind", "listing_kind must be property_sale, rent, or land")
		return
	}

	to := strings.TrimSpace(req.To)
	if to == "" {
		to = strings.TrimSpace(os.Getenv("ADMIN_NOTIFY_EMAIL"))
	}
	if to == "" {
		utils.JSONError(ctx, http.StatusBadRequest, "missing_recipient", "Provide to or set ADMIN_NOTIFY_EMAIL on the server")
		return
	}

	in, err := services.LoadListingAdminNotifyInput(kind, req.ListingID)
	if err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "listing_not_found", err.Error())
		return
	}

	ok, err := services.SendAdminListingEmail(to, in, true)
	if err != nil {
		utils.JSONError(ctx, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	if !ok {
		utils.JSONError(ctx, http.StatusBadGateway, "send_failed", "email was not sent")
		return
	}

	ctx.JSON(iris.Map{
		"ok":      true,
		"to":      to,
		"listing": iris.Map{"kind": kind, "id": in.ID, "title": in.Title},
	})
}
