#!/usr/bin/env bash
# Writes the secrets of the DEV stack into untracked files, once. Nothing here
# is ever committed or reused: `task dev:clean` deletes them so they are rolled.
set -euo pipefail
dir="$(cd "$(dirname "$0")" && pwd)/.generated"
mkdir -p "$dir"
if [[ -s "$dir/outline.env" && -s "$dir/dex.env" ]]; then exit 0; fi
umask 077
client_secret="$(openssl rand -hex 24)"
{
  echo "SECRET_KEY=$(openssl rand -hex 32)"
  echo "UTILS_SECRET=$(openssl rand -hex 32)"
  echo "OIDC_CLIENT_SECRET=$client_secret"
} > "$dir/outline.env"
echo "OUTLINE_OIDC_CLIENT_SECRET=$client_secret" > "$dir/dex.env"
