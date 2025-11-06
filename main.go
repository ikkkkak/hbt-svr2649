package main

import (
	"apartments-clone-server/models"
	"apartments-clone-server/routes"
	"apartments-clone-server/services"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	websocketHub "apartments-clone-server/websocket"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
)

// Optional authentication middleware - allows requests with or without JWT tokens
func optionalAuthMiddleware(ctx iris.Context) {
	authHeader := ctx.GetHeader("Authorization")
	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		// Token present, try to verify it
		accessTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
		accessTokenVerifier.WithDefaultBlocklist()
		accessTokenVerifierMiddleware := accessTokenVerifier.Verify(func() interface{} {
			return new(utils.AccessToken)
		})

		// Attempt verification (do not rely on status code)
		accessTokenVerifierMiddleware(ctx)
		if claims := jwt.Get(ctx); claims != nil {
			if accessToken, ok := claims.(*utils.AccessToken); ok {
				ctx.Values().Set("userID", accessToken.ID)
				fmt.Printf("🔍 Optional auth: User ID %d authenticated\n", accessToken.ID)
			}
		} else {
			fmt.Printf("🔍 Optional auth: Invalid token - proceeding without auth\n")
		}
	} else {
		fmt.Printf("🔍 Optional auth: No token or invalid token - proceeding without auth\n")
	}
	// Always continue to the next handler
	ctx.Next()
}

// Helper functions for notification text
func getNotificationTitle(language, notificationType string) string {
	notifications := map[string]map[string]string{
		"en": {
			"discover": "Discover Properties",
			"explore":  "Explore Properties",
		},
		"ar": {
			"discover": "اكتشف العقارات",
			"explore":  "استكشف العقارات",
		},
		"fr": {
			"discover": "Découvrez des propriétés",
			"explore":  "Explorez les propriétés",
		},
	}

	if lang, exists := notifications[language]; exists {
		if title, exists := lang[notificationType]; exists {
			return title
		}
	}

	// Fallback to English
	return notifications["en"][notificationType]
}

func getNotificationBody(language, notificationType, location string) string {
	notifications := map[string]map[string]string{
		"en": {
			"discover": "Discover amazing properties near you!",
			"explore":  "Discover properties in " + location,
		},
		"ar": {
			"discover": "اكتشف عقارات مذهلة بالقرب منك!",
			"explore":  "اكتشف العقارات في " + location,
		},
		"fr": {
			"discover": "Découvrez des propriétés incroyables près de chez vous !",
			"explore":  "Découvrez des propriétés à " + location,
		},
	}

	if lang, exists := notifications[language]; exists {
		if body, exists := lang[notificationType]; exists {
			return body
		}
	}

	// Fallback to English
	return notifications["en"][notificationType]
}

func main() {
	fmt.Println("🚀 Starting apartments-clone-server...")
	fmt.Println("🔍 Debug: Main function started")

	// Only load .env in development
	if os.Getenv("RENDER") == "" {
		fmt.Println("🔍 Debug: Loading .env file...")
		godotenv.Load()
		fmt.Println("📁 Loaded .env file")
	} else {
		fmt.Println("🌐 Running on Render (production)")
	}

	fmt.Println("🔍 Debug: About to initialize services...")

	// Initialize services with error handling
	fmt.Println("🔧 Initializing database...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ Panic during database initialization: %v\n", r)
				fmt.Println("⚠️  Continuing without database...")
			}
		}()
		storage.InitializeDB()
		fmt.Println("✅ Database initialized successfully")
		// Auto-migrate chat and moderation tables (idempotent)
		if err := storage.DB.AutoMigrate(
			&models.ChatMessage{},
			&models.HiddenProperty{}, &models.PropertyReport{}, &models.UserFlag{}, &models.HiddenVideo{}, &models.PropertySaleReport{}, &models.LandmarkReport{}, &models.HiddenPropertySale{}, &models.PropertySaleVideo{}, &models.PropertySaleVideoLike{}, &models.PropertySaleVideoSave{}, &models.PropertySaleVideoComment{}, &models.PropertySaleVideoReport{}, &models.HiddenPropertySaleVideo{}, &models.UserBlockedOrganization{},
			&models.City{}, &models.Zone{},
		); err != nil {
			fmt.Printf("❌ Failed to migrate moderation tables: %v\n", err)
		} else {
			fmt.Println("✅ Tables migrated (chat_messages, hidden_properties, property_reports, user_flags, hidden_videos, property_sale_reports, landmark_reports, property_sale_videos, user_blocked_organizations)")
		}
	}()

	fmt.Println("🔧 Initializing S3...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ Panic during S3 initialization: %v\n", r)
				fmt.Println("⚠️  Continuing without S3...")
			}
		}()
		storage.InitializeS3()
		fmt.Println("✅ S3 initialized successfully")
	}()

	fmt.Println("🔧 Initializing Redis...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ Panic during Redis initialization: %v\n", r)
				fmt.Println("⚠️  Continuing without Redis...")
			}
		}()
		storage.InitializeRedis()
		fmt.Println("✅ Redis initialized successfully")
	}()

	// Initialize FCM (Firebase Cloud Messaging)
	fmt.Println("🔧 Initializing FCM...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ Panic during FCM initialization: %v\n", r)
				fmt.Println("⚠️  Continuing without FCM (will use Expo Push fallback)...")
			}
		}()
		if err := pushsvc.InitializeFCM(); err != nil {
			log.Printf("⚠️ FCM initialization failed: %v", err)
			log.Printf("⚠️ Push notifications will fall back to Expo Push service")
		} else {
			fmt.Println("✅ FCM initialized successfully")
		}
	}()

	// Start background push worker
	pushsvc.StartPushWorker()

	fmt.Println("🔧 Initializing WebSocket Hub...")
	websocketHub.InitHub()
	fmt.Println("✅ WebSocket Hub initialized successfully")

	fmt.Println("🔧 Creating Iris app...")
	app := iris.New()
	app.Validator = validator.New()

	// CORS configuration
	fmt.Println("🔧 Setting up CORS...")
	app.AllowMethods(iris.MethodOptions)
	app.UseRouter(func(ctx iris.Context) {
		ctx.Header("Access-Control-Allow-Origin", ctx.GetHeader("Origin"))
		ctx.Header("Vary", "Origin")
		ctx.Header("Access-Control-Allow-Credentials", "true")
		ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
		ctx.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
		// If Authorization header is missing but cookie exists, promote cookie to header
		if ctx.Method() != iris.MethodOptions {
			if ctx.GetHeader("Authorization") == "" {
				if tok := ctx.GetCookie("accessToken"); tok != "" {
					ctx.Request().Header.Set("Authorization", "Bearer "+tok)
				}
			}
		}
		if ctx.Method() == iris.MethodOptions {
			ctx.StatusCode(iris.StatusNoContent)
			return
		}
		ctx.Next()
	})

	// Minimal middleware - compression only
	fmt.Println("🔧 Setting up middleware...")
	app.Use(iris.Compression)

	// JWT Verifiers
	resetTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("EMAIL_TOKEN_SECRET")))
	resetTokenVerifier.WithDefaultBlocklist()
	resetTokenVerifierMiddleware := resetTokenVerifier.Verify(func() interface{} {
		return new(utils.ForgotPasswordToken)
	})

	accessTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
	accessTokenVerifier.WithDefaultBlocklist()
	accessTokenVerifierMiddleware := accessTokenVerifier.Verify(func() interface{} {
		return new(utils.AccessToken)
	})
	// Fixed JWT middleware - ensure userID is set before handler
	originalVerifier := accessTokenVerifier.Verify(func() interface{} {
		return new(utils.AccessToken)
	})

	accessTokenVerifierMiddleware = func(ctx iris.Context) {
		fmt.Printf("🔍 JWT Middleware - Starting authentication\n")

		// Call the original middleware
		originalVerifier(ctx)

		// Extract userID from claims if token was valid
		if claims := jwt.Get(ctx); claims != nil {
			if accessToken, ok := claims.(*utils.AccessToken); ok {
				ctx.Values().Set("userID", accessToken.ID)
				fmt.Printf("✅ JWT Middleware - User ID %d authenticated and set in context\n", accessToken.ID)
			} else {
				fmt.Printf("❌ JWT Middleware - Claims not AccessToken type: %T\n", claims)
			}
		} else {
			fmt.Printf("❌ JWT Middleware - No claims found\n")
		}

		// Verify userID is set before calling next
		if userID, ok := ctx.Values().Get("userID").(uint); ok && userID > 0 {
			fmt.Printf("✅ JWT Middleware - UserID %d confirmed in context before next\n", userID)
		} else {
			fmt.Printf("❌ JWT Middleware - UserID not properly set in context - BLOCKING REQUEST\n")
			ctx.StatusCode(401)
			ctx.JSON(iris.Map{"error": "unauthorized"})
			return
		}

		ctx.Next()
	}

	refreshTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("REFRESH_TOKEN_SECRET")))
	refreshTokenVerifier.WithDefaultBlocklist()
	refreshTokenVerifierMiddleware := refreshTokenVerifier.Verify(func() interface{} {
		return new(jwt.Claims)
	})

	refreshTokenVerifier.Extractors = append(refreshTokenVerifier.Extractors, func(ctx iris.Context) string {
		var tokenInput utils.RefreshTokenInput
		err := ctx.ReadJSON(&tokenInput)
		if err != nil {
			return ""
		}
		return tokenInput.RefreshToken
	})

	// Health check endpoint - CRITICAL for Render
	fmt.Println("🔧 Setting up health check endpoint...")
	app.Get("/health", func(ctx iris.Context) {
		ctx.JSON(iris.Map{"status": "ok", "message": "Server is running"})
	})
	// Secondary health check path to match current Render setting
	app.Get("/healthz", func(ctx iris.Context) {
		ctx.JSON(iris.Map{"status": "ok", "message": "Server is running"})
	})

	// Simple test endpoint
	app.Get("/test", func(ctx iris.Context) {
		ctx.JSON(iris.Map{"status": "ok", "message": "Test endpoint working"})
	})

	// Debug notification settings for a user
	app.Get("/api/debug/notifications/{userID:uint}", func(ctx iris.Context) {
		userID, _ := ctx.Params().GetUint("userID")

		var user models.User
		if err := storage.DB.First(&user, userID).Error; err != nil {
			ctx.StatusCode(iris.StatusNotFound)
			ctx.JSON(iris.Map{"error": "User not found"})
			return
		}

		// Debug notification settings
		notificationService := services.NewNotificationService()
		notificationService.DebugUserNotificationSettings(userID)

		// Return user notification info
		var pushTokens []string
		if user.PushTokens != nil {
			json.Unmarshal(user.PushTokens, &pushTokens)
		}

		ctx.JSON(iris.Map{
			"userID":              userID,
			"allowsNotifications": user.AllowsNotifications != nil && *user.AllowsNotifications,
			"hasPushTokens":       user.PushTokens != nil,
			"pushTokenCount":      len(pushTokens),
			"pushTokens":          pushTokens,
		})
	})

	// Test notification endpoints
	app.Get("/api/notifications/test", func(ctx iris.Context) {
		ctx.JSON(iris.Map{
			"status":  "ok",
			"message": "Notification endpoints are working",
			"endpoints": []string{
				"POST /api/notifications/register",
				"POST /api/notifications/send-location",
			},
		})
	})

	// Routes
	user := app.Party("/api/user")
	{
		user.Post("/register", routes.Register)
		user.Post("/login", routes.Login)
		user.Post("/register-phone", routes.RegisterPhone)
		user.Post("/login-phone", routes.LoginPhone)
		user.Post("/facebook", routes.FacebookLoginOrSignUp)
		user.Post("/google", routes.GoogleLoginOrSignUp)
		user.Post("/apple", routes.AppleLoginOrSignUp)
		user.Post("/forgotpassword", routes.ForgotPassword)
		user.Post("/resetpassword", resetTokenVerifierMiddleware, routes.ResetPassword)
		user.Get("/search", accessTokenVerifierMiddleware, routes.SearchUsers)
		user.Get("/{id}/properties/saved", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetUserSavedProperties)
		user.Patch("/{id}/properties/saved", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.AlterUserSavedProperties)
		user.Patch("/{id}/pushtoken", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.AlterPushToken)
		user.Patch("/{id}/settings/notifications", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.AllowsNotifications)
		user.Get("/{id}/properties/contacted", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetUserContactedProperties)
		user.Patch("/{id}/profile", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.UpdateUserProfile)
		user.Get("/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUser)
		user.Get("/profile/status", accessTokenVerifierMiddleware, routes.GetUserProfileStatusNew)
		user.Post("/verification", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SubmitVerification)
		// Feedback
		user.Post("/feedback", accessTokenVerifierMiddleware, routes.CreateFeedback)

		// User Profile routes
		user.Get("/profile", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUserProfile)
		user.Post("/profile", accessTokenVerifierMiddleware, routes.CreateOrUpdateUserProfile)
		user.Put("/profile", accessTokenVerifierMiddleware, routes.CreateOrUpdateUserProfile)
		user.Delete("/profile", accessTokenVerifierMiddleware, routes.DeleteUserProfile)
		user.Delete("/account", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeleteUserAccount)

		// User Wishlist routes
		user.Get("/wishlist", accessTokenVerifierMiddleware, routes.GetUserWishlist)
		user.Post("/wishlist", accessTokenVerifierMiddleware, routes.AddToUserWishlist)
		user.Delete("/wishlist/{propertyID:uint}", accessTokenVerifierMiddleware, routes.RemoveFromUserWishlist)

		// User moderation routes
		user.Get("/blocked", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetBlockedUsers)
		user.Get("/hidden-properties", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHiddenProperties)
		user.Get("/hidden-videos", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHiddenVideos)
	}

	// Video reporting routes (Public - optional auth for better filtering)
	app.Post("/api/videos/{id:uint}/report", optionalAuthMiddleware, routes.ReportVideoPublic)
	app.Post("/api/users/{id:uint}/flag", optionalAuthMiddleware, routes.FlagUserPublic)
	app.Post("/api/videos/{id:uint}/hide", optionalAuthMiddleware, routes.HideVideoPublic)
	// Unblock/unhide routes (authenticated)
	app.Delete("/api/users/{id:uint}/unblock", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UnblockUser)
	app.Delete("/api/videos/{id:uint}/unhide", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UnhideVideo)
	app.Post("/api/users/{id:uint}/block", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.BlockUser)

	// Admin routes for video reports
	app.Get("/api/admin/flagged-videos", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetFlaggedVideos)
	app.Put("/api/admin/reports/{id:uint}/status", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateReportStatus)

	property := app.Party("/api/property")
	{
		property.Post("/", routes.CreateProperty)
		property.Get("/{id}", routes.GetProperty)
		property.Get("/userid/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetPropertiesByUserID)
		property.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteProperty)
		property.Patch("/update/{id}", accessTokenVerifierMiddleware, routes.UpdateProperty)
		property.Post("/search", optionalAuthMiddleware, routes.GetPropertiesByBoundingBox)
		property.Delete("/image", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeletePropertyImage)
	}

	// Image upload endpoint
	app.Post("/api/upload-image", func(ctx iris.Context) {
		var req struct {
			Image    string `json:"image"`
			PublicID string `json:"publicId"`
		}

		if err := ctx.ReadJSON(&req); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid JSON"})
			return
		}

		if req.Image == "" {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Image is required"})
			return
		}

		// Upload to Cloudinary
		urlMap := storage.UploadBase64Image(req.Image, req.PublicID)
		if urlMap == nil || urlMap["url"] == "" {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to upload image to Cloudinary"})
			return
		}

		ctx.JSON(iris.Map{"url": urlMap["url"]})
	})

	// Admin routes
	admin := app.Party("/api/admin", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		admin.Get("/users", routes.AdminListUsers)
		admin.Patch("/users/{id:uint}/role", utils.SuperAdminOnlyMiddleware, routes.AdminChangeUserRole)
		admin.Get("/users/{id:uint}", routes.AdminGetUser)
		admin.Post("/users/{id:uint}/verify", routes.AdminVerifyUser)
		admin.Get("/properties", routes.AdminListProperties)
		admin.Get("/properties/{id:uint}", routes.AdminGetProperty)
		admin.Patch("/properties/{id:uint}/status", routes.AdminUpdatePropertyStatus)
		admin.Post("/properties/{id:uint}/flag", routes.AdminFlagProperty)
		admin.Get("/experiences", routes.AdminListExperiences)
		admin.Get("/experiences/{id:uint}", routes.AdminGetExperience)
		admin.Patch("/experiences/{id:uint}/status", routes.AdminUpdateExperienceStatus)
		admin.Get("/reservations", routes.AdminListReservations)
		admin.Get("/reservations/{id:uint}", routes.AdminGetReservation)
		admin.Post("/reservations/{id:uint}/cancel", routes.AdminCancelReservation)
		admin.Patch("/reservations/{id:uint}/status", routes.AdminUpdateReservationStatus)
		admin.Get("/reviews", routes.AdminListReviews)
		admin.Patch("/reviews/{id:uint}/status", routes.AdminUpdateReviewVisibility)
		admin.Delete("/reviews/{id:uint}", routes.AdminDeleteReview)
		admin.Get("/videos", routes.AdminListVideos)
		admin.Get("/videos/{id:uint}", routes.AdminGetVideo)
		admin.Patch("/videos/{id:uint}/status", routes.AdminUpdateVideoStatus)
		admin.Post("/videos/{id:uint}/force_unpublish", routes.AdminForceUnpublishVideo)
		admin.Get("/videos/{id:uint}/comments", routes.AdminListVideoComments)
		admin.Delete("/videos/{id:uint}/comments/{comment_id:uint}", routes.AdminDeleteVideoComment)
		// Promotional videos management
		admin.Post("/videos/promotional", routes.AdminCreatePromotionalVideo)
		admin.Get("/videos/promotional", routes.AdminListPromotionalVideos)
		admin.Patch("/videos/promotional/{id:uint}", routes.AdminUpdatePromotionalVideo)
		admin.Delete("/videos/promotional/{id:uint}", routes.AdminDeletePromotionalVideo)
		admin.Get("/feedback", routes.AdminListFeedback)
		admin.Get("/stats", routes.AdminStats)
		admin.Get("/activity", routes.AdminActivity)
		admin.Get("/groups", routes.AdminListGroups)
		admin.Get("/groups/{id:uint}", routes.AdminGetGroup)
		admin.Patch("/groups/{id:uint}", routes.AdminUpdateGroup)
		admin.Post("/export", routes.AdminCreateExport)
		admin.Get("/export/{id:string}", routes.AdminGetExport)
	}

	availability := app.Party("/api/availability")
	{
		availability.Get("/property/{propertyID}", routes.GetPropertyAvailability)
		availability.Post("/property", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetPropertyAvailability)
		availability.Post("/property/bulk", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetBulkPropertyAvailability)
		availability.Get("/pricing/{propertyID}", routes.GetPropertyPricing)
		availability.Post("/pricing", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetPropertyPricing)
		availability.Get("/discounts/{propertyID}", routes.GetPropertyDiscounts)
		availability.Post("/discounts", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertyDiscount)
		availability.Post("/block", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.BlockPropertyDates)
		availability.Get("/blocks/{propertyID}", routes.GetPropertyBlocks)
		availability.Post("/calculate-price", routes.CalculateBookingPrice)
	}

	categories := app.Party("/api/categories")
	{
		categories.Get("/", routes.GetCategories)
		categories.Get("/amenities", routes.GetAmenities)
		categories.Get("/amenities/categories", routes.GetAmenityCategories)
		categories.Get("/property/{id}", routes.GetPropertyCategories)
		categories.Get("/property/{id}/amenities", routes.GetPropertyAmenities)
		categories.Put("/property/{id}", accessTokenVerifierMiddleware, routes.UpdatePropertyCategories)
		categories.Put("/property/{id}/amenities", accessTokenVerifierMiddleware, routes.UpdatePropertyAmenities)
	}

	location := app.Party("/api/location")
	{
		location.Get("/near/{location}", routes.GetPropertiesNearLocation)
		location.Get("/locations", routes.GetAvailableLocations)
		location.Get("/coordinates", optionalAuthMiddleware, routes.GetPropertiesByCoordinates)
		location.Get("/search", optionalAuthMiddleware, routes.GetPropertiesWithFilters)
	}

	// Nearby POIs (schools, hospitals, restaurants)
	app.Get("/api/nearby", routes.NearbyHandler)

	apartment := app.Party("/api/apartment")
	{
		apartment.Get("/property/{id}", routes.GetReservationsByPropertyID)
		apartment.Post("/property/{id}", accessTokenVerifierMiddleware, routes.CreateReservation)
		apartment.Patch("/{id}/status", accessTokenVerifierMiddleware, routes.UpdateReservationStatus)
		apartment.Post("/expire-pending", routes.ExpirePendingReservations)
		apartment.Delete("/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CancelReservation)
		apartment.Post("/property/{id}/validate", routes.ValidateReservationAvailability)
		apartment.Get("/host/reservations", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostReservations)
		apartment.Patch("/{id}/mark-viewed", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkReservationAsViewed)
	}

	// New canonical reservations routes
	reservations := app.Party("/api/reservations")
	{
		reservations.Get("/user/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetUserReservations)
	}

	review := app.Party("/api/review")
	{
		review.Post("/property/{id}", accessTokenVerifierMiddleware, routes.CreateReview)
	}

	conversation := app.Party("/api/conversation")
	{
		conversation.Post("/", accessTokenVerifierMiddleware, routes.CreateConversation)
		conversation.Get("/{id}", accessTokenVerifierMiddleware, routes.GetConversationByID)
		conversation.Get("/user/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetConversationsByUserID)
	}

	messages := app.Party("/api/messages")
	{
		messages.Post("/", accessTokenVerifierMiddleware, routes.CreateMessage)
		messages.Get("/", accessTokenVerifierMiddleware, routes.ListMessages)
		messages.Post("/state", accessTokenVerifierMiddleware, routes.SetMessageState)
	}

	notifications := app.Party("/api/notifications")
	// Upload routes (Cloudinary)
	upload := app.Party("/api/upload")
	{
		upload.Post("/image", routes.UploadImage)
		upload.Post("/video", routes.UploadVideo)
	}
	{
		notifications.Post("/test-push", routes.SendTestNotification)
		notifications.Post("/test-detailed/{userID:int}", routes.SendDetailedTestNotification)
		notifications.Post("/welcome", routes.SendWelcomeNotification)
		notifications.Get("/settings", accessTokenVerifierMiddleware, routes.GetUserNotificationSettings)
		notifications.Put("/settings", accessTokenVerifierMiddleware, routes.UpdateUserNotificationSettings)
		// Add the missing notification endpoints
		notifications.Post("/register", func(ctx iris.Context) {
			var req struct {
				UserID      *uint  `json:"user_id"` // Nullable for anonymous users
				PushToken   string `json:"push_token"`
				Language    string `json:"language"`
				Location    string `json:"location"`
				Coordinates struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"coordinates"`
			}
			if err := ctx.ReadJSON(&req); err != nil {
				fmt.Printf("❌ Invalid JSON in registration request: %v\n", err)
				ctx.StatusCode(iris.StatusBadRequest)
				ctx.JSON(iris.Map{"error": "Invalid JSON"})
				return
			}

			fmt.Printf("📝 Received registration request: user_id=%v, language=%s, location=%s, push_token=%s\n",
				req.UserID, req.Language, req.Location, req.PushToken[:20]+"...")

			// Create or update notification preference in database
			now := time.Now()
			pref := models.NotificationPreference{
				UserID:     req.UserID,
				PushToken:  req.PushToken,
				Language:   req.Language,
				Location:   req.Location,
				Latitude:   req.Coordinates.Latitude,
				Longitude:  req.Coordinates.Longitude,
				Enabled:    true,
				LastActive: &now,
			}

			// Use push token as unique identifier (since it's unique per device)
			var existingPref models.NotificationPreference
			result := storage.DB.Where("push_token = ?", req.PushToken).First(&existingPref)

			if result.Error != nil {
				// Create new record
				if err := storage.DB.Create(&pref).Error; err != nil {
					fmt.Printf("❌ Error creating notification preference: %v\n", err)
					ctx.StatusCode(iris.StatusInternalServerError)
					ctx.JSON(iris.Map{"error": "Failed to create notification preference"})
					return
				}
				fmt.Printf("✅ Created new notification preference for push token: %s\n", req.PushToken[:20]+"...")
			} else {
				// Update existing record
				now := time.Now()
				existingPref.Language = req.Language
				existingPref.Location = req.Location
				existingPref.Latitude = req.Coordinates.Latitude
				existingPref.Longitude = req.Coordinates.Longitude
				existingPref.Enabled = true
				existingPref.LastActive = &now
				existingPref.UserID = req.UserID // Update user ID in case user logged in

				if err := storage.DB.Save(&existingPref).Error; err != nil {
					fmt.Printf("❌ Error updating notification preference: %v\n", err)
					ctx.StatusCode(iris.StatusInternalServerError)
					ctx.JSON(iris.Map{"error": "Failed to update notification preference"})
					return
				}
				fmt.Printf("✅ Updated notification preference for push token: %s\n", req.PushToken[:20]+"...")
			}

			ctx.JSON(iris.Map{
				"success": true,
				"message": "Notification preferences saved",
			})
		})
		notifications.Post("/send-location", func(ctx iris.Context) {
			fmt.Printf("📨 Received POST request to /api/notifications/send-location\n")

			var req struct {
				UserID      *uint  `json:"user_id"` // Nullable for anonymous users
				Language    string `json:"language"`
				Location    string `json:"location"`
				Coordinates struct {
					Latitude  float64 `json:"latitude"`
					Longitude float64 `json:"longitude"`
				} `json:"coordinates"`
			}
			if err := ctx.ReadJSON(&req); err != nil {
				fmt.Printf("❌ Failed to parse JSON: %v\n", err)
				ctx.StatusCode(iris.StatusBadRequest)
				ctx.JSON(iris.Map{"error": "Invalid JSON"})
				return
			}

			fmt.Printf("✅ Successfully parsed JSON request\n")

			fmt.Printf("🔔 Received notification request: user_id=%v, language=%s, location=%s\n", req.UserID, req.Language, req.Location)

			// Debug: Show all notification preferences in database
			var allPrefs []models.NotificationPreference
			storage.DB.Find(&allPrefs)
			fmt.Printf("🔍 Total notification preferences in database: %d\n", len(allPrefs))
			for i, p := range allPrefs {
				if i < 5 { // Show first 5
					fmt.Printf("  %d: user_id=%v, location=%s, enabled=%v, push_token=%s\n",
						i+1, p.UserID, p.Location, p.Enabled, p.PushToken[:20]+"...")
				}
			}

			// Find notification preference by user ID or push token
			var pref models.NotificationPreference
			var query *gorm.DB

			if req.UserID != nil {
				// Look for registered user
				fmt.Printf("🔍 Looking for registered user with ID: %d\n", *req.UserID)
				query = storage.DB.Where("user_id = ? AND enabled = ?", *req.UserID, true).First(&pref)
			} else {
				// Look for anonymous user by location (approximate match)
				fmt.Printf("🔍 Looking for anonymous user with location: %s\n", req.Location)
				query = storage.DB.Where("user_id IS NULL AND enabled = ? AND location = ?", true, req.Location).First(&pref)
			}

			if query.Error != nil {
				fmt.Printf("❌ No notification preferences found: %v\n", query.Error)
				fmt.Printf("🔍 Tried query: user_id=%v, location=%s\n", req.UserID, req.Location)
				ctx.StatusCode(iris.StatusNotFound)
				ctx.JSON(iris.Map{"error": "User notification preferences not found"})
				return
			}

			fmt.Printf("✅ Found notification preference for push token: %s\n", pref.PushToken[:20]+"...")

			// RATE LIMITING: Check if notifications were sent recently (within last 2 hours)
			if pref.LastNotificationSent != nil {
				timeSinceLastNotification := time.Since(*pref.LastNotificationSent)
				if timeSinceLastNotification < 2*time.Hour {
					fmt.Printf("⏰ Rate limiting: Last notification sent %v ago, skipping to prevent spam\n", timeSinceLastNotification)
					ctx.JSON(iris.Map{
						"success":      true,
						"message":      "Notifications rate limited - too recent",
						"rate_limited": true,
					})
					return
				}
			}

			// Update last active time and notification sent time
			now := time.Now()
			pref.LastActive = &now
			pref.LastNotificationSent = &now
			storage.DB.Save(&pref)

			// Schedule single notification with proper timing
			go func() {
				fmt.Printf("⏰ Scheduling notification for user (app went to background)...\n")

				// Single notification after 10 seconds (when app is definitely in background)
				time.Sleep(10 * time.Second)

				notification := map[string]interface{}{
					"to":    pref.PushToken,
					"title": getNotificationTitle(pref.Language, "discover"),
					"body":  getNotificationBody(pref.Language, "discover", pref.Location),
					"data": map[string]interface{}{
						"type":        "location_discovery",
						"location":    pref.Location,
						"coordinates": map[string]float64{"latitude": pref.Latitude, "longitude": pref.Longitude},
					},
				}

				fmt.Printf("📤 Sending notification (app in background)...\n")
				expoPushURL := "https://exp.host/--/api/v2/push/send"
				notificationJSON, _ := json.Marshal(notification)
				httpReq, _ := http.NewRequest("POST", expoPushURL, bytes.NewBuffer(notificationJSON))
				httpReq.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(httpReq)
				if err != nil {
					fmt.Printf("❌ Error sending notification: %v\n", err)
				} else {
					fmt.Printf("✅ Notification sent, status: %d\n", resp.StatusCode)
					resp.Body.Close()
				}
			}()

			fmt.Printf("✅ Background notification scheduled for user\n")
			ctx.JSON(iris.Map{
				"success": true,
				"message": "Background notification scheduled",
			})
		})
	}

	collection := app.Party("/api/collection")
	{
		collection.Post("/", accessTokenVerifierMiddleware, routes.CreateCollection)
		collection.Get("/", accessTokenVerifierMiddleware, routes.GetUserCollections)
		collection.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateCollection)
		collection.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteCollection)
		collection.Post("/add-property", accessTokenVerifierMiddleware, routes.AddPropertyToCollection)
		collection.Post("/remove-property", accessTokenVerifierMiddleware, routes.RemovePropertyFromCollection)
		collection.Post("/remove-from-all", accessTokenVerifierMiddleware, routes.RemovePropertyFromAllCollections)
		collection.Get("/{id}/properties", accessTokenVerifierMiddleware, routes.GetCollectionProperties)
	}

	experienceCollection := app.Party("/api/experience-collection")
	{
		experienceCollection.Post("/", accessTokenVerifierMiddleware, routes.CreateExperienceCollection)
		experienceCollection.Get("/", accessTokenVerifierMiddleware, routes.GetUserExperienceCollections)
		experienceCollection.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateExperienceCollection)
		experienceCollection.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteExperienceCollection)
		experienceCollection.Post("/add-experience", accessTokenVerifierMiddleware, routes.AddExperienceToCollection)
		experienceCollection.Post("/remove-experience", accessTokenVerifierMiddleware, routes.RemoveExperienceFromCollection)
		experienceCollection.Post("/remove-from-all", accessTokenVerifierMiddleware, routes.RemoveExperienceFromAllCollections)
		experienceCollection.Get("/{id}/experiences", accessTokenVerifierMiddleware, routes.GetCollectionExperiences)
		experienceCollection.Get("/saved", accessTokenVerifierMiddleware, routes.GetUserSavedExperiences)
	}

	video := app.Party("/api/video")
	{
		video.Post("/", routes.CreateVideo)
		video.Get("/feed", routes.GetVideoFeed)
		video.Post("/like", accessTokenVerifierMiddleware, routes.LikeVideo)
		video.Post("/unlike", accessTokenVerifierMiddleware, routes.UnlikeVideo)
		video.Post("/save", accessTokenVerifierMiddleware, routes.SaveVideo)
		video.Post("/unsave", accessTokenVerifierMiddleware, routes.UnsaveVideo)
		video.Post("/comment", accessTokenVerifierMiddleware, routes.CreateVideoComment)
		video.Get("/comment/{videoID}", optionalAuthMiddleware, routes.GetVideoComments)
		video.Put("/comment/{id}", accessTokenVerifierMiddleware, routes.UpdateVideoComment)
		video.Delete("/comment/{id}", accessTokenVerifierMiddleware, routes.DeleteVideoComment)
		video.Post("/comment/like", accessTokenVerifierMiddleware, routes.LikeVideoComment)
		video.Post("/comment/unlike", accessTokenVerifierMiddleware, routes.UnlikeVideoComment)
		video.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteVideo)
		video.Get("/liked", accessTokenVerifierMiddleware, routes.GetLikedVideos)
		video.Get("/saved", accessTokenVerifierMiddleware, routes.GetSavedVideos)
	}

	// Property Sale Videos routes
	propertySaleVideos := app.Party("/api/property-sale-videos")
	{
		propertySaleVideos.Post("/", accessTokenVerifierMiddleware, routes.CreatePropertySaleVideo)
		propertySaleVideos.Get("/feed", optionalAuthMiddleware, routes.GetPropertySaleVideoFeed)
		propertySaleVideos.Post("/{id:uint}/like", accessTokenVerifierMiddleware, routes.LikePropertySaleVideo)
		propertySaleVideos.Post("/{id:uint}/unlike", accessTokenVerifierMiddleware, routes.UnlikePropertySaleVideo)
		propertySaleVideos.Post("/{id:uint}/save", accessTokenVerifierMiddleware, routes.SavePropertySaleVideo)
		propertySaleVideos.Post("/{id:uint}/unsave", accessTokenVerifierMiddleware, routes.UnsavePropertySaleVideo)
		propertySaleVideos.Get("/{id:uint}/comments", optionalAuthMiddleware, routes.GetPropertySaleVideoComments)
		propertySaleVideos.Post("/{id:uint}/comments", accessTokenVerifierMiddleware, routes.CreatePropertySaleVideoComment)
	}

	experience := app.Party("/api/experience")
	{
		experience.Post("/", accessTokenVerifierMiddleware, routes.CreateExperience)
		experience.Get("/", accessTokenVerifierMiddleware, routes.GetUserExperiences)
		experience.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateExperience)
		experience.Post("/{id}/submit", accessTokenVerifierMiddleware, routes.SubmitExperienceForReview)
		experience.Get("/{id}", routes.GetExperienceDetails)
		experience.Get("/public", routes.GetPublicExperiences)
		// Invites & participants
		experience.Post("/{id}/invites", accessTokenVerifierMiddleware, routes.CreateExperienceInvites)
		experience.Get("/{id}/participants", routes.ListParticipants)
		experience.Post("/{id}/participants/{userID}/remove", accessTokenVerifierMiddleware, routes.RemoveParticipant)
		// Groups - Simplified
		experience.Post("/{id}/groups", accessTokenVerifierMiddleware, routes.CreateOrOpenGroup)
		experience.Get("/groups", accessTokenVerifierMiddleware, routes.ListMyGroups)
		experience.Get("/groups/{groupId}/members", accessTokenVerifierMiddleware, routes.GetGroupMembers)
		experience.Post("/groups/{groupId}/join", accessTokenVerifierMiddleware, routes.JoinGroup)
		experience.Post("/groups/{groupId}/leave", accessTokenVerifierMiddleware, routes.LeaveGroup)
		experience.Delete("/groups/{groupId}", accessTokenVerifierMiddleware, routes.DeleteGroup)
		// Availability
		experience.Get("/{id}/availability", routes.ListAvailability)
		experience.Post("/{id}/availability", accessTokenVerifierMiddleware, routes.SetAvailability)
		// Booking
		experience.Post("/book", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateExperienceBooking)
		experience.Get("/bookings", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetExperienceBookings)
		experience.Get("/host-bookings", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostExperienceBookings)
		experience.Patch("/bookings/{id}/mark-read", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkBookingAsRead)
		experience.Delete("/bookings/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CancelExperienceBooking)
	}

	invites := app.Party("/api/invites")
	{
		invites.Get("/", accessTokenVerifierMiddleware, routes.ListInvites)
		invites.Post("/{inviteID}/accept", accessTokenVerifierMiddleware, routes.AcceptInvite)
		invites.Post("/{inviteID}/decline", accessTokenVerifierMiddleware, routes.DeclineInvite)
		invites.Post("/{inviteID}/cancel", accessTokenVerifierMiddleware, routes.CancelInvite)
	}

	groups := app.Party("/api/groups")
	{
		groups.Get("/mine", accessTokenVerifierMiddleware, routes.ListMyGroups)
		groups.Get("/{groupID}/members", routes.GetGroupMembers)
		groups.Post("/{groupID}/members/{memberID}/role", accessTokenVerifierMiddleware, routes.UpdateMemberRole)
		groups.Post("/{groupID}/members/{memberID}/remove", accessTokenVerifierMiddleware, routes.RemoveGuest)
		groups.Post("/{groupID}/leave", accessTokenVerifierMiddleware, routes.LeaveGroup)
		groups.Post("/{groupID}/finalize", accessTokenVerifierMiddleware, routes.FinalizeGroup)
		groups.Put("/{groupID}", accessTokenVerifierMiddleware, routes.UpdateGroup)
		groups.Delete("/{groupID}", accessTokenVerifierMiddleware, routes.DeleteGroup)
		// Chat
		groups.Get("/{groupID}/messages", accessTokenVerifierMiddleware, routes.ListGroupMessages)
		groups.Post("/{groupID}/messages", accessTokenVerifierMiddleware, routes.SendGroupMessage)
		groups.Get("/{groupID}/messages/{msgId:uint}/reads", accessTokenVerifierMiddleware, routes.GetMessageReads)
		groups.Post("/{groupID}/messages/{msgId:uint}/reads", accessTokenVerifierMiddleware, routes.MarkMessageRead)
		groups.Post("/{groupID}/typing", accessTokenVerifierMiddleware, routes.Typing)
		groups.Get("/{groupID}/typing", accessTokenVerifierMiddleware, routes.ListTyping)
		// WebSocket endpoint (use existing authenticated hub)
		groups.Get("/{groupID:uint}/ws", websocketHub.HandleWebSocket)
		groups.Get("/{groupID:uint}/ws2", websocketHub.HandleWebSocket)
		// Broadcast hook for newly created messages (call after DB save)
		groups.Post("/{groupID:uint}/messages/broadcast", routes.BroadcastGroupMessageHandler)
		// Wishlist
		groups.Get("/{groupID}/wishlist", accessTokenVerifierMiddleware, routes.ListGroupWishlist)
		groups.Post("/{groupID}/wishlist", accessTokenVerifierMiddleware, routes.AddGroupWishlist)
		groups.Post("/{groupID}/wishlist/{wishlistID}/like", accessTokenVerifierMiddleware, routes.LikeGroupWishlist)
		// Share
		groups.Post("/{groupID}/share/property", accessTokenVerifierMiddleware, routes.SharePropertyToGroup)
		// Discovery
		groups.Post("/discover", accessTokenVerifierMiddleware, routes.DiscoverGroups)
		groups.Post("/request-join", accessTokenVerifierMiddleware, routes.RequestToJoinGroup)
		groups.Get("/my-requests", accessTokenVerifierMiddleware, routes.GetMyJoinRequests)
		// groups.Get("/{groupID}/requests", accessTokenVerifierMiddleware, routes.GetGroupJoinRequests)
		groups.Post("/requests/{requestID}/respond", accessTokenVerifierMiddleware, routes.RespondToJoinRequest)
		// Group Management - Clean Implementation
		groups.Post("/{groupID}/quit", accessTokenVerifierMiddleware, routes.QuitGroup)
		groups.Post("/{groupID}/block/{userID:uint}", accessTokenVerifierMiddleware, routes.BlockUserInGroup)
		groups.Delete("/{groupID}/unblock/{userID:uint}", accessTokenVerifierMiddleware, routes.UnblockUserInGroup)
		groups.Get("/{groupID}/quit-history", accessTokenVerifierMiddleware, routes.GetGroupQuitHistory)
		groups.Get("/{groupID}/blocked-users", accessTokenVerifierMiddleware, routes.GetBlockedUsersInGroup)

		// Group Invite Codes
		groups.Post("/{id:uint}/invite-code", accessTokenVerifierMiddleware, routes.GenerateInviteCode)
		groups.Get("/invite/{code}", routes.GetGroupByInviteCode)
		groups.Post("/invite/join", accessTokenVerifierMiddleware, routes.JoinGroupWithCode)
	}

	chat := app.Party("/api/chat")
	{
		chat.Post("/start-direct", accessTokenVerifierMiddleware, routes.StartDirectConversation)
	}

	// Direct Messages - Clean Implementation
	directMessages := app.Party("/api/direct-messages")
	{
		directMessages.Post("/", accessTokenVerifierMiddleware, routes.SendDirectMessage)
		directMessages.Get("/{userID:uint}", accessTokenVerifierMiddleware, routes.GetDirectMessages)
		directMessages.Post("/{messageID:uint}/read", accessTokenVerifierMiddleware, routes.MarkDirectMessageRead)
	}

	// User Blocking for Direct Messages - Clean Implementation
	userBlocks := app.Party("/api/user-blocks")
	{
		userBlocks.Post("/{userID:uint}", accessTokenVerifierMiddleware, routes.BlockUser)
		userBlocks.Delete("/{userID:uint}", accessTokenVerifierMiddleware, routes.UnblockUser)
		userBlocks.Get("/", accessTokenVerifierMiddleware, routes.GetBlockedUsers)
	}

	// Location Discovery routes (Public with optional auth for better filtering)
	locationDiscovery := app.Party("/api/location-discovery")
	{
		locationDiscovery.Get("/criteria", routes.GetLocationCriteria)
		locationDiscovery.Get("/criteria/{criteriaId}/properties", routes.GetLocationProperties)
		locationDiscovery.Get("/property/{propertyId}/criteria", routes.GetPropertyLocationCriteria)
		locationDiscovery.Post("/initialize", routes.InitializeLocationCriteriaEndpoint)
		locationDiscovery.Post("/assign-properties", routes.AssignPropertiesToCriteriaEndpoint)
	}

	// Expo push token registration (simple, unauthenticated demo; in prod use JWT)
	app.Post("/api/users/push-token", routes.RegisterPushToken)

	// Legacy compatibility: conversations endpoint expected by client
	app.Get("/api/conversations", routes.ListConversationsAlias)

	// Properties Search
	properties := app.Party("/api/properties")
	{
		properties.Get("/search", routes.SearchProperties)
		// Public moderation endpoints (optional auth)
		properties.Post("/{id:uint}/hide", optionalAuthMiddleware, routes.HidePropertyPublic)
		properties.Post("/{id:uint}/report", optionalAuthMiddleware, routes.ReportPropertyPublic)
		// Unhide property (authenticated)
		properties.Delete("/{id:uint}/unhide", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UnhideProperty)
	}

	// Reviews
	reviews := app.Party("/api/reviews")
	{
		reviews.Get("/property/{propertyId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ListPropertyReviews)
		reviews.Post("/property/{propertyId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertyReview)
	}

	// Property Selling System Routes
	organization := app.Party("/api/organization")
	{
		organization.Post("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateOrganization)
		organization.Get("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUserOrganization)
		organization.Put("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateOrganization)
		organization.Get("/agents", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationAgents)
		organization.Post("/agents", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.AddAgent)
		organization.Patch("/agents/{agentID:uint}/status", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateAgentStatus)
		// Organization moderation (user-level)
		organization.Post("/{id:uint}/block", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.BlockOrganization)
		organization.Delete("/{id:uint}/unblock", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UnblockOrganization)
		organization.Get("/blocked", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetBlockedOrganizations)
	}

	propertySales := app.Party("/api/property-sales")
	{
		propertySales.Post("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertySale)
		propertySales.Get("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUserPropertySales)
		propertySales.Get("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetPropertySale)
		propertySales.Put("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdatePropertySale)
		propertySales.Post("/{id:uint}/submit", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SubmitPropertyForVerification)
		propertySales.Post("/{id:uint}/publish", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.PublishProperty)
		propertySales.Post("/{id:uint}/offers", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateOffer)
		propertySales.Get("/public", optionalAuthMiddleware, routes.GetPublishedProperties)
		propertySales.Get("/public/{id:uint}", routes.GetPublishedProperty) // Add public endpoint for individual property
		propertySales.Post("/{id:uint}/report", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ReportPublishedPropertySale)
		propertySales.Post("/{id:uint}/hide", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.HidePropertySale)
		propertySales.Get("/offers/organization", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationOffers)
		propertySales.Patch("/offers/{id:uint}/status", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateOfferStatus)
		propertySales.Get("/{id:uint}/offer-insights", routes.PublicOfferInsights)
	}

	landmarks := app.Party("/api/landmarks")
	{
		landmarks.Post("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateLandmark)
		landmarks.Get("/organization", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationLandmarks)
		landmarks.Get("/public", routes.GetPublicLandmarks)
		landmarks.Post("/{id:uint}/report", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ReportLandmark)
		landmarks.Patch("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateLandmark)
		landmarks.Delete("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeleteLandmark)
		landmarks.Post("/{id:uint}/submit", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SubmitLandmarkForVerification)
		landmarks.Patch("/{id:uint}/verify", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.VerifyLandmark)
		landmarks.Get("/pending", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetPendingLandmarks)
	}

	propertyTours := app.Party("/api/property-tours")
	{
		propertyTours.Post("/property/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.BookPropertyTour)
		propertyTours.Get("/my-bookings", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUserTourBookings)
		propertyTours.Get("/property/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetPropertyTourBookings)
		propertyTours.Patch("/{id:uint}/status", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateTourStatus)
		propertyTours.Get("/organization", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationTourBookings)
		propertyTours.Get("/agent", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetAgentTourBookings)
		propertyTours.Delete("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CancelTour)
	}

	// Admin routes for property selling system
	adminPropertySales := app.Party("/api/admin/property-sales", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
	{
		adminPropertySales.Get("/", routes.AdminGetPropertySales)
		adminPropertySales.Patch("/{id:uint}/verify", routes.AdminVerifyProperty)
		adminPropertySales.Post("/{id:uint}/publish", routes.AdminPublishProperty)
	}

	adminOrganizations := app.Party("/api/admin/organizations", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
	{
		adminOrganizations.Get("/", routes.AdminGetOrganizations)
		adminOrganizations.Patch("/{orgID:uint}/status", routes.AdminUpdateOrganizationStatus)
	}

	// Admin routes for landmark verification
	adminLandmarks := app.Party("/api/admin/landmarks", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminLandmarks.Get("/", routes.AdminGetAllLandmarks)
		adminLandmarks.Get("/pending", routes.GetPendingLandmarks)
		adminLandmarks.Patch("/{id:uint}/verify", routes.VerifyLandmark)
	}

	// Cities and Zones routes
	cities := app.Party("/api/cities")
	{
		cities.Get("/", routes.GetCities)
		cities.Get("/{cityId:uint}/zones", routes.GetZonesByCity)
	}

	// Admin Cities and Zones routes
	adminCities := app.Party("/api/admin/cities", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminCities.Get("/", routes.AdminGetAllCities)
		adminCities.Post("/", routes.AdminCreateCity)
		adminCities.Patch("/{id:uint}", routes.AdminUpdateCity)
		adminCities.Delete("/{id:uint}", routes.AdminDeleteCity)
	}

	adminZones := app.Party("/api/admin/zones", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminZones.Get("/", routes.AdminGetAllZones)
		adminZones.Post("/", routes.AdminCreateZone)
		adminZones.Patch("/{id:uint}", routes.AdminUpdateZone)
		adminZones.Delete("/{id:uint}", routes.AdminDeleteZone)
	}

	app.Post("/api/refresh", refreshTokenVerifierMiddleware, utils.RefreshToken)

	// Properties within polygon (simple point-in-polygon)
	app.Post("/api/properties/search-polygon", func(ctx iris.Context) {
		var req struct {
			Polygon []struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"polygon"`
		}
		if err := ctx.ReadJSON(&req); err != nil || len(req.Polygon) < 3 {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid polygon"})
			return
		}

		// Ray casting algorithm to check if point is inside polygon
		inside := func(lat, lng float64) bool {
			n := len(req.Polygon)
			inside := false
			for i, j := 0, n-1; i < n; j, i = i, i+1 {
				xi, yi := req.Polygon[i].Longitude, req.Polygon[i].Latitude
				xj, yj := req.Polygon[j].Longitude, req.Polygon[j].Latitude
				intersect := ((yi > lat) != (yj > lat)) && (lng < (xj-xi)*(lat-yi)/(yj-yi)+xi)
				if intersect {
					inside = !inside
				}
			}
			return inside
		}

		// Fetch all properties from database
		var properties []models.Property
		if err := storage.DB.Preload("Host").Find(&properties).Error; err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
			return
		}

		// Filter properties within polygon
		var filteredProperties []models.Property
		for _, prop := range properties {
			if inside(float64(prop.Lat), float64(prop.Lng)) {
				filteredProperties = append(filteredProperties, prop)
			}
		}

		ctx.JSON(iris.Map{"properties": filteredProperties})
	})

	// Translation service endpoint
	app.Post("/api/translate", func(ctx iris.Context) {
		var req struct {
			Text string `json:"text"`
			From string `json:"from"` // optional; use "auto" if empty
			To   string `json:"to"`
		}
		if err := ctx.ReadJSON(&req); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid JSON"})
			return
		}
		if req.Text == "" || req.To == "" {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "'text' and 'to' are required"})
			return
		}

		source := req.From
		if source == "" {
			source = "auto"
		}

		// Try multiple translation services for better reliability
		translationServices := []string{
			"https://libretranslate.de/translate",           // German instance
			"https://translate.argosopentech.com/translate", // Argos OpenTech
			"https://libretranslate.com/translate",          // Original instance
		}

		var translated string
		var detected string

		for _, ltURL := range translationServices {
			payload := map[string]string{
				"q":      req.Text,
				"source": source,
				"target": req.To,
			}
			body, _ := json.Marshal(payload)
			httpReq, err := http.NewRequest("POST", ltURL, bytes.NewBuffer(body))
			if err != nil {
				continue
			}
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(httpReq)
			if err != nil {
				continue
			}

			var ltResp struct {
				TranslatedText   string `json:"translatedText"`
				DetectedLanguage string `json:"detectedLanguage"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&ltResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			if ltResp.TranslatedText != "" && ltResp.TranslatedText != req.Text {
				translated = ltResp.TranslatedText
				detected = ltResp.DetectedLanguage
				break
			}
		}

		// Fallback if all services failed or returned original text
		if translated == "" {
			translated = req.Text
			detected = "unknown"
		}

		ctx.JSON(iris.Map{
			"original":   req.Text,
			"translated": translated,
			"from":       source,
			"to":         req.To,
			"detected":   detected,
		})
	})

	// Get port from environment; in Render, PORT must be provided. Default only in local dev.
	port := os.Getenv("PORT")
	if port == "" {
		if os.Getenv("RENDER") != "" {
			log.Fatalf("❌ PORT environment variable not set by platform; cannot start web service")
		}
		port = "4000"
		fmt.Println("⚠️  PORT environment variable not set, defaulting to 4000 (local dev)")
	}
	addr := "0.0.0.0:" + port // Explicitly bind to all interfaces for Render

	fmt.Printf("🚀 Server starting on %s\n", addr)
	fmt.Printf("🌐 Health check available at: http://%s/health\n", addr)
	fmt.Printf("📡 API endpoints available at: http://%s/api/\n", addr)

	// Start server and bind to the provided PORT (Render requires binding to $PORT)
	fmt.Println("🎯 Attempting to start server with app.Listen()...")
	if err := app.Listen(addr); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}
