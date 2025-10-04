package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/datatypes"
)

// CreateExperience creates a new experience
func CreateExperience(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var input struct {
		Title              string  `json:"title" validate:"required"`
		City               string  `json:"city" validate:"required"`
		Language           string  `json:"language" validate:"required"`
		Focus              string  `json:"focus" validate:"required"`
		HasHostedBefore    bool    `json:"hasHostedBefore"`
		HostedFor          string  `json:"hostedFor"`
		Description        string  `json:"description"`
		Duration           int     `json:"duration"`
		WhatWeDo           string  `json:"whatWeDo"`
		WhatToBring        string  `json:"whatToBring"`
		BringRequired      bool    `json:"bringRequired"`
		MinAge             int     `json:"minAge"`
		MaxAge             int     `json:"maxAge"`
		ActivityLevel      string  `json:"activityLevel"`
		DifficultyLevel    string  `json:"difficultyLevel"`
		GroupSize          int     `json:"groupSize"`
		StartTime          string  `json:"startTime"`
		EndTime            string  `json:"endTime"`
		PricePerPerson     float64 `json:"pricePerPerson"`
		GroupDiscounts     string  `json:"groupDiscounts"`
		ArrivalTime        int     `json:"arrivalTime"`
		CancellationPolicy string  `json:"cancellationPolicy"`
		VideoURL           string  `json:"videoURL"`
		Photos             string  `json:"photos"` // JSON string of photos array
	}

	if err := ctx.ReadJSON(&input); err != nil {
		fmt.Printf("Error reading JSON input: %v\n", err)
		utils.HandleValidationErrors(err, ctx)
		return
	}

	fmt.Printf("Received experience data: Title=%s, City=%s, Language=%s, Focus=%s\n",
		input.Title, input.City, input.Language, input.Focus)

	// Check if user has verified identity
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found"})
		return
	}

	experience := models.Experience{
		HostID:             userID,
		Title:              input.Title,
		City:               input.City,
		Language:           input.Language,
		Focus:              input.Focus,
		HasHostedBefore:    input.HasHostedBefore,
		HostedFor:          input.HostedFor,
		Description:        input.Description,
		Duration:           input.Duration,
		WhatWeDo:           input.WhatWeDo,
		WhatToBring:        input.WhatToBring,
		BringRequired:      input.BringRequired,
		MinAge:             input.MinAge,
		MaxAge:             input.MaxAge,
		ActivityLevel:      input.ActivityLevel,
		DifficultyLevel:    input.DifficultyLevel,
		GroupSize:          input.GroupSize,
		StartTime:          input.StartTime,
		EndTime:            input.EndTime,
		PricePerPerson:     input.PricePerPerson,
		ArrivalTime:        input.ArrivalTime,
		CancellationPolicy: input.CancellationPolicy,
		VideoURL:           input.VideoURL,
		Status:             "draft",
		ReviewStatus:       "pending",
	}

	// Parse group discounts if provided
	if input.GroupDiscounts != "" {
		// keep raw JSON to store in datatypes.JSON
		var raw json.RawMessage = json.RawMessage([]byte(input.GroupDiscounts))
		experience.GroupDiscounts = datatypes.JSON(raw)
	}

	// Parse photos if provided
	if input.Photos != "" {
		// keep raw JSON to store in datatypes.JSON
		var raw json.RawMessage = json.RawMessage([]byte(input.Photos))
		experience.Photos = datatypes.JSON(raw)
	}

	fmt.Printf("About to create experience: Title=%s, City=%s, HostID=%d\n",
		experience.Title, experience.City, experience.HostID)

	if err := storage.DB.Create(&experience).Error; err != nil {
		fmt.Printf("Error creating experience: %v\n", err)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Failed to create experience", "details": err.Error()})
		return
	}

	fmt.Printf("Experience created successfully with ID: %d\n", experience.ID)
	ctx.JSON(iris.Map{"success": true, "experience": experience})
}

// GetUserExperiences returns experiences for a specific host
func GetUserExperiences(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID

	var experiences []models.Experience
	if err := storage.DB.Where("host_id = ?", userID).
		Order("created_at DESC").
		Find(&experiences).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{"success": true, "experiences": experiences})
}

// UpdateExperience updates an existing experience
func UpdateExperience(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	id := ctx.Params().Get("id")

	var input struct {
		Title              string  `json:"title"`
		City               string  `json:"city"`
		Language           string  `json:"language"`
		Focus              string  `json:"focus"`
		HasHostedBefore    bool    `json:"hasHostedBefore"`
		HostedFor          string  `json:"hostedFor"`
		Description        string  `json:"description"`
		Duration           int     `json:"duration"`
		WhatWeDo           string  `json:"whatWeDo"`
		WhatToBring        string  `json:"whatToBring"`
		BringRequired      bool    `json:"bringRequired"`
		MinAge             int     `json:"minAge"`
		MaxAge             int     `json:"maxAge"`
		ActivityLevel      string  `json:"activityLevel"`
		DifficultyLevel    string  `json:"difficultyLevel"`
		GroupSize          int     `json:"groupSize"`
		StartTime          string  `json:"startTime"`
		EndTime            string  `json:"endTime"`
		PricePerPerson     float64 `json:"pricePerPerson"`
		GroupDiscounts     string  `json:"groupDiscounts"`
		ArrivalTime        int     `json:"arrivalTime"`
		CancellationPolicy string  `json:"cancellationPolicy"`
		VideoURL           string  `json:"videoURL"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var experience models.Experience
	if err := storage.DB.Where("id = ? AND host_id = ?", id, userID).First(&experience).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Experience not found"})
		return
	}

	// Update fields
	if input.Title != "" {
		experience.Title = input.Title
	}
	if input.City != "" {
		experience.City = input.City
	}
	if input.Language != "" {
		experience.Language = input.Language
	}
	if input.Focus != "" {
		experience.Focus = input.Focus
	}
	experience.HasHostedBefore = input.HasHostedBefore
	if input.HostedFor != "" {
		experience.HostedFor = input.HostedFor
	}
	if input.Description != "" {
		experience.Description = input.Description
	}
	if input.Duration > 0 {
		experience.Duration = input.Duration
	}
	if input.WhatWeDo != "" {
		experience.WhatWeDo = input.WhatWeDo
	}
	if input.WhatToBring != "" {
		experience.WhatToBring = input.WhatToBring
	}
	experience.BringRequired = input.BringRequired
	if input.MinAge > 0 {
		experience.MinAge = input.MinAge
	}
	if input.MaxAge > 0 {
		experience.MaxAge = input.MaxAge
	}
	if input.ActivityLevel != "" {
		experience.ActivityLevel = input.ActivityLevel
	}
	if input.DifficultyLevel != "" {
		experience.DifficultyLevel = input.DifficultyLevel
	}
	if input.GroupSize > 0 {
		experience.GroupSize = input.GroupSize
	}
	if input.StartTime != "" {
		experience.StartTime = input.StartTime
	}
	if input.EndTime != "" {
		experience.EndTime = input.EndTime
	}
	if input.PricePerPerson > 0 {
		experience.PricePerPerson = input.PricePerPerson
	}
	if input.ArrivalTime > 0 {
		experience.ArrivalTime = input.ArrivalTime
	}
	if input.CancellationPolicy != "" {
		experience.CancellationPolicy = input.CancellationPolicy
	}
	if input.VideoURL != "" {
		experience.VideoURL = input.VideoURL
	}

	// Parse group discounts if provided
	if input.GroupDiscounts != "" {
		var raw json.RawMessage = json.RawMessage([]byte(input.GroupDiscounts))
		experience.GroupDiscounts = datatypes.JSON(raw)
	}

	if err := storage.DB.Save(&experience).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{"success": true, "experience": experience})
}

// SubmitExperienceForReview submits an experience for review
func SubmitExperienceForReview(ctx iris.Context) {
	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	userID := claims.ID
	id := ctx.Params().Get("id")

	var experience models.Experience
	if err := storage.DB.Where("id = ? AND host_id = ?", id, userID).First(&experience).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Experience not found"})
		return
	}

	// Check if experience has all required fields
	if experience.Title == "" || experience.City == "" || experience.Description == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Please complete all required fields before submitting"})
		return
	}

	// Update status
	experience.Status = "pending"
	experience.ReviewStatus = "pending"
	experience.ReviewNotes = "Experience submitted for review. We will contact you within 5 hours."

	if err := storage.DB.Save(&experience).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"success":    true,
		"message":    "Experience submitted for review. We will contact you within 5 hours.",
		"experience": experience,
	})
}

// GetExperienceDetails returns detailed information about an experience
func GetExperienceDetails(ctx iris.Context) {
	id := ctx.Params().Get("id")

	var experience models.Experience
	if err := storage.DB.Preload("Host").
		First(&experience, id).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Experience not found"})
		return
	}

	ctx.JSON(iris.Map{"success": true, "experience": experience})
}

// GetPublicExperiences returns public experiences for discovery
func GetPublicExperiences(ctx iris.Context) {
	page := ctx.URLParamIntDefault("page", 1)
	limit := ctx.URLParamIntDefault("limit", 10)
	city := ctx.URLParam("city")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	offset := (page - 1) * limit

	var experiences []models.Experience
	query := storage.DB.Where("status = ?", "live").
		Preload("Host").
		Order("created_at DESC")

	if city != "" {
		query = query.Where("city ILIKE ?", "%"+city+"%")
	}

	if err := query.Limit(limit).Offset(offset).Find(&experiences).Error; err != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{"success": true, "experiences": experiences, "page": page})
}
