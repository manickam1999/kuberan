#!/bin/sh
set -eu

BACKUP_DIR="/backups"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
BACKUP_FILE="${BACKUP_DIR}/kuberan_${TIMESTAMP}.dump"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Validate required environment variables
for var in DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME; do
    eval val="\${${var}:-}"
    if [ -z "$val" ]; then
        log "ERROR: ${var} is not set"
        exit 1
    fi
done

log "Starting backup of ${DB_NAME}@${DB_HOST}:${DB_PORT}..."

# Run pg_dump
# --no-owner: skip ownership commands (portable across different users)
# --no-acl: skip access privilege commands
# -Fc: custom format (compressed, supports selective restore)
export PGPASSWORD="${DB_PASSWORD}"
if pg_dump \
    -h "${DB_HOST}" \
    -p "${DB_PORT}" \
    -U "${DB_USER}" \
    -d "${DB_NAME}" \
    --no-owner \
    --no-acl \
    -Fc \
    -f "${BACKUP_FILE}"; then

    FILESIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
    log "Backup completed: ${BACKUP_FILE} (${FILESIZE})"
else
    log "ERROR: pg_dump failed with exit code $?"
    rm -f "${BACKUP_FILE}"
    exit 1
fi

# Prune old backups
PRUNED=$(find "${BACKUP_DIR}" -name "kuberan_*.dump" -type f -mtime "+${RETENTION_DAYS}" | wc -l | tr -d ' ')
if [ "$PRUNED" -gt 0 ]; then
    find "${BACKUP_DIR}" -name "kuberan_*.dump" -type f -mtime "+${RETENTION_DAYS}" -delete
    log "Pruned ${PRUNED} backup(s) older than ${RETENTION_DAYS} days"
fi

REMAINING=$(find "${BACKUP_DIR}" -name "kuberan_*.dump" -type f | wc -l | tr -d ' ')
log "Backup complete. ${REMAINING} backup(s) on disk."

# ---------------------------------------------------------------------------
# Off-host, client-side-encrypted replication to R2 (plans/017 Phase 0).
#
# Guarded on RCLONE_CONFIG_R2CRYPT_REMOTE so local-only / self-hosted setups
# that have not configured R2 keep working with on-disk backups only. The
# rclone remotes ("r2", "r2crypt", "minio") are defined entirely via
# RCLONE_CONFIG_* env vars, so no config file is needed. Because `set -e` is
# active, any failure below aborts the script before the healthcheck ping,
# leaving the dead-man's-switch silent so the monitor flips to "down".
# ---------------------------------------------------------------------------
if [ -n "${RCLONE_CONFIG_R2CRYPT_REMOTE:-}" ]; then
    log "Pushing dump off-host to r2crypt:db/ ..."
    rclone copy "${BACKUP_FILE}" r2crypt:db/ --s3-no-check-bucket
    log "Off-host dump push complete."

    # Mirror the MinIO receipts bucket off-host (no-op until the receipts
    # deploy sets RECEIPTS_BUCKET). rclone sync deletes remote extras, so R2
    # bucket versioning must be enabled to survive a bad sync (see RUNBOOK).
    if [ -n "${RECEIPTS_BUCKET:-}" ]; then
        log "Mirroring receipts bucket ${RECEIPTS_BUCKET} off-host ..."
        rclone sync "minio:${RECEIPTS_BUCKET}" r2crypt:receipts/ --s3-no-check-bucket
        log "Receipts mirror complete."
    fi
else
    log "R2 off-host replication not configured (RCLONE_CONFIG_R2CRYPT_REMOTE unset); skipping."
fi

# Dead-man's-switch: ping the healthcheck only on full success. Any earlier
# failure (set -e) skips this, so a missed ping signals a broken backup run.
if [ -n "${HEALTHCHECK_URL:-}" ]; then
    curl -fsS -m 10 "${HEALTHCHECK_URL}" >/dev/null || log "WARN: healthcheck ping failed"
fi
