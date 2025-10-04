package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
)

// GET /admin/groups?creator_id=&active=&min_members=&page=&per_page=
func AdminListGroups(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	perPage := ctx.URLParamIntDefault("per_page", 25)
	if perPage <= 0 || perPage > 100 {
		perPage = 25
	}

	creatorID := ctx.URLParamDefault("creator_id", "")
	active := ctx.URLParamDefault("active", "")
	minMembers := ctx.URLParamDefault("min_members", "")

	q := storage.DB.Model(&models.ExperienceGroup{})
	if creatorID != "" {
		q = q.Where("creator_id = ?", creatorID)
	}
	if active != "" {
		if active == "true" {
			q = q.Where("is_active = true")
		}
		if active == "false" {
			q = q.Where("is_active = false")
		}
	}

	// members count filter
	if minMembers != "" {
		if n, err := strconv.Atoi(minMembers); err == nil {
			q = q.Joins("LEFT JOIN experience_group_members egm ON egm.group_id = experience_groups.id").
				Group("experience_groups.id").
				Having("COUNT(egm.id) >= ?", n)
		}
	}

	var total int64
	q.Count(&total)

	var groups []models.ExperienceGroup
	if err := q.Preload("Members").Offset((page - 1) * perPage).Limit(perPage).Order("created_at DESC").Find(&groups).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	utils.JSONPage(ctx, groups, page, perPage, total)
}

// GET /admin/groups/:id
func AdminGetGroup(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var g models.ExperienceGroup
	if err := storage.DB.Preload("Members").First(&g, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "group not found")
		return
	}
	ctx.JSON(iris.Map{"data": g, "meta": iris.Map{}, "links": iris.Map{}})
}

// PATCH /admin/groups/:id { lock: bool, delete: bool, remove_member_id?: number }
func AdminUpdateGroup(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var body struct {
		Lock           *bool  `json:"lock"`
		Delete         *bool  `json:"delete"`
		RemoveMemberID *uint  `json:"remove_member_id"`
		Note           string `json:"note"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "invalid body")
		return
	}
	var g models.ExperienceGroup
	if err := storage.DB.First(&g, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "group not found")
		return
	}
	before := g

	if body.Delete != nil && *body.Delete {
		if err := storage.DB.Delete(&g).Error; err != nil {
			utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		utils.Audit(ctx, "group.delete", "group", id, before, nil)
		ctx.StatusCode(http.StatusNoContent)
		return
	}

	if body.Lock != nil {
		g.Status = "locked"
	}
	if err := storage.DB.Save(&g).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	if body.RemoveMemberID != nil {
		storage.DB.Where("group_id = ? AND user_id = ?", g.ID, *body.RemoveMemberID).Delete(&models.ExperienceGroupMember{})
	}

	utils.Audit(ctx, "group.update", "group", g.ID, before, g)
	ctx.JSON(iris.Map{"data": g})
}
