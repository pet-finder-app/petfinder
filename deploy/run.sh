#!/usr/bin/env bash
set -euo pipefail

deployment_root=${1:?Deployment directory is required}
export API_IMAGE=${2:?Image digest is required}
test -f "$deployment_root/.env"

# The project name keeps the database volume stable across release directories.
compose=(docker compose --project-name petfinder-production --env-file "$deployment_root/.env" -f compose.production.yaml)
"${compose[@]}" config --quiet
"${compose[@]}" pull
"${compose[@]}" up -d --wait --wait-timeout 180 db
# Always run migrations, including when retrying a previously failed release.
"${compose[@]}" run --rm --no-deps migrate
if ! "${compose[@]}" up -d --no-deps --wait --wait-timeout 180 api; then
  "${compose[@]}" logs --no-color --tail=100 api
  exit 1
fi
printf '%s\n' "$API_IMAGE" > "$deployment_root/current-image"
