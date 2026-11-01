#!/bin/sh
# Automated restore drill: restore the newest (or a given) app dump into a
# scratch database on the same server, run sanity queries, then drop it.
# A backup that has never been restored is not a backup.
#
# Usage (inside the backup container):
#   /restore-test.sh                 # newest /backups/kuberan_*.dump
#   /restore-test.sh /backups/x.dump # a specific dump
#
# Environment:
#   RESTORE_TEST_ENABLED      must be "true" for the scheduled (cron) run;
#                             a manual run with --force ignores it
#   RESTORE_TEST_DB           scratch database name (default kuberan_restore_test)
#   RESTORE_TEST_HEALTHCHECK_URL  optional dead-man's-switch pinged on success
#
# Needs CREATEDB on the server (the self-hosted kuberan role has it). The
# scratch database is dropped on exit, success or failure.
set -eu
umask 077

# shellcheck source-path=SCRIPTDIR source=lib.sh
. "${BACKUP_LIB:-$(dirname "$0")/lib.sh}"

FORCE=false
if [ "${1:-}" = "--force" ]; then
    FORCE=true
    shift
fi

if [ "$FORCE" != "true" ] && [ "${RESTORE_TEST_ENABLED:-false}" != "true" ]; then
    log "Restore test disabled (set RESTORE_TEST_ENABLED=true, or run with --force)."
    exit 0
fi

require_db_env

SCRATCH_DB="${RESTORE_TEST_DB:-kuberan_restore_test}"
case "$SCRATCH_DB" in
    "$DB_NAME" | hydra | postgres | template0 | template1 | "")
        log "ERROR: refusing to use '${SCRATCH_DB}' as the scratch database"
        exit 1
        ;;
esac
case "$SCRATCH_DB" in
    *[!a-z0-9_]*)
        log "ERROR: RESTORE_TEST_DB may only contain [a-z0-9_]"
        exit 1
        ;;
esac

DUMP="${1:-}"
if [ -z "$DUMP" ]; then
    # Newest app dump by mtime (file names have no spaces).
    # shellcheck disable=SC2012
    DUMP="$(ls -1t "${BACKUP_DIR:-/backups}"/kuberan_*.dump 2>/dev/null | head -n 1 || true)"
fi
if [ -z "$DUMP" ] || [ ! -f "$DUMP" ]; then
    log "ERROR: no dump found to restore"
    exit 1
fi

setup_pgpass

drop_scratch() {
    psql -d "$DB_NAME" -v ON_ERROR_STOP=1 -q -c "DROP DATABASE IF EXISTS ${SCRATCH_DB};" || true
}
# Replace lib.sh's trap: drop the scratch DB, then remove the pgpass file.
trap 'drop_scratch; rm -f "$PGPASSFILE"' EXIT

log "Restore test: ${DUMP} -> ${SCRATCH_DB}@${DB_HOST}:${DB_PORT}"
drop_scratch
psql -d "$DB_NAME" -v ON_ERROR_STOP=1 -q -c "CREATE DATABASE ${SCRATCH_DB};"

pg_restore --no-owner --no-acl --exit-on-error -d "$SCRATCH_DB" "$DUMP"
log "pg_restore finished"

q() {
    psql -d "$SCRATCH_DB" -v ON_ERROR_STOP=1 -tA -c "$1"
}

DIRTY="$(q "SELECT dirty FROM schema_migrations LIMIT 1;")"
VERSION="$(q "SELECT version FROM schema_migrations LIMIT 1;")"
if [ "$DIRTY" != "f" ]; then
    log "ERROR: restored schema_migrations is dirty or missing (dirty='${DIRTY}')"
    exit 1
fi
USERS="$(q "SELECT count(*) FROM users;")"
ACCOUNTS="$(q "SELECT count(*) FROM accounts;")"
TXNS="$(q "SELECT count(*) FROM transactions;")"
log "Sanity: migration version ${VERSION}, ${USERS} users, ${ACCOUNTS} accounts, ${TXNS} transactions"
if [ "$USERS" -lt 1 ]; then
    log "ERROR: restored database has no users"
    exit 1
fi

log "Restore test PASSED"
if [ -n "${RESTORE_TEST_HEALTHCHECK_URL:-}" ]; then
    curl -fsS -m 10 "${RESTORE_TEST_HEALTHCHECK_URL}" >/dev/null || log "WARN: restore-test healthcheck ping failed"
fi
