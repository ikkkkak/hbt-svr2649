// One-off operational tool: backfill habitat_plots.geom (PostGIS) from
// geom_geojson for rows where it's NULL — the tile engine filters
// `geom IS NOT NULL`, so un-backfilled plots are invisible on the map.
//
// Safe by construction: additive (only fills NULLs), idempotent, per-row
// exception handling so malformed geometry rows are skipped, and it is the
// EXACT same SQL the server runs at startup (storage.backfillHabitatPlotGeom)
// — this tool just runs it now, against whatever DB_CONNECTION_STRING in
// .env points at, without waiting for a redeploy.
//
// Usage:  go run ./cmd/fixhabitatgeom
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DB_CONNECTION_STRING")
	if dsn == "" {
		log.Fatal("DB_CONNECTION_STRING is required (via .env or env)")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	report := func(label string) (pending int64) {
		db.Raw(`
			SELECT COUNT(*) FROM habitat_plots
			WHERE geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')
		`).Scan(&pending)
		var sector448952 int64
		db.Raw(`
			SELECT COUNT(*) FROM habitat_plots
			WHERE sector_id = 448952 AND geom IS NOT NULL
		`).Scan(&sector448952)
		fmt.Printf("[%s] pending geom backfill: %d | sector 448952 tile-visible plots: %d\n",
			label, pending, sector448952)
		return pending
	}

	pending := report("before")
	if pending == 0 {
		fmt.Println("Nothing to backfill.")
		return
	}

	// Batched loop — same statement the server runs at startup.
	for batch := 1; ; batch++ {
		fmt.Printf("Running backfill batch %d (up to 300000 rows)...\n", batch)
		if err := db.Exec(`
			DO $$
			DECLARE
				r RECORD;
			BEGIN
				FOR r IN
					SELECT id, geom_geojson FROM habitat_plots
					WHERE geom IS NULL AND geom_geojson IS NOT NULL AND geom_geojson::text NOT IN ('null', '{}')
					LIMIT 300000
				LOOP
					BEGIN
						UPDATE habitat_plots
						SET geom = ST_SetSRID(
							ST_GeomFromGeoJSON(
								CASE
									WHEN r.geom_geojson->>'type' = 'Feature' THEN r.geom_geojson->'geometry'
									WHEN r.geom_geojson->>'type' = 'FeatureCollection' THEN r.geom_geojson->'features'->0->'geometry'
									ELSE r.geom_geojson
								END
							), 4326)
						WHERE id = r.id;
					EXCEPTION WHEN OTHERS THEN
						CONTINUE;
					END;
				END LOOP;
			END $$;
		`).Error; err != nil {
			log.Fatalf("backfill batch %d failed: %v", batch, err)
		}
		remaining := report(fmt.Sprintf("after batch %d", batch))
		if remaining == 0 || remaining >= pending {
			// done, or no progress (all remaining rows are malformed) — stop.
			break
		}
		pending = remaining
	}
	fmt.Println("Backfill complete.")
}
