package places

// PlaceType is the kind of nearby place (restaurant, hospital, school).
const (
	TypeRestaurant = "restaurant"
	TypeHospital   = "hospital"
	TypeSchool     = "school"
)

// PlaceTypes are the types we fetch from Google.
var PlaceTypes = []string{TypeRestaurant, TypeHospital, TypeSchool}

// NearbyPlace is the DTO returned to the frontend for one place.
type NearbyPlace struct {
	Name      string  `json:"name"`
	Rating    float64 `json:"rating"`
	Reviews   int     `json:"reviews"`
	Address   string  `json:"address"`
	Phone     string  `json:"phone,omitempty"`
	Photo     string  `json:"photo,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	DistanceKm float64 `json:"distance_km,omitempty"`
	Website   string  `json:"website,omitempty"`
}

// NearbyResponse is the GET /property-sales/:id/nearby response.
type NearbyResponse struct {
	Restaurants []NearbyPlace `json:"restaurants"`
	Hospitals   []NearbyPlace `json:"hospitals"`
	Schools     []NearbyPlace `json:"schools"`
}
