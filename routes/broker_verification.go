package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
)

type brokerVerificationSubmitInput struct {
	ProfilePhotoURL string   `json:"profile_photo_url"`
	IDType          string   `json:"id_type"` // passport | national_id
	IDFrontImage    string   `json:"id_front_image"`
	IDBackImage     string   `json:"id_back_image"`
	SelfieImage     string   `json:"selfie_image"`
	LicenseURL      string   `json:"license_url"`
	SpokenLanguages []string `json:"spoken_languages"`
}

func normalizeBrokerIDType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "passport":
		return "passport"
	case "national_id", "id_card", "id", "national id", "carte_identite":
		return "national_id"
	default:
		return ""
	}
}

func uploadBrokerImageIfNeeded(raw, folder string, userID uint) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	res := storage.UploadBase64ImageOptimized(u, folder+"/"+strconv.FormatUint(uint64(userID), 10))
	if res["url"] != "" {
		return res["url"]
	}
	return u
}

func brokerStatusPayload(u *models.User) iris.Map {
	verified := services.IsVerifiedBroker(u)
	showProfile := services.BrokerProfileVisibleOnListings(u)
	out := iris.Map{
		"status":                        strings.TrimSpace(u.BrokerStatus),
		"broker_id":                     services.BrokerIDString(u),
		"is_verified":                   verified,
		"show_profile_on_listings":      showProfile,
		"broker_profile_visible":        showProfile,
		"submitted_at":                  u.BrokerSubmittedAt,
		"verified_at":                   u.BrokerVerifiedAt,
		"rejection_notes":               strings.TrimSpace(u.BrokerRejectionNotes),
		"spoken_languages":              parseUserStringJSON(u.BrokerSpokenLanguages),
	}
	if verified {
		out["expected_views_boost_pct"] = 35
		out["expected_ranking_boost"] = "priority"
	} else if strings.EqualFold(u.BrokerStatus, "none") || u.BrokerStatus == "" {
		out["expected_views_boost_pct"] = 35
		out["expected_inquiry_boost_pct"] = 22
		out["estimated_monthly_leads_mru"] = 85000
	}
	return out
}

func parseUserStringJSON(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	clean := make([]string, 0, len(out))
	for _, s := range out {
		if t := strings.TrimSpace(s); t != "" {
			clean = append(clean, t)
		}
	}
	return clean
}

// GET /api/user/broker-verification
func GetBrokerVerificationStatus(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "User not found")
		return
	}
	ctx.JSON(iris.Map{"data": brokerStatusPayload(&user)})
}

// PATCH /api/user/broker-verification/settings
func UpdateBrokerVerificationSettings(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var body struct {
		ShowProfileOnListings *bool `json:"show_profile_on_listings"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.ShowProfileOnListings == nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "show_profile_on_listings is required")
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if !services.IsVerifiedBroker(&user) {
		utils.JSONError(ctx, http.StatusForbidden, "not_verified", "Broker verification required before changing this setting")
		return
	}

	user.BrokerShowProfileOnListings = *body.ShowProfileOnListings
	if err := storage.DB.Save(&user).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "save_failed", "Could not save settings")
		return
	}

	ctx.JSON(iris.Map{
		"message": "Settings updated",
		"data":    brokerStatusPayload(&user),
	})
}

// POST /api/user/broker-verification
func SubmitBrokerVerification(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var input brokerVerificationSubmitInput
	if err := ctx.ReadJSON(&input); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "Invalid JSON")
		return
	}

	if strings.TrimSpace(input.ProfilePhotoURL) == "" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "profile_required", "A profile photo is required")
		return
	}
	idType := normalizeBrokerIDType(input.IDType)
	if idType == "" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "id_type_required", "Choose passport or national ID card")
		return
	}
	switch idType {
	case "passport":
		if strings.TrimSpace(input.IDFrontImage) == "" {
			utils.JSONError(ctx, http.StatusUnprocessableEntity, "passport_required", "Passport photo is required")
			return
		}
	case "national_id":
		if strings.TrimSpace(input.IDFrontImage) == "" || strings.TrimSpace(input.IDBackImage) == "" {
			utils.JSONError(ctx, http.StatusUnprocessableEntity, "id_card_required", "National ID front and back are required")
			return
		}
	}
	if len(input.SpokenLanguages) == 0 {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "languages_required", "Select at least one language you speak")
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if services.IsVerifiedBroker(&user) {
		utils.JSONError(ctx, http.StatusConflict, "already_verified", "Your broker identity is already verified")
		return
	}
	if strings.EqualFold(user.BrokerStatus, "pending") {
		utils.JSONError(ctx, http.StatusConflict, "already_pending", "Your application is already under review")
		return
	}

	avatar := uploadBrokerImageIfNeeded(input.ProfilePhotoURL, "broker/profile", userID)
	idFront := uploadBrokerImageIfNeeded(input.IDFrontImage, "broker/id_front", userID)
	idBack := uploadBrokerImageIfNeeded(input.IDBackImage, "broker/id_back", userID)
	selfie := uploadBrokerImageIfNeeded(input.SelfieImage, "broker/selfie", userID)
	license := uploadBrokerImageIfNeeded(input.LicenseURL, "broker/license", userID)

	langsJSON, _ := json.Marshal(input.SpokenLanguages)
	now := time.Now().UTC()
	user.AvatarURL = avatar
	user.IDType = idType
	user.IDFrontImage = idFront
	user.IDBackImage = idBack
	user.SelfieImage = selfie
	if license != "" {
		user.BrokerLicenseURL = license
	}
	user.BrokerSpokenLanguages = datatypes.JSON(langsJSON)
	user.BrokerStatus = "pending"
	user.BrokerSubmittedAt = &now
	user.BrokerRejectionNotes = ""

	if err := storage.DB.Save(&user).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "save_failed", "Could not submit application")
		return
	}

	iv := models.IdentityVerification{
		UserID:       user.ID,
		DocumentType: "broker_identity",
		DocumentURL:  idFront,
		Status:       "pending",
		Notes:        "Broker identity verification submitted",
	}
	storage.DB.Create(&iv)

	ctx.JSON(iris.Map{
		"message": "Application submitted. We typically review within 24–48 hours.",
		"data":    brokerStatusPayload(&user),
	})
}

// GET /api/admin/broker-verifications/pending
func AdminListPendingBrokerVerifications(ctx iris.Context) {
	var users []models.User
	if err := storage.DB.
		Where("broker_status = ?", "pending").
		Order("broker_submitted_at ASC NULLS LAST, updated_at ASC").
		Limit(200).
		Find(&users).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}

	items := make([]iris.Map, 0, len(users))
	for _, u := range users {
		items = append(items, iris.Map{
			"id":                u.ID,
			"firstName":         u.FirstName,
			"lastName":          u.LastName,
			"email":             u.Email,
			"phoneNumber":       u.PhoneNumber,
			"avatarURL":         u.AvatarURL,
			"broker_status":     u.BrokerStatus,
			"broker_submitted_at": u.BrokerSubmittedAt,
			"spoken_languages":  parseUserStringJSON(u.BrokerSpokenLanguages),
			"id_type":           u.IDType,
			"id_front_image":    u.IDFrontImage,
			"id_back_image":     u.IDBackImage,
			"selfie_image":      u.SelfieImage,
			"license_url":       u.BrokerLicenseURL,
		})
	}

	ctx.JSON(iris.Map{"data": items, "meta": iris.Map{"count": len(items)}})
}

// POST /api/admin/users/:id/broker-verification { status: approved|rejected, notes? }
func AdminReviewBrokerVerification(ctx iris.Context) {
	adminID, _ := ctx.Values().Get("userID").(uint)
	targetID, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}

	var body struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_payload", "Invalid JSON")
		return
	}
	status := strings.ToLower(strings.TrimSpace(body.Status))
	if status != "approved" && status != "rejected" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_status", "status must be approved or rejected")
		return
	}

	var user models.User
	if err := storage.DB.First(&user, targetID).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if !strings.EqualFold(user.BrokerStatus, "pending") {
		utils.JSONError(ctx, http.StatusConflict, "not_pending", "User has no pending broker application")
		return
	}

	before := user
	now := time.Now().UTC()
	user.BrokerStatus = status
	user.BrokerVerifiedBy = &adminID

	if status == "approved" {
		if services.BrokerIDString(&user) == "" {
			brokerID, genErr := services.GenerateBrokerID()
			if genErr != nil {
				utils.JSONError(ctx, http.StatusInternalServerError, "id_generation_failed", genErr.Error())
				return
			}
			user.BrokerID = &brokerID
		}
		user.BrokerVerifiedAt = &now
		user.TrueBroker = true
		user.BrokerRejectionNotes = ""
		v := true
		user.IsVerified = &v
		user.VerificationStatus = "verified"
	} else {
		user.BrokerRejectionNotes = strings.TrimSpace(body.Notes)
		user.BrokerVerifiedAt = nil
	}

	if err := storage.DB.Save(&user).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	iv := models.IdentityVerification{
		UserID:       user.ID,
		DocumentType: "broker_identity_review",
		Status:       status,
		Notes:        strings.TrimSpace(body.Notes),
	}
	storage.DB.Create(&iv)
	utils.Audit(ctx, "broker.verify", "user", user.ID, before, user)

	ctx.JSON(iris.Map{
		"message": "Broker verification updated",
		"data":    brokerStatusPayload(&user),
	})
}
