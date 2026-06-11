package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"net/http"

	"github.com/kataras/iris/v12"
)

type hostShareConsentInput struct {
	ShareProfileWithHosts bool `json:"shareProfileWithHosts"`
}

// GetHostShareConsent returns the logged-in buyer's host-share preference.
func GetHostShareConsent(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithJSON(http.StatusUnauthorized, iris.Map{"error": "unauthorized"})
		return
	}
	var u models.User
	if err := storage.DB.Select(
		"id", "share_profile_with_hosts", "host_share_locked_host_id", "updated_at",
	).First(&u, userID).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "user not found"})
		return
	}
	accepted := false
	decided := u.ShareProfileWithHosts != nil
	if decided {
		accepted = *u.ShareProfileWithHosts
	}
	var lockedHost *uint
	if u.HostShareLockedHostID != nil && *u.HostShareLockedHostID > 0 {
		lockedHost = u.HostShareLockedHostID
	}
	ctx.JSON(iris.Map{
		"success":                  true,
		"share_profile_with_hosts": accepted,
		"has_decided":              decided,
		"locked_host_id":           lockedHost,
		"max_hosts_per_buyer":      1,
		"max_buyers_per_property":  services.MaxPendingBuyerMatchesPerProperty,
	})
}

// PutHostShareConsent stores opt-in or opt-out (logged-in buyers only).
func PutHostShareConsent(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StopWithJSON(http.StatusUnauthorized, iris.Map{"error": "unauthorized"})
		return
	}
	var body hostShareConsentInput
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid body"})
		return
	}

	share := body.ShareProfileWithHosts
	updates := map[string]any{
		"share_profile_with_hosts": share,
	}
	if !share {
		updates["host_share_locked_host_id"] = nil
	}

	if err := storage.DB.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to save preference"})
		return
	}
	if !share {
		_ = services.ClearHostShareForBuyer(userID)
	}

	var lockedHost *uint
	if share {
		var u models.User
		if err := storage.DB.Select("host_share_locked_host_id").First(&u, userID).Error; err == nil &&
			u.HostShareLockedHostID != nil && *u.HostShareLockedHostID > 0 {
			lockedHost = u.HostShareLockedHostID
		}
	}

	ctx.JSON(iris.Map{
		"success":                  true,
		"share_profile_with_hosts": share,
		"has_decided":              true,
		"locked_host_id":           lockedHost,
	})
}
