package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

// adminIdentityUserDTO returns safe user fields for admin identity review (no password).
func adminIdentityUserDTO(u *models.User, historyCount int) iris.Map {
	phone := ""
	if u.PhoneNumber != nil {
		phone = *u.PhoneNumber
	}
	isVerified := false
	if u.IsVerified != nil {
		isVerified = *u.IsVerified
	}
	row := iris.Map{
		"userId":             u.ID,
		"firstName":          u.FirstName,
		"lastName":           u.LastName,
		"email":              u.Email,
		"phoneNumber":        phone,
		"avatarURL":          u.AvatarURL,
		"role":               u.Role,
		"verificationStatus": u.VerificationStatus,
		"isVerified":         isVerified,
		"idType":             u.IDType,
		"idNumber":           u.IDNumber,
		"idFrontImage":       u.IDFrontImage,
		"idBackImage":        u.IDBackImage,
		"selfieImage":        u.SelfieImage,
		"brokerId":           services.BrokerIDString(u),
		"brokerStatus":       u.BrokerStatus,
		"createdAt":          u.CreatedAt,
		"updatedAt":          u.UpdatedAt,
		"historyCount":       historyCount,
	}
	return row
}

func adminIdentityVerificationBaseQuery() *gorm.DB {
	// Users who submitted identity docs or have verification history rows.
	return storage.DB.Model(&models.User{}).Where(
		`(
			COALESCE(TRIM(id_front_image), '') != '' OR
			COALESCE(TRIM(id_number), '') != '' OR
			COALESCE(TRIM(selfie_image), '') != '' OR
			EXISTS (
				SELECT 1 FROM identity_verifications iv
				WHERE iv.user_id = users.id
			)
		)`,
	)
}

func applyAdminIdentitySearch(query *gorm.DB, q string, userIDParam string) *gorm.DB {
	q = strings.TrimSpace(q)
	if uidParam := strings.TrimSpace(userIDParam); uidParam != "" {
		if uid, err := strconv.ParseUint(uidParam, 10, 64); err == nil {
			return query.Where("users.id = ?", uid)
		}
	}

	if q == "" {
		return query
	}

	if uid, err := strconv.ParseUint(q, 10, 64); err == nil {
		return query.Where(
			"users.id = ? OR LOWER(TRIM(broker_id)) = LOWER(?) OR id_number ILIKE ?",
			uid, q, "%"+q+"%",
		)
	}

	like := "%" + strings.ToLower(q) + "%"
	return query.Where(
		`LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ? OR LOWER(email) LIKE ?
		 OR LOWER(COALESCE(phone_number, '')) LIKE ? OR LOWER(id_number) LIKE ?
		 OR LOWER(broker_id) LIKE ?`,
		like, like, like, like, like, like,
	)
}

// GET /admin/identity-verifications?q=&user_id=&status=&page=&per_page=
func AdminListIdentityVerifications(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	if page < 1 {
		page = 1
	}
	perPage := ctx.URLParamIntDefault("per_page", 24)
	if perPage <= 0 || perPage > 100 {
		perPage = 24
	}

	q := strings.TrimSpace(ctx.URLParamDefault("q", ""))
	userIDParam := strings.TrimSpace(ctx.URLParamDefault("user_id", ""))
	status := strings.TrimSpace(strings.ToLower(ctx.URLParamDefault("status", "")))

	query := adminIdentityVerificationBaseQuery()
	query = applyAdminIdentitySearch(query, q, userIDParam)

	if status != "" {
		query = query.Where("LOWER(TRIM(verification_status)) = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	var users []models.User
	if err := query.Order("updated_at DESC").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&users).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	rows := make([]iris.Map, 0, len(users))
	for i := range users {
		u := &users[i]
		var historyCount int64
		storage.DB.Model(&models.IdentityVerification{}).
			Where("user_id = ?", u.ID).
			Count(&historyCount)
		rows = append(rows, adminIdentityUserDTO(u, int(historyCount)))
	}

	ctx.JSON(iris.Map{
		"data": rows,
		"meta": iris.Map{
			"page":     page,
			"per_page": perPage,
			"total":    total,
		},
		"links": iris.Map{},
	})
}

// GET /admin/identity-verifications/:user_id
func AdminGetIdentityVerificationUser(ctx iris.Context) {
	userID, err := ctx.Params().GetUint("user_id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid user id")
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "user not found")
		return
	}

	hasFootprint :=
		strings.TrimSpace(user.IDFrontImage) != "" ||
			strings.TrimSpace(user.IDNumber) != "" ||
			strings.TrimSpace(user.SelfieImage) != ""

	var historyCount int64
	storage.DB.Model(&models.IdentityVerification{}).Where("user_id = ?", userID).Count(&historyCount)
	if !hasFootprint && historyCount == 0 {
		utils.JSONError(ctx, http.StatusNotFound, "no_verification", "no identity verification on file for this user")
		return
	}

	var verifs []models.IdentityVerification
	storage.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&verifs)

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"user":          adminIdentityUserDTO(&user, int(historyCount)),
			"verifications": verifs,
		},
		"meta":  iris.Map{},
		"links": iris.Map{},
	})
}
