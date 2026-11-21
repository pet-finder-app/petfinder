SHELL := /bin/sh

DATABASE_URL ?= postgres://petfinder:petfinder@localhost:5432/petfinder?sslmode=disable
MIGRATIONS_DIR ?= db/migrations
E2E_DATABASE ?= petfinder_e2e
E2E_DATABASE_URL ?= postgres://petfinder:petfinder@localhost:5432/$(E2E_DATABASE)?sslmode=disable

.DEFAULT_GOAL := help

.PHONY: help run build test test-race test-e2e lint generate fmt check migrate-up migrate-down migrate-create docker-up docker-down docker-logs docs-generate docs-check docs-validate

help: ## List the available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run the API locally
	go run ./cmd/api

build: ## Build the API into bin/petfinder
	mkdir -p bin
	go build -trimpath -o bin/petfinder ./cmd/api

test: ## Run the tests
	go test ./...

test-race: ## Run the tests with the race detector
	go test -race -coverprofile=coverage.out ./...

test-e2e: ## Run the end-to-end tests on a freshly recreated database
	docker compose up -d --wait db
	docker compose exec -T db sh -c 'dropdb -U "$$POSTGRES_USER" --if-exists --force $(E2E_DATABASE) && createdb -U "$$POSTGRES_USER" $(E2E_DATABASE)'
	$(MAKE) migrate-up DATABASE_URL="$(E2E_DATABASE_URL)"
	E2E_DATABASE_URL="$(E2E_DATABASE_URL)" go test -tags e2e -count=1 ./e2e/...

lint: ## Run the locally installed golangci-lint
	golangci-lint run ./...

generate: docs-generate ## Generate the SQLC code and OpenAPI
	sqlc generate

fmt: ## Format the Go code
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

check: generate test-race lint docs-validate ## Run every local check

migrate-up: ## Apply all pending migrations
	docker run --rm --network host -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations:ro" migrate/migrate:v4.18.3 -path=/migrations -database="$(DATABASE_URL)" up

migrate-down: ## Revert the last migration
	docker run --rm --network host -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations:ro" migrate/migrate:v4.18.3 -path=/migrations -database="$(DATABASE_URL)" down 1

migrate-create: ## Create a migration: make migrate-create NAME=name
	@test -n "$(NAME)" || (echo "informe NAME=nome_da_migracao" && exit 1)
	docker run --rm --user "$$(id -u):$$(id -g)" -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" migrate/migrate:v4.18.3 create -ext sql -dir /migrations -seq "$(NAME)"

docker-up: ## Start the database, migrations and API
	docker compose up --build -d

docker-down: ## Stop the services without deleting data
	docker compose down

docker-logs: ## Follow the API logs
	docker compose logs -f api

docs-generate: ## Generate build/openapi.yaml from the Go routes and types
	go run ./cmd/openapi

docs-check: ## Check that the generated OpenAPI artifact is up to date
	go run ./cmd/openapi -check

docs-validate: docs-generate ## Validate the generated OpenAPI contract in a container
	docker run --rm -v "$(CURDIR):/spec:ro" redocly/cli:1.34.3 lint /spec/build/openapi.yaml
