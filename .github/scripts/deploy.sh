#!/usr/bin/env bash
set -euo pipefail

: "${VPS_HOST:?Set VPS_HOST}"
: "${VPS_USER:?Set VPS_USER}"
: "${VPS_DEPLOY_PATH:?Set VPS_DEPLOY_PATH}"
: "${VPS_SSH_KEY:?Set VPS_SSH_KEY}"
: "${VPS_KNOWN_HOSTS:?Set VPS_KNOWN_HOSTS}"
: "${API_IMAGE:?Set API_IMAGE}"
: "${GITHUB_SHA:?Set GITHUB_SHA}"
VPS_PORT=${VPS_PORT:-22}

# These values also become part of a remote shell command.
[[ "$VPS_HOST" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]]
[[ "$VPS_USER" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]]
[[ "$VPS_PORT" =~ ^[0-9]+$ ]]
[[ "$VPS_DEPLOY_PATH" =~ ^/[a-zA-Z0-9_./-]+$ && "$VPS_DEPLOY_PATH" != / ]]
[[ "$GITHUB_SHA" =~ ^[a-f0-9]{40}$ ]]
[[ "$API_IMAGE" =~ ^ghcr.io/[a-z0-9_./-]+@sha256:[a-f0-9]{64}$ ]]

ssh_dir=$(mktemp -d)
trap 'rm -rf "$ssh_dir"' EXIT
chmod 700 "$ssh_dir"
printf '%s\n' "$VPS_SSH_KEY" > "$ssh_dir/key"
printf '%s\n' "$VPS_KNOWN_HOSTS" > "$ssh_dir/known_hosts"
chmod 600 "$ssh_dir/key" "$ssh_dir/known_hosts"
ssh_options=(-i "$ssh_dir/key" -p "$VPS_PORT" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$ssh_dir/known_hosts" -o ConnectTimeout=15)
destination="$VPS_USER@$VPS_HOST"
release_dir="$VPS_DEPLOY_PATH/releases/$GITHUB_SHA"

tar -czf - compose.production.yaml deploy/run.sh |
  ssh "${ssh_options[@]}" "$destination" "mkdir -p '$release_dir' && tar -xzf - -C '$release_dir'"
ssh "${ssh_options[@]}" "$destination" \
  "cd '$release_dir' && bash deploy/run.sh '$VPS_DEPLOY_PATH' '$API_IMAGE'"
