# syntax=docker/dockerfile:1

# Single-artifact build (ADR decision 33): the Go binary embeds frontend/dist
# and serves API + SPA same-origin. Migrations are NOT run on boot — operators
# run `docker run … api migrate up` explicitly (ADR decision 7).

# --- Stage 1: frontend build -------------------------------------------------
FROM node:24-alpine AS frontend
WORKDIR /build
# pnpm via corepack, version pinned by the packageManager field in package.json.
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# --- Stage 2: Go build -------------------------------------------------------
FROM golang:1.26-alpine AS api
WORKDIR /build
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
# Overlay the real frontend build onto the embed placeholder path.
COPY --from=frontend /build/dist ./internal/api/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- Stage 3: runtime --------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/api /api
EXPOSE 4000
ENTRYPOINT ["/api"]
