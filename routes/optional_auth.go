package routes

import (
	"apartments-clone-server/utils"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

// OptionalAuthUserID returns the user id when optionalAuthMiddleware validated Bearer JWT.
// Do not use jsonWT.Get alone — optionalAuthMiddleware sets ctx.Values, not the JWT middleware context.
func OptionalAuthUserID(ctx iris.Context) uint {
	if ctxUserID, ok := ctx.Values().Get("userID").(uint); ok && ctxUserID > 0 {
		return ctxUserID
	}
	if jwtClaims := ctx.Values().Get("jwt.claims"); jwtClaims != nil {
		if accessToken, ok := jwtClaims.(*utils.AccessToken); ok && accessToken.ID > 0 {
			return accessToken.ID
		}
	}
	if claims := jsonWT.Get(ctx); claims != nil {
		if accessToken, ok := claims.(*utils.AccessToken); ok && accessToken.ID > 0 {
			return accessToken.ID
		}
	}
	return 0
}
