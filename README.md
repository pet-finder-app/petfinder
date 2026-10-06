# PetFinder API

API em Go para descoberta e adoção segura de animais. Toda adoção é mediada por uma organização verificada: o adotante e o responsável pelo animal conversam com a organização em canais separados, sem contato privado entre si.

## Tecnologias

- Go 1.25 e `net/http`;
- PostgreSQL e `pgx`;
- SQLC para acesso tipado ao banco;
- golang-migrate para versionamento do schema;
- Docker Compose para o ambiente local;
- OpenAPI 3.1 gerado com Huma para o contrato HTTP.

## Início rápido com Docker

Requer Docker com o plugin Compose.

```sh
cp .env.example .env
make docker-up
curl http://localhost:8080/health
```

O Compose inicia o PostgreSQL, aplica `db/migrations` e só então inicia a API. O endpoint de saúde deve responder:

```json
{"status":"ok"}
```

Use `make docker-logs` para acompanhar a API e `make docker-down` para encerrar os containers sem apagar o volume do banco. O segredo JWT do exemplo serve apenas para desenvolvimento; troque-o em qualquer ambiente compartilhado.

## Desenvolvimento local

Para executar a API fora do container, instale Go 1.25, SQLC e golangci-lint. Mantenha apenas o banco no Docker:

```sh
cp .env.example .env
docker compose up -d db
make migrate-up
set -a; . ./.env; set +a
make run
```

Comandos principais:

| Comando | Finalidade |
|---|---|
| `make test` | Executa os testes |
| `make test-race` | Executa testes, detector de corrida e cobertura |
| `make generate` | Regenera SQLC e o contrato OpenAPI |
| `make lint` | Executa o golangci-lint |
| `make check` | Executa geração, testes, lint e validação do OpenAPI |
| `make build` | Gera `bin/petfinder` |
| `make migrate-up` | Aplica migrações pendentes |
| `make migrate-down` | Reverte a última migração |
| `make docs-generate` | Gera `build/openapi.yaml` sem precisar de banco ou configuração |
| `make docs-check` | Verifica se o YAML gerado está atualizado |
| `make docs-validate` | Gera e valida o OpenAPI com Redocly |

## Configuração

A aplicação lê configuração exclusivamente do ambiente. Consulte [.env.example](.env.example) para um arquivo local completo.

| Variável | Obrigatória | Padrão | Descrição |
|---|---:|---|---|
| `DATABASE_URL` | sim | — | URL PostgreSQL |
| `JWT_SECRET` | sim | — | Segredo com no mínimo 32 bytes |
| `HTTP_ADDR` | não | `:8080` | Endereço de escuta HTTP |
| `APP_ENV` | não | `development` | Nome do ambiente |
| `JWT_ISSUER` | não | `petfinder-api` | Emissor dos access tokens |
| `JWT_AUDIENCE` | não | `petfinder` | Audiência dos access tokens |
| `ACCESS_TOKEN_TTL` | não | `15m` | Duração do access token |
| `REFRESH_TOKEN_TTL` | não | `720h` | Duração máxima da sessão renovável |
| `SHUTDOWN_TIMEOUT` | não | `10s` | Prazo para encerramento gracioso |

Em produção, use TLS no proxy de entrada, um segredo aleatório vindo de um cofre, PostgreSQL com SSL e credenciais distintas das usadas localmente.

## Contrato HTTP

O contrato OpenAPI 3.1 é gerado pelo Huma a partir do mesmo registro de rotas usado pelo servidor e dos tipos Go de requisição e resposta. Não há mais um YAML mantido manualmente. Os tags de validação dos corpos JSON também são aplicados nas requisições; regras de negócio e autorização continuam nos handlers.

Com a API em execução, consulte `/openapi.yaml` ou `/openapi.json`. Para exportar sem iniciar o servidor ou conectar ao PostgreSQL:

```sh
make docs-generate
make docs-check
# destino opcional, inclusive stdout com -output -
go run ./cmd/openapi -output build/petfinder.yaml
```

O arquivo `build/openapi.yaml` é um artefato ignorado pelo Git. Para modificar o contrato, altere `internal/api/routes.go` e os tipos em `internal/api/contracts.go`, depois regenere. Os modelos de resposta do banco vêm do código SQLC.

Rotas de coleção aceitam `page` (a partir de 1) e `page_size` (1 a 100). Erros usam um objeto de problema com `type`, `title`, `status`, `detail` e, em falhas de validação, `errors`. O cabeçalho `X-Request-ID` permite correlacionar requisições e logs.

Após autenticar, envie o access token:

```http
Authorization: Bearer <access_token>
```

Access tokens duram 15 minutos. O refresh token é rotacionado a cada renovação; senhas, refresh tokens e OTPs são persistidos somente como hashes.

## Banco de dados e SQLC

Migrações ficam em `db/migrations`, consultas em `db/queries` e o código gerado em `internal/database`. Para alterar o banco:

```sh
make migrate-create NAME=nome_da_mudanca
# edite os arquivos .up.sql e .down.sql criados
make migrate-up
make generate
make test
```

Não edite manualmente `internal/database`: altere o SQL e regenere o pacote.

## Estrutura

```text
cmd/api/             composição e inicialização da API
db/migrations/       evolução do schema PostgreSQL
db/queries/          consultas processadas pelo SQLC
cmd/openapi/         exportação do contrato OpenAPI gerado
build/openapi.yaml   artefato gerado, não editável manualmente
internal/api/        handlers, validação e middleware HTTP
internal/auth/       senhas, tokens e autenticação
internal/config/     carregamento da configuração
internal/database/   código gerado pelo SQLC
internal/domain/     tipos e regras do domínio
```

Antes de enviar mudanças, execute `make check`. Testes que dependem do PostgreSQL devem usar um banco isolado e nunca o volume local com dados importantes.
