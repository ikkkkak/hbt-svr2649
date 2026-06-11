package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

// PropertySalesCacheService handles caching for property sales listings
type PropertySalesCacheService struct {
	cache  *CacheService
	config CacheConfig
	logger *log.Logger
}

// NewPropertySalesCacheService creates a new property sales cache service
func NewPropertySalesCacheService(cache *CacheService, config CacheConfig) *PropertySalesCacheService {
	return &PropertySalesCacheService{
		cache:  cache,
		config: config,
		logger: log.New(log.Writer(), "[PropertySalesCache] ", log.LstdFlags|log.Lshortfile),
	}
}

// ─────────────────────────────────────────────────────────────────
// Property Sales List Caching
// ─────────────────────────────────────────────────────────────────

// CachedPropertySalesList represents cached property sales list
type CachedPropertySalesList struct {
	Properties []CachedPropertySalesCard `json:"properties"`
	NextCursor string                    `json:"next_cursor"`
	Total      int64                     `json:"total"`
	CachedAt   time.Time                 `json:"cached_at"`
}

// CachedPropertySalesCard represents a cached property card (lightweight)
type CachedPropertySalesCard struct {
	ID               uint      `json:"id"`
	Title            string    `json:"title"`
	Address          string    `json:"address"`
	City             string    `json:"city"`
	State            string    `json:"state"`
	Country          string    `json:"country"`
	Price            float64   `json:"price"`
	Bedrooms         int       `json:"bedrooms"`
	Bathrooms        int       `json:"bathrooms"`
	Area             float64   `json:"area"`
	PropertyType     string    `json:"property_type"`
	YearBuilt        int       `json:"year_built"`
	ThumbnailURL     string    `json:"thumbnail_url"`
	Images           []string  `json:"images"`
	Videos           []string  `json:"videos"`
	OrganizationName string    `json:"organization_name"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

// GetPropertySalesListFromCache retrieves cached property sales list
func (psc *PropertySalesCacheService) GetPropertySalesListFromCache(ctx context.Context, pageNum int, limit int) (*CachedPropertySalesList, error) {
	key := FormatKey(PropertySalesPageKey, pageNum, limit)

	var cached CachedPropertySalesList
	err := psc.cache.Get(ctx, key, &cached)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(cached.Properties) == 0 {
		return nil, nil // Cache miss
	}

	psc.logger.Printf("💾 Property sales list cache hit: page=%d, items=%d", pageNum, len(cached.Properties))
	return &cached, nil
}

// SetPropertySalesListCache caches property sales list
func (psc *PropertySalesCacheService) SetPropertySalesListCache(
	ctx context.Context,
	pageNum int,
	limit int,
	properties []models.PropertySale,
	nextCursor string,
	total int64,
) error {
	key := FormatKey(PropertySalesPageKey, pageNum, limit)

	// Convert to lightweight cached format
	cached := CachedPropertySalesList{
		Properties: make([]CachedPropertySalesCard, len(properties)),
		NextCursor: nextCursor,
		Total:      total,
		CachedAt:   time.Now(),
	}

	for i, prop := range properties {
		var orgName string
		if prop.Organization != nil {
			orgName = prop.Organization.Name
		}
		images := prop.Images
		if images == nil {
			images = []string{}
		}
		videos := prop.Videos
		if videos == nil {
			videos = []string{}
		}
		thumb := ""
		if len(images) > 0 && images[0] != "" {
			thumb = images[0]
		}

		cached.Properties[i] = CachedPropertySalesCard{
			ID:               prop.ID,
			Title:            prop.Title,
			Address:          prop.Address,
			City:             prop.City,
			State:            prop.State,
			Country:          prop.Country,
			Price:            prop.ListingPrice,
			Bedrooms:         prop.Bedrooms,
			Bathrooms:        prop.Bathrooms,
			Area:             float64(prop.SquareFootage),
			PropertyType:     prop.PropertyType,
			YearBuilt:        prop.YearBuilt,
			ThumbnailURL:     thumb,
			Images:           images,
			Videos:           videos,
			OrganizationName: orgName,
			Latitude:         prop.Latitude,
			Longitude:        prop.Longitude,
			Status:           prop.Status,
			CreatedAt:        prop.CreatedAt,
		}
	}

	err := psc.cache.Set(ctx, key, cached, psc.config.PropertyListTTL)
	if err != nil {
		psc.logger.Printf("⚠️ Failed to cache property sales list: %v", err)
		return err
	}

	psc.logger.Printf("✅ Cached property sales list: page=%d, items=%d, ttl=%v", pageNum, len(properties), psc.config.PropertyListTTL)
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Property Details Caching
// ─────────────────────────────────────────────────────────────────

// CachedPropertySalesDetails represents cached full property details
type CachedPropertySalesDetails struct {
	ID                 uint                   `json:"id"`
	Title              string                 `json:"title"`
	Description        string                 `json:"description"`
	Address            string                 `json:"address"`
	City               string                 `json:"city"`
	State              string                 `json:"state"`
	Country            string                 `json:"country"`
	Price              float64                `json:"price"`
	Bedrooms           int                    `json:"bedrooms"`
	Bathrooms          int                    `json:"bathrooms"`
	Area               float64                `json:"area"`
	PropertyType       string                 `json:"property_type"`
	YearBuilt          int                    `json:"year_built"`
	Latitude           float64                `json:"latitude"`
	Longitude          float64                `json:"longitude"`
	Status             string                 `json:"status"`
	ImageURLs          []string               `json:"image_urls"`
	Amenities          []string               `json:"amenities"`
	OrganizationID     uint                   `json:"organization_id"`
	OrganizationName   string                 `json:"organization_name"`
	OrganizationPhone  string                 `json:"organization_phone"`
	OrganizationWebsite string                `json:"organization_website"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// GetPropertySalesDetailsFromCache retrieves cached property details
func (psc *PropertySalesCacheService) GetPropertySalesDetailsFromCache(ctx context.Context, propertyID uint) (*CachedPropertySalesDetails, error) {
	key := FormatKey(PropertyDetailsKey, propertyID)

	var cached CachedPropertySalesDetails
	err := psc.cache.Get(ctx, key, &cached)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if cached.ID == 0 {
		return nil, nil // Cache miss
	}

	psc.logger.Printf("💾 Property details cache hit: property=%d", propertyID)
	return &cached, nil
}

// SetPropertySalesDetailsCache caches full property details
func (psc *PropertySalesCacheService) SetPropertySalesDetailsCache(
	ctx context.Context,
	property *models.PropertySale,
) error {
	key := FormatKey(PropertyDetailsKey, property.ID)

	var orgName, orgPhone, orgWebsite string
	var orgID uint
	if property.Organization != nil {
		orgName = property.Organization.Name
		orgPhone = property.Organization.Phone
		orgWebsite = property.Organization.Website
	}
	if property.OrganizationID != nil {
		orgID = *property.OrganizationID
	}

	// Extract image URLs from Images field
	imageURLs := property.Images
	if imageURLs == nil {
		imageURLs = []string{}
	}

	// Extract amenities from AmenityList (Name is AmenityNames type, extract English name)
	amenities := []string{}
	for _, am := range property.AmenityList {
		// Extract English name from translation object
		if am.Name.En != "" {
			amenities = append(amenities, am.Name.En)
		}
	}

	cached := CachedPropertySalesDetails{
		ID:               property.ID,
		Title:            property.Title,
		Description:      property.Description,
		Address:          property.Address,
		City:             property.City,
		State:            property.State,
		Country:          property.Country,
		Price:            property.ListingPrice,
		Bedrooms:         property.Bedrooms,
		Bathrooms:        property.Bathrooms,
		Area:             float64(property.SquareFootage),
		PropertyType:     property.PropertyType,
		YearBuilt:        property.YearBuilt,
		Latitude:         property.Latitude,
		Longitude:        property.Longitude,
		Status:           property.Status,
		ImageURLs:        imageURLs,
		Amenities:        amenities,
		OrganizationID:   orgID,
		OrganizationName: orgName,
		OrganizationPhone: orgPhone,
		OrganizationWebsite: orgWebsite,
		CreatedAt:        property.CreatedAt,
		UpdatedAt:        property.UpdatedAt,
	}

	err := psc.cache.Set(ctx, key, cached, psc.config.PropertyDetailsTTL)
	if err != nil {
		psc.logger.Printf("⚠️ Failed to cache property details: %v", err)
		return err
	}

	psc.logger.Printf("✅ Cached property details: property=%d, ttl=%v", property.ID, psc.config.PropertyDetailsTTL)
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Search Caching (with query hashing)
// ─────────────────────────────────────────────────────────────────

// GetPropertySearchFromCache retrieves cached search results
func (psc *PropertySalesCacheService) GetPropertySearchFromCache(
	ctx context.Context,
	searchQuery map[string]interface{},
) (*CachedPropertySalesList, error) {
	// Hash search query to create cache key
	queryHash := psc.hashSearchQuery(searchQuery)
	key := FormatKey(PropertySalesSearchKey, queryHash)

	var cached CachedPropertySalesList
	err := psc.cache.Get(ctx, key, &cached)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	if len(cached.Properties) == 0 {
		return nil, nil
	}

	psc.logger.Printf("💾 Property search cache hit: query=%s", queryHash)
	return &cached, nil
}

// SetPropertySearchCache caches search results
func (psc *PropertySalesCacheService) SetPropertySearchCache(
	ctx context.Context,
	searchQuery map[string]interface{},
	properties []models.PropertySale,
) error {
	queryHash := psc.hashSearchQuery(searchQuery)
	key := FormatKey(PropertySalesSearchKey, queryHash)

	cached := CachedPropertySalesList{
		Properties: make([]CachedPropertySalesCard, len(properties)),
		CachedAt:   time.Now(),
	}

	for i, prop := range properties {
		var orgName string
		if prop.Organization != nil {
			orgName = prop.Organization.Name
		}
		images := prop.Images
		if images == nil {
			images = []string{}
		}
		videos := prop.Videos
		if videos == nil {
			videos = []string{}
		}
		thumb := ""
		if len(images) > 0 && images[0] != "" {
			thumb = images[0]
		}
		cached.Properties[i] = CachedPropertySalesCard{
			ID:               prop.ID,
			Title:            prop.Title,
			Address:          prop.Address,
			City:             prop.City,
			State:            prop.State,
			Country:          prop.Country,
			Price:            prop.ListingPrice,
			Bedrooms:         prop.Bedrooms,
			Bathrooms:        prop.Bathrooms,
			Area:             float64(prop.SquareFootage),
			PropertyType:     prop.PropertyType,
			YearBuilt:        prop.YearBuilt,
			ThumbnailURL:     thumb,
			Images:           images,
			Videos:           videos,
			OrganizationName: orgName,
			Latitude:         prop.Latitude,
			Longitude:        prop.Longitude,
			Status:           prop.Status,
			CreatedAt:        prop.CreatedAt,
		}
	}

	err := psc.cache.Set(ctx, key, cached, psc.config.PropertyListTTL)
	if err != nil {
		psc.logger.Printf("⚠️ Failed to cache search results: %v", err)
		return err
	}

	psc.logger.Printf("✅ Cached search results: query=%s, items=%d", queryHash, len(properties))
	return nil
}

// hashSearchQuery creates a hash of search parameters for caching
func (psc *PropertySalesCacheService) hashSearchQuery(query map[string]interface{}) string {
	// Convert query to deterministic string
	queryStr := fmt.Sprintf("%+v", query)
	hash := md5.Sum([]byte(queryStr))
	return fmt.Sprintf("%x", hash)
}

// ─────────────────────────────────────────────────────────────────
// Prefetching
// ─────────────────────────────────────────────────────────────────

// PreloadPropertySalesList preloads first page of property sales
func (psc *PropertySalesCacheService) PreloadPropertySalesList(ctx context.Context) error {
	psc.logger.Printf("🔄 Starting property sales list preload")

	var properties []models.PropertySale
	db := storage.DB

	err := db.WithContext(ctx).
		Preload("Organization").
		Where("status = ?", "published").
		Order("created_at DESC").
		Limit(10).
		Find(&properties).Error

	if err != nil {
		psc.logger.Printf("❌ Failed to preload properties: %v", err)
		return err
	}

	// Cache the properties
	err = psc.SetPropertySalesListCache(ctx, 1, 10, properties, "", int64(len(properties)))
	if err != nil {
		psc.logger.Printf("⚠️ Failed to cache preloaded properties: %v", err)
		return err
	}

	psc.logger.Printf("✅ Property sales list preloaded: %d properties cached", len(properties))
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Cache Invalidation
// ─────────────────────────────────────────────────────────────────

// InvalidatePropertySalesLists invalidates all sales list caches
func (psc *PropertySalesCacheService) InvalidatePropertySalesLists(ctx context.Context) error {
	return psc.cache.InvalidatePropertySalesCaches(ctx)
}

// InvalidatePropertyDetails invalidates details for a specific property
func (psc *PropertySalesCacheService) InvalidatePropertyDetails(ctx context.Context, propertyID uint) error {
	keys := []string{
		FormatKey(PropertyDetailsKey, propertyID),
		FormatKey(PropertySaleKey, propertyID),
	}
	return psc.cache.Delete(ctx, keys...)
}

// InvalidateAllPropertyCaches invalidates all property-related caches
func (psc *PropertySalesCacheService) InvalidateAllPropertyCaches(ctx context.Context) error {
	// Invalidate both lists and searches
	err1 := psc.InvalidatePropertySalesLists(ctx)
	err2 := psc.cache.DeletePattern(ctx, GetCacheKeyPrefix(PropertySalesSearchKey))

	if err1 != nil {
		return err1
	}
	return err2
}
