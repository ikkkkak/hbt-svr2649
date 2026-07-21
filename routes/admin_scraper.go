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

// POST /admin/scraper/sources/:id/run — start a scrape in the BACKGROUND and
// return immediately. Scraping (robots fetch + polite wait + page fetch +
// parse) can exceed a reverse proxy's request timeout; running it inline made
// the proxy return 502. The admin polls GET /scraper/sources (last_status) or
// /scraper/listings to see the outcome.
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

	// Mark running so the UI shows progress immediately.
	storage.DB.Model(&src).Update("last_status", "running…")

	go func(s models.ScrapedSource) {
		c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := services.NewWebScraper().ScrapeSource(c, &s); err != nil {
			storage.DB.Model(&models.ScrapedSource{}).Where("id = ?", s.ID).
				Update("last_status", "error: "+err.Error())
		}
	}(src)

	ctx.StatusCode(http.StatusAccepted)
	ctx.JSON(iris.Map{
		"status":     "started",
		"source_id":  id,
		"poll":       "GET /api/admin/scraper/listings?source_id=" + strconv.FormatUint(uint64(id), 10),
	})
}

// GET /admin/scraper/runs?source_id=&limit= — audit trail of scrape runs.
func AdminListScrapeRuns(ctx iris.Context) {
	q := storage.DB.Model(&models.ScrapeRun{})
	if sid := ctx.URLParam("source_id"); sid != "" {
		if v, err := strconv.ParseUint(sid, 10, 32); err == nil {
			q = q.Where("source_id = ?", v)
		}
	}
	limit := ctx.URLParamIntDefault("limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var runs []models.ScrapeRun
	if err := q.Order("created_at DESC").Limit(limit).Find(&runs).Error; err != nil {
		utils.JSONError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": runs})
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
