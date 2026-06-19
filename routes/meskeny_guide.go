package routes

import (
	"os"
	"strconv"
	"strings"

	"apartments-clone-server/models"
	"apartments-clone-server/services/meskenyguide"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"
)

func syncHostLocaleFromRequest(ctx iris.Context, userID uint) {
	meskenyguide.SyncHostLocaleAsync(userID, ctx.GetHeader("X-App-Locale"))
}

func canAccessSaleListing(userID, propertySaleID uint) bool {
	var ps models.PropertySale
	if err := storage.DB.First(&ps, propertySaleID).Error; err != nil {
		return false
	}
	if ps.OwnerID != nil && *ps.OwnerID == userID {
		return true
	}
	if ps.OrganizationID != nil {
		var org models.Organization
		if storage.DB.Select("owner_id").First(&org, *ps.OrganizationID).Error == nil && org.OwnerID == userID {
			return true
		}
		var agent models.Agent
		if storage.DB.Where("user_id = ? AND organization_id = ? AND status = ?", userID, *ps.OrganizationID, "approved").First(&agent).Error == nil {
			return true
		}
	}
	return false
}

// GET /api/host/guide/feed
func GetGuideFeed(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	page, _ := ctx.URLParamInt("page")
	if page < 1 {
		page = 1
	}
	limit, _ := ctx.URLParamInt("limit")
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	q := storage.DB.Model(&models.GuideComment{}).
		Where("host_id = ? AND parent_id IS NULL", userID)

	if sev := strings.TrimSpace(ctx.URLParam("severity")); sev != "" {
		q = q.Where("severity = ?", sev)
	}
	if cat := strings.TrimSpace(ctx.URLParam("category")); cat != "" {
		q = q.Where("category = ?", cat)
	}
	if lid, err := ctx.URLParamInt("listing_id"); err == nil && lid > 0 {
		q = q.Where("property_sale_id = ?", lid)
	}
	if ctx.URLParam("needs_action") == "1" {
		q = q.Where("status IN ?", []string{models.GuideStatusUnread, models.GuideStatusRead})
		q = q.Where("severity IN ?", []string{models.GuideSeverityAction, models.GuideSeverityUrgent})
	}

	sort := ctx.URLParamDefault("sort", "newest")
	if sort == "needs_action" {
		q = q.Order("CASE WHEN status = 'unread' THEN 0 WHEN severity = 'urgent' THEN 1 WHEN severity = 'action' THEN 2 ELSE 3 END, created_at DESC")
	} else {
		q = q.Order("created_at DESC")
	}

	var total int64
	q.Count(&total)

	var comments []models.GuideComment
	q.Preload("PropertySale", func(db *gorm.DB) *gorm.DB {
		return db.Select("id", "title", "images", "city", "view_count")
	}).Offset(offset).Limit(limit).Find(&comments)

	ctx.JSON(iris.Map{
		"comments": comments,
		"page":     page,
		"limit":    limit,
		"total":    total,
	})
}

// GET /api/host/guide/listings/:propertySaleId/comments
func GetListingGuideComments(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertySaleID, err := ctx.Params().GetUint("propertySaleId")
	if err != nil || propertySaleID == 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid listing id"})
		return
	}
	if !canAccessSaleListing(userID, propertySaleID) {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden"})
		return
	}

	highlightID, _ := ctx.URLParamInt("highlight")

	var comments []models.GuideComment
	storage.DB.Where("property_sale_id = ? AND host_id = ? AND parent_id IS NULL", propertySaleID, userID).
		Order("created_at ASC").
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Find(&comments)

	meskenyguide.MarkListingCommentsRead(userID, propertySaleID)

	ctx.JSON(iris.Map{
		"comments":    comments,
		"highlightId": highlightID,
	})
}

// GET /api/host/guide/comments/:id
func GetGuideComment(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		return
	}
	var c models.GuideComment
	if err := storage.DB.Preload("Replies").First(&c, id).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		return
	}
	if c.HostID != userID {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}
	ctx.JSON(iris.Map{"comment": c})
}

// POST /api/host/guide/comments/:id/implement
func ImplementGuideComment(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	id, _ := ctx.Params().GetUint("id")
	if err := meskenyguide.MarkImplemented(id, userID); err != nil {
		if meskenyguide.IsNotFound(err) {
			ctx.StatusCode(iris.StatusNotFound)
		} else {
			ctx.StatusCode(iris.StatusForbidden)
		}
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"ok": true})
}

// POST /api/host/guide/comments/:id/dismiss
func DismissGuideComment(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	id, _ := ctx.Params().GetUint("id")
	if err := meskenyguide.MarkDismissed(id, userID); err != nil {
		if meskenyguide.IsNotFound(err) {
			ctx.StatusCode(iris.StatusNotFound)
		} else {
			ctx.StatusCode(iris.StatusForbidden)
		}
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"ok": true})
}

// POST /api/host/guide/comments/:id/reply
func ReplyGuideComment(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	id, _ := ctx.Params().GetUint("id")
	var req struct {
		Body string `json:"body"`
	}
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Body) == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "body required"})
		return
	}
	reply, err := meskenyguide.AddHostReply(id, userID, strings.TrimSpace(req.Body))
	if err != nil {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"error": "forbidden"})
		return
	}
	ctx.JSON(iris.Map{"reply": reply})
}

// GET /api/host/guide/listing-previews?ids=1,2,3
func GetGuideListingPreviews(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	syncHostLocaleFromRequest(ctx, userID)
	idsParam := strings.TrimSpace(ctx.URLParam("ids"))
	if idsParam == "" {
		ctx.JSON(iris.Map{"previews": map[uint]meskenyguide.ListingGuidePreview{}})
		return
	}
	var ids []uint
	for _, part := range strings.Split(idsParam, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseUint(part, 10, 64); err == nil && n > 0 {
			ids = append(ids, uint(n))
		}
	}
	previews := meskenyguide.GetListingPreviews(userID, ids)
	ctx.JSON(iris.Map{"previews": previews})
}

// GET /api/host/guide/grouped
func GetGuideGrouped(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	syncHostLocaleFromRequest(ctx, userID)
	limit, _ := ctx.URLParamInt("limit")
	groups := meskenyguide.GetGuideGroupedByCategory(userID, limit)
	ctx.JSON(iris.Map{"groups": groups})
}

// GET /api/host/guide/unread-count
func GetGuideUnreadCount(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	var count int64
	storage.DB.Model(&models.GuideComment{}).
		Where("host_id = ? AND parent_id IS NULL AND status = ?", userID, models.GuideStatusUnread).
		Count(&count)
	ctx.JSON(iris.Map{"count": count})
}

// POST /api/host/guide/dev/trigger — manual trigger for QA (non-prod)
func DevTriggerGuide(ctx iris.Context) {
	if strings.EqualFold(os.Getenv("ENV"), "production") {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}
	propertySaleID, _ := ctx.URLParamInt("property_sale_id")
	trigger := ctx.URLParamDefault("trigger", models.GuideTriggerFirstInquiry)
	if propertySaleID <= 0 {
		ctx.StatusCode(iris.StatusBadRequest)
		return
	}
	if err := meskenyguide.ManualTriggerListing(uint(propertySaleID), trigger); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"ok": true, "trigger": trigger, "listing_id": propertySaleID})
}

// ParseGuideDeepLink extracts listing + comment ids from meskeny.mr/listing/{id}/guide/{commentId}
func ParseGuideDeepLink(link string) (listingID, commentID uint) {
	link = strings.TrimSpace(link)
	parts := strings.Split(strings.TrimPrefix(link, "meskeny.mr/listing/"), "/")
	if len(parts) < 3 {
		return 0, 0
	}
	lid, _ := strconv.ParseUint(parts[0], 10, 64)
	if parts[1] == "guide" && len(parts) >= 2 {
		cid, _ := strconv.ParseUint(parts[2], 10, 64)
		return uint(lid), uint(cid)
	}
	return uint(lid), 0
}
