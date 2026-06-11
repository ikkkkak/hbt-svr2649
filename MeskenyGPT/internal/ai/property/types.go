package property

// Property represents a raw DB property row (sale/rent or landmark).
type Property struct {
	ID       uint
	Title    string
	Price    float64
	Currency string
	City     string
	Bedrooms int
	Image    string
	Type     string // "rent" or "sale"
	Source   string // "property_sale" or "landmark"
	// Landmark-only (optional)
	Area          float64
	LocationLabel string
	Lat           float64
	Lng           float64
	PlotNumber    string
	PlotCorners   []LatLng
	CadastreLinked bool
	QuartierLabel string
}

// Filters is the structured search request used by the property layer.
type Filters struct {
	City       string
	Zone       string
	Quartier   string
	QuartierID uint
	PlotNumber string
	Type       string
	Purpose    string
	BudgetMin  float64
	BudgetMax  float64
	Query      string // Raw user message for title/description matching (landmarks)
}

// Card is the shape that the mobile app expects in AI responses.
type Card struct {
	ID       uint    `json:"id"`
	Title    string  `json:"title"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
	City     string  `json:"city"`
	Bedrooms int     `json:"bedrooms"`
	Image    string  `json:"image"`
	Type     string  `json:"type"`   // "rent" or "sale"
	Source   string  `json:"source"` // "property_sale" | "landmark" — for routing to correct detail screen
	// Optional map / land fields
	SizeM2        float64  `json:"size_m2,omitempty"`
	LocationLabel string   `json:"location_label,omitempty"`
	Lat           float64  `json:"lat,omitempty"`
	Lng           float64  `json:"lng,omitempty"`
	PlotNumber    string   `json:"plot_number,omitempty"`
	PlotCorners   []LatLng `json:"plot_corners,omitempty"`
	CadastreLinked bool    `json:"cadastre_linked,omitempty"`
	QuartierLabel string  `json:"quartier_label,omitempty"`
}

