SHELL := /bin/sh

DATABASE_URL ?= postgres://petfinder:petfinder@localhost:5432/petfinder?sslmode=disable
MIGRATIONS_DIR ?= db/migrations

.DEFAULT_GOAL := help

.PHONY: help run build test test-race lint generate fmt check migrate-up migrate-down migrate-create docker-up docker-down docker-logs docs-validate

help: ## Lista os comandos disponíveis
	@awk 'BEGIN {FS = ":.*## "; printf "Uso: make <alvo>\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Executa a API localmente
	go run ./cmd/api

build: ## Compila a API em bin/petfinder
	mkdir -p bin
	go build -trimpath -o bin/petfinder ./cmd/api

test: ## Executa os testes
	go test ./...

test-race: ## Executa os testes com detector de corrida
	go test -race -coverprofile=coverage.out ./...

lint: ## Executa o golangci-lint instalado localmente
	golangci-lint run ./...

generate: ## Gera o código SQLC
	sqlc generate

fmt: ## Formata o código Go
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

check: generate test-race lint docs-validate ## Executa todas as verificações locais

migrate-up: ## Aplica todas as migrações pendentes
	docker run --rm --network host -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations:ro" migrate/migrate:v4.18.3 -path=/migrations -database="$(DATABASE_URL)" up

migrate-down: ## Reverte a última migração
	docker run --rm --network host -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations:ro" migrate/migrate:v4.18.3 -path=/migrations -database="$(DATABASE_URL)" down 1

migrate-create: ## Cria uma migração: make migrate-create NAME=nome
	@test -n "$(NAME)" || (echo "informe NAME=nome_da_migracao" && exit 1)
	docker run --rm --user "$$(id -u):$$(id -g)" -v "$(CURDIR)/$(MIGRATIONS_DIR):/migrations" migrate/migrate:v4.18.3 create -ext sql -dir /migrations -seq "$(NAME)"

docker-up: ## Sobe banco, migrações e API
	docker compose up --build -d

docker-down: ## Encerra os serviços sem apagar os dados
	docker compose down

docker-logs: ## Acompanha os logs da API
	docker compose logs -f api

docs-validate: ## Valida o contrato OpenAPI em um container
	docker run --rm -v "$(CURDIR):/spec:ro" redocly/cli:1.34.3 lint /spec/docs/openapi.yaml
