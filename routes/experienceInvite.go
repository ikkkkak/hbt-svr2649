package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

func generateShortToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

type CreateInvitesInput struct {
	InviteeUserIDs []uint `json:"inviteeUserIDs"`
	CreateLink     bool   `json:"createLink"`
	ExpiresInHours int    `json:"expiresInHours"`
}

func CreateExperienceInvites(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	userToken := tok.(*utils.AccessToken)
	experienceID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithError(http.StatusBadRequest, err)
		return
	}

	var input CreateInvitesInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithError(http.StatusBadRequest, err)
		return
	}

	// Optional expiration
	var expiresAt *time.Time
	if input.ExpiresInHours > 0 {
		exp := time.Now().Add(time.Duration(input.ExpiresInHours) * time.Hour)
		expiresAt = &exp
	}

	tx := storage.DB

	// Create user-targeted invites (idempotent on pending)
	for _, inviteeID := range input.InviteeUserIDs {
		invite := models.ExperienceInvite{
			ExperienceID:  experienceID,
			InviterID:     userToken.ID,
			InviteeUserID: &inviteeID,
			Status:        "pending",
			ExpiresAt:     expiresAt,
		}
		// Use map to avoid writing empty LinkToken that breaks unique index
		tx.Where("experience_id = ? AND inviter_id = ? AND invitee_user_id = ? AND status = ?", experienceID, userToken.ID, inviteeID, "pending").
			Attrs(map[string]interface{}{"link_token": nil}).
			FirstOrCreate(&invite)
	}

	// Optionally create a link invite
	var linkToken string
	if input.CreateLink {
		token := generateShortToken(24)
		invite := models.ExperienceInvite{
			ExperienceID: experienceID,
			InviterID:    userToken.ID,
			LinkToken:    &token,
			Status:       "pending",
			ExpiresAt:    expiresAt,
		}
		if err := tx.Create(&invite).Error; err == nil {
			linkToken = token
		}
	}

	ctx.JSON(iris.Map{"success": true, "linkToken": linkToken})
}

func ListInvites(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	userToken := tok.(*utils.AccessToken)
	var invites []models.ExperienceInvite
	storage.DB.
		Preload("Inviter").
		Preload("Invitee").
		Where("inviter_id = ? OR invitee_user_id = ?", userToken.ID, userToken.ID).
		Order("created_at DESC").
		Find(&invites)
	ctx.JSON(iris.Map{"success": true, "invites": invites})
}

func AcceptInvite(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	userToken := tok.(*utils.AccessToken)
	inviteID, err := ctx.Params().GetUint("inviteID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	var invite models.ExperienceInvite
	if err := storage.DB.First(&invite, inviteID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Ownership (invitee or link-based accept)
	if invite.InviteeUserID != nil && *invite.InviteeUserID != userToken.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	// Capacity check
	var exp models.Experience
	storage.DB.First(&exp, invite.ExperienceID)
	var count int64
	storage.DB.Model(&models.ExperienceParticipant{}).Where("experience_id = ? AND status = ?", exp.ID, "joined").Count(&count)
	if int(count) >= exp.GroupSize {
		ctx.JSON(iris.Map{"success": false, "error": "full"})
		return
	}

	// Mark accepted and add participant
	storage.DB.Model(&invite).Updates(map[string]interface{}{"status": "accepted"})
	participant := models.ExperienceParticipant{ExperienceID: exp.ID, UserID: userToken.ID, Status: "joined", JoinedAt: time.Now()}
	storage.DB.Where("experience_id = ? AND user_id = ?", exp.ID, userToken.ID).FirstOrCreate(&participant)

	// Also join inviter's latest group for this experience so chat becomes visible
	var grp models.ExperienceGroup
	if err := storage.DB.
		Where("experience_id = ? AND owner_id = ? AND status IN ?", exp.ID, invite.InviterID, []string{"pending", "active", "ready"}).
		Order("created_at DESC").
		First(&grp).Error; err == nil {
		now := time.Now()
		// upsert membership
		var existing models.ExperienceGroupMember
		if err := storage.DB.Where("group_id = ? AND user_id = ?", grp.ID, userToken.ID).First(&existing).Error; err != nil {
			storage.DB.Create(&models.ExperienceGroupMember{GroupID: grp.ID, UserID: userToken.ID, State: "joined", JoinedAt: &now})
		} else {
			storage.DB.Model(&existing).Updates(map[string]interface{}{"state": "joined", "joined_at": &now, "left_at": nil})
		}
	}

	ctx.JSON(iris.Map{"success": true})
}

func DeclineInvite(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	userToken := tok.(*utils.AccessToken)
	inviteID, err := ctx.Params().GetUint("inviteID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var invite models.ExperienceInvite
	if err := storage.DB.First(&invite, inviteID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if invite.InviteeUserID != nil && *invite.InviteeUserID != userToken.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	storage.DB.Model(&invite).Updates(map[string]interface{}{"status": "declined"})
	ctx.JSON(iris.Map{"success": true})
}

func CancelInvite(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	userToken := tok.(*utils.AccessToken)
	inviteID, err := ctx.Params().GetUint("inviteID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var invite models.ExperienceInvite
	if err := storage.DB.First(&invite, inviteID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if invite.InviterID != userToken.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	storage.DB.Model(&invite).Updates(map[string]interface{}{"status": "cancelled"})
	ctx.JSON(iris.Map{"success": true})
}

func ListParticipants(ctx iris.Context) {
	experienceID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var participants []models.ExperienceParticipant
	storage.DB.Where("experience_id = ? AND status = ?", experienceID, "joined").Preload("User").Find(&participants)
	ctx.JSON(iris.Map{"success": true, "participants": participants})
}

func RemoveParticipant(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	userToken := tok.(*utils.AccessToken)
	experienceID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	targetUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Ensure requester is host of the experience
	var exp models.Experience
	if err := storage.DB.First(&exp, experienceID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if exp.HostID != userToken.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	// Mark participant removed
	storage.DB.Model(&models.ExperienceParticipant{}).
		Where("experience_id = ? AND user_id = ? AND status = ?", experienceID, targetUserID, "joined").
		Updates(map[string]interface{}{"status": "removed", "left_at": time.Now()})

	ctx.JSON(iris.Map{"success": true})
}
