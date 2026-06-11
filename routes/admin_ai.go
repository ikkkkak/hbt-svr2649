package routes

import (
	"net/http"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

type AIInteractionWithFeedback struct {
	models.AIInteraction
	ThumbsUp   int64 `json:"thumbs_up"`
	ThumbsDown int64 `json:"thumbs_down"`
}

// GET /api/admin/ai/interactions — list MeskenyGPT interactions with feedback aggregates (admin only)
func AdminListAIInteractions(ctx iris.Context) {
	// Optional: simple limit for now; can be extended with pagination later
	limitParam := ctx.URLParamIntDefault("limit", 200)
	if limitParam <= 0 || limitParam > 1000 {
		limitParam = 200
	}

	var rows []AIInteractionWithFeedback

	db := storage.DB.
		Table("ai_interactions AS i").
		Select(`
			i.*,
			COALESCE(SUM(CASE WHEN f.signal = 'thumbs_up' THEN 1 ELSE 0 END), 0)   AS thumbs_up,
			COALESCE(SUM(CASE WHEN f.signal = 'thumbs_down' THEN 1 ELSE 0 END), 0) AS thumbs_down
		`).
		Joins("LEFT JOIN ai_feedback AS f ON f.interaction_id = i.id").
		Group("i.id").
		Order("i.created_at DESC").
		Limit(limitParam)

	if err := db.Find(&rows).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "failed_to_load_interactions", "message": err.Error()})
		return
	}

	ctx.JSON(iris.Map{
		"data": rows,
	})
}

