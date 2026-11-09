#!/bin/sh
# Reset the public demo instance: drop the demo database, recreate and migrate
# it, register the demo login and load the sample household (POST /demo/load).
# Visitors may write anything during the day; this wipes it all.
#
# DESTRUCTIVE. It refuses to run unless DEMO_RESET_CONFIRM=drop-database, which
# only docker-compose.demo.yml sets. Never point it at a real instance.
#
# Required environment (docker-compose.demo.yml provides all of it):
#   DB_HOST DB_USER DB_PASSWORD DB_NAME   Postgres owner/superuser (DB_PORT default 5432)
#   API_URL                               e.g. http://api:8080
#   DEMO_EMAIL DEMO_PASSWORD              the public demo login
# Optional: DEMO_FIRST_NAME (Demo), DEMO_BASE_CURRENCY (MYR), WAIT_SECONDS (120),
# MIGRATE_BIN (/migrate; run from the directory that holds migrations/).
set -eu

log() { printf '%s demo-reset: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
die() { log "ERROR: $*"; exit 1; }

[ "${DEMO_RESET_CONFIRM:-}" = "drop-database" ] \
    || die "DEMO_RESET_CONFIRM=drop-database is not set; refusing to drop a database"
for v in DB_HOST DB_USER DB_PASSWORD DB_NAME API_URL DEMO_EMAIL DEMO_PASSWORD; do
    eval "val=\${$v:-}"
    [ -n "$val" ] || die "$v is required"
done
case "$DB_NAME" in
    *[!A-Za-z0-9_]*) die "DB_NAME must be letters, digits and underscores only" ;;
esac

export PGPASSWORD="$DB_PASSWORD"
DB_PORT="${DB_PORT:-5432}"
WAIT_SECONDS="${WAIT_SECONDS:-120}"
export PGCONNECT_TIMEOUT=5
psql_admin() { psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 -qtA "$@"; }

# retry <seconds> <description> <command...>: run until it succeeds or time runs out.
retry() {
    limit="$1"; what="$2"; shift 2
    waited=0
    until "$@" >/dev/null 2>&1; do
        waited=$((waited + 2))
        [ "$waited" -le "$limit" ] || die "timed out waiting for $what"
        sleep 2
    done
}

log "waiting for postgres"
retry "$WAIT_SECONDS" postgres psql_admin -c 'SELECT 1'

log "dropping and recreating database ${DB_NAME}"
# FORCE disconnects the api's pooled connections; database/sql reconnects.
psql_admin -c "DROP DATABASE IF EXISTS ${DB_NAME} WITH (FORCE)"
psql_admin -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER}"

log "running migrations"
"${MIGRATE_BIN:-/migrate}" up

# On the very first boot the api came up against an empty, unmigrated database,
# so wait only for its health endpoint, not for any data.
log "waiting for the api"
retry "$WAIT_SECONDS" "the api" curl -fsS "${API_URL}/api/health"

register_body=$(jq -n \
    --arg email "$DEMO_EMAIL" --arg password "$DEMO_PASSWORD" \
    --arg first "${DEMO_FIRST_NAME:-Demo}" --arg cur "${DEMO_BASE_CURRENCY:-MYR}" \
    '{email: $email, password: $password, first_name: $first, base_currency: $cur}')
login_body=$(jq -n --arg email "$DEMO_EMAIL" --arg password "$DEMO_PASSWORD" \
    '{email: $email, password: $password}')

log "registering the demo user"
code=$(curl -sS -o /tmp/register.out -w '%{http_code}' -X POST "${API_URL}/api/v1/auth/register" \
    -H 'Content-Type: application/json' -d "$register_body") || die "register request failed"
[ "$code" = "201" ] || [ "$code" = "200" ] || die "register returned HTTP ${code}"

log "signing in"
token=$(curl -fsS -X POST "${API_URL}/api/v1/auth/login" \
    -H 'Content-Type: application/json' -d "$login_body" | jq -er '.access_token') \
    || die "login failed"

log "loading the sample household"
code=$(curl -sS -o /tmp/load.out -w '%{http_code}' -X POST "${API_URL}/api/v1/demo/load" \
    -H "Authorization: Bearer ${token}") || die "demo load request failed"
[ "$code" = "201" ] || die "POST /demo/load returned HTTP ${code} (is DEMO_ENABLED=true on the api?)"

log "done: $(jq -c '{total: .total}' /tmp/load.out 2>/dev/null || echo loaded)"
