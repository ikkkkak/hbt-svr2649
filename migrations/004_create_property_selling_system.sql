-- Migration: Create Property Selling System
-- Description: Creates tables for organizations, agents, property sales, tours, and inquiries

-- Create organizations table
CREATE TABLE organizations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    logo VARCHAR(500),
    website VARCHAR(255),
    phone VARCHAR(50),
    email VARCHAR(255),
    address TEXT,
    city VARCHAR(100),
    state VARCHAR(100),
    country VARCHAR(100),
    postal_code VARCHAR(20),
    license_number VARCHAR(100),
    tax_id VARCHAR(100),
    business_type VARCHAR(50) DEFAULT 'brokerage',
    status VARCHAR(50) DEFAULT 'pending',
    is_active BOOLEAN DEFAULT true,
    owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

-- Create agents table
CREATE TABLE agents (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    license_number VARCHAR(100),
    specialization VARCHAR(100),
    experience INTEGER DEFAULT 0,
    bio TEXT,
    languages JSONB DEFAULT '[]',
    status VARCHAR(50) DEFAULT 'pending',
    is_active BOOLEAN DEFAULT true,
    total_sales INTEGER DEFAULT 0,
    total_value DECIMAL(15,2) DEFAULT 0,
    rating DECIMAL(3,2) DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

-- Create property_sales table
CREATE TABLE property_sales (
    id SERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id INTEGER REFERENCES agents(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    property_type VARCHAR(100),
    category VARCHAR(100),
    address TEXT NOT NULL,
    city VARCHAR(100) NOT NULL,
    state VARCHAR(100) NOT NULL,
    country VARCHAR(100) NOT NULL,
    postal_code VARCHAR(20),
    latitude DECIMAL(10,8),
    longitude DECIMAL(11,8),
    bedrooms INTEGER,
    bathrooms INTEGER,
    square_footage INTEGER,
    lot_size DECIMAL(10,2),
    year_built INTEGER,
    parking_spaces INTEGER,
    listing_price DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    price_per_sqft DECIMAL(10,2),
    property_tax DECIMAL(10,2),
    hoa DECIMAL(10,2),
    images JSONB DEFAULT '[]',
    videos JSONB DEFAULT '[]',
    virtual_tour VARCHAR(500),
    floor_plan VARCHAR(500),
    features JSONB DEFAULT '[]',
    amenities JSONB DEFAULT '[]',
    status VARCHAR(50) DEFAULT 'draft',
    is_verified BOOLEAN DEFAULT false,
    is_published BOOLEAN DEFAULT false,
    is_featured BOOLEAN DEFAULT false,
    verified_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    verified_at TIMESTAMP NULL,
    verification_notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

-- Create property_tours table
CREATE TABLE property_tours (
    id SERIAL PRIMARY KEY,
    property_sale_id INTEGER NOT NULL REFERENCES property_sales(id) ON DELETE CASCADE,
    customer_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tour_date TIMESTAMP NOT NULL,
    tour_time VARCHAR(10),
    duration INTEGER DEFAULT 60,
    tour_type VARCHAR(50) DEFAULT 'in_person',
    status VARCHAR(50) DEFAULT 'pending',
    customer_notes TEXT,
    agent_notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

-- Create property_inquiries table
CREATE TABLE property_inquiries (
    id SERIAL PRIMARY KEY,
    property_sale_id INTEGER NOT NULL REFERENCES property_sales(id) ON DELETE CASCADE,
    customer_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject VARCHAR(255),
    message TEXT NOT NULL,
    inquiry_type VARCHAR(50) DEFAULT 'general',
    status VARCHAR(50) DEFAULT 'new',
    response TEXT,
    responded_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    responded_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL
);

-- Create indexes for better performance
CREATE INDEX idx_organizations_owner_id ON organizations(owner_id);
CREATE INDEX idx_organizations_status ON organizations(status);
CREATE INDEX idx_agents_user_id ON agents(user_id);
CREATE INDEX idx_agents_organization_id ON agents(organization_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_property_sales_organization_id ON property_sales(organization_id);
CREATE INDEX idx_property_sales_agent_id ON property_sales(agent_id);
CREATE INDEX idx_property_sales_status ON property_sales(status);
CREATE INDEX idx_property_sales_city_state ON property_sales(city, state);
CREATE INDEX idx_property_sales_price ON property_sales(listing_price);
CREATE INDEX idx_property_tours_property_id ON property_tours(property_sale_id);
CREATE INDEX idx_property_tours_customer_id ON property_tours(customer_id);
CREATE INDEX idx_property_tours_date ON property_tours(tour_date);
CREATE INDEX idx_property_inquiries_property_id ON property_inquiries(property_sale_id);
CREATE INDEX idx_property_inquiries_customer_id ON property_inquiries(customer_id);

-- Add constraints
ALTER TABLE organizations ADD CONSTRAINT chk_organizations_status 
    CHECK (status IN ('pending', 'approved', 'rejected', 'suspended'));

ALTER TABLE agents ADD CONSTRAINT chk_agents_status 
    CHECK (status IN ('pending', 'approved', 'rejected', 'suspended'));

ALTER TABLE property_sales ADD CONSTRAINT chk_property_sales_status 
    CHECK (status IN ('draft', 'pending_verification', 'verified', 'published', 'sold', 'withdrawn'));

ALTER TABLE property_tours ADD CONSTRAINT chk_property_tours_status 
    CHECK (status IN ('pending', 'confirmed', 'completed', 'cancelled', 'no_show'));

ALTER TABLE property_inquiries ADD CONSTRAINT chk_property_inquiries_status 
    CHECK (status IN ('new', 'responded', 'closed'));

-- Add unique constraints
ALTER TABLE organizations ADD CONSTRAINT uq_organizations_owner_id UNIQUE (owner_id);
ALTER TABLE agents ADD CONSTRAINT uq_agents_user_id UNIQUE (user_id);

-- Insert sample data for testing
INSERT INTO organizations (name, description, owner_id, status, business_type) VALUES
('Premium Real Estate', 'Luxury property specialists in Nouakchott', 1, 'approved', 'brokerage'),
('Mauritania Properties', 'Affordable housing solutions', 2, 'approved', 'agency');

-- Update timestamps trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_organizations_updated_at BEFORE UPDATE ON organizations FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_agents_updated_at BEFORE UPDATE ON agents FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_property_sales_updated_at BEFORE UPDATE ON property_sales FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_property_tours_updated_at BEFORE UPDATE ON property_tours FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_property_inquiries_updated_at BEFORE UPDATE ON property_inquiries FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
