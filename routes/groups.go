package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
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
	gm := models.GroupMember{GroupID: g.ID, UserID: uid, Role: "owner"}
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
