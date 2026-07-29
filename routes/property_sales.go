package routes

import (
	"apartments-clone-server/goldproperty"
	"apartments-clone-server/models"
	"apartments-clone-server/places"
	"apartments-clone-server/services"
	pushsvc "apartments-clone-server/services/push"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jwt "github.com/kataras/iris/v12/middleware/jwt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func optInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// filterHTTPMediaURLs keeps only absolute http(s) links; normalizes GCS paths; drops data: blobs.
func filterHTTPMediaURLs(urls []string) []string {
	return storage.NormalizePublicMediaURLs(urls)
}

// propertySaleOrgOwnerID returns the organization owner's user id when the listing belongs to an org.
func propertySaleOrgOwnerID(property *models.PropertySale) uint {
	if property.OrganizationID == nil || *property.OrganizationID == 0 {
		return 0
	}
	if property.Organization != nil {
		return property.Organization.OwnerID
	}
	var org models.Organization
	if err := storage.DB.Select("id", "owner_id").First(&org, *property.OrganizationID).Error; err != nil {
		return 0
	}
	return org.OwnerID
}

func saleListingHasVerifiedBroker(p models.PropertySale) bool {
	if p.Owner != nil && services.IsVerifiedBroker(p.Owner) {
		return true
	}
	if p.Organization != nil && p.Organization.OwnerID != 0 {
		owner := p.Organization.Owner
		if owner.ID != 0 && services.IsVerifiedBroker(&owner) {
			return true
		}
	}
	return false
}

// propertySaleUserCanEdit mirrors UpdatePropertySale permission rules.
func propertySaleUserCanEdit(userID uint, property *models.PropertySale) bool {
	if property.OwnerID != nil && *property.OwnerID == userID {
		return true
	}
	if oid := propertySaleOrgOwnerID(property); oid != 0 && oid == userID {
		return true
	}
	if property.AgentID != nil {
		var agent models.Agent
		if err := storage.DB.Where("id = ? AND user_id = ?", *property.AgentID, userID).First(&agent).Error; err == nil {
			return true
		}
	}
	if property.OrganizationID != nil {
		var member models.OrganizationMember
		if err := storage.DB.Where("user_id = ? AND organization_id = ? AND status = ? AND is_active = ?",
			userID, *property.OrganizationID, "active", true).First(&member).Error; err == nil {
			if member.HasPermission(models.PermissionPropertyEdit) || member.Role == "admin" || member.Role == "manager" || member.Role == "editor" {
				return true
			}
		}
	}
	return false
}

// propertySaleUserCanToggleGold restricts the Gold flag to platform admins and agency leadership.
func propertySaleUserCanToggleGold(_ uint, userRole string, _ *models.PropertySale) bool {
	return userRole == "admin" || userRole == "super_admin"
}

// triggerNewPropertyNotification sends notifications when a property is published
func triggerNewPropertyNotification(property models.PropertySale) {
	// Get first image URL for rich notification
	var imageURL string
	if len(property.Images) > 0 && property.Images[0] != "" {
		imageURL = property.Images[0]
	}

	
	// Get city/zone names
	cityName := property.City
	zoneName := ""
	if property.ZoneRef != nil {
		zoneName = property.ZoneRef.Name
	} else if property.ZoneID != nil {
		var zone models.Zone
		if err := storage.DB.First(&zone, *property.ZoneID).Error; err == nil {
			zoneName = zone.Name
		}
	}

	// Try to send targeted notifications first
	err := services.NotificationServiceInstance.SendNewPropertyNotification(
		property.ID,
		property.Title,
		property.CityID,
		cityName,
		property.ZoneID,
		zoneName,
		property.Bedrooms,
		property.Bathrooms,
		property.SquareFootage,
		imageURL,
		property.CreatedAt,
	)

	// If targeted notifications fail or no users match, send generic notifications
	// This ensures we always notify some users even if personalization fails
	if err != nil {
		log.Printf("⚠️ Targeted notifications failed, sending generic notifications: %v", err)
		
		// Get all users with notifications enabled (fallback)
		var users []models.User
		if err := storage.DB.Where("allows_notifications = ?", true).
			Limit(100). // Limit to avoid spamming too many users
			Find(&users).Error; err == nil {
			var userIDs []uint
			for _, user := range users {
				userIDs = append(userIDs, user.ID)
			}
			
			services.NotificationServiceInstance.SendGenericPropertyNotification(
				property.ID,
				property.Title,
				cityName,
				property.Bedrooms,
				property.Bathrooms,
				property.SquareFootage,
				imageURL,
				property.CreatedAt,
				userIDs,
			)
		}
	}
}

// triggerInvestmentOpportunityNotification sends a targeted push to devices
// explicitly interested in investment opportunities.
func triggerInvestmentOpportunityNotification(property models.PropertySale) {
	var prefs []models.AnonymousUserPreference
	if err := storage.DB.Where("last_active >= ?", time.Now().AddDate(0, -3, 0)).Find(&prefs).Error; err != nil {
		log.Printf("⚠️ triggerInvestmentOpportunityNotification: failed loading preferences: %v", err)
		return
	}
	if len(prefs) == 0 {
		return
	}

	interestedDevice := make(map[string]struct{})
	for _, p := range prefs {
		if strings.TrimSpace(p.DeviceID) == "" {
			continue
		}
		for _, it := range p.Interests {
			if strings.EqualFold(strings.TrimSpace(it), "investment") {
				interestedDevice[p.DeviceID] = struct{}{}
				break
			}
		}
	}
	if len(interestedDevice) == 0 {
		return
	}

	var devices []models.MarketingDevice
	if err := storage.DB.
		Where("marketing_opt_in = true AND fcm_token != ''").
		Find(&devices).Error; err != nil {
		log.Printf("⚠️ triggerInvestmentOpportunityNotification: failed loading marketing devices: %v", err)
		return
	}

	var tokens []string
	now := time.Now()
	allowedDeviceIDs := make([]uint, 0, len(devices))
	for _, d := range devices {
		if _, ok := interestedDevice[d.DeviceID]; !ok {
			continue
		}
		// Per-device anti-spam: never send marketing/investment bursts in the same minute.
		if d.NextSendAt != nil && now.Before(*d.NextSendAt) {
			continue
		}
		if d.LastSentAt != nil && now.Sub(*d.LastSentAt) < time.Minute {
			continue
		}
		t := strings.TrimSpace(d.FCMToken)
		if t == "" {
			continue
		}
		tokens = append(tokens, t)
		allowedDeviceIDs = append(allowedDeviceIDs, d.ID)
	}
	if len(tokens) == 0 {
		return
	}

	imageURL := ""
	if len(property.Images) > 0 {
		imageURL = strings.TrimSpace(property.Images[0])
	}

	title := "Hot Investment Alert"
	body := fmt.Sprintf("New potential deal in %s: %s. Tap to review ROI potential now.", property.City, property.Title)
	data := map[string]string{
		"type":       "investment_opportunity",
		"screen":     "PropertySaleDetails",
		"propertyId": strconv.FormatUint(uint64(property.ID), 10),
	}
	if err := pushsvc.SendExpoPushWithImage(tokens, title, body, imageURL, data); err != nil {
		log.Printf("⚠️ triggerInvestmentOpportunityNotification: push failed: %v", err)
		return
	}
	// Record send timestamps to prevent duplicate bursts to the same device.
	next := now.Add(time.Minute)
	if len(allowedDeviceIDs) > 0 {
		_ = storage.DB.Model(&models.MarketingDevice{}).
			Where("id IN ?", allowedDeviceIDs).
			Updates(map[string]interface{}{
				"last_sent_at": now,
				"next_send_at": next,
			}).Error
	}
}

// CreatePropertySale creates a new property for sale (sync; dashboards / legacy clients).
func CreatePropertySale(ctx iris.Context) {
	start := time.Now()
	log.Printf("🏠 CreatePropertySale SYNC hit path=%s content-length=%s", ctx.Path(), ctx.GetHeader("Content-Length"))

	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}
	log.Printf("🏠 CreatePropertySale start userID=%d", userID)

	var input PropertySaleCreatePayload
	if err := ctx.ReadJSON(&input); err != nil {
		log.Printf("❌ CreatePropertySale ReadJSON: %v", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON", "details": err.Error()})
		return
	}
	log.Printf("📥 CreatePropertySale body read in %s (images=%d videos=%d)", time.Since(start), len(input.Images), len(input.Videos))

	propertyID, err := ExecuteCreatePropertySale(userID, &input, nil)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			ctx.StatusCode(http.StatusGatewayTimeout)
			ctx.JSON(iris.Map{"error": "Database timeout while creating property"})
			return
		}
		details := err.Error()
		if strings.Contains(details, "required") ||
			strings.Contains(details, "must") ||
			strings.Contains(details, "at least") {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Validation failed", "details": details})
			return
		}
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create property", "details": details})
		return
	}
	log.Printf("🏁 CreatePropertySale responded id=%d in %s", propertyID, time.Since(start))

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(iris.Map{
		"message": "Property created successfully",
		"id":      propertyID,
	})
}

// GetUserPropertySales gets all property sales for user (organization or individual)
func GetUserPropertySales(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an ACTIVE organization (as owner or active member)
	var organization models.Organization
	var hasOrganization bool

	// First check if user is owner
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		hasOrganization = true
	} else {
		// Check if user is an ACTIVE member (not removed) - directly query to ensure status is "active"
		var member models.OrganizationMember
		if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
			Preload("Organization").
			First(&member).Error; err == nil {
			organization = member.Organization
			hasOrganization = true
		}
	}

	var properties []models.PropertySale
	query := storage.DB.Preload("Agent.User").Preload("Organization").Preload("Owner")

	if hasOrganization {
		// Fetch properties from organization OR individual properties owned by user
		// CRITICAL: Include personal properties (organization_id IS NULL AND owner_id = userID)
		// This ensures personal properties created BEFORE joining are still visible
		query = query.Where(
			"organization_id = ? OR (organization_id IS NULL AND owner_id = ?)",
			organization.ID, userID,
		)
	} else {
		// User has no organization - fetch ALL individual properties owned by user
		// CRITICAL: After leaving an agency, personal properties (organization_id IS NULL) must be visible
		// This ensures personal properties are visible after leaving
		query = query.Where("organization_id IS NULL AND owner_id = ?", userID)
	}

	if err := query.Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	// Localize titles/descriptions based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
	}

	ctx.JSON(iris.Map{"properties": properties})
}

// GetPropertySale gets a specific property sale
func GetPropertySale(ctx iris.Context) {
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Agent.User").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	property.Title = utils.ResolveLocalizedText(property.Title, property.TitleTranslations, lang)
	property.Description = utils.ResolveLocalizedText(property.Description, property.DescriptionTranslations, lang)

	redactPropertySaleHostNote(&property, optionalAuthUserID(ctx))

	expandPropertySaleVideoRows([]models.PropertySale{property})

	ctx.JSON(iris.Map{"property": property})
}

// GetPropertySaleGoldInsights returns distribution metrics for Gold listings (visible to anyone who can edit the sale).
func GetPropertySaleGoldInsights(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	if propertyID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	if !propertySaleUserCanEdit(userID, &property) {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Forbidden"})
		return
	}

	var stat models.GoldPropertyStat
	_ = storage.DB.Where("property_sale_id = ?", property.ID).First(&stat).Error
	feed := stat.FeedImpressions
	detail := stat.DetailViews
	notify := stat.NotificationsSent
	var engagement float64
	if feed > 0 {
		engagement = float64(detail) / float64(feed) * 100
	}

	ctx.JSON(iris.Map{
		"property_id":             property.ID,
		"title":                   property.Title,
		"is_gold":                 property.IsGold,
		"view_count":              property.ViewCount,
		"feed_impressions":        feed,
		"detail_views":            detail,
		"notifications_sent":    notify,
		"feed_to_detail_rate_pct": engagement,
		"stats_updated_at":        stat.UpdatedAt,
	})
}

// AdminBackfillNearbyPlaces runs the nearby-places backfill for existing property sales (admin only).
// Query params:
//   - all=1 or all=true: run until all properties with coordinates are done (max 1000 per call), then return totals.
//   - batch_size, offset: for a single batch (default 50, 0). Use multiple calls to paginate.
//   - max: with all=1, cap how many to process (default 1000).
func AdminBackfillNearbyPlaces(ctx iris.Context) {
	if places.DefaultService == nil {
		ctx.StatusCode(http.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "Nearby places service not configured (GOOGLE_PLACES_API_KEY)"})
		return
	}
	runAll := ctx.URLParam("all") == "1" || strings.EqualFold(ctx.URLParam("all"), "true")
	if runAll {
		maxToProcess := ctx.URLParamIntDefault("max", 1000)
		if maxToProcess <= 0 {
			maxToProcess = 1000
		}
		result := places.DefaultService.BackfillAll(maxToProcess)
		ctx.JSON(iris.Map{
			"message":   "Backfill all completed (existing properties with coordinates)",
			"processed": result.Processed,
			"skipped":   result.Skipped,
			"failed":    result.Failed,
		})
		return
	}
	batchSize := ctx.URLParamIntDefault("batch_size", 50)
	offset := ctx.URLParamIntDefault("offset", 0)
	if batchSize <= 0 || batchSize > 200 {
		batchSize = 50
	}
	result := places.DefaultService.Backfill(batchSize, offset)
	ctx.JSON(iris.Map{
		"message":   "Backfill run completed",
		"processed": result.Processed,
		"skipped":   result.Skipped,
		"failed":    result.Failed,
	})
}

// GetPropertySaleNearby returns nearby restaurants, hospitals, and schools for a property (from Google Places, stored in DB).
func GetPropertySaleNearby(ctx iris.Context) {
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	if propertyID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}
	if places.DefaultService == nil {
		ctx.JSON(iris.Map{"restaurants": []interface{}{}, "hospitals": []interface{}{}, "schools": []interface{}{}})
		return
	}
	// Optionally trigger refresh if data is stale (async)
	if places.DefaultService.NeedsRefresh(uint(propertyID)) {
		var prop models.PropertySale
		if err := storage.DB.Select("id, latitude, longitude").First(&prop, propertyID).Error; err == nil && prop.Latitude != 0 && prop.Longitude != 0 {
			go places.DefaultService.FetchAndSaveNearby(prop.ID, prop.Latitude, prop.Longitude)
		}
	}
	out, err := places.DefaultService.GetNearbyForProperty(uint(propertyID))
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to load nearby places"})
		return
	}
	ctx.JSON(out)
}

// CreateOffer allows an authenticated user to submit an offer on a property sale
func CreateOffer(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	propertyIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	propertyID := uint(propertyIDU64)

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var payload struct {
		Amount  float64 `json:"amount"`
		Message string  `json:"message"`
	}
	if err := ctx.ReadJSON(&payload); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid payload"})
		return
	}
	if payload.Amount <= 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Amount must be greater than zero"})
		return
	}

	offer := models.PropertyOffer{
		PropertyID: property.ID,
		UserID:     userID,
		Amount:     payload.Amount,
		Message:    payload.Message,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}
	if err := storage.DB.Create(&offer).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create offer"})
		return
	}

	// Get user info for the message
	var user models.User
	if err := storage.DB.First(&user, userID).Error; err != nil {
		fmt.Printf("❌ Failed to get user info: %v\n", err)
	}
	userName := user.FirstName + " " + user.LastName
	if userName == " " {
		userName = "مستخدم"
	}

	// Determine property owner ID
	var ownerID uint
	if property.OwnerID != nil {
		ownerID = *property.OwnerID
	} else if property.OrganizationID != nil && property.Organization.OwnerID != 0 {
		ownerID = property.Organization.OwnerID
	}

	if ownerID > 0 && ownerID != userID {
		// Create Arabic message for the offer
		arabicMessage := fmt.Sprintf(
			"🏠 عرض شراء جديد!\n\n"+
				"السلام عليكم،\n\n"+
				"أنا مهتم بشراء عقارك '%s'.\n\n"+
				"💰 **مبلغ العرض:** %.0f MRU\n\n",
			property.Title, payload.Amount)

		if payload.Message != "" {
			arabicMessage += fmt.Sprintf("📝 **رسالتي:**\n%s\n\n", payload.Message)
		}

		arabicMessage += "أرجو التواصل معي لمناقشة التفاصيل.\n\nشكراً لك! 🙏"

		// Create direct message between user and owner
		directMessage := models.DirectMessage{
			SenderID:   userID,
			ReceiverID: ownerID,
			Content:    arabicMessage,
			IsRead:     false,
		}
		if err := storage.DB.Create(&directMessage).Error; err != nil {
			fmt.Printf("❌ Failed to create direct message for offer: %v\n", err)
		} else {
			fmt.Printf("✅ Created direct message for offer from user %d to owner %d\n", userID, ownerID)
		}

		// Send push notification to owner
		go func() {
			services.NotificationServiceInstance.SendPropertyOfferNotificationToHost(
				offer.ID,
				property.ID,
				ownerID,
				userID,
				userName,
				property.Title,
				payload.Amount,
			)
		}()
	}

	ctx.JSON(iris.Map{"offer": offer, "ok": true})
}

// GetOrganizationOffers lists all offers for properties owned by the authenticated user's organization
func GetOrganizationOffers(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	// Find organization by owner
	var org models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&org).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	// Join offers with property_sales to filter by organization
	type OfferWithJoins struct {
		models.PropertyOffer
		Property models.PropertySale `gorm:"embedded"`
		User     models.User         `gorm:"embedded"`
	}

	var offers []models.PropertyOffer
	if err := storage.DB.
		Preload("Property").
		Preload("Property.Organization").
		Preload("User").
		Where("property_id IN (SELECT id FROM property_sales WHERE organization_id = ?)", org.ID).
		Order("created_at DESC").
		Find(&offers).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch offers"})
		return
	}

	// Build lightweight response with user name/initials
	resp := make([]iris.Map, 0, len(offers))
	for _, o := range offers {
		fullName := ""
		if o.User.FirstName != "" || o.User.LastName != "" {
			fullName = (o.User.FirstName + " " + o.User.LastName)
		}
		resp = append(resp, iris.Map{
			"id":         o.ID,
			"amount":     o.Amount,
			"message":    o.Message,
			"status":     o.Status,
			"created_at": o.CreatedAt,
			"user": iris.Map{
				"id":          o.UserID,
				"firstName":   o.User.FirstName,
				"lastName":    o.User.LastName,
				"email":       o.User.Email,
				"avatarURL":   o.User.AvatarURL,
				"displayName": fullName,
			},
			"property": iris.Map{
				"id":    o.PropertyID,
				"title": o.Property.Title,
			},
		})
	}
	ctx.JSON(iris.Map{"offers": resp})
}

// UpdateOfferStatus updates an offer's status (accept/reject/withdraw)
func UpdateOfferStatus(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	offerIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	offerID := uint(offerIDU64)

	var offer models.PropertyOffer
	if err := storage.DB.Preload("Property").First(&offer, offerID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Offer not found"})
		return
	}

	// Only org owner of the property can update
	var property models.PropertySale
	if err := storage.DB.First(&property, offer.PropertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	// Check if property has an organization
	if property.OrganizationID == nil {
		// Property belongs to individual owner - need to check ownership differently
		// For now, allow if user is authenticated (we may need to add user_id to PropertySale later)
		// TODO: Add user_id field to PropertySale to track individual owners
	} else {
		var org models.Organization
		if err := storage.DB.First(&org, *property.OrganizationID).Error; err != nil {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Access denied"})
			return
		}
		if org.OwnerID != userID {
			ctx.StatusCode(http.StatusForbidden)
			ctx.JSON(iris.Map{"error": "Access denied"})
			return
		}
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := ctx.ReadJSON(&payload); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	if payload.Status != "accepted" && payload.Status != "rejected" && payload.Status != "withdrawn" && payload.Status != "pending" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid status"})
		return
	}

	offer.Status = payload.Status
	if err := storage.DB.Save(&offer).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update offer"})
		return
	}

	ctx.JSON(iris.Map{"offer": offer, "ok": true})
}

// PublicOfferInsights returns aggregated offer insights for a published property
func PublicOfferInsights(ctx iris.Context) {
	propertyIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	propertyID := uint(propertyIDU64)

	// Only for published properties
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND status = ? AND is_published = ? AND is_deactivated = ? AND deleted_at IS NULL", propertyID, "published", true, false).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Aggregate offers
	type Row struct {
		Count int64
		Min   float64
		Max   float64
		Avg   float64
	}
	var row Row
	if err := storage.DB.
		Raw("SELECT COUNT(*) as count, COALESCE(MIN(amount),0) as min, COALESCE(MAX(amount),0) as max, COALESCE(AVG(amount),0) as avg FROM property_offers WHERE property_id = ?", propertyID).
		Scan(&row).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to compute insights"})
		return
	}

	ctx.JSON(iris.Map{
		"offers": iris.Map{
			"count":    row.Count,
			"lowest":   row.Min,
			"highest":  row.Max,
			"average":  row.Avg,
			"currency": "MRU",
		},
		"property": iris.Map{"id": property.ID, "title": property.Title},
	})
}

// GetPublicPropertyOffers returns all individual offers for a published property (for chart visualization)
func GetPublicPropertyOffers(ctx iris.Context) {
	propertyIDU64, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	propertyID := uint(propertyIDU64)

	// Only for published properties
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND status = ? AND is_published = ? AND is_deactivated = ? AND deleted_at IS NULL", propertyID, "published", true, false).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var offers []models.PropertyOffer
	if err := storage.DB.Where("property_id = ?", propertyID).
		Order("created_at ASC").
		Find(&offers).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch offers"})
		return
	}

	// Build lightweight response
	resp := make([]iris.Map, 0, len(offers))
	for _, o := range offers {
		resp = append(resp, iris.Map{
			"id":         o.ID,
			"amount":     o.Amount,
			"price":      o.Amount, // Alias for compatibility
			"created_at": o.CreatedAt.Format(time.RFC3339),
			"status":     o.Status,
		})
	}

	ctx.JSON(iris.Map{"offers": resp})
}

// UpdatePropertySale updates a property sale
// Allows: property owner, organization owner, assigned agent, or organization members with edit permissions
func UpdatePropertySale(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Used for price-drop notifications (smart, anti-spam).
	oldListingPrice := property.ListingPrice
	oldWasPublished := property.Status == "published" && property.IsPublished
	listingPriceWasUpdated := false

	if !propertySaleUserCanEdit(userID, &property) {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You do not have permission to edit this property"})
		return
	}

	var input struct {
		Title           string                   `json:"title"`
		Description     string                   `json:"description"`
		PropertyType    string                   `json:"property_type"`
		Category        string                   `json:"category"`
		Address         string                   `json:"address"`
		City            string                   `json:"city"`
		CityID          *uint                    `json:"city_id"`
		ZoneID          *uint                    `json:"zone_id"`
		QuartierID      *uint                    `json:"quartier_id"`
		SubQuartierID   *uint                    `json:"sub_quartier_id"`
		State           string                   `json:"state"`
		Country         string                   `json:"country"`
		PostalCode      string                   `json:"postal_code"`
		Latitude        float64                  `json:"latitude"`
		Longitude       float64                  `json:"longitude"`
		Bedrooms        int                      `json:"bedrooms"`
		Bathrooms       int                      `json:"bathrooms"`
		SquareFootage   int                      `json:"square_footage"`
		Area            int                      `json:"area"` // Alias for square_footage
		LotSize         float64                  `json:"lot_size"`
		YearBuilt       int                      `json:"year_built"`
		ParkingSpaces   int                      `json:"parking_spaces"`
		ListingPrice    float64                  `json:"listing_price"`
		Price           float64                  `json:"price"` // Alias for listing_price
		Currency        string                   `json:"currency"`
		PropertyTax     float64                  `json:"property_tax"`
		HOA             float64                  `json:"hoa"`
		Images          []string                 `json:"images"`
		Videos          []string                 `json:"videos"`
		VirtualTour     string                   `json:"virtual_tour"`
		FloorPlans      []models.FloorPlan       `json:"floor_plans"`
		Neighborhood    *models.NeighborhoodInfo `json:"neighborhood"`
		IndoorFeatures  []string                 `json:"indoor_features"`
		OutdoorFeatures []string                 `json:"outdoor_features"`
		Features        []string                 `json:"features"`    // Legacy
		Amenities       []string                 `json:"amenities"`   // Legacy
		AmenityIDs      []uint                   `json:"amenity_ids"` // New - amenity IDs from database
		PaperTypes      []string                 `json:"paper_types"`
		AgentID         *uint                    `json:"agent_id"`
		Status          string                   `json:"status"`
		Truckeck        *bool                    `json:"truckeck"` // Only applied when user is admin/super_admin
		HostPrivateNote *string                  `json:"host_private_note"`
		IsGold          *bool                    `json:"is_gold"` // Agency owner/manager or platform admin only
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Only platform admins can set Truckeck (quality control validated)
	userRole, _ := ctx.Values().Get("userRole").(string)
	if input.Truckeck != nil && (userRole == "admin" || userRole == "super_admin") {
		property.Truckeck = *input.Truckeck
	}
	if input.IsGold != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "Gold listing can only be changed from admin dashboard"})
		return
	}

	// Update fields
	titleChanged := false
	descChanged := false
	if input.Title != "" {
		property.Title = input.Title
		titleChanged = true
	}
	if input.Description != "" {
		property.Description = input.Description
		descChanged = true
	}
	if input.PropertyType != "" {
		property.PropertyType = input.PropertyType
	}
	if input.Category != "" {
		property.Category = input.Category
	}
	if input.Address != "" {
		property.Address = input.Address
	}
	if input.City != "" {
		property.City = input.City
	}
	if input.CityID != nil {
		property.CityID = input.CityID
	}
	if input.ZoneID != nil {
		property.ZoneID = input.ZoneID
	}
	if input.QuartierID != nil {
		property.QuartierID = input.QuartierID
	}
	if input.SubQuartierID != nil {
		property.SubQuartierID = input.SubQuartierID
	}
	if input.State != "" {
		property.State = input.State
	}
	if input.Country != "" {
		property.Country = input.Country
	}
	if input.PostalCode != "" {
		property.PostalCode = input.PostalCode
	}
	if input.Latitude != 0 {
		property.Latitude = input.Latitude
	}
	if input.Longitude != 0 {
		property.Longitude = input.Longitude
	}
	if input.Bedrooms != 0 {
		property.Bedrooms = input.Bedrooms
	}
	if input.Bathrooms != 0 {
		property.Bathrooms = input.Bathrooms
	}
	// Handle both square_footage and area (alias)
	if input.SquareFootage != 0 {
		property.SquareFootage = input.SquareFootage
	} else if input.Area != 0 {
		property.SquareFootage = input.Area
	}
	if input.LotSize != 0 {
		property.LotSize = input.LotSize
	}
	if input.YearBuilt != 0 {
		property.YearBuilt = input.YearBuilt
	}
	if input.ParkingSpaces != 0 {
		property.ParkingSpaces = input.ParkingSpaces
	}
	// Handle both listing_price and price (alias)
	if input.ListingPrice != 0 {
		property.ListingPrice = input.ListingPrice
		listingPriceWasUpdated = true
		// Recalculate price per square foot
		if property.SquareFootage > 0 {
			property.PricePerSqFt = input.ListingPrice / float64(property.SquareFootage)
		}
	} else if input.Price != 0 {
		property.ListingPrice = input.Price
		listingPriceWasUpdated = true
		// Recalculate price per square foot
		if property.SquareFootage > 0 {
			property.PricePerSqFt = input.Price / float64(property.SquareFootage)
		}
	}
	if input.Currency != "" {
		property.Currency = input.Currency
	}
	if input.PropertyTax != 0 {
		property.PropertyTax = input.PropertyTax
	}
	if input.HOA != 0 {
		property.HOA = input.HOA
	}
	if input.Images != nil {
		sanitized, err := storage.SanitizeHTTPMediaURLs("images", input.Images)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		property.Images = sanitized
	}
	if input.Videos != nil {
		sanitized, err := storage.SanitizeHTTPMediaURLs("videos", input.Videos)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		property.Videos = sanitized
	}
	if input.VirtualTour != "" {
		property.VirtualTour = input.VirtualTour
	}
	if input.FloorPlans != nil {
		property.FloorPlans = input.FloorPlans
	}
	if input.Neighborhood != nil {
		property.Neighborhood = input.Neighborhood
	}
	if input.HostPrivateNote != nil {
		property.HostPrivateNote = sanitizeHostPrivateNote(*input.HostPrivateNote)
	}
	// Handle indoor and outdoor features
	if input.IndoorFeatures != nil || input.OutdoorFeatures != nil {
		var allFeatures []string
		if input.IndoorFeatures != nil {
			allFeatures = append(allFeatures, input.IndoorFeatures...)
		}
		if input.OutdoorFeatures != nil {
			allFeatures = append(allFeatures, input.OutdoorFeatures...)
		}
		property.Features = allFeatures
	} else if input.Features != nil {
		property.Features = input.Features
	}

	// Handle amenities - update both legacy and new format
	if input.AmenityIDs != nil && len(input.AmenityIDs) > 0 {
		// Update Many2Many relationship
		var amenities []models.Amenity
		if err := storage.DB.Where("id IN ?", input.AmenityIDs).Find(&amenities).Error; err == nil {
			if err := storage.DB.Model(&property).Association("AmenityList").Replace(amenities); err != nil {
				log.Printf("⚠️  Warning: Failed to update amenities: %v", err)
			}
		}
		// Also update legacy JSON field for backward compatibility
		if input.Amenities != nil {
			property.Amenities = input.Amenities
		}
	} else if input.Amenities != nil {
		property.Amenities = input.Amenities
	}
	if input.PaperTypes != nil {
		property.PaperTypes = input.PaperTypes
	}
	if input.AgentID != nil {
		property.AgentID = input.AgentID
	}
	if input.Status != "" {
		property.Status = input.Status
	}

	// Update translations if title or description changed
	if titleChanged || descChanged {
		if titleChanged {
			titleTranslations := services.TranslateAllLanguages(property.Title)
			if b, err := json.Marshal(titleTranslations); err == nil {
				property.TitleTranslations = b
			}
		}
		if descChanged {
			descTranslations := services.TranslateAllLanguages(property.Description)
			if b, err := json.Marshal(descTranslations); err == nil {
				property.DescriptionTranslations = b
			}
		}
	}

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update property"})
		return
	}

	if input.Videos != nil {
		if err := SyncPropertySaleVideoRows(property.ID, userID, property.Videos); err != nil {
			log.Printf("⚠️ UpdatePropertySale SyncPropertySaleVideoRows failed id=%d: %v", property.ID, err)
		}
	}

	// Smart notification: price drop on a published listing.
	// Best-effort: never block the update request on notification issues.
	if listingPriceWasUpdated &&
		oldWasPublished &&
		property.Status == "published" &&
		property.IsPublished &&
		property.ListingPrice < oldListingPrice {
		imageURL := ""
		if len(property.Images) > 0 {
			imageURL = property.Images[0]
		}

		oldPrice := oldListingPrice
		newPrice := property.ListingPrice
		propTitle := property.Title
		propID := property.ID
		go func() {
			_ = services.NotificationServiceInstance.NotifyPriceDropForPropertySale(
				propID,
				propTitle,
				oldPrice,
				newPrice,
				imageURL,
			)
		}()
	}

	// 🔴 INVALIDATE PROPERTY CACHE
	go func(propID uint) {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		propertySalesCache.InvalidatePropertyDetails(bgCtx, propID)
		propertySalesCache.InvalidatePropertySalesLists(bgCtx)
	}(property.ID)

	ctx.JSON(iris.Map{
		"message":  "Property updated successfully",
		"property": property,
	})
}

// SubmitPropertyForVerification submits a property for verification
func SubmitPropertyForVerification(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND organization_id = ?", propertyID, organization.ID).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check if property is in draft status
	if property.Status != "draft" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property must be in draft status to submit for verification"})
		return
	}

	// Update status to pending verification
	property.Status = "pending_verification"
	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to submit property for verification"})
		return
	}

	ctx.JSON(iris.Map{
		"message":  "Property submitted for verification successfully",
		"property": property,
	})
}

// AdminGetPropertySales gets all property sales (admin only)
func AdminGetPropertySales(ctx iris.Context) {
	var properties []models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Agent.User").Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	ctx.JSON(iris.Map{"properties": properties})
}

// AdminDeletePropertySale permanently deletes a property sale and dependent rows (admin only).
func AdminDeletePropertySale(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Unscoped().First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	if err := storage.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM property_sale_amenities WHERE property_sale_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.PropertyPlace{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.HiddenPropertySale{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.PropertySaleReport{}).Error; err != nil {
			return err
		}

		var videoIDs []uint
		if err := tx.Unscoped().Model(&models.PropertySaleVideo{}).Where("property_sale_id = ?", id).Pluck("id", &videoIDs).Error; err != nil {
			return err
		}

		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.NotificationDeliveryLog{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.NotificationEvent{}).Error; err != nil {
			return err
		}
		if len(videoIDs) > 0 {
			if err := tx.Unscoped().Where("video_sale_id IN ?", videoIDs).Delete(&models.NotificationEvent{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.Interaction{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("entity_type = ? AND entity_id = ?", models.EntityPropertySale, id).Delete(&models.Interaction{}).Error; err != nil {
			return err
		}
		if len(videoIDs) > 0 {
			if err := tx.Unscoped().Where("entity_type = ? AND entity_id IN ?", models.EntityPropertySaleVideo, videoIDs).Delete(&models.Interaction{}).Error; err != nil {
				return err
			}
		}

		if len(videoIDs) > 0 {
			if err := tx.Unscoped().Where("property_sale_video_id IN ?", videoIDs).Delete(&models.PropertySaleVideoLike{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("property_sale_video_id IN ?", videoIDs).Delete(&models.PropertySaleVideoSave{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("property_sale_video_id IN ?", videoIDs).Delete(&models.PropertySaleVideoReport{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("property_sale_video_id IN ?", videoIDs).Delete(&models.HiddenPropertySaleVideo{}).Error; err != nil {
				return err
			}
		}

		// Comments use property_sale_video_id = property_sales.id (synthetic); remove leaves first for parent_id self-FK.
		for {
			res := tx.Exec(`
				DELETE FROM property_sale_video_comments c
				WHERE c.property_sale_video_id = ?
				  AND c.id NOT IN (
					SELECT parent_id FROM property_sale_video_comments
					WHERE parent_id IS NOT NULL AND property_sale_video_id = ?
				  )`, id, id)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				break
			}
		}

		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.PropertySaleVideo{}).Error; err != nil {
			return err
		}

		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.PropertyTour{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_sale_id = ?", id).Delete(&models.PropertyInquiry{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("property_id = ?", id).Delete(&models.PropertyOffer{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&models.PropertySale{}, id).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("❌ AdminDeletePropertySale id=%d: %v", id, err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to delete property"})
		return
	}

	log.Printf("✅ Property sale %d permanently deleted by admin", id)
	ctx.JSON(iris.Map{"success": true, "message": "Property sale deleted successfully"})
}

// AdminDeactivatePropertySale deactivates a property sale (admin only) - hides from public feed
func AdminDeactivatePropertySale(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}
	var property models.PropertySale
	if err := storage.DB.First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	if err := storage.DB.Model(&property).Update("is_deactivated", true).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to deactivate property"})
		return
	}
	log.Printf("✅ Property sale %d deactivated by admin", id)

	// 🔴 INVALIDATE PROPERTY CACHE
	go func(propID uint) {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		propertySalesCache.InvalidatePropertyDetails(bgCtx, propID)
		propertySalesCache.InvalidatePropertySalesLists(bgCtx)
	}(id)

	ctx.JSON(iris.Map{"success": true, "message": "Property deactivated successfully"})
}

// AdminReactivatePropertySale reactivates a property sale (admin only) - shows in public feed
func AdminReactivatePropertySale(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}
	var property models.PropertySale
	if err := storage.DB.First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	if err := storage.DB.Model(&property).Update("is_deactivated", false).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to reactivate property"})
		return
	}
	log.Printf("✅ Property sale %d reactivated by admin", id)

	// 🔴 INVALIDATE PROPERTY CACHE
	go func(propID uint) {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		propertySalesCache.InvalidatePropertyDetails(bgCtx, propID)
		propertySalesCache.InvalidatePropertySalesLists(bgCtx)
	}(id)

	ctx.JSON(iris.Map{"success": true, "message": "Property reactivated successfully"})
}

// AdminMarkPropertySaleAsSold marks a sale listing as sold (admin only).
// Important: this does NOT auto-deactivate the listing.
func AdminMarkPropertySaleAsSold(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	if err := storage.DB.Model(&property).Updates(map[string]interface{}{
		"is_sold": true,
		"status":  "sold",
	}).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to mark property as sold"})
		return
	}

	log.Printf("✅ Property sale %d marked as sold by admin", id)

	go func(propID uint) {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		propertySalesCache.InvalidatePropertyDetails(bgCtx, propID)
		propertySalesCache.InvalidatePropertySalesLists(bgCtx)
	}(id)

	ctx.JSON(iris.Map{"success": true, "message": "Property marked as sold successfully"})
}

// AdminGetPropertySaleByID returns a single property sale by ID (admin only)
func AdminGetPropertySaleByID(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}
	var property models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Owner").Preload("Agent.User").First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	ctx.JSON(iris.Map{"property": property})
}

// AdminSetPropertySaleOrganization sets or clears the listing agency (organization_id). Admin only.
// Sending {"organization_id": null} clears the link. Clears agent_id when the organization changes
// so the listing does not keep an agent from a different brokerage.
func AdminSetPropertySaleOrganization(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var raw map[string]json.RawMessage
	if err := ctx.ReadJSON(&raw); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	v, ok := raw["organization_id"]
	if !ok {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "organization_id is required (use null to clear)"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	switch string(v) {
	case "null":
		property.OrganizationID = nil
		property.AgentID = nil
	default:
		var oid uint
		if err := json.Unmarshal(v, &oid); err != nil || oid == 0 {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "organization_id must be a positive integer or null"})
			return
		}
		var org models.Organization
		if err := storage.DB.First(&org, oid).Error; err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Organization not found"})
			return
		}
		property.OrganizationID = &oid
		property.AgentID = nil
	}

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update property organization"})
		return
	}

	go func(propID uint) {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		propertySalesCache.InvalidatePropertyDetails(bgCtx, propID)
		propertySalesCache.InvalidatePropertySalesLists(bgCtx)
	}(uint(id))

	var out models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Owner").Preload("Agent.User").First(&out, id).Error; err != nil {
		out = property
	}

	ctx.JSON(iris.Map{
		"success":  true,
		"message":  "Property organization updated",
		"property": out,
	})
}

// AdminUpdatePropertySale updates all property sale fields (admin only)
func AdminUpdatePropertySale(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}
	var property models.PropertySale
	if err := storage.DB.Preload("Organization").First(&property, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var input struct {
		Title           string                   `json:"title"`
		Description     string                   `json:"description"`
		PropertyType    string                   `json:"property_type"`
		Category        string                   `json:"category"`
		Address         string                   `json:"address"`
		City            string                   `json:"city"`
		CityID          *uint                    `json:"city_id"`
		ZoneID          *uint                    `json:"zone_id"`
		QuartierID      *uint                    `json:"quartier_id"`
		SubQuartierID   *uint                    `json:"sub_quartier_id"`
		State           string                   `json:"state"`
		Country         string                   `json:"country"`
		PostalCode      string                   `json:"postal_code"`
		Latitude        float64                  `json:"latitude"`
		Longitude       float64                  `json:"longitude"`
		Bedrooms        *int                     `json:"bedrooms"`
		Bathrooms       *int                     `json:"bathrooms"`
		SquareFootage   *int                     `json:"square_footage"`
		Area            *int                     `json:"area"`
		LotSize         *float64                 `json:"lot_size"`
		YearBuilt       *int                     `json:"year_built"`
		ParkingSpaces   *int                     `json:"parking_spaces"`
		ListingPrice    *float64                 `json:"listing_price"`
		Price           *float64                 `json:"price"`
		Currency        string                   `json:"currency"`
		PropertyTax     *float64                 `json:"property_tax"`
		HOA             *float64                 `json:"hoa"`
		Images          []string                 `json:"images"`
		Videos          []string                 `json:"videos"`
		VirtualTour     string                   `json:"virtual_tour"`
		FloorPlans      []models.FloorPlan       `json:"floor_plans"`
		Neighborhood    *models.NeighborhoodInfo `json:"neighborhood"`
		IndoorFeatures  []string                 `json:"indoor_features"`
		OutdoorFeatures []string                 `json:"outdoor_features"`
		Features        []string                 `json:"features"`
		Amenities       []string                 `json:"amenities"`
		AmenityIDs      []uint                   `json:"amenity_ids"`
		AgentID         *uint                    `json:"agent_id"`
		Status          string                   `json:"status"`
		IsInvestmentOpportunity *bool            `json:"is_investment_opportunity"`
		IsGold          *bool                    `json:"is_gold"`
		Truckeck        *bool                    `json:"truckeck"`
	}
	wasInvestment := property.IsInvestmentOpportunity

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	log.Printf("📋 AdminUpdatePropertySale: PATCH property %d received", id)
	if input.Truckeck != nil {
		log.Printf("   ✅ Truckeck in request: %v (will update)", *input.Truckeck)
	} else {
		log.Printf("   ⚠️ Truckeck NOT in request body (nil)")
	}

	titleChanged := false
	descChanged := false
	if input.Title != "" {
		property.Title = input.Title
		titleChanged = true
	}
	if input.Description != "" {
		property.Description = input.Description
		descChanged = true
	}
	if input.PropertyType != "" {
		property.PropertyType = input.PropertyType
	}
	if input.Category != "" {
		property.Category = input.Category
	}
	if input.Address != "" {
		property.Address = input.Address
	}
	if input.City != "" {
		property.City = input.City
	}
	if input.CityID != nil {
		property.CityID = input.CityID
	}
	if input.ZoneID != nil {
		property.ZoneID = input.ZoneID
	}
	if input.QuartierID != nil {
		property.QuartierID = input.QuartierID
	}
	if input.SubQuartierID != nil {
		property.SubQuartierID = input.SubQuartierID
	}
	if input.State != "" {
		property.State = input.State
	}
	if input.Country != "" {
		property.Country = input.Country
	}
	if input.PostalCode != "" {
		property.PostalCode = input.PostalCode
	}
	if input.Latitude != 0 {
		property.Latitude = input.Latitude
	}
	if input.Longitude != 0 {
		property.Longitude = input.Longitude
	}
	if input.Bedrooms != nil {
		property.Bedrooms = *input.Bedrooms
	}
	if input.Bathrooms != nil {
		property.Bathrooms = *input.Bathrooms
	}
	if input.SquareFootage != nil {
		property.SquareFootage = *input.SquareFootage
	} else if input.Area != nil {
		property.SquareFootage = *input.Area
	}
	if input.LotSize != nil {
		property.LotSize = *input.LotSize
	}
	if input.YearBuilt != nil {
		property.YearBuilt = *input.YearBuilt
	}
	if input.ParkingSpaces != nil {
		property.ParkingSpaces = *input.ParkingSpaces
	}
	if input.ListingPrice != nil {
		property.ListingPrice = *input.ListingPrice
		if property.SquareFootage > 0 {
			property.PricePerSqFt = *input.ListingPrice / float64(property.SquareFootage)
		}
	} else if input.Price != nil {
		property.ListingPrice = *input.Price
		if property.SquareFootage > 0 {
			property.PricePerSqFt = *input.Price / float64(property.SquareFootage)
		}
	}
	if input.Currency != "" {
		property.Currency = input.Currency
	}
	if input.PropertyTax != nil {
		property.PropertyTax = *input.PropertyTax
	}
	if input.HOA != nil {
		property.HOA = *input.HOA
	}
	if input.Images != nil {
		sanitized, err := storage.SanitizeHTTPMediaURLs("images", input.Images)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		property.Images = sanitized
	}
	if input.Videos != nil {
		sanitized, err := storage.SanitizeHTTPMediaURLs("videos", input.Videos)
		if err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
		property.Videos = sanitized
	}
	if input.VirtualTour != "" {
		property.VirtualTour = input.VirtualTour
	}
	if input.FloorPlans != nil {
		property.FloorPlans = input.FloorPlans
	}
	if input.Neighborhood != nil {
		property.Neighborhood = input.Neighborhood
	}
	if input.IndoorFeatures != nil || input.OutdoorFeatures != nil {
		var allFeatures []string
		if input.IndoorFeatures != nil {
			allFeatures = append(allFeatures, input.IndoorFeatures...)
		}
		if input.OutdoorFeatures != nil {
			allFeatures = append(allFeatures, input.OutdoorFeatures...)
		}
		property.Features = allFeatures
	} else if input.Features != nil {
		property.Features = input.Features
	}
	if input.AmenityIDs != nil && len(input.AmenityIDs) > 0 {
		var amenities []models.Amenity
		if err := storage.DB.Where("id IN ?", input.AmenityIDs).Find(&amenities).Error; err == nil {
			storage.DB.Model(&property).Association("AmenityList").Replace(amenities)
		}
		if input.Amenities != nil {
			property.Amenities = input.Amenities
		}
	} else if input.Amenities != nil {
		property.Amenities = input.Amenities
	}
	if input.AgentID != nil {
		property.AgentID = input.AgentID
	}
	if input.Status != "" {
		property.Status = input.Status
	}
	if input.IsInvestmentOpportunity != nil {
		property.IsInvestmentOpportunity = *input.IsInvestmentOpportunity
	}
	if input.IsGold != nil {
		property.IsGold = *input.IsGold
	}
	if input.Truckeck != nil {
		property.Truckeck = *input.Truckeck
		// Explicit Update to guarantee truckeck is persisted (GORM Save can skip zero values in some cases)
		storage.DB.Model(&models.PropertySale{}).Where("id = ?", id).Update("truckeck", *input.Truckeck)
	}

	if titleChanged {
		titleTranslations := services.TranslateAllLanguages(property.Title)
		if b, err := json.Marshal(titleTranslations); err == nil {
			property.TitleTranslations = b
		}
	}
	if descChanged {
		descTranslations := services.TranslateAllLanguages(property.Description)
		if b, err := json.Marshal(descTranslations); err == nil {
			property.DescriptionTranslations = b
		}
	}

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update property"})
		return
	}

	// Invalidate caches so mobile app and lists get fresh data
	go func(propID uint) {
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		propertySalesCache.InvalidatePropertyDetails(bgCtx, propID)
		propertySalesCache.InvalidatePropertySalesLists(bgCtx)
	}(uint(id))

	// Reload from DB to ensure response includes actual persisted state (e.g. truckeck)
	var refreshed models.PropertySale
	if err := storage.DB.First(&refreshed, id).Error; err == nil {
		property = refreshed
	}

	log.Printf("✅ Property sale %d updated by admin (truckeck=%v)", id, property.Truckeck)

	// Fire targeted investment alert when toggled ON by admin.
	if !wasInvestment && property.IsInvestmentOpportunity && property.IsPublished && !property.IsDeactivated && !property.IsSold {
		go triggerInvestmentOpportunityNotification(property)
	}

	ctx.JSON(iris.Map{"success": true, "message": "Property updated successfully", "property": property})
}

// AdminVerifyProperty verifies a property (admin only)
func AdminVerifyProperty(ctx iris.Context) {
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	uid := ctx.Values().Get("userID")
	var adminID uint
	if v, ok := uid.(uint); ok {
		adminID = v
	} else if v2, ok := uid.(int); ok {
		adminID = uint(v2)
	} else {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var input struct {
		IsVerified        bool   `json:"is_verified"`
		VerificationNotes string `json:"verification_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	property.IsVerified = input.IsVerified
	property.VerificationNotes = input.VerificationNotes
	property.VerifiedBy = &adminID

	if input.IsVerified {
		property.Status = "verified"
		property.VerifiedAt = &[]time.Time{time.Now()}[0]
	} else {
		property.Status = "draft"
		property.VerifiedAt = nil
	}

	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to verify property"})
		return
	}

	ctx.JSON(iris.Map{
		"message":  "Property verification updated successfully",
		"property": property,
	})
}

// PublishProperty publishes a verified property
func PublishProperty(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	propertyID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Check if user has an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err != nil {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "User must have an organization"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND organization_id = ?", propertyID, organization.ID).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	// Check if property is verified
	if !property.IsVerified {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property must be verified before publishing"})
		return
	}

	// Update status to published
	property.Status = "published"
	property.IsPublished = true
	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to publish property"})
		return
	}

	// Trigger notification to users with matching favorite city
	go triggerNewPropertyNotification(property)
	services.QueueSemanticIndex("sale", property.ID)
	go services.BatchNotifyInterestedUsers(context.Background(), property)

	ctx.JSON(iris.Map{
		"message":  "Property published successfully",
		"property": property,
	})
}

// AdminPublishProperty allows admins to publish a verified property sale regardless of ownership
func AdminPublishProperty(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil || id == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.Where("id = ?", id).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	if !property.IsVerified {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Property must be verified before publishing"})
		return
	}

	property.Status = "published"
	property.IsPublished = true
	if err := storage.DB.Save(&property).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to publish property"})
		return
	}

	// Trigger notification to users with matching favorite city
	go triggerNewPropertyNotification(property)

	ctx.JSON(iris.Map{"message": "Property published successfully", "property": property})
}

// applyCardFeedTrim reserved for future payload shaping — images/videos are URL strings only;
// full galleries are returned so carousel/detail can show every photo (client prefetches progressively).
func applyCardFeedTrim(_ []models.PropertySale) {}

// propertySaleGalleryURLs merges main images + classified room photos (deduped, stable order).
func propertySaleGalleryURLs(ps *models.PropertySale) []string {
	if ps == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(ps.Images)+8)
	add := func(urls []string) {
		for _, raw := range urls {
			u := strings.TrimSpace(raw)
			if u == "" {
				continue
			}
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	add(ps.Images)
	for _, cp := range ps.ClassifiedPhotos {
		add(cp.Photos)
	}
	return out
}

func expandPropertySaleGalleries(properties []models.PropertySale) {
	for i := range properties {
		merged := propertySaleGalleryURLs(&properties[i])
		if len(merged) > 0 {
			properties[i].Images = merged
		}
	}
	expandPropertySaleVideoRows(properties)
}

// GetPublishedProperties gets all published properties for public viewing
// Works for both authenticated and unauthenticated users
// Properties are rotated per-request to show fresh content (TikTok-like cycling)
// Properties with images are always prioritized at the top
func GetPublishedProperties(ctx iris.Context) {
	// Optional auth: extract userID from context or Authorization header
	var userID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
			fmt.Printf("🔍 GetPublishedProperties: User ID from context: %d\n", userID)
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			fmt.Printf("🔍 GetPublishedProperties: Parsing Authorization header\n")
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
					fmt.Printf("🔍 GetPublishedProperties: User ID from token: %d\n", userID)
				}
			}
		}
	}
	deviceID := strings.TrimSpace(ctx.GetHeader("X-Device-ID"))

	// 🔴 TRY REDIS CACHE FIRST
	page := ctx.URLParamIntDefault("page", 1)
	if page < 1 {
		page = 1
	}
	limit := ctx.URLParamIntDefault("limit", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// Check if there are any filters - if no filters, try cache
	hasFilters := ctx.URLParam("bedrooms") != "" || ctx.URLParam("bathrooms") != "" ||
		ctx.URLParam("year_built") != "" || ctx.URLParam("city_id") != "" ||
		ctx.URLParam("zone_id") != "" || ctx.URLParam("min_area") != "" ||
		ctx.URLParam("max_area") != "" || ctx.URLParam("quartier_id") != "" ||
		ctx.URLParam("min_price") != "" || ctx.URLParam("max_price") != "" ||
		ctx.URLParam("investment_opportunity") != "" || ctx.URLParam("property_type") != ""
	smartFeedMode := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("feed_mode", "legacy"))) == "smart"

	// Card feed omits heavy columns — default to card unless client asks for full payload.
	fieldsParam := strings.TrimSpace(ctx.URLParamDefault("fields", "card"))
	cardFields := fieldsParam != "full" && fieldsParam != "all"
	if !hasFilters && !smartFeedMode && page <= 3 && !cardFields { // Only cache first 3 pages, rest go to DB
		bgCtx := context.Background()
		cacheConfig := services.DefaultCacheConfig()
		cacheService := services.NewCacheService(storage.Redis)
		propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
		
		if cachedList, err := propertySalesCache.GetPropertySalesListFromCache(bgCtx, page, limit); err == nil && cachedList != nil && len(cachedList.Properties) > 0 {
			fmt.Printf("💾 PROPERTY LIST CACHE HIT - Page %d (instant response)\n", page)
			// Convert cached cards back to full PropertySale for response
			properties := make([]models.PropertySale, len(cachedList.Properties))
			for i, card := range cachedList.Properties {
				images := card.Images
				if images == nil {
					images = []string{}
				}
				videos := card.Videos
				if videos == nil {
					videos = []string{}
				}
				properties[i] = models.PropertySale{
					ID:           card.ID,
					Title:        card.Title,
					PropertyType: card.PropertyType,
					Address:      card.Address,
					City:         card.City,
					State:        card.State,
					Country:      card.Country,
					ListingPrice: card.Price,
					Bedrooms:     card.Bedrooms,
					Bathrooms:    card.Bathrooms,
					SquareFootage: int(card.Area),
					YearBuilt:    card.YearBuilt,
					Latitude:     card.Latitude,
					Longitude:    card.Longitude,
					Status:       card.Status,
					CreatedAt:    card.CreatedAt,
					Images:       images,
					Videos:       videos,
				}
			}
			hasMore := cachedList.NextCursor != ""
			ctx.JSON(iris.Map{
				"data": properties,
				"properties": properties,
				"hasMore": hasMore,
				"nextCursor": strconv.Itoa(page + 1),
				"meta": iris.Map{"total": cachedList.Total, "page": page, "limit": limit},
				"source": "cache",
			})
			return
		}
	}

	q := storage.DB.Model(&models.PropertySale{}).
		Where("property_sales.status = ? OR property_sales.is_published = ?", "published", true).
		// Keep normal hidden behavior for deactivated listings, except sold ones
		// which we intentionally show for traction/social-proof.
		Where("(property_sales.is_deactivated = ? OR property_sales.is_sold = ?)", false, true).
		Where("property_sales.deleted_at IS NULL")
	if cardFields {
		// Low-bandwidth feed: skip huge text columns; keep classified_photos for full gallery URLs.
		q = q.Omit(
			"Description", "DescriptionTranslations",
			"FloorPlans", "Neighborhood",
			"Features", "Amenities", "HostPrivateNote", "VerificationNotes", "VirtualTour",
		)
		q = q.Preload("Organization", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name", "phone", "email", "website", "banner_image", "logo", "owner_id")
		})
		q = q.Preload("Organization.Owner", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "first_name", "last_name", "avatar_url", "phone_number", "is_verified", "verification_status")
		})
		q = q.Preload("Owner", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "first_name", "last_name", "avatar_url", "phone_number", "is_verified", "verification_status")
		})
		q = q.Preload("Agent", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "user_id", "organization_id")
		})
	} else {
		q = q.Preload("Organization").
			Preload("Organization.Owner").
			Preload("Owner").
			Preload("Agent.User")
	}

	if userID > 0 {
		fmt.Printf("🔍 GetPublishedProperties: Applying filters for user ID: %d\n", userID)
		// Exclude items from blocked users (either the agent's user or the organization's owner)
		q = q.Joins("LEFT JOIN agents ON agents.id = property_sales.agent_id").
			Joins("LEFT JOIN organizations ON organizations.id = property_sales.organization_id").
			Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagger_id = ? AND uf.status = 'active' AND (uf.flagged_user_id = agents.user_id OR uf.flagged_user_id = organizations.owner_id))", userID)

		// Exclude hidden property sales
		q = q.Where("NOT EXISTS (SELECT 1 FROM hidden_property_sales hps WHERE hps.property_sale_id = property_sales.id AND hps.user_id = ? AND hps.deleted_at IS NULL)", userID)
		// Exclude properties from organizations explicitly blocked by the user
		q = q.Where("NOT EXISTS (SELECT 1 FROM user_blocked_organizations ubo WHERE ubo.user_id = ? AND ubo.organization_id = property_sales.organization_id AND ubo.status = 'active')", userID)
		fmt.Printf("🔍 GetPublishedProperties: Applied blocking and hiding filters\n")
	} else {
		fmt.Printf("🔍 GetPublishedProperties: No user ID, showing all property sales\n")
	}

	// Apply optional filters
	if v := ctx.URLParam("bedrooms"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q = q.Where("property_sales.bedrooms >= ?", n)
		}
	}
	if v := ctx.URLParam("bathrooms"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q = q.Where("property_sales.bathrooms >= ?", n)
		}
	}
	// Year built filter - minimum year built (properties built in this year or later)
	// IMPORTANT: Exclude properties with NULL or 0 year_built values
	if v := ctx.URLParam("year_built"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			fmt.Printf("🔍 GetPublishedProperties: Applying year_built filter: >= %d (excluding NULL/0 values)\n", n)
			q = q.Where("property_sales.year_built >= ? AND property_sales.year_built > 0", n)
		}
	}
	if v := ctx.URLParam("country_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.country_id = ?", n)
		}
	}
	// City filter
	if v := ctx.URLParam("city_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.city_id = ?", n)
		}
	}
	// Zone filter
	if v := ctx.URLParam("zone_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.zone_id = ?", n)
		}
	}
	// Area filter with range logic (e.g., if user selects 500, show 450-550 with priority to exactly 500)
	var areaPriorityValue *int // Store the target area for priority sorting
	if minAreaStr := ctx.URLParam("min_area"); minAreaStr != "" {
		if minArea, err := strconv.Atoi(minAreaStr); err == nil && minArea > 0 {
			if maxAreaStr := ctx.URLParam("max_area"); maxAreaStr != "" {
				// Both min and max provided - exact range filter
				if maxArea, err := strconv.Atoi(maxAreaStr); err == nil && maxArea > 0 {
					q = q.Where("property_sales.square_footage >= ? AND property_sales.square_footage <= ?", minArea, maxArea)
					fmt.Printf("🔍 GetPublishedProperties: Applying area filter: %d-%d m²\n", minArea, maxArea)
				}
			} else {
				// Only min provided - use range logic (10% buffer)
				// If user selects 500, show 450-550 (500 ± 50, which is 10% of 500)
				buffer := int(float64(minArea) * 0.1) // 10% buffer
				if buffer < 10 {
					buffer = 10 // Minimum 10 m² buffer
				}
				rangeMin := minArea - buffer
				rangeMax := minArea + buffer
				if rangeMin < 0 {
					rangeMin = 0
				}
				q = q.Where("property_sales.square_footage >= ? AND property_sales.square_footage <= ?", rangeMin, rangeMax)
				areaPriorityValue = &minArea // Store target for priority sorting
				fmt.Printf("🔍 GetPublishedProperties: Applying area filter with range logic: %d m² (range: %d-%d m²)\n", minArea, rangeMin, rangeMax)
			}
		}
	} else if maxAreaStr := ctx.URLParam("max_area"); maxAreaStr != "" {
		// Only max provided
		if maxArea, err := strconv.Atoi(maxAreaStr); err == nil && maxArea > 0 {
			q = q.Where("property_sales.square_footage <= ?", maxArea)
			fmt.Printf("🔍 GetPublishedProperties: Applying max area filter: <= %d m²\n", maxArea)
		}
	}
	// Quartier filter
	if v := ctx.URLParam("quartier_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q = q.Where("property_sales.quartier_id = ?", n)
			fmt.Printf("🔍 GetPublishedProperties: Applying quartier_id filter: %d\n", n)
		}
	}
	// Price range filter for property sales
	if v := ctx.URLParam("min_price"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			q = q.Where("property_sales.listing_price >= ?", n)
			fmt.Printf("🔍 GetPublishedProperties: Applying min_price filter: %.2f\n", n)
		}
	}
	if v := ctx.URLParam("max_price"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			q = q.Where("property_sales.listing_price <= ?", n)
			fmt.Printf("🔍 GetPublishedProperties: Applying max_price filter: %.2f\n", n)
		}
	}
	if v := strings.ToLower(strings.TrimSpace(ctx.URLParam("investment_opportunity"))); v == "1" || v == "true" || v == "yes" {
		q = q.Where("property_sales.is_investment_opportunity = ?", true)
		fmt.Printf("🔍 GetPublishedProperties: Applying investment_opportunity filter\n")
	}
	if v := strings.TrimSpace(ctx.URLParam("property_type")); v != "" && !strings.EqualFold(v, "all") {
		q = q.Where("LOWER(TRIM(property_sales.property_type)) = ?", strings.ToLower(v))
		fmt.Printf("🔍 GetPublishedProperties: Applying property_type filter: %s\n", v)
	}

	// DB-level pagination: limit=10, deterministic ORDER BY for instant response
	offset := (page - 1) * limit

	// Stable ordering: created_at DESC, id DESC (no per-request rotation - enables correct pagination)
	q = q.Order("property_sales.created_at DESC, property_sales.id DESC")
	if areaPriorityValue != nil {
		// Area priority: exact matches first, then by created_at
		q = q.Order(fmt.Sprintf("ABS(property_sales.square_footage - %d) ASC, property_sales.created_at DESC, property_sales.id DESC", *areaPriorityValue))
	}

	// Count total matching (before Offset/Limit)
	// Smart feed mode returns a fully ranked feed slice (device/user aware).
	if smartFeedMode {
		properties, totalCount, hasMore, nextCursor := buildSmartPropertyFeedPage(q, userID, deviceID, page, limit)
		lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
		for i := range properties {
			p := &properties[i]
			p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
			p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
		}
		redactPropertySaleSliceForViewer(properties, userID)
		if cardFields {
			expandPropertySaleGalleries(properties)
			applyCardFeedTrim(properties)
		}
		payload := iris.Map{
			"data":       properties,
			"properties": properties,
			"hasMore":    hasMore,
			"nextCursor": nextCursor,
			"meta":       iris.Map{"total": totalCount, "page": page, "limit": limit},
			"source":     "smart_feed",
		}
		if userID == 0 && page == 1 && !hasFilters && cardFields {
			go func(p iris.Map, lim int, language string) {
				bgCtx := context.Background()
				cacheSvc := services.NewCacheService(storage.Redis)
				key := services.FormatKey(services.PropertySalesSmartFeedAnonKey, lim, language)
				_ = cacheSvc.Set(bgCtx, key, p, 2*time.Minute)
			}(payload, limit, lang)
		}
		utils.RespondJSONWithETag(ctx, iris.StatusOK, payload)
		return
	}

	// Count total matching (before Offset/Limit)
	var totalCount int64
	if err := q.Session(&gorm.Session{}).Count(&totalCount).Error; err != nil {
		totalCount = int64(offset + limit + 1) // fallback: assume more pages
	}

	// Fetch only this page's rows - DB LIMIT/OFFSET for instant response
	var properties []models.PropertySale
	if err := q.Offset(offset).Limit(limit).Find(&properties).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch properties"})
		return
	}

	hasMore := int64(offset+len(properties)) < totalCount
	nextCursor := ""
	if hasMore {
		nextCursor = strconv.Itoa(page + 1)
	}

	fmt.Printf("✅ Returning %d property sales (page %d, rotated, hasMore=%v)\n", len(properties), page, hasMore)

	// Localize titles/descriptions based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range properties {
		p := &properties[i]
		p.Title = utils.ResolveLocalizedText(p.Title, p.TitleTranslations, lang)
		p.Description = utils.ResolveLocalizedText(p.Description, p.DescriptionTranslations, lang)
	}
	redactPropertySaleSliceForViewer(properties, userID)
	if cardFields {
		expandPropertySaleGalleries(properties)
		applyCardFeedTrim(properties)
	}

	// Return data + properties for frontend compatibility; frontend uses data|items|properties
	ctx.JSON(iris.Map{
		"data":       properties,
		"properties": properties,
		"hasMore":    hasMore,
		"nextCursor": nextCursor,
		"meta":       iris.Map{"total": totalCount, "page": page, "limit": limit},
		"source":     "database",
	})

	// 🔴 CACHE PROPERTIES FOR NEXT REQUEST (async, don't block response)
	if !hasFilters && page <= 3 {
		go func(props []models.PropertySale, pageNum int, lim int) {
			bgCtx := context.Background()
			cacheConfig := services.DefaultCacheConfig()
			cacheService := services.NewCacheService(storage.Redis)
			propertySalesCache := services.NewPropertySalesCacheService(cacheService, cacheConfig)
			nextCur := ""
			if hasMore {
				nextCur = strconv.Itoa(pageNum + 1)
			}
			if err := propertySalesCache.SetPropertySalesListCache(bgCtx, pageNum, lim, props, nextCur, totalCount); err != nil {
				fmt.Printf("⚠️ Failed to cache property sales list: %v\n", err)
			}
		}(properties, page, limit)
	}
}

type smartInterestProfile struct {
	TopCity        string
	TopZone        string
	TopType        string
	AvgPrice       float64
	HasProfile     bool
	InvestInterest bool // from device/user preference "investment"
}

// fetchPropertySalesPageNewestFirst paginates property sales newest-first (created_at DESC).
// Gold status does not affect ordering.
func fetchPropertySalesPageNewestFirst(q *gorm.DB, page, limit int) ([]models.PropertySale, int64, bool, string) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	var totalCount int64
	if err := q.Session(&gorm.Session{}).Count(&totalCount).Error; err != nil {
		totalCount = int64(offset + limit + 1)
	}

	var properties []models.PropertySale
	pageQ := q.Session(&gorm.Session{}).
		Order("property_sales.created_at DESC, property_sales.id DESC").
		Offset(offset).
		Limit(limit)
	if err := pageQ.Find(&properties).Error; err != nil {
		return []models.PropertySale{}, 0, false, ""
	}

	hasMore := int64(offset+len(properties)) < totalCount
	next := ""
	if hasMore {
		next = strconv.Itoa(page + 1)
	}
	return properties, totalCount, hasMore, next
}

func buildSmartPropertyFeedPage(q *gorm.DB, userID uint, deviceID string, page, limit int) ([]models.PropertySale, int64, bool, string) {
	// Smart feed currently uses the same newest-first ordering as legacy mode.
	// Gold badges remain on cards; ranking boosts can be reintroduced later without pinning gold.
	items, totalCount, hasMore, next := fetchPropertySalesPageNewestFirst(q, page, limit)
	if len(items) > 0 {
		go markPropertyFeedSeen(items, userID, deviceID)
	}
	return items, totalCount, hasMore, next
}

func loadLastSeenMap(userID uint, deviceID string, within time.Duration) map[uint]time.Time {
	cut := time.Now().Add(-within)
	q := storage.DB.Model(&models.PropertyFeedSeen{}).Where("seen_at >= ?", cut)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	} else if deviceID != "" {
		q = q.Where("device_id = ?", deviceID)
	}

	// Latest first so the first time we encounter a propertyID
	// is the most recent seen_at.
	var rows []models.PropertyFeedSeen
	_ = q.Order("seen_at DESC").Limit(5000).Find(&rows).Error

	out := make(map[uint]time.Time, len(rows))
	for _, r := range rows {
		if _, ok := out[r.PropertyID]; ok {
			continue
		}
		out[r.PropertyID] = r.SeenAt
	}
	return out
}

func loadInvestInterestFlag(userID uint, deviceID string) bool {
	if deviceID != "" {
		var ap models.AnonymousUserPreference
		if err := storage.DB.Where("device_id = ?", deviceID).First(&ap).Error; err == nil {
			for _, it := range ap.Interests {
				if strings.EqualFold(strings.TrimSpace(it), "investment") {
					return true
				}
			}
		}
	}
	if userID > 0 {
		var up models.UserProfile
		if err := storage.DB.Where("user_id = ?", userID).First(&up).Error; err == nil && up.Interests != nil {
			var interests []string
			_ = json.Unmarshal(up.Interests, &interests)
			for _, it := range interests {
				if strings.EqualFold(strings.TrimSpace(it), "investment") {
					return true
				}
			}
		}
	}
	return false
}

func loadSmartInterestProfile(userID uint, deviceID string) smartInterestProfile {
	invest := loadInvestInterestFlag(userID, deviceID)
	var behaviors []models.UserBehavior
	q := storage.DB.Model(&models.UserBehavior{}).Where("property_type = ?", "sale").Where("timestamp >= ?", time.Now().AddDate(0, 0, -60))
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	} else if deviceID != "" {
		q = q.Where("device_id = ?", deviceID)
	}
	_ = q.Order("timestamp DESC").Limit(1200).Find(&behaviors).Error
	if len(behaviors) == 0 {
		return smartInterestProfile{InvestInterest: invest}
	}

	// Batch-load property rows once. The previous implementation issued up to one
	// SELECT per behavior row (N+1), which could stall the public feed for tens
	// of seconds and make the mobile client sit on the skeleton until timeout.
	seenID := make(map[uint]struct{})
	orderedIDs := make([]uint, 0)
	const maxPropIDs = 500
	for _, b := range behaviors {
		if b.PropertyID == 0 {
			continue
		}
		if _, ok := seenID[b.PropertyID]; ok {
			continue
		}
		seenID[b.PropertyID] = struct{}{}
		orderedIDs = append(orderedIDs, b.PropertyID)
		if len(orderedIDs) >= maxPropIDs {
			break
		}
	}

	type priceTypeRow struct {
		ID           uint
		ListingPrice float64
		PropertyType string
	}
	propByID := make(map[uint]priceTypeRow, len(orderedIDs))
	if len(orderedIDs) > 0 {
		var rows []priceTypeRow
		_ = storage.DB.Model(&models.PropertySale{}).
			Select("id", "listing_price", "property_type").
			Where("id IN ?", orderedIDs).
			Find(&rows).Error
		for _, r := range rows {
			propByID[r.ID] = r
		}
	}

	cityCount := map[string]int{}
	zoneCount := map[string]int{}
	typeCount := map[string]int{}
	var priceSum float64
	var priceN int
	for _, b := range behaviors {
		if b.CityName != "" {
			cityCount[strings.ToLower(strings.TrimSpace(b.CityName))]++
		}
		if b.ZoneName != "" {
			zoneCount[strings.ToLower(strings.TrimSpace(b.ZoneName))]++
		}
		if p, ok := propByID[b.PropertyID]; ok {
			if p.PropertyType != "" {
				typeCount[strings.ToLower(strings.TrimSpace(p.PropertyType))]++
			}
			priceSum += p.ListingPrice
			priceN++
		}
	}
	return smartInterestProfile{
		TopCity:        topKey(cityCount),
		TopZone:        topKey(zoneCount),
		TopType:        topKey(typeCount),
		AvgPrice:       func() float64 { if priceN == 0 { return 0 }; return priceSum / float64(priceN) }(),
		HasProfile:     true,
		InvestInterest: invest,
	}
}

func topKey(m map[string]int) string {
	best := ""
	bestV := -1
	for k, v := range m {
		if v > bestV {
			bestV = v
			best = k
		}
	}
	return best
}

func smartPropertyScore(p models.PropertySale, profile smartInterestProfile) float64 {
	score := 0.0
	now := time.Now()
	age := now.Sub(p.CreatedAt)

	if profile.InvestInterest && p.IsInvestmentOpportunity {
		score += 20
	}

	if profile.TopCity != "" && strings.ToLower(strings.TrimSpace(p.City)) == profile.TopCity {
		score += 40
	}
	if profile.TopZone != "" {
		addr := strings.ToLower(strings.TrimSpace(p.Address))
		title := strings.ToLower(strings.TrimSpace(p.Title))
		if strings.Contains(addr, profile.TopZone) || strings.Contains(title, profile.TopZone) {
			score += 25
		}
	}
	if profile.TopType != "" && strings.Contains(strings.ToLower(strings.TrimSpace(p.PropertyType)), profile.TopType) {
		score += 30
	}
	if profile.AvgPrice > 0 {
		min := profile.AvgPrice * 0.8
		max := profile.AvgPrice * 1.2
		if p.ListingPrice >= min && p.ListingPrice <= max {
			score += 20
		}
	}
	if age <= 24*time.Hour {
		score += 20
	} else if age <= 7*24*time.Hour {
		score += 10
	}
	if p.ViewCount >= 500 {
		score += 15
	} else if p.ViewCount >= 100 {
		score += 10
	}
	return score
}

func smartDiversityMix(items []models.PropertySale, target int) []models.PropertySale {
	if target <= 0 || len(items) == 0 {
		return []models.PropertySale{}
	}
	now := time.Now()
	relevant := make([]models.PropertySale, 0)
	fresh := make([]models.PropertySale, 0)
	popular := make([]models.PropertySale, 0)
	randomPool := make([]models.PropertySale, 0)
	for _, p := range items {
		randomPool = append(randomPool, p)
		if now.Sub(p.CreatedAt) <= 7*24*time.Hour {
			fresh = append(fresh, p)
		}
		if p.ViewCount >= 100 {
			popular = append(popular, p)
		}
		relevant = append(relevant, p)
	}

	// already sorted by score before this function
	takeRelevant := int(float64(target) * 0.40)
	takeFresh := int(float64(target) * 0.25)
	takePopular := int(float64(target) * 0.10)
	takeRandom := maxInt(1, target-(takeRelevant+takeFresh+takePopular))

	out := make([]models.PropertySale, 0, target)
	used := map[uint]struct{}{}
	appendUnique := func(src []models.PropertySale, n int) {
		for _, p := range src {
			if len(out) >= target || n <= 0 {
				return
			}
			if _, ok := used[p.ID]; ok {
				continue
			}
			used[p.ID] = struct{}{}
			out = append(out, p)
			n--
		}
	}
	appendUnique(relevant, takeRelevant)
	appendUnique(fresh, takeFresh)
	appendUnique(popular, takePopular)
	// deterministic pseudo-random from ID parity + creation recency
	sort.SliceStable(randomPool, func(i, j int) bool {
		ai := int(randomPool[i].ID%17) + int(time.Since(randomPool[i].CreatedAt).Hours())%29
		aj := int(randomPool[j].ID%17) + int(time.Since(randomPool[j].CreatedAt).Hours())%29
		return ai < aj
	})
	appendUnique(randomPool, takeRandom)
	appendUnique(relevant, target-len(out))
	return out
}

func markPropertyFeedSeen(items []models.PropertySale, userID uint, deviceID string) {
	if len(items) == 0 {
		return
	}
	now := time.Now()
	records := make([]models.PropertyFeedSeen, 0, len(items))
	ids := make([]uint, 0, len(items))
	for _, p := range items {
		ids = append(ids, p.ID)
		rec := models.PropertyFeedSeen{
			PropertyID: p.ID,
			SeenAt:     now,
		}
		if userID > 0 {
			rec.UserID = &userID
		} else {
			rec.DeviceID = deviceID
		}
		records = append(records, rec)
	}
	// Single batch insert dramatically reduces per-row DB round trips.
	_ = storage.DB.
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&records).
		Error
	goldproperty.RecordFeedImpressionsBatch(ids)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GetPublishedProperty - Get single published property (public access)
// Returns 200 { data, property } or 404 { error } - consistent schema
func GetPublishedProperty(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}
	reqID := ctx.GetHeader("X-Request-ID")
	if reqID == "" {
		reqID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano()/1e6, id)
	}

	var property models.PropertySale
	if err := storage.DB.Preload("Organization").Preload("Organization.Owner").Preload("Agent.User").Preload("Owner").Preload("AmenityList").Where("id = ? AND (status = ? OR is_published = ?) AND (is_deactivated = ? OR is_sold = ?) AND deleted_at IS NULL", id, "published", true, false, true).First(&property).Error; err != nil {
		log.Printf("[GetPublishedProperty] NOT_FOUND requestId=%s propertyId=%d", reqID, id)
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}
	log.Printf("[GetPublishedProperty] OK requestId=%s propertyId=%d", reqID, id)

	// SECURITY: Track view count (exclude owner views)
	// Manipulate device/user identification to prevent owner view counting
	go func() {
		// Get user ID from context (if authenticated)
		userIDVal := ctx.Values().Get("userID")
		var viewerUserID uint
		if userIDVal != nil {
			if id, ok := userIDVal.(uint); ok {
				viewerUserID = id
			}
		}
		
		// Get device ID from header (for anonymous tracking)
		deviceID := ctx.GetHeader("X-Device-ID")
		phoneNumber := ctx.GetHeader("X-Phone-Number")
		
		// SECURITY: Check if viewer is the owner - if so, don't count view
		isOwner := false
		if property.OwnerID != nil && viewerUserID > 0 {
			if *property.OwnerID == viewerUserID {
				isOwner = true
			}
		}
		
		// Also check organization owner
		if !isOwner && property.Organization != nil && property.Organization.OwnerID > 0 && viewerUserID > 0 {
			if property.Organization.OwnerID == viewerUserID {
				isOwner = true
			}
		}
		
		// SECURITY: Manipulate device/phone identification for owner detection
		// If owner's phone number or device ID matches, don't count view
		if !isOwner && property.Owner != nil {
			if property.Owner.PhoneNumber != nil && phoneNumber != "" {
				if *property.Owner.PhoneNumber == phoneNumber {
					isOwner = true
				}
			}
		}
		
		// Only increment view count if not owner
		if !isOwner {
			// Increment view count
			storage.DB.Model(&property).UpdateColumn("view_count", gorm.Expr("view_count + ?", 1))
			if property.IsGold {
				goldproperty.RecordDetailView(property.ID)
			}

			// Daily charts in Host Studio read from interactions — mirror each counted view.
			var uidPtr *uint
			if viewerUserID > 0 {
				uidPtr = &viewerUserID
			}
			var devPtr *string
			if deviceID != "" {
				devPtr = &deviceID
			}
			services.InteractionServiceInstance().RecordPropertySaleView(property.ID, uidPtr, devPtr)
			
			// Get updated view count
			var newViewCount int64
			storage.DB.Model(&property).Select("view_count").Scan(&newViewCount)
			
			log.Printf("👁️ View count incremented for property %d (viewer: userID=%d, deviceID=%s, new count: %d)", property.ID, viewerUserID, func() string {
				if len(deviceID) > 10 {
					return deviceID[:10] + "..."
				}
				return deviceID
			}(), newViewCount)
			
			// Check if we hit a milestone (100, 200, 300, etc.)
			milestone := (newViewCount / 100) * 100
			previousMilestone := ((newViewCount - 1) / 100) * 100
			
			if milestone > 0 && milestone != previousMilestone && milestone > property.LastMilestoneNotified {
				// Hit a new milestone - notify host and viewers
				propertyImage := ""
				if len(property.Images) > 0 {
					propertyImage = property.Images[0]
				}
				
				// Notify host (async - don't block response)
				go func() {
					if err := services.NotificationServiceInstance.NotifyHostOnViewMilestone(
						property.ID,
						newViewCount,
						property.Title,
						propertyImage,
					); err != nil {
						log.Printf("❌ Failed to notify host on milestone: %v", err)
					}
				}()
				
				// Notify viewers (async)
				go func() {
					if err := services.NotificationServiceInstance.NotifyViewersOnViewMilestone(
						property.ID,
						newViewCount,
						property.Title,
						propertyImage,
					); err != nil {
						log.Printf("❌ Failed to notify viewers on milestone: %v", err)
					}
				}()
			}
		} else {
			log.Printf("🔒 Owner view detected for property %d - view not counted", property.ID)
		}
	}()

	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	property.Title = utils.ResolveLocalizedText(property.Title, property.TitleTranslations, lang)
	property.Description = utils.ResolveLocalizedText(property.Description, property.DescriptionTranslations, lang)

	// Expose amenity IDs for frontend mapping; keep legacy Amenities untouched
	if len(property.AmenityList) > 0 {
		ids := make([]uint, 0, len(property.AmenityList))
		for _, am := range property.AmenityList {
			if am.ID > 0 {
				ids = append(ids, uint(am.ID))
			}
		}
		property.AmenityIDs = ids
	}

	redactPropertySaleHostNote(&property, optionalAuthUserID(ctx))
	redactPropertySaleBrokerProfile(&property)

	if merged := propertySaleGalleryURLs(&property); len(merged) > 0 {
		property.Images = merged
	}

	// Consistent schema: 200 with { data: property } for clients; also { property } for backward compat
	ctx.Header("Cache-Control", "public, max-age=60, stale-while-revalidate=120")
	ctx.StatusCode(http.StatusOK)
	ctx.JSON(iris.Map{"data": property, "property": property})
}

// ReportPublishedPropertySale allows an authenticated user to report a published property sale
func ReportPublishedPropertySale(ctx iris.Context) {
	userIDVal := ctx.Values().Get("userID")
	if userIDVal == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)
	
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property ID"})
		return
	}

	// Ensure property is published
	var property models.PropertySale
	if err := storage.DB.Where("id = ? AND (status = ? OR is_published = ?) AND (is_deactivated = ? OR is_sold = ?) AND deleted_at IS NULL", id, "published", true, false, true).First(&property).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property not found"})
		return
	}

	var body struct {
		Reason      string `json:"reason"`
		Description string `json:"description"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	report := models.PropertySaleReport{
		ReporterID:     userID,
		PropertySaleID: property.ID,
		Reason:         body.Reason,
		Description:    body.Description,
	}
	if err := storage.DB.Create(&report).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create report"})
		return
	}

	ctx.JSON(iris.Map{"success": true})
}

// HidePropertySale allows an authenticated user to hide a property sale
func HidePropertySale(ctx iris.Context) {
	userIDInterface := ctx.Values().Get("userID")
	if userIDInterface == nil {
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Authentication required"})
		return
	}
	userID := userIDInterface.(uint)

	propertySaleID, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid property sale ID"})
		return
	}

	var input struct {
		Reason string `json:"reason" validate:"required"`
	}
	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Check if property sale exists and is published
	var propertySale models.PropertySale
	if err := storage.DB.Where("id = ? AND (status = ? OR is_published = ?)", propertySaleID, "published", true).First(&propertySale).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Property sale not found or not published"})
		return
	}

	// Check if already hidden by this user
	var existingHide models.HiddenPropertySale
	if err := storage.DB.Where("user_id = ? AND property_sale_id = ? AND deleted_at IS NULL", userID, propertySaleID).First(&existingHide).Error; err == nil {
		ctx.JSON(iris.Map{"success": true, "message": "Property sale already hidden"})
		return
	}

	hide := models.HiddenPropertySale{
		UserID:         &userID,
		PropertySaleID: propertySaleID,
		Reason:         input.Reason,
	}

	if err := storage.DB.Create(&hide).Error; err != nil {
		fmt.Printf("❌ Error hiding property sale: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to hide property sale"})
		return
	}

	fmt.Printf("✅ Property sale %d hidden by user %d\n", propertySaleID, userID)
	ctx.JSON(iris.Map{"success": true, "message": "Property sale hidden successfully"})
}
