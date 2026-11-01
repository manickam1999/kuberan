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
