#!/usr/bin/env bash
set -euo pipefail

workspace_path="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$workspace_path/backend"
command -v go >/dev/null || { echo 'Install Go before running migrations.' >&2; exit 1; }

# Read the secret locally, without putting it in shell history or command arguments.
if [[ -z "${GOOSE_DBSTRING:-}" ]]; then
  IFS= read -r -s -p 'Neon direct PostgreSQL URL (with TLS): ' GOOSE_DBSTRING
  printf '\n'
fi
if [[ "$GOOSE_DBSTRING" != postgres://* && "$GOOSE_DBSTRING" != postgresql://* ]]; then
  echo 'Use the direct PostgreSQL connection URL from your Neon project.' >&2
  exit 1
fi
tls_pattern='[?&]sslmode=(require|verify-ca|verify-full)(&|$)'
if [[ ! "$GOOSE_DBSTRING" =~ $tls_pattern ]]; then
  echo 'The connection URL must retain sslmode=require (or verify-ca/verify-full).' >&2
  exit 1
fi
export GOOSE_DRIVER=postgres GOOSE_DBSTRING
trap 'unset GOOSE_DBSTRING' EXIT

go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -env none -dir db/migrations status
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -env none -dir db/migrations up
go run github.com/pressly/goose/v3/cmd/goose@v3.24.3 -env none -dir db/migrations version
