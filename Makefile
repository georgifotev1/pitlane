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
	go run ./cmd/web

test:
	go test ./...

audit:
	gofmt -w $$(find cmd internal ui -name '*.go')
	go vet ./...
	go test ./...

build:
	docker build -t pitlane:latest .

build-local:
	CGO_ENABLED=0 go build -trimpath -o bin/pitlane ./cmd/web

migrate-up:
	go run ./cmd/web migrate up

migrate-down:
	go run ./cmd/web migrate down

migrate-status:
	go run ./cmd/web migrate status

# A demonstration garage with a year of history, for showing the product.
demo-seed:
	go run ./cmd/web demo seed

demo-reset:
	go run ./cmd/web demo reset

demo-drop:
	go run ./cmd/web demo drop
