package utils

import (
	"strconv"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

func UserIDMiddleware(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	// Get userID from context (set by accessTokenVerifierMiddleware)
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	userID := strconv.FormatUint(uint64(uid), 10)

	if userID != id {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}
	ctx.Next()
}

// UserIDFromTokenMiddleware extracts user ID from JWT token and stores it in context
// Use this for routes that don't have {id} parameter in URL
func UserIDFromTokenMiddleware(ctx iris.Context) {
	// Get userID from context (set by accessTokenVerifierMiddleware)
	uid, ok := ctx.Values().Get("userID").(uint)
	if !ok || uid == 0 {
		// Fallback to jwt.Get if not in context (for backward compatibility)
		claimsInterface := jwt.Get(ctx)
		if claimsInterface == nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "Invalid user ID in token"})
			return
		}
		claims, ok := claimsInterface.(*AccessToken)
		if !ok || claims == nil || claims.ID == 0 {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "Invalid user ID in token"})
			return
		}
		uid = claims.ID
	}

	ctx.Values().Set("userID", uid)
	ctx.Next()
}

// AdminOnlyMiddleware ensures the requester has admin or super_admin role
func AdminOnlyMiddleware(ctx iris.Context) {
	// Try to get from context first (set by accessTokenVerifierMiddleware)
	uid, _ := ctx.Values().Get("userID").(uint)
	role, _ := ctx.Values().Get("userRole").(string)

	// Fallback to jwt.Get if not in context (for backward compatibility)
	if uid == 0 || role == "" {
		claimsInterface := jwt.Get(ctx)
		if claimsInterface == nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "unauthorized", "message": "authentication required"})
			return
		}

		claims, ok := claimsInterface.(*AccessToken)
		if !ok || claims == nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "unauthorized", "message": "invalid token"})
			return
		}
		uid = claims.ID
		role = claims.Role
	}

	if role != "admin" && role != "super_admin" {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{
			"error":     "forbidden",
			"message":   "admin access required",
			"user_role": role,
			"user_id":   uid,
		})
		return
	}
	// Ensure userID is available to downstream handlers
	ctx.Values().Set("userID", uid)
	ctx.Next()
}

// SuperAdminOnlyMiddleware ensures only super admins can access
func SuperAdminOnlyMiddleware(ctx iris.Context) {
	// Try to get from context first (set by accessTokenVerifierMiddleware)
	role, _ := ctx.Values().Get("userRole").(string)

	// Fallback to jwt.Get if not in context (for backward compatibility)
	if role == "" {
		claimsInterface := jwt.Get(ctx)
		if claimsInterface == nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "unauthorized", "message": "authentication required"})
			return
		}
		claims, ok := claimsInterface.(*AccessToken)
		if !ok || claims == nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			ctx.JSON(iris.Map{"error": "unauthorized", "message": "invalid token"})
			return
		}
		role = claims.Role
	}

	if role != "super_admin" {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden", "message": "super_admin access required"})
		return
	}
	ctx.Next()
}
