#!/bin/sh
# Shared helpers for backup.sh and restore-test.sh (sourced, not executed).

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Loud, multi-line warning on stderr so it stands out in `docker logs`.
warn_loud() {
    {
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] ################################################################"
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARNING: $1"
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] ################################################################"
    } >&2
}

require_db_env() {
    for var in DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME; do
        eval val="\${${var}:-}"
        if [ -z "$val" ]; then
            log "ERROR: ${var} is not set"
            exit 1
        fi
    done
}

# Escape a field for a .pgpass line (backslash and colon are special).
pgpass_escape() {
    printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/:/\\:/g'
}

# Write a private (0600) .pgpass for DB_* and point libpq at it, so the
# password is never exported as PGPASSWORD (visible in /proc/<pid>/environ of
# every child) or passed on a command line. DB_PASSWORD is then unset so
# children (pg_dump, rclone, curl) do not inherit it either. The file lives on
# the /tmp tmpfs and is removed on exit.
setup_pgpass() {
    PGPASSFILE="$(mktemp /tmp/pgpass.XXXXXX)"
    export PGPASSFILE
    chmod 0600 "$PGPASSFILE"
    printf '%s:%s:*:%s:%s\n' \
        "$(pgpass_escape "$DB_HOST")" \
        "$(pgpass_escape "$DB_PORT")" \
        "$(pgpass_escape "$DB_USER")" \
        "$(pgpass_escape "$DB_PASSWORD")" > "$PGPASSFILE"
    unset DB_PASSWORD
    # Honour the app's SSL mode (require on Supabase); libpq default otherwise.
    export PGSSLMODE="${DB_SSLMODE:-prefer}"
    export PGHOST="$DB_HOST" PGPORT="$DB_PORT" PGUSER="$DB_USER"
    trap 'rm -f "$PGPASSFILE"' EXIT
}

# record_backup_run <started_at epoch> <ok|failed> <size_bytes or empty> <error>
# Inserts one row into backup_runs (the API's "last backup" heartbeat, plan 023
# 1.7). Strictly best-effort: a failed insert (table missing on an older schema,
# database unreachable) is logged and never changes the backup's outcome.
record_backup_run() {
    _err="$(printf '%s' "$4" | cut -c1-500)"
    # psql variables (:'name') quote the values, so nothing is spliced into SQL.
    if ! PGCONNECT_TIMEOUT=10 psql -d "$DB_NAME" -v ON_ERROR_STOP=1 -q \
        -v started="$1" -v finished="$(date +%s)" -v status="$2" -v size="$3" -v err="$_err" \
        -f - > /dev/null 2>&1 <<'SQL'
INSERT INTO backup_runs (started_at, finished_at, status, size_bytes, error)
VALUES (to_timestamp(:started), to_timestamp(:finished), :'status', NULLIF(:'size', '')::bigint, :'err');
SQL
    then
        log "WARN: could not record the backup run in backup_runs (ignored)"
    fi
    return 0
}
