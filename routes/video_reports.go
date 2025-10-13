package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// ReportVideo - POST /api/videos/{id}/report
func ReportVideo(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
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

	// Check if video exists
	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}

	// Check if user already reported this video
	var existingReport models.VideoReport
	if err := storage.DB.Where("video_id = ? AND reporter_id = ?", videoID, userID).First(&existingReport).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "You have already reported this video"})
		return
	}

	// Create report
	report := models.VideoReport{
		VideoID:     videoID,
		ReporterID:  &userID,
		Reason:      input.Reason,
		Description: input.Description,
		Status:      "pending",
	}

	if err := storage.DB.Create(&report).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create report"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Video reported successfully",
		"report":  report,
	})
}

// FlagUser - POST /api/users/{id}/flag
func FlagUser(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	flaggedUserID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	// Can't flag yourself
	if userID == flaggedUserID {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Cannot flag yourself"})
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

	// Check if user exists
	var flaggedUser models.User
	if err := storage.DB.First(&flaggedUser, flaggedUserID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found"})
		return
	}

	// Check if user already flagged this user
	var existingFlag models.UserFlag
	if err := storage.DB.Where("flagged_user_id = ? AND flagger_id = ?", flaggedUserID, userID).First(&existingFlag).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "You have already flagged this user"})
		return
	}

	// Create flag
	flag := models.UserFlag{
		FlaggedUserID: flaggedUserID,
		FlaggerID:     &userID,
		Reason:        input.Reason,
		Description:   input.Description,
		Status:        "active",
	}

	if err := storage.DB.Create(&flag).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to flag user"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "User flagged successfully",
		"flag":    flag,
	})
}

// HideVideo - POST /api/videos/{id}/hide
func HideVideo(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	var input struct {
		Reason string `json:"reason" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if video exists
	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}

	// Check if user already hid this video
	var existingHidden models.HiddenVideo
	if err := storage.DB.Where("video_id = ? AND user_id = ?", videoID, userID).First(&existingHidden).Error; err == nil {
		ctx.StatusCode(http.StatusConflict)
		ctx.JSON(iris.Map{"error": "You have already hidden this video"})
		return
	}

	// Create hidden video record
	hiddenVideo := models.HiddenVideo{
		VideoID: videoID,
		UserID:  &userID,
		Reason:  input.Reason,
	}

	if err := storage.DB.Create(&hiddenVideo).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to hide video"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Video hidden successfully",
		"hidden":  hiddenVideo,
	})
}

// GetFlaggedVideos - GET /api/admin/flagged-videos (Admin only)
func GetFlaggedVideos(ctx iris.Context) {
	// This would require admin authentication
	// For now, we'll implement basic functionality

	var reports []models.VideoReport
	if err := storage.DB.
		Preload("Video").
		Preload("Video.User").
		Preload("Video.Property").
		Preload("Reporter").
		Order("created_at DESC").
		Find(&reports).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch flagged videos"})
		return
	}

	// Debug: Log the first report to see what's being returned
	if len(reports) > 0 {
		fmt.Printf("🔍 DEBUG: First report video data: %+v\n", reports[0].Video)
		if reports[0].Video.User.ID != 0 {
			fmt.Printf("🔍 DEBUG: Video user data: %+v\n", reports[0].Video.User)
		} else {
			fmt.Printf("🔍 DEBUG: Video user data is empty\n")
		}
	}

	ctx.JSON(iris.Map{
		"success": true,
		"reports": reports,
	})
}

// UpdateReportStatus - PUT /api/admin/reports/{id}/status (Admin only)
func UpdateReportStatus(ctx iris.Context) {
	reportID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid report ID"})
		return
	}

	var input struct {
		Status     string `json:"status" validate:"required,oneof=pending reviewed resolved dismissed"`
		AdminNotes string `json:"admin_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var report models.VideoReport
	if err := storage.DB.First(&report, reportID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Report not found"})
		return
	}

	report.Status = input.Status
	report.AdminNotes = input.AdminNotes

	if err := storage.DB.Save(&report).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update report status"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Report status updated successfully",
		"report":  report,
	})
}

// Public versions of video reporting functions (no authentication required)

// ReportVideoPublic - POST /api/videos/{id}/report (Public)
func ReportVideoPublic(ctx iris.Context) {
	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
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

	// Check if video exists
	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}

	// Try to get user ID if authenticated (optional)
	var reporterID *uint = nil
	if v := ctx.Values().Get("userID"); v != nil {
		switch t := v.(type) {
		case uint:
			u := t
			reporterID = &u
		case int:
			u := uint(t)
			reporterID = &u
		case float64:
			u := uint(t)
			reporterID = &u
		case string:
			if parsed, err := strconv.ParseUint(t, 10, 64); err == nil {
				u := uint(parsed)
				reporterID = &u
			}
		}
		if reporterID != nil {
			fmt.Printf("🔍 User authenticated for report - User ID: %d\n", *reporterID)
		}
	}
	// Fallback: parse Authorization header directly if still nil
	if reporterID == nil {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					uid := claims.ID
					reporterID = &uid
					fmt.Printf("🔍 Fallback auth (report) - User ID: %d\n", uid)
				}
			}
		} else {
			fmt.Printf("🔍 Anonymous report - no user ID\n")
		}
	}

	// Create report (authenticated or anonymous)
	report := models.VideoReport{
		VideoID:     videoID,
		ReporterID:  reporterID, // User ID if authenticated, nil if anonymous
		Reason:      input.Reason,
		Description: input.Description,
		Status:      "pending",
	}

	if err := storage.DB.Create(&report).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create report"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Video reported successfully",
		"report":  report,
	})
}

// FlagUserPublic - POST /api/users/{id}/flag (Public)
func FlagUserPublic(ctx iris.Context) {
	flaggedUserID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
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

	// Check if user exists
	var flaggedUser models.User
	if err := storage.DB.First(&flaggedUser, flaggedUserID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found"})
		return
	}

	// Create flag without user authentication
	flag := models.UserFlag{
		FlaggedUserID: flaggedUserID,
		FlaggerID:     nil, // nil indicates anonymous flag
		Reason:        input.Reason,
		Description:   input.Description,
		Status:        "active",
	}

	if err := storage.DB.Create(&flag).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to flag user"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "User flagged successfully",
		"flag":    flag,
	})
}

// HideVideoPublic - POST /api/videos/{id}/hide (Public)
func HideVideoPublic(ctx iris.Context) {
	videoID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid video ID"})
		return
	}

	var input struct {
		Reason string `json:"reason" validate:"required"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Check if video exists
	var video models.Video
	if err := storage.DB.First(&video, videoID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Video not found"})
		return
	}

	// Try to get user ID if authenticated (optional)
	var userID *uint = nil
	if v := ctx.Values().Get("userID"); v != nil {
		switch t := v.(type) {
		case uint:
			u := t
			userID = &u
		case int:
			u := uint(t)
			userID = &u
		case float64:
			u := uint(t)
			userID = &u
		case string:
			if parsed, err := strconv.ParseUint(t, 10, 64); err == nil {
				u := uint(parsed)
				userID = &u
			}
		}
		if userID != nil {
			fmt.Printf("🔍 User authenticated for hide - User ID: %d\n", *userID)
		}
	}
	// Fallback: parse Authorization header directly if still nil
	if userID == nil {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					uid := claims.ID
					userID = &uid
					fmt.Printf("🔍 Fallback auth (hide) - User ID: %d\n", uid)
				}
			}
		} else {
			fmt.Printf("🔍 Anonymous hide - no user ID\n")
		}
	}

	// Create hidden video record (authenticated or anonymous)
	hiddenVideo := models.HiddenVideo{
		VideoID: videoID,
		UserID:  userID, // User ID if authenticated, nil if anonymous
		Reason:  input.Reason,
	}

	if err := storage.DB.Create(&hiddenVideo).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to hide video"})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Video hidden successfully",
		"hidden":  hiddenVideo,
	})
}
