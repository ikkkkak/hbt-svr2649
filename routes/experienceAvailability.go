package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"net/http"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

type setAvailabilityInput struct {
	Dates  []string `json:"dates"`  // ISO YYYY-MM-DD
	Status string   `json:"status"` // available | blocked
}

func ListAvailability(ctx iris.Context) {
	expID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	var rows []models.ExperienceAvailability
	storage.DB.Where("experience_id = ?", expID).Find(&rows)
	ctx.JSON(iris.Map{"success": true, "availability": rows})
}

func SetAvailability(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	expID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Ensure owner
	var exp models.Experience
	if err := storage.DB.First(&exp, expID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if exp.HostID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var input setAvailabilityInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}
	if input.Status != "available" && input.Status != "blocked" {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	for _, d := range input.Dates {
		t, parseErr := time.Parse("2006-01-02", d)
		if parseErr != nil {
			continue
		}
		row := models.ExperienceAvailability{ExperienceID: expID, Date: t, Status: input.Status}
		storage.DB.Where("experience_id = ? AND date = ?", expID, t).Assign(map[string]interface{}{"status": input.Status}).FirstOrCreate(&row)
	}
	ctx.JSON(iris.Map{"success": true})
}
