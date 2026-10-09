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
- **Multi-tenancy:** one database, isolated per garage with PostgreSQL row-level security
- **Packaging:** multi-stage Docker build with a distroless runtime image
- **Local tooling:** Docker Compose, Makefile, Mailpit for catching emails in development
- **Deployment targets:** Docker Compose, or Fly.io using the same image

## Design highlights

- **Tenant isolation in the database, not just the app.** Every garage is a tenant protected by row-level security, so a bug in a query can't leak another garage's data.
- **Single self-contained binary.** Templates, CSS and migrations are embedded, so the image contains just `/pitlane` and nothing else.
- **Migrations are part of the deploy.** The executable runs them itself (`pitlane migrate up`), and the Fly.io release command does the same before a new version starts.
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

With Docker Compose:

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

`seed` creates it, `reset` rebuilds it from scratch (run this before a demonstration to undo whatever the last one clicked on), and `drop` removes it.

The default login is `demo@pitlane.bg` / `pitlane-demo`; override with `-email`, `-password` and `-garage`.

The demonstration garage is a normal tenant, isolated by the same row-level security as every other, and it is flagged `is_demo`. That flag is what lets `reset` and `drop` delete data at all: pointed at a real garage they refuse and change nothing.

## Architecture

The implementation follows the Snippetbox style from Alex Edwards: handlers are methods on a small `application` struct, dependencies are explicit, templates are rendered server-side, forms retain validation errors, middleware is hand-written, and successful writes use post/redirect/get. See [`ADR.md`](ADR.md) for the decisions behind it.

## Status and roadmap

Pitlane is in daily use by a single garage, running on the owner's own machine. It is not currently hosted as a public service. 
