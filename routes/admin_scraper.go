package routes

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
)

// POST /admin/scraper/sources — register a URL for MeskenyGPT to scrape.
func AdminCreateScrapedSource(ctx iris.Context) {
	var body struct {
		Name      string         `json:"name"`
		URL       string         `json:"url"`
		Kind      string         `json:"kind"`
		Selectors datatypes.JSON `json:"selectors"`
		Active    *bool          `json:"active"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.URL == "" || body.Name == "" {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "name and url are required")
		return
	}
	kind := body.Kind
	if kind == "" {
		kind = "property_sale"
	}
	src := models.ScrapedSource{
		Name:      body.Name,
		URL:       body.URL,
		Kind:      kind,
		Selectors: body.Selectors,
		Active:    true,
	}
	if body.Active != nil {
		src.Active = *body.Active
	}
	if err := storage.DB.Create(&src).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": src})
}

// GET /admin/scraper/sources — list registered sources.
func AdminListScrapedSources(ctx iris.Context) {
	var sources []models.ScrapedSource
	if err := storage.DB.Order("created_at DESC").Find(&sources).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": sources})
}

// PATCH /admin/scraper/sources/:id — update selectors / active / kind.
func AdminUpdateScrapedSource(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var src models.ScrapedSource
	if err := storage.DB.First(&src, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "source not found")
		return
	}
	var body struct {
		Name      *string         `json:"name"`
		URL       *string         `json:"url"`
		Kind      *string         `json:"kind"`
		Selectors *datatypes.JSON `json:"selectors"`
		Active    *bool           `json:"active"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		utils.JSONError(ctx, http.StatusUnprocessableEntity, "invalid_payload", "invalid body")
		return
	}
	if body.Name != nil {
		src.Name = *body.Name
	}
	if body.URL != nil {
		src.URL = *body.URL
	}
	if body.Kind != nil {
		src.Kind = *body.Kind
	}
	if body.Selectors != nil {
		src.Selectors = *body.Selectors
	}
	if body.Active != nil {
		src.Active = *body.Active
	}
	if err := storage.DB.Save(&src).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": src})
}

// DELETE /admin/scraper/sources/:id
func AdminDeleteScrapedSource(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := storage.DB.Delete(&models.ScrapedSource{}, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"success": true})
}

// POST /admin/scraper/sources/:id/run — scrape now, returns the run summary.
func AdminRunScrapedSource(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		utils.JSONError(ctx, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var src models.ScrapedSource
	if err := storage.DB.First(&src, id).Error; err != nil {
		utils.JSONError(ctx, http.StatusNotFound, "not_found", "source not found")
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := services.NewWebScraper().ScrapeSource(c, &src)
	if err != nil {
		src.LastStatus = "error: " + err.Error()
		storage.DB.Model(&src).Update("last_status", src.LastStatus)
		utils.JSONError(ctx, http.StatusBadGateway, "scrape_failed", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": res, "source": src})
}

// GET /admin/scraper/listings?source_id=&kind=&limit= — browse scraped rows.
func AdminListScrapedListings(ctx iris.Context) {
	q := storage.DB.Model(&models.ScrapedListing{})
	if sid := ctx.URLParam("source_id"); sid != "" {
		if v, err := strconv.ParseUint(sid, 10, 32); err == nil {
			q = q.Where("source_id = ?", v)
		}
	}
	if kind := ctx.URLParam("kind"); kind != "" {
		q = q.Where("kind = ?", kind)
	}
	limit := ctx.URLParamIntDefault("limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []models.ScrapedListing
	if err := q.Order("scraped_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": rows})
}
