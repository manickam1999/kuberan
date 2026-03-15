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
