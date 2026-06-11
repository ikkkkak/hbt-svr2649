package routes

import "apartments-clone-server/models"

// PropertySaleCreatePayload is the JSON body for creating a property sale listing.
type PropertySaleCreatePayload struct {
	Title            string                   `json:"title"`
	Description      string                   `json:"description"`
	PropertyType     string                   `json:"property_type"`
	Price            float64                  `json:"price"`
	Bedrooms         *int                     `json:"bedrooms"`
	Bathrooms        *int                     `json:"bathrooms"`
	Area             float64                  `json:"area"`
	YearBuilt        float64                  `json:"year_built"`
	Address          string                   `json:"address"`
	City             string                   `json:"city"`
	CityID           *uint                    `json:"city_id"`
	ZoneID           *uint                    `json:"zone_id"`
	QuartierID       *uint                    `json:"quartier_id"`
	State            string                   `json:"state"`
	Country          string                   `json:"country"`
	CountryID        *uint                    `json:"country_id"`
	PostalCode       string                   `json:"postal_code"`
	Latitude         float64                  `json:"latitude"`
	Longitude        float64                  `json:"longitude"`
	IndoorFeatures   []string                 `json:"indoor_features"`
	OutdoorFeatures  []string                 `json:"outdoor_features"`
	Amenities        []string                 `json:"amenities"`
	AmenityIDs       []uint                   `json:"amenity_ids"`
	PaperTypes       []string                 `json:"paper_types"`
	Images           []string                 `json:"images"`
	ClassifiedPhotos []models.ClassifiedPhoto `json:"classified_photos"`
	Videos           []string                 `json:"videos"`
	FloorPlans       []models.FloorPlan       `json:"floor_plans"`
	Neighborhood     *models.NeighborhoodInfo `json:"neighborhood"`
	AgentID          *uint                    `json:"agent_id"`
	HostPrivateNote  string                   `json:"host_private_note"`
}
