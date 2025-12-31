-- Migration: Clear Arabic and French translations
-- Description: Sets ar and fr to empty strings in title_translations and description_translations
-- This allows the translation script to re-translate them properly

-- Clear translations for Properties (rent properties)
UPDATE properties
SET 
  title_translations = jsonb_set(
    jsonb_set(
      COALESCE(title_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  ),
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  ),
  neighborhood_description_translations = jsonb_set(
    jsonb_set(
      COALESCE(neighborhood_description_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE 
  title_translations IS NOT NULL 
  OR description_translations IS NOT NULL
  OR neighborhood_description_translations IS NOT NULL;

-- Clear translations for Property Sales
UPDATE property_sales
SET 
  title_translations = jsonb_set(
    jsonb_set(
      COALESCE(title_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  ),
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE 
  title_translations IS NOT NULL 
  OR description_translations IS NOT NULL;

-- Clear translations for Landmarks
UPDATE landmarks
SET 
  title_translations = jsonb_set(
    jsonb_set(
      COALESCE(title_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  ),
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE 
  title_translations IS NOT NULL 
  OR description_translations IS NOT NULL;

