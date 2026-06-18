package routes

import (
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
)

// BatchGetPropertySales returns lightweight cards for many listing IDs in one request.
// POST /api/batch/property-sales  { "ids": [1,2,3], "fields": "card", "lang": "en" }
func BatchGetPropertySales(ctx iris.Context) {
	var body struct {
		IDs    []uint `json:"ids"`
		Fields string `json:"fields"`
		Lang   string `json:"lang"`
	}
	if err := ctx.ReadJSON(&body); err != nil || len(body.IDs) == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "ids required"})
		return
	}
	if len(body.IDs) > 50 {
		body.IDs = body.IDs[:50]
	}

	userID := optionalUserIDFromContext(ctx)
	lang := strings.ToLower(strings.TrimSpace(body.Lang))
	if lang == "" {
		lang = "en"
	}
	fieldsParam := strings.TrimSpace(body.Fields)
	if fieldsParam == "" {
		fieldsParam = "card"
	}
	cardFields := fieldsParam != "full" && fieldsParam != "all"

	q := storage.DB.Model(&models.PropertySale{}).
		Where("property_sales.id IN ?", body.IDs).
		Where("property_sales.deleted_at IS NULL").
		Where("(property_sales.status = ? OR property_sales.is_published = ? OR property_sales.is_sold = ?)", "published", true, true)

	if cardFields {
		q = q.Omit(
			"Description", "DescriptionTranslations",
			"FloorPlans", "Neighborhood",
			"Features", "Amenities", "HostPrivateNote", "VerificationNotes", "VirtualTour",
		)
		q = q.Preload("Organization", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name", "logo", "banner_image", "owner_id")
		})
		q = q.Preload("Owner", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "first_name", "last_name", "avatar_url")
		})
	} else {
		q = q.Preload("Organization").Preload("Owner")
	}

	if userID > 0 {
		q = q.Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID)
	}

	var properties []models.PropertySale
	if err := q.Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed to fetch listings"})
		return
	}

	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		if !cardFields {
			p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
		}
	}
	redactPropertySaleSliceForViewer(properties, userID)
	if cardFields {
		expandPropertySaleGalleries(properties)
		applyCardFeedTrim(properties)
	}

	byID := make(map[uint]models.PropertySale, len(properties))
	for _, p := range properties {
		byID[p.ID] = p
	}
	ordered := make([]models.PropertySale, 0, len(body.IDs))
	for _, id := range body.IDs {
		if p, ok := byID[id]; ok {
			ordered = append(ordered, p)
		}
	}

	payload := iris.Map{
		"properties": ordered,
		"count":      len(ordered),
	}
	utils.RespondJSONWithETag(ctx, http.StatusOK, payload)
}
