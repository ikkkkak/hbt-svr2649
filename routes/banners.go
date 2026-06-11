package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"net/http"
	"sync"

	"github.com/kataras/iris/v12"
)

var (
	bannersSchemaOnce       sync.Once
	bannersHasPositionField bool
)

func detectBannersPositionColumn() bool {
	bannersSchemaOnce.Do(func() {
		var count int64
		err := storage.DB.Raw(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'banners' AND column_name = 'position'",
		).Scan(&count).Error
		if err != nil {
			bannersHasPositionField = false
			return
		}
		bannersHasPositionField = count > 0
	})
	return bannersHasPositionField
}

// BannerResponse ensures width/height are always in the JSON output for the app
type BannerResponse struct {
	ID        uint   `json:"id"`
	ImageURL  string `json:"image_url"`
	LinkURL   string `json:"link_url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SortOrder int    `json:"sort_order"`
	IsActive  bool   `json:"is_active"`
}

// GetBanners returns all active banners for the property sale feed (public)
func GetBanners(ctx iris.Context) {
	position := ctx.URLParamDefault("position", "")
	var banners []models.Banner
	q := storage.DB.Where("is_active = ?", true)
	if position != "" && detectBannersPositionColumn() {
		q = q.Where("position = ?", position)
	}
	if err := q.Order("sort_order ASC, id ASC").Find(&banners).Error; err != nil {
		log.Printf("❌ [GetBanners] DB error: %v", err)
		// Banners should never break the feed experience.
		ctx.JSON(iris.Map{"banners": []BannerResponse{}})
		return
	}
	// Map to explicit response DTO so width/height from DB are passed through exactly
	out := make([]BannerResponse, 0, len(banners))
	for _, b := range banners {
		out = append(out, BannerResponse{
			ID:        b.ID,
			ImageURL:  b.ImageURL,
			LinkURL:   b.LinkURL,
			Width:     b.Width,
			Height:    b.Height,
			SortOrder: b.SortOrder,
			IsActive:  b.IsActive,
		})
	}
	log.Printf("📣 [GetBanners] Returning %d banner(s), position=%q", len(out), position)
	ctx.JSON(iris.Map{"banners": out})
}

// ListAdminBanners returns all banners for admin (including inactive)
func ListAdminBanners(ctx iris.Context) {
	var banners []models.Banner
	if err := storage.DB.Order("sort_order ASC, id ASC").Find(&banners).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "Failed to fetch banners"})
		return
	}
	ctx.JSON(iris.Map{"banners": banners})
}

// CreateBannerInput for admin create
type CreateBannerInput struct {
	ImageURL  string `json:"image_url"`
	LinkURL   string `json:"link_url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SortOrder int    `json:"sort_order"`
}

// CreateBanner creates a new banner (admin only)
func CreateBanner(ctx iris.Context) {
	var in CreateBannerInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "Invalid payload"})
		return
	}
	if in.ImageURL == "" {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "image_url is required"})
		return
	}
	width, height := in.Width, in.Height
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 200
	}
	banner := models.Banner{
		ImageURL:  in.ImageURL,
		LinkURL:   in.LinkURL,
		Width:     width,
		Height:    height,
		SortOrder: in.SortOrder,
		IsActive:  true,
	}
	if err := storage.DB.Create(&banner).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "Failed to create banner"})
		return
	}
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(banner)
}

// UpdateBannerInput for admin update
type UpdateBannerInput struct {
	ImageURL  *string `json:"image_url"`
	LinkURL   *string `json:"link_url"`
	Width     *int    `json:"width"`
	Height    *int    `json:"height"`
	SortOrder *int    `json:"sort_order"`
	IsActive  *bool   `json:"is_active"`
}

// UpdateBanner updates a banner (admin only)
func UpdateBanner(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "Invalid banner ID"})
		return
	}
	var in UpdateBannerInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "Invalid payload"})
		return
	}
	var banner models.Banner
	if err := storage.DB.First(&banner, id).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "Banner not found"})
		return
	}
	if in.ImageURL != nil {
		banner.ImageURL = *in.ImageURL
	}
	if in.LinkURL != nil {
		banner.LinkURL = *in.LinkURL
	}
	if in.Width != nil && *in.Width > 0 {
		banner.Width = *in.Width
	}
	if in.Height != nil && *in.Height > 0 {
		banner.Height = *in.Height
	}
	if in.SortOrder != nil {
		banner.SortOrder = *in.SortOrder
	}
	if in.IsActive != nil {
		banner.IsActive = *in.IsActive
	}
	if err := storage.DB.Save(&banner).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "Failed to update banner"})
		return
	}
	ctx.JSON(banner)
}

// DeleteBanner soft-deletes a banner (admin only)
func DeleteBanner(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "Invalid banner ID"})
		return
	}
	if err := storage.DB.Delete(&models.Banner{}, id).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "Failed to delete banner"})
		return
	}
	ctx.StatusCode(http.StatusNoContent)
}
