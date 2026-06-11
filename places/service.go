package places

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"log"
	"math"
)

// HaversineDistanceKm returns distance in km between two points (lat/lng in degrees).
func HaversineDistanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	lat1R, lng1R := rad(lat1), rad(lng1)
	lat2R, lng2R := rad(lat2), rad(lng2)
	dlat := lat2R - lat1R
	dlng := lng2R - lng1R
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1R)*math.Cos(lat2R)*math.Sin(dlng/2)*math.Sin(dlng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

// Service orchestrates Google client and repository.
type Service struct {
	client *GooglePlacesClient
	repo   *Repository
}

// DefaultService is set from main and used by routes (CreatePropertySale, GetNearby).
var DefaultService *Service

// NewService builds a places service. apiKey for Google (can be empty to disable).
func NewService(apiKey string) *Service {
	return &Service{
		client: NewGooglePlacesClient(apiKey),
		repo:   NewRepository(),
	}
}

// FetchAndSaveNearby fetches nearby places for a property and saves them. Safe to call async.
func (s *Service) FetchAndSaveNearby(propertySaleID uint, lat, lng float64) {
	if s.client == nil {
		return
	}
	byType := s.client.EnrichNearby(lat, lng, true)
	photoURLFn := func(ref string) string {
		if s.client != nil {
			return s.client.PhotoURL(ref)
		}
		return ""
	}
	if err := s.repo.Save(propertySaleID, lat, lng, byType, photoURLFn); err != nil {
		log.Printf("places: Save property_sale_id=%d err=%v", propertySaleID, err)
		return
	}
	log.Printf("places: saved nearby for property_sale_id=%d", propertySaleID)
}

// GetNearbyForProperty returns stored places grouped by type for the API response.
func (s *Service) GetNearbyForProperty(propertySaleID uint) (NearbyResponse, error) {
	list, err := s.repo.GetByPropertySaleID(propertySaleID)
	if err != nil {
		return NearbyResponse{}, err
	}
	out := NearbyResponse{
		Restaurants: []NearbyPlace{},
		Hospitals:   []NearbyPlace{},
		Schools:     []NearbyPlace{},
	}
	for _, p := range list {
		np := NearbyPlace{
			Name:       p.Name,
			Rating:     p.Rating,
			Reviews:    p.ReviewsCount,
			Address:    p.Address,
			Phone:      p.Phone,
			Photo:      p.PhotoURL,
			Latitude:   p.Latitude,
			Longitude:  p.Longitude,
			DistanceKm: p.DistanceKm,
			Website:    p.Website,
		}
		switch p.Type {
		case TypeRestaurant:
			out.Restaurants = append(out.Restaurants, np)
		case TypeHospital:
			out.Hospitals = append(out.Hospitals, np)
		case TypeSchool:
			out.Schools = append(out.Schools, np)
		}
	}
	return out, nil
}

// NeedsRefresh returns true if we should refetch (no data or older than 30 days).
func (s *Service) NeedsRefresh(propertySaleID uint) bool {
	return s.repo.NeedsRefresh(propertySaleID)
}

// BackfillResult is the result of a backfill run.
type BackfillResult struct {
	Processed int   `json:"processed"`
	Failed    int   `json:"failed"`
	Skipped   int   `json:"skipped"` // no coords or already fresh
}

// Backfill fetches nearby places for existing properties with coordinates. BatchSize and offset for chunking.
func (s *Service) Backfill(batchSize, offset int) BackfillResult {
	if s.client == nil || storage.DB == nil {
		return BackfillResult{}
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	var props []struct {
		ID        uint
		Latitude  float64
		Longitude float64
	}
	err := storage.DB.Model(&models.PropertySale{}).
		Select("id, latitude, longitude").
		Where("latitude IS NOT NULL AND longitude != 0 AND latitude != 0").
		Order("id ASC").
		Offset(offset).Limit(batchSize).
		Find(&props).Error
	if err != nil {
		log.Printf("places: Backfill query err=%v", err)
		return BackfillResult{}
	}
	var result BackfillResult
	for _, p := range props {
		if !s.repo.NeedsRefresh(p.ID) {
			result.Skipped++
			continue
		}
		s.FetchAndSaveNearby(p.ID, p.Latitude, p.Longitude)
		result.Processed++
	}
	return result
}

// BackfillAll runs Backfill in a loop until no more properties need processing or maxToProcess is reached.
// Use for "one-shot" backfill of all existing properties with coordinates (e.g. from admin "Run backfill" button).
func (s *Service) BackfillAll(maxToProcess int) BackfillResult {
	if maxToProcess <= 0 {
		maxToProcess = 1000
	}
	batchSize := 50
	var total BackfillResult
	for offset := 0; total.Processed+total.Skipped < maxToProcess; offset += batchSize {
		r := s.Backfill(batchSize, offset)
		total.Processed += r.Processed
		total.Skipped += r.Skipped
		total.Failed += r.Failed
		if r.Processed == 0 && r.Skipped == 0 {
			break // no more rows with coordinates
		}
	}
	return total
}