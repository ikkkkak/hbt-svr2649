package utils

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"crypto/rand"
	"os"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// AccessTokenExpiry returns the access token lifetime. Uses ACCESS_TOKEN_EXPIRY_MINUTES (default 15).
func AccessTokenExpiry() time.Duration {
	m := 15
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

// CreateTokenPair creates access and refresh tokens with database storage
// deviceID is optional for tracking device sessions
func CreateTokenPair(id uint, deviceID string) (*jwt.TokenPair, error) {
	accessTokenSigner := jwt.NewSigner(jwt.HS256, os.Getenv("ACCESS_TOKEN_SECRET"), AccessTokenExpiry())
	// Refresh token: long-lived (30 days)
	refreshTokenSigner := jwt.NewSigner(jwt.HS256, os.Getenv("REFRESH_TOKEN_SECRET"), 30*24*time.Hour)

	userID := strconv.FormatUint(uint64(id), 10)

	refreshClaims := jwt.Claims{Subject: userID}

	// Load role for embedding into access token
	var u models.User
	role := "user"
	if err := storage.DB.Select("id, role").First(&u, id).Error; err == nil && u.Role != "" {
		role = u.Role
	}

	accessTokenClaims := AccessToken{
		ID:   id,
		Role: role,
	}

	accessToken, err := accessTokenSigner.Sign(accessTokenClaims)
	if err != nil {
		return nil, err
	}

	refreshToken, err := refreshTokenSigner.Sign(refreshClaims)
	if err != nil {
		return nil, err
	}

	// Store refresh token in database
	refreshTokenRecord := models.RefreshToken{
		Token:     string(refreshToken),
		UserID:    id,
		DeviceID:  deviceID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Revoked:   false,
	}
	if err := storage.DB.Create(&refreshTokenRecord).Error; err != nil {
		return nil, err
	}

	// Also store in Redis for backward compatibility (will be removed in future)
	storage.Redis.Set(bgContext, string(refreshToken), "true", 30*24*time.Hour+5*time.Minute)

	var tokenPair jwt.TokenPair
	tokenPair.AccessToken = accessToken
	tokenPair.RefreshToken = refreshToken

	return &tokenPair, nil
}

// RefreshToken handles token refresh with rotation (database-based)
func RefreshToken(ctx iris.Context) {
	token := jwt.GetVerifiedToken(ctx)
	if token == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "invalid refresh token"})
		return
	}

	tokenStr := string(token.Token)
	userID, parseErr := strconv.ParseUint(token.StandardClaims.Subject, 10, 32)
	if parseErr != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid token format"})
		return
	}

	// Find refresh token in database
	var refreshTokenRecord models.RefreshToken
	if err := storage.DB.Where("token = ? AND user_id = ?", tokenStr, uint(userID)).First(&refreshTokenRecord).Error; err != nil {
		// Fallback to Redis for backward compatibility
		validToken, tokenErr := storage.Redis.Get(bgContext, tokenStr).Result()
		if tokenErr != nil || validToken != "true" {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "refresh token not found or invalid"})
			return
		}
		// If found in Redis, migrate to database
		refreshTokenRecord = models.RefreshToken{
			Token:     tokenStr,
			UserID:    uint(userID),
			ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
			Revoked:   false,
		}
		storage.DB.Create(&refreshTokenRecord)
		storage.Redis.Del(bgContext, tokenStr)
	}

	// Check if token is revoked
	if refreshTokenRecord.Revoked {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "refresh token has been revoked"})
		return
	}

	// Check if token is expired
	if time.Now().After(refreshTokenRecord.ExpiresAt) {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "refresh token has expired"})
		return
	}

	// Get device ID from request header (optional)
	deviceID := ctx.GetHeader("X-Device-ID")
	if deviceID == "" {
		deviceID = refreshTokenRecord.DeviceID // Use existing device ID if available
	}

	// Revoke old refresh token (token rotation)
	now := time.Now()
	refreshTokenRecord.Revoked = true
	refreshTokenRecord.RevokedAt = &now
	storage.DB.Save(&refreshTokenRecord)

	// Also remove from Redis if exists
	storage.Redis.Del(bgContext, tokenStr)

	// Create new token pair
	tokenPair, tokenPairErr := CreateTokenPair(uint(userID), deviceID)
	if tokenPairErr != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to generate new tokens"})
		return
	}

	ctx.JSON(iris.Map{
		"accessToken":  string(tokenPair.AccessToken),
		"refreshToken": string(tokenPair.RefreshToken),
		"expiresIn":    AccessTokenExpiresInSeconds(),
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

type RefreshTokenInput struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}
