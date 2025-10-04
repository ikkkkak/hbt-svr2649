package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/kataras/iris/v12"
	jsonWT "github.com/kataras/iris/v12/middleware/jwt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
)

// SearchUsers allows searching users by name or email (auth required)
func SearchUsers(ctx iris.Context) {
	q := ctx.URLParamDefault("q", "")
	limit := ctx.URLParamIntDefault("limit", 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if len(q) < 1 {
		ctx.JSON(iris.Map{"success": true, "users": []interface{}{}})
		return
	}
	var users []models.User
	search := "%" + q + "%"
	storage.DB.Limit(limit).
		Where("lower(first_name) LIKE lower(?) OR lower(last_name) LIKE lower(?) OR lower(email) LIKE lower(?)", search, search, search).
		Select("id, first_name, last_name, avatar_url").
		Find(&users)
	ctx.JSON(iris.Map{"success": true, "users": users})
}

func Register(ctx iris.Context) {
	var userInput RegisterUserInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var newUser models.User
	userExists, userExistsErr := getAndHandleUserExists(&newUser, userInput.Email)
	if userExistsErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if userExists == true {
		utils.CreateEmailAlreadyRegistered(ctx)
		return
	}

	hashedPassword, hashErr := hashAndSaltPassword(userInput.Password)
	if hashErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	newUser = models.User{
		FirstName:   userInput.FirstName,
		LastName:    userInput.LastName,
		Email:       strings.ToLower(userInput.Email),
		Password:    hashedPassword,
		SocialLogin: false}

	storage.DB.Create(&newUser)

	returnUser(newUser, ctx)
}

func Login(ctx iris.Context) {
	var userInput LoginUserInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var existingUser models.User
	errorMsg := "Invalid email or password."
	userExists, userExistsErr := getAndHandleUserExists(&existingUser, userInput.Email)
	if userExistsErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if userExists == false {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", errorMsg, ctx)
		return
	}

	// Questionable as to whether you should let userInput know they logged in with Oauth
	// typically the fewer things said the better
	// If you don't want this, simply comment it out and the app will still work
	if existingUser.SocialLogin == true {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", "Social Login Account", ctx)
		return
	}

	passwordErr := bcrypt.CompareHashAndPassword([]byte(existingUser.Password), []byte(userInput.Password))
	if passwordErr != nil {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", errorMsg, ctx)
		return
	}

	returnUser(existingUser, ctx)
}

func RegisterPhone(ctx iris.Context) {
	var userInput RegisterPhoneInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Validate phone number format
	if !utils.ValidatePhoneNumber(userInput.PhoneNumber) {
		utils.CreateError(iris.StatusBadRequest, "Validation Error", "Invalid phone number format. Mauritanian phone numbers must be 8 digits starting with 2, 3, or 4.", ctx)
		return
	}

	var newUser models.User
	userExists, userExistsErr := getAndHandleUserExistsByPhone(&newUser, userInput.PhoneNumber)
	if userExistsErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if userExists == true {
		utils.CreateError(iris.StatusConflict, "Registration Error", "Phone number already registered.", ctx)
		return
	}

	hashedPassword, hashErr := hashAndSaltPassword(userInput.Password)
	if hashErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	// Format phone number for storage
	formattedPhone := utils.NormalizePhoneNumber(userInput.PhoneNumber)

	newUser = models.User{
		FirstName:   userInput.FirstName,
		LastName:    userInput.LastName,
		PhoneNumber: formattedPhone,
		Password:    hashedPassword,
	}

	storage.DB.Create(&newUser)

	returnUser(newUser, ctx)
}

func LoginPhone(ctx iris.Context) {
	var userInput LoginPhoneInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	// Validate phone number format
	if !utils.ValidatePhoneNumber(userInput.PhoneNumber) {
		utils.CreateError(iris.StatusBadRequest, "Validation Error", "Invalid phone number format. Mauritanian phone numbers must be 8 digits starting with 2, 3, or 4.", ctx)
		return
	}

	var existingUser models.User
	errorMsg := "Invalid phone number or password."
	userExists, userExistsErr := getAndHandleUserExistsByPhone(&existingUser, userInput.PhoneNumber)
	if userExistsErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if userExists == false {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", errorMsg, ctx)
		return
	}

	// Check if it's a social login account
	if existingUser.SocialLogin == true {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", "Social Login Account", ctx)
		return
	}

	passwordErr := bcrypt.CompareHashAndPassword([]byte(existingUser.Password), []byte(userInput.Password))
	if passwordErr != nil {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", errorMsg, ctx)
		return
	}

	returnUser(existingUser, ctx)
}

func FacebookLoginOrSignUp(ctx iris.Context) {
	var userInput FacebookOrGoogleUserInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	endpoint := "https://graph.facebook.com/me?fields=id,name,email&access_token=" + userInput.AccessToken
	client := &http.Client{}
	req, _ := http.NewRequest("GET", endpoint, nil)
	res, facebookErr := client.Do(req)
	if facebookErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	defer res.Body.Close()
	body, bodyErr := ioutil.ReadAll(res.Body)
	if bodyErr != nil {
		log.Panic(bodyErr)
		utils.CreateInternalServerError(ctx)
		return
	}

	var facebookBody FacebookUserRes
	json.Unmarshal(body, &facebookBody)

	if facebookBody.Email != "" {
		var user models.User
		userExists, userExistsErr := getAndHandleUserExists(&user, facebookBody.Email)

		if userExistsErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}

		if userExists == false {
			nameArr := strings.SplitN(facebookBody.Name, " ", 2)
			user = models.User{FirstName: nameArr[0], LastName: nameArr[1], Email: facebookBody.Email, SocialLogin: true, SocialProvider: "Facebook"}
			storage.DB.Create(&user)

			returnUser(user, ctx)
			return
		}

		if user.SocialLogin == true && user.SocialProvider == "Facebook" {
			returnUser(user, ctx)
			return
		}

		utils.CreateEmailAlreadyRegistered(ctx)
		return
	}
}

func GoogleLoginOrSignUp(ctx iris.Context) {
	var userInput FacebookOrGoogleUserInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	endpoint := "https://www.googleapis.com/userinfo/v2/me"

	client := &http.Client{}
	req, _ := http.NewRequest("GET", endpoint, nil)
	header := "Bearer " + userInput.AccessToken
	req.Header.Set("Authorization", header)
	res, googleErr := client.Do(req)
	if googleErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	defer res.Body.Close()
	body, bodyErr := ioutil.ReadAll(res.Body)
	if bodyErr != nil {
		log.Panic(bodyErr)
		utils.CreateInternalServerError(ctx)
		return
	}

	var googleBody GoogleUserRes
	json.Unmarshal(body, &googleBody)

	if googleBody.Email != "" {
		var user models.User
		userExists, userExistsErr := getAndHandleUserExists(&user, googleBody.Email)

		if userExistsErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}

		if userExists == false {
			user = models.User{FirstName: googleBody.GivenName, LastName: googleBody.FamilyName, Email: googleBody.Email, SocialLogin: true, SocialProvider: "Google"}
			storage.DB.Create(&user)

			returnUser(user, ctx)
			return
		}

		if user.SocialLogin == true && user.SocialProvider == "Google" {
			returnUser(user, ctx)
			return
		}

		utils.CreateEmailAlreadyRegistered(ctx)
		return

	}
}

func AppleLoginOrSignUp(ctx iris.Context) {
	var userInput AppleUserInput
	err := ctx.ReadJSON(&userInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	res, httpErr := http.Get("https://appleid.apple.com/auth/keys")
	if httpErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	defer res.Body.Close()

	body, bodyErr := ioutil.ReadAll(res.Body)
	if bodyErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	jwks, jwksErr := keyfunc.NewJSON(body)
	//The JWKS.Keyfunc method will automatically select the key with the matching kid (if present) and return its public key as the correct Go type to its caller.
	token, tokenErr := jwt.Parse(userInput.IdentityToken, jwks.Keyfunc)

	if jwksErr != nil || tokenErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if !token.Valid {
		utils.CreateError(iris.StatusUnauthorized, "Unauthorized", "Invalid user token.", ctx)
		return
	}

	email := fmt.Sprint(token.Claims.(jwt.MapClaims)["email"])
	if email != "" {
		var user models.User
		userExists, userExistsErr := getAndHandleUserExists(&user, email)

		if userExistsErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}

		if userExists == false {
			user = models.User{FirstName: "", LastName: "", Email: email, SocialLogin: true, SocialProvider: "Apple"}
			storage.DB.Create(&user)

			returnUser(user, ctx)
			return
		}

		if user.SocialLogin == true && user.SocialProvider == "Apple" {
			returnUser(user, ctx)
			return
		}

		utils.CreateEmailAlreadyRegistered(ctx)
		return
	}
}

func ForgotPassword(ctx iris.Context) {
	var emailInput EmailRegisteredInput
	err := ctx.ReadJSON(&emailInput)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var user models.User
	userExists, userExistsErr := getAndHandleUserExists(&user, emailInput.Email)

	if userExistsErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if !userExists {
		utils.CreateError(iris.StatusUnauthorized, "Credentials Error", "Invalid email.", ctx)
		return
	}

	if userExists {
		if user.SocialLogin {
			utils.CreateError(iris.StatusUnauthorized, "Credentials Error", "Social Login Account", ctx)
			return
		}

		link := "exp://192.168.30.24:19000/--/resetpassword/"
		token, tokenErr := utils.CreateForgotPasswordToken(user.ID, user.Email)

		if tokenErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}

		link += token
		subject := "Forgot Your Password?"

		html := `
		<p>It looks like you forgot your password. 
		If you did, please click the link below to reset it. 
		If you did not, disregard this email. Please update your password
		within 10 minutes, otherwise you will have to repeat this
		process. <a href=` + link + `>Click to Reset Password</a>
		</p><br />`

		emailSent, emailSentErr := utils.SendMail(user.Email, subject, html)
		if emailSentErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}

		if emailSent {
			ctx.JSON(iris.Map{
				"emailSent": true,
			})
			return
		}

		ctx.JSON(iris.Map{"emailSent": false})
	}
}

func ResetPassword(ctx iris.Context) {
	var password ResetPasswordInput
	err := ctx.ReadJSON(&password)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	hashedPassword, hashErr := hashAndSaltPassword(password.Password)
	if hashErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	claims := jsonWT.Get(ctx).(*utils.ForgotPasswordToken)

	var user models.User
	storage.DB.Model(&user).Where("id = ?", claims.ID).Update("password", hashedPassword)

	ctx.JSON(iris.Map{
		"passwordReset": true,
	})
}

func GetUserSavedProperties(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	user := getUserByID(id, ctx)
	if user == nil {
		return
	}

	var properties []models.Property
	var savedProperties []uint
	unmarshalErr := json.Unmarshal(user.SavedProperties, &savedProperties)
	if unmarshalErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	propertiesExist := storage.DB.Where("id IN ?", savedProperties).Find(&properties)

	if propertiesExist.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(properties)
}

func AlterUserSavedProperties(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	user := getUserByID(id, ctx)
	if user == nil {
		return
	}

	var req AlterSavedPropertiesInput
	err := ctx.ReadJSON(&req)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	propertyID := strconv.FormatUint(uint64(req.PropertyID), 10)

	validPropertyID := GetPropertyAndAssociationsByPropertyID(propertyID, ctx)

	if validPropertyID == nil {
		return
	}

	var savedProperties []uint
	var unMarshalledProperties []uint

	if user.SavedProperties != nil {
		unmarshalErr := json.Unmarshal(user.SavedProperties, &unMarshalledProperties)

		if unmarshalErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}
	}

	if req.Op == "add" {
		if !slices.Contains(unMarshalledProperties, req.PropertyID) {
			savedProperties = append(unMarshalledProperties, req.PropertyID)
		} else {
			savedProperties = unMarshalledProperties
		}
	} else if req.Op == "remove" && len(unMarshalledProperties) > 0 {
		for _, propertyID := range unMarshalledProperties {
			if req.PropertyID != propertyID {
				savedProperties = append(savedProperties, propertyID)
			}
		}
	}

	marshalledProperties, marshalErr := json.Marshal(savedProperties)

	if marshalErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	user.SavedProperties = marshalledProperties

	rowsUpdated := storage.DB.Model(&user).Updates(user)

	if rowsUpdated.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.StatusCode(iris.StatusNoContent)
}

func GetUserContactedProperties(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	var conversations []models.Conversation
	conversationsExist := storage.DB.Where("tenant_id = ?", id).Find(&conversations)
	if conversationsExist.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	if conversationsExist.RowsAffected == 0 {
		utils.CreateNotFound(ctx)
		return
	}

	var properties []models.Property
	var propertyIDs []uint
	for _, conversation := range conversations {
		propertyIDs = append(propertyIDs, conversation.PropertyID)
	}

	propertiesExist := storage.DB.Where("id IN ?", propertyIDs).Find(&properties)

	if propertiesExist.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(properties)
}

func AlterPushToken(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	user := getUserByID(id, ctx)
	if user == nil {
		return
	}

	var req AlterPushTokenInput
	err := ctx.ReadJSON(&req)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	var unMarshalledTokens []string
	var pushTokens []string

	if user.PushTokens != nil {
		unmarshalErr := json.Unmarshal(user.PushTokens, &unMarshalledTokens)

		if unmarshalErr != nil {
			utils.CreateInternalServerError(ctx)
			return
		}
	}

	if req.Op == "add" {
		if !slices.Contains(unMarshalledTokens, req.Token) {
			pushTokens = append(unMarshalledTokens, req.Token)
		} else {
			pushTokens = unMarshalledTokens
		}
	} else if req.Op == "replace" {
		// Log old tokens before replacing
		if len(unMarshalledTokens) > 0 {
			log.Printf("🔄 TOKEN REPLACE: REPLACING OLD TOKENS for user %d:", user.ID)
			for i, token := range unMarshalledTokens {
				log.Printf("🔄 OLD TOKEN %d: %s", i+1, token)
			}
		} else {
			log.Printf("🔄 TOKEN REPLACE: No old tokens found for user %d", user.ID)
		}

		// Replace all tokens with this new one
		pushTokens = []string{req.Token}
		log.Printf("🔄 TOKEN REPLACE: NEW TOKEN for user %d: %s", user.ID, req.Token)
		log.Printf("✅ TOKEN REPLACE: Successfully replaced all tokens with new token for user %d", user.ID)
	} else if req.Op == "remove" && len(unMarshalledTokens) > 0 {
		for _, token := range unMarshalledTokens {
			if req.Token != token {
				pushTokens = append(pushTokens, token)
			}
		}
	}

	marshalledTokens, marshalErr := json.Marshal(pushTokens)

	if marshalErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	user.PushTokens = marshalledTokens

	rowsUpdated := storage.DB.Model(&user).Updates(user)

	if rowsUpdated.Error != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.StatusCode(iris.StatusNoContent)
}

func AllowsNotifications(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	log.Printf("🔄 NOTIFICATIONS ENDPOINT: Received request for user %s", id)

	user := getUserByID(id, ctx)
	if user == nil {
		log.Printf("❌ NOTIFICATIONS ERROR: User %s not found", id)
		return
	}

	var req AllowsNotificationsInput
	err := ctx.ReadJSON(&req)
	if err != nil {
		log.Printf("❌ NOTIFICATIONS ERROR: Failed to parse JSON: %v", err)
		utils.HandleValidationErrors(err, ctx)
		return
	}

	log.Printf("🔄 NOTIFICATIONS REQUEST: User %s, Setting to: %v", id, req.AllowsNotifications != nil && *req.AllowsNotifications)

	user.AllowsNotifications = req.AllowsNotifications

	// Clear push tokens when notifications are disabled
	if req.AllowsNotifications != nil && !*req.AllowsNotifications {
		// Log the old tokens before clearing
		if user.PushTokens != nil {
			var oldTokens []string
			if json.Unmarshal(user.PushTokens, &oldTokens) == nil {
				log.Printf("🗑️ TOKENS BEING CLEARED for user %d:", user.ID)
				for i, token := range oldTokens {
					log.Printf("🗑️ OLD TOKEN %d: %s", i+1, token)
				}
			}
		}

		// First update allowsNotifications
		updateResult := storage.DB.Model(&user).Update("allows_notifications", false)
		if updateResult.Error != nil {
			log.Printf("❌ NOTIFICATIONS ERROR: Failed to update allows_notifications: %v", updateResult.Error)
			utils.CreateInternalServerError(ctx)
			return
		}

		// Then explicitly clear push_tokens using raw SQL to ensure NULL is set
		clearTokensResult := storage.DB.Model(&user).Update("push_tokens", nil)
		if clearTokensResult.Error != nil {
			log.Printf("❌ TOKENS ERROR: Failed to clear push_tokens: %v", clearTokensResult.Error)
			utils.CreateInternalServerError(ctx)
			return
		}

		log.Printf("🗑️ TOKENS CLEARED: PushTokens explicitly set to NULL for user %d", user.ID)
	} else if req.AllowsNotifications != nil && *req.AllowsNotifications {
		log.Printf("✅ NOTIFICATIONS ENABLED: User %d enabled notifications", user.ID)
		// Update allowsNotifications to true
		updateResult := storage.DB.Model(&user).Update("allows_notifications", true)
		if updateResult.Error != nil {
			log.Printf("❌ NOTIFICATIONS ERROR: Failed to update allows_notifications: %v", updateResult.Error)
			utils.CreateInternalServerError(ctx)
			return
		}
	}

	// Skip the general Updates() call since we're doing specific updates above
	var rowsUpdated *gorm.DB
	if req.AllowsNotifications == nil {
		// Only do general update if we're not handling notifications
		rowsUpdated = storage.DB.Model(&user).Updates(user)
	} else {
		// For notification updates, we've already done the specific updates above
		rowsUpdated = &gorm.DB{Error: nil} // Fake success since we already updated
	}

	if rowsUpdated.Error != nil {
		log.Printf("❌ NOTIFICATIONS ERROR: Database update failed: %v", rowsUpdated.Error)
		utils.CreateInternalServerError(ctx)
		return
	}

	// Verify the update by re-fetching the user
	var updatedUser models.User
	storage.DB.First(&updatedUser, user.ID)

	if req.AllowsNotifications != nil {
		log.Printf("✅ NOTIFICATIONS VERIFIED: User %d - AllowsNotifications in DB: %v", user.ID, updatedUser.AllowsNotifications != nil && *updatedUser.AllowsNotifications)

		if !*req.AllowsNotifications {
			// Verify tokens were cleared
			if updatedUser.PushTokens == nil {
				log.Printf("✅ TOKENS VERIFIED: Tokens successfully cleared for user %d", user.ID)
			} else {
				log.Printf("❌ TOKENS VERIFICATION FAILED: Tokens still exist for user %d", user.ID)
			}
		}
	}

	ctx.StatusCode(iris.StatusNoContent)
}

func getAndHandleUserExists(user *models.User, email string) (exists bool, err error) {
	userExistsQuery := storage.DB.Where("email = ?", strings.ToLower(email)).Limit(1).Find(&user)

	if userExistsQuery.Error != nil {
		return false, userExistsQuery.Error
	}

	userExists := userExistsQuery.RowsAffected > 0

	if userExists == true {
		return true, nil
	}

	return false, nil
}

func getAndHandleUserExistsByPhone(user *models.User, phoneNumber string) (exists bool, err error) {
	// Format phone number before lookup
	formattedPhone := utils.NormalizePhoneNumber(phoneNumber)
	userExistsQuery := storage.DB.Where("phone_number = ?", formattedPhone).Limit(1).Find(&user)

	if userExistsQuery.Error != nil {
		return false, userExistsQuery.Error
	}

	userExists := userExistsQuery.RowsAffected > 0

	if userExists == true {
		return true, nil
	}

	return false, nil
}

func hashAndSaltPassword(password string) (hashedPassword string, err error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func getUserByID(id string, ctx iris.Context) *models.User {
	var user models.User
	userExists := storage.DB.Where("id = ?", id).Find(&user)

	if userExists.Error != nil {
		utils.CreateInternalServerError(ctx)
		return nil
	}

	if userExists.RowsAffected == 0 {
		utils.CreateError(iris.StatusNotFound, "Not Found", "User not found", ctx)
		return nil
	}

	return &user
}

func UpdateUserProfile(ctx iris.Context) {
	params := ctx.Params()
	id := params.Get("id")

	user := getUserByID(id, ctx)
	if user == nil {
		return
	}

	claims := jsonWT.Get(ctx).(*utils.AccessToken)
	if user.ID != claims.ID {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}

	var input UpdateProfileInput
	err := ctx.ReadJSON(&input)
	if err != nil {
		utils.HandleValidationErrors(err, ctx)
		return
	}

	fmt.Printf("UpdateUserProfile - Received input: FirstName='%s', LastName='%s', AvatarURL='%s'\n",
		input.FirstName, input.LastName, input.AvatarURL)

	// Upload avatar if provided
	avatarURL := input.AvatarURL
	if avatarURL != "" && !strings.Contains(avatarURL, "res.cloudinary.com") {
		// Generate unique filename with timestamp
		timestamp := time.Now().UnixNano() / int64(time.Millisecond)
		publicID := fmt.Sprintf("hosts/%d/avatar_%d", user.ID, timestamp)
		urlMap := storage.UploadBase64Image(avatarURL, publicID)
		if urlMap != nil && urlMap["url"] != "" {
			avatarURL = urlMap["url"]
		}
	}

	// Convert arrays to JSON, ensure never null
	languages := input.Languages
	if languages == nil {
		languages = []string{}
	}
	languagesJSON, _ := json.Marshal(languages)

	skills := input.Skills
	if skills == nil {
		skills = []string{}
	}
	skillsJSON, _ := json.Marshal(skills)

	// Update user profile
	user.FirstName = input.FirstName
	user.LastName = input.LastName
	user.AvatarURL = avatarURL
	user.DateOfBirth = input.DateOfBirth
	user.Bio = input.Bio
	user.Languages = languagesJSON
	user.Skills = skillsJSON

	storage.DB.Save(user)

	fmt.Printf("UpdateUserProfile - Saved user: FirstName='%s', LastName='%s', AvatarURL='%s'\n",
		user.FirstName, user.LastName, user.AvatarURL)

	ctx.JSON(iris.Map{
		"ID":          user.ID,
		"firstName":   user.FirstName,
		"lastName":    user.LastName,
		"email":       user.Email,
		"avatarURL":   user.AvatarURL,
		"dateOfBirth": user.DateOfBirth,
		"bio":         user.Bio,
		"languages":   input.Languages,
		"skills":      input.Skills,
	})
}

func GetUser(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{
			"message": "User ID not found in context",
		})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{
			"message": "Invalid user ID format",
		})
		return
	}

	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{
			"message": "User not found",
		})
		return
	}

	// Normalize JSON fields (languages/skills) to arrays
	var langs []string
	if len(user.Languages) > 0 {
		_ = json.Unmarshal(user.Languages, &langs)
	}
	var skills []string
	if len(user.Skills) > 0 {
		_ = json.Unmarshal(user.Skills, &skills)
	}

	// Pointer safe boolean
	isVerified := false
	if user.IsVerified != nil {
		isVerified = *user.IsVerified
	}

	ctx.JSON(iris.Map{
		"ID":                 user.ID,
		"firstName":          user.FirstName,
		"lastName":           user.LastName,
		"email":              user.Email,
		"phoneNumber":        user.PhoneNumber,
		"avatarURL":          user.AvatarURL,
		"dateOfBirth":        user.DateOfBirth,
		"bio":                user.Bio,
		"languages":          langs,
		"skills":             skills,
		"isVerified":         isVerified,
		"verificationStatus": user.VerificationStatus,
	})
}

// GetUserProfileStatus returns the profile completion status for group discovery
func GetUserProfileStatus(ctx iris.Context) {
	tok := jsonWT.Get(ctx)
	if tok == nil {
		ctx.StopWithStatus(http.StatusUnauthorized)
		return
	}
	user := tok.(*utils.AccessToken)

	var userProfile models.User
	if err := storage.DB.First(&userProfile, user.ID).Error; err != nil {
		ctx.StopWithStatus(http.StatusNotFound)
		return
	}

	// Check profile completion criteria
	hasName := userProfile.FirstName != "" || userProfile.LastName != ""
	hasBio := userProfile.Bio != ""
	hasAvatar := userProfile.AvatarURL != ""

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
			"firstName": userProfile.FirstName,
			"lastName":  userProfile.LastName,
			"bio":       userProfile.Bio,
			"avatarURL": userProfile.AvatarURL,
			"email":     userProfile.Email,
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

func SubmitVerification(ctx iris.Context) {
	fmt.Printf("=== VERIFICATION SUBMISSION START ===\n")
	userID := ctx.Values().Get("userID").(uint)
	var input VerificationInput

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message": "Invalid input",
			"error":   err.Error(),
		})
		return
	}

	fmt.Printf("Input received - IDType: %s, IDNumber: %s, FrontImage length: %d\n",
		input.IDType, input.IDNumber, len(input.IDFrontImage))

	// Validate input
	if input.IDType == "" || input.IDNumber == "" || input.IDFrontImage == "" || input.IDBackImage == "" || input.SelfieImage == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{
			"message": "All verification fields are required",
		})
		return
	}

	// Get user
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{
			"message": "User not found",
		})
		return
	}

	// Upload verification images to Cloudinary
	idFrontURL := input.IDFrontImage
	if !strings.Contains(idFrontURL, "res.cloudinary.com") {
		urlMap := storage.UploadBase64Image(idFrontURL, "verification/"+strconv.FormatUint(uint64(user.ID), 10)+"/id_front")
		if urlMap != nil && urlMap["url"] != "" {
			idFrontURL = urlMap["url"]
		}
	}

	idBackURL := input.IDBackImage
	if !strings.Contains(idBackURL, "res.cloudinary.com") {
		urlMap := storage.UploadBase64Image(idBackURL, "verification/"+strconv.FormatUint(uint64(user.ID), 10)+"/id_back")
		if urlMap != nil && urlMap["url"] != "" {
			idBackURL = urlMap["url"]
		}
	}

	selfieURL := input.SelfieImage
	if !strings.Contains(selfieURL, "res.cloudinary.com") {
		urlMap := storage.UploadBase64Image(selfieURL, "verification/"+strconv.FormatUint(uint64(user.ID), 10)+"/selfie")
		if urlMap != nil && urlMap["url"] != "" {
			selfieURL = urlMap["url"]
		}
	}

	// Update user verification data
	user.IDType = input.IDType
	user.IDNumber = input.IDNumber
	user.IDFrontImage = idFrontURL
	user.IDBackImage = idBackURL
	user.SelfieImage = selfieURL
	user.VerificationStatus = "pending"
	user.IsVerified = &[]bool{false}[0]

	// Save to database
	if err := storage.DB.Save(&user).Error; err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{
			"message": "Failed to save verification data",
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"message": "Verification submitted successfully",
		"user":    user,
	})
}

func returnUser(user models.User, ctx iris.Context) {
	tokenPair, tokenErr := utils.CreateTokenPair(user.ID)
	if tokenErr != nil {
		utils.CreateInternalServerError(ctx)
		return
	}

	ctx.JSON(iris.Map{
		"ID":                  user.ID,
		"firstName":           user.FirstName,
		"lastName":            user.LastName,
		"email":               user.Email,
		"phoneNumber":         user.PhoneNumber,
		"savedProperties":     user.SavedProperties,
		"allowsNotifications": user.AllowsNotifications,
		"accessToken":         string(tokenPair.AccessToken),
		"refreshToken":        string(tokenPair.RefreshToken),
	})

}

type RegisterUserInput struct {
	FirstName string `json:"firstName" validate:"required,max=256"`
	LastName  string `json:"lastName" validate:"required,max=256"`
	Email     string `json:"email" validate:"required,max=256,email"`
	Password  string `json:"password" validate:"required,min=8,max=256"`
}

type UpdateProfileInput struct {
	FirstName   string   `json:"firstName"`
	LastName    string   `json:"lastName"`
	AvatarURL   string   `json:"avatarURL"`
	DateOfBirth string   `json:"dateOfBirth"`
	Bio         string   `json:"bio"`
	Languages   []string `json:"languages"`
	Skills      []string `json:"skills"`
}

type VerificationInput struct {
	IDType       string `json:"idType"`
	IDNumber     string `json:"idNumber"`
	IDFrontImage string `json:"idFrontImage"`
	IDBackImage  string `json:"idBackImage"`
	SelfieImage  string `json:"selfieImage"`
}

type LoginUserInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type RegisterPhoneInput struct {
	FirstName   string `json:"firstName" validate:"required,max=256"`
	LastName    string `json:"lastName" validate:"required,max=256"`
	PhoneNumber string `json:"phoneNumber" validate:"required"`
	Password    string `json:"password" validate:"required,min=8,max=256"`
}

type LoginPhoneInput struct {
	PhoneNumber string `json:"phoneNumber" validate:"required"`
	Password    string `json:"password" validate:"required"`
}

type FacebookOrGoogleUserInput struct {
	AccessToken string `json:"accessToken" validate:"required"`
}

type AppleUserInput struct {
	IdentityToken string `json:"identityToken" validate:"required"`
}

type FacebookUserRes struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type GoogleUserRes struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
}

type EmailRegisteredInput struct {
	Email string `json:"email" validate:"required"`
}

type ResetPasswordInput struct {
	Password string `json:"password" validate:"required,min=8,max=256"`
}

type AlterSavedPropertiesInput struct {
	PropertyID uint   `json:"propertyID" validate:"required"`
	Op         string `json:"op" validate:"required"`
}

type AlterPushTokenInput struct {
	Token string `json:"token" validate:"required"`
	Op    string `json:"op" validate:"required"`
}

type AllowsNotificationsInput struct {
	AllowsNotifications *bool `json:"allowsNotifications" validate:"required"`
}
