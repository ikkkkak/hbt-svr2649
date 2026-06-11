package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// resolvePropertySaleOrganizationID returns org id for owner or active member (fast SQL, no Preload).
func resolvePropertySaleOrganizationID(db *sql.DB, userID uint) *uint {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var orgID uint
	err := db.QueryRowContext(ctx,
		"SELECT id FROM organizations WHERE owner_id = $1 AND deleted_at IS NULL LIMIT 1",
		userID,
	).Scan(&orgID)
	if err == nil && orgID > 0 {
		return &orgID
	}

	var memberOrgID uint
	err = db.QueryRowContext(ctx,
		`SELECT organization_id FROM organization_members
		 WHERE user_id = $1 AND status = 'active' AND is_active = true AND deleted_at IS NULL
		 LIMIT 1`,
		userID,
	).Scan(&memberOrgID)
	if err == nil && memberOrgID > 0 {
		return &memberOrgID
	}
	return nil
}

func validateCreatePropertySaleInput(input *struct {
	Title        string
	Description  string
	PropertyType string
	Price        float64
	Area         float64
	Address      string
	City         string
	Images       []string
	Videos       []string
}) string {
	if strings.TrimSpace(input.Title) == "" {
		return "title is required"
	}
	if strings.TrimSpace(input.Description) == "" {
		return "description is required"
	}
	if strings.TrimSpace(input.PropertyType) == "" {
		return "property_type is required"
	}
	if input.Price <= 0 {
		return "price must be greater than 0"
	}
	if input.Area <= 0 {
		return "area must be greater than 0"
	}
	if strings.TrimSpace(input.Address) == "" {
		return "address is required"
	}
	if strings.TrimSpace(input.City) == "" {
		return "city is required"
	}
	if len(input.Images) == 0 && len(input.Videos) == 0 {
		return "at least one image or one video is required"
	}
	return ""
}

// fastInsertPropertySale — minimal row first (fast RETURNING id), full row patched async.
func fastInsertPropertySale(userID uint, p *models.PropertySale) (uint, error) {
	sqlDB, err := storage.SQLDB()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()

	imagesJSON, _ := json.Marshal(p.Images)
	videosJSON, _ := json.Marshal(p.Videos)

	var orgID sql.NullInt64
	if p.OrganizationID != nil && *p.OrganizationID > 0 {
		orgID = sql.NullInt64{Int64: int64(*p.OrganizationID), Valid: true}
	}

	var newID uint
	// Omit price_per_sqft — not present on all deployed DBs (migration 004 may be pending).
	err = sqlDB.QueryRowContext(ctx, `
INSERT INTO property_sales (
  owner_id, organization_id, title, description, property_type, category,
  address, city, state, country, latitude, longitude,
  square_footage, listing_price, currency, status,
  is_verified, is_published, images, videos,
  created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6,
  $7, $8, $9, $10, $11, $12,
  $13, $14, $15, $16,
  false, false, $17::jsonb, $18::jsonb,
  NOW(), NOW()
) RETURNING id`,
		userID, orgID,
		p.Title, p.Description, p.PropertyType, p.Category,
		p.Address, p.City, p.State, p.Country, p.Latitude, p.Longitude,
		p.SquareFootage, p.ListingPrice, p.Currency, p.Status,
		string(imagesJSON), string(videosJSON),
	).Scan(&newID)
	return newID, err
}

// patchPropertySaleDetails fills optional columns after the client already got 201.
func patchPropertySaleDetails(propertyID uint, p *models.PropertySale) {
	sqlDB, err := storage.SQLDB()
	if err != nil {
		log.Printf("⚠️ patchPropertySaleDetails: no db: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	featuresJSON, _ := json.Marshal(p.Features)
	amenitiesJSON, _ := json.Marshal(p.Amenities)
	papersJSON, _ := json.Marshal(p.PaperTypes)
	classifiedJSON, _ := json.Marshal(p.ClassifiedPhotos)
	floorPlansJSON, _ := json.Marshal(p.FloorPlans)
	var neighborhood sql.NullString
	if p.Neighborhood != nil {
		if b, err := json.Marshal(p.Neighborhood); err == nil && len(b) > 2 {
			neighborhood = sql.NullString{String: string(b), Valid: true}
		}
	}
	var agentID sql.NullInt64
	if p.AgentID != nil && *p.AgentID > 0 {
		agentID = sql.NullInt64{Int64: int64(*p.AgentID), Valid: true}
	}

	_, err = sqlDB.ExecContext(ctx, `
UPDATE property_sales SET
  country_id = $2, city_id = $3, zone_id = $4, quartier_id = $5,
  bedrooms = $6, bathrooms = $7, year_built = $8,
  postal_code = $9, host_private_note = $10,
  paper_types = $11::jsonb, features = $12::jsonb, amenities = $13::jsonb,
  classified_photos = $14::jsonb, floor_plans = $15::jsonb, neighborhood = $16::jsonb,
  agent_id = $17, updated_at = NOW()
WHERE id = $1`,
		propertyID, p.CountryID, p.CityID, p.ZoneID, p.QuartierID,
		p.Bedrooms, p.Bathrooms, p.YearBuilt,
		p.PostalCode, p.HostPrivateNote,
		string(papersJSON), string(featuresJSON), string(amenitiesJSON),
		string(classifiedJSON), string(floorPlansJSON), neighborhood,
		agentID,
	)
	if err != nil {
		log.Printf("⚠️ patchPropertySaleDetails id=%d: %v", propertyID, err)
	}
}

func patchPropertySaleOrganization(propertyID uint, organizationID uint) {
	sqlDB, err := storage.SQLDB()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = sqlDB.ExecContext(ctx,
		`UPDATE property_sales SET organization_id = $2, updated_at = NOW() WHERE id = $1`,
		propertyID, organizationID,
	)
	if err != nil {
		log.Printf("⚠️ patchPropertySaleOrganization id=%d org=%d: %v", propertyID, organizationID, err)
	}
}
