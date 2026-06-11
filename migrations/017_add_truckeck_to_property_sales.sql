-- Add truckeck column to property_sales (quality control validated by admin)
ALTER TABLE property_sales ADD COLUMN IF NOT EXISTS truckeck BOOLEAN DEFAULT FALSE;
