package places

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func haversineDistanceKm(lat1, lng1, lat2, lng2 float64) float64 {
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

const refreshAgeDays = 30

// Repository persists and reads property_places.
type Repository struct {
	db *gorm.DB
}

// NewRepository returns a repository using the global DB.
func NewRepository() *Repository {
	return &Repository{db: storage.DB}
}

// GetByPropertySaleID returns all stored places for a property, grouped by type.
func (r *Repository) GetByPropertySaleID(propertySaleID uint) ([]models.PropertyPlace, error) {
	if r.db == nil {
		return nil, nil
	}
	var list []models.PropertyPlace
	err := r.db.Where("property_sale_id = ?", propertySaleID).Order("place_type, distance_km ASC").Find(&list).Error
	return list, err
}

// Save replaces places for a property: delete existing and insert new.
// propertyLat, propertyLng are used to compute distance_km for each place.
func (r *Repository) Save(propertySaleID uint, propertyLat, propertyLng float64, byType map[string][]RawPlace, photoURLFn func(photoRef string) string) error {
	if r.db == nil {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("property_sale_id = ?", propertySaleID).Delete(&models.PropertyPlace{}).Error; err != nil {
			return err
		}
		now := time.Now()
		seen := make(map[string]struct{}, 64)
		records := make([]models.PropertyPlace, 0, 64)
		for typ, places := range byType {
			for _, p := range places {
				if p.PlaceID == "" {
					continue
				}
				key := typ + "::" + p.PlaceID
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				photoURL := ""
				if photoURLFn != nil && p.PhotoRef != "" {
					photoURL = photoURLFn(p.PhotoRef)
				}
				distKm := haversineDistanceKm(propertyLat, propertyLng, p.Lat, p.Lng)
				rec := models.PropertyPlace{
					PropertySaleID: propertySaleID,
					PlaceID:        p.PlaceID,
					Name:           p.Name,
					Type:           typ,
					Rating:         p.Rating,
					ReviewsCount:   p.Reviews,
					Address:        p.Address,
					Phone:          p.Phone,
					Website:        p.Website,
					Latitude:       p.Lat,
					Longitude:      p.Lng,
					PhotoURL:       photoURL,
					DistanceKm:     distKm,
					LastFetchedAt:  &now,
				}
				records = append(records, rec)
			}
		}
		if len(records) == 0 {
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "property_sale_id"}, {Name: "place_id"}},
			DoNothing: true,
		}).CreateInBatches(&records, 100).Error; err != nil {
			return err
		}
		return nil
	})
}

// NeedsRefresh returns true if there are no places or the oldest last_fetched_at is older than refreshAgeDays.
func (r *Repository) NeedsRefresh(propertySaleID uint) bool {
	if r.db == nil {
		return true
	}
	var count int64
	r.db.Model(&models.PropertyPlace{}).Where("property_sale_id = ?", propertySaleID).Count(&count)
	if count == 0 {
		return true
	}
	var last time.Time
	r.db.Model(&models.PropertyPlace{}).Where("property_sale_id = ?", propertySaleID).Select("COALESCE(MAX(last_fetched_at), '1970-01-01')").Scan(&last)
	return time.Since(last) > refreshAgeDays*24*time.Hour
}
