-- Migration: Make organization_id nullable in property_sales and landmarks
-- Description: Allows individuals to create properties and lands without an organization

-- Make organization_id nullable in property_sales table
ALTER TABLE property_sales 
    ALTER COLUMN organization_id DROP NOT NULL;

-- Update the foreign key constraint to allow NULL
-- First, drop the existing foreign key constraint
ALTER TABLE property_sales 
    DROP CONSTRAINT IF EXISTS property_sales_organization_id_fkey;

-- Recreate the foreign key constraint with ON DELETE SET NULL for NULL values
ALTER TABLE property_sales 
    ADD CONSTRAINT property_sales_organization_id_fkey 
    FOREIGN KEY (organization_id) 
    REFERENCES organizations(id) 
    ON DELETE SET NULL;

-- Make organization_id nullable in landmarks table (if it exists)
ALTER TABLE landmarks 
    ALTER COLUMN organization_id DROP NOT NULL;

-- Update the foreign key constraint for landmarks to allow NULL
ALTER TABLE landmarks 
    DROP CONSTRAINT IF EXISTS landmarks_organization_id_fkey;

ALTER TABLE landmarks 
    ADD CONSTRAINT landmarks_organization_id_fkey 
    FOREIGN KEY (organization_id) 
    REFERENCES organizations(id) 
    ON DELETE SET NULL;

