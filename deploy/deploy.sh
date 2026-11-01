#!/usr/bin/env bash
#
# Production deploy: build images tagged with the git SHA, back up the
# database, run migrations, start everything health-gated, and keep the last
# few image tags for rollback.
#
# Usage: ./deploy/deploy.sh
#
# Environment overrides:
#   SKIP_BACKUP=1        skip the pre-migration backup (NOT recommended)
#   KEEP_IMAGES=3        image tags to keep per service for rollback
#   WAIT_TIMEOUT=300     seconds to wait for services to become healthy
#
# -E: the ERR trap (rollback instructions) also fires inside functions.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
KEEP_IMAGES="${KEEP_IMAGES:-3}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300}"
# Images built by docker-compose.prod.yml (mcp reuses kuberan-api).
IMAGES=(kuberan-api kuberan-web kuberan-bot kuberan-oracle kuberan-backup)
# Run-to-completion services: excluded from the health-gated wait.
ONE_SHOT_SERVICES=" hydra-migrate minio-init oracle "
# Backup service uid (apps/backup/Dockerfile); owns ./backups.
BACKUP_UID=10001

cd "$PROJECT_DIR"

# Verify .env.prod exists
if [ ! -f .env.prod ]; then
    echo "ERROR: .env.prod not found. Copy .env.prod.example to .env.prod and fill in values."
    exit 1
fi

# Preflight: settings the api/mcp/migrate binaries refuse to start without in
# production. Checking them here fails the deploy before anything is built or
# migrated, instead of at the health wait.
env_value() {
    # Last assignment wins, like compose; strips surrounding quotes.
    grep -E "^[[:space:]]*$1=" .env.prod | tail -n 1 | cut -d= -f2- \
        | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"\(.*\)"$/\1/' -e "s/^'\(.*\)'$/\1/"
}
PREFLIGHT_ERRORS=()
JWT_SECRET_VALUE="$(env_value JWT_SECRET)"
if [ "${#JWT_SECRET_VALUE}" -lt 32 ]; then
    PREFLIGHT_ERRORS+=("JWT_SECRET must be at least 32 bytes (generate with: openssl rand -hex 32). Changing it signs everyone out once.")
fi
CORS_ORIGIN_VALUE="$(env_value CORS_ORIGIN)"
if [ -z "$CORS_ORIGIN_VALUE" ] || [[ "$CORS_ORIGIN_VALUE" == *"*"* ]]; then
    PREFLIGHT_ERRORS+=("CORS_ORIGIN must list the exact web origin(s), e.g. https://app.example.com (no '*').")
fi
if [ "${#PREFLIGHT_ERRORS[@]}" -gt 0 ]; then
    echo "ERROR: .env.prod needs updating before this deploy:"
    for e in "${PREFLIGHT_ERRORS[@]}"; do echo "  - $e"; done
    echo "See docs/backup-and-dr.md#upgrade-notes."
    exit 1
fi

# Compose reads .env.prod for ${VAR} interpolation. The .env symlink makes
# ad-hoc commands (cron's `compose run oracle`, docs snippets) see the same
# values without --env-file.
ln -sf .env.prod .env
COMPOSE=(docker compose -f docker-compose.prod.yml --env-file .env.prod)

GIT_SHA="$(git rev-parse --short=12 HEAD)"

# The image tag the running api uses, i.e. what a rollback returns to.
PREVIOUS_TAG=""
API_CONTAINER="$("${COMPOSE[@]}" ps -q api 2>/dev/null | head -n 1 || true)"
if [ -n "$API_CONTAINER" ]; then
    RUNNING_IMAGE="$(docker inspect --format '{{.Config.Image}}' "$API_CONTAINER" 2>/dev/null || true)"
    PREVIOUS_TAG="${RUNNING_IMAGE##*:}"
fi
if [ "$PREVIOUS_TAG" = "$GIT_SHA" ]; then
    PREVIOUS_TAG=""  # redeploying the same commit
elif [ -z "$PREVIOUS_TAG" ] || [ "$PREVIOUS_TAG" = "latest" ]; then
    # Deploys before SHA tagging only had `latest`, which this deploy moves.
    # Pin the current images under a stable tag so they survive as the
    # rollback target instead of becoming dangling and being pruned.
    PREVIOUS_TAG=""
    if docker image inspect kuberan-api:latest >/dev/null 2>&1; then
        PREVIOUS_TAG="pre-${GIT_SHA}"
        for img in "${IMAGES[@]}"; do
            if docker image inspect "${img}:latest" >/dev/null 2>&1; then
                docker tag "${img}:latest" "${img}:${PREVIOUS_TAG}"
            fi
        done
    fi
fi
export IMAGE_TAG="$GIT_SHA"
PREDEPLOY_BACKUP=""

on_failure() {
    local rc=$?
    echo ""
    echo "!!! Deploy of ${GIT_SHA} FAILED (exit ${rc}) during: ${STAGE:-unknown}"
    echo ""
    echo "Diagnose:"
    echo "  ${COMPOSE[*]} ps"
    echo "  ${COMPOSE[*]} logs --tail=200 <service>"
    echo ""
    if [ -n "$PREVIOUS_TAG" ]; then
        echo "Roll back to the previous images (${PREVIOUS_TAG}):"
        echo "  for i in ${IMAGES[*]}; do docker image inspect \$i:${PREVIOUS_TAG} >/dev/null 2>&1 && docker tag \$i:${PREVIOUS_TAG} \$i:latest; done"
        echo "  IMAGE_TAG=${PREVIOUS_TAG} ${COMPOSE[*]} up -d --no-build"
    else
        echo "No previous SHA-tagged images found; roll back by checking out the"
        echo "previous commit (git checkout <sha>) and re-running ./deploy/deploy.sh."
    fi
    if [ -n "$PREDEPLOY_BACKUP" ]; then
        echo ""
        echo "If migrations ran and must be undone, restore the pre-deploy backup"
        echo "(./backups/, label 'predeploy') per docs/backup-and-dr.md, or step"
        echo "back one migration with:"
        echo "  ${COMPOSE[*]} run --rm --no-deps -e DB_PORT=5432 --entrypoint /migrate api down 1"
    fi
    exit "$rc"
}
trap on_failure ERR

service_enabled() {
    "${COMPOSE[@]}" config --services | grep -qx "$1"
}

STAGE="build"
echo "==> Building images (tag ${IMAGE_TAG})..."
"${COMPOSE[@]}" build
# The oracle sits behind a profile and is only ever started by the host cron
# job (`compose run`), so the plain build above skips it. Build it explicitly
# so cron picks up the new image; passing --profile here instead of above
# avoids overriding the COMPOSE_PROFILES set in .env.prod.
"${COMPOSE[@]}" --profile oracle build oracle

STAGE="database start"
if service_enabled postgres; then
    # Start (or recreate, e.g. onto new networks) the database first so the
    # backup and migrations below can reach it.
    echo "==> Ensuring postgres is up..."
    "${COMPOSE[@]}" up -d --wait --wait-timeout "$WAIT_TIMEOUT" postgres
fi

STAGE="pre-migration backup"
if [ "${SKIP_BACKUP:-0}" = "1" ]; then
    echo "==> WARNING: SKIP_BACKUP=1, deploying WITHOUT a pre-migration backup."
elif service_enabled backup; then
    echo "==> Taking a pre-migration backup..."
    # ./backups must be writable by the unprivileged backup user (older
    # deploys wrote it as root). One-shot root container with only CHOWN.
    mkdir -p backups
    "${COMPOSE[@]}" run --rm --no-deps --user 0 --cap-add CHOWN \
        --entrypoint chown backup -R "${BACKUP_UID}:${BACKUP_UID}" /backups
    "${COMPOSE[@]}" run --rm --no-deps \
        -e BACKUP_LABEL=predeploy -e BACKUP_SKIP_RECEIPTS=true \
        --entrypoint /backup.sh backup
    PREDEPLOY_BACKUP="yes"
else
    echo "==> WARNING: the 'backup' profile is not enabled (COMPOSE_PROFILES);"
    echo "    no pre-migration backup was taken. Rely on your provider's backups"
    echo "    (e.g. Supabase PITR) or enable the backup profile."
fi

STAGE="migrations"
echo "==> Running database migrations..."
# Migrations use advisory locks which require a direct connection,
# not the Supavisor transaction-mode pooler (port 6543).
# Override DB_PORT to use Supabase's direct connection (port 5432).
"${COMPOSE[@]}" run --rm --no-deps -e DB_PORT=5432 --entrypoint /migrate api up

STAGE="start services"
echo "==> Starting services (waiting up to ${WAIT_TIMEOUT}s for health)..."
LONG_RUNNING=()
while IFS= read -r svc; do
    case "$ONE_SHOT_SERVICES" in
        *" $svc "*) ;;
        *) LONG_RUNNING+=("$svc") ;;
    esac
done < <("${COMPOSE[@]}" config --services)
"${COMPOSE[@]}" up -d --wait --wait-timeout "$WAIT_TIMEOUT" "${LONG_RUNNING[@]}"

STAGE="tag images"
# `latest` follows the running deploy so cron (`compose run oracle`) and
# ad-hoc commands without IMAGE_TAG use the same images.
for img in "${IMAGES[@]}"; do
    if docker image inspect "${img}:${GIT_SHA}" >/dev/null 2>&1; then
        docker tag "${img}:${GIT_SHA}" "${img}:latest"
    fi
done

trap - ERR

echo "==> Pruning old images (keeping the last ${KEEP_IMAGES} tags per service)..."
for img in "${IMAGES[@]}"; do
    # Newest first; never touch `latest` or the tag just deployed.
    docker image ls "$img" --format '{{.CreatedAt}}\t{{.Tag}}' \
        | sort -r | cut -f2 \
        | grep -v -e '^latest$' -e '^<none>$' \
        | awk -v keep="$KEEP_IMAGES" 'NR > keep' \
        | while IFS= read -r tag; do
            [ "$tag" = "$GIT_SHA" ] && continue
            docker image rm "${img}:${tag}" >/dev/null 2>&1 || true
        done || true
done
# Dangling layers only (untagged); SHA-tagged rollback images are kept.
docker image prune -f >/dev/null

echo "==> Done. Deployed ${GIT_SHA}. Services:"
"${COMPOSE[@]}" ps

echo ""
if [ -n "$PREVIOUS_TAG" ]; then
    echo "Rollback: IMAGE_TAG=${PREVIOUS_TAG} ${COMPOSE[*]} up -d --no-build"
    echo "          (then retag latest; see docs/backup-and-dr.md#rollback)"
fi
echo "Site should be live at ${NEXT_PUBLIC_API_URL:-https://localhost}"
