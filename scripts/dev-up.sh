#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../deploy/compose"
[ -f .env ] || cp .env.example .env
docker compose --env-file .env up -d --build
