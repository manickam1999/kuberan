#!/bin/sh
# Dump the Kuberan app database (and Hydra's database, when it is a separate
# database on the same server), verify every dump, prune old ones, and push
# them off-host. See docs/backup-and-dr.md.
#
# Optional environment:
#   BACKUP_RETENTION_DAYS   on-disk retention (default 30)
#   BACKUP_LABEL            tag in the file name, e.g. "predeploy"
#   BACKUP_HYDRA_DB_NAME    Hydra database to dump (default "hydra"; empty
#                           disables). Skipped when it does not exist, e.g. on
#                           Supabase where Hydra is a schema in the main DB.
#   BACKUP_SKIP_RECEIPTS    "true" skips the MinIO receipts mirror
#   BACKUP_REQUIRE_OFFHOST  "true" fails the run when R2 is not configured
#   HEALTHCHECK_URL         dead-man's-switch pinged only on full success
set -eu
# Dumps contain every user's financial data: owner-only from creation.
umask 077

# shellcheck source-path=SCRIPTDIR source=lib.sh
. "${BACKUP_LIB:-$(dirname "$0")/lib.sh}"

BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
LABEL="${BACKUP_LABEL:-}"
SUFFIX="${LABEL:+${LABEL}_}${TIMESTAMP}.dump"
HYDRA_DB="${BACKUP_HYDRA_DB_NAME-hydra}"

require_db_env
setup_pgpass

# dump_db <database> <output file>
# Writes to <file>.partial and only renames after pg_restore --list has read
# the whole archive back, so a truncated or corrupt dump never looks valid.
dump_db() {
    db="$1"
    out="$2"
    log "Dumping ${db}@${DB_HOST}:${DB_PORT} -> ${out}"
    # --no-owner/--no-acl: portable across users; -Fc: compressed custom format.
    if ! pg_dump -d "$db" --no-owner --no-acl -Fc -f "${out}.partial"; then
        log "ERROR: pg_dump of ${db} failed"
        rm -f "${out}.partial"
        exit 1
    fi
    if ! pg_restore --list "${out}.partial" > /dev/null; then
        log "ERROR: ${out} failed verification (pg_restore --list)"
        rm -f "${out}.partial"
        exit 1
    fi
    mv "${out}.partial" "$out"
    log "Verified ${out} ($(du -h "$out" | cut -f1))"
}

DUMPS=""

APP_FILE="${BACKUP_DIR}/kuberan_${SUFFIX}"
dump_db "$DB_NAME" "$APP_FILE"
DUMPS="$APP_FILE"

if [ -n "$HYDRA_DB" ] && [ "$HYDRA_DB" != "$DB_NAME" ]; then
    # psql variables (:'name') quote the value, so the name is never spliced
    # into SQL text.
    HYDRA_EXISTS="$(printf "SELECT 1 FROM pg_database WHERE datname = :'name';\n" \
        | psql -d "$DB_NAME" -v ON_ERROR_STOP=1 -v name="$HYDRA_DB" -tA -f -)"
    if [ "$HYDRA_EXISTS" = "1" ]; then
        HYDRA_FILE="${BACKUP_DIR}/hydra_${SUFFIX}"
        dump_db "$HYDRA_DB" "$HYDRA_FILE"
        DUMPS="$DUMPS $HYDRA_FILE"
    else
        log "No separate '${HYDRA_DB}' database on this server; skipping (on Supabase the hydra schema is inside the ${DB_NAME} dump)."
    fi
fi

# Prune old backups (and any stale partial files from crashed runs)
for pattern in 'kuberan_*.dump' 'hydra_*.dump'; do
    PRUNED=$(find "${BACKUP_DIR}" -name "$pattern" -type f -mtime "+${RETENTION_DAYS}" | wc -l | tr -d ' ')
    if [ "$PRUNED" -gt 0 ]; then
        find "${BACKUP_DIR}" -name "$pattern" -type f -mtime "+${RETENTION_DAYS}" -delete
        log "Pruned ${PRUNED} ${pattern} backup(s) older than ${RETENTION_DAYS} days"
    fi
done
find "${BACKUP_DIR}" -name '*.dump.partial' -type f -mtime +1 -delete

REMAINING=$(find "${BACKUP_DIR}" -name "*.dump" -type f | wc -l | tr -d ' ')
log "Backup complete. ${REMAINING} dump(s) on disk."

# ---------------------------------------------------------------------------
# Off-host, client-side-encrypted replication to R2 (plans/017 Phase 0).
#
# Guarded on RCLONE_CONFIG_R2CRYPT_REMOTE so local-only setups keep working
# with on-disk backups only (with a loud warning, or a hard failure when
# BACKUP_REQUIRE_OFFHOST=true). The rclone remotes ("r2", "r2crypt", "minio")
# are defined entirely via RCLONE_CONFIG_* env vars, so no config file is
# needed. Because `set -e` is active, any failure below aborts the script
# before the healthcheck ping, leaving the dead-man's-switch silent so the
# monitor flips to "down".
# ---------------------------------------------------------------------------
if [ -n "${RCLONE_CONFIG_R2CRYPT_REMOTE:-}" ]; then
    for f in $DUMPS; do
        log "Pushing $(basename "$f") off-host to r2crypt:db/ ..."
        rclone copy "$f" r2crypt:db/ --s3-no-check-bucket
    done
    log "Off-host dump push complete."

    # Mirror the MinIO receipts bucket off-host. rclone sync deletes remote
    # extras, and R2 has no object versioning, so a bad sync is destructive;
    # harden with a bucket lock and/or --backup-dir (docs/backup-and-dr.md §1d).
    if [ -n "${RECEIPTS_BUCKET:-}" ] && [ "${BACKUP_SKIP_RECEIPTS:-false}" != "true" ]; then
        log "Mirroring receipts bucket ${RECEIPTS_BUCKET} off-host ..."
        rclone sync "minio:${RECEIPTS_BUCKET}" r2crypt:receipts/ --s3-no-check-bucket
        log "Receipts mirror complete."
    fi
else
    warn_loud "Off-host replication is NOT configured (RCLONE_CONFIG_R2CRYPT_REMOTE unset). Backups exist ONLY on this host: a single disk loss destroys the database AND every backup. Configure R2 per docs/backup-and-dr.md."
    if [ "${BACKUP_REQUIRE_OFFHOST:-false}" = "true" ]; then
        log "ERROR: BACKUP_REQUIRE_OFFHOST=true; failing the run."
        exit 1
    fi
fi

# Dead-man's-switch: ping the healthcheck only on full success. Any earlier
# failure (set -e) skips this, so a missed ping signals a broken backup run.
if [ -n "${HEALTHCHECK_URL:-}" ]; then
    curl -fsS -m 10 "${HEALTHCHECK_URL}" >/dev/null || log "WARN: healthcheck ping failed"
fi
