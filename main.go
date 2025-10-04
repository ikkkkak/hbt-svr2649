// package main

// import (
// 	"apartments-clone-server/routes"
// 	"apartments-clone-server/storage"
// 	"apartments-clone-server/utils"
// 	"fmt"
// 	"log"
// 	"os"

// 	"github.com/go-playground/validator/v10"
// 	"github.com/joho/godotenv"
// 	"github.com/kataras/iris/v12"
// 	"github.com/kataras/iris/v12/middleware/jwt"
// )

// func main() {
// 	godotenv.Load()
// 	storage.InitializeDB()
// 	storage.InitializeS3()
// 	storage.InitializeRedis()

// 	app := iris.New()
// 	app.Validator = validator.New()

// 	// CORS for NovaDashboard (http://localhost:3000)
// 	app.AllowMethods(iris.MethodOptions)
// 	app.UseRouter(func(ctx iris.Context) {
// 		ctx.Header("Access-Control-Allow-Origin", ctx.GetHeader("Origin"))
// 		ctx.Header("Vary", "Origin")
// 		ctx.Header("Access-Control-Allow-Credentials", "true")
// 		ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
// 		ctx.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
// 		if ctx.Method() == iris.MethodOptions {
// 			ctx.StatusCode(iris.StatusNoContent)
// 			return
// 		}
// 		ctx.Next()
// 	})

// 	// Add only essential middleware, skip request logging
// 	app.Use(iris.Compression)

// 	resetTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("EMAIL_TOKEN_SECRET")))
// 	resetTokenVerifier.WithDefaultBlocklist()
// 	resetTokenVerifierMiddleware := resetTokenVerifier.Verify(func() interface{} {
// 		return new(utils.ForgotPasswordToken)
// 	})

// 	accessTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
// 	accessTokenVerifier.WithDefaultBlocklist()
// 	accessTokenVerifierMiddleware := accessTokenVerifier.Verify(func() interface{} {
// 		return new(utils.AccessToken)
// 	})

// 	refreshTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("REFRESH_TOKEN_SECRET")))
// 	refreshTokenVerifier.WithDefaultBlocklist()
// 	refreshTokenVerifierMiddleware := refreshTokenVerifier.Verify(func() interface{} {
// 		return new(jwt.Claims)
// 	})

// 	refreshTokenVerifier.Extractors = append(refreshTokenVerifier.Extractors, func(ctx iris.Context) string {
// 		var tokenInput utils.RefreshTokenInput
// 		err := ctx.ReadJSON(&tokenInput)
// 		if err != nil {
// 			return ""
// 		}

// 		return tokenInput.RefreshToken
// 	})

// 	user := app.Party("/api/user")
// 	{
// 		user.Post("/register", routes.Register)
// 		user.Post("/login", routes.Login)
// 		user.Post("/register-phone", routes.RegisterPhone)
// 		user.Post("/login-phone", routes.LoginPhone)
// 		user.Post("/facebook", routes.FacebookLoginOrSignUp)
// 		user.Post("/google", routes.GoogleLoginOrSignUp)
// 		user.Post("/apple", routes.AppleLoginOrSignUp)
// 		user.Post("/forgotpassword", routes.ForgotPassword)
// 		user.Post("/resetpassword", resetTokenVerifierMiddleware, routes.ResetPassword)
// 		user.Get("/search", accessTokenVerifierMiddleware, routes.SearchUsers)
// 		user.Get("/{id}/properties/saved", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetUserSavedProperties)
// 		user.Patch("/{id}/properties/saved", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.AlterUserSavedProperties)
// 		user.Patch("/{id}/pushtoken", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.AlterPushToken)
// 		user.Patch("/{id}/settings/notifications", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.AllowsNotifications)
// 		user.Get("/{id}/properties/contacted", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetUserContactedProperties)
// 		user.Patch("/{id}/profile", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.UpdateUserProfile)
// 		user.Get("/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetUser)
// 		user.Get("/profile/status", accessTokenVerifierMiddleware, routes.GetUserProfileStatusNew)
// 		user.Post("/verification", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SubmitVerification)
// 		// Feedback
// 		user.Post("/feedback", accessTokenVerifierMiddleware, routes.CreateFeedback)

// 		// User Profile routes
// 		user.Get("/profile", accessTokenVerifierMiddleware, routes.GetUserProfile)
// 		user.Post("/profile", accessTokenVerifierMiddleware, routes.CreateOrUpdateUserProfile)
// 		user.Put("/profile", accessTokenVerifierMiddleware, routes.CreateOrUpdateUserProfile)
// 		user.Delete("/profile", accessTokenVerifierMiddleware, routes.DeleteUserProfile)
// 	}
// 	property := app.Party("/api/property")

// 	// Admin routes
// 	admin := app.Party("/api/admin", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
// 	{
// 		admin.Get("/users", routes.AdminListUsers)
// 		admin.Patch("/users/{id:uint}/role", utils.SuperAdminOnlyMiddleware, routes.AdminChangeUserRole)
// 		admin.Get("/users/{id:uint}", routes.AdminGetUser)
// 		admin.Post("/users/{id:uint}/verify", routes.AdminVerifyUser)
// 		admin.Get("/properties", routes.AdminListProperties)
// 		admin.Get("/properties/{id:uint}", routes.AdminGetProperty)
// 		admin.Patch("/properties/{id:uint}/status", routes.AdminUpdatePropertyStatus)
// 		admin.Post("/properties/{id:uint}/flag", routes.AdminFlagProperty)
// 		admin.Get("/experiences", routes.AdminListExperiences)
// 		admin.Get("/experiences/{id:uint}", routes.AdminGetExperience)
// 		admin.Patch("/experiences/{id:uint}/status", routes.AdminUpdateExperienceStatus)
// 		admin.Get("/reservations", routes.AdminListReservations)
// 		admin.Get("/reservations/{id:uint}", routes.AdminGetReservation)
// 		admin.Post("/reservations/{id:uint}/cancel", routes.AdminCancelReservation)
// 		admin.Patch("/reservations/{id:uint}/status", routes.AdminUpdateReservationStatus)
// 		admin.Get("/reviews", routes.AdminListReviews)
// 		admin.Patch("/reviews/{id:uint}/status", routes.AdminUpdateReviewVisibility)
// 		admin.Delete("/reviews/{id:uint}", routes.AdminDeleteReview)
// 		admin.Get("/videos", routes.AdminListVideos)
// 		admin.Get("/videos/{id:uint}", routes.AdminGetVideo)
// 		admin.Patch("/videos/{id:uint}/status", routes.AdminUpdateVideoStatus)
// 		admin.Post("/videos/{id:uint}/force_unpublish", routes.AdminForceUnpublishVideo)
// 		admin.Get("/videos/{id:uint}/comments", routes.AdminListVideoComments)
// 		admin.Delete("/videos/{id:uint}/comments/{comment_id:uint}", routes.AdminDeleteVideoComment)
// 		admin.Get("/feedback", routes.AdminListFeedback)
// 		admin.Get("/stats", routes.AdminStats)
// 		admin.Get("/activity", routes.AdminActivity)
// 		admin.Get("/groups", routes.AdminListGroups)
// 		admin.Get("/groups/{id:uint}", routes.AdminGetGroup)
// 		admin.Patch("/groups/{id:uint}", routes.AdminUpdateGroup)
// 		admin.Post("/export", routes.AdminCreateExport)
// 		admin.Get("/export/{id:string}", routes.AdminGetExport)
// 	}
// 	{
// 		property.Post("/", routes.CreateProperty)
// 		property.Get("/{id}", routes.GetProperty)
// 		property.Get("/userid/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetPropertiesByUserID)
// 		property.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteProperty)
// 		property.Patch("/update/{id}", accessTokenVerifierMiddleware, routes.UpdateProperty)
// 		property.Post("/search", routes.GetPropertiesByBoundingBox)
// 		property.Delete("/image", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeletePropertyImage)
// 	}
// 	availability := app.Party("/api/availability")
// 	{
// 		availability.Get("/property/{propertyID}", routes.GetPropertyAvailability)
// 		availability.Post("/property", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetPropertyAvailability)
// 		availability.Post("/property/bulk", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetBulkPropertyAvailability)
// 		availability.Get("/pricing/{propertyID}", routes.GetPropertyPricing)
// 		availability.Post("/pricing", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.SetPropertyPricing)
// 		availability.Get("/discounts/{propertyID}", routes.GetPropertyDiscounts)
// 		availability.Post("/discounts", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertyDiscount)
// 		availability.Post("/block", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.BlockPropertyDates)
// 		availability.Get("/blocks/{propertyID}", routes.GetPropertyBlocks)
// 		availability.Post("/calculate-price", routes.CalculateBookingPrice)
// 	}
// 	categories := app.Party("/api/categories")
// 	{
// 		categories.Get("/", routes.GetCategories)
// 		categories.Get("/amenities", routes.GetAmenities)
// 		categories.Get("/amenities/categories", routes.GetAmenityCategories)
// 		categories.Get("/property/{id}", routes.GetPropertyCategories)
// 		categories.Get("/property/{id}/amenities", routes.GetPropertyAmenities)
// 		categories.Put("/property/{id}", accessTokenVerifierMiddleware, routes.UpdatePropertyCategories)
// 		categories.Put("/property/{id}/amenities", accessTokenVerifierMiddleware, routes.UpdatePropertyAmenities)
// 	}
// 	location := app.Party("/api/location")
// 	{
// 		location.Get("/near/{location}", routes.GetPropertiesNearLocation)
// 		location.Get("/locations", routes.GetAvailableLocations)
// 		location.Get("/coordinates", routes.GetPropertiesByCoordinates)
// 		location.Get("/search", routes.GetPropertiesWithFilters)
// 	}
// 	apartment := app.Party("/api/apartment")
// 	{
// 		apartment.Get("/property/{id}", routes.GetReservationsByPropertyID)
// 		apartment.Post("/property/{id}", accessTokenVerifierMiddleware, routes.CreateReservation)
// 		apartment.Patch("/{id}/status", accessTokenVerifierMiddleware, routes.UpdateReservationStatus)
// 		apartment.Post("/expire-pending", routes.ExpirePendingReservations)
// 		apartment.Delete("/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CancelReservation)
// 		apartment.Post("/property/{id}/validate", routes.ValidateReservationAvailability)
// 		apartment.Get("/host/reservations", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostReservations)
// 	}

// 	// New canonical reservations routes
// 	reservations := app.Party("/api/reservations")
// 	{
// 		reservations.Get("/user/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetUserReservations)
// 	}
// 	review := app.Party("/api/review")
// 	{
// 		review.Post("/property/{id}", accessTokenVerifierMiddleware, routes.CreateReview)
// 	}
// 	conversation := app.Party("/api/conversation")
// 	{
// 		conversation.Post("/", accessTokenVerifierMiddleware, routes.CreateConversation)
// 		conversation.Get("/{id}", accessTokenVerifierMiddleware, routes.GetConversationByID)
// 		conversation.Get("/user/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetConversationsByUserID)
// 	}
// 	messages := app.Party("/api/messages")
// 	{
// 		messages.Post("/", accessTokenVerifierMiddleware, routes.CreateMessage)
// 		messages.Get("/", accessTokenVerifierMiddleware, routes.ListMessages)
// 		messages.Post("/state", accessTokenVerifierMiddleware, routes.SetMessageState)
// 	}
// 	notifications := app.Party("/api/notifications")
// 	{
// 		notifications.Post("/test-push", routes.SendTestNotification)
// 		notifications.Post("/test-detailed/{userID:int}", routes.SendDetailedTestNotification)
// 		notifications.Post("/welcome", routes.SendWelcomeNotification)
// 		notifications.Get("/settings", accessTokenVerifierMiddleware, routes.GetUserNotificationSettings)
// 		notifications.Put("/settings", accessTokenVerifierMiddleware, routes.UpdateUserNotificationSettings)
// 	}
// 	collection := app.Party("/api/collection")
// 	{
// 		collection.Post("/", accessTokenVerifierMiddleware, routes.CreateCollection)
// 		collection.Get("/", accessTokenVerifierMiddleware, routes.GetUserCollections)
// 		collection.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateCollection)
// 		collection.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteCollection)
// 		collection.Post("/add-property", accessTokenVerifierMiddleware, routes.AddPropertyToCollection)
// 		collection.Post("/remove-property", accessTokenVerifierMiddleware, routes.RemovePropertyFromCollection)
// 		collection.Post("/remove-from-all", accessTokenVerifierMiddleware, routes.RemovePropertyFromAllCollections)
// 		collection.Get("/{id}/properties", accessTokenVerifierMiddleware, routes.GetCollectionProperties)
// 	}

// 	experienceCollection := app.Party("/api/experience-collection")
// 	{
// 		experienceCollection.Post("/", accessTokenVerifierMiddleware, routes.CreateExperienceCollection)
// 		experienceCollection.Get("/", accessTokenVerifierMiddleware, routes.GetUserExperienceCollections)
// 		experienceCollection.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateExperienceCollection)
// 		experienceCollection.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteExperienceCollection)
// 		experienceCollection.Post("/add-experience", accessTokenVerifierMiddleware, routes.AddExperienceToCollection)
// 		experienceCollection.Post("/remove-experience", accessTokenVerifierMiddleware, routes.RemoveExperienceFromCollection)
// 		experienceCollection.Post("/remove-from-all", accessTokenVerifierMiddleware, routes.RemoveExperienceFromAllCollections)
// 		experienceCollection.Get("/{id}/experiences", accessTokenVerifierMiddleware, routes.GetCollectionExperiences)
// 		experienceCollection.Get("/saved", accessTokenVerifierMiddleware, routes.GetUserSavedExperiences)
// 	}

// 	video := app.Party("/api/video")
// 	{
// 		video.Post("/", accessTokenVerifierMiddleware, routes.CreateVideo)
// 		video.Get("/feed", routes.GetVideoFeed)
// 		video.Post("/like", accessTokenVerifierMiddleware, routes.LikeVideo)
// 		video.Post("/unlike", accessTokenVerifierMiddleware, routes.UnlikeVideo)
// 		video.Post("/save", accessTokenVerifierMiddleware, routes.SaveVideo)
// 		video.Post("/unsave", accessTokenVerifierMiddleware, routes.UnsaveVideo)
// 		video.Post("/comment", accessTokenVerifierMiddleware, routes.CreateVideoComment)
// 		video.Get("/comment/{videoID}", accessTokenVerifierMiddleware, routes.GetVideoComments)
// 		video.Put("/comment/{id}", accessTokenVerifierMiddleware, routes.UpdateVideoComment)
// 		video.Delete("/comment/{id}", accessTokenVerifierMiddleware, routes.DeleteVideoComment)
// 		video.Post("/comment/like", accessTokenVerifierMiddleware, routes.LikeVideoComment)
// 		video.Post("/comment/unlike", accessTokenVerifierMiddleware, routes.UnlikeVideoComment)
// 		video.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteVideo)
// 		video.Get("/liked", accessTokenVerifierMiddleware, routes.GetLikedVideos)
// 		video.Get("/saved", accessTokenVerifierMiddleware, routes.GetSavedVideos)
// 	}

// 	experience := app.Party("/api/experience")
// 	{
// 		experience.Post("/", accessTokenVerifierMiddleware, routes.CreateExperience)
// 		experience.Get("/", accessTokenVerifierMiddleware, routes.GetUserExperiences)
// 		experience.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateExperience)
// 		experience.Post("/{id}/submit", accessTokenVerifierMiddleware, routes.SubmitExperienceForReview)
// 		experience.Get("/{id}", routes.GetExperienceDetails)
// 		experience.Get("/public", routes.GetPublicExperiences)
// 		// Invites & participants
// 		experience.Post("/{id}/invites", accessTokenVerifierMiddleware, routes.CreateExperienceInvites)
// 		experience.Get("/{id}/participants", routes.ListParticipants)
// 		experience.Post("/{id}/participants/{userID}/remove", accessTokenVerifierMiddleware, routes.RemoveParticipant)
// 		// Groups
// 		experience.Post("/{id}/groups", accessTokenVerifierMiddleware, routes.CreateOrOpenGroup)
// 		// Availability
// 		experience.Get("/{id}/availability", routes.ListAvailability)
// 		experience.Post("/{id}/availability", accessTokenVerifierMiddleware, routes.SetAvailability)
// 		// Booking
// 		experience.Post("/book", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreateExperienceBooking)
// 		experience.Get("/bookings", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetExperienceBookings)
// 		experience.Get("/host-bookings", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostExperienceBookings)
// 		experience.Patch("/bookings/{id}/mark-read", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.MarkBookingAsRead)
// 		experience.Delete("/bookings/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CancelExperienceBooking)
// 	}

// 	invites := app.Party("/api/invites")
// 	{
// 		invites.Get("/", accessTokenVerifierMiddleware, routes.ListInvites)
// 		invites.Post("/{inviteID}/accept", accessTokenVerifierMiddleware, routes.AcceptInvite)
// 		invites.Post("/{inviteID}/decline", accessTokenVerifierMiddleware, routes.DeclineInvite)
// 		invites.Post("/{inviteID}/cancel", accessTokenVerifierMiddleware, routes.CancelInvite)
// 	}

// 	groups := app.Party("/api/groups")
// 	{
// 		groups.Get("/mine", accessTokenVerifierMiddleware, routes.ListMyGroups)
// 		groups.Get("/{groupID}/members", routes.GetGroupMembers)
// 		groups.Post("/{groupID}/members/{memberID}/role", accessTokenVerifierMiddleware, routes.UpdateMemberRole)
// 		groups.Post("/{groupID}/members/{memberID}/remove", accessTokenVerifierMiddleware, routes.RemoveGuest)
// 		groups.Post("/{groupID}/leave", accessTokenVerifierMiddleware, routes.LeaveGroup)
// 		groups.Post("/{groupID}/finalize", accessTokenVerifierMiddleware, routes.FinalizeGroup)
// 		groups.Put("/{groupID}", accessTokenVerifierMiddleware, routes.UpdateGroup)
// 		groups.Delete("/{groupID}", accessTokenVerifierMiddleware, routes.DeleteGroup)
// 		// Chat
// 		groups.Get("/{groupID}/messages", accessTokenVerifierMiddleware, routes.ListGroupMessages)
// 		groups.Post("/{groupID}/messages", accessTokenVerifierMiddleware, routes.SendGroupMessage)
// 		groups.Post("/{groupID}/typing", accessTokenVerifierMiddleware, routes.Typing)
// 		groups.Get("/{groupID}/typing", accessTokenVerifierMiddleware, routes.ListTyping)
// 		// Wishlist
// 		groups.Get("/{groupID}/wishlist", accessTokenVerifierMiddleware, routes.ListGroupWishlist)
// 		groups.Post("/{groupID}/wishlist", accessTokenVerifierMiddleware, routes.AddGroupWishlist)
// 		groups.Post("/{groupID}/wishlist/{wishlistID}/like", accessTokenVerifierMiddleware, routes.LikeGroupWishlist)
// 		// Share
// 		groups.Post("/{groupID}/share/property", accessTokenVerifierMiddleware, routes.SharePropertyToGroup)
// 		// Discovery
// 		groups.Post("/discover", accessTokenVerifierMiddleware, routes.DiscoverGroups)
// 		groups.Post("/request-join", accessTokenVerifierMiddleware, routes.RequestToJoinGroup)
// 		groups.Get("/my-requests", accessTokenVerifierMiddleware, routes.GetMyJoinRequests)
// 		groups.Get("/{groupID}/requests", accessTokenVerifierMiddleware, routes.GetGroupJoinRequests)
// 		groups.Post("/requests/{requestID}/respond", accessTokenVerifierMiddleware, routes.RespondToJoinRequest)
// 	}

// 	chat := app.Party("/api/chat")
// 	{
// 		chat.Post("/start-direct", accessTokenVerifierMiddleware, routes.StartDirectConversation)
// 	}

// 	// Location Discovery routes
// 	locationDiscovery := app.Party("/api/location-discovery")
// 	{
// 		locationDiscovery.Get("/criteria", routes.GetLocationCriteria)
// 		locationDiscovery.Get("/criteria/{criteriaId}/properties", routes.GetLocationProperties)
// 		locationDiscovery.Get("/property/{propertyId}/criteria", routes.GetPropertyLocationCriteria)
// 		locationDiscovery.Post("/initialize", routes.InitializeLocationCriteriaEndpoint)
// 		locationDiscovery.Post("/assign-properties", routes.AssignPropertiesToCriteriaEndpoint)
// 	}

// 	// Properties Search
// 	properties := app.Party("/api/properties")
// 	{
// 		properties.Get("/search", routes.SearchProperties)
// 	}

// 	// Reviews
// 	reviews := app.Party("/api/reviews")
// 	{
// 		reviews.Get("/property/{propertyId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ListPropertyReviews)
// 		reviews.Post("/property/{propertyId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertyReview)
// 	}

// 	app.Post("/api/refresh", refreshTokenVerifierMiddleware, utils.RefreshToken)

// 	// // Get the port from the environment, fallback to 8080
// 	// port := os.Getenv("PORT")
// 	// if port == "" {
// 	// 	port = "4000"
// 	// }

// 	// app.Listen(":" + port) // notice the ":" before the port
// 	// Get Render's assigned PORT
// 	// Get Render's PORT
// 	port := os.Getenv("PORT")
// 	if port == "" {
// 		port = "4000" // fallback for local dev
// 	}
// 	addr := ":" + port

// 	fmt.Println("🚀 Starting server on 🆗🆗🆗", addr)

// 	// Listen once and handle errors
// 	if err := app.Listen(addr); err != nil {
// 		log.Fatalf("❌ failed to start server: %v", err)
// 	}

// }

package main

import (
	"apartments-clone-server/routes"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"fmt"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

func main() {
	// Only load .env in development
	if os.Getenv("RENDER") == "" {
		godotenv.Load()
	}

	// Initialize services
	storage.InitializeDB()
	storage.InitializeS3()
	storage.InitializeRedis()

	app := iris.New()
	app.Validator = validator.New()

	// CORS configuration
	app.AllowMethods(iris.MethodOptions)
	app.UseRouter(func(ctx iris.Context) {
		ctx.Header("Access-Control-Allow-Origin", ctx.GetHeader("Origin"))
		ctx.Header("Vary", "Origin")
		ctx.Header("Access-Control-Allow-Credentials", "true")
		ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
		ctx.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
		if ctx.Method() == iris.MethodOptions {
			ctx.StatusCode(iris.StatusNoContent)
			return
		}
		ctx.Next()
	})

	// Minimal middleware - compression only
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
	app.Get("/health", func(ctx iris.Context) {
		ctx.JSON(iris.Map{"status": "ok"})
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
		user.Post("/feedback", accessTokenVerifierMiddleware, routes.CreateFeedback)
		user.Get("/profile", accessTokenVerifierMiddleware, routes.GetUserProfile)
		user.Post("/profile", accessTokenVerifierMiddleware, routes.CreateOrUpdateUserProfile)
		user.Put("/profile", accessTokenVerifierMiddleware, routes.CreateOrUpdateUserProfile)
		user.Delete("/profile", accessTokenVerifierMiddleware, routes.DeleteUserProfile)
	}

	property := app.Party("/api/property")
	{
		property.Post("/", routes.CreateProperty)
		property.Get("/{id}", routes.GetProperty)
		property.Get("/userid/{id}", accessTokenVerifierMiddleware, utils.UserIDMiddleware, routes.GetPropertiesByUserID)
		property.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteProperty)
		property.Patch("/update/{id}", accessTokenVerifierMiddleware, routes.UpdateProperty)
		property.Post("/search", routes.GetPropertiesByBoundingBox)
		property.Delete("/image", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.DeletePropertyImage)
	}

	admin := app.Party("/api/admin", accessTokenVerifierMiddleware, utils.AdminOnlyMiddleware)
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
		location.Get("/coordinates", routes.GetPropertiesByCoordinates)
		location.Get("/search", routes.GetPropertiesWithFilters)
	}

	apartment := app.Party("/api/apartment")
	{
		apartment.Get("/property/{id}", routes.GetReservationsByPropertyID)
		apartment.Post("/property/{id}", accessTokenVerifierMiddleware, routes.CreateReservation)
		apartment.Patch("/{id}/status", accessTokenVerifierMiddleware, routes.UpdateReservationStatus)
		apartment.Post("/expire-pending", routes.ExpirePendingReservations)
		apartment.Delete("/{id}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CancelReservation)
		apartment.Post("/property/{id}/validate", routes.ValidateReservationAvailability)
		apartment.Get("/host/reservations", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.GetHostReservations)
	}

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
	{
		notifications.Post("/test-push", routes.SendTestNotification)
		notifications.Post("/test-detailed/{userID:int}", routes.SendDetailedTestNotification)
		notifications.Post("/welcome", routes.SendWelcomeNotification)
		notifications.Get("/settings", accessTokenVerifierMiddleware, routes.GetUserNotificationSettings)
		notifications.Put("/settings", accessTokenVerifierMiddleware, routes.UpdateUserNotificationSettings)
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
		video.Get("/feed", routes.GetVideoFeed)
		video.Post("/like", accessTokenVerifierMiddleware, routes.LikeVideo)
		video.Post("/unlike", accessTokenVerifierMiddleware, routes.UnlikeVideo)
		video.Post("/save", accessTokenVerifierMiddleware, routes.SaveVideo)
		video.Post("/unsave", accessTokenVerifierMiddleware, routes.UnsaveVideo)
		video.Post("/comment", accessTokenVerifierMiddleware, routes.CreateVideoComment)
		video.Get("/comment/{videoID}", accessTokenVerifierMiddleware, routes.GetVideoComments)
		video.Put("/comment/{id}", accessTokenVerifierMiddleware, routes.UpdateVideoComment)
		video.Delete("/comment/{id}", accessTokenVerifierMiddleware, routes.DeleteVideoComment)
		video.Post("/comment/like", accessTokenVerifierMiddleware, routes.LikeVideoComment)
		video.Post("/comment/unlike", accessTokenVerifierMiddleware, routes.UnlikeVideoComment)
		video.Delete("/{id}", accessTokenVerifierMiddleware, routes.DeleteVideo)
		video.Get("/liked", accessTokenVerifierMiddleware, routes.GetLikedVideos)
		video.Get("/saved", accessTokenVerifierMiddleware, routes.GetSavedVideos)
	}

	experience := app.Party("/api/experience")
	{
		experience.Post("/", accessTokenVerifierMiddleware, routes.CreateExperience)
		experience.Get("/", accessTokenVerifierMiddleware, routes.GetUserExperiences)
		experience.Put("/{id}", accessTokenVerifierMiddleware, routes.UpdateExperience)
		experience.Post("/{id}/submit", accessTokenVerifierMiddleware, routes.SubmitExperienceForReview)
		experience.Get("/{id}", routes.GetExperienceDetails)
		experience.Get("/public", routes.GetPublicExperiences)
		experience.Post("/{id}/invites", accessTokenVerifierMiddleware, routes.CreateExperienceInvites)
		experience.Get("/{id}/participants", routes.ListParticipants)
		experience.Post("/{id}/participants/{userID}/remove", accessTokenVerifierMiddleware, routes.RemoveParticipant)
		experience.Post("/{id}/groups", accessTokenVerifierMiddleware, routes.CreateOrOpenGroup)
		experience.Get("/{id}/availability", routes.ListAvailability)
		experience.Post("/{id}/availability", accessTokenVerifierMiddleware, routes.SetAvailability)
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
		groups.Get("/{groupID}/messages", accessTokenVerifierMiddleware, routes.ListGroupMessages)
		groups.Post("/{groupID}/messages", accessTokenVerifierMiddleware, routes.SendGroupMessage)
		groups.Post("/{groupID}/typing", accessTokenVerifierMiddleware, routes.Typing)
		groups.Get("/{groupID}/typing", accessTokenVerifierMiddleware, routes.ListTyping)
		groups.Get("/{groupID}/wishlist", accessTokenVerifierMiddleware, routes.ListGroupWishlist)
		groups.Post("/{groupID}/wishlist", accessTokenVerifierMiddleware, routes.AddGroupWishlist)
		groups.Post("/{groupID}/wishlist/{wishlistID}/like", accessTokenVerifierMiddleware, routes.LikeGroupWishlist)
		groups.Post("/{groupID}/share/property", accessTokenVerifierMiddleware, routes.SharePropertyToGroup)
		groups.Post("/discover", accessTokenVerifierMiddleware, routes.DiscoverGroups)
		groups.Post("/request-join", accessTokenVerifierMiddleware, routes.RequestToJoinGroup)
		groups.Get("/my-requests", accessTokenVerifierMiddleware, routes.GetMyJoinRequests)
		groups.Get("/{groupID}/requests", accessTokenVerifierMiddleware, routes.GetGroupJoinRequests)
		groups.Post("/requests/{requestID}/respond", accessTokenVerifierMiddleware, routes.RespondToJoinRequest)
	}

	chat := app.Party("/api/chat")
	{
		chat.Post("/start-direct", accessTokenVerifierMiddleware, routes.StartDirectConversation)
	}

	locationDiscovery := app.Party("/api/location-discovery")
	{
		locationDiscovery.Get("/criteria", routes.GetLocationCriteria)
		locationDiscovery.Get("/criteria/{criteriaId}/properties", routes.GetLocationProperties)
		locationDiscovery.Get("/property/{propertyId}/criteria", routes.GetPropertyLocationCriteria)
		locationDiscovery.Post("/initialize", routes.InitializeLocationCriteriaEndpoint)
		locationDiscovery.Post("/assign-properties", routes.AssignPropertiesToCriteriaEndpoint)
	}

	properties := app.Party("/api/properties")
	{
		properties.Get("/search", routes.SearchProperties)
	}

	reviews := app.Party("/api/reviews")
	{
		reviews.Get("/property/{propertyId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.ListPropertyReviews)
		reviews.Post("/property/{propertyId:uint}", accessTokenVerifierMiddleware, utils.UserIDFromTokenMiddleware, routes.CreatePropertyReview)
	}

	app.Post("/api/refresh", refreshTokenVerifierMiddleware, utils.RefreshToken)

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}
	addr := "0.0.0.0:" + port

	fmt.Printf("🚀 Server starting on %s\n", addr)

	// Start server
	if err := app.Listen(addr); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}
