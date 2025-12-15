-- 010_add_property_translations.sql
-- Adds JSONB translation fields for rent properties, property sales, and landmarks.

-- Rent properties
ALTER TABLE properties
  ADD COLUMN IF NOT EXISTS title_translations JSONB DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS description_translations JSONB DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS neighborhood_description_translations JSONB DEFAULT '{}'::jsonb;

-- Property sales
ALTER TABLE property_sales
  ADD COLUMN IF NOT EXISTS title_translations JSONB DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS description_translations JSONB DEFAULT '{}'::jsonb;

-- Landmarks
ALTER TABLE landmarks
  ADD COLUMN IF NOT EXISTS title_translations JSONB DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS description_translations JSONB DEFAULT '{}'::jsonb;


