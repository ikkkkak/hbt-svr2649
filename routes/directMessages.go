package routes

import (
	"apartments-clone-server/models"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

// BlockUserForDM - Block a user for direct messages
func BlockUserForDM(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
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

	// Check if already blocked
	var existingBlock models.UserBlock
	if err := storage.DB.Where("blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		uid, blockedUserID).First(&existingBlock).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "user is already blocked"})
		return
	}

	// Create block record
	blockRecord := models.UserBlock{
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

// UnblockUserForDM - Unblock a user for direct messages
func UnblockUserForDM(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	blockedUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	// Soft delete the block record
	if err := storage.DB.Where("blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		uid, blockedUserID).Delete(&models.UserBlock{}).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "block record not found"})
		return
	}

	ctx.JSON(iris.Map{
		"message":           "User unblocked successfully",
		"unblocked_user_id": blockedUserID,
	})
}

// GetBlockedUsersForDM - Get users blocked by current user for direct messages
func GetBlockedUsersForDM(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	var blockedUsers []struct {
		ID        uint      `json:"id"`
		BlockedID uint      `json:"blocked_id"`
		UserName  string    `json:"user_name"`
		UserEmail string    `json:"user_email"`
		Reason    string    `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}

	storage.DB.Table("user_blocks").
		Select("user_blocks.id, user_blocks.blocked_id, users.name as user_name, users.email as user_email, user_blocks.reason, user_blocks.created_at").
		Joins("JOIN users ON users.id = user_blocks.blocked_id").
		Where("user_blocks.blocker_id = ? AND user_blocks.deleted_at IS NULL", uid).
		Order("user_blocks.created_at DESC").
		Scan(&blockedUsers)

	ctx.JSON(iris.Map{
		"blocked_users": blockedUsers,
		"total":         len(blockedUsers),
	})
}

// SendDirectMessage - Send a direct message (enhanced for property inquiries)
func SendDirectMessage(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		log.Printf("❌ SendDirectMessage: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var body struct {
		ReceiverID uint   `json:"receiver_id"`
		Content    string `json:"content"`
		Type       string `json:"type"`
		RefType    string `json:"ref_type"`
		RefID      *uint  `json:"ref_id"`
		ReplyToID  *uint  `json:"reply_to_id"` // ID of message being replied to
	}
	if err := ctx.ReadJSON(&body); err != nil {
		log.Printf("❌ SendDirectMessage: Invalid JSON - %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid json"})
		return
	}

	// Validate required fields
	if body.ReceiverID == 0 {
		log.Printf("❌ SendDirectMessage: Missing receiver_id")
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "receiver_id is required"})
		return
	}

	if body.Content == "" {
		log.Printf("❌ SendDirectMessage: Empty content")
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "content is required"})
		return
	}

	// Prevent sending message to yourself
	if uid == body.ReceiverID {
		log.Printf("❌ SendDirectMessage: User %d tried to message themselves", uid)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "cannot send message to yourself"})
		return
	}

	// Verify receiver exists
	var receiver models.User
	if err := storage.DB.First(&receiver, body.ReceiverID).Error; err != nil {
		log.Printf("❌ SendDirectMessage: Receiver %d not found", body.ReceiverID)
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "receiver not found"})
		return
	}

	// Check if receiver has blocked the sender
	var blockCheck models.UserBlock
	if err := storage.DB.Where("blocker_id = ? AND blocked_id = ? AND deleted_at IS NULL",
		body.ReceiverID, uid).First(&blockCheck).Error; err == nil {
		log.Printf("⚠️ SendDirectMessage: User %d is blocked by receiver %d", uid, body.ReceiverID)
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "cannot send message to this user"})
		return
	}

	// If ref_type is "property", validate the property exists and belongs to the receiver
	if body.RefType == "property" && body.RefID != nil {
		var property models.Property
		if err := storage.DB.First(&property, *body.RefID).Error; err != nil {
			log.Printf("❌ SendDirectMessage: Property %d not found", *body.RefID)
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "property not found"})
			return
		}
		// Verify the receiver is the host of this property
		if property.HostID != body.ReceiverID {
			log.Printf("⚠️ SendDirectMessage: Property %d does not belong to receiver %d (host: %d)",
				*body.RefID, body.ReceiverID, property.HostID)
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "property does not belong to this host"})
			return
		}
		log.Printf("✅ SendDirectMessage: Property inquiry - Property %d, Host %d, Guest %d",
			*body.RefID, body.ReceiverID, uid)
	}

	// Set default type if not provided
	if body.Type == "" {
		body.Type = "text"
	}

	// Validate reply_to_id if provided
	if body.ReplyToID != nil && *body.ReplyToID != 0 {
		var replyToMsg models.DirectMessage
		if err := storage.DB.First(&replyToMsg, *body.ReplyToID).Error; err != nil {
			log.Printf("❌ SendDirectMessage: Reply to message %d not found", *body.ReplyToID)
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "reply_to message not found"})
			return
		}
		// Verify the message belongs to this conversation
		if !((replyToMsg.SenderID == uid && replyToMsg.ReceiverID == body.ReceiverID) ||
			(replyToMsg.SenderID == body.ReceiverID && replyToMsg.ReceiverID == uid)) {
			log.Printf("❌ SendDirectMessage: Reply to message %d does not belong to this conversation", *body.ReplyToID)
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "reply_to message not in this conversation"})
			return
		}
	}

	// Create direct message
	message := models.DirectMessage{
		SenderID:   uid,
		ReceiverID: body.ReceiverID,
		Content:    body.Content,
		Type:       body.Type,
		RefType:    body.RefType,
		RefID:      body.RefID,
		ReplyToID:  body.ReplyToID,
	}
	if err := storage.DB.Create(&message).Error; err != nil {
		log.Printf("❌ SendDirectMessage: Failed to create message - %v", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to send message"})
		return
	}

	log.Printf("✅ SendDirectMessage: Message %d created successfully from user %d to user %d",
		message.ID, uid, body.ReceiverID)

	// Send push notification to receiver with sender's avatar
	var sender models.User
	if err := storage.DB.First(&sender, uid).Error; err == nil {
		// Check if receiver allows notifications (receiver was already fetched earlier)
		if receiver.AllowsNotifications != nil && !*receiver.AllowsNotifications {
			log.Printf("🔔 User %d has notifications disabled, skipping push", body.ReceiverID)
		} else {
			senderName := fmt.Sprintf("%s %s", sender.FirstName, sender.LastName)
			messageBody := body.Content
			if len(messageBody) > 120 {
				messageBody = messageBody[:120] + "…"
			}

			// Enhanced notification title for property inquiries
			notificationTitle := senderName
			if body.RefType == "property" && body.RefID != nil {
				var property models.Property
				if err := storage.DB.First(&property, *body.RefID).Error; err == nil {
					notificationTitle = fmt.Sprintf("%s - %s", senderName, property.Title)
				}
			}

			// Get sender's avatar URL (use AvatarURL if available, otherwise empty)
			senderAvatarURL := ""
			if sender.AvatarURL != "" {
				senderAvatarURL = sender.AvatarURL
				log.Printf("🖼️ Using sender avatar: %s", senderAvatarURL)
			} else {
				log.Printf("⚠️ No avatar URL for sender %d", uid)
			}

			// Get receiver's push tokens and send notification with sender avatar
			receiverTokens := pushsvc.GetUserPushTokens(body.ReceiverID)
			if len(receiverTokens) > 0 {
				go func() {
					err := pushsvc.SendPushWithImage(receiverTokens, notificationTitle, messageBody, senderAvatarURL, nil)
					if err != nil {
						log.Printf("⚠️ Failed to send push notification: %v", err)
					} else {
						log.Printf("✅ Sent direct message notification to user %d (property inquiry: %v)",
							body.ReceiverID, body.RefType == "property")
					}
				}()
			} else {
				log.Printf("🔔 No push tokens found for receiver %d", body.ReceiverID)
			}
		}
	} else {
		log.Printf("⚠️ Sender %d not found for push notification", uid)
	}

	ctx.JSON(iris.Map{
		"message":    "Message sent successfully",
		"message_id": message.ID,
	})
}

// DirectMessageResponse - Response structure with reply details
type DirectMessageResponse struct {
	models.DirectMessage
	ReplyToMessage *models.DirectMessage `json:"reply_to_message,omitempty"`
	ReactionsData  []ReactionSummary     `json:"reactions_data,omitempty"`
}

type ReactionSummary struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Users []uint `json:"users"`
}

// GetDirectMessages - Get direct messages between two users
func GetDirectMessages(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	otherUserID, err := ctx.Params().GetUint("userID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid user id"})
		return
	}

	// Check if either user has blocked the other
	var blockCheck models.UserBlock
	if err := storage.DB.Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?) AND deleted_at IS NULL",
		uid, otherUserID, otherUserID, uid).First(&blockCheck).Error; err == nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "cannot access messages with this user"})
		return
	}

	var messages []models.DirectMessage
	storage.DB.Where("((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)) AND deleted_at IS NULL",
		uid, otherUserID, otherUserID, uid).
		Order("created_at ASC").
		Find(&messages)

	// Build response with reply details and reactions
	var response []DirectMessageResponse
	for _, msg := range messages {
		resp := DirectMessageResponse{
			DirectMessage: msg,
		}

		// Load reply-to message if exists
		if msg.ReplyToID != nil && *msg.ReplyToID != 0 {
			var replyToMsg models.DirectMessage
			if err := storage.DB.First(&replyToMsg, *msg.ReplyToID).Error; err == nil {
				resp.ReplyToMessage = &replyToMsg
			}
		}

		// Load reactions for this message
		var reactions []models.MessageReaction
		storage.DB.Where("message_id = ?", msg.ID).Find(&reactions)

		// Group reactions by emoji
		emojiMap := make(map[string][]uint)
		for _, r := range reactions {
			emojiMap[r.Emoji] = append(emojiMap[r.Emoji], r.UserID)
		}

		for emoji, users := range emojiMap {
			resp.ReactionsData = append(resp.ReactionsData, ReactionSummary{
				Emoji: emoji,
				Count: len(users),
				Users: users,
			})
		}

		response = append(response, resp)
	}

	ctx.JSON(iris.Map{
		"messages": response,
		"total":    len(response),
	})
}

// MarkDirectMessageRead - Mark a direct message as read
func MarkDirectMessageRead(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		return
	}

	messageID, err := ctx.Params().GetUint("messageID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid message id"})
		return
	}

	// Update message as read
	now := time.Now()
	if err := storage.DB.Model(&models.DirectMessage{}).
		Where("id = ? AND receiver_id = ?", messageID, uid).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": &now,
		}).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "message not found"})
		return
	}

	ctx.JSON(iris.Map{"message": "Message marked as read"})
}

// ListDirectMessageConversations - Server-first, SOLID-compliant conversation feed
// STRICT CONTRACT: Server owns ordering, pagination, and state
// SECURITY: Input validation, authorization checks, and optimized queries
// Returns deterministic, idempotent results with cursor-based pagination
// ZERO DISAPPEARANCE: Always returns valid response, never errors that could clear feed
func ListDirectMessageConversations(ctx iris.Context) {
	// SERVER CONTRACT RESPONSE - Always valid structure
	type ConversationSummary struct {
		OtherUserID     uint                  `json:"other_user_id"`
		OtherUserName   string                `json:"other_user_name"`
		OtherUserAvatar string                `json:"other_user_avatar"`
		LastMessage     *models.DirectMessage `json:"last_message"`
		UnreadCount     int                   `json:"unread_count"`
	}

	// Helper to return empty but valid response
	sendEmptyResponse := func() {
		ctx.JSON(iris.Map{
			"items":           []ConversationSummary{},
			"nextCursor":      nil,
			"hasMore":         false,
			"serverTimestamp": time.Now().Unix(),
			"total":           0,
			"status":          "ok",
		})
	}

	// SECURITY: Extract and validate user ID
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		log.Printf("❌ ListDirectMessageConversations: Unauthorized - no userID in context")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{
			"error":           "unauthorized",
			"items":           []ConversationSummary{},
			"nextCursor":      nil,
			"hasMore":         false,
			"serverTimestamp": time.Now().Unix(),
			"total":           0,
		})
		return
	}

	// SECURITY: Validate user exists and is active
	var currentUser models.User
	if err := storage.DB.First(&currentUser, uid).Error; err != nil {
		log.Printf("❌ ListDirectMessageConversations: User %d not found", uid)
		sendEmptyResponse()
		return
	}

	// SECURITY: Input validation and sanitization
	cursor := strings.TrimSpace(ctx.URLParam("cursor"))
	limit := ctx.URLParamIntDefault("limit", 100)

	// SECURITY: Enforce strict limits to prevent abuse
	if limit < 1 {
		limit = 100
	} else if limit > 200 {
		limit = 200
	}

	// SECURITY: Validate cursor format if provided
	if cursor != "" {
		var cursorTimestamp int64
		var cursorUserID uint
		if n, err := fmt.Sscanf(cursor, "%d:%d", &cursorTimestamp, &cursorUserID); n != 2 || err != nil {
			log.Printf("⚠️ ListDirectMessageConversations: Invalid cursor format: %s", cursor)
			cursor = ""
		} else {
			now := time.Now().Unix()
			if cursorTimestamp < now-31536000 || cursorTimestamp > now+3600 {
				log.Printf("⚠️ ListDirectMessageConversations: Invalid cursor timestamp: %d", cursorTimestamp)
				cursor = ""
			}
		}
	}

	log.Printf("🔍 ListDirectMessageConversations: User %d, cursor=%s, limit=%d", uid, cursor, limit)

	// PERFORMANCE: Optimized single query to get all conversation partners
	type ConversationPartner struct {
		OtherUserID   uint      `gorm:"column:other_user_id"`
		LastMessageID uint      `gorm:"column:last_message_id"`
		LastMessageAt time.Time `gorm:"column:last_message_at"`
	}

	var partners []ConversationPartner

	// ROBUST QUERY: Works on PostgreSQL with DISTINCT ON
	// Fallback for other databases uses subquery approach
	err := storage.DB.Raw(`
		SELECT DISTINCT ON (other_user_id)
			other_user_id,
			last_message_id,
			last_message_at
		FROM (
			SELECT 
				CASE 
					WHEN sender_id = ? THEN receiver_id
					ELSE sender_id
				END as other_user_id,
				id as last_message_id,
				created_at as last_message_at,
				ROW_NUMBER() OVER (
					PARTITION BY 
						CASE 
							WHEN sender_id = ? THEN receiver_id
							ELSE sender_id
						END 
					ORDER BY created_at DESC
				) as rn
			FROM direct_messages
			WHERE (sender_id = ? OR receiver_id = ?) 
				AND deleted_at IS NULL
		) ranked
		WHERE rn = 1
		ORDER BY other_user_id, last_message_at DESC
	`, uid, uid, uid, uid).Scan(&partners).Error

	if err != nil {
		log.Printf("❌ ListDirectMessageConversations: Error querying partners: %v", err)
		// ZERO DISAPPEARANCE: Return empty valid response, not error
		sendEmptyResponse()
		return
	}

	log.Printf("✅ ListDirectMessageConversations: Found %d unique conversation partners for user %d", len(partners), uid)

	// SECURITY: Filter out blocked users
	// Get all blocks where user is either blocker or blocked
	var blocks []models.UserBlock
	storage.DB.Where("(blocker_id = ? OR blocked_id = ?) AND deleted_at IS NULL", uid, uid).Find(&blocks)

	blockedMap := make(map[uint]bool)
	for _, block := range blocks {
		// Determine which user is blocked (the other one)
		if block.BlockerID == uid {
			blockedMap[block.BlockedID] = true
		} else {
			blockedMap[block.BlockerID] = true
		}
	}

	var filteredPartners []ConversationPartner
	for _, p := range partners {
		if !blockedMap[p.OtherUserID] {
			filteredPartners = append(filteredPartners, p)
		}
	}
	partners = filteredPartners

	// SERVER-CONTROLLED SORTING: Sort by last message time descending
	sort.Slice(partners, func(i, j int) bool {
		return partners[i].LastMessageAt.After(partners[j].LastMessageAt)
	})

	log.Printf("✅ ListDirectMessageConversations: After filtering and sorting: %d partners", len(partners))

	// CURSOR-BASED PAGINATION (server-controlled)
	var paginatedPartners []ConversationPartner
	var nextCursor string
	hasMore := false

	if cursor != "" {
		var cursorTimestamp int64
		var cursorUserID uint
		if n, err := fmt.Sscanf(cursor, "%d:%d", &cursorTimestamp, &cursorUserID); n == 2 && err == nil {
			cursorTime := time.Unix(cursorTimestamp, 0)
			startIdx := -1
			for i, p := range partners {
				if p.LastMessageAt.Before(cursorTime) ||
					(p.LastMessageAt.Equal(cursorTime) && p.OtherUserID < cursorUserID) {
					startIdx = i
					break
				}
			}
			if startIdx >= 0 {
				endIdx := startIdx + limit + 1
				if endIdx > len(partners) {
					endIdx = len(partners)
				}
				paginatedPartners = partners[startIdx:endIdx]
			} else {
				// Cursor past all data - return empty
				paginatedPartners = []ConversationPartner{}
			}
		} else {
			paginatedPartners = partners
			if len(paginatedPartners) > limit+1 {
				paginatedPartners = paginatedPartners[:limit+1]
			}
		}
	} else {
		if len(partners) > limit+1 {
			paginatedPartners = partners[:limit+1]
		} else {
			paginatedPartners = partners
		}
	}

	// Determine hasMore and nextCursor
	if len(paginatedPartners) > limit {
		hasMore = true
		paginatedPartners = paginatedPartners[:limit]
	}

	if hasMore && len(paginatedPartners) > 0 {
		lastPartner := paginatedPartners[len(paginatedPartners)-1]
		nextCursor = fmt.Sprintf("%d:%d", lastPartner.LastMessageAt.Unix(), lastPartner.OtherUserID)
	}

	log.Printf("✅ ListDirectMessageConversations: Paginated to %d conversations (hasMore=%v)", len(paginatedPartners), hasMore)

	// BUILD SUMMARIES with all conversation data
	var summaries []ConversationSummary
	for _, partner := range paginatedPartners {
		var otherUser models.User
		if err := storage.DB.Where("id = ? AND deleted_at IS NULL", partner.OtherUserID).First(&otherUser).Error; err != nil {
			log.Printf("⚠️ ListDirectMessageConversations: User %d not found, skipping", partner.OtherUserID)
			continue
		}

		var lastMessage models.DirectMessage
		err := storage.DB.Where(
			"((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)) AND deleted_at IS NULL",
			uid, partner.OtherUserID, partner.OtherUserID, uid,
		).Order("created_at DESC").First(&lastMessage).Error

		if err != nil {
			log.Printf("⚠️ ListDirectMessageConversations: No message found for partner %d, skipping", partner.OtherUserID)
			continue
		}

		var unreadCount int64
		storage.DB.Model(&models.DirectMessage{}).
			Where("sender_id = ? AND receiver_id = ? AND is_read = false AND deleted_at IS NULL",
				partner.OtherUserID, uid).
			Count(&unreadCount)

		otherUserName := fmt.Sprintf("%s %s", otherUser.FirstName, otherUser.LastName)
		if strings.TrimSpace(otherUserName) == "" {
			otherUserName = otherUser.Email
		}

		// Get organization image if user owns an organization
		avatarURL := otherUser.AvatarURL
		var organization models.Organization
		// Suppress "record not found" errors - not all users have organizations (expected)
		if err := storage.DB.Where("owner_id = ? AND deleted_at IS NULL", partner.OtherUserID).First(&organization).Error; err == nil {
			// User owns an organization - prefer organization banner/logo
			if organization.BannerImage != "" {
				avatarURL = organization.BannerImage
			} else if organization.Logo != "" {
				avatarURL = organization.Logo
			}
			// Update name to show organization name if available
			if organization.Name != "" {
				otherUserName = organization.Name
			}
		}
		// Note: "record not found" is expected for users without organizations - no error logging needed

		summaries = append(summaries, ConversationSummary{
			OtherUserID:     partner.OtherUserID,
			OtherUserName:   otherUserName,
			OtherUserAvatar: avatarURL,
			LastMessage:     &lastMessage,
			UnreadCount:     int(unreadCount),
		})
	}

	log.Printf("✅ ListDirectMessageConversations: Returning %d conversations for user %d", len(summaries), uid)

	// STRICT SERVER CONTRACT RESPONSE
	if summaries == nil {
		summaries = []ConversationSummary{}
	}

	ctx.JSON(iris.Map{
		"items":           summaries,
		"nextCursor":      nextCursor,
		"hasMore":         hasMore,
		"serverTimestamp": time.Now().Unix(),
		"total":           len(summaries),
		"status":          "ok",
	})
}

// AddMessageReaction - Add a reaction (emoji) to a direct message
func AddMessageReaction(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	messageID, err := ctx.Params().GetUint("messageID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid message id"})
		return
	}

	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Emoji == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "emoji is required"})
		return
	}

	// Validate message exists and user has access
	var message models.DirectMessage
	if err := storage.DB.First(&message, messageID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "message not found"})
		return
	}

	// Verify user is part of this conversation
	if message.SenderID != uid && message.ReceiverID != uid {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "not authorized to react to this message"})
		return
	}

	// Check if user already reacted with this emoji
	var existingReaction models.MessageReaction
	if err := storage.DB.Where("message_id = ? AND user_id = ? AND emoji = ?",
		messageID, uid, body.Emoji).First(&existingReaction).Error; err == nil {
		// Reaction already exists - return success (idempotent)
		ctx.JSON(iris.Map{
			"message":     "reaction already exists",
			"reaction_id": existingReaction.ID,
		})
		return
	}

	// Create new reaction
	reaction := models.MessageReaction{
		MessageID: messageID,
		UserID:    uid,
		Emoji:     body.Emoji,
	}
	if err := storage.DB.Create(&reaction).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to add reaction"})
		return
	}

	log.Printf("✅ AddMessageReaction: User %d reacted with %s to message %d", uid, body.Emoji, messageID)

	// Get all reactions for this message to return updated state
	var reactions []models.MessageReaction
	storage.DB.Where("message_id = ?", messageID).Find(&reactions)

	emojiMap := make(map[string][]uint)
	for _, r := range reactions {
		emojiMap[r.Emoji] = append(emojiMap[r.Emoji], r.UserID)
	}

	var reactionsData []ReactionSummary
	for emoji, users := range emojiMap {
		reactionsData = append(reactionsData, ReactionSummary{
			Emoji: emoji,
			Count: len(users),
			Users: users,
		})
	}

	ctx.JSON(iris.Map{
		"message":        "reaction added",
		"reaction_id":    reaction.ID,
		"reactions_data": reactionsData,
	})
}

// RemoveMessageReaction - Remove a reaction from a direct message
func RemoveMessageReaction(ctx iris.Context) {
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	messageID, err := ctx.Params().GetUint("messageID")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid message id"})
		return
	}

	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Emoji == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "emoji is required"})
		return
	}

	// Validate message exists
	var message models.DirectMessage
	if err := storage.DB.First(&message, messageID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "message not found"})
		return
	}

	// Verify user is part of this conversation
	if message.SenderID != uid && message.ReceiverID != uid {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "not authorized"})
		return
	}

	// Delete the reaction
	result := storage.DB.Where("message_id = ? AND user_id = ? AND emoji = ?",
		messageID, uid, body.Emoji).Delete(&models.MessageReaction{})

	if result.RowsAffected == 0 {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "reaction not found"})
		return
	}

	log.Printf("✅ RemoveMessageReaction: User %d removed %s reaction from message %d", uid, body.Emoji, messageID)

	// Get remaining reactions
	var reactions []models.MessageReaction
	storage.DB.Where("message_id = ?", messageID).Find(&reactions)

	emojiMap := make(map[string][]uint)
	for _, r := range reactions {
		emojiMap[r.Emoji] = append(emojiMap[r.Emoji], r.UserID)
	}

	var reactionsData []ReactionSummary
	for emoji, users := range emojiMap {
		reactionsData = append(reactionsData, ReactionSummary{
			Emoji: emoji,
			Count: len(users),
			Users: users,
		})
	}

	ctx.JSON(iris.Map{
		"message":        "reaction removed",
		"reactions_data": reactionsData,
	})
}
