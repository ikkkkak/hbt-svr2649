package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strings"
	"time"
)

const (
	// MaxPendingBuyerMatchesPerProperty caps how many buyers appear per listing.
	MaxPendingBuyerMatchesPerProperty = 5
)

// UserConsentedToHostShare is true only when the buyer explicitly opted in.
func UserConsentedToHostShare(u *models.User) bool {
	return u != nil && u.ShareProfileWithHosts != nil && *u.ShareProfileWithHosts
}

// UserLockedToAnotherHost is true when the buyer is already assigned to a different host.
func UserLockedToAnotherHost(u *models.User, hostID uint) bool {
	if u == nil || u.HostShareLockedHostID == nil || *u.HostShareLockedHostID == 0 {
		return false
	}
	return *u.HostShareLockedHostID != hostID
}

// MinimalBuyerLabel exposes only a first-name style label to hosts (no phone/email).
func MinimalBuyerLabel(u models.User) string {
	first := strings.TrimSpace(u.FirstName)
	if first != "" {
		return first
	}
	return "Buyer"
}

// TryLockBuyerToHost sets exclusive host lock on first share (idempotent for same host).
func TryLockBuyerToHost(buyerID, hostID uint) error {
	if buyerID == 0 || hostID == 0 {
		return nil
	}
	return storage.DB.Model(&models.User{}).
		Where("id = ? AND (host_share_locked_host_id IS NULL OR host_share_locked_host_id = 0)", buyerID).
		Update("host_share_locked_host_id", hostID).Error
}

// ClearHostShareForBuyer revokes consent side-effects: pending matches hidden from hosts.
func ClearHostShareForBuyer(buyerID uint) error {
	if buyerID == 0 {
		return nil
	}
	_ = storage.DB.Model(&models.PropertyMatch{}).
		Where("suggested_user_id = ? AND status = ?", buyerID, "pending").
		Update("status", "expired").Error
	return storage.DB.Model(&models.User{}).Where("id = ?", buyerID).Updates(map[string]any{
		"host_share_locked_host_id": nil,
	}).Error
}

// TrimPropertyPendingMatches keeps only the top N pending rows by score per property.
func TrimPropertyPendingMatches(propertyID, hostID uint, keep int) error {
	if keep <= 0 {
		return nil
	}
	var ids []uint
	if err := storage.DB.Model(&models.PropertyMatch{}).
		Select("id").
		Where("property_id = ? AND host_id = ? AND status = ?", propertyID, hostID, "pending").
		Order("match_score DESC, updated_at DESC").
		Offset(keep).
		Limit(500).
		Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	return storage.DB.Model(&models.PropertyMatch{}).
		Where("id IN ?", ids).
		Update("status", "expired").Error
}

// LoadSharePrefs loads consent + lock fields for many users in one query.
func LoadSharePrefs(userIDs []uint) (map[uint]models.User, error) {
	out := map[uint]models.User{}
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []models.User
	if err := storage.DB.
		Select("id", "first_name", "share_profile_with_hosts", "host_share_locked_host_id").
		Where("id IN ?", userIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, u := range rows {
		out[u.ID] = u
	}
	return out, nil
}

// ConsentDecidedAt is a helper for API responses when consent pointer is non-nil.
func ConsentDecidedAt(u *models.User) *time.Time {
	if u == nil || u.ShareProfileWithHosts == nil {
		return nil
	}
	t := u.UpdatedAt
	return &t
}
