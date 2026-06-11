-- Countries as parent of cities → zones → quartiers; origin country on listings.

CREATE TABLE IF NOT EXISTS countries (
    id SERIAL PRIMARY KEY,
    code VARCHAR(8) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL,
    name_ar VARCHAR(128) NOT NULL DEFAULT '',
    name_fr VARCHAR(128) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_countries_active_sort ON countries (is_active, sort_order, name);

-- Cities belong to a country
ALTER TABLE cities ADD COLUMN IF NOT EXISTS country_id INT REFERENCES countries(id);
CREATE INDEX IF NOT EXISTS idx_cities_country_id ON cities (country_id);

-- Listings: canonical origin country (denormalized for fast filters)
ALTER TABLE properties ADD COLUMN IF NOT EXISTS country_id INT REFERENCES countries(id);
CREATE INDEX IF NOT EXISTS idx_properties_country_id ON properties (country_id);

ALTER TABLE property_sales ADD COLUMN IF NOT EXISTS country_id INT REFERENCES countries(id);
CREATE INDEX IF NOT EXISTS idx_property_sales_country_id ON property_sales (country_id);

ALTER TABLE landmarks ADD COLUMN IF NOT EXISTS country_id INT REFERENCES countries(id);
CREATE INDEX IF NOT EXISTS idx_landmarks_country_id ON landmarks (country_id);

-- Seed Mauritania and link existing Mauritanian cities
INSERT INTO countries (code, name, name_ar, name_fr, is_active, sort_order)
VALUES ('MR', 'Mauritania', 'موريتانيا', 'Mauritanie', TRUE, 0)
ON CONFLICT (code) DO NOTHING;

UPDATE cities
SET country_id = (SELECT id FROM countries WHERE code = 'MR' LIMIT 1)
WHERE country_id IS NULL
  AND (
    LOWER(TRIM(COALESCE(country, ''))) IN ('mauritania', 'mauritanie', 'موريتانيا', '')
    OR country IS NULL
    OR country = ''
  );

UPDATE properties p
SET country_id = c.country_id
FROM cities c
WHERE p.city_id = c.id AND p.country_id IS NULL AND c.country_id IS NOT NULL;

UPDATE property_sales ps
SET country_id = c.country_id
FROM cities c
WHERE ps.city_id = c.id AND ps.country_id IS NULL AND c.country_id IS NOT NULL;

UPDATE landmarks l
SET country_id = c.country_id
FROM cities c
WHERE l.city_id = c.id AND l.country_id IS NULL AND c.country_id IS NOT NULL;

-- Fallback: match text country on properties without city_id
UPDATE properties
SET country_id = (SELECT id FROM countries WHERE code = 'MR' LIMIT 1)
WHERE country_id IS NULL
  AND LOWER(TRIM(COALESCE(country, ''))) IN ('mauritania', 'mauritanie', 'موريتانيا');

UPDATE property_sales
SET country_id = (SELECT id FROM countries WHERE code = 'MR' LIMIT 1)
WHERE country_id IS NULL
  AND LOWER(TRIM(COALESCE(country, ''))) IN ('mauritania', 'mauritanie', 'موريتانيا');
