-include .env
export

.PHONY: dev deps deps-down dev-api dev-frontend test types e2e build \
	migrate-up migrate-down migrate-status audit

.env:
	cp .env.example .env
	@echo "Created .env from .env.example — review before production use."

deps:
	docker compose up -d --wait postgres mailpit minio
	docker compose run --rm minio-init

deps-down:
	docker compose down

dev: .env deps
	$(MAKE) -j2 dev-api dev-frontend

dev-api:
	cd api && go run ./cmd/api

dev-frontend:
	cd frontend && pnpm dev

test:
	cd api && go test ./...

types:
	cd api && go tool tygo generate

e2e:
	@echo "e2e: Playwright journeys arrive in Phase 2" && exit 1

build:
	@echo "build: production Dockerfile arrives in Phase 1" && exit 1

migrate-up migrate-down migrate-status:
	@echo "$@: goose migrations arrive in Phase 1" && exit 1

audit:
	cd api && go vet ./...
	cd frontend && pnpm exec tsc -b
	cd frontend && pnpm exec oxlint src
	cd frontend && pnpm audit
	$(MAKE) types
	git diff --exit-code frontend/src/lib/generated
