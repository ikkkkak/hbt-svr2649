package main

import (
	meskenygpt "apartments-clone-server/MeskenyGPT/ai"
	"apartments-clone-server/models"
	"apartments-clone-server/places"
	"apartments-clone-server/realtime"
	"apartments-clone-server/routes"
	"apartments-clone-server/services"
	"apartments-clone-server/services/listing_ai"
	"apartments-clone-server/services/meskenyguide"
	"apartments-clone-server/services/videoprocessing"
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
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/kataras/iris/v12"
	irisjwt "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
)

// Optional authentication middleware - allows requests with or without JWT tokens.
// On expired access token: sets X-Token-Expired: true (client should call POST /api/auth/refresh) and continues as unauthenticated; no log spam.
func optionalAuthMiddleware(ctx iris.Context) {
	authHeader := ctx.GetHeader("Authorization")
	if token := strings.TrimSpace(authHeader); strings.HasPrefix(token, "Bearer ") {
		rawToken := strings.TrimPrefix(token, "Bearer ")
		if rawToken != "" {
			parsed := utils.ParseAccessToken(rawToken)
			if parsed.Claims != nil {
				ctx.Values().Set("userID", parsed.Claims.ID)
				ctx.Values().Set("jwt.claims", parsed.Claims)
			} else if parsed.Expired {
				ctx.Header("X-Token-Expired", "true")
			}
		}
	}
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

	// Only load .env in development (Cloud Run / Docker do not have .env — set env vars in Cloud Run console)
	if os.Getenv("RENDER") == "" {
		fmt.Println("🔍 Debug: Loading .env file...")
		godotenv.Load()
		fmt.Println("📁 Loaded .env file")
	} else {
		fmt.Println("🌐 Running on Render (production)")
	}

	// Required for JWT: login and auth will return 401 if these are missing (e.g. on Cloud Run without env vars)
	if s := strings.TrimSpace(os.Getenv("ACCESS_TOKEN_SECRET")); s == "" {
		log.Fatalf("❌ ACCESS_TOKEN_SECRET is required. Set it in Cloud Run: Service → Edit → Variables → ACCESS_TOKEN_SECRET (use the same value as local .env)")
	}
	if s := strings.TrimSpace(os.Getenv("REFRESH_TOKEN_SECRET")); s == "" {
		log.Fatalf("❌ REFRESH_TOKEN_SECRET is required. Set it in Cloud Run: Service → Edit → Variables → REFRESH_TOKEN_SECRET (use the same value as local .env)")
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
		videoprocessing.SeedDefaultMusicTracks(storage.DB)
		
		// Start property cleanup scheduler (runs daily at 2 AM)
		cleanupService := services.NewPropertyCleanupService()
		cleanupService.StartCleanupScheduler()
		fmt.Println("✅ Database initialized successfully")
		// Auto-migrate chat, AI, and moderation tables (idempotent)
		if err := storage.DB.AutoMigrate(
			&models.ChatMessage{},
			&models.HiddenProperty{}, &models.PropertyReport{}, &models.UserFlag{}, &models.HiddenVideo{}, &models.PropertySaleReport{}, &models.LandmarkReport{}, &models.Landmark{}, &models.HiddenPropertySale{}, &models.PropertySaleVideo{}, &models.PropertySaleVideoLike{}, &models.PropertySaleVideoSave{}, &models.PropertySaleVideoComment{}, &models.PropertySaleVideoReport{}, &models.HiddenPropertySaleVideo{}, &models.UserBlockedOrganization{},
			&models.Country{}, &models.City{}, &models.Zone{}, &models.Quartier{},
			&models.AIChatSession{}, &models.AIChatMessage{},
			&models.DeviceRegistration{}, &models.DeviceSession{},
			&models.UserBehavior{}, // User behavior tracking for intelligent notifications
			&models.AnonymousUserPreference{}, // Anonymous user preferences for intelligent notifications
			&models.PropertyDNA{}, &models.AIEnrichedUser{}, &models.PropertyMatch{}, &models.UserBehaviorSummary{}, // Host suggestion engine
			&models.PropertyFeedSeen{}, // Smart property feed seen-history
			&models.CrashLog{}, // Crash logs for error tracking
			&models.DiscoveryEngagementLog{}, // Discovery spotlight engagement logs
			&models.GoldPropertyStat{}, // Gold property metrics
			&models.Interaction{}, &models.RecommendationCache{}, &models.NotificationEvent{}, &models.NotificationDeliveryLog{}, // Recommendation & notification system
			&models.NotificationCandidate{}, &models.UserNotificationQuota{}, &models.UserNotificationLearned{},
			&models.AIInteraction{}, &models.AIFeedback{}, &models.MarketSnapshot{}, &models.PropertyFeedSeen{},
			&models.ListingAIUsageEvent{},
			&models.GuideComment{}, &models.GuideNotification{}, &models.GuideHostPreference{},
			&models.HabitatPlan{}, &models.HabitatSector{}, &models.HabitatPlot{},
		); err != nil {
			fmt.Printf("❌ Failed to migrate moderation tables: %v\n", err)
		} else {
			fmt.Println("✅ Tables migrated (chat_messages, hidden_properties, property_reports, user_flags, hidden_videos, property_sale_reports, landmark_reports, property_sale_videos, user_blocked_organizations, ai_chat_sessions, ai_chat_messages, device_registrations, device_sessions)")
		}
	}()
	fmt.Println("🔧 Initializing media CDN...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ Panic during media CDN initialization: %v\n", r)
				fmt.Println("⚠️  Continuing without media CDN...")
			}
		}()
		storage.InitializeMediaCDN()
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

	// Start background push workers
	pushsvc.StartPushWorker()
	pushsvc.StartMarketingReminderWorker()
	services.StartHostModeNotificationScheduler()
	services.StartSmartNotificationScheduler()
	meskenyguide.StartScheduler()
	services.StartAINotificationQueue()
	services.StartNotificationOrchestratorWorkers()

	fmt.Println("🔧 Initializing WebSocket Hub...")
	websocketHub.InitHub()
	// Unified user-based realtime hub (direct messages + inbox updates)
	realtime.StartUserHubRedisSubscriber()
	videoprocessing.StartWorkers(storage.DB)
	videoprocessing.StartSlideshowWorkers(storage.DB)
	go func() {
		time.Sleep(2 * time.Second)
		videoprocessing.BackfillSlideshowVideosOnStart(storage.DB)
	}()
	if os.Getenv("VIDEO_BACKFILL_ENABLED") == "true" {
		go videoprocessing.BackfillPendingPropertySaleVideos(storage.DB, 50)
	}
	// Daily refresh token cleanup (keeps refresh_tokens table lean)
	go func() {
		// Run once on boot, then every 24h
		utils.PurgeOldRefreshTokens()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			utils.PurgeOldRefreshTokens()
		}
	}()
	fmt.Println("✅ WebSocket Hub initialized successfully")

	// Initialize MeskenyGPT AI service (shared AI infrastructure)
	fmt.Println("🔧 Initializing MeskenyGPT AI service...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("❌ Panic during MeskenyGPT initialization: %v\n", r)
			}
		}()
		aiCfg := meskenygpt.DefaultConfigFromEnv()
		svc := meskenygpt.NewService(aiCfg, storage.DB, storage.Redis)
		routes.MeskenyGPTService = svc
		services.MeskenyGPTService = svc
		fmt.Println("✅ MeskenyGPT AI service initialized successfully and wired to /api/ai/chat")
	}()

	// Listing AI worker (Add with AI — rent / sale / land drafts)
	fmt.Println("🔧 Initializing Listing AI worker...")
	listingAIGen := listing_ai.NewGenerator(services.NewAIService())
	listingAIWorker := listing_ai.NewWorker(listingAIGen)
	listing_ai.DefaultWorker = listingAIWorker
	routes.ListingAIWorker = listingAIWorker
	go func() {
		if _, err := listing_ai.GetLocationCatalog(); err != nil {
			fmt.Printf("⚠️ Listing AI catalog warm-up: %v\n", err)
		}
	}()
	fmt.Println("✅ Listing AI worker ready (/api/listing-ai/jobs)")

	// Nearby places (Google Places API) for property sales — restaurants, hospitals, schools
	places.DefaultService = places.NewService(os.Getenv("GOOGLE_PLACES_API_KEY"))
	if places.DefaultService != nil && os.Getenv("GOOGLE_PLACES_API_KEY") != "" {
		fmt.Println("✅ Nearby places service initialized (GOOGLE_PLACES_API_KEY set)")
	}

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

	// Error / very-slow-only logging — sub-300ms polling was flooding I/O and worsening stalls.
	app.Use(func(ctx iris.Context) {
		if ctx.Method() == iris.MethodOptions {
			ctx.Next()
			return
		}
		routes.IncActiveRequest()
		defer routes.DecActiveRequest()

		reqID := ctx.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		ctx.Values().Set("request_id", reqID)
		ctx.Header("X-Request-ID", reqID)

		start := time.Now()
		path := ctx.Path()
		method := ctx.Method()
		clientSource := ctx.GetHeader("X-Client-Source")
		ctx.Next()
		ms := time.Since(start).Milliseconds()
		status := ctx.GetStatusCode()
		if status >= 500 || (status >= 400 && ms >= 2000) || ms >= 8000 {
			tag := ""
			if ms >= 8000 {
				tag = " (slow)"
			}
			src := ""
			if clientSource != "" {
				src = fmt.Sprintf(" src=%s", clientSource)
			}
			log.Printf("← %s %s %dms status=%d req=%s%s%s", method, path, ms, status, reqID, src, tag)
		}
	})

	// Short CDN/client cache for public feed GETs — safe on weak networks (stale-while-revalidate).
	app.Use(func(ctx iris.Context) {
		ctx.Next()
		if ctx.Method() != iris.MethodGet || ctx.GetStatusCode() != iris.StatusOK {
			return
		}
		if ctx.GetHeader("Cache-Control") != "" {
			return
		}
		path := ctx.Path()
		if strings.Contains(path, "/feed") || strings.Contains(path, "/published") ||
			strings.HasSuffix(path, "/properties") {
			ctx.Header("Cache-Control", "public, max-age=30, stale-while-revalidate=120")
		}
	})

	// Minimal middleware - compression only
	fmt.Println("🔧 Setting up middleware...")
	app.Use(iris.Compression)

	// JWT Verifiers
	resetTokenVerifier := irisjwt.NewVerifier(irisjwt.HS256, []byte(os.Getenv("EMAIL_TOKEN_SECRET")))
	resetTokenVerifier.WithDefaultBlocklist()
	resetTokenVerifierMiddleware := resetTokenVerifier.Verify(func() interface{} {
		return new(utils.ForgotPasswordToken)
	})

	utils.InitAccessJWTVerifier()

	// Fixed JWT middleware - verify token and set userID in context (shared verifier, not per-request).
	accessTokenVerifierMiddleware := func(ctx iris.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if token := strings.TrimSpace(authHeader); strings.HasPrefix(token, "Bearer ") {
			rawToken := strings.TrimPrefix(token, "Bearer ")
			if rawToken != "" {
				parsed := utils.ParseAccessToken(rawToken)
				if parsed.Claims != nil {
					ctx.Values().Set("userID", parsed.Claims.ID)
					ctx.Values().Set("userRole", parsed.Claims.Role)
					ctx.Values().Set("jwt.claims", parsed.Claims)
					ctx.Next()
					return
				}
				if parsed.Expired {
					ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{
						"error": "access token expired",
						"code":  "TOKEN_EXPIRED",
					})
					return
				}
			}
		}
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "unauthorized", "code": "NO_TOKEN"})
	}

	// Auth routes — Zero Re-Login: refresh accepts opaque or legacy JWT, no middleware
	auth := app.Party("/api/auth")
	{
		auth.Post("/refresh", utils.RefreshToken)
		auth.Post("/logout", utils.RevokeRefreshToken)
		auth.Post("/logout-all", accessTokenVerifierMiddleware, utils.RevokeAllRefreshTokens)
		auth.Get("/sessions", accessTokenVerifierMiddleware, utils.ListRefreshTokenSessions)
	}

	// OAuth 2.0 naming alias (RFC 6749 token endpoint)
	tokenParty := app.Party("/api/token")
	{
		tokenParty.Post("/refresh", utils.RefreshToken)
	}

	// Health — liveness vs readiness vs deep diagnostics
	fmt.Println("🔧 Setting up health check endpoints...")
	app.Get("/health", routes.HealthLive)
	app.Get("/healthz", routes.HealthLive)
	app.Get("/api/health", routes.HealthLive)
	app.Get("/api/health/ready", routes.HealthReady)
	app.Get("/api/health/deep", routes.HealthDeep)

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
		// User behavior tracking routes
		user.Post("/behavior/track", optionalAuthMiddleware, routes.TrackUserBehavior)
		user.Post("/favorite-city", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetFavoriteCity)
		user.Get("/favorite-city", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetFavoriteCity)
		user.Get("/notification-orchestrator", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetMyOrchestratorNotifications)
		user.Get("/host-share-consent", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostShareConsent)
		user.Put("/host-share-consent", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.PutHostShareConsent)
	}
	// Debug endpoint to view behavior stats (public for debugging)
	app.Get("/api/user/behavior/stats", routes.GetBehaviorStats)

	// Notification orchestrator (AI delivery) — ingest via X-Internal-Key; feedback via Bearer
	app.Post("/api/orchestrator/candidates", routes.PostOrchestratorCandidatesInternal)
	app.Post("/api/orchestrator/feedback", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.PostOrchestratorFeedback)

	// Append-only interaction tracking for recommendation engine (optional auth; deviceId for anonymous)
	app.Post("/api/interactions/track", optionalAuthMiddleware, routes.TrackInteraction)

	// Recommendation feed and suggested content (optional auth; device_id for anonymous)
	rec := app.Party("/api/recommendations", optionalAuthMiddleware)
	{
		rec.Get("/feed", routes.GetRecommendedFeed)
		rec.Get("/suggested-properties", routes.GetSuggestedProperties)
		rec.Get("/suggested-videos", routes.GetSuggestedVideosForProperty)
	}
	{	
		user.Post("/check-exists", routes.CheckUserExists)
		user.Post("/register", routes.Register)
		user.Post("/login", routes.Login)
		user.Post("/register-phone", routes.RegisterPhone)
		user.Post("/login-phone", routes.LoginPhone)
		user.Post("/facebook", routes.FacebookLoginOrSignUp)
		user.Post("/facebook/code", routes.FacebookLoginWithCode)
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
		user.Get("/broker-verification", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetBrokerVerificationStatus)
		user.Post("/broker-verification", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SubmitBrokerVerification)
		user.Patch("/broker-verification/settings", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateBrokerVerificationSettings)
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
		// Property Sale Wishlist
		user.Get("/wishlist/property-sales", accessTokenVerifierMiddleware, routes.GetUserPropertySaleWishlist)
		user.Post("/wishlist/property-sales", accessTokenVerifierMiddleware, routes.AddPropertySaleToWishlist)
		user.Delete("/wishlist/property-sales/{propertySaleID:uint}", accessTokenVerifierMiddleware, routes.RemovePropertySaleFromWishlist)
		user.Get("/wishlist/landmarks", accessTokenVerifierMiddleware, routes.GetUserLandmarkWishlist)
		user.Delete("/wishlist/{propertyID:uint}", accessTokenVerifierMiddleware, routes.RemoveFromUserWishlist)

		// Host Mode Tracking routes
		user.Post("/host-mode/switch", accessTokenVerifierMiddleware, routes.RecordHostModeSwitch)
		user.Post("/host-mode/interaction", accessTokenVerifierMiddleware, routes.RecordHostModeInteraction)

		// User moderation routes
		user.Get("/blocked", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetBlockedUsers)
		user.Get("/hidden-properties", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHiddenProperties)
		user.Get("/hidden-videos", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHiddenVideos)
	}

	// Stories routes under /api with auth on protected endpoints (verifier then attacher)
	// Inbox endpoint uses optional auth to show read status
	storiesParty := app.Party("/api/stories")
	{
		// Protected routes
		storiesParty.Post("/upload", accessTokenVerifierMiddleware, routes.UploadStory)
		storiesParty.Post("/{storyId:uint}/view", accessTokenVerifierMiddleware, routes.PostStoryView)
		storiesParty.Post("/{storyId:uint}/like", accessTokenVerifierMiddleware, routes.PostStoryLikeToggle)
		storiesParty.Delete("/{storyId:uint}", accessTokenVerifierMiddleware, routes.DeleteStory)

		// Public routes (inbox uses optional auth for read status)
		storiesParty.Get("/inbox", optionalAuthMiddleware, routes.GetStoriesInbox)
		storiesParty.Get("/{userId:uint}", routes.GetUserStories)
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
		property.Get("/host-properties/{id}", optionalAuthMiddleware, routes.GetHostPropertiesByPropertyID)
		property.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteProperty)
		property.Patch("/update/{id}", accessTokenVerifierMiddleware, routes.UpdateProperty)
		property.Post("/search", optionalAuthMiddleware, routes.GetPropertiesByBoundingBox)
		property.Delete("/image", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeletePropertyImage)
	}

	// Image upload endpoint (used by older clients; wraps the same GCS pipeline as /api/upload/image)
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

		urlMap := storage.UploadBase64ImageOptimized(req.Image, req.PublicID)
		if urlMap == nil || urlMap["url"] == "" {
			msg := "Failed to upload image"
			if urlMap != nil && urlMap["error"] != "" {
				msg = urlMap["error"]
			}
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.JSON(iris.Map{"error": msg})
			return
		}

		ctx.JSON(iris.Map{"url": urlMap["url"]})
	})

	// Admin routes
	admin := app.Party("/api/admin", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		admin.Get("/orchestrator/stats", routes.AdminOrchestratorStats)
		admin.Get("/orchestrator/users/{userID:uint}/log", routes.AdminOrchestratorUserLog)
		admin.Get("/users", routes.AdminListUsers)
		admin.Patch("/users/{id:uint}/role", utils.SuperAdminOnlyMiddleware, routes.AdminChangeUserRole)
		admin.Patch("/users/{id:uint}", routes.AdminUpdateUser)
		admin.Get("/users/{id:uint}", routes.AdminGetUser)
		admin.Post("/users/{id:uint}/verify", routes.AdminVerifyUser)
		admin.Post("/users/{id:uint}/broker-verification", routes.AdminReviewBrokerVerification)
		admin.Get("/broker-verifications/pending", routes.AdminListPendingBrokerVerifications)
		admin.Get("/identity-verifications", routes.AdminListIdentityVerifications)
		admin.Get("/identity-verifications/{user_id:uint}", routes.AdminGetIdentityVerificationUser)
		admin.Get("/properties", routes.AdminListProperties)
		admin.Get("/properties/{id:uint}", routes.AdminGetProperty)
		admin.Patch("/properties/{id:uint}/status", routes.AdminUpdatePropertyStatus)
		admin.Post("/properties/{id:uint}/reassign-locations", routes.AdminReassignPropertyLocations)
		admin.Post("/properties/{id:uint}/flag", routes.AdminFlagProperty)
		admin.Delete("/properties/{id:uint}", routes.AdminDeleteProperty)
		admin.Get("/music-tracks", routes.AdminListMusicTracks)
		admin.Post("/music-tracks", routes.AdminCreateMusicTrack)
		admin.Post("/music-tracks/upload", routes.AdminUploadMusicFile)
		admin.Patch("/music-tracks/{id:uint}", routes.AdminUpdateMusicTrack)
		admin.Delete("/music-tracks/{id:uint}", routes.AdminDeleteMusicTrack)
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
		admin.Post("/email/test", routes.AdminSendTestListingEmail)
		admin.Get("/stats", routes.AdminStats)
		admin.Get("/moderation/pending", routes.AdminModerationPending)
		admin.Get("/activity", routes.AdminActivity)
		admin.Get("/notifications/new-homes", routes.AdminNewHomesNotificationStats)
		admin.Get("/notifications/new-homes/devices", routes.AdminNewHomesNotificationDeviceTiming)
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
	app.Get("/api/bootstrap", optionalAuthMiddleware, routes.GetAppBootstrap)
	app.Post("/api/batch/property-sales", optionalAuthMiddleware, routes.BatchGetPropertySales)
	app.Post("/api/sync/mutations", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.PostSyncMutations)

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
	// Upload routes (CDN from MEDIA_CDN)
	upload := app.Party("/api/upload")
	{
		upload.Post("/image", accessTokenVerifierMiddleware, routes.UploadImage)
		upload.Post("/image/binary", accessTokenVerifierMiddleware, routes.UploadImageBinary)
		// Chunked video upload (register before /video — specific paths first)
		upload.Post("/video/init", accessTokenVerifierMiddleware, routes.InitChunkUpload)
		upload.Get("/video/{uploadId}/status", accessTokenVerifierMiddleware, routes.GetChunkUploadStatus)
		upload.Put("/video/{uploadId}/chunk", accessTokenVerifierMiddleware, routes.UploadVideoChunk)
		upload.Post("/video/{uploadId}/complete", accessTokenVerifierMiddleware, routes.CompleteChunkUpload)
		upload.Post("/video/stream", accessTokenVerifierMiddleware, routes.UploadVideoStream)
		// Legacy base64 video upload (small files / admin only; large bodies rejected)
		upload.Post("/video", accessTokenVerifierMiddleware, routes.UploadVideo)
	}
	{
		notifications.Post("/marketing/device", routes.RegisterMarketingDevice)
		notifications.Put("/marketing/device/link", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.LinkMarketingDeviceToUser)
		notifications.Post("/test-push", routes.SendTestNotification)
		notifications.Post("/test-detailed/{userID:int}", routes.SendDetailedTestNotification)
		notifications.Post("/welcome", routes.SendWelcomeNotification)
		notifications.Get("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ListUserNotifications)
		notifications.Patch("/{id:uint}/read", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkUserNotificationRead)
		notifications.Patch("/read-all", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkAllUserNotificationsRead)
		notifications.Get("/settings", accessTokenVerifierMiddleware, routes.GetUserNotificationSettings)
		notifications.Put("/settings", accessTokenVerifierMiddleware, routes.UpdateUserNotificationSettings)
		// Add the missing notification endpoints
		notifications.Post("/register", func(ctx iris.Context) {
			var req struct {
				UserID      *uint  `json:"user_id"` // Nullable for anonymous users
					DeviceID    string `json:"device_id"` // Device identifier for anonymous users
				PushToken   string `json:"push_token"`
				Language    string `json:"language"`
				Location    string `json:"location"`
				// Optional: IANA timezone + quiet hours + smart push daily cap (used by smart notification gates).
				Timezone       string `json:"timezone"`
				QuietStartHour *int   `json:"quiet_start_hour"`
				QuietEndHour   *int   `json:"quiet_end_hour"`
				MaxSmartPerDay *int   `json:"max_smart_per_day"`
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
			fmt.Printf("📝 Received registration request: user_id=%v, device_id=%s, language=%s, location=%s, push_token=%s\n",
				req.UserID, func() string {
					if req.DeviceID != "" && len(req.DeviceID) > 10 {
						return req.DeviceID[:10] + "..."
					}
					return req.DeviceID
				}(), req.Language, req.Location, req.PushToken[:20]+"...")

			// Create or update notification preference in database
			now := time.Now()
			pref := models.NotificationPreference{
				UserID:         req.UserID,
				DeviceID:       req.DeviceID,
				PushToken:      req.PushToken,
				Language:       req.Language,
				Location:       req.Location,
				Latitude:       req.Coordinates.Latitude,
				Longitude:      req.Coordinates.Longitude,
				Enabled:        true,
				LastActive:     &now,
				Timezone:       strings.TrimSpace(req.Timezone),
				QuietStartHour: 22,
				QuietEndHour:   7,
				MaxSmartPerDay: 2,
			}
			if req.QuietStartHour != nil && *req.QuietStartHour >= 0 && *req.QuietStartHour <= 23 {
				pref.QuietStartHour = *req.QuietStartHour
			}
			if req.QuietEndHour != nil && *req.QuietEndHour >= 0 && *req.QuietEndHour <= 23 {
				pref.QuietEndHour = *req.QuietEndHour
			}
			if req.MaxSmartPerDay != nil && *req.MaxSmartPerDay >= 1 && *req.MaxSmartPerDay <= 20 {
				pref.MaxSmartPerDay = *req.MaxSmartPerDay
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
				if strings.TrimSpace(req.Timezone) != "" {
					existingPref.Timezone = strings.TrimSpace(req.Timezone)
				}
				if req.QuietStartHour != nil && *req.QuietStartHour >= 0 && *req.QuietStartHour <= 23 {
					existingPref.QuietStartHour = *req.QuietStartHour
				}
				if req.QuietEndHour != nil && *req.QuietEndHour >= 0 && *req.QuietEndHour <= 23 {
					existingPref.QuietEndHour = *req.QuietEndHour
				}
				if req.MaxSmartPerDay != nil && *req.MaxSmartPerDay >= 1 && *req.MaxSmartPerDay <= 20 {
					existingPref.MaxSmartPerDay = *req.MaxSmartPerDay
				}
				if req.DeviceID != "" {
					existingPref.DeviceID = req.DeviceID // Update device ID
				}

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

	// Video utilities
	videos := app.Party("/api/videos")
	{
		videos.Get("/watermark", routes.GetWatermarkedVideo)
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
		video.Post("/", accessTokenVerifierMiddleware, routes.CreateVideo)
		video.Get("/{id:uint}/streaming", accessTokenVerifierMiddleware, routes.GetVideoStreamingStatus)
		video.Get("/{id:uint}/streaming/events", accessTokenVerifierMiddleware, routes.VideoProcessingSSE)
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
		video.Post("/{videoID:uint}/view", optionalAuthMiddleware, routes.RecordVideoView)
		video.Get("/{videoID:uint}/viewers", accessTokenVerifierMiddleware, routes.GetVideoViewers)
		video.Get("/unseen", optionalAuthMiddleware, routes.GetUnseenVideos)
		video.Post("/mark-all-viewed", optionalAuthMiddleware, routes.MarkAllVideosAsViewed)
		video.Get("/count", routes.GetTotalVideoCount)
	}

	// Property Sale Videos routes
	propertySaleVideos := app.Party("/api/property-sale-videos")
	{
		propertySaleVideos.Post("/", accessTokenVerifierMiddleware, routes.CreatePropertySaleVideo)
		propertySaleVideos.Get("/feed", optionalAuthMiddleware, routes.GetPropertySaleVideoFeed)
		propertySaleVideos.Post("/{id:uint}/view", optionalAuthMiddleware, routes.RecordPropertySaleVideoView)
		propertySaleVideos.Post("/{id:uint}/like", accessTokenVerifierMiddleware, routes.LikePropertySaleVideo)
		propertySaleVideos.Post("/{id:uint}/unlike", accessTokenVerifierMiddleware, routes.UnlikePropertySaleVideo)
		propertySaleVideos.Post("/{id:uint}/save", accessTokenVerifierMiddleware, routes.SavePropertySaleVideo)
		propertySaleVideos.Post("/{id:uint}/unsave", accessTokenVerifierMiddleware, routes.UnsavePropertySaleVideo)
		propertySaleVideos.Get("/{id:uint}/comments", optionalAuthMiddleware, routes.GetPropertySaleVideoComments)
		propertySaleVideos.Post("/{id:uint}/comments", accessTokenVerifierMiddleware, routes.CreatePropertySaleVideoComment)
		propertySaleVideos.Put("/comments/{id:uint}", accessTokenVerifierMiddleware, routes.UpdatePropertySaleVideoComment)
		propertySaleVideos.Delete("/comments/{id:uint}", accessTokenVerifierMiddleware, routes.DeletePropertySaleVideoComment)
		propertySaleVideos.Post("/comments/{id:uint}/like", accessTokenVerifierMiddleware, routes.LikePropertySaleVideoComment)
		propertySaleVideos.Post("/comments/{id:uint}/unlike", accessTokenVerifierMiddleware, routes.UnlikePropertySaleVideoComment)
		propertySaleVideos.Get("/admin/stats", accessTokenVerifierMiddleware, routes.GetPropertySaleVideosByOrganizationOrHost)
		propertySaleVideos.Get("/{id:uint}/streaming", accessTokenVerifierMiddleware, routes.GetPropertySaleVideoStreamingStatus)
		propertySaleVideos.Get("/{id:uint}/streaming/events", accessTokenVerifierMiddleware, routes.PropertySaleVideoProcessingSSE)
	}

	// Device Registration routes (public endpoint for silent registration)
	app.Post("/api/device/register", routes.RegisterDevice)
	app.Post("/api/device/session/start", routes.StartDeviceSession)
	app.Post("/api/device/session/end", routes.EndDeviceSession)
	app.Get("/api/device/preferences", routes.GetDevicePreferences)
	app.Put("/api/device/preferences", routes.UpsertDevicePreferences)
	app.Get("/api/device/analytics", accessTokenVerifierMiddleware, routes.GetDeviceAnalytics)
	app.Get("/api/device/daily-usage", accessTokenVerifierMiddleware, routes.GetDeviceDailyUsage)
	app.Get("/api/device/daily-usage", accessTokenVerifierMiddleware, routes.GetDeviceDailyUsage)

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

	// Unified real-time messaging websocket (direct messages + inbox updates)
	// NOTE: Mobile baseURL is ".../api" so websocket path must be "/api/ws".
	app.Get("/api/ws", routes.MessagingWS)

	// AI Chat routes - Meskeny AI
	aiChat := app.Party("/api/ai")
	{
		// Chat + greeting can be used without being logged in (optional auth).
		// Sessions / history remain authenticated-only.
		aiChat.Post("/chat", optionalAuthMiddleware, routes.SendAIChatMessage)
		aiChat.Post("/agent/run", optionalAuthMiddleware, routes.SendAIAgentRun)
		aiChat.Post("/agent/filters", optionalAuthMiddleware, routes.UpdateAIAgentFilters)
		aiChat.Post("/feedback", optionalAuthMiddleware, routes.SendAIFeedback)
		aiChat.Get("/greeting", optionalAuthMiddleware, routes.GetAIGreeting)
		aiChat.Get("/sessions", accessTokenVerifierMiddleware, routes.GetAIChatSessions)
		aiChat.Get("/sessions/{sessionId:uint}", accessTokenVerifierMiddleware, routes.GetAIChatSession)
		aiChat.Post("/sessions", accessTokenVerifierMiddleware, routes.CreateAIChatSession)
		aiChat.Delete("/sessions/{sessionId:uint}", accessTokenVerifierMiddleware, routes.DeleteAIChatSession)
	}

	// Listing AI — async draft generation for Add with AI flows
	listingAI := app.Party("/api/listing-ai", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware)
	{
		listingAI.Post("/jobs", routes.PostListingAIJob)
		listingAI.Get("/jobs/{jobId}", routes.GetListingAIJob)
		listingAI.Post("/events", routes.PostListingAIEvent)
	}

	// Direct Messages - Clean Implementation
	directMessages := app.Party("/api/direct-messages")
	{
		directMessages.Post("/", accessTokenVerifierMiddleware, routes.SendDirectMessage)
		directMessages.Get("/", accessTokenVerifierMiddleware, routes.ListDirectMessageConversations)
		directMessages.Get("/{userID:uint}", accessTokenVerifierMiddleware, routes.GetDirectMessages)
		directMessages.Post("/with/{userID:uint}/read", accessTokenVerifierMiddleware, routes.MarkDirectMessageThreadRead)
		directMessages.Post("/{messageID:uint}/read", accessTokenVerifierMiddleware, routes.MarkDirectMessageRead)
		// Message reactions
		directMessages.Post("/{messageID:uint}/reactions", accessTokenVerifierMiddleware, routes.AddMessageReaction)
		directMessages.Delete("/{messageID:uint}/reactions", accessTokenVerifierMiddleware, routes.RemoveMessageReaction)
	}

	// User Blocking for Direct Messages - Clean Implementation
	userBlocks := app.Party("/api/user-blocks")
	{
		userBlocks.Post("/{userID:uint}", accessTokenVerifierMiddleware, routes.BlockUser)
		userBlocks.Delete("/{userID:uint}", accessTokenVerifierMiddleware, routes.UnblockUser)
		userBlocks.Get("/", accessTokenVerifierMiddleware, routes.GetBlockedUsers)
	}

	host := app.Party("/api/host")
	{
		host.Get("/studio", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostStudio)
		host.Get("/suggestions", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostSuggestions)
		host.Get("/suggestions/pending-count", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostSuggestionPendingCount)
		host.Post("/suggestions/{match_id:uint}/contact", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ContactHostSuggestion)
		host.Post("/suggestions/{match_id:uint}/dismiss", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DismissHostSuggestion)

		guide := host.Party("/guide")
		{
			guide.Get("/feed", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetGuideFeed)
			guide.Get("/listing-previews", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetGuideListingPreviews)
			guide.Get("/grouped", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetGuideGrouped)
			guide.Get("/unread-count", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetGuideUnreadCount)
			guide.Get("/listings/{propertySaleId:uint}/comments", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetListingGuideComments)
			guide.Get("/comments/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetGuideComment)
			guide.Post("/comments/{id:uint}/implement", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ImplementGuideComment)
			guide.Post("/comments/{id:uint}/dismiss", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DismissGuideComment)
			guide.Post("/comments/{id:uint}/reply", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ReplyGuideComment)
			guide.Post("/dev/trigger", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DevTriggerGuide)
		}
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
		properties.Get("/", optionalAuthMiddleware, routes.ListProperties) // GET /api/properties - paginated approved properties
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
		// Organization Member Management (RBAC)
		organization.Post("/invite-code", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GenerateOrganizationInviteCode)
		organization.Get("/invite-code/{code}", routes.ValidateOrganizationInviteCode) // Public endpoint for preview
		organization.Post("/join", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.JoinOrganization)
		organization.Post("/leave", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.LeaveOrganization)
		organization.Get("/members", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationMembers)
		organization.Patch("/members/{memberId:uint}/role", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateOrganizationMemberRole)
		organization.Delete("/members/{memberId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.RemoveOrganizationMember)
		organization.Get("/check-personal-content", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CheckUserCanCreatePersonalContent)
	}

	// ProfileSheet public endpoints — organizations
	profileSheet := app.Party("/api/organizations/{orgID:uint}")
	{
		profileSheet.Get("/profile-sheet", routes.GetOrganizationProfileSheet)
		profileSheet.Get("/properties-sheet", routes.GetOrganizationPropertiesForSheet)
		profileSheet.Get("/landmarks-sheet", routes.GetOrganizationLandmarksForSheet)
		profileSheet.Post("/follow", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ToggleOrganizationFollow)
	}

	// ProfileSheet public endpoints — individual users
	userProfileSheet := app.Party("/api/users/{userID:uint}")
	{
		userProfileSheet.Get("/profile-sheet", routes.GetUserProfileSheet)
		userProfileSheet.Get("/properties-sheet", routes.GetUserPropertiesForSheet)
		userProfileSheet.Get("/landmarks-sheet", routes.GetUserLandmarksForSheet)
	}

	// Property like endpoint
	app.Patch("/api/properties/{propertyID:uint}/like", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.TogglePropertyLike)

	propertySales := app.Party("/api/property-sales")
	propertySales.Use(func(ctx iris.Context) {
		if ctx.Method() == http.MethodPost || ctx.Method() == http.MethodGet {
			p := ctx.Path()
			if strings.Contains(p, "property-sales") &&
				(strings.Contains(p, "create-jobs") || strings.HasSuffix(p, "property-sales/") || p == "/api/property-sales") {
				log.Printf("📨 %s %s cl=%s", ctx.Method(), p, ctx.GetHeader("Content-Length"))
			}
		}
		ctx.Next()
	})
	{
		propertySales.Get("/create-jobs/ping", accessTokenVerifierMiddleware, func(ctx iris.Context) {
			ctx.StatusCode(iris.StatusNoContent)
		})
		propertySales.Post("/create-jobs", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.PostPropertySaleCreateJob)
		propertySales.Get("/create-jobs/{jobId}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetPropertySaleCreateJob)
		propertySales.Post("", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertySale)
		propertySales.Get("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUserPropertySales)
		propertySales.Get("/{id:uint}/gold-insights", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetPropertySaleGoldInsights)
		propertySales.Get("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetPropertySale)
		propertySales.Put("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdatePropertySale)
		propertySales.Post("/{id:uint}/submit", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SubmitPropertyForVerification)
		propertySales.Post("/{id:uint}/publish", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.PublishProperty)
		propertySales.Post("/{id:uint}/offers", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateOffer)
		propertySales.Get("/public", optionalAuthMiddleware, routes.GetPublishedProperties)
		propertySales.Get("/public/{id:uint}", routes.GetPublishedProperty) // Add public endpoint for individual property
		propertySales.Get("/{id:uint}/nearby", optionalAuthMiddleware, routes.GetPropertySaleNearby)
		propertySales.Post("/{id:uint}/report", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ReportPublishedPropertySale)
		propertySales.Post("/{id:uint}/hide", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.HidePropertySale)
		propertySales.Get("/offers/organization", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationOffers)
		propertySales.Patch("/offers/{id:uint}/status", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UpdateOfferStatus)
		propertySales.Get("/{id:uint}/offer-insights", routes.PublicOfferInsights)
		// Property Management Routes
		propertySales.Put("/{id:uint}/deactivate", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeactivatePropertySale)
		propertySales.Put("/{id:uint}/reactivate", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ReactivatePropertySale)
		propertySales.Delete("/{id:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeletePropertySale)
		propertySales.Put("/{id:uint}/sold", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkPropertySaleAsSold)
		propertySales.Put("/{id:uint}/unsold", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkPropertySaleAsUnsold)
		// Contact Host Route
		propertySales.Post("/contact-host", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ContactPropertySaleHost)
		propertySales.Get("/{id:uint}/offers", routes.GetPublicPropertyOffers)
	}

	propertyVideoJobs := app.Party("/api/property-video-jobs", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware)
	{
		propertyVideoJobs.Get("/by-listing", routes.GetPropertyVideoJobByListing)
		propertyVideoJobs.Get("/{id:uint}", routes.GetPropertyVideoGenerationJob)
	}

	landmarks := app.Party("/api/landmarks")
	{
		landmarks.Post("/", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateLandmark)
		landmarks.Get("/organization", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetOrganizationLandmarks)
		landmarks.Get("/public", routes.GetPublicLandmarks)
		landmarks.Get("/videos/feed", optionalAuthMiddleware, routes.GetLandmarkVideosFeed)
		landmarks.Get("/{id:uint}", optionalAuthMiddleware, routes.GetLandmarkByID)
		landmarks.Post("/{id:uint}/like", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.LikeLandmarkVideo)
		landmarks.Post("/{id:uint}/unlike", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UnlikeLandmarkVideo)
		landmarks.Post("/{id:uint}/save", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SaveLandmarkVideo)
		landmarks.Post("/{id:uint}/unsave", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.UnsaveLandmarkVideo)
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
		adminPropertySales.Post("/backfill-nearby", routes.AdminBackfillNearbyPlaces)
		adminPropertySales.Get("/{id:uint}", routes.AdminGetPropertySaleByID)
		adminPropertySales.Patch("/{id:uint}/organization", routes.AdminSetPropertySaleOrganization)
		adminPropertySales.Patch("/{id:uint}", routes.AdminUpdatePropertySale)
		adminPropertySales.Patch("/{id:uint}/verify", routes.AdminVerifyProperty)
		adminPropertySales.Post("/{id:uint}/publish", routes.AdminPublishProperty)
		adminPropertySales.Patch("/{id:uint}/sold", routes.AdminMarkPropertySaleAsSold)
		adminPropertySales.Patch("/{id:uint}/deactivate", routes.AdminDeactivatePropertySale)
		adminPropertySales.Patch("/{id:uint}/reactivate", routes.AdminReactivatePropertySale)
		adminPropertySales.Delete("/{id:uint}", routes.AdminDeletePropertySale)
	}

	adminOrganizations := app.Party("/api/admin/organizations", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
	{
		adminOrganizations.Get("/", routes.AdminGetOrganizations)
		adminOrganizations.Post("/", routes.AdminCreateOrganization)
		adminOrganizations.Patch("/{orgID:uint}/status", routes.AdminUpdateOrganizationStatus)
	}

	// Admin routes for landmark verification
	adminLandmarks := app.Party("/api/admin/landmarks", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminLandmarks.Get("/", routes.AdminGetAllLandmarks)
		adminLandmarks.Get("/pending", routes.GetPendingLandmarks)
		adminLandmarks.Get("/{id:uint}", routes.AdminGetLandmarkByID)
		adminLandmarks.Patch("/{id:uint}", routes.AdminUpdateLandmark)
		adminLandmarks.Patch("/{id:uint}/verify", routes.VerifyLandmark)
		adminLandmarks.Patch("/{id:uint}/coordinates", routes.AdminUpdateLandmarkCoordinates)
		adminLandmarks.Patch("/{id:uint}/organization", routes.AdminSetLandmarkOrganization)
		adminLandmarks.Delete("/{id:uint}", routes.AdminDeleteLandmark)
	}

	adminInsights := app.Party("/api/admin/insights", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
	{
		adminInsights.Get("/mobile-ai", routes.AdminGetMobileAIInsights)
		// Heavy: rebuilds user_behavior_summary (cron or manual). See README_HOST_SUGGESTIONS.md
		adminInsights.Post("/host-suggestions/refresh-behavior-summary", routes.AdminRefreshUserBehaviorSummary)
	}

	// Cities and Zones routes
	countries := app.Party("/api/countries")
	{
		countries.Get("/", routes.GetCountries)
		countries.Get("/{countryId:uint}/cities", routes.GetCitiesByCountry)
	}

	cities := app.Party("/api/cities")
	{
		cities.Get("/", routes.GetCities)
		cities.Get("/{cityId:uint}/zones", routes.GetZonesByCity)
		cities.Get("/zones/{zoneId:uint}/quartiers", routes.GetQuartiersByZone)
	}

	// Habitat cadastre (Plan → Sector → Plot) — public read APIs
	habitat := app.Party("/api/habitat")
	{
		habitat.Get("/plans", routes.GetHabitatPlans)
		habitat.Get("/plans/{planId:uint}/sectors", routes.GetHabitatSectorsByPlan)
		habitat.Get("/sectors/{sectorId:uint}/plots", routes.GetHabitatPlotsBySector)
		habitat.Get("/plots/bbox", routes.GetHabitatPlotsInBBox)
		habitat.Get("/plots/lookup", routes.LookupHabitatPlotForListing)
		habitat.Get("/plots/lookup_in_sector", routes.LookupHabitatPlotInSector)
		habitat.Get("/plots/{plotId:uint}/for_sale_landmark", routes.GetForSaleLandmarkByPlot)
		habitat.Get("/plots/{plotId:uint}", routes.GetHabitatPlot)
		habitat.Get("/search", routes.SearchHabitatPlots)
	}

	adminCountries := app.Party("/api/admin/countries", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminCountries.Get("/", routes.AdminGetAllCountries)
		adminCountries.Post("/", routes.AdminCreateCountry)
		adminCountries.Patch("/{id:uint}", routes.AdminUpdateCountry)
		adminCountries.Delete("/{id:uint}", routes.AdminDeleteCountry)
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

	adminQuartiers := app.Party("/api/admin/quartiers", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminQuartiers.Get("/", routes.AdminGetAllQuartiers)
		adminQuartiers.Post("/", routes.AdminCreateQuartier)
		adminQuartiers.Patch("/{id:uint}", routes.AdminUpdateQuartier)
		adminQuartiers.Delete("/{id:uint}", routes.AdminDeleteQuartier)
	}

	adminLocations := app.Party("/api/admin/locations", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminLocations.Get("/bulk/example", routes.AdminGetLocationBulkExample)
		adminLocations.Post("/bulk", routes.AdminBulkImportLocations)
	}

	adminHabitat := app.Party("/api/admin/habitat", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminHabitat.Get("/bulk/example", routes.AdminGetHabitatBulkExample)
		adminHabitat.Post("/bulk", routes.AdminHabitatBulkImport)
	}

	// Admin MeskenyGPT analytics (AI interactions + feedback) + structured knowledge (RAG)
	adminAI := app.Party("/api/admin/ai", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
	{
		adminAI.Get("/interactions", routes.AdminListAIInteractions)
		adminAI.Get("/listing-usage", routes.AdminGetListingAIUsageStats)
		adminAI.Get("/knowledge", routes.AdminListMeskenyKnowledge)
		adminAI.Get("/knowledge/{id:uint}", routes.AdminGetMeskenyKnowledge)
		adminAI.Post("/knowledge", routes.AdminCreateMeskenyKnowledge)
		adminAI.Patch("/knowledge/{id:uint}", routes.AdminUpdateMeskenyKnowledge)
		adminAI.Delete("/knowledge/{id:uint}", routes.AdminDeleteMeskenyKnowledge)
	}

	// Admin Property Types (categories) - add, edit, delete, seed
	adminCategories := app.Party("/api/admin/categories", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminCategories.Get("/", routes.AdminListCategories)
		adminCategories.Post("/", routes.AdminCreateCategory)
		adminCategories.Post("/seed-property", routes.AdminSeedPropertyCategories)
		adminCategories.Patch("/{id:int}", routes.AdminUpdateCategory)
		adminCategories.Delete("/{id:int}", routes.AdminDeleteCategory)
	}

	adminAmenities := app.Party("/api/admin/amenities", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminAmenities.Get("/", routes.AdminListAmenities)
		adminAmenities.Post("/", routes.AdminCreateAmenity)
		adminAmenities.Post("/seed", routes.AdminSeedAmenities)
		adminAmenities.Patch("/{id:int}", routes.AdminUpdateAmenity)
		adminAmenities.Delete("/{id:int}", routes.AdminDeleteAmenity)
	}

	// Crash Logs - Public endpoint (crashes can happen before login)
	app.Post("/api/crash-logs", routes.CreateCrashLog)

	// Admin Crash Logs - Requires admin authentication
	adminCrashLogs := app.Party("/api/admin/crash-logs", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminCrashLogs.Get("/", routes.GetCrashLogs)
		adminCrashLogs.Get("/stats", routes.GetCrashLogStats)
		adminCrashLogs.Get("/{id:uint}", routes.GetCrashLog)
		adminCrashLogs.Patch("/{id:uint}", routes.UpdateCrashLog)
	}

	// Banners - Public (for property sale feed)
	app.Get("/api/banners", routes.GetBanners)

	// Banners - Admin CRUD
	adminBanners := app.Party("/api/admin/banners", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware, utils.UserIDFromTokenMiddleware)
	{
		adminBanners.Get("/", routes.ListAdminBanners)
		adminBanners.Post("/", routes.CreateBanner)
		adminBanners.Patch("/{id:uint}", routes.UpdateBanner)
		adminBanners.Delete("/{id:uint}", routes.DeleteBanner)
	}

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

		// Fetch only public rent properties from database:
		// status in approved/live/published AND active=true.
		var properties []models.Property
		if err := storage.DB.
			Preload("Host").
			Where("LOWER(status) IN (?)", []string{"approved", "published"}).
			Where("COALESCE(is_active, ?) = ?", true, true).
			Where("COALESCE(is_flagged, false) = ?", false).
			Find(&properties).Error; err != nil {
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

	// Translation service endpoint (uses MarianMT)
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

		// Use MarianMT service
		translated, err := services.TranslateOnceDirect(req.Text, req.To)
		if err != nil {
			log.Printf("❌ Translation error: %v", err)
			// Fallback to original text on error
			translated = req.Text
		}

		// Detect source language (simple heuristic)
		detected := services.DetectSourceLanguageDirect(req.Text)

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
		// In container/cloud environments, PORT must be provided.
		port = "4000"
	}
	addr := "0.0.0.0:" + port // Explicitly bind to all interfaces

	fmt.Printf("🚀 Server starting on %s\n", addr)
	fmt.Printf("🌐 Health check available at: http://localhost:%s/health\n", port)
	fmt.Printf("📡 API endpoints available at: http://localhost:%s/api/\n", port)
	fmt.Println("📋 Listing flow: GET /health | POST /api/upload/image | POST /api/property-sales/create-jobs/ | POST /api/property-sales/")

	fmt.Println("🎯 Attempting to start server with app.Listen()...")
	if err := app.Listen(addr); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}
