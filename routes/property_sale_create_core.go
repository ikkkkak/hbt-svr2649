package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/places"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
)

// ProgressReporter receives overall publish percent (58–100) and step key during server-side create.
type ProgressReporter func(percent int, step string)

func noopProgress(int, string) {}

// ExecuteCreatePropertySale runs validation, insert, and schedules background work.
// Percent values align with the mobile app publishProgress bands (58–100 = server work).
func ExecuteCreatePropertySale(
	userID uint,
	input *PropertySaleCreatePayload,
	report ProgressReporter,
) (propertyID uint, err error) {
	if report == nil {
		report = noopProgress
	}
	start := time.Now()
	report(58, "validate")

	input.Images = filterHTTPMediaURLs(input.Images)
	input.Videos = filterHTTPMediaURLs(input.Videos)
	if len(input.Description) > 12000 {
		input.Description = input.Description[:12000]
	}
	report(62, "normalize")

	if msg := validateCreatePropertySaleInput(&struct {
		Title        string
		Description  string
		PropertyType string
		Price        float64
		Area         float64
		Address      string
		City         string
		Images       []string
		Videos       []string
	}{
		Title: input.Title, Description: input.Description, PropertyType: input.PropertyType,
		Price: input.Price, Area: input.Area, Address: input.Address, City: input.City,
		Images: input.Images, Videos: input.Videos,
	}); msg != "" {
		return 0, errors.New(msg)
	}
	report(65, "ready")

	areaSq := int(input.Area + 0.5)
	if areaSq <= 0 {
		return 0, errors.New("area must be greater than 0")
	}
	yearBuilt := int(input.YearBuilt + 0.5)

	var pricePerSqFt float64
	if areaSq > 0 {
		pricePerSqFt = input.Price / float64(areaSq)
	}

	var allFeatures []string
	allFeatures = append(allFeatures, input.IndoorFeatures...)
	allFeatures = append(allFeatures, input.OutdoorFeatures...)

	state := strings.TrimSpace(input.State)
	if state == "" {
		state = "-"
	}
	country := strings.TrimSpace(input.Country)
	if country == "" {
		country = "Mauritania"
	}
	var countryID *uint
	if cid, cName, _ := resolveListingCountry(input.CountryID, input.CityID, country); cid != nil {
		countryID = cid
		country = cName
	}

	ownerIDPtr := &userID
	property := models.PropertySale{
		OwnerID:          ownerIDPtr,
		AgentID:          input.AgentID,
		Title:            input.Title,
		Description:      input.Description,
		PropertyType:     input.PropertyType,
		Category:         "residential",
		Address:          input.Address,
		City:             input.City,
		CityID:           input.CityID,
		ZoneID:           input.ZoneID,
		QuartierID:       input.QuartierID,
		State:            state,
		Country:          country,
		CountryID:        countryID,
		PostalCode:       input.PostalCode,
		Latitude:         input.Latitude,
		Longitude:        input.Longitude,
		Bedrooms:         optInt(input.Bedrooms),
		Bathrooms:        optInt(input.Bathrooms),
		SquareFootage:    areaSq,
		YearBuilt:        yearBuilt,
		ListingPrice:     input.Price,
		Currency:         "USD",
		PricePerSqFt:     pricePerSqFt,
		Status:           "draft",
		IsVerified:       false,
		IsPublished:      false,
		HostPrivateNote:  sanitizeHostPrivateNote(input.HostPrivateNote),
		PaperTypes:       input.PaperTypes,
		Images:           input.Images,
		Videos:           input.Videos,
		Features:         allFeatures,
		Amenities:        input.Amenities,
		ClassifiedPhotos: input.ClassifiedPhotos,
		FloorPlans:       input.FloorPlans,
		Neighborhood:     input.Neighborhood,
	}

	report(72, "inserting")
	propertyID, err = fastInsertPropertySale(userID, &property)
	if err != nil {
		log.Printf("❌ ExecuteCreatePropertySale insert failed after %s: %v", time.Since(start), err)
		return 0, err
	}
	log.Printf("✅ ExecuteCreatePropertySale created id=%d in %s", propertyID, time.Since(start))
	services.NotifyAdminNewListing(services.ListingAdminNotifyInput{
		Kind:         services.ListingKindPropertySale,
		ID:           propertyID,
		Title:        property.Title,
		City:         property.City,
		Price:        property.ListingPrice,
		Currency:     property.Currency,
		PropertyType: property.PropertyType,
		HostUserID:   userID,
		Status:       property.Status,
	})
	report(88, "row_created")

	videoURLs := append([]string(nil), input.Videos...)
	amenityIDs := append([]uint(nil), input.AmenityIDs...)
	titleCopy := property.Title
	descCopy := property.Description
	propCopy := property

	report(92, "setup")

	go func(uid, pid uint) {
		if sqlDB, err := storage.SQLDB(); err == nil {
			if orgID := resolvePropertySaleOrganizationID(sqlDB, uid); orgID != nil {
				patchPropertySaleOrganization(pid, *orgID)
			}
		}
	}(userID, propertyID)

	go patchPropertySaleDetails(propertyID, &propCopy)

	go func(propertyID uint, title, description string) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("⚠️ Panic in async translation for property %d: %v", propertyID, r)
			}
		}()
		titleTranslations := services.TranslateAllLanguages(title)
		descTranslations := services.TranslateAllLanguages(description)
		titleJSON, err1 := json.Marshal(titleTranslations)
		descJSON, err2 := json.Marshal(descTranslations)
		if err1 != nil || err2 != nil {
			return
		}
		_ = storage.DB.Model(&models.PropertySale{}).
			Where("id = ?", propertyID).
			Updates(map[string]interface{}{
				"title_translations":       titleJSON,
				"description_translations": descJSON,
			}).Error
	}(propertyID, titleCopy, descCopy)

	go func(pid, uid uint, videos []string, aIDs []uint) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("⚠️ Panic in CreatePropertySale post-create id=%d: %v", pid, r)
			}
		}()
		if len(videos) > 0 {
			_ = SyncPropertySaleVideoRows(pid, uid, videos)
		}
		if len(aIDs) > 0 {
			var amenities []models.Amenity
			if err := storage.DB.Where("id IN ?", aIDs).Find(&amenities).Error; err == nil {
				var prop models.PropertySale
				if err := storage.DB.First(&prop, pid).Error; err == nil {
					_ = storage.DB.Model(&prop).Association("AmenityList").Replace(amenities)
				}
			}
		}
	}(propertyID, userID, videoURLs, amenityIDs)

	if property.Latitude != 0 && property.Longitude != 0 && places.DefaultService != nil {
		go places.DefaultService.FetchAndSaveNearby(propertyID, property.Latitude, property.Longitude)
	}

	report(100, "complete")
	return propertyID, nil
}
