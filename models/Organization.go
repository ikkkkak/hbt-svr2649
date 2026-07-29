package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Organization represents a real estate organization/brokerage
type Organization struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description"`
	Logo        string `json:"logo"`
	BannerImage string `json:"banner_image"`
	Website     string `json:"website"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	City        string `json:"city"`
	State       string `json:"state"`
	Country     string `json:"country"`
	PostalCode  string `json:"postal_code"`

	// Business Information
	LicenseNumber string `json:"license_number"`
	TaxID         string `json:"tax_id"`
	BusinessType  string `json:"business_type"` // "brokerage", "agency", "individual"

	// Status
	Status   string `json:"status" gorm:"default:'pending'"` // pending, approved, rejected, suspended
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Owner Information
	OwnerID uint `json:"owner_id" gorm:"not null"`
	Owner   User `json:"owner" gorm:"foreignKey:OwnerID"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Edit tracking - 30 day cooldown per field
	LastNameEdit        *time.Time `json:"last_name_edit"`
	LastDescriptionEdit *time.Time `json:"last_description_edit"`
	LastBusinessTypeEdit *time.Time `json:"last_business_type_edit"`
	LastBannerEdit      *time.Time `json:"last_banner_edit"`
	LastLogoEdit        *time.Time `json:"last_logo_edit"`

	// Relationships
	Agents     []Agent        `json:"agents" gorm:"foreignKey:OrganizationID"`
	Properties []PropertySale `json:"properties" gorm:"foreignKey:OrganizationID"`
}

// Agent represents an agent working for an organization
type Agent struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	UserID         uint         `json:"user_id" gorm:"not null;unique"`
	User           User         `json:"user" gorm:"foreignKey:UserID"`
	OrganizationID uint         `json:"organization_id" gorm:"not null"`
	Organization   Organization `json:"organization" gorm:"foreignKey:OrganizationID"`

	// Agent Information
	LicenseNumber  string   `json:"license_number"`
	Specialization string   `json:"specialization"` // residential, commercial, luxury, etc.
	Experience     int      `json:"experience"`     // years of experience
	Bio            string   `json:"bio"`
	Languages      []string `json:"languages" gorm:"type:json"`

	// Status
	Status   string `json:"status" gorm:"default:'pending'"` // pending, approved, rejected, suspended
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Performance Metrics
	TotalSales int     `json:"total_sales" gorm:"default:0"`
	TotalValue float64 `json:"total_value" gorm:"default:0"`
	Rating     float64 `json:"rating" gorm:"default:0"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	AssignedProperties []PropertySale `json:"assigned_properties" gorm:"foreignKey:AgentID"`
}

// PropertySale represents a property for sale
type PropertySale struct {
	ID             uint          `json:"id" gorm:"primaryKey"`
	OrganizationID *uint         `json:"organization_id"` // Optional - can be nil for individual owners
	Organization   *Organization `json:"organization" gorm:"foreignKey:OrganizationID"`
	OwnerID        *uint         `json:"owner_id"` // Optional - tracks individual owner when organization_id is nil
	Owner          *User         `json:"owner" gorm:"foreignKey:OwnerID"`
	AgentID        *uint        `json:"agent_id"` // Optional - can be assigned later
	Agent          *Agent       `json:"agent" gorm:"foreignKey:AgentID"`

	// Property Information
	Title        string `json:"title" gorm:"not null"`
	Description  string `json:"description"`
	PropertyType string `json:"property_type"` // house, apartment, commercial, land, etc.
	Category     string `json:"category"`      // residential, commercial, industrial, etc.

	// Stored translations for title/description
	TitleTranslations       datatypes.JSON `json:"title_translations" gorm:"column:title_translations;type:jsonb"`
	DescriptionTranslations datatypes.JSON `json:"description_translations" gorm:"column:description_translations;type:jsonb"`

	// Location
	Address      string  `json:"address" gorm:"not null"`
	City         string  `json:"city" gorm:"not null"`
	CityID       *uint   `json:"city_id" gorm:"column:city_id"`
	ZoneID       *uint   `json:"zone_id" gorm:"column:zone_id"`
	QuartierID    *uint   `json:"quartier_id" gorm:"column:quartier_id"`
	SubQuartierID *uint   `json:"sub_quartier_id" gorm:"column:sub_quartier_id"`
	CountryID    *uint   `json:"country_id" gorm:"column:country_id;index"`
	State        string  `json:"state" gorm:"not null"`
	Country      string  `json:"country" gorm:"not null"`
	PostalCode   string  `json:"postal_code"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`

	// Property Details
	Bedrooms      int     `json:"bedrooms"`
	Bathrooms     int     `json:"bathrooms"`
	SquareFootage int     `json:"square_footage"`
	LotSize       float64 `json:"lot_size"`
	YearBuilt     int     `json:"year_built"`
	ParkingSpaces int     `json:"parking_spaces"`

	// Financial Information
	ListingPrice float64 `json:"listing_price" gorm:"not null"`
	Currency     string  `json:"currency" gorm:"default:'USD'"`
	PricePerSqFt float64 `json:"price_per_sqft"`
	PropertyTax  float64 `json:"property_tax"`
	HOA          float64 `json:"hoa"`

	// Media
	Images      []string `json:"images" gorm:"type:jsonb;serializer:json"`
	Videos      []string `json:"videos" gorm:"type:jsonb;serializer:json"`
	// Canonical video rows with HLS URLs (loaded on detail/list responses, not persisted here)
	SaleVideos []PropertySaleVideo `json:"saleVideos,omitempty" gorm:"-"`
	VirtualTour string   `json:"virtual_tour"`
	// Classified photos by room type (separate from main images)
	ClassifiedPhotos []ClassifiedPhoto `json:"classified_photos" gorm:"type:jsonb;serializer:json"`
	// Detailed floor plans (per floor) and neighborhood info
	FloorPlans   []FloorPlan       `json:"floor_plans" gorm:"type:jsonb;serializer:json"`
	Neighborhood *NeighborhoodInfo `json:"neighborhood" gorm:"type:jsonb;serializer:json"`

	// Features and Amenities
	Features  []string `json:"features" gorm:"type:jsonb;serializer:json"`
	Amenities []string `json:"amenities" gorm:"type:jsonb;serializer:json"` // Legacy - kept for backward compatibility
	// Fraud-prevention paper labels selected by the lister (e.g. titre foncier, quitane, lettre).
	PaperTypes []string `json:"paper_types" gorm:"type:jsonb;serializer:json"`

	// Amenities with translations (Many2Many relationship)
	AmenityList []Amenity `json:"amenity_list" gorm:"many2many:property_sale_amenities;"`

	// AmenityIDs exposes amenity IDs for frontend mapping (not persisted)
	AmenityIDs []uint `json:"amenity_ids" gorm:"-"`

	// Status and Verification
	Status      string `json:"status" gorm:"default:'draft'"` // draft, pending_verification, verified, published, sold, withdrawn
	IsVerified  bool   `json:"is_verified" gorm:"default:false"`
	IsPublished bool   `json:"is_published" gorm:"default:false"`
	IsFeatured  bool   `json:"is_featured" gorm:"default:false"`
	// Admin-only: high-priority distribution across feeds, discovery, and analytics.
	IsGold bool `json:"is_gold" gorm:"default:false;index"`
	// Admin-curated deal flag used for targeted "investment opportunity" pushes.
	IsInvestmentOpportunity bool `json:"is_investment_opportunity" gorm:"default:false;index"`
	// Truckeck: quality control validated by admin (Oqood of this property confirmed)
	Truckeck bool `json:"truckeck" gorm:"default:false"`

	// Property Management
	IsDeactivated bool   `json:"is_deactivated" gorm:"default:false;index"` // Host can deactivate property
	IsSold        bool   `json:"is_sold" gorm:"default:false;index"`       // Host can mark as sold
	
	// View Count - Tracks total views (excluding owner views)
	ViewCount int64 `json:"view_count" gorm:"default:0;index"`
	
	// Last milestone notified (to avoid duplicate notifications)
	LastMilestoneNotified int64 `json:"last_milestone_notified" gorm:"default:0"`

	// Host-only note (never show to guests; redact on public API responses)
	HostPrivateNote string `json:"host_private_note,omitempty" gorm:"type:text;column:host_private_note"`

	// Verification Information
	VerifiedBy        *uint      `json:"verified_by"`
	VerifiedAt        *time.Time `json:"verified_at"`
	VerificationNotes string     `json:"verification_notes"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	TourBookings []PropertyTour    `json:"tour_bookings" gorm:"foreignKey:PropertySaleID"`
	Inquiries    []PropertyInquiry `json:"inquiries" gorm:"foreignKey:PropertySaleID"`
	CityRef      *City             `json:"cityRef" gorm:"foreignKey:CityID;references:ID"`
	ZoneRef      *Zone             `json:"zoneRef" gorm:"foreignKey:ZoneID;references:ID"`
	QuartierRef  *Quartier         `json:"quartierRef" gorm:"foreignKey:QuartierID;references:ID"`
}

// ClassifiedPhoto represents a photo classified by room type
type ClassifiedPhoto struct {
	RoomType string   `json:"room_type"` // e.g., "kitchen", "hall", "bedroom", "bathroom", etc.
	Photos   []string `json:"photos"`    // Array of photo URLs for this room type
}

// FloorPlan describes a single floor layout and details
type FloorPlan struct {
	Name        string   `json:"name"` // e.g., "Ground Floor", "First Floor"
	Bedrooms    int      `json:"bedrooms"`
	Bathrooms   int      `json:"bathrooms"`
	Kitchens    int      `json:"kitchens"`
	LivingRooms int      `json:"living_rooms"`
	Halls       int      `json:"halls"`
	Balconies   int      `json:"balconies"`
	AreaSqm     float64  `json:"area_sqm"`
	Notes       string   `json:"notes"`  // free-form notes about this floor
	Images      []string `json:"images"` // uploaded image URLs for the floor plan
}

// NeighborhoodInfo captures subjective details like noise levels and nearby notes
type NeighborhoodInfo struct {
	NoiseLevel   string `json:"noise_level"` // e.g., "Very quiet", "Moderate", "Lively"
	SafetyLevel  string `json:"safety_level"`
	TrafficLevel string `json:"traffic_level"`
	Notes        string `json:"notes"` // free text about the neighborhood
}

// PropertyTour represents a tour booking for a property
type PropertyTour struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	PropertySaleID uint         `json:"property_sale_id" gorm:"not null"`
	PropertySale   PropertySale `json:"property_sale" gorm:"foreignKey:PropertySaleID"`

	// Customer Information
	CustomerID uint `json:"customer_id" gorm:"not null"`
	Customer   User `json:"customer" gorm:"foreignKey:CustomerID"`

	// Tour Details
	TourDate time.Time `json:"tour_date" gorm:"not null"`
	TourTime string    `json:"tour_time"` // "09:00", "14:30", etc.
	Duration int       `json:"duration"`  // minutes
	TourType string    `json:"tour_type"` // in_person, virtual, video_call

	// Status
	Status string `json:"status" gorm:"default:'pending'"` // pending, confirmed, completed, cancelled, no_show

	// Notes
	CustomerNotes string `json:"customer_notes"`
	AgentNotes    string `json:"agent_notes"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// PropertyInquiry represents an inquiry about a property
type PropertyInquiry struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	PropertySaleID uint         `json:"property_sale_id" gorm:"not null"`
	PropertySale   PropertySale `json:"property_sale" gorm:"foreignKey:PropertySaleID"`

	// Customer Information
	CustomerID uint `json:"customer_id" gorm:"not null"`
	Customer   User `json:"customer" gorm:"foreignKey:CustomerID"`

	// Inquiry Details
	Subject     string `json:"subject"`
	Message     string `json:"message" gorm:"not null"`
	InquiryType string `json:"inquiry_type"` // general, pricing, availability, financing

	// Status
	Status string `json:"status" gorm:"default:'new'"` // new, responded, closed

	// Response
	Response    string     `json:"response"`
	RespondedBy *uint      `json:"responded_by"`
	RespondedAt *time.Time `json:"responded_at"`

	// Timestamps
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// PropertyOffer represents a user's purchase offer on a property for sale
type PropertyOffer struct {
	ID         uint         `json:"id" gorm:"primaryKey"`
	PropertyID uint         `json:"property_id" gorm:"index;not null"`
	Property   PropertySale `json:"-" gorm:"foreignKey:PropertyID;constraint:OnDelete:CASCADE"`
	UserID     uint         `json:"user_id" gorm:"index;not null"`
	User       User         `json:"user" gorm:"foreignKey:UserID"`
	Amount     float64      `json:"amount" gorm:"not null"`
	Message    string       `json:"message"`
	Status     string       `json:"status" gorm:"default:'pending'"` // pending, accepted, rejected, withdrawn
	CreatedAt  time.Time    `json:"created_at"`
}

// LandmarkPublisherContact is attached by public landmark APIs (organization or individual owner).
type LandmarkPublisherContact struct {
	Type    string `json:"type"` // organization | individual
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Website string `json:"website,omitempty"`
}

// Landmark represents a custom land plot with full property information
type Landmark struct {
	ID             uint          `json:"id" gorm:"primaryKey"`
	OrganizationID *uint         `json:"organization_id" gorm:"index"` // Optional - can be nil for individual owners
	Organization   *Organization `json:"organization" gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE"`
	OwnerID        *uint         `json:"owner_id"` // Optional - tracks individual owner when organization_id is nil
	Owner          *User         `json:"owner" gorm:"foreignKey:OwnerID"`

	// Basic Information
	Title       string         `json:"title" gorm:"not null"`
	Description string         `json:"description" gorm:"type:text"`
	Images      datatypes.JSON `json:"images" gorm:"type:json"` // Array of image URLs
	VideoURL    *string        `json:"video_url" gorm:"column:video_url"`         // Optional video URL
	MediaType   string         `json:"media_type" gorm:"column:media_type;default:'images'"` // images | video | both

	// Stored translations
	TitleTranslations       datatypes.JSON `json:"title_translations" gorm:"column:title_translations;type:jsonb"`
	DescriptionTranslations datatypes.JSON `json:"description_translations" gorm:"column:description_translations;type:jsonb"`

	// Land Details
	Area      float64        `json:"area"` // in square meters
	AreaUnit  string         `json:"area_unit" gorm:"default:'sqm'"`
	LandType  string         `json:"land_type"` // residential, commercial, agricultural, etc.
	Zoning    string         `json:"zoning"`
	Utilities datatypes.JSON `json:"utilities" gorm:"type:json"` // Available utilities

	// Optional: number of lots / parcels
	Lots *int `json:"lots,omitempty"`

	// Location Coordinates (4 points forming the plot)
	// Optional: may be nil if user skips “highlight location” step
	Point1Lat *float64 `json:"point1_lat,omitempty"`
	Point1Lng *float64 `json:"point1_lng,omitempty"`
	Point2Lat *float64 `json:"point2_lat,omitempty"`
	Point2Lng *float64 `json:"point2_lng,omitempty"`
	Point3Lat *float64 `json:"point3_lat,omitempty"`
	Point3Lng *float64 `json:"point3_lng,omitempty"`
	Point4Lat *float64 `json:"point4_lat,omitempty"`
	Point4Lng *float64 `json:"point4_lng,omitempty"`

	// Structured location (country → city → zone → quartier)
	CountryID  *uint `json:"country_id" gorm:"index"`
	CityID     *uint `json:"city_id" gorm:"index"`
	ZoneID     *uint `json:"zone_id" gorm:"index"`
	QuartierID    *uint `json:"quartier_id" gorm:"index"`
	SubQuartierID *uint `json:"sub_quartier_id" gorm:"index"` // optional finer sub-quartier / sub-sector

	// Cadastre: host-confirmed Habitat GIS plot
	HabitatPlotID *uint `json:"habitat_plot_id" gorm:"index"`
	PlotConfirmed bool  `json:"plot_confirmed" gorm:"default:false"`

	// Extended Land meta
	District        string         `json:"district"`               // e.g., Tevragh-Zeina
	Region          string         `json:"region"`                 // e.g., NCE Secteur 3
	PlotNumber      string         `json:"plot_number"`            // e.g., 15
	ElevationMeters float64        `json:"elevation_m"`            // e.g., +0.48
	Sides           datatypes.JSON `json:"sides" gorm:"type:json"` // e.g., ["20m","35m","20m","35m"]

	// Pricing
	Price    float64 `json:"price" gorm:"default:0"`
	Currency string  `json:"currency" gorm:"default:'MRU'"`

	// Property Papers & Verification
	PropertyPapers    datatypes.JSON `json:"property_papers" gorm:"type:json"` // Array of document URLs
	// Fraud-prevention paper labels selected by the lister (e.g. titre foncier, quitane, lettre).
	PaperTypes        []string       `json:"paper_types" gorm:"type:jsonb;serializer:json"`
	IsVerified        bool           `json:"is_verified" gorm:"default:false"`
	VerifiedAt        *time.Time     `json:"verified_at"`
	VerifiedBy        *uint          `json:"verified_by"` // Admin user ID who verified
	VerificationNotes string         `json:"verification_notes" gorm:"type:text"`

	// Status
	Status                  string `json:"status" gorm:"default:'draft'"` // draft, pending_verification, verified, rejected, inactive
	IsPublished             bool   `json:"is_published" gorm:"default:false"`
	IsInvestmentOpportunity bool   `json:"is_investment_opportunity" gorm:"default:false;index"`
	IsGoodDeal              bool   `json:"is_good_deal" gorm:"default:false;index"`
	IsGold                  bool   `json:"is_gold" gorm:"default:false;index"`

	// Host-only note (never show to guests; redact on public API responses)
	HostPrivateNote string `json:"host_private_note,omitempty" gorm:"type:text;column:host_private_note"`

	// PublisherContact is set by API handlers for guests (not a DB column).
	PublisherContact *LandmarkPublisherContact `json:"publisherContact,omitempty" gorm:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
