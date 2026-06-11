-- Land listing: structured location (city/zone/quartier), cadastre plot link, and host plot confirmation.

ALTER TABLE landmarks
    ADD COLUMN IF NOT EXISTS city_id BIGINT,
    ADD COLUMN IF NOT EXISTS zone_id BIGINT,
    ADD COLUMN IF NOT EXISTS quartier_id BIGINT,
    ADD COLUMN IF NOT EXISTS habitat_plot_id BIGINT,
    ADD COLUMN IF NOT EXISTS plot_confirmed BOOLEAN NOT NULL DEFAULT false;

-- FKs (idempotent)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'landmarks_city_id_fkey') THEN
        ALTER TABLE landmarks
            ADD CONSTRAINT landmarks_city_id_fkey
            FOREIGN KEY (city_id) REFERENCES cities(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'landmarks_zone_id_fkey') THEN
        ALTER TABLE landmarks
            ADD CONSTRAINT landmarks_zone_id_fkey
            FOREIGN KEY (zone_id) REFERENCES zones(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'landmarks_quartier_id_fkey') THEN
        ALTER TABLE landmarks
            ADD CONSTRAINT landmarks_quartier_id_fkey
            FOREIGN KEY (quartier_id) REFERENCES quartiers(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'landmarks_habitat_plot_id_fkey') THEN
        ALTER TABLE landmarks
            ADD CONSTRAINT landmarks_habitat_plot_id_fkey
            FOREIGN KEY (habitat_plot_id) REFERENCES habitat_plots(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_landmarks_city_id ON landmarks(city_id);
CREATE INDEX IF NOT EXISTS idx_landmarks_zone_id ON landmarks(zone_id);
CREATE INDEX IF NOT EXISTS idx_landmarks_quartier_id ON landmarks(quartier_id);
CREATE INDEX IF NOT EXISTS idx_landmarks_habitat_plot_id ON landmarks(habitat_plot_id);
CREATE INDEX IF NOT EXISTS idx_landmarks_plot_confirmed ON landmarks(plot_confirmed);

COMMENT ON COLUMN landmarks.city_id IS 'Required for new listings: FK to cities';
COMMENT ON COLUMN landmarks.zone_id IS 'Required for new listings: FK to zones';
COMMENT ON COLUMN landmarks.quartier_id IS 'Required for new listings: FK to quartiers (sector in UI)';
COMMENT ON COLUMN landmarks.habitat_plot_id IS 'Optional link to habitat_plots after cadastre match';
COMMENT ON COLUMN landmarks.plot_confirmed IS 'Host confirmed cadastre plot match before publish';
