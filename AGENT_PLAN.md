# AGENT_PLAN.md — Phase-by-Phase Execution Plan

For the Code agent. Rules of engagement live in `AGENTS.md`; decisions live in `ADR.md`. Phases run strictly in order; each ends with a **Verification gate** the agent runs and shows, then an **owner approval stop**.

**Deployment note:** the owner will buy the Hetzner VPS *later*. Therefore all phases run against the local docker-compose stack, the app is kept *deployment-ready* from Phase 1 (Dockerfile + prod config), and actual VPS deployment is the final phase (D), executable whenever the server exists.

---

## Phase 0 — Monorepo scaffold

**Objective:** both worlds exist, compile, and talk to each other through the type pipeline.

Steps:
1. Repo root: `git init`; add `Makefile`, `README.md`, `.gitignore` (Go + Node + `frontend/dist`), `docker-compose.yml` with services: `postgres:18` (healthcheck — ADR "latest stable major"; mount data at `/var/lib/postgresql`, not the pre-18 path), `mailpit` (SMTP 1025 / UI 8025), `minio` (+ bucket bootstrap job). `.env.example` with every variable the config struct will read (PORT, ENV, DSN, SESSION_*, SMTP_*, R2_*, TRUSTED_PROXY).
2. `api/`: `go mod init`; `cmd/api/main.go` (load config → slog → pgx pool stub → HTTP server with graceful shutdown); `internal/config` (typed env helpers: string/int/duration/bool, fail-fast validation); `internal/api` with `routes.go`, `middleware.go` (recover→problem+json 500, request ID, logging, security headers — the outer four only), `render.go` (envelope + problem+json helpers), `dto/` (empty `HealthResponse` to prime the pipeline); `GET /api/v1/healthz`.
3. `frontend/`: Vite React-TS scaffold; `tsconfig` strict + `noUncheckedIndexedAccess`; Tailwind via Vite plugin; shadcn init (style tokens only, no components yet); TanStack Router (root layout + index route) and Query (provider, devtools in dev); `src/lib/api.ts` (typed fetch wrapper: base `/api/v1`, envelope unwrap, `ProblemError` class); `vite.config.ts` dev proxy `/api → http://localhost:4000`.
4. tygo: `api/tygo.yaml` targeting `internal/api/dto` → `../frontend/src/lib/generated/types.ts`; `make types`; index page calls `/healthz` through the typed client and renders the result.
5. Makefile targets per AGENTS.md; CI-style `make audit` including tygo staleness check (`make types && git diff --exit-code frontend/src/lib/generated`).

**Verification gate:** `docker compose up -d` healthy → `make dev` → browser shows healthz data rendered via generated types; `go vet ./...` and `tsc --noEmit` clean; `make audit` green. **STOP for approval.**

---

## Phase 1 — Deployment-ready walking skeleton (no server yet)

**Objective:** the production artifact exists and runs, even though nowhere to host it yet.

Steps:
1. goose: `migrations/` embedded; `api migrate up|down|status` subcommand (cobra-free — plain `os.Args` switch in main, Edwards-style).
2. SPA serving: `internal/api/spa.go` — serve embedded `dist` (immutable cache headers on hashed assets), `index.html` fallback for non-`/api` routes. Behind a build tag or dir-exists check so `make dev` (no dist) still works.
3. `Dockerfile` (3 stages): node → `vite build`; golang → copy `frontend/dist` into `api/` embed path, `CGO_ENABLED=0` build, run migrations OFF by default; distroless final. Target image < ~40MB.
4. River: dependency added, client + worker started in main (empty worker set), its migrations run alongside goose. Graceful shutdown drains workers.
5. `docker-compose.prod.yml` (or compose profile): the built image + postgres, for a full local prod rehearsal.
6. `scripts/backup.sh`: `pg_dump | gzip → R2 (or MinIO locally)` + retention note; wire nothing yet — Phase D schedules it.

**Verification gate:** `make build` succeeds; `docker compose -f docker-compose.prod.yml up` serves the SPA and API from ONE container at :4000; `api migrate status` works inside the image; backup script tested against MinIO. **STOP.**

---

## Phase 2 — Tenancy foundation + authentication

**Objective:** signup/login works end to end; RLS is live; the isolation test exists.

Steps:
1. Migration 0001: `tenants`, `users` (global-unique email, bcrypt hash, role enum), scs `sessions` table, `password_reset_tokens` (used later); RLS policies (`USING` + `WITH CHECK`) on tenants-owned tables; app DB role: non-owner, non-superuser; `FORCE ROW LEVEL SECURITY`.
2. `internal/store`: pool setup; `store.WithTenant(ctx, tenantID, fn)` tx helper issuing `SET LOCAL app.tenant_id`; `UserStore`, `TenantStore` with integration tests (testcontainers harness built here: container per test binary, goose up, per-test cleanup).
3. Auth endpoints: `POST /auth/signup` (creates tenant + owner, validator in play), `POST /auth/login` (dummy-bcrypt on unknown email, RenewToken, 204 + cookie), `POST /auth/logout`, `GET /auth/me` (user + computed permissions). scs LoadAndSave, CSRF (`http.CrossOriginProtection`), strict auth rate limiter, `requireAuth` (401) + tenant-context middleware + `requirePermission` (403) — completing the middleware chain per ADR §19.
4. `internal/domain`: Role, Permission, the `map[Role][]Permission`.
5. Frontend: `/login`, `/signup` routes (RHF + the 422→setError mapper built here as `lib/formErrors.ts`); auth-guarded layout route using a `useMe` query; 401 interceptor in `api.ts` → redirect to login preserving destination.
6. **Two-tenant isolation test**: harness that will grow with every entity — signs up A and B, asserts B sees none of A via API calls.
7. ~~Playwright setup + **Journey #1**: signup → login → dashboard shell renders.~~ (Playwright removed 2026-07-20 — see ADR §29 revision; manual smoke replaces the journey. UI verification done by hand + a small unit-test layer for pure functions per Phase 2.5.)

**Verification gate:** `make test` green including isolation test; manual: signup two tenants in two browsers, verify separation; 401 vs 403 verified with curl. **STOP.**

---

## Phase 2.5 — i18n foundation (Bulgarian only, no fallback)

**Objective:** every user-facing string in the SPA flows through `@lingui/react`; Bulgarian is the default (and only) locale; server error messages reach the UI as stable codes that the SPA maps to translated strings. After this phase, no new code commits English strings to JSX without `<Trans>` / `t`. Added 2026-07-20 per owner direction; precedes Phase 3 so the pattern-setting slice is built i18n-native.

Steps:
1. Install LinguiJS: `@lingui/react`, `@lingui/core` (runtime); `@lingui/macro`, `@lingui/vite-plugin`, `@lingui/cli` (build/extract). Configure `lingui.config.ts` with `locale: ["bg"]`, `fallbackLocale: "bg"`, `sourceLocale: "bg"`. Wire `vite-plugin-lingui` in `vite.config.ts`. `make types` and `make audit` updated to call `pnpm lingui extract` + `pnpm lingui compile` so a missing translation breaks CI.
2. `frontend/src/i18n/`: `Provider.tsx` mounted in `main.tsx`; `config.ts` (i18n instance, default `"bg"`); `detector.ts` (localStorage → navigator → `bg`); `format.ts` (currency/number/date helpers via `Intl.*` for `bg-BG`).
3. `frontend/src/locales/bg.po` — single source of truth. Every hardcoded string in the current 5 routes + 2 RHF rules + index.html title gets a real Bulgarian translation with native grammar (count plurals, definite articles, gender agreement). Translations reviewed by the owner, not auto-generated from the English.
4. Replace hardcoded JSX with `<Trans>` / `t` macros in `routes/login.tsx`, `routes/signup.tsx`, `routes/index.tsx`, `routes/_authed.tsx`, `routes/_authed/dashboard.tsx`. `<html lang="bg">` synced from the active locale in `main.tsx`. Document title moved to a React-managed `<Helmet>`-equivalent (TanStack Router `head`).
5. **Server-side error codes**: add a `code` field to `problemDetail` in `internal/api/render.go`. Emit codes from every error site:
   - validator → `required`, `invalid_email`, `too_short`, `too_long`, `invalid`
   - `auth_handlers.go` → `invalid_credentials` (the three 401 sites), `invalid_json` (400), `internal_error` (500)
   - the existing `errors` map on 422 keeps its shape (field → string) but the string is now a stable code-prefixed message like `required:изисква се` so the client can dispatch on the prefix
6. **Client mapper**: `frontend/src/lib/errorCodes.ts` — `errorCodeToMessage(code, field) → string` table, BG strings. `lib/formErrors.ts` updated: if the value is `"code:message"`, use the code for dispatch and the BG message for display. Unknown codes fall back to the server-supplied message.
7. `make audit` extends: `pnpm lingui extract` then `git diff --exit-code frontend/src/locales` (catches untranslated strings); `pnpm lingui compile` to make sure catalog is valid; tygo staleness; `tsc --noEmit`; `go vet ./...`; `pnpm audit`.

**Verification gate:** `pnpm dev` shows Bulgarian strings in the browser; toggle locale persistence works; 401/422 errors render in Bulgarian from the code map; no console warnings; `tsc --noEmit` and `go test ./...` and `go vet ./...` clean; `make audit` green. **STOP.**

---

## Phase 3 — Customers (pattern-setting slice — extra review)

**Objective:** the full-stack CRUD pattern every later entity copies.

Steps:
1. Migration: `customers` (+ `archived_at`, RLS, indexes on `(tenant_id, name)`).
2. `CustomerStore` (List w/ pagination+search, Get, Create, Update, Archive) — column-list constants + scan helpers; integration tests per method incl. tenant scoping.
3. Handlers + DTOs (`CustomerResponse`, `CreateCustomerRequest`…): validator rules, 422s, audit log writes, envelope + metadata on list.
4. `make types`; `api.ts` customer functions; `lib/queryKeys.ts` started.
5. UI: customers list route (typed search params: `?page&search&archived`), detail, create/edit forms (shadcn: table, dialog, input, button — added now), archive with confirm; Query mutations + invalidation.
6. Extend isolation test with customers endpoints.
7. Document the slice pattern in README ("how to add an entity").

**Verification gate:** full CRUD in browser incl. validation errors under fields, pagination in URL surviving refresh; all tests green; owner reviews the PATTERN, not just the feature. **STOP — this approval blesses the template.**

---

## Phase 4 — Cars
Pattern replication: migration (plate unique per tenant, VIN, make/model/year, mileage, `archived_at`, RLS), store+tests, handlers+DTOs, types, UI nested under customer detail. Isolation test extended.
**Gate:** CRUD green, tests green, pattern held without modification (deviations = discuss). **STOP.**

## Phase 5 — Offers + line-item editor
Migrations: `offers` (status enum, `send_status`, totals snapshot cols, RLS) + `offer_items`. Domain: Money (int64 cents) ops, tax basis-points calc — unit tested. Store: tx-based create/update with items; immutability enforced (draft-only writes; 409 problem+json otherwise). Handlers: CRUD + `POST /offers/{id}/status`. UI: `useFieldArray` editor, `useWatch` display totals, status actions. Isolation test extended.
**Gate:** quote lifecycle draft→sent in browser; server totals exact (table-driven tests incl. rounding); post-send edit → 409 with clean UI message. **STOP.**

## Phase 6 — Offer PDF
`internal/pdf`: `OfferRenderer` interface + maroto impl (tenant logo from FileStore — stub local until Phase 9 if needed, address/VAT, locale formatting, items table, totals, terms). `GET /offers/{id}/pdf` streaming + `?disposition=inline`. UI: iframe preview dialog + download link.
**Gate:** golden-flow test asserts PDF generates non-empty w/ correct metadata; visual check by owner; preview + download work in browser. **STOP.**

## Phase 7 — Send offer by email (River's debut)
`internal/mailer`: `Mailer` interface, go-mail SMTP impl, embedded offer email template (+ text alt). River: `SendOfferEmail` job (typed args), worker calls Mailer with generated PDF attached; enqueue **in the same tx** as status/`send_status` change. `POST /offers/{id}/send` (recipient prefilled from customer, Reply-To garage). UI: send dialog, `send_status` badge, retry action. Tests: tx-enqueue assertion, worker→mock-Mailer, Mailpit end-to-end.
**Gate:** email with PDF lands in Mailpit UI; kill-the-API-mid-send rehearsal shows River retry; manual walkthrough of customer→car→offer→send by the owner. **STOP.**

## Phase 8 — Repairs + conversion
Migrations `repairs`/`repair_items` (RLS, `offer_id` nullable FK). Accept-offer endpoint: tx copies items (price freeze), links provenance. Status flow open→in_progress→completed; mileage update on completion. UI: repairs board/list, convert action, item editing while open.
**Gate:** full offer→repair→completed lifecycle in browser; copy-not-share of items proven by test (edit repair item, offer unchanged). **STOP.**

## Phase 9 — Service history + attachments
History: SQL view/query (completed repairs ∪ `history_notes`), notes CRUD (RLS), car-detail timeline UI. Attachments: migration (nullable car_id/repair_id + CHECK exactly-one, RLS); `internal/filestore` R2 impl (MinIO locally); multipart upload handler (size cap, sniffed content-type), streaming download w/ `Content-Disposition: attachment`; UI photo upload/gallery on cars and repairs.
**Gate:** timeline correct incl. manual note; upload/download via MinIO green in tests and browser; oversized/spoofed-type uploads rejected. **STOP.**

## Phase 10 — Password reset + staff invitations
Reset per ADR §Security: hashed tokens, 1h, single-use, all-sessions destroyed, enumeration-safe, per-email throttle (Postgres), River-sent email; frontend `/forgot-password` + `/reset-password?token=` routes. Invitations: owner invites email+role, invite token → account completion; role management UI (permission-gated).
**Gate:** full reset via Mailpit; second use of token fails; mechanic invited then blocked from owner action — verified by the owner manually. **STOP.**

## Phase 11 — Hardening + release candidate
River periodic jobs (expired tokens, audit prune). Global rate limiter verified (429 + Retry-After; burst fits SPA page-mount). CSP audited against built bundle (no violations in console). Loading/empty/error states pass on every route; 404 route; problem+json rendering audited. `pnpm install --frozen-lockfile` + audit in `make audit`; Renovate config. Log line review (request/tenant/user IDs everywhere). Restore rehearsal from `scripts/backup.sh` output. Ops section in README.
**Gate:** `make audit && make test` all green from a clean clone; owner walkthrough of the full app; tag `v0.1.0-rc1`. **STOP — app is done pending a server.**

---

## Phase D — Deployment day (when the Hetzner VPS is purchased)

Owner prerequisites: Hetzner Cloud account; a small VPS (CX22-class, ~€4–8/mo, Ubuntu LTS) — resize later is trivial; a domain; R2 bucket + credentials (prod + backups); SMTP provider account (Resend) with SPF/DKIM DNS records for the app domain.

Agent/owner steps:
1. Server basics: SSH key-only, non-root user, ufw (22/80/443), unattended-upgrades.
2. Install Dokploy (official script); DNS A record → server; Dokploy project: app from the Dockerfile (or a registry image via CI), env vars from `.env.example` filled with prod values; postgres service with volume; Traefik TLS via Let's Encrypt.
3. `api migrate up` against prod DB (one-off command through Dokploy).
4. Schedule `scripts/backup.sh` nightly → versioned R2 backups bucket (Dokploy scheduled task or cron); **perform and document one restore rehearsal immediately**.
5. Point SMTP config at Resend; send a real offer email to an owned address; verify SPF/DKIM pass (headers).
6. Smoke: healthz, signup, login, offer→PDF→send on prod.

**Gate:** live domain, green TLS, backups scheduled AND restore proven, real email delivered. Tag `v0.1.0`. Launch.

---

## Standing verification (every phase)
`go vet ./...` · `go test ./...` · `tsc --noEmit` · tygo staleness check · Lingui catalog check (extracted messages match compiled catalog) · lint clean · repo compiles at every commit.
