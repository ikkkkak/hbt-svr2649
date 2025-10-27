package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

func requireGroupRole(groupID uint, userID uint, roles ...string) bool {
	var member models.GroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ? AND status = 'active'", groupID, userID).First(&member).Error; err != nil {
		return false
	}
	for _, r := range roles {
		if member.Role == r {
			return true
		}
	}
	return false
}

func isUserBlockedInGroup(groupID uint, userID uint) bool {
	var count int64
	storage.DB.Model(&models.GroupUserBlock{}).
		Where("group_id = ? AND blocked_id = ? AND deleted_at IS NULL", groupID, userID).
		Count(&count)
	return count > 0
}

// CreateGroup - owner becomes member with owner role
func CreateGroup(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		IsPublic    bool   `json:"is_public"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid json"})
		return
	}
	if body.Name == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "name required"})
		return
	}

	g := models.Group{Name: body.Name, Description: body.Description, IsPublic: body.IsPublic, OwnerID: uid}
	if err := storage.DB.Create(&g).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to create group"})
		return
	}
	gm := models.GroupMember{GroupID: g.ID, UserID: uid, Role: "owner", Status: "active"}
	storage.DB.Create(&gm)
	ctx.JSON(iris.Map{"group": g})
}

// MyGroups lists groups where user is a member
func MyGroups(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	var groups []models.Group
	if err := storage.DB.Joins("JOIN group_members gm ON gm.group_id = groups.id AND gm.user_id = ?", uid).Find(&groups).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed"})
		return
	}
	ctx.JSON(iris.Map{"groups": groups})
}

// PostMessage posts message if member and not banned
func PostMessage(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	gid, err := ctx.Params().GetUint("id")
	if err != nil || gid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	// banned check
	var ban models.GroupBan
	if err := storage.DB.Where("group_id = ? AND user_id = ? AND deleted_at IS NULL", gid, uid).First(&ban).Error; err == nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "banned"})
		return
	}
	// membership check
	if !requireGroupRole(gid, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Content == "" {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	msg := models.GroupMessage{GroupID: gid, UserID: uid, Content: body.Content}
	if err := storage.DB.Create(&msg).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"message": msg})
}

// GetMessages returns paginated messages
func GetMessages(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	gid, err := ctx.Params().GetUint("id")
	if err != nil || gid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	if !requireGroupRole(gid, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		return
	}
	limit := ctx.URLParamIntDefault("limit", 30)
	if limit < 1 || limit > 100 {
		limit = 30
	}
	before := ctx.URLParamIntDefault("before", 0)
	q := storage.DB.Where("group_id = ?", gid)
	if before > 0 {
		q = q.Where("id < ?", before)
	}
	var msgs []models.GroupMessage
	if err := q.Order("id DESC").Limit(limit).Find(&msgs).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"messages": msgs})
}

// CreateInvite generates expiring invite token
func CreateInvite(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	gid, err := ctx.Params().GetUint("id")
	if err != nil || gid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	if !requireGroupRole(gid, uid, "owner", "admin", "moderator") {
		ctx.StatusCode(http.StatusForbidden)
		return
	}
	// token
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)
	inv := models.GroupInvite{GroupID: gid, Token: token, ExpiresAt: time.Now().Add(72 * time.Hour), CreatedBy: uid}
	if err := storage.DB.Create(&inv).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"invite": iris.Map{"token": token, "expires_at": inv.ExpiresAt}})
}

// JoinByInvite consumes token
func JoinByInvite(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Token == "" {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	var inv models.GroupInvite
	if err := storage.DB.Where("token = ?", body.Token).First(&inv).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		return
	}
	if inv.UsedBy != nil || time.Now().After(inv.ExpiresAt) {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid invite"})
		return
	}
	// add membership if not exists
	var existing models.GroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", inv.GroupID, uid).First(&existing).Error; err != nil {
		storage.DB.Create(&models.GroupMember{GroupID: inv.GroupID, UserID: uid, Role: "member"})
	}
	now := time.Now()
	inv.UsedBy = &uid
	inv.UsedAt = &now
	storage.DB.Save(&inv)
	ctx.JSON(iris.Map{"ok": true})
}

// GetMessageReads returns users who read a message
func GetMessageReads(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	gid, err := ctx.Params().GetUint("id")
	if err != nil || gid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	mid, err := ctx.Params().GetUint("msgId")
	if err != nil || mid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	if !requireGroupRole(gid, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		return
	}
	type Row struct {
		UserID    uint
		FirstName string
		LastName  string
		AvatarURL string
		ReadAt    time.Time
	}
	var rows []Row
	q := storage.DB.Table("group_message_reads gmr").
		Select("gmr.user_id as user_id, users.first_name as first_name, users.last_name as last_name, users.avatar_url as avatar_url, gmr.read_at as read_at").
		Joins("JOIN users ON users.id = gmr.user_id").
		Where("gmr.group_id = ? AND gmr.message_id = ?", gid, mid).Order("gmr.read_at DESC")
	if err := q.Find(&rows).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"reads": rows})
}

// MarkMessageRead records that the current user read the message
func MarkMessageRead(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}
	gid, err := ctx.Params().GetUint("id")
	if err != nil || gid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	mid, err := ctx.Params().GetUint("msgId")
	if err != nil || mid == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		return
	}
	if !requireGroupRole(gid, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		return
	}
	// upsert by (group_id, user_id, message_id)
	rec := models.GroupMessageRead{GroupID: gid, UserID: uid, MessageID: mid, ReadAt: time.Now()}
	// try update existing first newer timestamp
	storage.DB.Where("group_id = ? AND user_id = ? AND message_id = ?", gid, uid, mid).Delete(&models.GroupMessageRead{})
	if err := storage.DB.Create(&rec).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		return
	}
	ctx.JSON(iris.Map{"ok": true})
}

// QuitGroup - User quits a group
func QuitGroup(ctx iris.Context) {
	fmt.Printf("🔍 QuitGroup called - Headers: %+v\n", ctx.Request().Header)
	fmt.Printf("🔍 QuitGroup called - Auth header: %s\n", ctx.GetHeader("Authorization"))

	uid, ok := ctx.Values().Get("userID").(uint)
	fmt.Printf("🔍 QuitGroup - UserID from context: %d, ok: %v\n", uid, ok)

	if !ok || uid == 0 {
		fmt.Printf("❌ QuitGroup - Unauthorized: uid=%d, ok=%v\n", uid, ok)
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid group id"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	ctx.ReadJSON(&body)

	// Check if user is a member
	var member models.GroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ? AND status = 'active'", groupID, uid).First(&member).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "not a member of this group"})
		return
	}

	// Check if user is the owner
	if member.Role == "owner" {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "group owner cannot quit. Transfer ownership first or delete the group"})
		return
	}

	// Update member status to quit
	now := time.Now()
	member.Status = "quit"
	member.QuitAt = &now
	if err := storage.DB.Save(&member).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to quit group"})
		return
	}

	// Create quit record
	quitRecord := models.GroupQuit{
		GroupID: groupID,
		UserID:  uid,
		Reason:  body.Reason,
		QuitAt:  now,
	}
	storage.DB.Create(&quitRecord)

	ctx.JSON(iris.Map{
		"message": "Successfully quit the group",
		"quit_at": now,
	})
}

// BlockUserInGroup - Block a user within a group
func BlockUserInGroup(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid group id"})
		return
	}

	blockedUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	if uid == blockedUserID {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "cannot block yourself"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	ctx.ReadJSON(&body)

	// Check if blocker is a member
	if !requireGroupRole(groupID, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "not a member of this group"})
		return
	}

	// Check if blocked user is a member
	if !requireGroupRole(groupID, blockedUserID, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "user is not a member of this group"})
		return
	}

	// Check if already blocked
	var existingBlock models.GroupUserBlock
	if err := storage.DB.Where("group_id = ? AND blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		groupID, uid, blockedUserID).First(&existingBlock).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "user is already blocked"})
		return
	}

	// Create block record
	blockRecord := models.GroupUserBlock{
		GroupID:   groupID,
		BlockerID: uid,
		BlockedID: blockedUserID,
		Reason:    body.Reason,
	}
	if err := storage.DB.Create(&blockRecord).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to block user"})
		return
	}

	ctx.JSON(iris.Map{
		"message":         "User blocked successfully",
		"blocked_user_id": blockedUserID,
	})
}

// UnblockUserInGroup - Unblock a user within a group
func UnblockUserInGroup(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid group id"})
		return
	}

	blockedUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	// Check if blocker is a member
	if !requireGroupRole(groupID, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "not a member of this group"})
		return
	}

	// Soft delete the block record
	if err := storage.DB.Where("group_id = ? AND blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		groupID, uid, blockedUserID).Delete(&models.GroupUserBlock{}).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "block record not found"})
		return
	}

	ctx.JSON(iris.Map{
		"message":           "User unblocked successfully",
		"unblocked_user_id": blockedUserID,
	})
}

// GetGroupQuitHistory - Get users who have quit the group
func GetGroupQuitHistory(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid group id"})
		return
	}

	// Check if user is a member (owner, admin, moderator can see quit history)
	if !requireGroupRole(groupID, uid, "owner", "admin", "moderator") {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "insufficient permissions"})
		return
	}

	var quits []struct {
		ID        uint      `json:"id"`
		UserID    uint      `json:"user_id"`
		UserName  string    `json:"user_name"`
		Reason    string    `json:"reason"`
		QuitAt    time.Time `json:"quit_at"`
		CreatedAt time.Time `json:"created_at"`
	}

	storage.DB.Table("group_quits").
		Select("group_quits.id, group_quits.user_id, users.name as user_name, group_quits.reason, group_quits.quit_at, group_quits.created_at").
		Joins("JOIN users ON users.id = group_quits.user_id").
		Where("group_quits.group_id = ? AND group_quits.deleted_at IS NULL", groupID).
		Order("group_quits.quit_at DESC").
		Scan(&quits)

	ctx.JSON(iris.Map{
		"quits": quits,
		"total": len(quits),
	})
}

// GetBlockedUsersInGroup - Get users blocked by current user in the group
func GetBlockedUsersInGroup(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid group id"})
		return
	}

	// Check if user is a member
	if !requireGroupRole(groupID, uid, "owner", "admin", "moderator", "member") {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "not a member of this group"})
		return
	}

	var blockedUsers []struct {
		ID        uint      `json:"id"`
		BlockedID uint      `json:"blocked_id"`
		UserName  string    `json:"user_name"`
		Reason    string    `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}

	storage.DB.Table("group_user_blocks").
		Select("group_user_blocks.id, group_user_blocks.blocked_id, users.name as user_name, group_user_blocks.reason, group_user_blocks.created_at").
		Joins("JOIN users ON users.id = group_user_blocks.blocked_id").
		Where("group_user_blocks.group_id = ? AND group_user_blocks.blocker_id = ? AND group_user_blocks.deleted_at IS NULL", groupID, uid).
		Order("group_user_blocks.created_at DESC").
		Scan(&blockedUsers)

	ctx.JSON(iris.Map{
		"blocked_users": blockedUsers,
		"total":         len(blockedUsers),
	})
}

// GenerateInviteCode - generate an invite code that expires in 5 minutes
func GenerateInviteCode(ctx iris.Context) {
	var uid uint

	// Try to get userID from context
	if userID, ok := ctx.Values().Get("userID").(uint); ok && userID > 0 {
		uid = userID
	} else if claims := jwt.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			uid = accessToken.ID
		}
	}

	if uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	groupID := ctx.Params().GetUintDefault("id", 0)
	fmt.Printf("🔍 GenerateInviteCode - GroupID: %d\n", groupID)
	if groupID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid group ID"})
		return
	}

	// Check if this is an experience group or regular group
	var expGroup models.ExperienceGroup
	var expMember models.ExperienceGroupMember
	isExperienceGroup := false

	// Try to find it in experience_groups first
	if err := storage.DB.Where("id = ?", groupID).First(&expGroup).Error; err == nil {
		isExperienceGroup = true
		fmt.Printf("🔍 GenerateInviteCode - Found experience group %d\n", groupID)
	} else {
		fmt.Printf("🔍 GenerateInviteCode - Group %d not found in experience_groups, checking regular groups\n", groupID)
	}

	if isExperienceGroup {
		// It's an experience group
		if err := storage.DB.Where("group_id = ? AND user_id = ? AND (state = 'joined' OR state = 'pending')", groupID, uid).First(&expMember).Error; err != nil {
			fmt.Printf("🔍 GenerateInviteCode - User %d is not a member of experience group %d: %v\n", uid, groupID, err)
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Only group members can create invite codes", "user_id": uid, "group_id": groupID})
			return
		}

		// Check if user is owner
		if expGroup.OwnerID != uid {
			fmt.Printf("🔍 GenerateInviteCode - User %d is not owner of experience group %d (owner: %d)\n", uid, groupID, expGroup.OwnerID)
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Only group owners can create invite codes", "user_id": uid, "group_id": groupID, "owner_id": expGroup.OwnerID})
			return
		}
	} else {
		// Check if user is owner or admin of regular group
		var member models.GroupMember
		if err := storage.DB.Where("group_id = ? AND user_id = ? AND status = 'active'", groupID, uid).First(&member).Error; err != nil {
			fmt.Printf("🔍 GenerateInviteCode - User %d is not a member of group %d: %v\n", uid, groupID, err)
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Only group admins can create invite codes", "user_id": uid, "group_id": groupID})
			return
		}

		// Check role
		hasPermission := false
		for _, role := range []string{"owner", "admin"} {
			if member.Role == role {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			fmt.Printf("🔍 GenerateInviteCode - User %d has role %s, requires owner or admin\n", uid, member.Role)
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Only group admins can create invite codes", "user_id": uid, "group_id": groupID, "user_role": member.Role})
			return
		}
	}

	// Generate a secure 10-character uppercase token
	tokenBytes := make([]byte, 5) // 5 bytes = 10 hex characters
	if _, err := rand.Read(tokenBytes); err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to generate invite code"})
		return
	}
	token := strings.ToUpper(hex.EncodeToString(tokenBytes))

	// Create invite with 5-minute expiry
	invite := models.GroupInvite{
		GroupID:   groupID,
		Token:     token,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		CreatedBy: uid,
	}

	if err := storage.DB.Create(&invite).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create invite code"})
		return
	}

	ctx.JSON(iris.Map{
		"code":       token,
		"expires_in": 300, // 5 minutes in seconds
		"expires_at": invite.ExpiresAt,
	})
}

// GetGroupByInviteCode - get group details for an invite code
func GetGroupByInviteCode(ctx iris.Context) {
	token := ctx.Params().GetStringDefault("code", "")
	if token == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invite code required"})
		return
	}

	var invite models.GroupInvite
	if err := storage.DB.Where("token = ? AND expires_at > ? AND used_by IS NULL AND deleted_at IS NULL", token, time.Now()).First(&invite).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Invalid or expired invite code"})
		return
	}

	// Try to get as experience group first
	var expGroup models.ExperienceGroup
	var group models.Group
	var memberCount int64
	var owner models.User
	var ownerID uint

	if err := storage.DB.Where("id = ?", invite.GroupID).First(&expGroup).Error; err == nil {
		// It's an experience group
		storage.DB.Model(&models.ExperienceGroupMember{}).
			Where("group_id = ? AND (state = 'joined' OR state = 'pending')", invite.GroupID).
			Count(&memberCount)

		ownerID = expGroup.OwnerID

		ctx.JSON(iris.Map{
			"group": iris.Map{
				"id":           expGroup.ID,
				"name":         expGroup.Name,
				"description":  "",
				"is_public":    expGroup.Privacy == "public",
				"member_count": memberCount,
				"owner": iris.Map{
					"id":   ownerID,
					"name": "Group Owner",
				},
			},
			"invite_code": token,
		})
		return
	}

	// It's a regular group
	if err := storage.DB.First(&group, invite.GroupID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Group not found"})
		return
	}

	// Get member count
	storage.DB.Model(&models.GroupMember{}).
		Where("group_id = ? AND status = 'active'", invite.GroupID).
		Count(&memberCount)

	// Get owner info
	storage.DB.First(&owner, group.OwnerID)

	ownerName := fmt.Sprintf("%s %s", owner.FirstName, owner.LastName)
	if ownerName == " " {
		ownerName = owner.Email
	}

	ctx.JSON(iris.Map{
		"group": iris.Map{
			"id":           group.ID,
			"name":         group.Name,
			"description":  group.Description,
			"is_public":    group.IsPublic,
			"member_count": memberCount,
			"owner": iris.Map{
				"id":   owner.ID,
				"name": ownerName,
			},
		},
		"invite_code": token,
	})
}

// JoinGroupWithCode - join a group using an invite code
func JoinGroupWithCode(ctx iris.Context) {
	var uid uint

	// Try to get userID from context
	if userID, ok := ctx.Values().Get("userID").(uint); ok && userID > 0 {
		uid = userID
	} else if claims := jwt.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok {
			uid = accessToken.ID
		}
	}

	if uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid request body"})
		return
	}

	if body.Code == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invite code required"})
		return
	}

	// Find valid invite
	var invite models.GroupInvite
	if err := storage.DB.Where("token = ? AND expires_at > ? AND used_by IS NULL AND deleted_at IS NULL", body.Code, time.Now()).First(&invite).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Invalid or expired invite code"})
		return
	}

	// Check if this is an experience group
	var expGroup models.ExperienceGroup
	if err := storage.DB.Where("id = ?", invite.GroupID).First(&expGroup).Error; err == nil {
		// It's an experience group - check if already a member
		var existingMember models.ExperienceGroupMember
		if err := storage.DB.Where("group_id = ? AND user_id = ? AND (state = 'joined' OR state = 'pending')", invite.GroupID, uid).First(&existingMember).Error; err == nil {
			ctx.StatusCode(http.StatusConflict)
			ctx.JSON(iris.Map{"error": "You are already a member of this group"})
			return
		}

		// Create experience group membership
		now := time.Now()
		member := models.ExperienceGroupMember{
			GroupID:  invite.GroupID,
			UserID:   uid,
			State:    "joined",
			Role:     "member",
			JoinedAt: &now,
		}

		if err := storage.DB.Create(&member).Error; err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to join group"})
			return
		}

		// Mark invite as used
		invite.UsedBy = &uid
		invite.UsedAt = &now
		storage.DB.Save(&invite)

		ctx.JSON(iris.Map{
			"success": true,
			"message": "Successfully joined the group",
			"group": iris.Map{
				"id":   expGroup.ID,
				"name": expGroup.Name,
			},
		})
		return
	}

	// It's a regular group - check if already a member
	var existingMember models.GroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ? AND status = 'active'", invite.GroupID, uid).First(&existingMember).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "You are already a member of this group"})
		return
	}

	// Check if user is banned from this group
	if isUserBlockedInGroup(invite.GroupID, uid) {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You are blocked from this group"})
		return
	}

	// Create group membership
	member := models.GroupMember{
		GroupID: invite.GroupID,
		UserID:  uid,
		Role:    "member",
		Status:  "active",
	}

	if err := storage.DB.Create(&member).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to join group"})
		return
	}

	// Mark invite as used
	now := time.Now()
	invite.UsedBy = &uid
	invite.UsedAt = &now
	storage.DB.Save(&invite)

	// Get group details to return
	var group models.Group
	storage.DB.First(&group, invite.GroupID)

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Successfully joined the group",
		"group": iris.Map{
			"id":   group.ID,
			"name": group.Name,
		},
	})
}
