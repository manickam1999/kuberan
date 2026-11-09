#!/usr/bin/env bash
#
# Validate both compose files with `docker compose config -q`.
#
#   - docker-compose.yml: validated as is.
#   - docker-compose.prod.yml: validated with an env file built from
#     .env.prod.example plus a dummy value for every variable the compose file
#     marks as required (${VAR:?msg} / ${VAR?msg}), with every profile the file
#     defines enabled, so services behind a profile are checked too.
#
# This catches YAML errors such as a duplicate key (see PR #59) before a deploy
# does. Nothing is started or built; the dummy values never leave a temp file.
#
# Usage: ./scripts/check-compose.sh        (run from anywhere)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TMP_ENV="$(mktemp "${TMPDIR:-/tmp}/kuberan-compose-env.XXXXXX")"
trap 'rm -f "$TMP_ENV"' EXIT

echo "==> docker-compose.yml"
docker compose -f docker-compose.yml config -q
echo "    ok"

PROD=docker-compose.prod.yml

# Required variables: every ${VAR:?...}, ${VAR?...} (and the :-less forms).
REQUIRED="$(grep -oE '\$\{[A-Za-z_][A-Za-z0-9_]*:?\?' "$PROD" | sed -E 's/^\$\{//; s/:?\?$//' | sort -u)"
if [ -z "$REQUIRED" ]; then
    echo "ERROR: found no required variables in $PROD; the pattern in this script is stale." >&2
    exit 1
fi

# Start from the example, drop any assignment of a required variable, then
# append dummies so the result does not depend on the example's own values.
PATTERN="^[[:space:]]*($(printf '%s\n' "$REQUIRED" | paste -sd'|' -))="
grep -vE "$PATTERN" .env.prod.example > "$TMP_ENV" || true
for var in $REQUIRED; do
    echo "${var}=dummy-${var}" >> "$TMP_ENV"
done

# Every profile the file defines (postgres, backup, oracle, ...). The profile
# list is read from compose itself so a new profile is covered automatically.
PROFILE_ARGS=()
while IFS= read -r p; do
    [ -n "$p" ] && PROFILE_ARGS+=(--profile "$p")
done < <(docker compose -f "$PROD" --env-file "$TMP_ENV" config --profiles)
# `bot` has no profile today; passing it is harmless and keeps this working if
# it gets one.
case " ${PROFILE_ARGS[*]} " in *" bot "*) ;; *) PROFILE_ARGS+=(--profile bot) ;; esac

echo "==> $PROD ($(printf '%s ' "${PROFILE_ARGS[@]}"))"
echo "    required variables filled with dummies: $(echo "$REQUIRED" | wc -l | tr -d ' ')"
docker compose -f "$PROD" --env-file "$TMP_ENV" "${PROFILE_ARGS[@]}" config -q
echo "    ok"
