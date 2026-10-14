#!/bin/sh
# Runs vetod for local development. Settings come from server/.env (copy
# .env.example). Under portless, PORT is assigned and vetod listens on it.
set -eu
cd "$(dirname "$0")/.."
if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi
exec go run ./cmd/vetod serve
