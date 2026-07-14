#!/usr/bin/env python3
"""
Import Habitat cadastre GeoJSON into PostgreSQL without "transaction is aborted" cascades.

Problem: wrapping many INSERTs in one BEGIN/COMMIT means the first failure aborts the
transaction; every later INSERT only reports "current transaction is aborted".

Fix: one transaction per sector, SAVEPOINT per plot (failed plots are skipped, sector still commits).

Usage:
  pip install psycopg2-binary
  set DATABASE_URL=postgresql://user:pass@localhost:5432/dbname
  python scripts/import_habitat_geojson.py --root "C:/path/to/geojson" --plan-code ARF --plan-name Arafat

Expected layout (example):
  geojson/
    Arafatt/
      Secteur_8.geojson
      Secteur_9.geojson
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path
from typing import Any, Iterator

try:
    import psycopg2
    from psycopg2.extras import Json
except ImportError:
    print("Install: pip install psycopg2-binary", file=sys.stderr)
    raise

# Cadastre parcel number keys only — do NOT use generic id/name (causes wrong plot labels).
PLOT_NUMBER_KEYS = (
    "plot_number",
    "plotNumber",
    "NUMERO",
    "numero",
    "Numero",
    "N",
    "n",
    "PARCEL",
    "parcel",
    "Parcel",
    "LOT",
    "lot",
)


def plot_number_from_feature(props: dict[str, Any], fallback_index: int) -> str:
    for k in PLOT_NUMBER_KEYS:
        v = props.get(k)
        if v is not None and str(v).strip():
            return str(v).strip()
    return str(fallback_index)


def as_float(v: Any) -> float | None:
    if v is None:
        return None
    try:
        return float(str(v).replace(",", ".").strip())
    except (TypeError, ValueError):
        return None


def as_int(v: Any) -> int | None:
    f = as_float(v)
    if f is None:
        return None
    return int(round(f))


def as_float_list(v: Any) -> list[float] | None:
    if v is None:
        return None
    if isinstance(v, list):
        out: list[float] = []
        for item in v:
            f = as_float(item)
            if f is not None:
                out.append(f)
        return out or None
    if isinstance(v, str):
        try:
            parsed = json.loads(v)
            return as_float_list(parsed)
        except json.JSONDecodeError:
            return None
    return None


def dimensions_string_from_props(props: dict[str, Any], sides: list[float] | None) -> str | None:
    for k in ("dimensions_string", "dimensions", "DIMENSIONS", "Dimensions", "dim", "DIM"):
        v = props.get(k)
        if v is not None and str(v).strip():
            return str(v).strip()
    if sides and len(sides) >= 3:
        return " ".join(f"{s:.1f}m" for s in sides)
    length = as_float(props.get("length_m") or props.get("length") or props.get("longueur"))
    width = as_float(props.get("width_m") or props.get("width") or props.get("largeur"))
    if length and width:
        return f"{length:.1f}m {width:.1f}m"
    return None


def iter_features(geojson_path: Path) -> Iterator[tuple[dict[str, Any], dict[str, Any]]]:
    with geojson_path.open(encoding="utf-8") as f:
        data = json.load(f)
    if data.get("type") == "FeatureCollection":
        features = data.get("features") or []
    elif data.get("type") == "Feature":
        features = [data]
    else:
        raise ValueError(f"Unsupported GeoJSON type in {geojson_path}")
    for i, feat in enumerate(features):
        if not isinstance(feat, dict):
            continue
        props = feat.get("properties") or {}
        geom = feat.get("geometry")
        if geom is None:
            continue
        yield props, geom


def sector_name_from_path(path: Path) -> str:
    return path.stem.replace("_", " ").strip()


def connect():
    url = os.environ.get("DATABASE_URL")
    if not url:
        raise SystemExit("Set DATABASE_URL=postgresql://user:pass@host:5432/db")
    return psycopg2.connect(url)


def ensure_plan(cur, code: str, name: str, name_ar: str) -> int:
    cur.execute(
        """
        INSERT INTO habitat_plans (code, name, name_ar)
        VALUES (%s, %s, %s)
        ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()
        RETURNING id
        """,
        (code, name, name_ar or name),
    )
    row = cur.fetchone()
    return int(row[0])


def ensure_sector(cur, plan_id: int, name: str, name_ar: str | None = None) -> int:
    cur.execute(
        """
        INSERT INTO habitat_sectors (plan_id, name, name_ar)
        VALUES (%s, %s, %s)
        ON CONFLICT (plan_id, name) DO UPDATE SET updated_at = NOW()
        RETURNING id
        """,
        (plan_id, name, name_ar or name),
    )
    row = cur.fetchone()
    return int(row[0])


def insert_plot(
    cur,
    plan_id: int,
    sector_id: int,
    plot_number: str,
    geom: dict[str, Any],
    props: dict[str, Any],
    data_file_path: str,
) -> None:
    lat = props.get("centroid_lat") or props.get("lat") or props.get("LAT")
    lng = props.get("centroid_lng") or props.get("lng") or props.get("LNG")
    area = props.get("area_m2") or props.get("area") or props.get("AREA")
    sides = as_float_list(
        props.get("sides_m") or props.get("sides") or props.get("SIDES") or props.get("cotes")
    )
    dims = dimensions_string_from_props(props, sides)
    length_m = as_float(props.get("length_m") or props.get("length") or props.get("longueur"))
    width_m = as_float(props.get("width_m") or props.get("width") or props.get("largeur"))
    # Overloaded source field: usually a numeric front-height measurement,
    # but for sectors with Ilot (city-block) subdivisions it's an alphanumeric
    # code like "08_NC" — as_float() here used to silently drop those to
    # NULL. Column is VARCHAR in the DB; keep it as a plain string.
    il_raw = props.get("il_value") or props.get("IL") or props.get("il")
    il_value = str(il_raw).strip() if il_raw is not None else None
    el_value = as_float(props.get("el_value") or props.get("EL") or props.get("el") or props.get("elevation"))
    res_value = as_float(props.get("res_value") or props.get("RES") or props.get("res"))
    l_value = props.get("l_value") or props.get("L") or props.get("l")
    i_value = as_int(props.get("i_value") or props.get("I") or props.get("i"))
    area_rounded = as_int(props.get("area_rounded") or props.get("area_rounded_m2"))

    cur.execute(
        """
        INSERT INTO habitat_plots (
            plan_id, sector_id, plot_number,
            l_value, i_value,
            area_m2, area_rounded,
            dimensions_string, length_m, width_m, sides_m,
            il_value, el_value, res_value,
            centroid_lat, centroid_lng,
            geom_geojson, raw_properties, data_file_path
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (sector_id, plot_number) DO UPDATE SET
            geom_geojson = EXCLUDED.geom_geojson,
            raw_properties = EXCLUDED.raw_properties,
            l_value = COALESCE(EXCLUDED.l_value, habitat_plots.l_value),
            i_value = COALESCE(EXCLUDED.i_value, habitat_plots.i_value),
            area_m2 = COALESCE(EXCLUDED.area_m2, habitat_plots.area_m2),
            area_rounded = COALESCE(EXCLUDED.area_rounded, habitat_plots.area_rounded),
            dimensions_string = COALESCE(NULLIF(EXCLUDED.dimensions_string, ''), habitat_plots.dimensions_string),
            length_m = COALESCE(EXCLUDED.length_m, habitat_plots.length_m),
            width_m = COALESCE(EXCLUDED.width_m, habitat_plots.width_m),
            sides_m = COALESCE(EXCLUDED.sides_m, habitat_plots.sides_m),
            il_value = COALESCE(EXCLUDED.il_value, habitat_plots.il_value),
            el_value = COALESCE(EXCLUDED.el_value, habitat_plots.el_value),
            res_value = COALESCE(EXCLUDED.res_value, habitat_plots.res_value),
            centroid_lat = COALESCE(EXCLUDED.centroid_lat, habitat_plots.centroid_lat),
            centroid_lng = COALESCE(EXCLUDED.centroid_lng, habitat_plots.centroid_lng),
            updated_at = NOW()
        """,
        (
            plan_id,
            sector_id,
            plot_number,
            str(l_value).strip() if l_value is not None else None,
            i_value,
            float(area) if area is not None else None,
            area_rounded,
            dims,
            length_m,
            width_m,
            sides,
            il_value,
            el_value,
            res_value,
            float(lat) if lat is not None else None,
            float(lng) if lng is not None else None,
            Json(geom),
            Json(props),
            data_file_path,
        ),
    )


def import_sector_file(
    conn,
    plan_id: int,
    geojson_path: Path,
    sector_label: str,
    *,
    dry_run: bool,
) -> tuple[int, int, str | None]:
    """Returns (inserted_or_updated, skipped, first_error_message)."""
    ok = 0
    skipped = 0
    first_error: str | None = None

    with conn:
        with conn.cursor() as cur:
            sector_id = ensure_sector(cur, plan_id, sector_label)

    with conn:
        with conn.cursor() as cur:
            cur.execute("BEGIN")
            for idx, (props, geom) in enumerate(iter_features(geojson_path)):
                pn = plot_number_from_feature(props, idx + 1)
                sp = f"sp_{re.sub(r'[^a-zA-Z0-9_]', '_', pn)[:40]}_{idx}"
                try:
                    cur.execute(f"SAVEPOINT {sp}")
                    if not dry_run:
                        insert_plot(
                            cur,
                            plan_id,
                            sector_id,
                            pn,
                            geom,
                            props,
                            str(geojson_path),
                        )
                    cur.execute(f"RELEASE SAVEPOINT {sp}")
                    ok += 1
                except Exception as e:
                    cur.execute(f"ROLLBACK TO SAVEPOINT {sp}")
                    skipped += 1
                    if first_error is None:
                        first_error = f"{pn}: {e}"
                    print(f"      ⚠️ {pn}: {e}")
            cur.execute("COMMIT")

    return ok, skipped, first_error


def collect_geojson_files(root: Path, plan_folder: str | None) -> list[Path]:
    base = root / plan_folder if plan_folder else root
    if not base.is_dir():
        raise SystemExit(f"Not a directory: {base}")
    files = sorted(base.glob("*.geojson")) + sorted(base.glob("**/*.geojson"))
    # de-dupe while preserving order
    seen: set[Path] = set()
    out: list[Path] = []
    for p in files:
        rp = p.resolve()
        if rp not in seen:
            seen.add(rp)
            out.append(p)
    return out


def main() -> None:
    ap = argparse.ArgumentParser(description="Import habitat GeoJSON with per-plot savepoints")
    ap.add_argument("--root", required=True, help="Root folder containing plan subfolders or .geojson files")
    ap.add_argument("--plan-folder", default=None, help="Subfolder under root (e.g. Arafatt)")
    ap.add_argument("--plan-code", required=True, help="Plan code, e.g. ARF")
    ap.add_argument("--plan-name", required=True, help="Plan display name, e.g. Arafat")
    ap.add_argument("--plan-name-ar", default="", help="Arabic plan name")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    root = Path(args.root)
    files = collect_geojson_files(root, args.plan_folder)
    if not files:
        raise SystemExit(f"No .geojson files under {root}")

    conn = connect()
    with conn:
        with conn.cursor() as cur:
            plan_id = ensure_plan(cur, args.plan_code, args.plan_name, args.plan_name_ar)

    total = 0
    total_skipped = 0
    n = len(files)
    plan_label = args.plan_folder or args.plan_name

    print(f"Plan {args.plan_code} (id={plan_id}) — {n} sector file(s)")

    for i, path in enumerate(files, start=1):
        sector_label = sector_name_from_path(path)
        ok, skipped, first_err = import_sector_file(
            conn, plan_id, path, sector_label, dry_run=args.dry_run
        )
        total += ok
        total_skipped += skipped
        err_hint = f" (first error: {first_err})" if first_err else ""
        print(f"  [{i}/{n}] {plan_label}/{path.name}: {ok} plots, skipped {skipped} (running total: {total:,}){err_hint}")

    print(f"\nDone. plots upserted: {total:,}, skipped: {total_skipped:,}")
    if total_skipped:
        print("Fix skipped rows (see first error per sector above), then re-run — ON CONFLICT will upsert.")


if __name__ == "__main__":
    main()
