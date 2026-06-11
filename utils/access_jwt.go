package utils

import (
	"os"
	"strings"
	"sync"

	irisjwt "github.com/kataras/iris/v12/middleware/jwt"
)

var (
	accessJWTOnce     sync.Once
	accessJWTVerifier *irisjwt.Verifier
)

// InitAccessJWTVerifier builds the shared access-token verifier once (blocklist + HS256).
func InitAccessJWTVerifier() {
	accessJWTOnce.Do(func() {
		accessJWTVerifier = irisjwt.NewVerifier(irisjwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
		accessJWTVerifier.WithDefaultBlocklist()
	})
}

// AccessTokenParseResult is returned by ParseAccessToken.
type AccessTokenParseResult struct {
	Claims  *AccessToken
	Expired bool
}

// ParseAccessToken validates a Bearer access JWT.
func ParseAccessToken(rawToken string) AccessTokenParseResult {
	if rawToken == "" {
		return AccessTokenParseResult{}
	}
	InitAccessJWTVerifier()
	claims := new(AccessToken)
	verifiedToken, err := accessJWTVerifier.VerifyToken([]byte(rawToken))
	if err == nil && verifiedToken != nil {
		if err := verifiedToken.Claims(claims); err == nil && claims != nil && claims.ID > 0 {
			return AccessTokenParseResult{Claims: claims}
		}
	}
	expired := err != nil && strings.Contains(strings.ToLower(err.Error()), "expired")
	return AccessTokenParseResult{Expired: expired}
}

// VerifyAccessToken parses and validates a Bearer access JWT. Returns nil if invalid or expired.
func VerifyAccessToken(rawToken string) *AccessToken {
	r := ParseAccessToken(rawToken)
	return r.Claims
}
