package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
)

// GetUserProfile retrieves the user's profile information
func GetUserProfile(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var profile models.UserProfile
	if err := storage.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		// If no profile exists, return empty profile
		ctx.JSON(iris.Map{
			"success": true,
			"profile": iris.Map{
				"id":                   0,
				"firstName":            "",
				"lastName":             "",
				"avatarURL":            "",
				"dateOfBirth":          "",
				"bio":                  "",
				"languages":            []string{},
				"skills":               []string{},
				"location":             "",
				"interests":            []string{},
				"occupation":           "",
				"company":              "",
				"website":              "",
				"instagram":            "",
				"twitter":              "",
				"linkedin":             "",
				"travelStyle":          "",
				"accommodationType":    "",
				"isPublic":             true,
				"isComplete":           false,
				"completionPercentage": 0,
			},
		})
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"profile": profile,
	})
}

// CreateOrUpdateUserProfile creates or updates the user's profile
func CreateOrUpdateUserProfile(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var input CreateOrUpdateProfileInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Upload avatar if provided and not already a Cloudinary URL
	avatarURL := input.AvatarURL
	if avatarURL != "" && !strings.Contains(avatarURL, "res.cloudinary.com") {
		// Generate unique filename with timestamp
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)
		publicID := fmt.Sprintf("profiles/%d/avatar_%d", user.ID, timestamp)
		urlMap := storage.UploadBase64Image(avatarURL, publicID)
		if urlMap != nil && urlMap["url"] != "" {
			avatarURL = urlMap["url"]
		}
	}

	// Convert arrays to JSON
	languagesJSON, _ := json.Marshal(input.Languages)
	skillsJSON, _ := json.Marshal(input.Skills)
	interestsJSON, _ := json.Marshal(input.Interests)

	// Check if profile exists
	var existingProfile models.UserProfile
	err := storage.DB.Where("user_id = ?", user.ID).First(&existingProfile).Error

	if err != nil {
		// Create new profile
		profile := models.UserProfile{
			UserID:            user.ID,
			FirstName:         input.FirstName,
			LastName:          input.LastName,
			AvatarURL:         avatarURL,
			DateOfBirth:       input.DateOfBirth,
			Bio:               input.Bio,
			Languages:         languagesJSON,
			Skills:            skillsJSON,
			Location:          input.Location,
			Interests:         interestsJSON,
			Occupation:        input.Occupation,
			Company:           input.Company,
			Website:           input.Website,
			Instagram:         input.Instagram,
			Twitter:           input.Twitter,
			LinkedIn:          input.LinkedIn,
			TravelStyle:       input.TravelStyle,
			AccommodationType: input.AccommodationType,
			IsPublic:          input.IsPublic,
		}

		// Calculate completion percentage
		profile.CalculateCompletionPercentage()

		if err := storage.DB.Create(&profile).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}

		ctx.JSON(iris.Map{
			"success": true,
			"profile": profile,
			"message": "Profile created successfully",
		})
	} else {
		// Update existing profile
		updates := map[string]interface{}{
			"first_name":         input.FirstName,
			"last_name":          input.LastName,
			"avatar_url":         avatarURL,
			"date_of_birth":      input.DateOfBirth,
			"bio":                input.Bio,
			"languages":          languagesJSON,
			"skills":             skillsJSON,
			"location":           input.Location,
			"interests":          interestsJSON,
			"occupation":         input.Occupation,
			"company":            input.Company,
			"website":            input.Website,
			"instagram":          input.Instagram,
			"twitter":            input.Twitter,
			"linkedin":           input.LinkedIn,
			"travel_style":       input.TravelStyle,
			"accommodation_type": input.AccommodationType,
			"is_public":          input.IsPublic,
		}

		if err := storage.DB.Model(&existingProfile).Updates(updates).Error; err != nil {
			ctx.StopWithStatus(http.StatusInternalServerError)
			return
		}

		// Recalculate completion percentage
		existingProfile.CalculateCompletionPercentage()
		storage.DB.Save(&existingProfile)

		ctx.JSON(iris.Map{
			"success": true,
			"profile": existingProfile,
			"message": "Profile updated successfully",
		})
	}
}

// GetUserProfileStatusNew returns the profile completion status using the new UserProfile system
func GetUserProfileStatusNew(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	// Get user email from the User table
	var userModel models.User
	if err := storage.DB.First(&userModel, user.ID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	var profile models.UserProfile
	if err := storage.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		// No profile exists
		ctx.JSON(iris.Map{
			"success": true,
			"profile": iris.Map{
				"firstName": "",
				"lastName":  "",
				"bio":       "",
				"avatarURL": "",
				"email":     userModel.Email,
			},
			"status": iris.Map{
				"canDiscoverGroups":    false,
				"completionPercentage": 0,
				"status":               "incomplete",
				"message":              "Please create your profile to discover groups",
				"hasName":              false,
				"hasBio":               false,
				"hasAvatar":            false,
			},
		})
		return
	}

	// Check profile completion criteria
	hasName := profile.FirstName != "" || profile.LastName != ""
	hasBio := profile.Bio != ""
	hasAvatar := profile.AvatarURL != ""

	// Calculate completion percentage
	completionCount := 0
	totalFields := 3 // name, bio, avatar

	if hasName {
		completionCount++
	}
	if hasBio {
		completionCount++
	}
	if hasAvatar {
		completionCount++
	}

	completionPercentage := (completionCount * 100) / totalFields

	// Determine status
	var status string
	var message string
	var canDiscoverGroups bool

	if hasName {
		canDiscoverGroups = true
		if completionPercentage >= 100 {
			status = "complete"
			message = "Profile is complete"
		} else if completionPercentage >= 66 {
			status = "good"
			message = "Profile is mostly complete"
		} else {
			status = "basic"
			message = "Profile has basic info"
		}
	} else {
		canDiscoverGroups = false
		status = "incomplete"
		message = "Please add your name to discover groups"
	}

	ctx.JSON(iris.Map{
		"success": true,
		"profile": iris.Map{
			"firstName": profile.FirstName,
			"lastName":  profile.LastName,
			"bio":       profile.Bio,
			"avatarURL": profile.AvatarURL,
			"email":     userModel.Email,
		},
		"status": iris.Map{
			"canDiscoverGroups":    canDiscoverGroups,
			"completionPercentage": completionPercentage,
			"status":               status,
			"message":              message,
			"hasName":              hasName,
			"hasBio":               hasBio,
			"hasAvatar":            hasAvatar,
		},
	})
}

// DeleteUserProfile deletes the user's profile
func DeleteUserProfile(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var profile models.UserProfile
	if err := storage.DB.Where("user_id = ?", user.ID).First(&profile).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	if err := storage.DB.Delete(&profile).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	ctx.JSON(iris.Map{
		"success": true,
		"message": "Profile deleted successfully",
	})
}

// Input structures
type CreateOrUpdateProfileInput struct {
	FirstName         string   `json:"firstName"`
	LastName          string   `json:"lastName"`
	AvatarURL         string   `json:"avatarURL"`
	DateOfBirth       string   `json:"dateOfBirth"`
	Bio               string   `json:"bio"`
	Languages         []string `json:"languages"`
	Skills            []string `json:"skills"`
	Location          string   `json:"location"`
	Interests         []string `json:"interests"`
	Occupation        string   `json:"occupation"`
	Company           string   `json:"company"`
	Website           string   `json:"website"`
	Instagram         string   `json:"instagram"`
	Twitter           string   `json:"twitter"`
	LinkedIn          string   `json:"linkedin"`
	TravelStyle       string   `json:"travelStyle"`
	AccommodationType string   `json:"accommodationType"`
	IsPublic          bool     `json:"isPublic"`
}
