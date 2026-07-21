-include .env
export

.PHONY: dev deps deps-down dev-api dev-frontend test types build \
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

dev-api: migrate-up
	cd api && go run ./cmd/api

dev-frontend:
	cd frontend && pnpm dev

test:
	cd api && go test ./...

types:
	cd api && go tool tygo generate

build:
	docker build -t pitlane:latest .

migrate-up:
	cd api && go run ./cmd/api migrate up

migrate-down:
	cd api && go run ./cmd/api migrate down

migrate-status:
	cd api && go run ./cmd/api migrate status

audit:
	cd api && go vet ./...
	cd frontend && pnpm exec tsc -b
	cd frontend && pnpm exec oxlint src
	cd frontend && pnpm audit
	$(MAKE) types
	git diff --exit-code frontend/src/lib/generated
	cd frontend && pnpm exec lingui extract
	cd frontend && pnpm exec lingui compile
	@bash -c '! grep -q "^msgstr \"\"$$" frontend/src/locales/*.po || (echo "ERROR: empty translations in PO catalog" && exit 1)'
	git diff --exit-code frontend/src/locales
