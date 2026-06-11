# Meskeny AI Agent — Master Architecture & Developer Brief

**Version:** 2.0 (grounded in repo)  
**Audience:** AI / full-stack developer  
**Status:** Single source of truth for the *next* generation of Meskeny intelligence  
**Last aligned with codebase:** `MeskenyGPT/`, `routes/ai_chat.go`, `apartmentsclone/screens/AIChatScreen.tsx`, `routes/meskeny_guide.go`

---

## 0. Product context (Meskeny)

Meskeny is a modern real-estate platform for **Mauritania**: digitizing how **land and properties** are listed, explored, and trusted. It bridges **structured listing data** (geo, papers, org/broker workflows) with everyday users—not a generic Western chatbot.

**Do not confuse two AI products already in production:**

| Product | Users | Where it lives | What it does today |
|--------|--------|----------------|-------------------|
| **MeskenyGPT** | Buyers, renters, curious users | `AIChatScreen.tsx` → `POST /api/ai/chat` → `MeskenyGPT/` | NL property search (DB-backed cards), valuation text, RAG FAQ, `deep_think` self-review (hidden) |
| **Meskeny Guide** | Hosts / sellers | `ListingGuideScreen`, `meskeny_guide.go`, org banners | Structured performance notes: diagnosis → cause → prescription (not the buyer chat) |
| **Broker Pro** (to build) | Agencies / brokers | *New surface* — extend MeskenyGPT + org tools | Leads, listing optimization, marketing packs, portfolio |

The master prompt’s **“Meskeny Guide (for clients)”** must be renamed in implementation to **MeskenyGPT Buyer Agent** to avoid clashing with the existing **Meskeny Guide** host product.

---

## 1. Executive vision (what we are building)

A **reasoning-first, multi-modal AI agent** embedded in Meskeny that:

1. **Shows its work** — Kimi/Qwen-style step timeline (Understand → Plan → Gather → Analyze → Verify → Deliver), not a black box.
2. **Self-checks before answering** — “Does this match the user’s inquiry?” with confidence and assumptions visible.
3. **Never hallucinates inventory** — listings, prices, and “registry verified” claims must come from **Meskeny DB or approved APIs only**.
4. **Speaks the user’s language** — AR / FR / EN first; **Chinese (zh)** as a prioritized module for broker/investor segment; Wolof/Pulaar as **phase 2** (detection + templated replies, not full LLM parity on day one).
5. **Feels premium** — typography, RTL, motion, accessibility, hard-case UX on par with top AI apps.

This is **not** a greenfield chatbot. Extend **MeskenyGPT** and **AIChatScreen**; add **Broker Pro** routes and UI where needed.

---

## 2. What already exists (do not rebuild)

### 2.1 Backend — `apartmentscloneserver/MeskenyGPT/`

```
MeskenyGPT/
  ai/service.go              # Public facade (main.go wires this)
  internal/ai/service.go     # HandleChatTurn orchestration
  internal/ai/lang/          # DetectLang (AR/FR/EN), intent, budget/city parsing
  internal/ai/property/      # DB search → property.Card (rent/sale/landmarks)
  internal/ai/rag/           # Mauritania context + admin playbook retrieval
  internal/ai/safety/        # Guard (bad words / injection basics)
  internal/ai/capture/       # Interaction logging + feedback
  internal/ai/client/        # OpenRouter LLM
  internal/ai/response/      # Messages, quick replies, no-results templates
  internal/ai/rules/         # Admin deterministic search rules
```

**`HandleChatTurn` pipeline today:**

1. Safety guard  
2. `lang.AnalyzeMessage` → intent + city/zone/budget  
3. **Search readiness gate** — `lang.ShouldClarifyBeforeSearch` blocks empty geo queries (`city= zone= budget=0`) and returns proactive clarification (`response/clarification.go`)  
4. Property intents → **pure DB** `property.Store` → cards (anti-hallucination)  
5. Zero results → `response/no_results.go` with budget/zone alternatives + alert chips  
6. Else → RAG + OpenRouter conversational reply  
6. `DeepThink` → `refineWithSelfReview` (2 extra LLM calls, **not exposed as steps to UI**)  
7. `enforceNoCardsResponseIntegrity` / `enforceMeskenyIdentity`  
8. Log interaction + Redis cache of last cards per session  

**HTTP:** `routes/ai_chat.go` — `deep_think`, `history`, `shared_property`, `[MESKENY_PICKER]` block from mobile.

**P0 implemented (this branch):**

- `POST /api/ai/agent/run` — SSE step stream (`routes/ai_agent_run.go`)  
- `HandleAgentRun` — real steps + verification (`MeskenyGPT/internal/ai/run_agent.go`)  
- **Proactive clarification** — `lang/search_readiness.go` + `response/clarification.go` wired in `HandleChatTurn` and `HandleAgentRun` (blocks empty DB searches)  
- Rate limits per tier (`agent_ratelimit.go`)  
- Mobile: `agentRunService.ts`, `AgentRunTimeline.tsx`, wired in `AIChatScreen.tsx`  
- Fallback to `POST /api/ai/chat` when streaming unavailable  
- Tests: `lang/search_readiness_test.go`, `response/clarification_test.go`

**Not implemented (gaps vs aspirational spec):**

- ~~No SSE/WebSocket step streaming~~ (SSE done for agent run)  
- ~~No Chinese (`LangZH`) in `lang/intent_types.go`~~ (detection + clarification chips done; full broker UI pending)  
- No structured `agent_run` JSON returned to client  
- No live **Ministry land registry API** — only `paper_types` on listings (titre foncier, etc.)  
- No Manus API integration in repo  
- No SimilarWeb integration  
- No paywall tiers on agent features yet  

### 2.2 Mobile — `apartmentsclone/screens/AIChatScreen.tsx`

- Full chat UI (Grok-inspired), sessions, property cards, map, quick replies  
- **`thinkHarderEnabled`** → sends `deep_think: true`  
- **Fake thinking UI:** rotating labels (`thinking.searching`, `thinking.analyzing`, …) while waiting for **one** HTTP response — **not** tied to real server steps  
- Badge **“Thought harder”** after reply if deep think was on — user does **not** see review notes or verifier block  
- `AiSearchFiltersPicker` → `[MESKENY_PICKER]` deterministic machine block  

### 2.3 Related AI (out of scope for MeskenyGPT chat unless explicitly merged)

- `services/listing_ai/` — host listing copy generation jobs  
- `routes/meskeny_guide.go` — host Guide comments (separate from buyer chat)  
- `services/ai_notification_queue.go` — push copy via MeskenyGPT  

---

## 3. Target architecture — Reasoning Layer (v2)

### 3.1 Non-negotiable server contract

Add **`POST /api/agent/run`** (or extend `/api/ai/chat` with `Accept: text/event-stream`):

**Request:** same as today + `stream: true`, `persona: buyer | broker`, `locale`, optional `listing_id`.

**Response events (SSE):**

```json
{ "type": "run_started", "run_id": "uuid", "persona": "buyer" }
{ "type": "step_start", "step_id": "understand", "label": "Understanding your request" }
{ "type": "step_done", "step_id": "understand", "ms": 120, "detail": { "lang": "ar", "intent": "search_buy" } }
{ "type": "step_start", "step_id": "gather", "label": "Searching Meskeny listings" }
{ "type": "tool_call", "tool": "search_properties", "args": { "city": "Nouakchott", "budget_max_mru": 8000000 } }
{ "type": "step_done", "step_id": "gather", "ms": 340, "detail": { "count": 12 } }
{ "type": "step_start", "step_id": "verify", "label": "Checking answer matches your question" }
{ "type": "verification", "matches_intent": true, "confidence": 0.86, "assumptions": ["Budget interpreted as MRU"], "gaps": [] }
{ "type": "final", "message": { ... }, "propertyRecommendations": [ ... ], "quick_replies": [ ... ] }
{ "type": "run_complete", "ms": 2100 }
```

**Rules:**

- Steps must reflect **real work** (DB query finished → `gather` done). No fake 10s delays.  
- **Verifier step is mandatory** for `persona=buyer` when LLM text is shown without DB cards.  
- On failure: `step_error` + retry hint; never silent empty response.  
- **`run_id`** links to `capture.Interaction` for audit.

### 3.2 Map aspirational loop to implementation

| Spec step | MeskenyGPT v2 implementation |
|-----------|------------------------------|
| Language detection | Existing `lang.DetectLang` + add `LangZH`; expose in `step_done` detail |
| Intent classification | Existing `lang.AnalyzeMessage` |
| Context retrieval | Existing `rag.Retriever` + session card cache |
| Relevance check | New: structured verifier (small LLM call or rules) → `verification` event |
| Confidence scoring | Verifier output + “no cards” / low comparables rules |
| Response generation | Existing DB path or OpenRouter |
| Self-correction | Promote `refineWithSelfReview` to visible **verify** sub-step when `deep_think` |

### 3.3 Tool allowlist (LLM cannot invent SQL)

| Tool | Purpose | Auth |
|------|---------|------|
| `search_properties` | Rent/sale DB | public/anon limits |
| `search_landmarks` | Land | public |
| `get_listing` | One property | public |
| `get_comparables` | Valuation band | auth preferred |
| `get_guide_threads` | Host listing guide | host only |
| `get_portfolio_summary` | Broker stats | org member |
| `generate_marketing_pack` | AR/FR/EN/zh copy | broker tier |

---

## 4. Personas & capabilities (realistic phasing)

### 4.1 MeskenyGPT Buyer Agent (extend current chat)

**P0 — Must ship**

- Visible step timeline in `AIChatScreen` (consume SSE)  
- DB-backed search unchanged (hallucination-safe)  
- AR / FR / EN detection + replies (already mostly there)  
- Hard cases: ambiguous query, conflicting budget, no results, offline (reuse `apiRetry`)  
- Shared property / valuation mode (already in server; show comparables source in UI)

**P1**

- Chinese module: `LangZH`, MRU↔CNY in verifier detail, marketing blurbs for brokers  
- Proactive chips: price check, find rent, land checklist (from `paper_types`, not registry API)  
- Price-drop / saved-search alerts (webhook — separate service)

**P2 — Only with official API + legal sign-off**

- Land registry verification (spec §7.3 **RED ALERT** flows)  
- Until then: copy must say **“Meskeny shows declared papers; confirm with authorities.”**

### 4.2 Broker Pro Agent (new)

- Lead scoring from messages (org inbox — if not exists, stub with manual upload)  
- Listing optimization + Meskeny Guide integration (`meskeny_guide.go`)  
- Marketing generator (FB/WhatsApp/IG) — AR/FR/EN/zh  
- Weekly market email — batch job, not blocking chat  

### 4.3 Meskeny Guide (hosts) — do not merge into buyer chat

Keep **structured letters** (diagnosis / prescription). Optional: buyer chat can **link** “Open Guide for your listing” for hosts only.

---

## 5. Chinese market module (spec §5)

**In scope for P1:**

- Terminology map (zh ↔ MRU ↔ quartier names) in RAG admin KB  
- Simplified Chinese summaries on listing cards (broker-triggered)  
- Cultural notes as **static playbook** snippets (facing, floor 4, community) — no invented facts  
- Festival-aware **marketing templates** (CNY, Golden Week) — template library, not live web scrape  

**Out of scope until partnership:**

- WeChat Mini Program  
- Investment visa legal advice (informational checklist only)  
- “Diaspora network” matching  

---

## 6. Technical hardening (align spec §4 with repo)

| Requirement | Current | Action |
|-------------|---------|--------|
| Input sanitization | `safety.Guard` | Extend prompt-injection patterns; max body size on `ai_chat.go` |
| Rate limiting | Partial (global API) | Per-user + anon session limits on `/api/agent/run` |
| Audit trail | `capture.Interaction` | Store `run_id`, steps JSON, tools called |
| Circuit breaker | None for LLM | OpenRouter failover model in config |
| Retry | Client `apiRetry.ts` | Server jitter on OpenRouter |
| Timeout | `cfg.TimeoutSeconds` | Step-level budgets; stream first event &lt;800ms |
| Caching | Redis last cards | Redis for comparables / feed quality |
| No Map in RN cache | Lesson from Guide bug | Agent state: plain objects only |

**Land registry:** Do **not** implement “Ministry API” in copy or UI until integrated. Use `paper_types` + user education.

---

## 7. Monetization (feature flags day one)

| Tier | Buyer | Broker |
|------|-------|--------|
| **Free** | N runs/day, basic search, no zh pack | — |
| **Pro** | Deep verify visible, unlimited runs | Guide + marketing + portfolio agent |
| **Enterprise** | — | API, white-label widget (future) |
| **Pay-per-use** | Premium PDF report (future) | Orange Money / Mobicash later |

Implement `agent_tier` in JWT or org subscription table before gating UI.

---

## 8. UI/UX — `AIChatScreen.tsx` requirements

### 8.1 New components (Storybook + app)

- `AgentRunTimeline` — collapsible steps, states: pending / running / done / error  
- `VerificationCard` — matches intent?, confidence, assumptions, gaps  
- `StepDetailSheet` — expandable tool args + result counts (no raw SQL)  
- `CompletionBar` — “6 steps · 1.2s” (real timings from server)

### 8.2 Replace today’s placeholder thinking

Remove sole dependency on `thinkingStage` timer rotation when `stream: true`. Keep skeleton + dots only **between** events.

### 8.3 Design tokens (spec §8)

- Latin: system / Inter; Arabic: Noto Naskh Arabic; Chinese: Noto Sans SC  
- Meskeny accent `#C9A96E`; semantic success/warning/error  
- 8px grid, 12px cards, WCAG AA, `reduceMotion` support, RTL for AR  

### 8.4 Accessibility

- Each step: `accessibilityLabel` + state  
- Verification block readable by VoiceOver / TalkBack  

---

## 9. Optional integrations (evaluate, do not block P0)

### 9.1 Manus API (spec §4.1)

**Use if:** you need external long-running agent tasks with webhooks outside Meskeny infra.  
**Use MeskenyGPT instead if:** all tools are internal DB + OpenRouter (recommended for P0–P1).  
**Hybrid:** Meskeny orchestrator owns UX; Manus only for heavy async jobs (e.g. PDF report generation).

### 9.2 SimilarWeb (spec §4.2)

**Optional P3** for internal BI — not user-facing buyer chat. Competitor traffic ≠ listing truth.

---

## 10. Hard-case protocols (implement in orchestrator)

| Case | Server behavior | UI behavior |
|------|-----------------|-------------|
| Ambiguous / under-specified query | `lang.ShouldClarifyBeforeSearch` → `ProactiveClarificationOutput` (no DB until city/zone known) | Clarification card + city/rent/buy chips |
| Conflicting constraints | New verifier fails → offer 3 structured alternatives | Option chips A/B/C |
| Fraud / title | No registry: explain `paper_types`; escalate CTA to support | Amber banner, no RED “verified” without API |
| Frustrated user | Sentiment hook (keyword or small model) | Shorter copy + “Talk to Meskeny” |
| Registry/API down | N/A today | Disclaimer + cached stats only |
| LLM timeout | `APIErrorOutput` (exists) | Failed step + retry |

---

## 11. Implementation phases (16-week map)

| Phase | Weeks | Deliverables |
|-------|-------|--------------|
| **P0 Foundation** | 1–4 | SSE agent run, step UI, verifier event, buyer tools only, AR/FR/EN |
| **P1 Intelligence** | 5–8 | Broker Pro MVP, Guide link, marketing templates, Chinese module |
| **P2 Scale** | 9–12 | Tiers/paywall, async reports, webhooks |
| **P3 Polish** | 13–16 | A/B prompts, bias sampling, security audit, perf budgets |

---

## 12. Success metrics

- **Relevance** ≥90% on sampled turns (human rubric)  
- **Step UI satisfaction** ≥4.5/5  
- **Language detection** ≥98% AR/FR/EN  
- **Inquiry conversion** +25% vs pre-agent UI  
- **Zero** fabricated listing IDs in production logs (automated test on DB path)

---

## 13. Developer deliverables checklist

1. Architecture diagram (this doc + sequence diagram for SSE)  
2. OpenAPI for `/api/agent/run`  
3. `AgentRunTimeline` in Storybook  
4. Prompt playbook under `MeskenyGPT/internal/ai/prompt/`  
5. Tests: `lang/*_test.go`, orchestrator integration, Detox/E2E on AIChat  
6. Security: threat model for prompt injection + anon abuse  
7. Dashboards: step latency, verifier fail rate, token cost per persona  

---

## 14. One-paragraph mandate (paste to developer)

> Upgrade **MeskenyGPT** (`MeskenyGPT/internal/ai/service.go`) and **AIChatScreen** to a Kimi/Qwen-quality **visible reasoning** experience: stream real steps from Go via SSE, run DB tools for search (never hallucinate listings), mandatory **verification** event (“does this match the user?”), AR/FR/EN now and Chinese for brokers in P1. Keep **Meskeny Guide** as the host product (`meskeny_guide.go`). Do not claim land registry verification without an official API—use `paper_types` and disclaimers. Add **Broker Pro** persona with portfolio and marketing tools. Harden with rate limits, audit `run_id`, and tier flags for paid features. Optional: Manus for async jobs only; SimilarWeb for internal BI later.

---

## 15. File touch list (starting points)

| Layer | Files |
|-------|--------|
| Orchestrator | `MeskenyGPT/internal/ai/service.go`, new `agent/orchestrator.go`, `agent/steps.go` |
| HTTP | `routes/ai_chat.go` or `routes/agent_run.go`, `MeskenyGPT/internal/httpapi/handlers.go` |
| Lang | `MeskenyGPT/internal/ai/lang/*.go` |
| Mobile | `AIChatScreen.tsx`, new `components/agent/AgentRunTimeline.tsx`, `services/aiService.ts` |
| i18n | `i18n/locales/en.json` → `aiChat.steps.*`, `aiChat.verify.*` |
| Host Guide | `routes/meskeny_guide.go` (read-only tool for brokers) |

---

## 16. Engineering Spec v2.0 — implementation mandate (source brief)

This section captures the **full engineering brief** (anti-hallucination, 3 roles, dynamic steps, FilterContext, SSE). It is the target architecture; §2 documents what is **already shipped**.

### 16.1 Golden rule (blocking)

| Never | Always |
|-------|--------|
| LLM invents listings, prices, lot IDs | `search_properties` / DB path returns cards only |
| Empty geo query `city= zone=` | Clarify first (`lang.ShouldClarifyBeforeSearch`) |
| Claim registry “verified” without API | `paper_types` + disclaimer |

**Repo today:** property search intents → `property.Store` (deterministic, no LLM listing text). Conversational path still uses LLM — verifier step required.

### 16.2 Three agent roles (v2 target)

| Role | Trigger | Tools (target) | Repo status |
|------|---------|----------------|-------------|
| **PropertySearcher** | Search / browse / filters | `search_properties`, geo | **Partial** — DB search in orchestrator; role routed in `agent/role.go` |
| **PropertyAdvisor** | Advice, compare, greetings | search + stats | **Partial** — RAG + conversational |
| **MarketAnalyst** | Trends, valuation, districts | `get_market_stats`, verify_lot | **Stub** — valuation text only; no registry API |

Router: `MeskenyGPT/internal/ai/agent/role.go` → emitted on SSE `stream_start`.

### 16.3 FilterContext (Spec §7)

Picker selections must **persist in session** and apply to follow-ups (“buy” after picker must keep city/zone).

| Layer | File |
|-------|------|
| Redis store (30 min) | `MeskenyGPT/internal/ai/session/filters.go` |
| Hydrate + persist | `MeskenyGPT/internal/ai/session_filters.go` |
| History merge | `MeskenyGPT/internal/ai/lang/context_merge.go` |
| Picker block | `[MESKENY_PICKER]` in `lang/intent.go` |
| HTTP update | `POST /api/ai/agent/filters` |
| Mobile picker sync | `agentFiltersService.ts` → called on picker submit |

### 16.4 SSE protocol (v2 vs shipped)

| Event | v2 spec | Shipped (`/api/ai/agent/run`) |
|-------|---------|--------------------------------|
| `stream_start` | role, lang, rtl | **Yes** (2025-06) |
| `step` dynamic LLM labels | AI-generated | **Fixed** 6 steps (understand→deliver) — migrate in Phase 3 |
| `text` token deltas | Streaming answer | **No** — single `final` message |
| `follow_ups` | AI-generated chips | **Partial** — SSE `follow_ups` event + quick_replies actions |
| `cards` | Property grid | **`final.propertyRecommendations`** |

Mobile: `agentRunService.ts` (XHR SSE on RN), `AgentRunTimeline.tsx`.

### 16.5 Build roadmap (from brief)

1. **Week 1–2:** Anti-hallucination tests AH-001…008; block conversational “I found X listings” without DB (`enforceNoCardsResponseIntegrity` exists).  
2. **Week 2–3:** Role router + FilterContext (**started**).  
3. **Week 3–4:** Dynamic LLM steps + token streaming.  
4. **Week 4–5:** Geo API `/geo/cities|zones|quartiers` (pickers today use app data + picker POST).  
5. **Week 5–6:** AI-generated `follow_ups` event; perf &lt;800ms first SSE.

### 16.6 Anti-hallucination test suite (must pass before release)

- **AH-001:** TZ search, empty DB → no invented listings  
- **AH-002:** Budget filter → DB query with `max_price`  
- **AH-003:** Zero results → honest message + relax chips  
- **AH-004:** Lot verify → no invented beds (registry N/A)  
- **AH-005:** Market stats only from tool/DB  
- **AH-006–008:** AR/FR/EN language + role switch  

Automate in `MeskenyGPT/internal/ai/lang/*_test.go` + integration tests.

---

**End of specification.**  
All new work must extend this document via PR notes when scope changes.
