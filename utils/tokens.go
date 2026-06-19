package utils

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// refreshRateLimit: 10 requests per minute per IP (brief R-13)
var refreshRateMu sync.Mutex
var refreshRateCounts = map[string][]time.Time{}

func checkRefreshRateLimit(ip string) bool {
	refreshRateMu.Lock()
	defer refreshRateMu.Unlock()
	cutoff := time.Now().Add(-time.Minute)
	if refreshRateCounts[ip] == nil {
		refreshRateCounts[ip] = []time.Time{}
	}
	// Prune old entries
	valid := make([]time.Time, 0, len(refreshRateCounts[ip]))
	for _, t := range refreshRateCounts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= 10 {
		return false
	}
	refreshRateCounts[ip] = append(valid, time.Now())
	return true
}

// loginRateLimit: 10 requests per 15 minutes per IP
var loginRateMu sync.Mutex
var loginRateCounts = map[string][]time.Time{}

func checkLoginRateLimit(ip string) bool {
	loginRateMu.Lock()
	defer loginRateMu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	if loginRateCounts[ip] == nil {
		loginRateCounts[ip] = []time.Time{}
	}
	valid := make([]time.Time, 0, len(loginRateCounts[ip]))
	for _, t := range loginRateCounts[ip] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= 10 {
		return false
	}
	loginRateCounts[ip] = append(valid, time.Now())
	return true
}

func clientIP(ctx iris.Context) string {
	if x := ctx.GetHeader("X-Forwarded-For"); x != "" {
		if i := strings.Index(x, ","); i > 0 {
			return strings.TrimSpace(x[:i])
		}
		return strings.TrimSpace(x)
	}
	host, _, _ := net.SplitHostPort(ctx.Request().RemoteAddr)
	if host != "" {
		return host
	}
	return ctx.Request().RemoteAddr
}

// EnforceLoginRateLimit returns false if login attempts exceeded.
func EnforceLoginRateLimit(ctx iris.Context) bool {
	if !checkLoginRateLimit(clientIP(ctx)) {
		ctx.StatusCode(429)
		ctx.Header("Retry-After", "900")
		ctx.JSON(iris.Map{"error": "too many login attempts, try again later"})
		return false
	}
	return true
}

// PurgeOldRefreshTokens deletes refresh tokens that are expired/revoked older than 7 days.
// Call on a schedule (e.g. daily).
func PurgeOldRefreshTokens() {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	// Best-effort: ignore errors; never crash server.
	storage.DB.Exec(
		`DELETE FROM refresh_tokens WHERE (expires_at < ? OR revoked = true) AND expires_at < ?`,
		time.Now(), cutoff,
	)
}


// AccessTokenExpiry returns the access token lifetime. Uses ACCESS_TOKEN_EXPIRY_MINUTES (default 60).
func AccessTokenExpiry() time.Duration {
	m := 60
	if s := os.Getenv("ACCESS_TOKEN_EXPIRY_MINUTES"); s != "" {
		if n, _ := strconv.Atoi(s); n > 0 {
			m = n
		}
	}
	return time.Duration(m) * time.Minute
}

// AccessTokenExpiresInSeconds returns access token TTL in seconds for client (expiresIn).
func AccessTokenExpiresInSeconds() int {
	return int(AccessTokenExpiry().Seconds())
}

// RefreshTokenExpiry returns the refresh token lifetime.
// Uses REFRESH_TOKEN_EXPIRY_DAYS (default 14 — OAuth mobile session target).
func RefreshTokenExpiry() time.Duration {
	d := 14
	if s := os.Getenv("REFRESH_TOKEN_EXPIRY_DAYS"); s != "" {
		if n, _ := strconv.Atoi(s); n > 0 {
			d = n
		}
	}
	return time.Duration(d) * 24 * time.Hour
}

var bgContext = context.Background()

func CreateForgotPasswordToken(id uint, email string) (string, error) {
	signer := jwt.NewSigner(jwt.HS256, os.Getenv("EMAIL_TOKEN_SECRET"), 10*time.Minute)

	claims := ForgotPasswordToken{
		ID:    id,
		Email: email,
	}

	token, err := signer.Sign(claims)
	if err != nil {
		return "", err
	}

	return string(token), nil
}

// CreateTokenPair creates access token (JWT) + opaque refresh token.
// Refresh token: 64-byte random, base64url-encoded. Stored as SHA-256 hash only in DB.
func CreateTokenPair(id uint, deviceID string) (*jwt.TokenPair, error) {
	accessTokenSigner := jwt.NewSigner(jwt.HS256, os.Getenv("ACCESS_TOKEN_SECRET"), AccessTokenExpiry())
	refreshTTL := RefreshTokenExpiry()

	// Load role for embedding into access token
	var u models.User
	role := "user"
	if err := storage.DB.Select("id, role").First(&u, id).Error; err == nil && u.Role != "" {
		role = u.Role
	}

	accessTokenClaims := AccessToken{ID: id, Role: role}
	accessToken, err := accessTokenSigner.Sign(accessTokenClaims)
	if err != nil {
		return nil, err
	}

	// Opaque refresh token: 64-byte cryptographically random, base64url-encoded
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	refreshToken := base64.URLEncoding.EncodeToString(raw)
	refreshToken = strings.TrimRight(refreshToken, "=") // URL-safe, no padding

	// Store SHA-256 hash only — never plaintext
	hash := sha256.Sum256([]byte(refreshToken))
	tokenHash := hex.EncodeToString(hash[:])

	refreshTokenRecord := models.RefreshToken{
		TokenHash: tokenHash,
		UserID:    id,
		DeviceID:  deviceID,
		ExpiresAt: time.Now().Add(refreshTTL),
		Revoked:   false,
	}
	if err := storage.DB.Create(&refreshTokenRecord).Error; err != nil {
		return nil, err
	}

	var tokenPair jwt.TokenPair
	tokenPair.AccessToken = accessToken
	tokenPair.RefreshToken = []byte(refreshToken)
	return &tokenPair, nil
}

// RefreshTokenInput supports both refreshToken and refresh_token (brief uses snake_case)
type RefreshTokenBody struct {
	RefreshToken  string `json:"refreshToken"`
	RefreshToken2 string `json:"refresh_token"`
}

// RefreshToken handles token refresh with rotation. Supports:
// - Opaque tokens (new): hash lookup by token_hash
// - Legacy JWT: lookup by token string for backward compatibility
func RefreshToken(ctx iris.Context) {
	// Rate limit: 10/min per IP (brief R-13)
	if !checkRefreshRateLimit(clientIP(ctx)) {
		ctx.StatusCode(429)
		ctx.Header("Retry-After", "60")
		ctx.JSON(iris.Map{"error": "too many refresh attempts, try again later"})
		return
	}

	var body RefreshTokenBody
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid JSON"})
		return
	}
	tokenStr := body.RefreshToken
	if tokenStr == "" {
		tokenStr = body.RefreshToken2
	}
	if tokenStr == "" {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "missing refresh token"})
		return
	}

	var refreshTokenRecord models.RefreshToken
	if strings.Contains(tokenStr, ".") {
		// Legacy JWT: lookup by token string
		if err := storage.DB.Where("token = ?", tokenStr).First(&refreshTokenRecord).Error; err != nil {
			validToken, tokenErr := storage.Redis.Get(bgContext, tokenStr).Result()
			if tokenErr != nil || validToken != "true" {
				ctx.StatusCode(iris.StatusUnauthorized)
				ctx.JSON(iris.Map{"error": "refresh token not found or invalid"})
				return
			}
			// Redis fallback — extract user from JWT if possible
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("REFRESH_TOKEN_SECRET")))
			token, verr := verifier.VerifyToken([]byte(tokenStr))
			if verr != nil {
				ctx.StatusCode(iris.StatusUnauthorized)
				ctx.JSON(iris.Map{"error": "invalid refresh token"})
				return
			}
			userID, _ := strconv.ParseUint(token.StandardClaims.Subject, 10, 32)
			tok := tokenStr
			refreshTokenRecord = models.RefreshToken{
				Token:     &tok,
				UserID:    uint(userID),
				ExpiresAt: time.Now().Add(RefreshTokenExpiry()),
				Revoked:   false,
			}
			storage.DB.Create(&refreshTokenRecord)
			storage.Redis.Del(bgContext, tokenStr)
		}
	} else {
		// Opaque token: lookup by SHA-256 hash
		hash := sha256.Sum256([]byte(tokenStr))
		tokenHash := hex.EncodeToString(hash[:])
		if err := storage.DB.Where("token_hash = ?", tokenHash).First(&refreshTokenRecord).Error; err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "refresh token not found or invalid"})
			return
		}
	}

	if refreshTokenRecord.Revoked {
		// Token reuse detection: a revoked refresh token is being used again.
		// Security response: revoke ALL tokens for this user (force re-login everywhere).
		log.Printf("[auth] refresh reuse detected user_id=%d ip=%s", refreshTokenRecord.UserID, clientIP(ctx))
		storage.DB.Model(&models.RefreshToken{}).
			Where("user_id = ? AND revoked = false", refreshTokenRecord.UserID).
			Updates(map[string]interface{}{"revoked": true, "revoked_at": time.Now()})

		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{
			"error": "token reuse detected — all sessions invalidated",
			"code":  "REFRESH_REUSED",
		})
		return
	}
	if time.Now().After(refreshTokenRecord.ExpiresAt) {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "refresh token has expired", "code": "REFRESH_EXPIRED"})
		return
	}

	deviceID := ctx.GetHeader("X-Device-ID")
	if deviceID == "" {
		deviceID = refreshTokenRecord.DeviceID
	}

	// Token rotation: revoke old
	now := time.Now()
	refreshTokenRecord.Revoked = true
	refreshTokenRecord.RevokedAt = &now
	storage.DB.Save(&refreshTokenRecord)
	storage.Redis.Del(bgContext, tokenStr)

	tokenPair, err := CreateTokenPair(refreshTokenRecord.UserID, deviceID)
	if err != nil {
		log.Printf("[auth] refresh token issue failed user_id=%d: %v", refreshTokenRecord.UserID, err)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to generate new tokens"})
		return
	}

	log.Printf("[auth] refresh ok user_id=%d ip=%s", refreshTokenRecord.UserID, clientIP(ctx))
	ctx.JSON(iris.Map{
		"accessToken":  string(tokenPair.AccessToken),
		"refreshToken": string(tokenPair.RefreshToken),
		"expiresIn":    AccessTokenExpiresInSeconds(),
		"user_id":      refreshTokenRecord.UserID,
	})
}

// GenerateShortToken returns a URL-safe random string of the given length (bytes*2 hex).
func GenerateShortToken(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	// hex encoding doubles length; that's fine for uniqueness and safety
	const hex = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}

type ForgotPasswordToken struct {
	ID    uint   `json:"ID"`
	Email string `json:"email"`
}

type AccessToken struct {
	ID   uint   `json:"ID"`
	Role string `json:"role"`
}

// RefreshTokenInput used by extractor; RefreshTokenBody used by handler
type RefreshTokenInput struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

// RevokeRefreshToken handles POST /auth/logout — revokes the refresh token server-side
func RevokeRefreshToken(ctx iris.Context) {
	var body RefreshTokenBody
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid JSON"})
		return
	}
	tokenStr := body.RefreshToken
	if tokenStr == "" {
		tokenStr = body.RefreshToken2
	}
	if tokenStr == "" {
		ctx.StatusCode(iris.StatusOK)
		ctx.JSON(iris.Map{"message": "logged out"})
		return
	}

	if strings.Contains(tokenStr, ".") {
		// Legacy JWT: revoke by token string
		storage.DB.Model(&models.RefreshToken{}).Where("token = ?", tokenStr).Updates(map[string]interface{}{
			"revoked":    true,
			"revoked_at": time.Now(),
		})
		storage.Redis.Del(bgContext, tokenStr)
	} else {
		hash := sha256.Sum256([]byte(tokenStr))
		tokenHash := hex.EncodeToString(hash[:])
		storage.DB.Model(&models.RefreshToken{}).Where("token_hash = ?", tokenHash).Updates(map[string]interface{}{
			"revoked":    true,
			"revoked_at": time.Now(),
		})
	}

	ctx.StatusCode(iris.StatusOK)
	ctx.JSON(iris.Map{"message": "logged out"})
}

// RevokeAllRefreshTokens handles POST /auth/logout-all — revokes all sessions for the authenticated user.
func RevokeAllRefreshTokens(ctx iris.Context) {
	uidAny := ctx.Values().Get("userID")
	uid, ok := uidAny.(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}
	storage.DB.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked = false", uid).
		Updates(map[string]interface{}{"revoked": true, "revoked_at": time.Now()})
	ctx.JSON(iris.Map{"message": "all sessions revoked"})
}

type RefreshSessionDTO struct {
	ID        uint      `json:"id"`
	DeviceID   string   `json:"device_id"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Revoked    bool     `json:"revoked"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// ListRefreshTokenSessions handles GET /auth/sessions — lists active sessions for the authenticated user.
func ListRefreshTokenSessions(ctx iris.Context) {
	uidAny := ctx.Values().Get("userID")
	uid, ok := uidAny.(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var rows []models.RefreshToken
	if err := storage.DB.
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to fetch sessions"})
		return
	}

	out := make([]RefreshSessionDTO, 0, len(rows))
	for _, r := range rows {
		rr := r // copy
		last := rr.UpdatedAt
		out = append(out, RefreshSessionDTO{
			ID:         rr.ID,
			DeviceID:   rr.DeviceID,
			CreatedAt:  rr.CreatedAt,
			ExpiresAt:  rr.ExpiresAt,
			Revoked:    rr.Revoked,
			LastUsedAt: &last, // best-effort (we don't track last_used_at column yet)
		})
	}
	ctx.JSON(iris.Map{"sessions": out})
}

var errExpired = errors.New("expired")
