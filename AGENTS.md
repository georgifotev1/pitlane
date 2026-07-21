# AGENTS.md — Standing Instructions for this Project

## What this project is
A multi-tenant SaaS dashboard for car service owners: Go JSON API + React SPA, single deployable binary. Read `ADR.md` (the architecture contract) and `AGENT_PLAN.md` (the execution plan) before doing anything. `ROADMAP.md` gives the milestone narrative.

## Non-negotiable rules
1. **The ADR is law.** Do not deviate from any ADR decision without explicitly asking the owner first. If something seems wrong or outdated, stop and ask — never silently substitute.
2. **One phase at a time.** Work through `AGENT_PLAN.md` phases in order. Before writing code for a phase: briefly explain what you're about to build and why. After completing a phase: run its verification gate, show the results, and WAIT for owner approval before starting the next phase.
3. **Mentor mode.** Explain the code you write — the owner is learning this architecture, not just receiving it. Explain the Alex Edwards reasoning where it applies.
4. **Everything must compile and pass at every stop point.** Never leave the repo in a broken state between phases. Use TODO comments for intentionally deferred work.

## Architecture quick reference (details in ADR.md)
- Go: latest stable, stdlib ServeMux, slog, raw `database/sql` over `pgx/v5/stdlib`, goose (embedded), scs sessions (cookies), bcrypt (cost 12), River jobs, maroto PDFs, go-mail SMTP, R2 via AWS SDK v2, `x/time/rate`.
- API: `/api/v1`, camelCase JSON, named envelopes, RFC 9457 problem+json (with `requestId`; `errors` map on 422), offset pagination, 401 vs 403 discipline.
- Frontend: Vite SPA, TypeScript strict, TanStack Router + Query, React Hook Form (server-driven errors — NO Zod schema layer), Tailwind + shadcn/ui (**Base UI** primitives), tygo-generated types.
- Package manager is **pnpm** (lockfile `pnpm-lock.yaml`; CI install: `pnpm install --frozen-lockfile`). Never use npm/yarn commands.
- Multi-tenancy: shared schema + `tenant_id` + Postgres RLS (five layers — ADR §Multi-Tenancy). NEVER ship a tenant-owned table without its RLS policy.
- Same-origin everywhere: Vite proxy in dev, embedded SPA in prod. No CORS middleware.

## House laws (violations block the phase)
- Every SQL query uses placeholders (`$1…`). `fmt.Sprintf` anywhere near SQL is forbidden.
- Every query on tenant-owned tables includes `tenant_id`, including primary-key lookups. Store method signatures take `tenantID` explicitly.
- **Every store method gets a real-Postgres integration test** (testcontainers). This is the compensating control for choosing raw database/sql — it is mandatory, not aspirational.
- Every new tenant-owned table: RLS policy in the same migration + added to the two-tenant isolation test.
- Every mutating handler writes the audit log.
- Store/domain structs never serialize to JSON. Only `internal/api/dto` types cross the API boundary.
- `frontend/src/lib/generated/` is never hand-edited. After any DTO change: run `make types`, fix TS breakage, commit together.
- No `any` in TypeScript. `dangerouslySetInnerHTML` forbidden on user data.
- No new dependencies (Go or JS) without asking the owner. The ADR dependency budget is the allowed list.
- Bullet lists of new middleware, layers, or abstractions "for later" — don't. YAGNI until the plan says otherwise.

## Commands (keep these working from Phase 0 onward)
- `make dev` — compose deps up + Go API + Vite dev server (proxy to :4000)
- `make test` — Go tests (unit + testcontainers integration)
- `make types` — tygo: regenerate frontend/src/lib/generated/types.ts
- `make build` — production Docker image
- `make migrate-up / migrate-down / migrate-status` — goose via the api binary
- `make audit` — go vet, pnpm audit, tygo staleness check, Lingui catalog check (extracted messages match compiled catalog)

## Workflow per slice (Phases 4+ follow this order strictly)
migration → store (+ integration test) → handler (+ httptest) → dto → `make types` → api.ts client + query keys → UI → verification gate.

## Git discipline
Commit at every green verification gate with a message naming the phase. Never commit with failing tests or stale generated types.
