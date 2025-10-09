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
	ID             uint         `json:"id" gorm:"primaryKey"`
	OrganizationID uint         `json:"organization_id" gorm:"not null"`
	Organization   Organization `json:"organization" gorm:"foreignKey:OrganizationID"`
	AgentID        *uint        `json:"agent_id"` // Optional - can be assigned later
	Agent          *Agent       `json:"agent" gorm:"foreignKey:AgentID"`

	// Property Information
	Title        string `json:"title" gorm:"not null"`
	Description  string `json:"description"`
	PropertyType string `json:"property_type"` // house, apartment, commercial, land, etc.
	Category     string `json:"category"`      // residential, commercial, industrial, etc.

	// Location
	Address    string  `json:"address" gorm:"not null"`
	City       string  `json:"city" gorm:"not null"`
	State      string  `json:"state" gorm:"not null"`
	Country    string  `json:"country" gorm:"not null"`
	PostalCode string  `json:"postal_code"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`

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
	VirtualTour string   `json:"virtual_tour"`
	// Detailed floor plans (per floor) and neighborhood info
	FloorPlans   []FloorPlan       `json:"floor_plans" gorm:"type:jsonb;serializer:json"`
	Neighborhood *NeighborhoodInfo `json:"neighborhood" gorm:"type:jsonb;serializer:json"`

	// Features and Amenities
	Features  []string `json:"features" gorm:"type:jsonb;serializer:json"`
	Amenities []string `json:"amenities" gorm:"type:jsonb;serializer:json"`

	// Status and Verification
	Status      string `json:"status" gorm:"default:'draft'"` // draft, pending_verification, verified, published, sold, withdrawn
	IsVerified  bool   `json:"is_verified" gorm:"default:false"`
	IsPublished bool   `json:"is_published" gorm:"default:false"`
	IsFeatured  bool   `json:"is_featured" gorm:"default:false"`

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

// Landmark represents a custom land plot with full property information
type Landmark struct {
	ID             uint         `json:"id" gorm:"primaryKey"`
	OrganizationID uint         `json:"organization_id" gorm:"index;not null"`
	Organization   Organization `json:"organization" gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE"`

	// Basic Information
	Title       string         `json:"title" gorm:"not null"`
	Description string         `json:"description" gorm:"type:text"`
	Images      datatypes.JSON `json:"images" gorm:"type:json"` // Array of image URLs

	// Land Details
	Area      float64        `json:"area"` // in square meters
	AreaUnit  string         `json:"area_unit" gorm:"default:'sqm'"`
	LandType  string         `json:"land_type"` // residential, commercial, agricultural, etc.
	Zoning    string         `json:"zoning"`
	Utilities datatypes.JSON `json:"utilities" gorm:"type:json"` // Available utilities

	// Location Coordinates (4 points forming the plot)
	Point1Lat float64 `json:"point1_lat" gorm:"not null"`
	Point1Lng float64 `json:"point1_lng" gorm:"not null"`
	Point2Lat float64 `json:"point2_lat" gorm:"not null"`
	Point2Lng float64 `json:"point2_lng" gorm:"not null"`
	Point3Lat float64 `json:"point3_lat" gorm:"not null"`
	Point3Lng float64 `json:"point3_lng" gorm:"not null"`
	Point4Lat float64 `json:"point4_lat" gorm:"not null"`
	Point4Lng float64 `json:"point4_lng" gorm:"not null"`

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
	IsVerified        bool           `json:"is_verified" gorm:"default:false"`
	VerifiedAt        *time.Time     `json:"verified_at"`
	VerifiedBy        *uint          `json:"verified_by"` // Admin user ID who verified
	VerificationNotes string         `json:"verification_notes" gorm:"type:text"`

	// Status
	Status      string `json:"status" gorm:"default:'draft'"` // draft, pending_verification, verified, rejected, inactive
	IsPublished bool   `json:"is_published" gorm:"default:false"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
