# pitlane

Multi-tenant SaaS dashboard for car service owners: Go JSON API + React SPA, shipped as a single binary. Architecture contract: [`ADR.md`](ADR.md) · execution plan: [`AGENT_PLAN.md`](AGENT_PLAN.md) · milestones: [`ROADMAP.md`](ROADMAP.md).

## Prerequisites

- Go (version in `api/go.mod`)
- Node.js 24+ with **pnpm** (`corepack enable` if missing)
- Docker with Compose v2

## Quickstart

```sh
make dev
```

First run copies `.env.example` → `.env`, starts postgres/mailpit/minio, then runs the API and the Vite dev server in parallel.

| What          | URL                          |
| ------------- | ---------------------------- |
| App (Vite)    | http://localhost:5173        |
| API           | http://localhost:4000/api/v1 |
| Mailpit UI    | http://localhost:8025        |
| MinIO console | http://localhost:9001        |

## Commands

| Command               | What it does                                             |
| --------------------- | -------------------------------------------------------- |
| `make dev`            | Deps up + Go API + Vite dev server (proxy to :4000)      |
| `make test`           | Go tests (unit + testcontainers integration, Phase 2+)   |
| `make types`          | Regenerate `frontend/src/lib/generated/types.ts` (tygo)  |
| `make build`          | Production Docker image (`pitlane:latest`)               |
| `make migrate-up` / `migrate-down` / `migrate-status` | goose + River migrations via the api binary |
| `make audit`          | go vet, tsc, oxlint, pnpm audit, tygo staleness, Lingui catalog |
| `make deps` / `deps-down` | Start/stop the docker-compose stack                  |

## Production rehearsal

The single deployable artifact is the Docker image: one binary serves the API
and the embedded SPA, migrations run only as an explicit operator command.

```sh
make build                                                # pitlane:latest
docker compose -f docker-compose.prod.yml up -d db
docker compose -f docker-compose.prod.yml run --rm app migrate up
docker compose -f docker-compose.prod.yml up -d app       # http://localhost:4000
```

## Backups

`scripts/backup.sh` streams `pg_dump | gzip` into the `pitlane-backups` MinIO
bucket (R2 in production; override with the `BACKUP_S3_*` env vars). It only
writes timestamped objects — retention is a bucket lifecycle policy. Phase D
schedules it nightly and rehearses a restore.

## Adding an entity (the customer pattern)

Customers (Phase 3) are the reference slice every later entity copies. Work in
this order — each step compiles before the next, and nothing skips (AGENTS.md
"workflow per slice"). File references point at the customer implementation.

1. **Migration** — `api/migrations/sql/NNNN_<entity>.sql`. Table with
   `tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT`, plain
   `NOT NULL DEFAULT ''` columns for optional strings (Go reads them as plain
   strings — no `sql.NullString`), `archived_at timestamptz` for soft-deletes.
   Then the five-layer tenancy boilerplate: `GRANT … TO pitlane_app`,
   `ALTER TABLE … FORCE ROW LEVEL SECURITY`, and a
   `<entity>_isolation` policy with `USING` **and** `WITH CHECK` on
   `tenant_id = current_setting('app.tenant_id')::uuid`. Index `(tenant_id, …)`
   for the list ordering. **A tenant-owned table without its RLS policy is a
   blocking bug.**
2. **Domain** — `internal/domain/domain.go`: the plain struct (`*time.Time` for
   `archived_at`).
3. **Store** — `internal/store/<entity>.go`. Column-list constant + `scan<Entity>`
   helper co-located (scan order matches the column list). Every method runs
   inside `db.WithTenant(ctx, tenantID, …)` and filters on `tenant_id` — even
   primary-key lookups. `Get`/`Update`/`Archive` return `store.ErrNotFound` on
   zero rows. **Every method gets a real-Postgres integration test**
   (`internal/store/<entity>_test.go`), including a cross-tenant invisibility
   case — this is the compensating control for raw `database/sql`, not optional.
4. **DTO** — `internal/api/dto/dto.go`: `…Response`, `Create…Request`,
   `Update…Request`, and reuse `ListMetadata` for list envelopes. Only DTOs
   serialize; domain/store structs never cross the boundary.
5. **Handlers** — `internal/api/<entity>_handlers.go`. Decode → trim →
   `validate…` (returns the field→code map → 422 via `renderValidation`) →
   store call → **audit-log write on every mutation** → render. Single-entity
   responses use a named envelope (`{"customer": …}`); lists return
   `{"customers": [...], "metadata": {...}}`. Map `ErrNotFound` → 404.
6. **Routes** — `internal/api/routes.go`: wrap each route with
   `s.protected(domain.Permission…Read|Write, handler)` (requireAuth +
   requirePermission). Soft-delete is `POST /…/{id}/archive` → 204, leaving
   `DELETE` free for a future hard-delete of dependent-free records
   (ADR §Deletion policy). Wire the store into `Server`, `ServerDeps`,
   `NewServer`, `cmd/api/main.go`, and the test harness in `isolation_test.go`.
7. **Types** — `make types` (tygo). Never hand-edit `generated/`.
8. **Client** — `frontend/src/lib/api.ts` (single-key envelopes via `request`,
   whole-body list responses via `requestBody`) + `frontend/src/lib/queryKeys.ts`.
9. **UI** — a list route with typed URL search params (`validateSearch`), a
   detail route, and create/edit/archive dialogs. Reuse the RHF +
   `applyServerErrors` form pattern; every user-facing string flows through
   `<Trans>` / `t` with a real Bulgarian translation in `locales/bg.po`.
10. **Isolation test** — extend the two-tenant test
    (`internal/api/<entity>_handlers_test.go`) so tenant B provably cannot list,
    read, update, or archive tenant A's rows. Every entity joins this test.

## House rules

- `frontend/src/lib/generated/` is generated by tygo — never hand-edit; run `make types` after changing DTOs.
- Package manager is **pnpm**; lockfile is `pnpm-lock.yaml`, CI installs with `pnpm install --frozen-lockfile`.
- See `AGENTS.md` for the full standing instructions.
