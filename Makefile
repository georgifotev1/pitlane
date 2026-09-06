-include .env
export

.PHONY: dev deps deps-down test audit build build-local migrate-up migrate-down migrate-status demo-seed demo-reset demo-drop

.env:
	cp .env.example .env
	@echo "Created .env from .env.example"

deps:
	docker compose up -d --wait postgres mailpit

deps-down:
	docker compose down

dev: .env deps migrate-up
	cd api && go run ./cmd/web

test:
	cd api && go test ./...

audit:
	cd api && gofmt -w $$(find cmd internal ui -name '*.go')
	cd api && go vet ./...
	cd api && go test ./...

build:
	docker build -t pitlane:latest .

build-local:
	cd api && CGO_ENABLED=0 go build -trimpath -o ../bin/pitlane ./cmd/web

migrate-up:
	cd api && go run ./cmd/web migrate up

migrate-down:
	cd api && go run ./cmd/web migrate down

migrate-status:
	cd api && go run ./cmd/web migrate status

# A demonstration garage with a year of history, for showing the product.
demo-seed:
	cd api && go run ./cmd/web demo seed

demo-reset:
	cd api && go run ./cmd/web demo reset

demo-drop:
	cd api && go run ./cmd/web demo drop
