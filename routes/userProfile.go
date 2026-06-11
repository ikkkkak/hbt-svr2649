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

// GetUserProfile retrieves the user's profile information (combines User + UserProfile)
func GetUserProfile(ctx iris.Context) {
	fmt.Printf("🔍 GetUserProfile called - Method: %s, Path: %s\n", ctx.Method(), ctx.Path())
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User ID not found in context"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Invalid user ID format"})
		return
	}

	// Get user basic info
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		fmt.Printf("❌ User not found with ID %d: %v\n", userID, err)
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User not found"})
		return
	}

	fmt.Printf("✅ Found user: ID=%d, FirstName='%s', LastName='%s', Email='%s'\n",
		user.ID, user.FirstName, user.LastName, user.Email)

	// Get user profile (if exists)
	var profile models.UserProfile
	profileExists := storage.DB.Where("user_id = ?", userID).First(&profile).Error == nil

	// Parse JSON fields from User model
	var userLanguages []string
	if len(user.Languages) > 0 {
		json.Unmarshal(user.Languages, &userLanguages)
	}
	var userSkills []string
	if len(user.Skills) > 0 {
		json.Unmarshal(user.Skills, &userSkills)
	}

	// Parse JSON fields from UserProfile model
	var profileLanguages []string
	var profileSkills []string
	var profileInterests []string

	if profileExists {
		if len(profile.Languages) > 0 {
			json.Unmarshal(profile.Languages, &profileLanguages)
		}
		if len(profile.Skills) > 0 {
			json.Unmarshal(profile.Skills, &profileSkills)
		}
		if len(profile.Interests) > 0 {
			json.Unmarshal(profile.Interests, &profileInterests)
		}
	}

	// Merge data (UserProfile takes precedence over User)
	firstName := user.FirstName
	lastName := user.LastName
	avatarURL := user.AvatarURL
	dateOfBirth := user.DateOfBirth
	bio := user.Bio
	languages := userLanguages
	skills := userSkills

	if profileExists {
		if profile.FirstName != "" {
			firstName = profile.FirstName
		}
		if profile.LastName != "" {
			lastName = profile.LastName
		}
		if profile.AvatarURL != "" {
			avatarURL = profile.AvatarURL
		}
		if profile.DateOfBirth != "" {
			dateOfBirth = profile.DateOfBirth
		}
		if profile.Bio != "" {
			bio = profile.Bio
		}
		if len(profileLanguages) > 0 {
			languages = profileLanguages
		}
		if len(profileSkills) > 0 {
			skills = profileSkills
		}
	}

	// Calculate completion percentage
	completionPercentage := 0
	if profileExists {
		completionPercentage = profile.CalculateCompletionPercentage()
	} else {
		// Calculate based on User model
		fields := []bool{
			firstName != "",
			lastName != "",
			avatarURL != "",
			bio != "",
			len(languages) > 0,
			dateOfBirth != "",
		}
		completed := 0
		for _, field := range fields {
			if field {
				completed++
			}
		}
		completionPercentage = (completed * 100) / len(fields)
	}

	// Build response (exclude sensitive data like ID images, verification details)
	fmt.Printf("📤 Building response: firstName='%s', lastName='%s', email='%s', hasProfile=%v\n",
		firstName, lastName, user.Email, profileExists)

	response := iris.Map{
		"success": true,
		"profile": iris.Map{
			// Basic Info
			"firstName":   firstName,
			"lastName":    lastName,
			"email":       user.Email,
			"phoneNumber": user.PhoneNumber,
			"avatarURL":   avatarURL,
			"dateOfBirth": dateOfBirth,
			"bio":         bio,
			"hasProfile":  profileExists,

			// Languages and Skills
			"languages": languages,
			"skills":    skills,

			// Profile-specific fields (only if profile exists)
			"location": func() string {
				if profileExists {
					return profile.Location
				} else {
					return ""
				}
			}(),
			"interests": profileInterests,
			"occupation": func() string {
				if profileExists {
					return profile.Occupation
				} else {
					return ""
				}
			}(),
			"company": func() string {
				if profileExists {
					return profile.Company
				} else {
					return ""
				}
			}(),
			"website": func() string {
				if profileExists {
					return profile.Website
				} else {
					return ""
				}
			}(),
			"instagram": func() string {
				if profileExists {
					return profile.Instagram
				} else {
					return ""
				}
			}(),
			"twitter": func() string {
				if profileExists {
					return profile.Twitter
				} else {
					return ""
				}
			}(),
			"linkedin": func() string {
				if profileExists {
					return profile.LinkedIn
				} else {
					return ""
				}
			}(),
			"travelStyle": func() string {
				if profileExists {
					return profile.TravelStyle
				} else {
					return ""
				}
			}(),
			"accommodationType": func() string {
				if profileExists {
					return profile.AccommodationType
				} else {
					return ""
				}
			}(),

			// Status
			"isPublic": func() bool {
				if profileExists {
					return profile.IsPublic
				} else {
					return true
				}
			}(),
			"isComplete": func() bool {
				if profileExists {
					return profile.IsComplete
				} else {
					return false
				}
			}(),
			"completionPercentage": completionPercentage,

			// Verification status (safe to show)
			"isVerified": func() bool {
				if user.IsVerified != nil {
					return *user.IsVerified
				} else {
					return false
				}
			}(),
			"verificationStatus": user.VerificationStatus,

			// Timestamps
			"createdAt": func() string {
				if profileExists {
					return profile.CreatedAt.Format(time.RFC3339)
				} else {
					return user.CreatedAt.Format(time.RFC3339)
				}
			}(),
			"updatedAt": func() string {
				if profileExists {
					return profile.UpdatedAt.Format(time.RFC3339)
				} else {
					return user.UpdatedAt.Format(time.RFC3339)
				}
			}(),
		},
	}

	ctx.JSON(response)
}

// CreateOrUpdateUserProfile creates or updates the user's profile
func CreateOrUpdateUserProfile(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User ID not found in context"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		fmt.Printf("❌ CreateOrUpdateUserProfile: Invalid user ID format: %T\n", userIDInterface)
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Invalid user ID format"})
		return
	}

	fmt.Printf("✅ CreateOrUpdateUserProfile: Processing request for user ID %d\n", userID)
	fmt.Printf("🔍 Request Method: %s\n", ctx.Method())
	fmt.Printf("🔍 Request URL: %s\n", ctx.Path())
	fmt.Printf("🔍 Request Host: %s\n", ctx.Host())
	fmt.Printf("🔍 Request Full Path: %s\n", ctx.FullRequestURI())

	var input CreateOrUpdateProfileInput
	if err := ctx.ReadJSON(&input); err != nil {
		fmt.Printf("❌ CreateOrUpdateUserProfile: Invalid JSON input: %v\n", err)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON input"})
		return
	}

	fmt.Printf("📝 CreateOrUpdateUserProfile: Received data - FirstName='%s', LastName='%s', Email='%s'\n",
		input.FirstName, input.LastName, input.Email)
	fmt.Printf("📝 Full input data: %+v\n", input)

	// Upload avatar if provided and not already a hosted URL (Cloudinary or GCS)
	avatarURL := input.AvatarURL
	if avatarURL != "" &&
		!strings.Contains(avatarURL, "res.cloudinary.com") &&
		!strings.Contains(avatarURL, "storage.googleapis.com") {
		// Generate unique filename with timestamp
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)
		publicID := fmt.Sprintf("profiles/%d/avatar_%d", userID, timestamp)
		urlMap := storage.UploadBase64Image(avatarURL, publicID)
		if urlMap != nil && urlMap["url"] != "" {
			avatarURL = urlMap["url"]
		}
	}

	// Convert arrays to JSON
	languagesJSON, _ := json.Marshal(input.Languages)
	skillsJSON, _ := json.Marshal(input.Skills)
	interestsJSON, _ := json.Marshal(input.Interests)

	// Update user basic information in users table
	userUpdates := map[string]interface{}{}
	fmt.Printf("🔍 Checking FirstName: '%s' (len=%d)\n", input.FirstName, len(input.FirstName))
	if input.FirstName != "" {
		fmt.Printf("✅ Adding FirstName to updates\n")
		userUpdates["first_name"] = input.FirstName
	}

	fmt.Printf("🔍 Checking LastName: '%s' (len=%d)\n", input.LastName, len(input.LastName))
	if input.LastName != "" {
		fmt.Printf("✅ Adding LastName to updates\n")
		userUpdates["last_name"] = input.LastName
	}

	fmt.Printf("🔍 Checking Email: '%s' (len=%d)\n", input.Email, len(input.Email))
	// Email is always updateable, even if empty (to allow clearing it)
	if input.Email != "" {
		fmt.Printf("✅ Adding Email to updates\n")
		userUpdates["email"] = input.Email
	} else {
		fmt.Printf("⚠️ Email is empty, skipping update\n")
	}
	if input.PhoneNumber != nil {
		phone := strings.TrimSpace(*input.PhoneNumber)
		if phone == "" {
			userUpdates["phone_number"] = nil
		} else {
			if !utils.ValidatePhoneNumber(phone) {
				ctx.StatusCode(iris.StatusBadRequest)
				ctx.JSON(iris.Map{"error": "Invalid phone number format. Mauritanian phone numbers must be 8 digits starting with 2, 3, or 4."})
				return
			}
			formatted := utils.NormalizePhoneNumber(phone)
			var existing models.User
			if err := storage.DB.Where("phone_number = ? AND id != ?", formatted, userID).Limit(1).Find(&existing).Error; err == nil && existing.ID > 0 {
				ctx.StatusCode(iris.StatusBadRequest)
				ctx.JSON(iris.Map{"error": "This phone number is already registered to another account."})
				return
			}
			userUpdates["phone_number"] = formatted
		}
	}
	if input.AvatarURL != "" {
		userUpdates["avatar_url"] = avatarURL
	}
	if input.DateOfBirth != "" {
		userUpdates["date_of_birth"] = input.DateOfBirth
	}
	if input.Bio != "" {
		userUpdates["bio"] = input.Bio
	}
	if len(input.Languages) > 0 {
		userUpdates["languages"] = languagesJSON
	}
	if len(input.Skills) > 0 {
		userUpdates["skills"] = skillsJSON
	}

	// Update user table if there are updates
	fmt.Printf("📝 userUpdates map: %+v (len=%d)\n", userUpdates, len(userUpdates))
	if len(userUpdates) > 0 {
		fmt.Printf("📝 Updating user table with: %+v\n", userUpdates)
		result := storage.DB.Model(&models.User{}).Where("id = ?", userID).Updates(userUpdates)
		if result.Error != nil {
			fmt.Printf("❌ Failed to update user table: %v\n", result.Error)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to update user information"})
			return
		}
		fmt.Printf("✅ User table updated successfully (rows affected: %d)\n", result.RowsAffected)
	} else {
		fmt.Printf("⚠️ No user updates to perform\n")
	}

	// Check if profile exists
	var existingProfile models.UserProfile
	err := storage.DB.Where("user_id = ?", userID).First(&existingProfile).Error

	if err != nil {
		// Create new profile
		profile := models.UserProfile{
			UserID:            userID,
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
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to create profile"})
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
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to update profile"})
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
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User ID not found in context"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Invalid user ID format"})
		return
	}

	var profile models.UserProfile
	if err := storage.DB.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Profile not found"})
		return
	}

	if err := storage.DB.Delete(&profile).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to delete profile"})
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
	Email             string   `json:"email"`
	PhoneNumber       *string  `json:"phoneNumber"` // optional - for email users to add phone
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
