package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"strings"
)

// resolveListingCountry fills country_id and display names from explicit country or city FK.
func resolveListingCountry(countryID *uint, cityID *uint, countryText string) (*uint, string, string) {
	var c models.Country
	if countryID != nil && *countryID > 0 {
		if err := storage.DB.First(&c, *countryID).Error; err == nil {
			return countryID, c.Name, c.NameAr
		}
	}
	if cityID != nil && *cityID > 0 {
		var city models.City
		if err := storage.DB.First(&city, *cityID).Error; err == nil {
			if city.CountryID != nil && *city.CountryID > 0 {
				if err := storage.DB.First(&c, *city.CountryID).Error; err == nil {
					id := *city.CountryID
					return &id, c.Name, c.NameAr
				}
			}
			return nil, city.Country, city.CountryAr
		}
	}
	t := strings.TrimSpace(countryText)
	if t == "" {
		t = "Mauritania"
	}
	var byName models.Country
	if err := storage.DB.Where("is_active = ? AND (LOWER(name) = LOWER(?) OR LOWER(name_fr) = LOWER(?))",
		true, t, t).First(&byName).Error; err == nil {
		id := byName.ID
		return &id, byName.Name, byName.NameAr
	}
	return nil, t, t
}
