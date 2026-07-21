# Architecture Decision Record v2 — Car Service Dashboard SaaS (Go API + React SPA)

**Status:** Proposed — awaiting approval
**Date:** 2026-07-16
**Supersedes:** ADR v1 (server-rendered HTML architecture), retired by owner decision to build a JSON API + separate React SPA.
**Context:** Production-ready, multi-tenant SaaS for car service owners. MVP: customers, cars, repair offers, repairs, service history, PDF generation/preview/download, **email offer PDF to customer**, authentication, authorization. Backend philosophy: Alex Edwards — simple, idiomatic Go, stdlib first, minimal dependencies, explicit over magic, struct DI, no frameworks. Frontend philosophy: the same spirit translated — strict types, single sources of truth, dependencies that earn their place.

## Decision Summary

| # | Area | Decision |
|---|------|----------|
| 1 | Go version | Latest stable, pinned in `go.mod`, upgraded each release |
| 2 | Router | `net/http.ServeMux` (1.22+ method/wildcard routing); hand-rolled middleware chain. Chi noted as the closest call; Gin/Echo excluded as frameworks |
| 3 | Configuration | Env vars → explicit `Config` struct, validated in `main()`; `.env` via Makefile in dev only. Frontend config minimized by same-origin design |
| 4 | Logging | `log/slog` — text dev / JSON prod; per-request child loggers with request/tenant/user IDs; `request_id` echoed to clients |
| 5 | Database | PostgreSQL, latest stable major |
| 6 | Data access | **Raw `database/sql` over `pgx/v5/stdlib` (owner revision from sqlc)** — see Compensating Controls |
| 7 | Migrations | goose, plain SQL, embedded, explicit `api migrate` subcommand |
| 8 | API conventions | `/api/v1` URL versioning; plural nouns; camelCase JSON; ISO-8601 UTC; string UUIDs; **RFC 9457 problem+json errors** (+ `requestId`, `errors` field map, **`code` field for stable machine-readable error identity**); **Edwards-style named envelopes**; **offset/page pagination** with metadata |
| 9 | Authentication | **HttpOnly session cookies via `alexedwards/scs` (Postgres store)** for the SPA; auth middleware structured header-then-cookie so **opaque bearer tokens (hashed, Edwards-style) slot in for a future React Native app**; JWT rejected |
| 10 | Authorization | Hand-rolled RBAC: `owner`/`admin`/`mechanic` enum + `map[Role][]Permission`; `requireAuth` → 401, `requirePermission` → 403 (problem+json, never conflated); object-level checks in handlers; permissions exposed read-only via `GET /auth/me` |
| 11 | Password hashing | **bcrypt (owner override of argon2id recommendation)** — cost 12, validator enforces 72-byte max password; self-describing hashes keep argon2id migration open via re-hash-on-login |
| 12 | Validation | Homemade Edwards validator (~60 lines) → **422 problem+json with camelCase field keys** matching JSON names; Postgres constraints as second line. No go-playground/validator |
| 13 | PDF | **Server-side maroto v2 behind `OfferRenderer` interface**; generated on demand, never stored; `GET /offers/{id}/pdf` with `?disposition=inline` for SPA iframe preview. Client-side generation rejected (two-renderer drift, canonical-document integrity, mobile reuse) |
| 14 | Email | SMTP via `wneessen/go-mail` behind `Mailer` interface; bodies via `html/template`; Mailpit in dev; provider = config (Resend SMTP default). **Send-offer-PDF is MVP scope**: `From: app domain`, `Reply-To: garage`, freeze-on-send |
| 15 | File storage | **Cloudflare R2 from day one** (AWS SDK v2) behind `FileStore`; MinIO in dev; tenant-prefixed keys; all transfers proxied through authenticated handlers (presigned URLs deferred as scaling tool) |
| 16 | Caching | **No server cache.** Client caching owned by TanStack Query; `Cache-Control: no-store` on API, `immutable` on hashed assets. Redis declined |
| 17 | Background jobs | **River from day one (owner override of goroutines-first)** — Postgres-backed durable queue, transactional enqueue, periodic jobs replace the housekeeping ticker; asynq+Redis rejected (non-transactional enqueue + extra infra) |
| 18 | Rate limiting | `x/time/rate`: lenient global per-IP (burst sized for SPA parallel queries) + strict on `/auth/login` & `/auth/password-reset`; per-email reset throttle in Postgres; trusted-proxy IP extraction; 429 problem+json + `Retry-After` |
| 19 | Middleware | recover(→problem+json 500) → request ID → logging → security headers → rate limit → **CSRF: stdlib `CrossOriginProtection` + SameSite=Lax (required because cookies)** → scs LoadAndSave → requireAuth → tenant context → requirePermission. **No CORS middleware** (same-origin prod, Vite proxy dev); server-level timeouts, no TimeoutHandler |
| 20 | Backend testing | Stdlib `testing`, table-driven, hand-rolled assert; httptest (recorder + full-stack server with cookie jar); testcontainers-go with goose migrations; **every-store-method integration test rule (mandatory — see Compensating Controls)**; River job assertions; two-tenant isolation test in CI |
| 21 | Multi-tenancy | **Shared schema + `tenant_id`, hardened by Postgres RLS** — five-layer defense (below). Schema-per-tenant and DB-per-tenant rejected for this shape |
| 22 | Frontend build | **Vite, SPA mode** — dev proxy to Go, hashed static output; SSR rejected (login-walled dashboard, no SEO, no Node in prod); Next.js/Remix/Start excluded |
| 23 | TypeScript | **Strict mode from file one** + `noUncheckedIndexedAccess`; no `any` house rule |
| 24 | Type sharing | **Go→TS generation (tygo)** from `internal/api/dto` → committed `frontend/src/lib/generated/types.ts`; CI staleness check (`git diff --exit-code`); hand-written typed `api.ts` client. Full OpenAPI pre-designated for public API / mobile client era |
| 25 | Data fetching | **TanStack Query** — queries, mutations + invalidation, typed problem+json errors, `Retry-After`-respecting backoff; typed query-key constants. **No Redux/Zustand** — residual client state in `useState`/context |
| 26 | Client routing | **TanStack Router** — typed params, typed/validated search params (pagination in URL), auth-guarded layout route, Query prefetch in loaders. React Router noted as the safe alternative |
| 27 | Forms | **React Hook Form + server-driven validation** — shallow client checks only; shared 422→`setError` mapper; `useFieldArray` for the offer editor; totals via `useWatch` display-only (server recomputes). **No parallel Zod schema layer** (single-rulebook rule); targeted local refinements permitted as exceptions |
| 28 | Styling/UI | **Tailwind (Vite plugin) + shadcn/ui** — owned copied components on **Base UI** primitives (owner revision from Radix, 2026-07-17 — current shadcn default); brand tokens configured early; components added per-screen, never wholesale. CSS-in-JS and full component libraries rejected |
| 29 | Frontend testing | **Minimal (owner-calibrated, 2026-07-20 revision): unit tests on the few pure functions** (422→`setError` mapper, money formatting, error-code → message mapper). Playwright/journeys layer removed. Rationale: frontend logic deliberately thin, owner relies on manual smoke + the Go-side integration tests for regressions. Decision is reversible later if UI complexity grows. |
| 30 | Repo layout | **Monorepo**: `api/` (Go module) + `frontend/` (Node confined); structure below; no service layer until orchestration demands it |
| 31 | Domain model | Carried from v1 with additions — see Domain Model |
| 32 | Security | See consolidated review — SPA edition |
| 33 | Deployment | **Single artifact: Go binary embeds `frontend/dist`** (same-origin — the keystone of 9/19); Docker multi-stage (Node → Go → distroless, ~25MB); **Hetzner + Dokploy**, Traefik TLS; **nightly `pg_dump` → versioned R2 + rehearsed restores as a launch requirement**. Railway documented as the managed-Postgres alternative |

## Dependency Budget

**Go runtime (9):** `jackc/pgx/v5` (via stdlib driver) · `pressly/goose/v3` · `alexedwards/scs/v2` + pgxstore · `x/crypto/bcrypt` · `johnfercher/maroto/v2` · `wneessen/go-mail` · `aws-sdk-go-v2` S3 client (R2) · `golang.org/x/time/rate` · `riverqueue/river`.
**Go build/dev:** tygo (codegen), testcontainers-go (tests).
Router, config, logging, validation, middleware, CSRF, RBAC: stdlib or hand-written.

**Frontend runtime:** react · @tanstack/react-query · @tanstack/react-router · react-hook-form · tailwindcss · Base UI primitives (via owned shadcn copies) · **@lingui/react + @lingui/core** (i18n, native ICU MessageFormat, 2026-07-20). Package manager: **pnpm** (owner revision from npm, 2026-07-17).
**Frontend dev:** vite · typescript · oxlint · **@lingui/cli + @lingui/macro + @lingui/vite-plugin** (build-time message extraction).
House bias: adding frontend packages requires justification (supply-chain surface).

## Compensating Controls (for the `database/sql` revision)

Dropping sqlc traded compile-time schema-drift detection for hand-written explicitness (owner rationale: AI-assisted boilerplate). The contractual compensations:
1. **Every store method has a real-Postgres integration test** — `go test` plays the role `go build` played. Non-negotiable.
2. **Scan discipline** — column lists as package constants co-located with their scan helpers; query text and scan order live together.
3. **Placeholders always** (`$1…`); string-concatenated SQL is a review-blocker (`fmt.Sprintf` near queries is greppable).
4. RLS (below) backstops any query-logic failure at the database.

## Multi-Tenancy Design (five layers)

1. **Session** — `tenantID` set at login, server-side in scs; never read from URL/header/body. Future bearer tokens resolve to the same server-side truth.
2. **Context** — middleware copies `tenantID` + role into request context; single source for handlers and stores.
3. **Query layer** — every hand-written query on tenant tables includes `tenant_id`, including PK lookups (`WHERE id = $1 AND tenant_id = $2`); store signatures require `tenantID`. Greppable in store files.
4. **RLS backstop** — `USING`/`WITH CHECK` policies on `tenant_id = current_setting('app.tenant_id')::uuid`; per-request tx issues `SET LOCAL app.tenant_id`; app connects as **non-owner, non-superuser role**; `FORCE ROW LEVEL SECURITY`. A forgotten WHERE returns zero foreign rows.
5. **Tests** — mandatory two-tenant CI test attempts every endpoint cross-tenant, asserts zero leakage.

## API Contract Summary

`/api/v1` · plural nouns · camelCase · ISO-8601 UTC · string UUIDs · `POST→201+body`, `PUT` full update, `DELETE→204` · named envelopes (`{"customer": …}`, `{"customers": […], "metadata": {page, pageSize, total}}`) · errors RFC 9457 problem+json with `requestId` always and `errors: {field: message}` on 422 · 401 vs 403 discipline · `GET /healthz` · `X-Request-Id` on every response. DTOs live in `internal/api/dto` — the single source generating TS types; store/domain types never serialize.

## Domain Model

**Tenant** — name, address/VAT, logo key (R2), currency (ISO 4217), locale, default tax rate (basis points), settings.
**User** — `tenant_id`, globally-unique email, bcrypt hash, role enum. *Accepted limitation:* one email = one tenant; escape path: user↔tenant membership table.
**Customer** — person/company, contacts, notes. **Car** — belongs to Customer; plate unique per tenant, VIN, make/model/year, mileage.
**Offer / OfferItem** — `draft → sent → accepted | rejected | expired`; items: description, qty, unit price (cents), kind. *Immutability:* editable only in draft; `sent` freezes content (deterministic PDF regeneration; emailed PDF ≡ regenerable PDF). **`send_status` (`pending → sent | failed`) + `sent_to` + `sent_at`** for River job visibility and UI retry.
**Repair / RepairItem** — separate from Offer; optional `offer_id`; items **copied** on conversion (price freeze); `open → in_progress → completed`; mileage updated on completion.
**Service history — derived, not a table**: completed repairs per car (query/view) ∪ `history_notes` (manual external-work entries).
**Attachments** — one metadata table (R2 key, name, size, sniffed type); nullable `car_id`/`repair_id` FKs + CHECK exactly-one; no polymorphic owner columns.
**AuditLog** — append-only (tenant, user, action, entity, `jsonb` payload, ts); written explicitly in mutating handlers; pruned by River periodic job.
**Money** — integer cents: `bigint` → `int64`/`domain.Money` → TS `number` (2⁵³ headroom); tax as basis points; line totals computed; offer total snapshotted at send.
**Deletion policy** — Customers/Cars soft-delete (`archived_at`); Offers/Repairs never deleted; hard delete only for dependent-free records; FKs `RESTRICT`, never `CASCADE`.
Infrastructure tables (non-domain): scs sessions, River job tables (self-migrated), password-reset tokens.

## Repository Structure

```
api/
  cmd/api/            # main: config, DI, serve | migrate
  internal/
    config/  domain/  store/           # pool, RLS tx helper, hand-written SQL stores
    api/                                # handlers, routes, middleware, dto/ (TS-gen source), problem+json render
    validator/  jobs/                   # River workers + periodic
    pdf/  mailer/  filestore/
  migrations/  go.mod  tygo.yaml
frontend/
  src/ (routes/ components/ui lib/{api,generated,queryKeys})  e2e/  package.json
Makefile  docker-compose.yml (postgres, mailpit, minio)  Dockerfile  README.md
```

Dependency direction: `domain` → imported by `store`/`pdf`/`mailer` → imported by `api` → wired in `cmd/api`. Interfaces defined where consumed.

## Type-Sharing Pipeline

`internal/api/dto` structs (json tags) → **tygo** → `frontend/src/lib/generated/types.ts` (committed, never hand-edited; pointers→`|null`, const blocks→literal unions, `time.Time`→string, `int64`→number) → consumed by hand-written typed `api.ts` (~30 endpoint functions, envelope unwrapping, typed problem+json errors) → consumed by Query hooks. **CI fails on stale generation.** Fallback if tygo ever blocks: ~150-line owned `go generate` script.

## Security Design (SPA edition)

**CSRF** — required because cookies: `SameSite=Lax` + Go 1.25 `http.CrossOriginProtection`. **CORS — deliberately absent** (same-origin prod; Vite proxy dev; CORS = deliberate SOP relaxation we never grant; scoped middleware only if a legitimate cross-origin consumer appears).
**XSS** — React JSX escaping; `dangerouslySetInnerHTML` banned on user data; strict CSP on SPA shell (`default-src 'self'; script-src 'self'; frame-ancestors 'none'` — achievable because no third-party scripts, hashed self-hosted bundle); API `no-store` + `nosniff`; attachments `Content-Disposition: attachment`.
**SQLi** — placeholder discipline (review-blocker rule) + RLS backstop.
**Cookies** — HttpOnly, Secure, SameSite=Lax, random ID only. **Fixation** — `RenewToken` on login/privilege change.
**Brute force** — strict auth limits, per-email throttle, bcrypt cost, failure logging.
**Password reset** — token hashed (SHA-256) at rest, 1h expiry, single-use; success deletes all tokens + destroys other sessions; link targets frontend route `/reset-password?token=…` which POSTs to API.
**Enumeration** — identical responses + dummy bcrypt timing on unknown emails (login and reset).
**Uploads** — size caps, server-side sniffing, tenant prefixes, authorized streaming only.
**npm supply chain (new surface)** — committed `pnpm-lock.yaml`, `pnpm install --frozen-lockfile` in CI, `pnpm audit` signal, Renovate/Dependabot, bias against new packages; strict CSP (`connect-src 'self'`) as runtime exfiltration backstop.
**Transport** — TLS + HSTS at Traefik.
*Pivot ledger:* gained JSX default escaping + unusually strict CSP; paid with npm supply chain + cookie-CSRF care. Both handled; trade conscious.

## Owner Overrides & Revisions (on record)

- **Raw `database/sql` over sqlc** — rationale: AI-assisted boilerplate; price: the Compensating Controls section.
- **bcrypt over argon2id** — cost 12, 72-byte cap enforced; migration path open.
- **River from day one** — durable, transactional jobs on existing Postgres; asynq+Redis argued down (non-transactional enqueue, extra infra).
- **R2 from day one** — durability + multi-instance readiness; MinIO for dev.
- **Send-offer-email in MVP** — Reply-To pattern; freeze-on-send.
- **Minimal frontend testing** — 3 Playwright journeys + pure-function units; RTL/MSW deferred (inverted from unit-only after discussion: wiring bugs, not logic bugs, are the frontend risk).
- **Base UI over Radix (2026-07-17)** — shadcn/ui's current default registry is Base UI; owner directive. No shadcn components had been added yet, so the switch costs nothing.
- **pnpm over npm (2026-07-17)** — owner directive; lockfile is `pnpm-lock.yaml`, CI uses `pnpm install --frozen-lockfile`.
- **LinguiJS for i18n (2026-07-20)** — chosen over react-i18next and react-intl for: (1) Bulgarian requires ICU MessageFormat for correct count plurals (singular / count-form / many-form) and grammatical number, (2) compile-time message extraction matches the "everything must compile" house rule, (3) smallest runtime (~5 kB gz vs ~23 kB for react-i18next). One locale for now (`bg`); no fallback. Locale detected from `localStorage["pitlane:locale"]` then `navigator.language` then `bg`.
- **Drop Playwright, keep unit tests only (2026-07-20)** — frontend journeys removed entirely. Owner rationale: manual smoke + Go integration tests cover regressions. ADR §29 revised. Decision is reversible; re-add at the next complexity plateau.
- **Stable error codes in problem+json (2026-07-20)** — server emits a machine-readable `code` field alongside the human-readable `title`/`detail`. Client maps `code` → locale message. Decouples error translation from server-side string drift. The `errors` field map on 422 follows the same pattern: each value carries a code-prefixed message that the SPA unwraps via the `errorCodeToMessage` mapper.

## Consequences & Known Tradeoffs

- Schema drift is caught by tests, not the compiler (sqlc revision) — hence the mandatory store-test rule.
- Single-instance assumptions (in-memory rate limiter) deliberate; multi-instance upgrade paths named.
- Offer emails ride River with `send_status` visibility; SMTP provider deliverability (SPF/DKIM on app domain) is launch-checklist work.
- TanStack Router is younger than React Router — accepted for typed routes/search params; React Router is the documented retreat.
- tygo covers types, not endpoints — the hand-written client is the accepted gap; OpenAPI is the named upgrade at public-API/mobile time.
- Frontend-only changes redeploy the whole binary — non-cost at this scale; split serving is the named escape if ever needed.
- Postgres durability is ours: nightly dump → R2 + rehearsed restore is architecture, not ops trivia.

## Scaffolding Plan (upon approval — no code yet)

Monorepo skeleton per §30: Go module (config, slog, middleware chain, routes with TODO handlers, migration 0001 with RLS scaffolding, River setup, interface stubs, healthz), Vite+TS+Tailwind+shadcn init with router/query/client scaffolding, tygo pipeline wired (`make types` + CI check), Makefile, Dockerfile (3-stage), docker-compose (postgres, mailpit, minio), README. Everything compiles; both dev servers run; TODOs mark deferred implementation.
