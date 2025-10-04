package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
)

// ListUsers - GET /admin/users?role=&q=&page=&per_page=
func AdminListUsers(ctx iris.Context) {
	// Basic pagination
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	var users []models.User
	q := strings.TrimSpace(ctx.URLParamDefault("q", ""))
	role := strings.TrimSpace(ctx.URLParamDefault("role", ""))

	query := storage.DB.Model(&models.User{})
	if role != "" {
		query = query.Where("role = ?", role)
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query = query.Where("lower(first_name) LIKE ? OR lower(last_name) LIKE ? OR lower(email) LIKE ?", like, like, like)
	}

	var total int64
	query.Count(&total)
	query = query.Offset((page - 1) * perPage).Limit(perPage)
	if err := query.Find(&users).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "server_error", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"data":  users,
		"meta":  iris.Map{"page": page, "per_page": perPage, "total": total},
		"links": iris.Map{},
	})
}

// Change role - PATCH /admin/users/:id/role
func AdminChangeUserRole(ctx iris.Context) {
	// Only super_admin
	// Middleware enforces super admin. Here perform change.
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid_id"})
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Role == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid_role"})
		return
	}

	var user models.User
	if err := storage.DB.First(&user, id).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "not_found"})
		return
	}

	before := user
	user.Role = body.Role
	if err := storage.DB.Save(&user).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "server_error"})
		return
	}

	// Audit
	utils.Audit(ctx, "user.role_update", "user", user.ID, before, user)

	ctx.JSON(iris.Map{"data": user})
}
