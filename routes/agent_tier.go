package routes

import (
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// resolveAgentTier maps client hints + account to agent feature tier.
func resolveAgentTier(userID uint, persona, requestedTier string) string {
	req := strings.ToLower(strings.TrimSpace(requestedTier))
	if userID == 0 {
		return "anon"
	}
	if req == "pro" || req == "enterprise" {
		return req
	}
	p := strings.ToLower(strings.TrimSpace(persona))
	if req == "broker" || p == "broker" {
		return "broker"
	}
	// Auto-upgrade hosts with org or listings to broker tools.
	var orgOwned int64
	_ = storage.DB.Model(&models.Organization{}).Where("owner_id = ?", userID).Count(&orgOwned).Error
	if orgOwned > 0 {
		return "broker"
	}
	var member int64
	_ = storage.DB.Model(&models.OrganizationMember{}).
		Where("user_id = ? AND status = ? AND is_active = ? AND deleted_at IS NULL", userID, "active", true).
		Count(&member).Error
	if member > 0 {
		return "broker"
	}
	var saleCount int64
	_ = storage.DB.Model(&models.PropertySale{}).
		Where("owner_id = ? AND COALESCE(is_deactivated, false) = ?", userID, false).
		Count(&saleCount).Error
	if saleCount > 0 {
		return "broker"
	}
	return "free"
}
