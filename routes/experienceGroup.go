package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

type createGroupInput struct {
	Name        string `json:"name"`
	ExpiresInHr int    `json:"expiresInHr"`
	Privacy     string `json:"privacy"` // public | private
}

// Create or open an existing pending group for the user for the given experience
func CreateOrOpenGroup(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	experienceID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// If user already owns a pending group for this experience, reuse it
	var existing models.ExperienceGroup
	if err := storage.DB.Where("experience_id = ? AND owner_id = ? AND status = ?", experienceID, user.ID, "pending").First(&existing).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "group": existing})
		return
	}

	var input createGroupInput
	_ = ctx.ReadJSON(&input)

	var expiresAt *time.Time
	if input.ExpiresInHr > 0 {
		exp := time.Now().Add(time.Duration(input.ExpiresInHr) * time.Hour)
		expiresAt = &exp
	}

	privacy := input.Privacy
	if privacy != "private" {
		privacy = "public"
	}

	grp := models.ExperienceGroup{
		ExperienceID: &experienceID,
		OwnerID:      user.ID,
		Name:         input.Name,
		Status:       "pending",
		Privacy:      privacy,
		ExpiresAt:    expiresAt,
	}
	if err := storage.DB.Create(&grp).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	// Owner joins immediately as joined
	now := time.Now()
	storage.DB.Create(&models.ExperienceGroupMember{GroupID: grp.ID, UserID: user.ID, State: "joined", JoinedAt: &now})

	ctx.JSON(iris.Map{"success": true, "group": grp})
}

// List groups (owned or member)
func ListMyGroups(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var groups []models.ExperienceGroup
	storage.DB.
		Joins("JOIN experience_group_members m ON m.group_id = experience_groups.id").
		Where("m.user_id = ? AND m.state = ?", user.ID, "joined").
		Preload("Experience").
		Preload("Members", "state = ?", "joined").
		Preload("Members.User").
		Find(&groups)

	ctx.JSON(iris.Map{"success": true, "groups": groups})
}

// Get members and their states
func GetGroupMembers(ctx iris.Context) {
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var members []models.ExperienceGroupMember
	storage.DB.Where("group_id = ?", groupID).Preload("User").Find(&members)
	ctx.JSON(iris.Map{"success": true, "members": members})
}

// Join (accept) pending invite flows will call AcceptInvite which adds ExperienceParticipant.
// This endpoint allows direct join by link in the future. For now keep simple.
func LeaveGroup(ctx iris.Context) {
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
	now := time.Now()
	storage.DB.Model(&models.ExperienceGroupMember{}).Where("group_id = ? AND user_id = ? AND state = ?", groupID, user.ID, "joined").Updates(map[string]interface{}{"state": "left", "left_at": &now})
	ctx.JSON(iris.Map{"success": true})
}

// Finalize when full
func FinalizeGroup(ctx iris.Context) {
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

	var grp models.ExperienceGroup
	if err := storage.DB.Preload("Experience").First(&grp, groupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if grp.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}
	// Count joined members
	var joined int64
	storage.DB.Model(&models.ExperienceGroupMember{}).Where("group_id = ? AND state = ?", groupID, "joined").Count(&joined)
	if int(joined) < grp.Experience.GroupSize {
		ctx.JSON(iris.Map{"success": false, "error": "not_full"})
		return
	}
	storage.DB.Model(&grp).Update("status", "ready")
	ctx.JSON(iris.Map{"success": true, "group": grp})
}

type updateGroupInput struct {
	Name        *string `json:"name"`
	Status      *string `json:"status"`
	ExpiresInHr *int    `json:"expiresInHr"`
	PhotoURL    *string `json:"photoURL"`
	Privacy     *string `json:"privacy"`
}

// UpdateGroup allows the owner to update name, status, or expiry
func UpdateGroup(ctx iris.Context) {
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

	var grp models.ExperienceGroup
	if err := storage.DB.First(&grp, groupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if grp.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var input updateGroupInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	updates := map[string]interface{}{}
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.Status != nil {
		updates["status"] = *input.Status
	}
	// Allow updating photo independently from expiresInHr
	if input.PhotoURL != nil {
		updates["photo_url"] = *input.PhotoURL
	}
	if input.Privacy != nil {
		privacy := *input.Privacy
		if privacy != "private" {
			privacy = "public"
		}
		updates["privacy"] = privacy
	}
	if input.ExpiresInHr != nil {
		if *input.ExpiresInHr <= 0 {
			updates["expires_at"] = nil
		} else {
			exp := time.Now().Add(time.Duration(*input.ExpiresInHr) * time.Hour)
			updates["expires_at"] = &exp
		}
	}
	if len(updates) > 0 {
		if err := storage.DB.Model(&grp).Updates(updates).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}
	}

	ctx.JSON(iris.Map{"success": true, "group": grp})
}

type updateMemberRoleInput struct {
	Role string `json:"role"`
}

// Update member role (owner-only)
func UpdateMemberRole(ctx iris.Context) {
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
	memberID, err := ctx.Params().GetUint("memberID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	var grp models.ExperienceGroup
	if err := storage.DB.First(&grp, groupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if grp.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var input updateMemberRoleInput
	if err := ctx.ReadJSON(&input); err != nil || input.Role == "" {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	if err := storage.DB.Model(&models.ExperienceGroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, memberID).
		Update("role", input.Role).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"success": true})
}

// Remove guest (owner-only)
func RemoveGuest(ctx iris.Context) {
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
	memberID, err := ctx.Params().GetUint("memberID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	var grp models.ExperienceGroup
	if err := storage.DB.First(&grp, groupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if grp.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	if err := storage.DB.Where("group_id = ? AND user_id = ?", groupID, memberID).Delete(&models.ExperienceGroupMember{}).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"success": true})
}

// DeleteGroup allows the owner to delete the group and its memberships
func DeleteGroup(ctx iris.Context) {
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

	var grp models.ExperienceGroup
	if err := storage.DB.First(&grp, groupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if grp.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	// Delete members first, then group
	if err := storage.DB.Where("group_id = ?", groupID).Delete(&models.ExperienceGroupMember{}).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}
	if err := storage.DB.Delete(&grp).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{"success": true})
}
