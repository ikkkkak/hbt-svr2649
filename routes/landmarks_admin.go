package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"net/http"

	"github.com/kataras/iris/v12"
)

// AdminUpdateLandmarkCoordinates allows admin to update landmark coordinates
func AdminUpdateLandmarkCoordinates(ctx iris.Context) {
	landmarkID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	// Check if landmark exists
	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	var input struct {
		Point1Lat *float64 `json:"point1_lat"`
		Point1Lng *float64 `json:"point1_lng"`
		Point2Lat *float64 `json:"point2_lat"`
		Point2Lng *float64 `json:"point2_lng"`
		Point3Lat *float64 `json:"point3_lat"`
		Point3Lng *float64 `json:"point3_lng"`
		Point4Lat *float64 `json:"point4_lat"`
		Point4Lng *float64 `json:"point4_lng"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate and update coordinates
	if input.Point1Lat != nil {
		if *input.Point1Lat < -90 || *input.Point1Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point1_lat coordinate"})
			return
		}
		landmark.Point1Lat = *input.Point1Lat
	}
	if input.Point1Lng != nil {
		if *input.Point1Lng < -180 || *input.Point1Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point1_lng coordinate"})
			return
		}
		landmark.Point1Lng = *input.Point1Lng
	}
	if input.Point2Lat != nil {
		if *input.Point2Lat < -90 || *input.Point2Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point2_lat coordinate"})
			return
		}
		landmark.Point2Lat = *input.Point2Lat
	}
	if input.Point2Lng != nil {
		if *input.Point2Lng < -180 || *input.Point2Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point2_lng coordinate"})
			return
		}
		landmark.Point2Lng = *input.Point2Lng
	}
	if input.Point3Lat != nil {
		if *input.Point3Lat < -90 || *input.Point3Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point3_lat coordinate"})
			return
		}
		landmark.Point3Lat = *input.Point3Lat
	}
	if input.Point3Lng != nil {
		if *input.Point3Lng < -180 || *input.Point3Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point3_lng coordinate"})
			return
		}
		landmark.Point3Lng = *input.Point3Lng
	}
	if input.Point4Lat != nil {
		if *input.Point4Lat < -90 || *input.Point4Lat > 90 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point4_lat coordinate"})
			return
		}
		landmark.Point4Lat = *input.Point4Lat
	}
	if input.Point4Lng != nil {
		if *input.Point4Lng < -180 || *input.Point4Lng > 180 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid point4_lng coordinate"})
			return
		}
		landmark.Point4Lng = *input.Point4Lng
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark coordinates"})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Landmark coordinates updated successfully",
		"landmark": landmark,
	})
}

