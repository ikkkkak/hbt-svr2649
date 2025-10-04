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

type discoverGroupsInput struct {
	Privacy   string `json:"privacy"`   // public, private, all
	Location  string `json:"location"`  // city filter
	Interests string `json:"interests"` // comma-separated interests
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// DiscoverGroups allows travelers to search for groups to join
func DiscoverGroups(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	// Check if user has complete profile - try new UserProfile system first, fallback to old User system
	var userProfile models.UserProfile
	var hasProfile bool
	var firstName, lastName string

	if err := storage.DB.Where("user_id = ?", user.ID).First(&userProfile).Error; err != nil {
		fmt.Printf("DiscoverGroups - No UserProfile found for user %d, trying old system: %v\n", user.ID, err)

		// Fallback to old User system
		var userModel models.User
		if err := storage.DB.First(&userModel, user.ID).Error; err != nil {
			fmt.Printf("DiscoverGroups - No User found for user %d: %v\n", user.ID, err)
			ctx.JSON(iris.Map{
				"success": false,
				"error":   "profile_incomplete",
				"message": "Please create your profile before discovering groups",
			})
			return
		}

		firstName = userModel.FirstName
		lastName = userModel.LastName
		hasProfile = firstName != "" || lastName != ""
		fmt.Printf("DiscoverGroups - Using old User system for user %d: FirstName='%s', LastName='%s'\n", user.ID, firstName, lastName)
	} else {
		firstName = userProfile.FirstName
		lastName = userProfile.LastName
		hasProfile = firstName != "" || lastName != ""
		fmt.Printf("DiscoverGroups - Using new UserProfile system for user %d: FirstName='%s', LastName='%s'\n", user.ID, firstName, lastName)
	}

	// Only require basic profile info (name), bio is optional
	if !hasProfile {
		ctx.JSON(iris.Map{
			"success": false,
			"error":   "profile_incomplete",
			"message": "Please add your name to your profile before discovering groups",
		})
		return
	}

	var input discoverGroupsInput
	ctx.ReadJSON(&input)

	// Set defaults
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Offset < 0 {
		input.Offset = 0
	}

	query := storage.DB.Model(&models.ExperienceGroup{}).
		Joins("JOIN experiences ON experiences.id = experience_groups.experience_id").
		Where("experience_groups.status != ?", "cancelled").
		Preload("Experience").
		Preload("Owner")

	// Privacy filter
	if input.Privacy == "public" {
		query = query.Where("experience_groups.privacy = ?", "public")
	} else if input.Privacy == "private" {
		query = query.Where("experience_groups.privacy = ?", "private")
	}

	// Location filter
	if input.Location != "" {
		query = query.Where("LOWER(experiences.city) LIKE LOWER(?)", "%"+input.Location+"%")
	}

	// Interests matching (improved implementation)
	if input.Interests != "" {
		interests := strings.Split(input.Interests, ",")
		interestConditions := []string{}
		args := []interface{}{}

		for _, interest := range interests {
			interest = strings.TrimSpace(interest)
			if interest != "" {
				interestConditions = append(interestConditions,
					"(LOWER(experiences.focus) LIKE LOWER(?) OR LOWER(experiences.description) LIKE LOWER(?) OR LOWER(experiences.title) LIKE LOWER(?))")
				args = append(args, "%"+interest+"%", "%"+interest+"%", "%"+interest+"%")
			}
		}

		if len(interestConditions) > 0 {
			query = query.Where(strings.Join(interestConditions, " OR "), args...)
		}
	} else {
		// If no interests specified, try to match based on user's profile
		// Match by user's languages if available
		var userLanguages []string

		// Try to get languages from UserProfile first, then fallback to User
		if userProfile.Languages != nil {
			json.Unmarshal(userProfile.Languages, &userLanguages)
		} else {
			// Fallback to old User system for languages
			var userModel models.User
			if err := storage.DB.First(&userModel, user.ID).Error; err == nil && userModel.Languages != nil {
				json.Unmarshal(userModel.Languages, &userLanguages)
			}
		}

		if len(userLanguages) > 0 {
			fmt.Printf("DiscoverGroups - User languages: %v\n", userLanguages)
			// Temporarily disable language filter for debugging
			fmt.Printf("DiscoverGroups - Language filter temporarily disabled for debugging\n")
			// languageConditions := []string{}
			// args := []interface{}{}

			// for _, lang := range userLanguages {
			// 	lang = strings.TrimSpace(lang)
			// 	if lang != "" {
			// 		languageConditions = append(languageConditions, "LOWER(experiences.language) LIKE LOWER(?)")
			// 		args = append(args, "%"+lang+"%")
			// 	}
			// }

			// if len(languageConditions) > 0 {
			// 	fmt.Printf("DiscoverGroups - Applying language filter: %s with args: %v\n", strings.Join(languageConditions, " OR "), args)
			// 	query = query.Where("("+strings.Join(languageConditions, " OR ")+")", args...)
			// }
		} else {
			fmt.Printf("DiscoverGroups - No user languages found, skipping language filter\n")
		}
	}

	// Exclude groups user already owns or is member of
	query = query.Where("experience_groups.owner_id != ?", user.ID).
		Where("NOT EXISTS (SELECT 1 FROM experience_group_members WHERE group_id = experience_groups.id AND user_id = ?)", user.ID)

	// Debug: Print the SQL query (commented out due to GORM version compatibility)
	// sqlQuery := query.ToSQL(func(tx *gorm.DB) *gorm.DB { return tx })
	// fmt.Printf("DiscoverGroups - SQL Query: %s\n", sqlQuery)

	fmt.Printf("DiscoverGroups - Query filters: privacy=%s, location=%s, interests=%s, limit=%d, offset=%d\n",
		input.Privacy, input.Location, input.Interests, input.Limit, input.Offset)

	// Debug: Check total groups in database
	var totalGroups int64
	storage.DB.Model(&models.ExperienceGroup{}).Count(&totalGroups)
	fmt.Printf("DiscoverGroups - Total groups in database: %d\n", totalGroups)

	// Debug: Check groups by status
	var pendingGroups, activeGroups, readyGroups int64
	storage.DB.Model(&models.ExperienceGroup{}).Where("status = ?", "pending").Count(&pendingGroups)
	storage.DB.Model(&models.ExperienceGroup{}).Where("status = ?", "active").Count(&activeGroups)
	storage.DB.Model(&models.ExperienceGroup{}).Where("status = ?", "ready").Count(&readyGroups)
	fmt.Printf("DiscoverGroups - Groups by status: pending=%d, active=%d, ready=%d\n", pendingGroups, activeGroups, readyGroups)

	// Debug: Check groups owned by this user
	var userGroups int64
	storage.DB.Model(&models.ExperienceGroup{}).Where("owner_id = ?", user.ID).Count(&userGroups)
	fmt.Printf("DiscoverGroups - Groups owned by user %d: %d\n", user.ID, userGroups)

	// Debug: Try a simple query first
	var simpleGroups []models.ExperienceGroup
	if err := storage.DB.Model(&models.ExperienceGroup{}).Preload("Experience").Preload("Owner").Find(&simpleGroups).Error; err != nil {
		fmt.Printf("DiscoverGroups - Simple query error: %v\n", err)
	} else {
		fmt.Printf("DiscoverGroups - Simple query found %d groups\n", len(simpleGroups))
		for i, group := range simpleGroups {
			fmt.Printf("  Simple Group %d: ID=%d, Name='%s', OwnerID=%d, Privacy='%s', Experience Language='%s'\n",
				i, group.ID, group.Name, group.OwnerID, group.Privacy, group.Experience.Language)
		}
	}

	// Debug: Test the ownership filter
	var ownershipFilteredGroups []models.ExperienceGroup
	ownershipQuery := storage.DB.Model(&models.ExperienceGroup{}).
		Joins("JOIN experiences ON experiences.id = experience_groups.experience_id").
		Where("experience_groups.status != ?", "cancelled").
		Where("experience_groups.owner_id != ?", user.ID).
		Preload("Experience").Preload("Owner")

	if err := ownershipQuery.Find(&ownershipFilteredGroups).Error; err != nil {
		fmt.Printf("DiscoverGroups - Ownership filter error: %v\n", err)
	} else {
		fmt.Printf("DiscoverGroups - After ownership filter: %d groups\n", len(ownershipFilteredGroups))
		for i, group := range ownershipFilteredGroups {
			fmt.Printf("  Ownership Filtered Group %d: ID=%d, Name='%s', OwnerID=%d, Privacy='%s'\n",
				i, group.ID, group.Name, group.OwnerID, group.Privacy)
		}
	}

	// Debug: Test the membership filter
	var membershipFilteredGroups []models.ExperienceGroup
	membershipQuery := storage.DB.Model(&models.ExperienceGroup{}).
		Joins("JOIN experiences ON experiences.id = experience_groups.experience_id").
		Where("experience_groups.status != ?", "cancelled").
		Where("experience_groups.owner_id != ?", user.ID).
		Where("NOT EXISTS (SELECT 1 FROM experience_group_members WHERE group_id = experience_groups.id AND user_id = ?)", user.ID).
		Preload("Experience").Preload("Owner")

	if err := membershipQuery.Find(&membershipFilteredGroups).Error; err != nil {
		fmt.Printf("DiscoverGroups - Membership filter error: %v\n", err)
	} else {
		fmt.Printf("DiscoverGroups - After membership filter: %d groups\n", len(membershipFilteredGroups))
		for i, group := range membershipFilteredGroups {
			fmt.Printf("  Membership Filtered Group %d: ID=%d, Name='%s', OwnerID=%d, Privacy='%s'\n",
				i, group.ID, group.Name, group.OwnerID, group.Privacy)
		}
	}

	var groups []models.ExperienceGroup
	if err := query.Limit(input.Limit).Offset(input.Offset).Find(&groups).Error; err != nil {
		fmt.Printf("DiscoverGroups - Query error: %v\n", err)
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	fmt.Printf("DiscoverGroups - Found %d groups for user %d\n", len(groups), user.ID)
	for i, group := range groups {
		fmt.Printf("  Group %d: ID=%d, Name='%s', Privacy='%s', Experience='%s'\n",
			i, group.ID, group.Name, group.Privacy, group.Experience.Title)
	}

	ctx.JSON(iris.Map{"success": true, "groups": groups})
}

type joinGroupRequestInput struct {
	GroupID uint   `json:"groupID"`
	Message string `json:"message"`
}

// RequestToJoinGroup allows travelers to request joining a group
func RequestToJoinGroup(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var input joinGroupRequestInput
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Check if group exists and is joinable
	var group models.ExperienceGroup
	if err := storage.DB.Preload("Owner").First(&group, input.GroupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Check if user already requested
	var existingRequest models.GroupJoinRequest
	if err := storage.DB.Where("group_id = ? AND requester_id = ?", input.GroupID, user.ID).First(&existingRequest).Error; err == nil {
		ctx.JSON(iris.Map{"success": false, "error": "already_requested"})
		return
	}

	// Check if user is already a member
	var existingMember models.ExperienceGroupMember
	if err := storage.DB.Where("group_id = ? AND user_id = ?", input.GroupID, user.ID).First(&existingMember).Error; err == nil {
		ctx.JSON(iris.Map{"success": false, "error": "already_member"})
		return
	}

	// Create join request
	request := models.GroupJoinRequest{
		GroupID:     input.GroupID,
		RequesterID: user.ID,
		Status:      "pending",
		Message:     input.Message,
	}
	if err := storage.DB.Create(&request).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// Get requester details
	var requester models.User
	storage.DB.First(&requester, user.ID)

	// Create notification for group owner
	notification := models.Notification{
		UserID:  group.OwnerID,
		Type:    "group_join_request",
		Title:   "New Group Join Request",
		Message: requester.FirstName + " " + requester.LastName + " wants to join your group \"" + group.Name + "\"",
		RefType: "group",
		RefID:   input.GroupID,
	}
	storage.DB.Create(&notification)

	ctx.JSON(iris.Map{"success": true, "request": request})
}

// RespondToJoinRequest allows group owners to accept/decline join requests
func RespondToJoinRequest(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	requestID, err := ctx.Params().GetUint("requestID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	var input struct {
		Action string `json:"action"` // accept, decline
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Get the request
	var request models.GroupJoinRequest
	if err := storage.DB.Preload("Group").Preload("Requester").First(&request, requestID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Check if user is the group owner
	if request.Group.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	// Check if request is already processed
	if request.Status != "pending" {
		ctx.JSON(iris.Map{"success": false, "error": "request_already_processed"})
		return
	}

	// Update request status
	now := time.Now()
	updates := map[string]interface{}{
		"status":       input.Action + "d",
		"responded_at": &now,
	}
	if err := storage.DB.Model(&request).Updates(updates).Error; err != nil {
		ctx.StopWithStatus(http.StatusInternalServerError)
		return
	}

	// If accepted, add user to group
	if input.Action == "accept" {
		// Check if user is already a member to prevent duplicates
		var existingMember models.ExperienceGroupMember
		if err := storage.DB.Where("group_id = ? AND user_id = ?", request.GroupID, request.RequesterID).First(&existingMember).Error; err != nil {
			// User is not a member, create new membership
			member := models.ExperienceGroupMember{
				GroupID:  request.GroupID,
				UserID:   request.RequesterID,
				State:    "joined",
				Role:     "member",
				JoinedAt: &now,
			}
			storage.DB.Create(&member)
		} else {
			// User is already a member, just update their state if needed
			if existingMember.State != "joined" {
				storage.DB.Model(&existingMember).Updates(map[string]interface{}{
					"state":     "joined",
					"joined_at": &now,
					"left_at":   nil,
				})
			}
		}

		// Create notification for requester
		notification := models.Notification{
			UserID:  request.RequesterID,
			Type:    "group_join_accepted",
			Title:   "Join Request Accepted",
			Message: "Your request to join \"" + request.Group.Name + "\" has been accepted!",
			RefType: "group",
			RefID:   request.GroupID,
		}
		storage.DB.Create(&notification)
	} else {
		// Create notification for requester
		notification := models.Notification{
			UserID:  request.RequesterID,
			Type:    "group_join_declined",
			Title:   "Join Request Declined",
			Message: "Your request to join \"" + request.Group.Name + "\" was declined",
			RefType: "group",
			RefID:   request.GroupID,
		}
		storage.DB.Create(&notification)
	}

	ctx.JSON(iris.Map{"success": true})
}

// GetMyJoinRequests returns user's join requests
func GetMyJoinRequests(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var requests []models.GroupJoinRequest
	storage.DB.Where("requester_id = ?", user.ID).
		Preload("Group").
		Preload("Group.Experience").
		Preload("Group.Owner").
		Order("created_at DESC").
		Find(&requests)

	ctx.JSON(iris.Map{"success": true, "requests": requests})
}

// GetGroupJoinRequests returns join requests for a group (owner only)
func GetGroupJoinRequests(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)
	groupID, err := ctx.Params().GetUint("groupID")
	if err != nil {
		ctx.StopWithStatus(http.StatusBadRequest)
		return
	}

	// Check if user owns the group
	var group models.ExperienceGroup
	if err := storage.DB.First(&group, groupID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}
	if group.OwnerID != user.ID {
		ctx.StopWithStatus(http.StatusForbidden)
		return
	}

	var requests []models.GroupJoinRequest
	storage.DB.Where("group_id = ?", groupID).
		Preload("Requester").
		Order("created_at DESC").
		Find(&requests)

	ctx.JSON(iris.Map{"success": true, "requests": requests})
}
