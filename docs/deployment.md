# VPS deployment

The workflow runs unit tests, lint (including the `e2e` build tag), HTTP/WebSocket e2e tests and OpenAPI validation. Only a `push` to `main` that passes all of them publishes the image to GHCR and deploys it. Pull requests and other branches only run the checks.

The image contains the HTTP API, the WebSocket server, the migration binary and the schema files. PostgreSQL runs as another container in the same Compose project, with a persistent volume and no port published on the host. The API keeps a single replica because the chat hub lives in memory. Images are published for amd64 and arm64 Linux VPSs.

## GitHub configuration

Create the `production` environment and configure these secrets and variables in it (or in the repository):

| Kind | Name | Value |
|---|---|---|
| Secret | `VPS_HOST` | VPS hostname or IPv4 address |
| Secret | `VPS_USER` | SSH user with Docker access |
| Secret | `VPS_SSH_KEY` | Private SSH key without a passphrase, dedicated to deployment |
| Variable | `VPS_DEPLOY_PATH` | Absolute directory without spaces, for example `/opt/petfinder` |
| Variable | `VPS_PORT` | SSH port, optional; defaults to `22` |

The publish job uses `GITHUB_TOKEN` with `packages: write`; no PAT is needed to publish. If the GHCR package already exists, give this repository write access in the package settings.

The deploy job uses the local action [`.github/actions/deploy`](../.github/actions/deploy/action.yml). `VPS_HOST`, `VPS_USER`, `VPS_PORT` and `VPS_DEPLOY_PATH` may be secrets or variables; `VPS_SSH_KEY` must be a secret. For host and user, secrets take precedence; for port and directory, variables take precedence.

If the action reports missing values, check that the names are exact and were configured under **Settings > Secrets and variables > Actions** of this repository, or under **Settings > Environments > production**. Secrets from another environment, Dependabot or Codespaces are not available to this job. The server's `.env` configures the application but does not provide credentials or connection parameters to the GitHub runner. The action lists every missing required parameter without printing values.

SSH uses `StrictHostKeyChecking=accept-new`: it automatically accepts the server key on the first connection and rejects changes during the same deployment. The `known_hosts` file is temporary and does not persist between runs, so every deployment trusts the first connection again without verifying the VPS identity beforehand. `VPS_KNOWN_HOSTS` is not needed.

## Preparing the VPS

Install Docker Engine, the Docker Compose v2 plugin (with `up --wait` support), Bash and tar. The SSH user must run `docker` without sudo and be able to write to the chosen directory. Example for `/opt/petfinder`:

```sh
sudo install -d -o "$USER" -g "$(id -gn)" -m 750 /opt/petfinder
# Copy deploy/.env.example from this repository to /opt/petfinder/.env.
chmod 600 /opt/petfinder/.env
```

Edit `.env` before the first deployment: replace `POSTGRES_PASSWORD` and `JWT_SECRET`, and update the matching password in `DATABASE_URL`. The URL points to `db:5432`; special characters in the password must be URL-encoded there. The file stays on the VPS and is never uploaded by the workflow. The PostgreSQL credentials initialize a new volume; changing `.env` later does not change the password of an existing database. Add the SMTP variables from [configuration.md](configuration.md#transactional-e-mail) to enable e-mail.

For private images, log in once on the VPS, as the same user SSH uses, with a PAT from an account that can read the package and has `read:packages` (authorize SSO when required):

```sh
docker login ghcr.io --username YOUR_GITHUB_USER
# Enter the PAT at the Password prompt. Never put it on the command line.
```

Configure an HTTPS proxy on the host pointing to `127.0.0.1:8080` (or `API_PORT`) with WebSocket support: forward the `Upgrade` and `Connection` headers and allow long-lived connections. REST and WebSocket share the same port. The database is only reachable on the private Compose network; the example configuration uses `sslmode=disable` on that local network.

## Running and recovery

Each release copies only `compose.production.yaml` and `deploy/run.sh` to `<VPS_DEPLOY_PATH>/releases/<commit>`. The script pulls the image by its published digest, waits for the database to be healthy, runs the migrations and recreates the API, waiting for `/health`. Migration or health failures fail the job; the running API is only replaced after successful migrations. There is no automatic rollback: migrations must stay compatible with the previous version during the deployment, and restarting the API drops open WebSocket connections.

The fixed Compose project `petfinder-production` keeps the `petfinder-production_postgres-data` volume across releases. It is separate from the development Compose project. To migrate an existing database, restore a backup into that volume before the first deployment. The workflow never runs `down -v` and never removes images or volumes.

`<VPS_DEPLOY_PATH>/current-image` records the last digest that passed the health check. To inspect a deployment, from the release directory:

```sh
export API_IMAGE=$(cat /opt/petfinder/current-image)
docker compose -p petfinder-production --env-file /opt/petfinder/.env -f compose.production.yaml ps
docker compose -p petfinder-production --env-file /opt/petfinder/.env -f compose.production.yaml logs --tail=100 api
```

Re-running the `deploy` job reapplies the release and any pending migrations. To restore an earlier image that is compatible with the current schema, run from the earlier release directory:

```sh
bash deploy/run.sh /opt/petfinder ghcr.io/OWNER/REPOSITORY@sha256:PREVIOUS_DIGEST
```

Replace the path, image name and digest with the real values. Back up PostgreSQL before destructive migrations.
