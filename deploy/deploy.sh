#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
COMPOSE="docker compose -f docker-compose.prod.yml"

cd "$PROJECT_DIR"

# Verify .env.prod exists
if [ ! -f .env.prod ]; then
    echo "ERROR: .env.prod not found. Copy .env.prod.example to .env.prod and fill in values."
    exit 1
fi

# Symlink .env.prod to .env so Docker Compose reads it automatically
# for variable interpolation (${CF_TUNNEL_TOKEN}, ${NEXT_PUBLIC_API_URL}, etc.)
ln -sf .env.prod .env

# Ensure backups directory exists (used by the backup service)
mkdir -p backups

echo "==> Building images..."
$COMPOSE build
# The oracle sits behind a profile and is only ever started by the host cron
# job (`compose run`), so the plain build above skips it. Build it explicitly
# so cron picks up the new image; passing --profile here instead of above
# avoids overriding the COMPOSE_PROFILES set in .env.prod.
$COMPOSE --profile oracle build oracle

echo "==> Running database migrations..."
# Migrations use advisory locks which require a direct connection,
# not the Supavisor transaction-mode pooler (port 6543).
# Override DB_PORT to use Supabase's direct connection (port 5432).
$COMPOSE run --rm -e DB_PORT=5432 --entrypoint /migrate api up

echo "==> Starting services..."
$COMPOSE up -d

echo "==> Cleaning up old images..."
docker image prune -f

echo "==> Done. Services:"
$COMPOSE ps

echo ""
echo "Site should be live at ${NEXT_PUBLIC_API_URL:-https://localhost}"
