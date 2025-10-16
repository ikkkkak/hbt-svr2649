package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"net/http"
	"os"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// HidePropertyPublic - POST /api/properties/{id}/hide (Public; captures user if authenticated)
func HidePropertyPublic(ctx iris.Context) {
	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var input struct {
		Reason string `json:"reason" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Optional user extraction
	var userID *uint = nil
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = &id
		}
	}
	if userID == nil {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					id := claims.ID
					userID = &id
					fmt.Printf("🔍 Fallback auth (property hide) - User ID: %d\n", id)
				}
			}
		}
	}

	// Check duplicates (guard for nil userID - if anonymous allow write with nil to persist nothing per user)
	if userID != nil {
		var existing models.HiddenProperty
		if err := storage.DB.Where("property_id = ? AND user_id = ?", propertyID, *userID).First(&existing).Error; err == nil {
			ctx.StatusCode(http.StatusConflict)
			ctx.JSON(iris.Map{"error": "You have already hidden this property"})
			return
		}
	}

	rec := models.HiddenProperty{PropertyID: propertyID, UserID: userID, Reason: input.Reason}
	if err := storage.DB.Create(&rec).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to hide property"})
		return
	}
	fmt.Printf("✅ HiddenProperty created: user_id=%v property_id=%d id=%d\n", func() interface{} {
		if userID == nil {
			return nil
		}
		return *userID
	}(), propertyID, rec.ID)
	ctx.JSON(iris.Map{"success": true, "hidden": rec})
}

// ReportPropertyPublic - POST /api/properties/{id}/report (Public; requires user to be captured for persistence)
func ReportPropertyPublic(ctx iris.Context) {
	propertyID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var input struct {
		Reason      string `json:"reason" validate:"required"`
		Description string `json:"description"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Optional user extraction
	var reporterID *uint = nil
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			reporterID = &id
		}
	}
	if reporterID == nil {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					id := claims.ID
					reporterID = &id
					fmt.Printf("🔍 Fallback auth (property report) - User ID: %d\n", id)
				}
			}
		}
	}

	if reporterID != nil {
		var existing models.PropertyReport
		if err := storage.DB.Where("property_id = ? AND reporter_id = ?", propertyID, *reporterID).First(&existing).Error; err == nil {
			ctx.StatusCode(http.StatusConflict)
			ctx.JSON(iris.Map{"error": "You have already reported this property"})
			return
		}
	}

	rep := models.PropertyReport{PropertyID: propertyID, ReporterID: reporterID, Reason: input.Reason, Description: input.Description, Status: "pending"}
	if err := storage.DB.Create(&rep).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to report property"})
		return
	}
	ctx.JSON(iris.Map{"success": true, "report": rep})
}
