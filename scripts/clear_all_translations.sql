-- Clear ALL Arabic and French translations (titles AND descriptions)
-- This sets ar and fr to empty strings, keeping en translations intact
-- Run this before re-running the translation script

-- ============================================
-- PROPERTIES (Rent Properties)
-- ============================================
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
  ),
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  ),
  neighborhood_description_translations = jsonb_set(
    jsonb_set(
      COALESCE(neighborhood_description_translations, '{"en": ""}'::jsonb),
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

-- ============================================
-- PROPERTY SALES
-- ============================================
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
  ),
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE 
  title_translations IS NOT NULL 
  OR description_translations IS NOT NULL;

-- ============================================
-- LANDMARKS
-- ============================================
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
  ),
  description_translations = jsonb_set(
    jsonb_set(
      COALESCE(description_translations, '{"en": ""}'::jsonb),
      '{ar}',
      '""'::jsonb
    ),
    '{fr}',
    '""'::jsonb
  )
WHERE 
  title_translations IS NOT NULL 
  OR description_translations IS NOT NULL;

-- ============================================
-- VERIFICATION QUERY (Check results)
-- ============================================
SELECT 
  'Properties' as table_name,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE title_translations->>'ar' = '' AND title_translations->>'fr' = '') as cleared_titles,
  COUNT(*) FILTER (WHERE description_translations->>'ar' = '' AND description_translations->>'fr' = '') as cleared_descriptions
FROM properties
WHERE title_translations IS NOT NULL OR description_translations IS NOT NULL

UNION ALL

SELECT 
  'Property Sales' as table_name,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE title_translations->>'ar' = '' AND title_translations->>'fr' = '') as cleared_titles,
  COUNT(*) FILTER (WHERE description_translations->>'ar' = '' AND description_translations->>'fr' = '') as cleared_descriptions
FROM property_sales
WHERE title_translations IS NOT NULL OR description_translations IS NOT NULL

UNION ALL

SELECT 
  'Landmarks' as table_name,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE title_translations->>'ar' = '' AND title_translations->>'fr' = '') as cleared_titles,
  COUNT(*) FILTER (WHERE description_translations->>'ar' = '' AND description_translations->>'fr' = '') as cleared_descriptions
FROM landmarks
WHERE title_translations IS NOT NULL OR description_translations IS NOT NULL;

