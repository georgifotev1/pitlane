# Implementation Roadmap v2 — Go API + React SPA

Companion to `ADR.md` (v2). Milestones are dependency-ordered vertical slices — each cuts through **both codebases** (migration → store → handler → DTO → `make types` → client → UI) and ends testable. One milestone = one approval-gated development unit.

## M0 — Scaffold (both worlds)
Monorepo per ADR §30. Go: config, slog, middleware chain, routes w/ TODOs, goose+River wiring, interface stubs, healthz. Frontend: Vite+TS(strict)+Tailwind+shadcn init, TanStack Router shell, Query provider, typed `api.ts` skeleton. tygo pipeline live (`make types` + CI staleness check). Makefile, 3-stage Dockerfile, compose (postgres, mailpit, minio), README.
**Exit:** `make dev` runs both servers with Vite proxy; a page renders data from a stub endpoint through generated types; `go test ./...` green; image builds.

## M1 — Walking skeleton, deployed
Migration 0001; `api migrate` subcommand; graceful shutdown (draining River workers); SPA fallback + embedded `dist` serving; deploy to Hetzner/Dokploy with Traefik TLS; nightly `pg_dump → R2` configured and **one restore rehearsed**.
**Exit:** the app is live on a real domain, one container, green healthz, proven backup path. Deployment risk retired in week one.

## M2 — Tenancy foundation + auth
Migrations: tenants, users, sessions, RLS policies (FORCE RLS, non-owner role). Store RLS tx helper. API: signup (tenant+owner), login/logout (scs, bcrypt, RenewToken), `GET /auth/me` (+permissions), CSRF active, strict auth rate limits, 401/403 problem+json discipline. Frontend: login/signup pages, auth-guarded layout route, `useMe` hook, 401→login redirect wiring.
**Exit:** two tenants sign up and log in via the SPA; **two-tenant isolation test** in CI (mandatory forever); manual smoke of the login → dashboard flow by the owner.

## M3 — Customers (the pattern-setting slice)
Full vertical: list (paginated, typed search params in URL) / detail / create / edit / archive. Go validator → 422 → shared `setError` mapper; audit log writes; soft-delete. **This slice defines the house pattern both codebases copy** — extra care and review here.
**Exit:** customer management usable end to end; store integration tests + handler tests green; pattern documented in README.

## M4 — Cars
Pattern replication under Customer: plate unique/tenant, VIN, mileage; archive rules.
**Exit:** cars attach to customers; the M3 pattern confirmed cheap to copy.

## M5 — Offers + line-item editor
Status machine, cents math, tenant default tax; `useFieldArray` editor with add/remove rows, `useWatch` display totals (server recomputes); immutability enforced (draft-only edits).
**Exit:** a complete quote drafted and marked sent; totals exact server-side; frozen after send.

## M6 — Offer PDF
maroto behind `OfferRenderer`: tenant branding (R2 logo), locale formatting, item table, totals, terms. `GET /offers/{id}/pdf` streaming; `?disposition=inline`; SPA iframe preview + download button (cookie rides along — no blob dance).
**Exit:** sent offer previews in-app and downloads as a clean, correct PDF.

## M7 — Send offer by email (River's debut)
Mailer SMTP impl (go-mail) + offer email template; `POST /offers/{id}/send` enqueues River job **in the same tx** as status change; `send_status` lifecycle (`pending→sent|failed`) surfaced in UI with retry; Reply-To pattern; Mailpit assertions in tests. SPF/DKIM checklist for the app domain documented.
**Exit:** offer email with PDF attachment lands in Mailpit locally end to end; owner walkthrough of customer→car→offer→send passes.

## M8 — Repairs + conversion
Repair CRUD; accept-offer→create-repair copying items (price freeze); status flow; mileage update on completion.
**Exit:** offer-to-repair lifecycle complete.

## M9 — Service history
Derived query/view (completed repairs ∪ `history_notes`), chronological on car detail; manual external-work entry form.
**Exit:** full history per car, zero duplicated data.

## M10 — Attachments
Multipart upload through `FileStore`→R2 (size caps, server-side sniffing, tenant prefixes); authorized streaming download; photos on cars and repairs; MinIO-backed tests.
**Exit:** damage photos upload/download safely.

## M11 — Password reset + staff invitations
Reset per ADR §32 (hashed tokens, expiry, single-use, session destruction, enumeration-safe, per-email throttle); frontend `/reset-password` route; owner invites staff with role assignment (exercises permission map + 403 UX).
**Exit:** reset works against Mailpit and prod SMTP; owner verifies mechanic hits owner-only action → clean 403.

## M12 — Hardening + launch polish
River periodic jobs (token cleanup, audit pruning); CSP verified strict in the built bundle; `pnpm install --frozen-lockfile`/audit/Renovate in CI; error/empty/loading states pass; 404/500 experiences; log review; restore rehearsal #2; ops README.
**Exit:** MVP launch-ready.

## Post-MVP backlog (likely order)
Offer versioning/revert flow · dashboard/reporting widgets · OpenAPI spec + generated clients (trigger: public API or React Native app) · bearer-token issuance for mobile · OAuth login · multi-garage user membership · per-garage sending domains (their DNS, DKIM) · RTL/MSW component test layer if UI complexity grows · presigned URL transfers if bandwidth ever demands.

## Standing rules, every milestone
Migration → store (+ **mandatory integration test per store method**) → handler → DTO → `make types` → client → UI, in that order. Every new tenant table ships its RLS policy and joins the isolation test. Every mutating handler writes the audit log. Placeholders always; `fmt.Sprintf` near SQL blocks review. Generated types never hand-edited. No milestone starts before the previous is approved.
