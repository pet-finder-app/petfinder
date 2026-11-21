# Development

## With Docker

Requires Docker with the Compose plugin.

```sh
cp .env.example .env
make docker-up
curl http://localhost:8080/health
```

Compose starts PostgreSQL, applies `db/migrations` and only then starts the API. The health endpoint should answer:

```json
{ "status": "ok" }
```

Use `make docker-logs` to follow the API and `make docker-down` to stop the containers without deleting the database volume. The example JWT secret is for development only; change it in any shared environment.

## Outside the container

To run the API outside the container, install Go 1.25, SQLC and golangci-lint. Keep only the database in Docker:

```sh
cp .env.example .env
docker compose up -d db
make migrate-up
set -a; . ./.env; set +a
make run
```

Main commands:

| Command              | Purpose                                                          |
| -------------------- | ---------------------------------------------------------------- |
| `make test`          | Run the tests                                                    |
| `make test-race`     | Run tests with the race detector and coverage                    |
| `make test-e2e`      | Run the end-to-end tests against the Compose PostgreSQL          |
| `make generate`      | Regenerate SQLC code and the OpenAPI contract                    |
| `make lint`          | Run golangci-lint                                                |
| `make check`         | Run generation, tests, lint and OpenAPI validation               |
| `make build`         | Build `bin/petfinder`                                            |
| `make migrate-up`    | Apply pending migrations                                         |
| `make migrate-down`  | Revert the last migration                                        |
| `make docs-generate` | Generate `build/openapi.yaml` without a database or configuration |
| `make docs-check`    | Check that the generated YAML is up to date                      |
| `make docs-validate` | Generate and validate the OpenAPI with Redocly                   |

## End-to-end tests

`e2e/` exercises the real API over HTTP and WebSocket: registration and sessions, organization verification, animal publication, applications, screening, chat, OTP-confirmed appointments up to a completed adoption, donations, agreements, follow-ups, surrenders and reports, including denied access. The tests sit behind the `e2e` build tag, so `make test` does not run them.

```sh
make test-e2e
```

The target starts only the Compose `db` service, recreates the `petfinder_e2e` database (the `petfinder` database is never touched), applies the migrations and runs `go test -tags e2e ./e2e/...`. The server runs inside the test process, connected to that database. To use another already-migrated database, set `E2E_DATABASE_URL` and run `go test -tags e2e ./e2e/...`. Every test creates unique users, organizations and cities, so they can run repeatedly against the same database.
