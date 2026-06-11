-- Habitat GIS: enforce Plan → Sector → Plot relations (denormalized plan_id on plots).

-- Denormalized plan_id on plots (speeds plan-level queries; must match sector.plan_id)
ALTER TABLE habitat_plots
    ADD COLUMN IF NOT EXISTS plan_id BIGINT;

UPDATE habitat_plots p
SET plan_id = s.plan_id
FROM habitat_sectors s
WHERE p.sector_id = s.id
  AND (p.plan_id IS NULL OR p.plan_id <> s.plan_id);

ALTER TABLE habitat_plots
    ALTER COLUMN plan_id SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'habitat_plots_plan_id_fkey'
    ) THEN
        ALTER TABLE habitat_plots
            ADD CONSTRAINT habitat_plots_plan_id_fkey
            FOREIGN KEY (plan_id) REFERENCES habitat_plans(id) ON DELETE CASCADE;
    END IF;
END $$;

-- sector_id FK (idempotent; table may have been created without named constraint)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'habitat_plots_sector_id_fkey'
    ) THEN
        ALTER TABLE habitat_plots
            ADD CONSTRAINT habitat_plots_sector_id_fkey
            FOREIGN KEY (sector_id) REFERENCES habitat_sectors(id) ON DELETE CASCADE;
    END IF;
END $$;

-- Unique plot number per sector
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'habitat_plots_sector_id_plot_number_key'
    ) THEN
        ALTER TABLE habitat_plots
            DROP CONSTRAINT habitat_plots_sector_id_plot_number_key;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'unique_plot_per_sector'
    ) THEN
        ALTER TABLE habitat_plots
            ADD CONSTRAINT unique_plot_per_sector UNIQUE (sector_id, plot_number);
    END IF;
END $$;

-- Search / join indexes (no CONCURRENTLY — safe inside migration runners)
CREATE INDEX IF NOT EXISTS idx_habitat_plots_plan ON habitat_plots(plan_id);
CREATE INDEX IF NOT EXISTS idx_habitat_plots_plan_sector ON habitat_plots(plan_id, sector_id);
CREATE INDEX IF NOT EXISTS idx_habitat_plots_number ON habitat_plots(plot_number);

-- sectors → plans (may already exist from 20260522)
CREATE INDEX IF NOT EXISTS idx_habitat_sectors_plan ON habitat_sectors(plan_id);

-- Optional: native float array for sides (only if column is still jsonb)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'habitat_plots'
          AND column_name = 'sides_m'
          AND data_type = 'jsonb'
    ) THEN
        ALTER TABLE habitat_plots
            ADD COLUMN IF NOT EXISTS sides_m_array DOUBLE PRECISION[];

        UPDATE habitat_plots
        SET sides_m_array = (
            SELECT array_agg((elem::text)::double precision)
            FROM jsonb_array_elements(sides_m) AS elem
        )
        WHERE sides_m IS NOT NULL
          AND jsonb_typeof(sides_m) = 'array';

        ALTER TABLE habitat_plots DROP COLUMN IF EXISTS sides_m;
        ALTER TABLE habitat_plots RENAME COLUMN sides_m_array TO sides_m;
    END IF;
END $$;

-- Dimension columns (no-op if already present from 20260522)
ALTER TABLE habitat_plots
    ADD COLUMN IF NOT EXISTS perimeter_m DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS il_value DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS el_value DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS res_value DOUBLE PRECISION;
