# Host Suggestions Engine

This document explains how Meskeny suggests buyers to a host (or broker/organization owner), how data is stored, and when a new user becomes visible in Suggestions.

## What the feature does

When a host opens Suggestions for a property:

1. The API returns **stored matches** from `property_matches` (fast path).
2. If data is stale or empty, a background refresh job is scheduled.
3. The host sees existing suggestions immediately.
4. New suggestions appear on the next fetch after background refresh persists them.

The endpoint is intentionally read-first to avoid DB overload from live recomputation per request.

## Who can access Suggestions

A caller can access suggestions for a property only if they own the listing:

- Direct owner (`property_sales.owner_id == auth user id`), or
- Organization owner (`property_sales.organization_id` belongs to an organization owned by auth user id).

This covers both direct hosts and broker-style organization owners.

## Main endpoint behavior

`GET /api/host/suggestions?property_id={id}`

- Requires auth token.
- Requires a valid `property_id`.
- Returns:
  - `suggestions`: list of suggested users
  - `total_matches`
  - `cached` (true when served from stored records)
  - `refreshing` (true when an async rebuild was scheduled)

Optional:

- `refresh=1` forces scheduling a rebuild path (still returns quickly with stored data if available).

## Data model used

- `property_dna`: normalized AI-oriented profile of a property
- `ai_enriched_users`: derived user intent + budget + urgency profile
- `user_behavior_summary`: **pre-aggregated** roll-up of `user_behaviors` (top city/zone, 90d counts, 180d avg price). Refreshed by admin/cron — host matching reads this first to avoid per-user CTE scans.
- `property_matches`: persisted match rows shown to hosts

## How a user becomes suggested

A user is suggested when all of the following pipeline stages pass:

1. **Candidate selection**
   - User appears in `user_behaviors`
   - `property_type = 'sale'`
   - Interaction in `('view','favorite','contact','click')`
   - Activity is recent (last 180 days)
   - Excludes the host user

2. **Buyer consent (required)**
   - User must have `share_profile_with_hosts = true` (set via `PUT /api/user/host-share-consent`).
   - Buyers are locked to **one host** via `host_share_locked_host_id` once matched.
   - Host API returns **minimal** buyer fields (first name label, score, tier, reasons) — no phone, email, or avatar.

3. **User enrichment**
   - Prefer **`user_behavior_summary`** when `last_updated` is recent (fast path).
   - Otherwise run the legacy per-user queries and persist in `ai_enriched_users` (with mutex + in-memory cache + update cooldown to reduce duplicate writes).

4. **Property DNA extraction**
   - Build/refresh property signals from title/description/price/type/location.
   - Persist in `property_dna`.

5. **Scoring**
   - Score components include:
     - Location match (zone/city)
     - Property-type affinity
     - Budget compatibility
     - Engagement level
     - Persona overlap
     - Urgency
   - Total score determines tier (`excellent`, `strong`, `good`).

6. **Threshold**
   - Only scores `>= 60` are kept as suggestions.

7. **Persistence**
   - At most **5** pending buyers per property (`MaxPendingBuyerMatchesPerProperty`).
   - Batch upsert into `property_matches` (single statement per chunk; falls back to per-row upsert if the DB lacks the partial unique index).
   - Status starts as `pending`. Rows already `contacted` / non-pending are not overwritten by refresh.

Only persisted rows are returned to UI.

## Why this is stable under load

The implementation includes safeguards:

- Read-first API (returns stored matches)
- Async recompute instead of request-path recompute
- Per-property in-process lock to prevent duplicate concurrent refresh jobs
- Candidate cap to bound compute cost
- Cooldowns on `ai_enriched_users` and `property_matches` updates

These controls prevent repetitive writes and connection-slot exhaustion during rapid refreshes.

## Performance / emergency ops (May 2026)

1. **Indexes + partial unique** (run on Postgres when ready): `migrations/20260506_host_suggestions_emergency.sql` — speeds behavior scans, avg-price joins, and enables fast `ON CONFLICT` upserts on `property_matches`.
2. **Refresh summary table** (heavy; run nightly or after large backfills):
   - `POST /api/admin/insights/host-suggestions/refresh-behavior-summary` (admin auth).
   - Or call `routes.RefreshUserBehaviorSummaryTable()` from a cron worker.
3. **GORM** will auto-create `user_behavior_summary` on deploy if `AutoMigrate` includes `UserBehaviorSummary`.

Until the summary table is populated, matching still works via the slower per-user SQL path.

## Contact and dismiss flow

- `POST /api/host/suggestions/{match_id}/contact`
  - Creates direct message and marks match as `contacted`.
- `POST /api/host/suggestions/{match_id}/dismiss`
  - Marks match as `dismissed`.

## Practical summary

A new user is suggested to a host/broker when the user has recent sale-intent behavior, scores high enough against that property's DNA, and the resulting match is persisted in `property_matches`. The host screen is a cheap read of those persisted results, with refresh handled in background.

## Mobile app (Host Suggestions UI)

The React Native `HostSuggestionsScreen` shows matches **sorted by score (interest %)**. For each row:

- **Left:** buyer avatar, name, and **interest percentage** (same as match score %).
- **Right:** mini card for the **matched listing** (title + price when available from the host’s property list).
- **Middle:** visual “link” between buyer and listing.

Only the **top 5** matches are fully interactive (contact / dismiss). Additional rows appear underneath with a **blur overlay** and a **Contact us** action to reach Meskeny support and unlock the full list (product/paywall copy — adjust email in app i18n / `mailto` as needed).

