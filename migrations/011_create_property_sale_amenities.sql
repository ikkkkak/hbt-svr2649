-- Migration: Create Property Sale Amenities Junction Table
-- Description: Creates junction table to link property sales with amenities (with translations)

-- Create property_sale_amenities junction table
CREATE TABLE IF NOT EXISTS property_sale_amenities (
    property_sale_id INTEGER NOT NULL REFERENCES property_sales(id) ON DELETE CASCADE,
    amenity_id INTEGER NOT NULL REFERENCES amenities(id) ON DELETE CASCADE,
    PRIMARY KEY (property_sale_id, amenity_id)
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_property_sale_amenities_property_sale_id ON property_sale_amenities(property_sale_id);
CREATE INDEX IF NOT EXISTS idx_property_sale_amenities_amenity_id ON property_sale_amenities(amenity_id);

-- Note: The amenities table already exists with translations (name JSONB with en/fr/ar)

