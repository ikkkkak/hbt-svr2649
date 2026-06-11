# Meskeny AI Notification Orchestrator (v1)

## Enable

Set in environment:

- `MESKENY_NOTIFICATION_ORCHESTRATOR=true` — routes all wired push paths through the orchestrator (candidate → score → send/delay/batch/drop).
- `ORCHESTRATOR_INTERNAL_KEY=<secret>` — required for `POST /api/orchestrator/candidates` (`X-Internal-Key` header).

**Default:** orchestrator is **off**; behavior matches the legacy direct FCM/Expo path.

## Database

Tables (see `migrations/20260207_notification_orchestrator.sql` + GORM AutoMigrate):

| Table | Purpose |
|--------|---------|
| `notification_candidates` | Every notification intent + AI decision + feedback |
| `user_notification_quota` | Optional rolling-window audit (MVP enforcement uses 24h count on `notification_candidates`) |
| `user_notification_preferences` | Learned/profile fields (defaults + quiet hours sync from `notification_preferences`) |

## Hard rule

Max **4** pushes per user per **24h sliding window** (counts rows with `sent_at` in last 24h and `ai_decision` ∈ `send`, `digest_sent`). Premium override: `daily_limit_override` on `user_notification_preferences`.

## Mapped notification sources (Meskeny codebase)

| Source | Mechanism | `notification_type` / notes |
|--------|-----------|-----------------------------|
| Hourly smart scheduler (`notification_scheduler.go`) | `sendToUserWithImage` | `rent_suggestion`, `continue_browsing`, `trending_properties`, `weekly_digest`, `nearby_property`, … + `legacy_event_type` for `notification_delivery_logs` after real send |
| AI Redis queue (`ai_notification_queue.go`) | `sendToUserWithImage` | `viewed_property_reminder`, `still_available`, `similar_properties`, `reengage_digest` |
| New property broadcast (`SendNewPropertyNotification`) | `SubmitNotificationCandidate` when orchestrator on | `new_property` |
| Transactional (`SendNotificationToUser`) | `SubmitNotificationCandidate` when orchestrator on | Uses `NotificationData.Type` (reservations, messages, tours, offers, …) |
| Discovery spotlight | `sendToUserWithImage` | Inherits `type` from `discoverpush` payload (consider adding `legacy_event_type` if you need delivery-log parity) |
| Internal / other backends | `POST /api/orchestrator/candidates` | Arbitrary `notification_type` |

**Anonymous / token-only** paths still use `sendRichToTokenWithGuards` (no user id) — **not** routed through the orchestrator.

## HTTP API

| Method | Path | Auth |
|--------|------|------|
| POST | `/api/orchestrator/candidates` | `X-Internal-Key` |
| POST | `/api/orchestrator/feedback` | Bearer |
| GET | `/api/user/notification-orchestrator` | Bearer (inbox + quota) |
| GET | `/api/admin/orchestrator/stats` | Admin |
| GET | `/api/admin/orchestrator/users/{userID}/log` | Admin |

## Workers

`StartNotificationOrchestratorWorkers()`: every minute processes due `delay` / `batch` rows and runs digest composition when `ai_decision = batch` and ≥2 pending items per user.

## Frontend follow-up

- Call `POST /api/orchestrator/feedback` on notification open/dismiss with `candidate_id` (add `candidate_id` to FCM/data payload in a future iteration).
- Optional: notification inbox UI using `GET /api/user/notification-orchestrator`.
