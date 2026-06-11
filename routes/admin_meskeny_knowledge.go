package routes

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

var allowedKnowledgeDocTypes = map[string]bool{
	"policy": true, "pricing": true, "zones": true, "faq": true, "product": true, "legal_other": true,
}

var allowedKnowledgeLocales = map[string]bool{
	"any": true, "ar": true, "fr": true, "en": true,
}

const maxKnowledgeTitleLen = 160
const maxKnowledgeBodyLen = 2000

func normalizeKnowledgePayload(docType, locale, intentScope, matchKeywords, title, body string) (string, string, string, string, string, string, bool) {
	docType = strings.TrimSpace(strings.ToLower(docType))
	locale = strings.TrimSpace(strings.ToLower(locale))
	if locale == "" {
		locale = "any"
	}
	intentScope = strings.TrimSpace(strings.ToLower(intentScope))
	if intentScope == "" {
		intentScope = "all"
	}
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	matchKeywords = strings.TrimSpace(strings.ToLower(matchKeywords))

	if !allowedKnowledgeDocTypes[docType] || !allowedKnowledgeLocales[locale] {
		return "", "", "", "", "", "", false
	}
	if title == "" || body == "" {
		return "", "", "", "", "", "", false
	}
	if utf8.RuneCountInString(title) > maxKnowledgeTitleLen || utf8.RuneCountInString(body) > maxKnowledgeBodyLen {
		return "", "", "", "", "", "", false
	}
	return docType, locale, intentScope, matchKeywords, title, body, true
}

// GET /api/admin/ai/knowledge
func AdminListMeskenyKnowledge(ctx iris.Context) {
	activeOnly := ctx.URLParamDefault("active_only", "") == "1" || ctx.URLParamDefault("active_only", "") == "true"
	q := storage.DB.Model(&models.MeskenyKnowledgeEntry{}).Order("priority DESC, id DESC").Limit(500)
	if activeOnly {
		q = q.Where("active = ?", true)
	}
	var rows []models.MeskenyKnowledgeEntry
	if err := q.Find(&rows).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "list_failed", "message": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"data": rows})
}

// GET /api/admin/ai/knowledge/{id:uint}
func AdminGetMeskenyKnowledge(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_id"})
		return
	}
	var row models.MeskenyKnowledgeEntry
	if err := storage.DB.First(&row, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "not_found"})
		return
	}
	ctx.JSON(iris.Map{"data": row})
}

type meskenyKnowledgeInput struct {
	DocType       string `json:"doc_type"`
	Locale        string `json:"locale"`
	IntentScope   string `json:"intent_scope"`
	MatchKeywords string `json:"match_keywords"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	Priority      int    `json:"priority"`
	Active        *bool  `json:"active"`
}

// POST /api/admin/ai/knowledge
func AdminCreateMeskenyKnowledge(ctx iris.Context) {
	var in meskenyKnowledgeInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_json", "message": err.Error()})
		return
	}
	dt, loc, isc, mkw, title, body, ok := normalizeKnowledgePayload(
		in.DocType, in.Locale, in.IntentScope, in.MatchKeywords, in.Title, in.Body)
	if !ok {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{
			"error": "validation_failed",
			"message": "doc_type must be policy|pricing|zones|faq|product|legal_other; locale any|ar|fr|en; title/body required; body max 2000 chars; title max 160 chars",
		})
		return
	}
	uid, _ := ctx.Values().Get("userID").(uint)
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	row := models.MeskenyKnowledgeEntry{
		DocType:         dt,
		Locale:          loc,
		IntentScope:     isc,
		MatchKeywords:   mkw,
		Title:           title,
		Body:            body,
		Priority:        in.Priority,
		Active:          active,
		CreatedByUserID: nonZeroUintPtr(uid),
	}
	if err := storage.DB.Create(&row).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "create_failed", "message": err.Error()})
		return
	}
	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{"data": row})
}

type meskenyKnowledgePatchInput struct {
	DocType       *string `json:"doc_type"`
	Locale        *string `json:"locale"`
	IntentScope   *string `json:"intent_scope"`
	MatchKeywords *string `json:"match_keywords"`
	Title         *string `json:"title"`
	Body          *string `json:"body"`
	Priority      *int    `json:"priority"`
	Active        *bool   `json:"active"`
}

// PATCH /api/admin/ai/knowledge/{id:uint}
func AdminUpdateMeskenyKnowledge(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_id"})
		return
	}
	var row models.MeskenyKnowledgeEntry
	if err := storage.DB.First(&row, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "not_found"})
		return
	}
	var in meskenyKnowledgePatchInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_json", "message": err.Error()})
		return
	}
	dt, loc, isc, mkw, title, body := row.DocType, row.Locale, row.IntentScope, row.MatchKeywords, row.Title, row.Body
	if in.DocType != nil {
		dt = *in.DocType
	}
	if in.Locale != nil {
		loc = *in.Locale
	}
	if in.IntentScope != nil {
		isc = *in.IntentScope
	}
	if in.MatchKeywords != nil {
		mkw = *in.MatchKeywords
	}
	if in.Title != nil {
		title = *in.Title
	}
	if in.Body != nil {
		body = *in.Body
	}
	ndt, nloc, nisc, nmkw, ntitle, nbody, ok := normalizeKnowledgePayload(dt, loc, isc, mkw, title, body)
	if !ok {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "validation_failed"})
		return
	}
	row.DocType, row.Locale, row.IntentScope, row.MatchKeywords = ndt, nloc, nisc, nmkw
	row.Title, row.Body = ntitle, nbody
	if in.Priority != nil {
		row.Priority = *in.Priority
	}
	if in.Active != nil {
		row.Active = *in.Active
	}
	if err := storage.DB.Save(&row).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "update_failed", "message": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"data": row})
}

// DELETE /api/admin/ai/knowledge/{id:uint} — soft delete
func AdminDeleteMeskenyKnowledge(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "invalid_id"})
		return
	}
	if err := storage.DB.Delete(&models.MeskenyKnowledgeEntry{}, id).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "delete_failed", "message": err.Error()})
		return
	}
	ctx.StatusCode(http.StatusNoContent)
}

func nonZeroUintPtr(u uint) *uint {
	if u == 0 {
		return nil
	}
	return &u
}
