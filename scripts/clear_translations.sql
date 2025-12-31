-- Quick SQL script to clear Arabic and French translations
-- Run this before re-running the translation script
-- This sets ar and fr to empty strings, keeping en translations intact

-- Properties (rent)
UPDATE properties
SET 
  title_translations = jsonb_set(
    jsonb_set(
      COALESCE(title_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE title_translations IS NOT NULL;

UPDATE properties
SET 
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE description_translations IS NOT NULL;

UPDATE properties
SET 
  neighborhood_description_translations = jsonb_set(
    jsonb_set(
      COALESCE(neighborhood_description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE neighborhood_description_translations IS NOT NULL;

-- Property Sales
UPDATE property_sales
SET 
  title_translations = jsonb_set(
    jsonb_set(
      COALESCE(title_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE title_translations IS NOT NULL;

UPDATE property_sales
SET 
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE description_translations IS NOT NULL;

-- Landmarks
UPDATE landmarks
SET 
  title_translations = jsonb_set(
    jsonb_set(
      COALESCE(title_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE title_translations IS NOT NULL;

UPDATE landmarks
SET 
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE description_translations IS NOT NULL;

-- Summary
SELECT 
  'Properties' as table_name,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE title_translations->>'ar' = '' OR title_translations->>'fr' = '') as cleared_title,
  COUNT(*) FILTER (WHERE description_translations->>'ar' = '' OR description_translations->>'fr' = '') as cleared_description
FROM properties
WHERE title_translations IS NOT NULL OR description_translations IS NOT NULL

UNION ALL

SELECT 
  'Property Sales' as table_name,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE title_translations->>'ar' = '' OR title_translations->>'fr' = '') as cleared_title,
  COUNT(*) FILTER (WHERE description_translations->>'ar' = '' OR description_translations->>'fr' = '') as cleared_description
FROM property_sales
WHERE title_translations IS NOT NULL OR description_translations IS NOT NULL

UNION ALL

SELECT 
  'Landmarks' as table_name,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE title_translations->>'ar' = '' OR title_translations->>'fr' = '') as cleared_title,
  COUNT(*) FILTER (WHERE description_translations->>'ar' = '' OR description_translations->>'fr' = '') as cleared_description
FROM landmarks
WHERE title_translations IS NOT NULL OR description_translations IS NOT NULL;

