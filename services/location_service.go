package services

import (
	"apartments-clone-server/models"
	"math"
)

// Key locations in Mauritania with their coordinates
var MauritaniaLocations = map[string]Location{
	"center": {
		Name:     "Centre de Nouakchott",
		Lat:      18.0735,
		Lng:      -15.9582,
		Radius:   5.0, // 5km radius
		Type:     "city_center",
		Priority: 1,
	},
	"port": {
		Name:     "Port de Nouakchott",
		Lat:      18.0833,
		Lng:      -15.9667,
		Radius:   3.0,
		Type:     "business",
		Priority: 2,
	},
	"airport": {
		Name:     "Aéroport International",
		Lat:      18.0975,
		Lng:      -15.9475,
		Radius:   4.0,
		Type:     "transport",
		Priority: 3,
	},
	"embassy": {
		Name:     "Quartier des Ambassades",
		Lat:      18.0900,
		Lng:      -15.9500,
		Radius:   2.0,
		Type:     "luxury",
		Priority: 4,
	},
	"beach": {
		Name:     "Plage de Nouakchott",
		Lat:      18.0600,
		Lng:      -15.9800,
		Radius:   3.0,
		Type:     "leisure",
		Priority: 5,
	},
	"market": {
		Name:     "Marché Capitale",
		Lat:      18.0700,
		Lng:      -15.9600,
		Radius:   2.0,
		Type:     "commercial",
		Priority: 6,
	},
}

type Location struct {
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Radius   float64 `json:"radius"` // in kilometers
	Type     string  `json:"type"`
	Priority int     `json:"priority"`
}

// Calculate distance between two points using Haversine formula
func CalculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// Check if a property is within radius of a location
func IsPropertyNearLocation(property *models.Property, location Location) bool {
	distance := CalculateDistance(
		float64(property.Lat),
		float64(property.Lng),
		location.Lat,
		location.Lng,
	)
	return distance <= location.Radius
}

// Get properties near a specific location
func GetPropertiesNearLocation(properties []models.Property, locationKey string) []models.Property {
	location, exists := MauritaniaLocations[locationKey]
	if !exists {
		return []models.Property{}
	}

	var nearbyProperties []models.Property
	for _, property := range properties {
		if IsPropertyNearLocation(&property, location) {
			nearbyProperties = append(nearbyProperties, property)
		}
	}

	return nearbyProperties
}

// Get all location keys sorted by priority
func GetLocationKeysByPriority() []string {
	var keys []string
	priorityMap := make(map[int]string)

	for key, location := range MauritaniaLocations {
		priorityMap[location.Priority] = key
	}

	for i := 1; i <= len(MauritaniaLocations); i++ {
		if key, exists := priorityMap[i]; exists {
			keys = append(keys, key)
		}
	}

	return keys
}

// Get location info by key
func GetLocationInfo(locationKey string) (Location, bool) {
	location, exists := MauritaniaLocations[locationKey]
	return location, exists
}
