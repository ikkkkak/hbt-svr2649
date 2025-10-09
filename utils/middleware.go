package utils

import (
	"strconv"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

func UserIDMiddleware(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	claims := jwt.Get(ctx).(*AccessToken)

	userID := strconv.FormatUint(uint64(claims.ID), 10)

	if userID != id {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}
	ctx.Next()
}

// UserIDFromTokenMiddleware extracts user ID from JWT token and stores it in context
// Use this for routes that don't have {id} parameter in URL
func UserIDFromTokenMiddleware(ctx iris.Context) {
	claims := jwt.Get(ctx).(*AccessToken)
	ctx.Values().Set("userID", claims.ID)
	ctx.Next()
}

// AdminOnlyMiddleware ensures the requester has admin or super_admin role
func AdminOnlyMiddleware(ctx iris.Context) {
	claims := jwt.Get(ctx).(*AccessToken)
	role := claims.Role
	if role != "admin" && role != "super_admin" {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden", "message": "admin access required"})
		return
	}
    // Ensure userID is available to downstream handlers
    ctx.Values().Set("userID", claims.ID)
	ctx.Next()
}

// SuperAdminOnlyMiddleware ensures only super admins can access
func SuperAdminOnlyMiddleware(ctx iris.Context) {
	claims := jwt.Get(ctx).(*AccessToken)
	role := claims.Role
	if role != "super_admin" {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden", "message": "super_admin access required"})
		return
	}
	ctx.Next()
}