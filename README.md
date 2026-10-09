# Pitlane

A small, server-rendered garage manager built in Go with PostgreSQL.

Pitlane replaces paper notes and spreadsheets for an independent garage: it keeps customers, cars, offers, repairs and supplier costs in one place, and shows the owner how much money each job actually made. It is built for, and used day to day by, a working mechanic.

## What it does

An owner can:

- create and update a garage profile;
- manage customers and cars;
- keep a service-history timeline for each car;
- create and edit offers with parts and labour lines;
- accept offers into repairs and track work through completion;
- see completed repairs in the service history and track recognized revenue;
- record what each part cost from the supplier and see the margin it leaves;
- open a print-friendly offer or download a generated PDF;
- receive a welcome email and reset a forgotten password by email.

## Tech stack

- **Language:** Go
- **Database:** PostgreSQL, with embedded [Goose](https://github.com/pressly/goose) migrations
- **Frontend:** server-rendered HTML templates and CSS, no JavaScript framework
- **Multi-tenancy:** one database, every garage's rows scoped by `tenant_id` in the application's store layer
- **Packaging:** multi-stage Docker build with a distroless runtime image
- **Local tooling:** Docker Compose, Makefile, Mailpit for catching emails in development
- **Hosting:** Google Cloud Run with a Neon PostgreSQL database and Brevo SMTP; the same image also runs under Docker Compose

## Design highlights

- **Tenant isolation in the store layer.** Every tenant-scoped query runs inside `store.WithTenant` and filters on `tenant_id`, and composite foreign keys stop a row from ever referencing another garage's data. The database itself enforces no row-level security, so a query that leaves out its `tenant_id` filter is a security bug.
- **Single self-contained binary.** Templates, CSS and migrations are embedded, so the image contains just `/pitlane` and nothing else.
- **Migrations ship in the binary.** The executable applies them itself (`pitlane migrate up`), against a separate `MIGRATE_DSN` when the database needs a direct, non-pooled endpoint.
- **Safe demo data.** A built-in demo mode creates a separate, flagged garage with a year of realistic data, and its reset/drop commands refuse to touch any real garage.
- **Explicit dependencies.** Handlers are methods on a small `application` struct, with hand-written middleware and no hidden globals.

## Run locally

Requirements: Go, Docker and Docker Compose.

```
make dev
```

This creates `.env`, starts PostgreSQL and Mailpit, applies the embedded migrations and runs the web app at <http://localhost:4000>.

On first use, choose **Create your garage**. Welcome and password-reset emails appear in Mailpit at <http://localhost:8025>.

### Useful commands

```
make test             # run Go tests
make audit            # format, vet and test
make migrate-status   # show migration status
make build            # build the pitlane:latest Docker image
make build-local      # build bin/pitlane
make deps-down        # stop local PostgreSQL and Mailpit
```

The executable also accepts flags:

```
go run ./cmd/web -addr=:4000 -dsn='postgres://...'
go run ./cmd/web migrate up
```

`DSN` is the runtime connection. `MIGRATE_DSN` is optional and defaults to `DSN`; set it when migrations need a different endpoint from the application, for example a direct (non-pooled) endpoint on a serverless Postgres provider. See `.env.example`.

## Project structure

```
cmd/web/       application entry point, routes, handlers and middleware
internal/      domain logic and data access
migrations/    SQL migrations (embedded into the binary)
ui/            HTML templates and static assets (embedded into the binary)
```

## Deployment

The image is a Go build plus a distroless runtime. It contains `/pitlane`, the embedded templates and CSS, and the embedded Goose migrations.

### Google Cloud Run

The hosted instance runs as the Cloud Run service `pitlane` in project `pitlane-233d9z`, region `europe-west1`, at <https://pitlane.fotev.dev>. PostgreSQL is on Neon and email goes through Brevo from `pitlane@mail.fotev.dev`.

- Configuration lives on the service. `ENV`, `APP_BASE_URL` and the `SMTP_*` settings are plain environment variables. `DSN` (Neon's pooled endpoint) and `SMTP_PASSWORD` come from Secret Manager (`pitlane-dsn`, `pitlane-smtp-password`).
- The service runs as `pitlane-run`, which can only read those secrets.
- To cap cost, the service scales to zero and is limited to one instance, 20 concurrent requests and a 30-second request timeout.

Apply new migrations first, from your machine, against Neon's **direct** (non-pooled) endpoint:

```
MIGRATE_DSN='postgres://...direct...?sslmode=require' go run ./cmd/web migrate up
```

Then deploy:

```
make deploy             # build with Cloud Build, deploy the service, then repoint the demo job
make deploy-job         # only repoint the demo job at the image the service is running
```

`make deploy` sends only the source code; the service keeps its settings between deploys. Change those with `gcloud run services update`. Check that the output ends with the demo job being updated; if it doesn't, run `make deploy-job`.

### Docker Compose

```
make build
docker compose -f docker-compose.prod.yml up -d db
docker compose -f docker-compose.prod.yml run --rm app migrate up
docker compose -f docker-compose.prod.yml up -d app
```

### Demonstration data

`pitlane demo` builds a separate garage with a year of finished work behind it, quotes in every state and a couple of jobs left standing still, so the product can be shown to a prospect on a real deployment. Everything is relative to the day it runs, so the dashboard always shows a full twelve months.

```
make demo-seed
docker compose -f docker-compose.prod.yml run --rm app demo reset
```

`seed` creates it, `reset` rebuilds it from scratch (and creates it if it is missing), and `drop` removes it. Run `reset` before a demonstration to undo whatever the last one clicked on.

The login is `demo@pitlane.bg`. The password comes from `DEMO_PASSWORD` and falls back to `pitlane-demo`; override any of them with `-email`, `-password` and `-garage`. Only the default password is printed after seeding, so a chosen one never ends up in logs.

On Cloud Run, the demo is rebuilt by the Cloud Run job `pitlane-demo-reset`, which runs `pitlane demo reset` with `DEMO_PASSWORD` from the `pitlane-demo-password` secret. Cloud Scheduler (`pitlane-demo-reset-weekly`) starts it every Monday at 04:00 Europe/Sofia. To rebuild it before a demonstration:

```
make demo-reset-cloud
```

The demonstration garage is a normal tenant, scoped by the same `tenant_id` filters as every other, and it is flagged `is_demo`. That flag is what lets `reset` and `drop` delete data at all: pointed at a real garage they refuse and change nothing.

## Architecture

The implementation follows the Snippetbox style from Alex Edwards: handlers are methods on a small `application` struct, dependencies are explicit, templates are rendered server-side, forms retain validation errors, middleware is hand-written, and successful writes use post/redirect/get.

## Status and roadmap

Pitlane is in daily use by a single garage. It is hosted on Google Cloud Run at <https://pitlane.fotev.dev>, with open signup and a demonstration garage rebuilt every week.

Planned:

- an automatic billing cap: the Google Cloud budget unlinks billing from the project if spend passes $1 a month.
