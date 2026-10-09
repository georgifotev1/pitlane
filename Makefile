-include .env
export

.PHONY: fmt-templates dev deps deps-down test audit build build-local migrate-up migrate-down migrate-status demo-seed demo-reset demo-drop deploy deploy-job demo-reset-cloud

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

audit: fmt-templates
	gofmt -w $$(find cmd internal ui -name '*.go')
	go vet ./...
	go test ./...

# Go templates confuse plain HTML formatters; gotmplfmt parses text/template.
fmt-templates:
	go tool gotmplfmt -w ui/html/*.html internal/mailer/templates/*.html.tmpl

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

# Cloud Run. Service settings (env vars, secrets, limits) persist between
# deploys, so only the source is sent. The demo reset job runs the same binary,
# so deploy repoints it at the image the service just started using.
GCP_PROJECT := pitlane-233d9z
GCP_REGION  := europe-west1
GCLOUD      := gcloud --project $(GCP_PROJECT)

deploy:
	$(GCLOUD) run deploy pitlane --source . --region $(GCP_REGION) --quiet
	$(MAKE) deploy-job

deploy-job:
	$(GCLOUD) run jobs update pitlane-demo-reset --region $(GCP_REGION) --args='demo,reset' \
		--image "$$($(GCLOUD) run services describe pitlane --region $(GCP_REGION) --format='value(spec.template.spec.containers[0].image)')"

# Rebuild the hosted demo now instead of waiting for the Monday schedule.
demo-reset-cloud:
	$(GCLOUD) run jobs execute pitlane-demo-reset --region $(GCP_REGION) --wait
