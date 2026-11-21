# PetFinder API

Go API for safe pet discovery and adoption. Every adoption is mediated by a verified NGO: the adopter and the animal's current owner each talk to the NGO, never privately to each other.

Go · PostgreSQL · SQLC · OpenAPI 3.1 (Huma) · Docker Compose

## Getting started

Requires Docker with the Compose plugin.

```sh
cp .env.example .env
make docker-up
curl http://localhost:8080/health   # {"status":"ok"}
```

Compose starts PostgreSQL, applies the migrations and then starts the API. While it runs, the contract is served at `/openapi.yaml`.

Before pushing, run `make check` (generation, tests, lint and OpenAPI validation) and `make test-e2e`.

## Documentation

| Topic | Document |
| --- | --- |
| Running locally, commands and tests | [docs/development.md](docs/development.md) |
| Environment variables and e-mail | [docs/configuration.md](docs/configuration.md) |
| HTTP contract, pagination, errors and tokens | [docs/http-contract.md](docs/http-contract.md) |
| Permissions and verification rules | [docs/authorization.md](docs/authorization.md) |
| Real-time chat (WebSocket) | [docs/realtime-chat.md](docs/realtime-chat.md) |
| Migrations, SQLC and media storage | [docs/database.md](docs/database.md) |
| Code layout | [docs/architecture.md](docs/architecture.md) |
| VPS deployment | [docs/deployment.md](docs/deployment.md) |
| Feature catalogue, route by route | [docs/features.md](docs/features.md) |
