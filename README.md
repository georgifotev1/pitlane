# Pitlane

A small, server-rendered garage manager built in Go. An owner can:

- create and update a garage profile;
- manage customers and cars;
- keep a service-history timeline for each car;
- create and edit offers with parts/labour lines;
- accept offers into repairs and track work through completion;
- see completed repairs in service history and track recognized revenue;
- record what each part cost from the supplier and see the margin it leaves;
- open a print-friendly offer or download a generated PDF;
- receive a welcome email and reset a forgotten password by email.

There is no React/Node build, JSON API, offer email, job queue or object storage.
Go embeds all HTML templates and CSS into the executable.

## Run locally

Requirements: Go, Docker and Docker Compose.

```sh
make dev
```

This creates `.env`, starts PostgreSQL and Mailpit, applies embedded migrations
and runs the web app at <http://localhost:4000>. On first use, choose **Create
your garage**. Welcome and reset emails appear in Mailpit at
<http://localhost:8025>.

Useful commands:

```sh
make test             # Go tests
make audit            # format, vet and test
make migrate-status
make build             # pitlane:latest Docker image
make build-local       # bin/pitlane
make deps-down
```

The executable also accepts flags:

```sh
go run ./cmd/web -addr=:4000 -dsn='postgres://...'
go run ./cmd/web migrate up
```

`DSN` is the runtime connection. `MIGRATE_DSN` is optional and defaults to
`DSN`; set it when migrations need a different endpoint from the application,
for example a direct (non-pooled) endpoint on a serverless Postgres provider.
See `.env.example`.

## Deployment

The image is a Go build plus a distroless runtime. It contains `/pitlane`, the
embedded templates/CSS, and embedded Goose migrations.

```sh
make build
docker compose -f docker-compose.prod.yml up -d db
docker compose -f docker-compose.prod.yml run --rm app migrate up
docker compose -f docker-compose.prod.yml up -d app
```

Fly.io uses the same image and runs `migrate up` as its release command. Set
`DSN`, `MIGRATE_DSN`, `APP_BASE_URL`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`,
`SMTP_PASSWORD`, and `SMTP_FROM` for production before deploying.

### Demonstration data

`pitlane demo` builds a separate garage with a year of finished work behind it,
quotes in every state and a couple of jobs left standing still, so the product
can be shown to a prospect on a real deployment. Everything is relative to the
day it runs, so the dashboard always shows a full twelve months.

```sh
make demo-seed                       # locally
fly ssh console -C "/pitlane demo reset"
docker compose -f docker-compose.prod.yml run --rm app demo reset
```

`seed` creates it, `reset` rebuilds it from scratch (run this before a
demonstration to undo whatever the last one clicked on), and `drop` removes it.
The default login is `demo@pitlane.bg` / `pitlane-demo`; override with `-email`,
`-password` and `-garage`.

The demonstration garage is a normal tenant, isolated by the same row-level
security as every other, and it is flagged `is_demo`. That flag is what lets
`reset` and `drop` delete data at all: pointed at a real garage they refuse and
change nothing.

## Architecture

The implementation follows the Snippetbox style from Alex Edwards: handlers are
methods on a small `application` struct, dependencies are explicit, templates
are rendered server-side, forms retain validation errors, middleware is
hand-written, and successful writes use post/redirect/get. See [`ADR.md`](ADR.md).
