package push

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	matched, _ := regexp.MatchString(`^[0-9a-fA-F]+$`, s)
	return matched
}

// parseTokensJSON robustly extracts a slice of tokens from various JSON shapes
func parseTokensJSON(raw json.RawMessage) []string {
	if raw == nil || len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single != "" {
			return []string{single}
		}
	}
	// try array of objects with "token" field
	var objArr []map[string]interface{}
	if err := json.Unmarshal(raw, &objArr); err == nil {
		for _, o := range objArr {
			if v, ok := o["token"].(string); ok && v != "" {
				arr = append(arr, v)
			}
		}
		if len(arr) > 0 {
			return arr
		}
	}
	return nil
}

// SetUserPushToken persists the Expo token in users.push_tokens (JSON array, unique per user)
func SetUserPushToken(userID uint, token string) {
	if userID == 0 || token == "" {
		log.Printf("⚠️ SetUserPushToken: invalid params userID=%d tokenLen=%d", userID, len(token))
		return
	}
	// Validate token length
	// Expo tokens: "ExponentPushToken[...]" (~41 chars)
	// FCM tokens: typically 152+ characters
	if len(token) < 20 {
		log.Printf("⚠️ SetUserPushToken: token too short! userID=%d tokenLen=%d token=%s", userID, len(token), token)
		return
	}
	prefix := token
	if len(token) > 12 {
		prefix = token[:12] + "..."
	}
	log.Printf("✅ SetUserPushToken: saving valid token userID=%d tokenLen=%d prefix=%s", userID, len(token), prefix)
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		return
	}
	var tokens []string
	if user.PushTokens != nil {
		tokens = parseTokensJSON(json.RawMessage(user.PushTokens))
	}
	// ensure unique
	exists := false
	for _, t := range tokens {
		if t == token {
			exists = true
			break
		}
	}
	if !exists {
		tokens = append(tokens, token)
	}
	b, _ := json.Marshal(tokens)
	if err := storage.DB.Model(&user).Update("push_tokens", b).Error; err != nil {
		log.Printf("⚠️ Failed saving push token for user %d: %v", userID, err)
	}
}

// RemoveTokenGlobally finds any users containing the token and removes it
func RemoveTokenGlobally(token string) {
	if token == "" {
		return
	}
	var users []models.User
	if err := storage.DB.Where("push_tokens IS NOT NULL").Find(&users).Error; err != nil {
		return
	}
	for _, u := range users {
		toks := parseTokensJSON(json.RawMessage(u.PushTokens))
		if len(toks) == 0 {
			continue
		}
		changed := false
		var kept []string
		for _, t := range toks {
			if t == token {
				changed = true
				continue
			}
			kept = append(kept, t)
		}
		if changed {
			b, _ := json.Marshal(kept)
			_ = storage.DB.Model(&models.User{}).Where("id = ?", u.ID).Update("push_tokens", b).Error
			log.Printf("🧹 Pruned invalid Expo token for user %d", u.ID)
		}
	}
}

// GetGroupPushTokens fetches members of the group and returns all Expo tokens except the sender's
func GetGroupPushTokens(groupID uint, excludeUserID uint) []string {
	var members []models.ExperienceGroupMember
	if err := storage.DB.Preload("User").Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		log.Printf("🔔 Push lookup error for group %d: %v", groupID, err)
		return nil
	}
	log.Printf("🔔 Push candidates for group %d (excluding user %d): %d members", groupID, excludeUserID, len(members))
	if len(members) == 0 {
		log.Printf("   • no members found for group %d", groupID)
	}
	var out []string
	for _, m := range members {
		if m.UserID == excludeUserID {
			continue
		}
		var tokens []string
		if m.User.PushTokens != nil {
			tokens = parseTokensJSON(json.RawMessage(m.User.PushTokens))
		}
		if len(tokens) == 0 {
			log.Printf("   • member userID=%d tokens=0", m.UserID)
		} else {
			// Build truncated preview list
			previews := make([]string, 0, len(tokens))
			for _, t := range tokens {
				if t == "" {
					continue
				}
				prev := t
				if len(prev) > 16 {
					prev = prev[:16] + "…"
				}
				previews = append(previews, prev)
			}
			log.Printf("   • member userID=%d tokens=%d [%s]", m.UserID, len(tokens), strings.Join(previews, ", "))
		}
		for _, t := range tokens {
			if t == "" {
				continue
			}
			// Filter out invalid/truncated tokens
			// Allow APNs tokens (64 chars), Expo tokens (~41 chars), and FCM tokens (152+ chars)
			if len(t) < 30 && !strings.HasPrefix(t, "ExponentPushToken") && !strings.HasPrefix(t, "ExpoPushToken") {
				log.Printf("   ⚠️ Skipping truncated token for userID=%d: length=%d", m.UserID, len(t))
				continue
			}
			// Accept multiple token formats:
			// - Expo tokens: "ExponentPushToken[...]"
			// - FCM tokens: 152+ character strings (full FCM registration tokens)
			// - APNs tokens: 64 hex character strings (iOS device tokens)
			// - Native Android tokens: 152+ character base64-like strings
			isExpoToken := strings.HasPrefix(t, "ExponentPushToken") || strings.HasPrefix(t, "ExpoPushToken")
			isAPNsToken := len(t) == 64 && !isExpoToken && isHexString(t) // iOS APNs tokens are 64 hex chars
			isFCMToken := len(t) > 50 && !isExpoToken && !isAPNsToken
			if !isExpoToken && !isFCMToken && !isAPNsToken {
				continue
			}
			out = append(out, t)
		}
	}
	return out
}

// LogGroupTokens logs members and their push token previews without returning them
func LogGroupTokens(groupID uint, excludeUserID uint) {
	var members []models.ExperienceGroupMember
	if err := storage.DB.Preload("User").Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		log.Printf("🔔 Push lookup error for group %d: %v", groupID, err)
		return
	}
	log.Printf("🔔 Push candidates for group %d (excluding user %d): %d members", groupID, excludeUserID, len(members))
	if len(members) == 0 {
		log.Printf("   • no members found for group %d", groupID)
	}
	for _, m := range members {
		if m.UserID == excludeUserID {
			continue
		}
		tokens := parseTokensJSON(json.RawMessage(m.User.PushTokens))
		if len(tokens) == 0 {
			log.Printf("   • member userID=%d tokens=0", m.UserID)
			continue
		}
		previews := make([]string, 0, len(tokens))
		for _, t := range tokens {
			if t == "" {
				continue
			}
			prev := t
			if len(prev) > 16 {
				prev = prev[:16] + "…"
			}
			previews = append(previews, prev)
		}
		log.Printf("   • member userID=%d tokens=%d [%s]", m.UserID, len(tokens), strings.Join(previews, ", "))
	}
}
