package services

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

const meskenyTeamEmail = "meskeny.team@meskeny.internal"

var (
	meskenyTeamMu   sync.Mutex
	meskenyTeamID   uint
	meskenyTeamInit bool
)

// MeskenyTeamDisplayName is the canonical English label; clients localize when is_meskeny_team is true.
func MeskenyTeamDisplayName() string {
	return "Meskeny Team"
}

// MeskenyTeamAvatarURL optional logo for push/inbox; clients may override with bundled asset.
func MeskenyTeamAvatarURL() string {
	if v := strings.TrimSpace(os.Getenv("MESKENY_TEAM_AVATAR_URL")); v != "" {
		return v
	}
	return ""
}

// EnsureMeskenyTeamUser returns the system account used for official admin → user messages.
func EnsureMeskenyTeamUser() (uint, error) {
	meskenyTeamMu.Lock()
	defer meskenyTeamMu.Unlock()

	if meskenyTeamInit && meskenyTeamID > 0 {
		return meskenyTeamID, nil
	}

	if v := strings.TrimSpace(os.Getenv("MESKENY_TEAM_USER_ID")); v != "" {
		if id, err := strconv.ParseUint(v, 10, 32); err == nil && id > 0 {
			var u models.User
			if err := storage.DB.Unscoped().First(&u, uint(id)).Error; err == nil {
				if u.DeletedAt.Valid {
					_ = storage.DB.Unscoped().Model(&u).Update("deleted_at", nil).Error
					log.Printf("♻️ Restored soft-deleted Meskeny Team user id=%d", u.ID)
				}
				meskenyTeamID = u.ID
				meskenyTeamInit = true
				return meskenyTeamID, nil
			}
			log.Printf("⚠️ MESKENY_TEAM_USER_ID=%d not found — falling back to system account", id)
		}
	}

	var user models.User
	err := storage.DB.Where("LOWER(email) = ?", strings.ToLower(meskenyTeamEmail)).First(&user).Error
	if err != nil {
		allows := true
		verified := true
		user = models.User{
			FirstName:           "Meskeny",
			LastName:            "Team",
			Email:               meskenyTeamEmail,
			Password:            randomHex(24),
			Role:                "system",
			AvatarURL:           MeskenyTeamAvatarURL(),
			AllowsNotifications: &allows,
			IsVerified:          &verified,
			VerificationStatus:  "verified",
		}
		if err := storage.DB.Create(&user).Error; err != nil {
			return 0, err
		}
		log.Printf("✅ Created Meskeny Team system user id=%d", user.ID)
	}

	meskenyTeamID = user.ID
	meskenyTeamInit = true
	return meskenyTeamID, nil
}

func IsMeskenyTeamUser(userID uint) bool {
	if userID == 0 {
		return false
	}
	teamID, err := EnsureMeskenyTeamUser()
	if err == nil && teamID > 0 && userID == teamID {
		return true
	}
	var u models.User
	if err := storage.DB.Unscoped().First(&u, userID).Error; err == nil {
		return strings.EqualFold(strings.TrimSpace(u.Email), meskenyTeamEmail)
	}
	return false
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "meskeny-team-system"
	}
	return hex.EncodeToString(b)
}
