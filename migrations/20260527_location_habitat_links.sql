-- Link listings zones/quartiers to cadastre plans/sectors for reliable plot lookup.

ALTER TABLE zones
    ADD COLUMN IF NOT EXISTS habitat_plan_id BIGINT REFERENCES habitat_plans(id) ON DELETE SET NULL;

ALTER TABLE quartiers
    ADD COLUMN IF NOT EXISTS habitat_sector_id BIGINT REFERENCES habitat_sectors(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_zones_habitat_plan ON zones(habitat_plan_id);
CREATE INDEX IF NOT EXISTS idx_quartiers_habitat_sector ON quartiers(habitat_sector_id);

-- Backfill zone → plan by name/code
UPDATE zones z
SET habitat_plan_id = p.id
FROM habitat_plans p
WHERE z.habitat_plan_id IS NULL
  AND p.is_active = TRUE
  AND (
    LOWER(TRIM(z.name)) = LOWER(TRIM(p.code))
    OR LOWER(TRIM(z.name)) = LOWER(TRIM(p.name))
    OR LOWER(TRIM(z.name_ar)) = LOWER(TRIM(p.name_ar))
    OR LOWER(TRIM(z.name)) = LOWER(TRIM(p.name))
  );

-- Backfill quartier → sector when zone plan matches
UPDATE quartiers q
SET habitat_sector_id = s.id
FROM habitat_sectors s
JOIN zones z ON z.id = q.zone_id
WHERE q.habitat_sector_id IS NULL
  AND z.habitat_plan_id IS NOT NULL
  AND s.plan_id = z.habitat_plan_id
  AND (
    LOWER(TRIM(q.name)) = LOWER(TRIM(s.name))
    OR LOWER(TRIM(q.name_ar)) = LOWER(TRIM(s.name_ar))
    OR LOWER(TRIM(q.name)) = LOWER(TRIM(s.code))
  );
