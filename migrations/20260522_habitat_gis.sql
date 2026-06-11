-- Habitat cadastre: Plan (district) → Sector (sub-zone) → Plot (parcel)
-- Geometry stored as JSONB + centroid lat/lng (PostGIS optional later).

CREATE TABLE IF NOT EXISTS habitat_plans (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    name_ar VARCHAR(100) NOT NULL,
    color VARCHAR(20) DEFAULT '#2a5298',
    bounds_geojson JSONB,
    centroid_lat DOUBLE PRECISION,
    centroid_lng DOUBLE PRECISION,
    sector_count INTEGER DEFAULT 0,
    plot_count BIGINT DEFAULT 0,
    total_area_m2 DOUBLE PRECISION DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS habitat_sectors (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES habitat_plans(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    name_ar VARCHAR(200),
    code VARCHAR(100),
    bounds_geojson JSONB,
    centroid_lat DOUBLE PRECISION,
    centroid_lng DOUBLE PRECISION,
    plot_count INTEGER DEFAULT 0,
    total_area_m2 DOUBLE PRECISION DEFAULT 0,
    original_size INTEGER,
    original_lat DOUBLE PRECISION,
    original_lng DOUBLE PRECISION,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(plan_id, name)
);

CREATE TABLE IF NOT EXISTS habitat_plots (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES habitat_plans(id) ON DELETE CASCADE,
    sector_id BIGINT NOT NULL REFERENCES habitat_sectors(id) ON DELETE CASCADE,
    plot_number VARCHAR(100) NOT NULL,
    l_value VARCHAR(100),
    i_value INTEGER,
    area_m2 DOUBLE PRECISION,
    area_rounded INTEGER,
    area_hectares DOUBLE PRECISION,
    perimeter_m DOUBLE PRECISION,
    sides_m DOUBLE PRECISION[],
    dimensions_string VARCHAR(200),
    length_m DOUBLE PRECISION,
    width_m DOUBLE PRECISION,
    il_value DOUBLE PRECISION,
    el_value DOUBLE PRECISION,
    res_value DOUBLE PRECISION,
    geom_geojson JSONB,
    centroid_lat DOUBLE PRECISION,
    centroid_lng DOUBLE PRECISION,
    corners JSONB,
    raw_properties JSONB,
    data_file_path VARCHAR(500),
    version INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT unique_plot_per_sector UNIQUE (sector_id, plot_number)
);

CREATE INDEX IF NOT EXISTS idx_habitat_sectors_plan ON habitat_sectors(plan_id);
CREATE INDEX IF NOT EXISTS idx_habitat_plots_plan ON habitat_plots(plan_id);
CREATE INDEX IF NOT EXISTS idx_habitat_plots_sector ON habitat_plots(sector_id);
CREATE INDEX IF NOT EXISTS idx_habitat_plots_plan_sector ON habitat_plots(plan_id, sector_id);
CREATE INDEX IF NOT EXISTS idx_habitat_plots_number ON habitat_plots(plot_number);
