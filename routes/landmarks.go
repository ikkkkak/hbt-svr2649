package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/services/videoprocessing"
	"apartments-clone-server/storage"
	"apartments-clone-server/utils"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	jwt "github.com/kataras/iris/v12/middleware/jwt"
)

// landmarkPublisherContactForAPI builds guest-facing publisher contact (organization or individual owner).
func landmarkPublisherContactForAPI(lm *models.Landmark) *models.LandmarkPublisherContact {
	if lm == nil {
		return nil
	}
	if lm.Organization != nil {
		return &models.LandmarkPublisherContact{
			Type:    "organization",
			Name:    lm.Organization.Name,
			Phone:   lm.Organization.Phone,
			Email:   lm.Organization.Email,
			Website: lm.Organization.Website,
		}
	}
	if lm.Owner != nil {
		name := strings.TrimSpace(fmt.Sprintf("%s %s", lm.Owner.FirstName, lm.Owner.LastName))
		if name == "" {
			name = lm.Owner.Email
		}
		phone := ""
		if lm.Owner.PhoneNumber != nil {
			phone = strings.TrimSpace(*lm.Owner.PhoneNumber)
		}
		return &models.LandmarkPublisherContact{
			Type:  "individual",
			Name:  name,
			Phone: phone,
			Email: lm.Owner.Email,
		}
	}
	return nil
}

// CreateLandmark creates a new landmark for an organization or individual owner
func CreateLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// SECURITY: Check if user is a member of an organization
	// If they are, ALL lands MUST belong to that organization
	var organizationID *uint

	// First check if user is owner of an organization
	var organization models.Organization
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		// User owns an organization, use it
		organizationID = &organization.ID
	} else {
		// Check if user is a member (not owner) of an organization
		var member models.OrganizationMember
		if err := storage.DB.Where("user_id = ? AND status = ? AND is_active = ?", userID, "active", true).
			Preload("Organization").
			First(&member).Error; err == nil {
			// User is a member - MUST assign to organization
			organizationID = &member.OrganizationID
			log.Printf("🔒 Security: User %d is a member of organization %d - land will be assigned to organization", userID, member.OrganizationID)
		} else {
			// User is not a member - allow creating as individual owner
			organizationID = nil
		}
	}

	var input struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Images      []string `json:"images"`
		VideoURL    *string  `json:"video_url"`
		Area        float64  `json:"area"`
		AreaUnit    string   `json:"area_unit"`
		LandType    string   `json:"land_type"`
		Zoning      string   `json:"zoning"`
		Utilities   []string `json:"utilities"`
		// Optional: only required when highlight_location=true
		HighlightLocation bool     `json:"highlight_location"`
		Point1Lat         *float64 `json:"point1_lat"`
		Point1Lng         *float64 `json:"point1_lng"`
		Point2Lat         *float64 `json:"point2_lat"`
		Point2Lng         *float64 `json:"point2_lng"`
		Point3Lat         *float64 `json:"point3_lat"`
		Point3Lng         *float64 `json:"point3_lng"`
		Point4Lat         *float64 `json:"point4_lat"`
		Point4Lng         *float64 `json:"point4_lng"`
		PropertyPapers    []string `json:"property_papers"`
		PaperTypes        []string `json:"paper_types"`
		// Structured location (required for new listings)
		CityID     *uint `json:"city_id"`
		ZoneID     *uint `json:"zone_id"`
		QuartierID *uint `json:"quartier_id"`
		// Cadastre match from Habitat GIS
		HabitatPlotID *uint `json:"habitat_plot_id"`
		PlotConfirmed bool  `json:"plot_confirmed"`
		// Display labels (kept for search/backward compatibility)
		District        string   `json:"district"`
		Region          string   `json:"region"`
		PlotNumber      string   `json:"plot_number"`
		ElevationMeters float64  `json:"elevation_m"`
		Sides           []string `json:"sides"`
		Price           float64  `json:"price"`
		Currency        string   `json:"currency"`
		Lots            *int     `json:"lots"`

		// Optional host-only note (never shown to guests on public reads)
		HostPrivateNote string `json:"host_private_note"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Validate required fields
	if input.Title == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Title is required"})
		return
	}
	if input.CityID == nil || *input.CityID == 0 ||
		input.ZoneID == nil || *input.ZoneID == 0 ||
		input.QuartierID == nil || *input.QuartierID == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "City, zone, and sector are required"})
		return
	}
	if strings.TrimSpace(input.PlotNumber) == "" {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Plot number is required"})
		return
	}
	if !input.PlotConfirmed {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Please confirm your plot details before publishing"})
		return
	}
	if err := validateLandmarkLocationIDs(*input.CityID, *input.ZoneID, *input.QuartierID); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": err.Error()})
		return
	}
	if input.HabitatPlotID != nil && *input.HabitatPlotID > 0 {
		if err := validateLandmarkHabitatPlot(input.HabitatPlotID, input.PlotNumber, *input.QuartierID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
	}
	// At least one of images or video is required
	if len(input.Images) == 0 && (input.VideoURL == nil || *input.VideoURL == "") {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "You must upload at least one image or one video"})
		return
	}
	// Determine media_type
	mediaType := "images"
	if len(input.Images) > 0 && input.VideoURL != nil && *input.VideoURL != "" {
		mediaType = "both"
	} else if input.VideoURL != nil && *input.VideoURL != "" {
		mediaType = "video"
	}

	if input.HighlightLocation {
		// Require all points when user chooses to highlight location
		if input.Point1Lat == nil || input.Point1Lng == nil ||
			input.Point2Lat == nil || input.Point2Lng == nil ||
			input.Point3Lat == nil || input.Point3Lng == nil ||
			input.Point4Lat == nil || input.Point4Lng == nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "All coordinate points are required"})
			return
		}

		// Validate coordinates are within reasonable bounds
		if !isValidCoordinate(*input.Point1Lat, *input.Point1Lng) ||
			!isValidCoordinate(*input.Point2Lat, *input.Point2Lng) ||
			!isValidCoordinate(*input.Point3Lat, *input.Point3Lng) ||
			!isValidCoordinate(*input.Point4Lat, *input.Point4Lng) {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": "Invalid coordinates"})
			return
		}
	} else {
		// If user skipped highlight, ensure we don't accidentally store partially-provided points
		input.Point1Lat, input.Point1Lng = nil, nil
		input.Point2Lat, input.Point2Lng = nil, nil
		input.Point3Lat, input.Point3Lng = nil, nil
		input.Point4Lat, input.Point4Lng = nil, nil
	}

	// Convert arrays to JSON
	filteredImages := filterHTTPMediaURLs(input.Images)
	if len(input.Images) > 0 && len(filteredImages) == 0 && (input.VideoURL == nil || strings.TrimSpace(*input.VideoURL) == "") {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Images must be uploaded to the server (local file paths are not allowed)"})
		return
	}
	imagesJSON, _ := json.Marshal(filteredImages)
	utilitiesJSON, _ := json.Marshal(input.Utilities)
	papersJSON, _ := json.Marshal(input.PropertyPapers)
	sidesJSON, _ := json.Marshal(input.Sides)

	// Ensure owner_id is always set (use pointer to uint)
	ownerIDPtr := &userID
	videoURL := (*string)(nil)
	if input.VideoURL != nil && *input.VideoURL != "" {
		videoURL = input.VideoURL
		log.Printf("📹 CreateLandmark: storing video_url=%s", *videoURL)
	}
	landmark := models.Landmark{
		OrganizationID: organizationID, // Can be nil for individual owners
		OwnerID:        ownerIDPtr,     // ALWAYS set owner_id to track individual owner
		Title:          input.Title,
		Description:    input.Description,
		Images:         imagesJSON,
		VideoURL:       videoURL,
		MediaType:      mediaType,
		Area:           input.Area,
		AreaUnit:       input.AreaUnit,
		LandType:       input.LandType,
		Zoning:         input.Zoning,
		Utilities:      utilitiesJSON,
		Lots:           input.Lots,
		Point1Lat:      input.Point1Lat,
		Point1Lng:      input.Point1Lng,
		Point2Lat:      input.Point2Lat,
		Point2Lng:      input.Point2Lng,
		Point3Lat:      input.Point3Lat,
		Point3Lng:      input.Point3Lng,
		Point4Lat:      input.Point4Lat,
		Point4Lng:      input.Point4Lng,
		PropertyPapers: papersJSON,
		PaperTypes:     input.PaperTypes,
		CityID:         input.CityID,
		ZoneID:         input.ZoneID,
		QuartierID:     input.QuartierID,
		HabitatPlotID:  input.HabitatPlotID,
		PlotConfirmed:  input.PlotConfirmed,
		// Location labels
		District:        input.District,
		Region:          input.Region,
		PlotNumber:      strings.TrimSpace(input.PlotNumber),
		ElevationMeters: input.ElevationMeters,
		Sides:           sidesJSON,
		Price:           input.Price,
		Currency:        input.Currency,
		Status:          "draft",
		IsPublished:     false,
		IsVerified:      false,
		HostPrivateNote: sanitizeHostPrivateNote(input.HostPrivateNote),
	}

	// One-time translations for landmark title & description
	titleTranslations := services.TranslateAllLanguages(input.Title)
	descTranslations := services.TranslateAllLanguages(input.Description)
	if b, err := json.Marshal(titleTranslations); err == nil {
		landmark.TitleTranslations = b
	}
	if b, err := json.Marshal(descTranslations); err == nil {
		landmark.DescriptionTranslations = b
	}

	if err := storage.DB.Create(&landmark).Error; err != nil {
		log.Printf("❌ CreateLandmark DB error: %v (video_url=%v)", err, videoURL != nil)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create landmark"})
		return
	}

	landCity := strings.TrimSpace(landmark.District)
	if landCity == "" {
		landCity = strings.TrimSpace(landmark.Region)
	}
	services.NotifyAdminNewListing(services.ListingAdminNotifyInput{
		Kind:         services.ListingKindLand,
		ID:           landmark.ID,
		Title:        landmark.Title,
		City:         landCity,
		Price:        landmark.Price,
		Currency:     landmark.Currency,
		PropertyType: landmark.LandType,
		HostUserID:   userID,
		Status:       landmark.Status,
	})

	if videoURL != nil {
		log.Printf("✅ CreateLandmark: saved landmark id=%d with video_url", landmark.ID)
	}

	landmarkID := landmark.ID
	imagesCopy := append([]string(nil), filteredImages...)
	hasUploadedVideo := videoURL != nil && strings.TrimSpace(*videoURL) != ""
	go func(lid, uid uint, imgs []string, hadVideo bool) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("⚠️ Panic in CreateLandmark post-create id=%d: %v", lid, r)
			}
		}()
		if !hadVideo && len(imgs) > 0 {
			if _, err := videoprocessing.EnqueueLandmarkSlideshow(storage.DB, lid, uid); err != nil {
				log.Printf("⚠️ slideshow enqueue land=%d: %v", lid, err)
			}
		}
	}(landmarkID, userID, imagesCopy, hasUploadedVideo)

	ctx.StatusCode(http.StatusCreated)
	ctx.JSON(landmark)
}

// GetOrganizationLandmarks gets all landmarks for a user (organization or individual)
func GetOrganizationLandmarks(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)

	// Check if user has an organization (as owner or member)
	var organization models.Organization
	var hasOrganization bool

	// First check if user is owner
	if err := storage.DB.Where("owner_id = ?", userID).First(&organization).Error; err == nil {
		hasOrganization = true
	} else {
		// Check if user is a member using helper function
		org, _, _ := services.GetUserOrganization(userID)
		if org != nil {
			organization = *org
			hasOrganization = true
		}
	}

	var landmarks []models.Landmark
	query := storage.DB.Preload("Organization").Preload("Owner")

	if hasOrganization {
		// Fetch landmarks from organization OR individual landmarks owned by user
		query = query.Where(
			"organization_id = ? OR (organization_id IS NULL AND owner_id = ?)",
			organization.ID, userID,
		)
	} else {
		// User has no organization - fetch only individual landmarks
		query = query.Where("organization_id IS NULL AND owner_id = ?", userID)
	}

	if err := query.Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	// Localize landmark fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range landmarks {
		l := &landmarks[i]
		l.Title = utils.ResolveLocalizedText(l.Title, l.TitleTranslations, lang)
		l.Description = utils.ResolveLocalizedText(l.Description, l.DescriptionTranslations, lang)
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// GetLandmarkByID returns a single landmark by ID with host info (organization or owner)
func GetLandmarkByID(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	var landmark models.Landmark
	if err := storage.DB.Preload("Organization").Preload("Organization.Owner").Preload("Owner").
		Where("id = ? AND is_verified = ? AND is_published = ? AND status = ?", id, true, true, "verified").
		First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}
	landmark.Title = utils.ResolveLocalizedText(landmark.Title, landmark.TitleTranslations, lang)
	landmark.Description = utils.ResolveLocalizedText(landmark.Description, landmark.DescriptionTranslations, lang)
	redactLandmarkHostNote(&landmark, optionalAuthUserID(ctx))
	redactLandmarkBrokerProfile(&landmark)
	// Build host info: organization (if any) or individual owner - uniform structure
	hostInfo := map[string]interface{}{
		"type":  "organization",
		"name":  "",
		"phone": "",
		"email": "",
	}
	if landmark.Organization != nil {
		hostInfo["type"] = "organization"
		hostInfo["name"] = landmark.Organization.Name
		hostInfo["phone"] = landmark.Organization.Phone
		hostInfo["email"] = landmark.Organization.Email
		hostInfo["website"] = landmark.Organization.Website
	} else if landmark.Owner != nil {
		hostInfo["type"] = "individual"
		hostInfo["name"] = strings.TrimSpace(fmt.Sprintf("%s %s", landmark.Owner.FirstName, landmark.Owner.LastName))
		if hostInfo["name"] == "" {
			hostInfo["name"] = landmark.Owner.Email
		}
		if landmark.Owner.PhoneNumber != nil && *landmark.Owner.PhoneNumber != "" {
			hostInfo["phone"] = *landmark.Owner.PhoneNumber
		}
		hostInfo["email"] = landmark.Owner.Email
	}
	landmark.PublisherContact = landmarkPublisherContactForAPI(&landmark)
	response := iris.Map{
		"landmark": landmark,
		"host":     hostInfo,
	}
	ctx.JSON(response)
}

// GetPublicLandmarks gets all verified and published landmarks for public display
// Works for both authenticated and unauthenticated users
// Landmarks are rotated per-request to show fresh content (TikTok-like cycling)
// Landmarks with images are always prioritized at the top
func GetPublicLandmarks(ctx iris.Context) {
	// Optional auth: extract userID
	var userID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
				}
			}
		}
	}

	q := storage.DB.Model(&models.Landmark{}).
		Preload("Organization").
		Preload("Owner").
		Where("landmarks.is_verified = ? AND landmarks.is_published = ? AND landmarks.status = ?", true, true, "verified")

	// Join organizations for city, zone, quartier filters and user blocking check
	hasCityFilter := ctx.URLParam("city") != "" || ctx.URLParam("city_id") != ""
	hasZoneFilter := ctx.URLParam("zone_id") != ""
	hasQuartierFilter := ctx.URLParam("quartier_id") != ""
	hasUserBlocking := userID > 0
	if hasCityFilter || hasZoneFilter || hasQuartierFilter || hasUserBlocking {
		q = q.Joins("LEFT JOIN organizations ON organizations.id = landmarks.organization_id")
	}

	// Filtering: Price range
	if minPrice := ctx.URLParamFloat64Default("min_price", 0); minPrice > 0 {
		q = q.Where("landmarks.price >= ?", minPrice)
	}
	if maxPrice := ctx.URLParamFloat64Default("max_price", 0); maxPrice > 0 {
		q = q.Where("landmarks.price <= ?", maxPrice)
	}
	if minArea := ctx.URLParamIntDefault("min_area", 0); minArea > 0 {
		if maxArea := ctx.URLParamIntDefault("max_area", 0); maxArea > 0 {
			q = q.Where("landmarks.area >= ? AND landmarks.area <= ?", minArea, maxArea)
		} else {
			buffer := int(float64(minArea) * 0.1)
			if buffer < 10 {
				buffer = 10
			}
			rangeMin := minArea - buffer
			if rangeMin < 0 {
				rangeMin = 0
			}
			rangeMax := minArea + buffer
			q = q.Where("landmarks.area >= ? AND landmarks.area <= ?", rangeMin, rangeMax)
		}
	} else if maxArea := ctx.URLParamIntDefault("max_area", 0); maxArea > 0 {
		q = q.Where("landmarks.area <= ?", maxArea)
	}
	if v := strings.ToLower(strings.TrimSpace(ctx.URLParam("investment_opportunity"))); v == "1" || v == "true" || v == "yes" {
		q = q.Where("landmarks.is_investment_opportunity = ?", true)
	}
	// Filtering: City (from organization) - support both city name and city_id
	if cityName := ctx.URLParam("city"); cityName != "" {
		q = q.Where("organizations.city = ?", cityName)
	}
	if cityID := ctx.URLParamIntDefault("city_id", 0); cityID > 0 {
		var city models.City
		if err := storage.DB.First(&city, uint(cityID)).Error; err == nil {
			q = q.Where(
				"(landmarks.city_id = ? OR (landmarks.city_id IS NULL AND (LOWER(organizations.city) = LOWER(?) OR LOWER(organizations.city) = LOWER(?))))",
				uint(cityID),
				city.Name,
				city.NameAr,
			)
		} else {
			q = q.Where("1 = 0")
		}
	}
	// Filtering: Zone
	if zoneID := ctx.URLParamIntDefault("zone_id", 0); zoneID > 0 {
		var zone models.Zone
		if err := storage.DB.First(&zone, uint(zoneID)).Error; err == nil {
			q = q.Where(
				"(landmarks.zone_id = ? OR (landmarks.zone_id IS NULL AND (LOWER(landmarks.district) = LOWER(?) OR LOWER(landmarks.region) = LOWER(?) OR LOWER(organizations.state) = LOWER(?) OR LOWER(landmarks.district) = LOWER(?) OR LOWER(landmarks.region) = LOWER(?) OR LOWER(organizations.state) = LOWER(?))))",
				uint(zoneID),
				zone.Name,
				zone.Name,
				zone.Name,
				zone.NameAr,
				zone.NameAr,
				zone.NameAr,
			)
		} else {
			q = q.Where("1 = 0")
		}
	}
	// Filtering: Quartier / sector
	if quartierID := ctx.URLParamIntDefault("quartier_id", 0); quartierID > 0 {
		var quartier models.Quartier
		if err := storage.DB.First(&quartier, uint(quartierID)).Error; err == nil {
			q = q.Where(
				"(landmarks.quartier_id = ? OR (landmarks.quartier_id IS NULL AND (LOWER(landmarks.region) = LOWER(?) OR LOWER(landmarks.district) = LOWER(?) OR LOWER(landmarks.region) = LOWER(?) OR LOWER(landmarks.district) = LOWER(?))))",
				uint(quartierID),
				quartier.Name,
				quartier.Name,
				quartier.NameAr,
				quartier.NameAr,
			)
		} else {
			q = q.Where("1 = 0")
		}
	}
	// Legacy: Zone/District (keep for backward compatibility)
	if district := ctx.URLParam("district"); district != "" {
		q = q.Where("landmarks.district = ?", district)
	}
	if region := ctx.URLParam("region"); region != "" {
		q = q.Where("landmarks.region = ?", region)
	}

	if hasUserBlocking {
		// Exclude landmarks from blocked organizations' owners
		q = q.Where("NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagger_id = ? AND uf.status = 'active' AND uf.flagged_user_id = organizations.owner_id)", userID)
	}

	var allLandmarks []models.Landmark
	if err := q.Find(&allLandmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	fmt.Printf("✅ Found %d landmarks before rotation\n", len(allLandmarks))

	// Separate landmarks with images from those without
	var withImages []models.Landmark
	var withoutImages []models.Landmark

	for _, landmark := range allLandmarks {
		// Check if landmark has images (Images is stored as datatypes.JSON which is []byte)
		hasImages := false
		if len(landmark.Images) > 0 {
			var images []string
			if err := json.Unmarshal(landmark.Images, &images); err == nil {
				// Check if images array contains non-empty strings
				for _, img := range images {
					if strings.TrimSpace(img) != "" {
						hasImages = true
						break
					}
				}
			}
		}

		if hasImages {
			withImages = append(withImages, landmark)
		} else {
			withoutImages = append(withoutImages, landmark)
		}
	}

	fmt.Printf("📸 Landmarks with images: %d, without images: %d\n", len(withImages), len(withoutImages))

	// Gold-aware ranking: boost gold visibility while preserving discovery variety.
	now := time.Now()
	rand.Seed(now.UnixNano())
	scoreLandmarks := func(items []models.Landmark) []models.Landmark {
		if len(items) <= 1 {
			return items
		}
		scored := make([]struct {
			landmark models.Landmark
			score    float64
		}, 0, len(items))
		for _, lm := range items {
			ageHours := now.Sub(lm.CreatedAt).Hours()
			if ageHours < 0 {
				ageHours = 0
			}
			// Freshness decays over ~30 days
			freshness := 1.0 - (ageHours / (24.0 * 30.0))
			if freshness < 0 {
				freshness = 0
			}
			if freshness > 1 {
				freshness = 1
			}

			// Gold gets a significant average lift with jitter so it's not always first.
			goldBoost := 0.0
			if lm.IsGold {
				goldBoost = 0.28 + rand.Float64()*0.30 // 0.28 - 0.58
			}
			score := (0.45 * freshness) + (0.35 * rand.Float64()) + (0.20 * goldBoost)
			scored = append(scored, struct {
				landmark models.Landmark
				score    float64
			}{landmark: lm, score: score})
		}
		sort.SliceStable(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})
		ordered := make([]models.Landmark, 0, len(scored))
		for _, s := range scored {
			ordered = append(ordered, s.landmark)
		}
		return ordered
	}

	withImages = scoreLandmarks(withImages)
	withoutImages = scoreLandmarks(withoutImages)

	// Combine: landmarks with images first, then without
	var landmarks []models.Landmark
	landmarks = append(landmarks, withImages...)
	landmarks = append(landmarks, withoutImages...)

	fmt.Printf("✅ Returning %d landmarks (rotated, images prioritized)\n", len(landmarks))

	// Localize landmark fields based on requested language
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))
	for i := range landmarks {
		l := &landmarks[i]
		l.Title = utils.ResolveLocalizedText(l.Title, l.TitleTranslations, lang)
		l.Description = utils.ResolveLocalizedText(l.Description, l.DescriptionTranslations, lang)
	}
	redactLandmarkSliceForViewer(landmarks, userID)
	for i := range landmarks {
		landmarks[i].PublisherContact = landmarkPublisherContactForAPI(&landmarks[i])
	}

	ctx.JSON(iris.Map{"landmarks": landmarks})
}

// GetLandmarkVideosFeed returns paginated landmark videos for the video feed tab
// Only verified/published landmarks with video_url are returned
// Format compatible with VideoFeedScreen (PropertySaleVideo-like structure)
func GetLandmarkVideosFeed(ctx iris.Context) {
	var userID uint = 0
	if v := ctx.Values().Get("userID"); v != nil {
		if id, ok := v.(uint); ok {
			userID = id
		}
	}
	if userID == 0 {
		if auth := ctx.GetHeader("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			verifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
			verifier.WithDefaultBlocklist()
			if token, err := verifier.VerifyToken([]byte(auth[7:])); err == nil {
				var claims utils.AccessToken
				if err := token.Claims(&claims); err == nil {
					userID = claims.ID
				}
			}
		}
	}

	page := 1
	if cursor := ctx.URLParam("cursor"); cursor != "" {
		if p, err := strconv.Atoi(cursor); err == nil && p > 0 {
			page = p
		}
	} else {
		page = ctx.URLParamIntDefault("page", 1)
	}
	limit := ctx.URLParamIntDefault("limit", 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	offset := (page - 1) * limit
	lang := strings.ToLower(strings.TrimSpace(ctx.URLParamDefault("lang", "en")))

	if offset == 0 {
		go func() {
			videoprocessing.ReconcileLandmarkSlideshowJobs(storage.DB)
			videoprocessing.EnqueueMissingLandmarkSlideshowsBatch(storage.DB, 25)
		}()
	}

	// Verified + published lands with a video (column or completed auto-slideshow job).
	q := storage.DB.Model(&models.Landmark{}).
		Preload("Organization").
		Preload("Owner").
		Where("landmarks.is_verified = ? AND landmarks.is_published = ?", true, true).
		Where(`(
			(landmarks.video_url IS NOT NULL AND TRIM(landmarks.video_url) <> '')
			OR EXISTS (
				SELECT 1 FROM property_video_generation_jobs j
				WHERE j.deleted_at IS NULL
					AND j.entity_type = 'land'
					AND j.entity_id = landmarks.id
					AND j.status = 'completed'
					AND TRIM(j.output_video_url) <> ''
			)
		)`)

	if userID > 0 {
		q = q.Joins("LEFT JOIN organizations ON organizations.id = landmarks.organization_id").
			Where("(organizations.id IS NULL OR NOT EXISTS (SELECT 1 FROM user_flags uf WHERE uf.flagger_id = ? AND uf.status = 'active' AND uf.flagged_user_id = organizations.owner_id))", userID)
	}
	// Price filter
	if minPrice := ctx.URLParamFloat64Default("min_price", 0); minPrice > 0 {
		q = q.Where("landmarks.price >= ?", minPrice)
	}
	if maxPrice := ctx.URLParamFloat64Default("max_price", 0); maxPrice > 0 {
		q = q.Where("landmarks.price <= ?", maxPrice)
	}
	// Zone/district filter
	if district := strings.TrimSpace(ctx.URLParam("district")); district != "" {
		q = q.Where("LOWER(landmarks.district) = LOWER(?)", district)
	}
	// Region (city) filter
	if region := strings.TrimSpace(ctx.URLParam("region")); region != "" {
		q = q.Where("LOWER(landmarks.region) = LOWER(?)", region)
	}

	// Hybrid ranking: freshness + random + gold boost (boosted but non-deterministic)
	var landmarks []models.Landmark
	if err := q.Order(`(
		0.45 * GREATEST(0, 1.0 - EXTRACT(EPOCH FROM (NOW() - landmarks.created_at)) / 86400.0 / 30.0) +
		0.35 * random() +
		0.20 * CASE WHEN landmarks.is_gold = true THEN (0.70 + random() * 0.30) ELSE random() * 0.35 END
	) DESC`).Limit(limit + 1).Offset(offset).Find(&landmarks).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmark videos"})
		return
	}

	hasMore := len(landmarks) > limit
	if hasMore {
		landmarks = landmarks[:limit]
	}

	// Resolve playback URL: landmarks.video_url, else newest completed slideshow job (and repair DB).
	jobVideoByLandmark := map[uint]string{}
	for _, lm := range landmarks {
		if lm.VideoURL != nil && strings.TrimSpace(*lm.VideoURL) != "" {
			continue
		}
		if videoprocessing.RepairLandmarkVideoFromJob(storage.DB, lm.ID) {
			var refreshed models.Landmark
			if storage.DB.Select("video_url").First(&refreshed, lm.ID).Error == nil &&
				refreshed.VideoURL != nil && strings.TrimSpace(*refreshed.VideoURL) != "" {
				jobVideoByLandmark[lm.ID] = strings.TrimSpace(*refreshed.VideoURL)
			}
			continue
		}
		if u := videoprocessing.LatestCompletedLandSlideshowURL(storage.DB, lm.ID); u != "" {
			jobVideoByLandmark[lm.ID] = u
		}
	}

	// Batch-fetch likes and saves counts + user's liked/saved state
	lmIDs := make([]uint, 0, len(landmarks))
	for _, lm := range landmarks {
		if resolveLandmarkFeedVideoURL(lm, jobVideoByLandmark[lm.ID]) != "" {
			lmIDs = append(lmIDs, lm.ID)
		}
	}
	likesCountMap := make(map[uint]int64)
	savesCountMap := make(map[uint]int64)
	userLikedMap := make(map[uint]bool)
	userSavedMap := make(map[uint]bool)
	if len(lmIDs) > 0 {
		var likeCounts []struct {
			LandmarkID uint
			Cnt        int64
		}
		storage.DB.Model(&models.LandmarkVideoLike{}).Select("landmark_id, COUNT(*) as cnt").
			Where("landmark_id IN ? AND deleted_at IS NULL", lmIDs).Group("landmark_id").Find(&likeCounts)
		for _, r := range likeCounts {
			likesCountMap[r.LandmarkID] = r.Cnt
		}
		var saveCounts []struct {
			LandmarkID uint
			Cnt        int64
		}
		storage.DB.Model(&models.LandmarkVideoSave{}).Select("landmark_id, COUNT(*) as cnt").
			Where("landmark_id IN ? AND deleted_at IS NULL", lmIDs).Group("landmark_id").Find(&saveCounts)
		for _, r := range saveCounts {
			savesCountMap[r.LandmarkID] = r.Cnt
		}
		if userID > 0 {
			var userLikes []models.LandmarkVideoLike
			storage.DB.Where("landmark_id IN ? AND user_id = ?", lmIDs, userID).Find(&userLikes)
			for _, l := range userLikes {
				userLikedMap[l.LandmarkID] = true
			}
			var userSaves []models.LandmarkVideoSave
			storage.DB.Where("landmark_id IN ? AND user_id = ?", lmIDs, userID).Find(&userSaves)
			for _, s := range userSaves {
				userSavedMap[s.LandmarkID] = true
			}
		}
	}

	var videos []map[string]interface{}
	for _, lm := range landmarks {
		playbackURL := resolveLandmarkFeedVideoURL(lm, jobVideoByLandmark[lm.ID])
		if playbackURL == "" {
			continue
		}
		lmForClient := lm
		if lmForClient.VideoURL == nil || strings.TrimSpace(*lmForClient.VideoURL) == "" {
			u := playbackURL
			lmForClient.VideoURL = &u
		}
		redactLandmarkHostNote(&lmForClient, userID)
		lmTitle := utils.ResolveLocalizedText(lmForClient.Title, lmForClient.TitleTranslations, lang)
		// Do not use landmark listing photos as video thumbnail — feed uses preview_blur / video still only.
		thumbnailURL := storage.ChunkUploadPreviewBlurURL(playbackURL)
		// Host info: organization (if any) or individual owner - avatar/logo for profile display
		orgName := ""
		orgLogo := ""
		orgID := interface{}(nil)
		if lmForClient.Organization != nil {
			orgName = lmForClient.Organization.Name
			orgLogo = lmForClient.Organization.Logo
			orgID = lmForClient.Organization.ID
		} else if lmForClient.Owner != nil {
			orgName = strings.TrimSpace(fmt.Sprintf("%s %s", lmForClient.Owner.FirstName, lmForClient.Owner.LastName))
			if orgName == "" {
				orgName = lmForClient.Owner.Email
			}
			if lmForClient.Owner.AvatarURL != "" {
				orgLogo = lmForClient.Owner.AvatarURL
			}
			orgID = lmForClient.Owner.ID
		}
		profileUserID := uint(0)
		if lmForClient.Organization != nil && lmForClient.Organization.ID > 0 {
			profileUserID = lmForClient.Organization.OwnerID
		} else if lmForClient.Owner != nil && lmForClient.Owner.ID > 0 {
			profileUserID = lmForClient.Owner.ID
		}
		video := map[string]interface{}{
			"ID":              lmForClient.ID,
			"landmarkID":      lmForClient.ID,
			"userID":            profileUserID,
			"landmark":          lmForClient,
			"videoURL":          playbackURL,
			"VideoURL":          playbackURL,
			"thumbnailURL":      thumbnailURL,
			"preview_blur_url":  thumbnailURL,
			"previewBlurURL":    thumbnailURL,
			"caption":       lmTitle,
			"title":         lmTitle,
			"likesCount":    likesCountMap[lmForClient.ID],
			"commentsCount": 0,
			"savesCount":    savesCountMap[lmForClient.ID],
			"viewCount":     0,
			"liked":         userLikedMap[lmForClient.ID],
			"saved":         userSavedMap[lmForClient.ID],
			"organization": map[string]interface{}{
				"id":      orgID,
				"name":    orgName,
				"logoURL": orgLogo,
			},
			"CreatedAt":       lmForClient.CreatedAt,
			"UpdatedAt":       lmForClient.UpdatedAt,
			"isAutoSlideshow": true,
		}
		videos = append(videos, video)
	}

	var nextCursor string
	if hasMore {
		nextCursor = fmt.Sprintf("%d", page+1)
	}
	fmt.Printf("📹 Landmark Videos Feed - Returning %d videos (page: %d)\n", len(videos), page)
	ctx.JSON(iris.Map{
		"videos":     videos,
		"nextCursor": nextCursor,
		"hasMore":    hasMore,
	})
}

// UpdateLandmark updates an existing landmark
func UpdateLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var landmark models.Landmark
	if err := storage.DB.Where("id = ?", landmarkID).First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Authorization: user must own the landmark (owner_id) or belong to its organization
	canUpdate := false
	if landmark.OwnerID != nil && *landmark.OwnerID == userID {
		canUpdate = true
	}
	if !canUpdate && landmark.OrganizationID != nil {
		var org models.Organization
		if err := storage.DB.Where("id = ? AND owner_id = ?", landmark.OrganizationID, userID).First(&org).Error; err == nil {
			canUpdate = true
		}
	}
	if !canUpdate && landmark.OrganizationID != nil {
		var member models.OrganizationMember
		if err := storage.DB.Where("organization_id = ? AND user_id = ? AND status = ? AND is_active = ?",
			landmark.OrganizationID, userID, "active", true).First(&member).Error; err == nil {
			canUpdate = true
		}
	}
	if !canUpdate {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You do not have permission to update this landmark"})
		return
	}

	var input struct {
		Title             string    `json:"title"`
		Description       string    `json:"description"`
		Images            *[]string `json:"images"` // present in JSON => update (may be empty)
		VideoURL          *string   `json:"video_url"`
		HighlightLocation *bool     `json:"highlight_location"`
		Point1Lat         *float64  `json:"point1_lat"`
		Point1Lng         *float64  `json:"point1_lng"`
		Point2Lat         *float64  `json:"point2_lat"`
		Point2Lng         *float64  `json:"point2_lng"`
		Point3Lat         *float64  `json:"point3_lat"`
		Point3Lng         *float64  `json:"point3_lng"`
		Point4Lat         *float64  `json:"point4_lat"`
		Point4Lng         *float64  `json:"point4_lng"`
		PropertyPapers    []string  `json:"property_papers"`
		PaperTypes        []string  `json:"paper_types"`
		CityID            *uint     `json:"city_id"`
		ZoneID            *uint     `json:"zone_id"`
		QuartierID        *uint     `json:"quartier_id"`
		HabitatPlotID     *uint     `json:"habitat_plot_id"`
		PlotConfirmed     *bool     `json:"plot_confirmed"`
		Status            string    `json:"status"`
		District          string    `json:"district"`
		Region            string    `json:"region"`
		PlotNumber        string    `json:"plot_number"`
		ElevationMeters   float64   `json:"elevation_m"`
		Sides             []string  `json:"sides"`
		Price             float64   `json:"price"`
		Currency          string    `json:"currency"`
		Lots              *int      `json:"lots"`
		Area              float64   `json:"area"`
		AreaUnit          string    `json:"area_unit"`
		LandType          string    `json:"land_type"`
		Zoning            string    `json:"zoning"`
		HostPrivateNote   *string   `json:"host_private_note"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Update fields if provided
	titleChanged := false
	descChanged := false
	if input.Title != "" {
		landmark.Title = input.Title
		titleChanged = true
	}
	if input.Description != "" {
		landmark.Description = input.Description
		descChanged = true
	}
	if input.Status != "" {
		landmark.Status = input.Status
	}

	// Media: images and/or video
	if input.Images != nil {
		urls := filterHTTPMediaURLs(*input.Images)
		if b, err := json.Marshal(urls); err == nil {
			landmark.Images = b
		}
		videoURL := landmark.VideoURL
		if input.VideoURL != nil {
			videoURL = input.VideoURL
		}
		hasVideo := videoURL != nil && strings.TrimSpace(*videoURL) != ""
		switch {
		case len(urls) > 0 && hasVideo:
			landmark.MediaType = "both"
		case hasVideo:
			landmark.MediaType = "video"
		case len(urls) > 0:
			landmark.MediaType = "images"
		default:
			landmark.MediaType = "images"
		}
	}
	if input.VideoURL != nil {
		landmark.VideoURL = input.VideoURL
	}

	// Area and land fields
	if input.Area > 0 {
		landmark.Area = input.Area
	}
	if input.AreaUnit != "" {
		landmark.AreaUnit = input.AreaUnit
	}
	if input.LandType != "" {
		landmark.LandType = input.LandType
	}
	if input.Zoning != "" {
		landmark.Zoning = input.Zoning
	}
	if input.HostPrivateNote != nil {
		landmark.HostPrivateNote = sanitizeHostPrivateNote(*input.HostPrivateNote)
	}

	// Optional lots
	if input.Lots != nil {
		landmark.Lots = input.Lots
	}

	// Location highlight update:
	// - highlight_location=true + all four points => set coordinates
	// - highlight_location=true without points => keep existing (e.g. photo-only edit)
	// - highlight_location=false => clear coordinates
	if input.HighlightLocation != nil {
		if *input.HighlightLocation {
			hasAllPoints := input.Point1Lat != nil && input.Point1Lng != nil &&
				input.Point2Lat != nil && input.Point2Lng != nil &&
				input.Point3Lat != nil && input.Point3Lng != nil &&
				input.Point4Lat != nil && input.Point4Lng != nil
			if hasAllPoints {
				if !isValidCoordinate(*input.Point1Lat, *input.Point1Lng) ||
					!isValidCoordinate(*input.Point2Lat, *input.Point2Lng) ||
					!isValidCoordinate(*input.Point3Lat, *input.Point3Lng) ||
					!isValidCoordinate(*input.Point4Lat, *input.Point4Lng) {
					ctx.StatusCode(http.StatusBadRequest)
					ctx.JSON(iris.Map{"error": "Invalid coordinates"})
					return
				}
				landmark.Point1Lat, landmark.Point1Lng = input.Point1Lat, input.Point1Lng
				landmark.Point2Lat, landmark.Point2Lng = input.Point2Lat, input.Point2Lng
				landmark.Point3Lat, landmark.Point3Lng = input.Point3Lat, input.Point3Lng
				landmark.Point4Lat, landmark.Point4Lng = input.Point4Lat, input.Point4Lng
			}
		} else {
			landmark.Point1Lat, landmark.Point1Lng = nil, nil
			landmark.Point2Lat, landmark.Point2Lng = nil, nil
			landmark.Point3Lat, landmark.Point3Lng = nil, nil
			landmark.Point4Lat, landmark.Point4Lng = nil, nil
		}
	}

	// New metadata updates
	if input.District != "" {
		landmark.District = input.District
	}
	if input.Region != "" {
		landmark.Region = input.Region
	}
	if input.PlotNumber != "" {
		landmark.PlotNumber = input.PlotNumber
	}
	if input.ElevationMeters != 0 {
		landmark.ElevationMeters = input.ElevationMeters
	}
	if input.Price != 0 {
		landmark.Price = input.Price
	}
	if input.Currency != "" {
		landmark.Currency = input.Currency
	}
	if input.Sides != nil {
		if b, err := json.Marshal(input.Sides); err == nil {
			landmark.Sides = b
		}
	}
	if input.PaperTypes != nil {
		landmark.PaperTypes = input.PaperTypes
	}
	if input.PropertyPapers != nil {
		if b, err := json.Marshal(input.PropertyPapers); err == nil {
			landmark.PropertyPapers = b
		}
	}
	if input.CityID != nil && *input.CityID > 0 {
		landmark.CityID = input.CityID
	}
	if input.ZoneID != nil && *input.ZoneID > 0 {
		landmark.ZoneID = input.ZoneID
	}
	if input.QuartierID != nil && *input.QuartierID > 0 {
		landmark.QuartierID = input.QuartierID
	}
	if input.HabitatPlotID != nil {
		landmark.HabitatPlotID = input.HabitatPlotID
	}
	if input.PlotConfirmed != nil {
		landmark.PlotConfirmed = *input.PlotConfirmed
	}
	if landmark.CityID != nil && landmark.ZoneID != nil && landmark.QuartierID != nil {
		if err := validateLandmarkLocationIDs(*landmark.CityID, *landmark.ZoneID, *landmark.QuartierID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
	}
	if landmark.QuartierID != nil && landmark.PlotNumber != "" {
		if err := validateLandmarkHabitatPlot(landmark.HabitatPlotID, landmark.PlotNumber, *landmark.QuartierID); err != nil {
			ctx.StatusCode(http.StatusBadRequest)
			ctx.JSON(iris.Map{"error": err.Error()})
			return
		}
	}

	// Update translations if title or description changed
	if titleChanged || descChanged {
		if titleChanged {
			titleTranslations := services.TranslateAllLanguages(landmark.Title)
			if b, err := json.Marshal(titleTranslations); err == nil {
				landmark.TitleTranslations = b
			}
		}
		if descChanged {
			descTranslations := services.TranslateAllLanguages(landmark.Description)
			if b, err := json.Marshal(descTranslations); err == nil {
				landmark.DescriptionTranslations = b
			}
		}
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark"})
		return
	}

	ctx.JSON(landmark)
}

// DeleteLandmark soft deletes a landmark
func DeleteLandmark(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	var landmark models.Landmark
	if err := storage.DB.Where("id = ?", landmarkID).First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Authorization: owner or org member
	canDelete := false
	if landmark.OwnerID != nil && *landmark.OwnerID == userID {
		canDelete = true
	}
	if !canDelete && landmark.OrganizationID != nil {
		var org models.Organization
		if err := storage.DB.Where("id = ? AND owner_id = ?", landmark.OrganizationID, userID).First(&org).Error; err == nil {
			canDelete = true
		}
	}
	if !canDelete && landmark.OrganizationID != nil {
		var member models.OrganizationMember
		if err := storage.DB.Where("organization_id = ? AND user_id = ? AND status = ? AND is_active = ?",
			landmark.OrganizationID, userID, "active", true).First(&member).Error; err == nil {
			canDelete = true
		}
	}
	if !canDelete {
		ctx.StatusCode(http.StatusForbidden)
		ctx.JSON(iris.Map{"error": "You do not have permission to delete this landmark"})
		return
	}

	// Soft delete by setting status to inactive
	landmark.Status = "inactive"
	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to delete landmark"})
		return
	}

	ctx.JSON(iris.Map{"message": "Landmark deleted successfully"})
}

// SubmitLandmarkForVerification submits a landmark for admin verification
func SubmitLandmarkForVerification(ctx iris.Context) {
	userID := ctx.Values().Get("userID").(uint)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)

	// Get user's agent record
	var agent models.Agent
	if err := storage.DB.Where("user_id = ?", userID).First(&agent).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "User must be an agent"})
		return
	}

	// Check if landmark exists and belongs to user's organization
	var landmark models.Landmark
	if err := storage.DB.Where("id = ? AND organization_id = ?", landmarkID, agent.OrganizationID).First(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Update status to pending verification
	landmark.Status = "pending_verification"
	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to submit landmark for verification"})
		return
	}

	ctx.JSON(iris.Map{"message": "Landmark submitted for verification"})
}

// VerifyLandmark verifies a landmark (admin only)
func VerifyLandmark(ctx iris.Context) {
	fmt.Println("VerifyLandmark called")

	// Get userID from context with proper error handling
	userIDInterface := ctx.Values().Get("userID")
	fmt.Printf("userIDInterface: %v, type: %T\n", userIDInterface, userIDInterface)

	if userIDInterface == nil {
		fmt.Println("userID is nil")
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "User not authenticated"})
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		fmt.Printf("Failed to convert userID to uint, got type: %T\n", userIDInterface)
		ctx.StatusCode(http.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Invalid user ID"})
		return
	}

	fmt.Printf("userID: %d\n", userID)
	landmarkID, _ := strconv.ParseUint(ctx.Params().Get("id"), 10, 32)
	fmt.Printf("landmarkID: %d\n", landmarkID)

	var input struct {
		IsVerified        bool   `json:"is_verified"`
		VerificationNotes string `json:"verification_notes"`
	}

	if err := ctx.ReadJSON(&input); err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	// Get landmark
	var landmark models.Landmark
	if err := storage.DB.First(&landmark, landmarkID).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}

	// Update verification status
	landmark.IsVerified = input.IsVerified
	landmark.VerificationNotes = input.VerificationNotes
	landmark.VerifiedBy = &userID

	if input.IsVerified {
		now := time.Now()
		landmark.VerifiedAt = &now
		landmark.Status = "verified"
		landmark.IsPublished = true
	} else {
		landmark.Status = "rejected"
		landmark.IsPublished = false
	}

	if err := storage.DB.Save(&landmark).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to update landmark verification"})
		return
	}

	if input.IsVerified {
		landmarkID := landmark.ID
		jobUserID := userID
		if landmark.OwnerID != nil && *landmark.OwnerID > 0 {
			jobUserID = *landmark.OwnerID
		}
		go func() {
			videoprocessing.RepairLandmarkVideoFromJob(storage.DB, landmarkID)
			var lm models.Landmark
			if err := storage.DB.First(&lm, landmarkID).Error; err != nil {
				return
			}
			hasVideo := lm.VideoURL != nil && strings.TrimSpace(*lm.VideoURL) != ""
			if !hasVideo && videoprocessing.LatestCompletedLandSlideshowURL(storage.DB, landmarkID) == "" {
				if _, err := videoprocessing.EnqueueLandmarkSlideshow(storage.DB, landmarkID, jobUserID); err != nil {
					log.Printf("⚠️ VerifyLandmark slideshow enqueue land=%d: %v", landmarkID, err)
				}
			}
		}()
	}

	ctx.JSON(iris.Map{"message": "Landmark verification updated"})
}

func resolveLandmarkFeedVideoURL(lm models.Landmark, jobFallback string) string {
	if lm.VideoURL != nil {
		if u := storage.NormalizePlaybackMediaURL(strings.TrimSpace(*lm.VideoURL)); u != "" {
			return u
		}
	}
	if jobFallback != "" {
		return storage.NormalizePlaybackMediaURL(jobFallback)
	}
	return ""
}

// GetPendingLandmarks gets landmarks pending verification (admin only)
func GetPendingLandmarks(ctx iris.Context) {
	fmt.Println("GetPendingLandmarks called")

	// First, let's see ALL landmarks to debug
	var allLandmarks []models.Landmark
	if err := storage.DB.Preload("Organization").Find(&allLandmarks).Error; err != nil {
		fmt.Printf("Error fetching all landmarks: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}

	fmt.Printf("Found %d total landmarks\n", len(allLandmarks))
	for i, landmark := range allLandmarks {
		fmt.Printf("Landmark %d: ID=%d, Title=%s, Status=%s, IsVerified=%t\n",
			i+1, landmark.ID, landmark.Title, landmark.Status, landmark.IsVerified)
	}

	// Now get pending ones
	landmarks, err := loadLandmarksForAdmin(
		storage.DB.Model(&models.Landmark{}).Where("status = ?", "pending_verification"),
	)
	if err != nil {
		fmt.Printf("Error fetching pending landmarks: %v\n", err)
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch pending landmarks"})
		return
	}

	fmt.Printf("Found %d pending landmarks\n", len(landmarks))
	ctx.JSON(iris.Map{"landmarks": enrichAdminLandmarks(landmarks)})
}

// AdminGetAllLandmarks gets all landmarks for admin review (full media + location + plot details).
func AdminGetAllLandmarks(ctx iris.Context) {
	landmarks, err := loadLandmarksForAdmin(storage.DB.Model(&models.Landmark{}))
	if err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to fetch landmarks"})
		return
	}
	ctx.JSON(iris.Map{"landmarks": enrichAdminLandmarks(landmarks)})
}

// AdminGetLandmarkByID returns one landmark with full admin review payload.
func AdminGetLandmarkByID(ctx iris.Context) {
	id, err := ctx.Params().GetUint("id")
	if err != nil {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}
	var landmark models.Landmark
	if err := storage.DB.Preload("Organization").Preload("Owner").First(&landmark, id).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}
	enriched := enrichAdminLandmarks([]models.Landmark{landmark})
	if len(enriched) == 0 {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
		return
	}
	ctx.JSON(iris.Map{"landmark": enriched[0]})
}

// Helper function to validate coordinates
func isValidCoordinate(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

// validateLandmarkLocationIDs ensures city → zone → quartier hierarchy is consistent.
func validateLandmarkLocationIDs(cityID, zoneID, quartierID uint) error {
	var city models.City
	if err := storage.DB.First(&city, cityID).Error; err != nil {
		return fmt.Errorf("invalid city")
	}
	var zone models.Zone
	if err := storage.DB.Where("id = ? AND city_id = ?", zoneID, cityID).First(&zone).Error; err != nil {
		return fmt.Errorf("zone does not belong to the selected city")
	}
	var quartier models.Quartier
	if err := storage.DB.Where("id = ? AND zone_id = ?", quartierID, zoneID).First(&quartier).Error; err != nil {
		return fmt.Errorf("sector does not belong to the selected zone")
	}
	return nil
}

// validateLandmarkHabitatPlot checks cadastre plot link when provided.
func validateLandmarkHabitatPlot(habitatPlotID *uint, plotNumber string, quartierID uint) error {
	if habitatPlotID == nil || *habitatPlotID == 0 {
		return nil
	}
	var plot models.HabitatPlot
	if err := storage.DB.First(&plot, *habitatPlotID).Error; err != nil {
		return fmt.Errorf("cadastre plot not found")
	}
	pn := strings.TrimSpace(plotNumber)
	if pn == "" {
		return fmt.Errorf("plot number is required")
	}
	if !strings.EqualFold(strings.TrimSpace(plot.PlotNumber), pn) {
		return fmt.Errorf("plot number does not match the cadastre record")
	}
	sectorID, _, err := resolveHabitatSectorIDForQuartier(quartierID)
	if err != nil {
		return fmt.Errorf("could not match your sector to cadastre data")
	}
	if plot.SectorID != sectorID {
		return fmt.Errorf("cadastre plot is not in the selected sector")
	}
	return nil
}

// LikeLandmarkVideo likes a landmark's video (for feed)
func LikeLandmarkVideo(ctx iris.Context) {
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
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}
	var existing models.LandmarkVideoLike
	if err := storage.DB.Where("landmark_id = ? AND user_id = ?", id, userID).First(&existing).Error; err == nil {
		var likesCount int64
		storage.DB.Model(&models.LandmarkVideoLike{}).Where("landmark_id = ? AND deleted_at IS NULL", id).Count(&likesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Already liked", "likesCount": likesCount})
		return
	}
	like := models.LandmarkVideoLike{LandmarkID: id, UserID: userID}
	if err := storage.DB.Create(&like).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to like"})
		return
	}
	var likesCount int64
	storage.DB.Model(&models.LandmarkVideoLike{}).Where("landmark_id = ? AND deleted_at IS NULL", id).Count(&likesCount)
	ctx.JSON(iris.Map{"success": true, "likesCount": likesCount})
}

// UnlikeLandmarkVideo unlikes a landmark's video
func UnlikeLandmarkVideo(ctx iris.Context) {
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
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}
	storage.DB.Where("landmark_id = ? AND user_id = ?", id, userID).Delete(&models.LandmarkVideoLike{})
	var likesCount int64
	storage.DB.Model(&models.LandmarkVideoLike{}).Where("landmark_id = ? AND deleted_at IS NULL", id).Count(&likesCount)
	ctx.JSON(iris.Map{"success": true, "likesCount": likesCount})
}

// SaveLandmarkVideo saves a landmark's video
func SaveLandmarkVideo(ctx iris.Context) {
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
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}
	var existing models.LandmarkVideoSave
	if err := storage.DB.Where("landmark_id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).First(&existing).Error; err == nil {
		var savesCount int64
		storage.DB.Model(&models.LandmarkVideoSave{}).Where("landmark_id = ? AND deleted_at IS NULL", id).Count(&savesCount)
		ctx.JSON(iris.Map{"success": true, "message": "Already saved", "saved": true, "savesCount": savesCount})
		return
	}
	save := models.LandmarkVideoSave{LandmarkID: id, UserID: userID}
	if err := storage.DB.Create(&save).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to save"})
		return
	}
	var savesCount int64
	storage.DB.Model(&models.LandmarkVideoSave{}).Where("landmark_id = ? AND deleted_at IS NULL", id).Count(&savesCount)
	ctx.JSON(iris.Map{"success": true, "saved": true, "savesCount": savesCount})
}

// UnsaveLandmarkVideo unsaves a landmark's video
func UnsaveLandmarkVideo(ctx iris.Context) {
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
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}
	storage.DB.Where("landmark_id = ? AND user_id = ?", id, userID).Delete(&models.LandmarkVideoSave{})
	var savesCount int64
	storage.DB.Model(&models.LandmarkVideoSave{}).Where("landmark_id = ? AND deleted_at IS NULL", id).Count(&savesCount)
	ctx.JSON(iris.Map{"success": true, "saved": false, "savesCount": savesCount})
}

// ReportLandmark allows an authenticated user to report a public landmark
func ReportLandmark(ctx iris.Context) {
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
		ctx.JSON(iris.Map{"error": "Invalid landmark ID"})
		return
	}

	// Ensure landmark is public/verified
	var lm models.Landmark
	if err := storage.DB.Where("id = ? AND is_verified = ? AND is_published = ? AND status = ?", id, true, true, "verified").First(&lm).Error; err != nil {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Landmark not found"})
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

	rep := models.LandmarkReport{
		ReporterID:  userID,
		LandmarkID:  lm.ID,
		Reason:      body.Reason,
		Description: body.Description,
	}
	if err := storage.DB.Create(&rep).Error; err != nil {
		ctx.StatusCode(http.StatusInternalServerError)
		ctx.JSON(iris.Map{"error": "Failed to create report"})
		return
	}

	ctx.JSON(iris.Map{"success": true})
}
