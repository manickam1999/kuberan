# Multi-currency: operator guide and deploy-day runbook

Plan 024 makes Kuberan store every amount in the currency it happened in and convert only when it reads. This page is
what you follow on deploy day. It covers what changed, the ordered runbook (backup to checks), and how to roll back.

> **Conventions used on this page.** Commands run on the server from your **install directory** (for example `/opt/kuberan`
> or `~/kuberan`). They work in bash and zsh, on Linux and macOS: where the two differ, both forms are shown. Anything
> in `<ANGLE_BRACKETS>` is a placeholder for your own value. Never paste real account ids, emails or statement contents
> into issues or the repository.

## Deploy-day checklist

The short version. Each step is explained in the section named in brackets; stop at the first step that does not give
its expected outcome.

1. **Export two IBKR statements** (Annual 2025, plus a Custom range from 2026-01-01 to the latest date IBKR allows) into a
   folder outside the git checkout. Expect two CSV files whose periods are contiguous. [2.1]
2. **Check currency codes** with SQL. Expect only three-letter upper-case codes (`MYR`, `USD`). [2.2]
3. **Take a manual backup** labelled `manual-multicurrency`. Expect a new `kuberan_manual-multicurrency_<timestamp>.dump`
   at the top of `ls -lt backups/`. [2.3]
4. **Pause the oracle cron line** in whichever crontab holds it. Expect `#PAUSED` in front of it and no oracle container
   running. [2.4]
5. **Deploy** (`git pull && ./deploy/deploy.sh`). Expect no `ERROR` and migration version `47`. A build-stage failure
   (for example a GitHub 504) changes nothing: just re-run. [3, 3.1]
6. **Check the restore points.** Expect a `predeploy` dump (or at least the `manual-multicurrency` one from step 3), then take a
   `pre-datasteps` dump. Expect it to be the newest file in `ls -lt backups/`. [3.2]
7. **Set up the shell and log in.** Expect `{"error": null, "has_token": true}` and `base_currency` `MYR`. [3.3]
8. **Fix RBOT** (needs only `PIPELINE_API_KEY`). Expect HTTP 200 with `exchange: LSE`, `currency: USD`. [4.1]
9. **Reconcile**: dry run, check every unexpected mismatch against your holdings, then `--apply`. Expect
   `applied` equal to `mismatches`; a Yahoo `fetch failed` row (exit code 1) is harmless. [4.2]
10. **Backfill** FX and prices (silent for a few minutes). Expect an FX table, then a prices table with `REJECTED` 0 on
    every row. [4.3]
11. **Back up again** (`pre-ibkr-import`), note the account ids, create the `IBKR Cash (USD)` account. Expect
    `account.currency: "USD"`. [5.1, 5.2]
12. **Import dry run.** Expect `applied: false`, `errors: []`, every security `quantity_ok` true. [5.3]
13. **Import apply.** Expect `applied: true`, `errors: []`. [5.4]
14. **Review and deactivate the old MYR IBKR Cash account.** Expect a negative `legacy_account_balance_after` and no
    transfers left in it. [5.5]
15. **Run the oracle once by hand, then restore the cron line.** Expect no fetch errors and no `currency_mismatch`
    rejections. [6.1]
16. **Post-deploy checks** (snapshots, holdings, portfolio, `fx_missing`). Expect a small net-worth change (the real
    migration moved -0.04%). [6.2]
17. **Delete the statements and report JSONs.** Expect an empty statements folder. [6.3]

Which steps need what: step 8 needs only `PIPELINE_API_KEY`; steps 9 and 10 (the oracle subcommands) need no login at
all; steps 11 to 14 and 16 need the **user token** (account creation can also be done in the web app; the import has
no web UI and is API only).

Contents:

1. [What changed](#1-what-changed)
2. [Before you deploy](#2-before-you-deploy)
3. [Deploy](#3-deploy)
4. [Fix the securities and backfill history](#4-fix-the-securities-and-backfill-history)
5. [Import the IBKR statements](#5-import-the-ibkr-statements)
6. [Resume the oracle and check the result](#6-resume-the-oracle-and-check-the-result)
7. [Rollback](#7-rollback)
8. [Reference](#8-reference)

## 1. What changed

- **Prices keep their own currency.** The oracle used to convert every foreign price to MYR before posting it. It now
  posts the price in the security's currency (USD for US and USD-line LSE listings) and Kuberan converts when it shows
  you a value. `TARGET_CURRENCY` is gone from the oracle (setting it only logs a warning).
- **Exchange rates are stored.** A new `fx_rates` table holds daily rates. Valuations use the latest rate, history uses
  the rate on the day of the event, and a snapshot is never re-valued afterwards. A missing rate is never treated as
  1:1: the amount is left out and the response says `fx_missing`.
- **Holdings are in the security's currency.** A USD stock's price, cost basis, fees, dividends and realized gain are USD,
  matching your broker statement. Each trade also stores a frozen MYR (base currency) figure at that day's rate, so
  gain/loss is split into price return and FX return.
- **Cash accounts can be any currency.** Transfers between accounts of different currencies need the amount that arrives
  (`to_amount`); buying or selling a security with cash in another currency needs `funding_amount` / `deposit_amount`.
  Account lists, analytics, budgets, receivables and snapshots add up in your reporting currency
  (`users.base_currency`, default MYR; change it under Settings).
- **The reporting currency locks once you have investment history.** Cost basis and realized gains are stored in the
  reporting currency of the day of each trade, so changing it afterwards would mislabel them. `PATCH /profile` with a
  different `base_currency` returns 409 `BASE_CURRENCY_LOCKED` as soon as you have any investment transaction (the same
  value is always accepted; users without investment history can still change it). Keep MYR for this deploy.
- **IBKR import.** `POST /api/v1/imports/ibkr` rebuilds your Interactive Brokers holdings and the USD cash ledger from
  Activity Statement CSVs so they match the statements exactly. See [ibkr-import.md](ibkr-import.md) for the details.
- **Web, MCP and Telegram bot** show amounts with their currency (`RM 1,234.50`, `US$1,234.50`) and total in your
  reporting currency.

Database migrations 38 to 47 are applied by the deploy:

| Migration | What it does |
|---|---|
| 000038 | `fx_rates` table |
| 000039 | `users.base_currency` (default `MYR`) |
| 000040 | `security_prices.currency`, filled from each security's current currency label (all `MYR` today) |
| 000041 | `portfolio_snapshots.currency` / `fx_missing` (existing snapshots become `MYR`) |
| 000042 | native investment amounts: `fx_rate_to_base`, `base_amount`, `*_base` columns; existing rows get base = stored amount |
| 000043 | `transactions.to_amount` (cross-currency transfers) |
| 000044 | `transactions.import_ref` (importer idempotency) |
| 000045 | `budgets.currency` (existing budgets take the owner's base currency) |
| 000046 | `receivable_settlements.account_amount` |
| 000047 | compatibility defaults (`'MYR'`) on `security_prices.currency` and `budgets.currency`, so older API images can still insert (see [Rollback](#7-rollback)) |

All of them are additive. None of them changes a stored amount.

> **Why the order below matters.** After the deploy, every security is still labelled `MYR` and the API rejects a USD price
> for a `MYR` security (`currency_mismatch`). Until the securities are corrected, the oracle must stay off, and the import
> refuses a statement for a security whose currency disagrees with the statement. So: deploy, correct the securities,
> backfill, import, and only then resume the oracle.

## 2. Before you deploy

### 2.0 Tools, database access and the install directory

You need `curl` and `jq` on the server (macOS: `brew install jq`). Check `docker compose version` too.

The SQL in this runbook is plain SQL. How you run it depends on where your database is. Define **one** of these helpers
once per shell, then every `kpsql ...` command below works (or paste the SQL into your provider's SQL editor instead).

- **Self-hosted** (the `postgres` compose profile; `DB_HOST=postgres`):

  ```bash
  export DB_USER=<DB_USER> DB_NAME=<DB_NAME>   # the values from .env.prod
  kpsql() { docker compose -f docker-compose.prod.yml --env-file .env.prod exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" "$@"; }
  ```

- **Supabase** (no local postgres container, so `docker compose exec postgres ...` does not work). Use the **Supabase
  dashboard > SQL Editor**, or a local `psql` against the same host, user and database as in `.env.prod` but on port
  **5432** (the app uses the pooler on 6543; the deploy and the backups use 5432, see
  [database-setup.md](database-setup.md#option-b-supabase)). It prompts for the password, so it never lands in shell history:

  ```bash
  kpsql() { psql "host=<DB_HOST> port=5432 user=<DB_USER> dbname=<DB_NAME> sslmode=require" "$@"; }
  ```

  (macOS: `brew install libpq` if `psql` is missing.)

Test it: `kpsql -c "SELECT count(*) FROM securities;"`.

### 2.1 Export the IBKR Activity Statements

In Client Portal: **Performance & Reports > Statements > Activity**, format **CSV**. Two files are enough:

1. the **Annual 2025** statement (1 January to 31 December 2025),
2. a **Custom Date Range** statement from **2026-01-01** to the latest date IBKR lets you pick.

The two periods must be contiguous with no overlap (the second starts the day after the first ends) and come from the
same account. The 2025 statement must end on 31 December 2025, because the import uses its Ending Cash as the opening
balance on 1 January 2026 (`cash_start_date=2026-01-01`). A third "to today" file is needed only if the second one ends
earlier than your most recent trade: a trade dated after the last statement's end is reported as `trades_after_statement`
and the apply is refused.

Statements contain the account holder's name and account number. Store them **outside the git checkout**, in a private
folder, and keep the import reports there too (a report contains the IBKR account id):

```bash
mkdir -p ~/ibkr-statements && chmod 700 ~/ibkr-statements
export STMT_DIR=~/ibkr-statements
```

Move the CSVs into `$STMT_DIR`. The server handles them in memory only; delete everything when you are done (6.3).

### 2.2 Check currency codes

Migration 40 copies `securities.currency` (VARCHAR(10)) into a new VARCHAR(3) column. A longer value aborts the migration
and with it the deploy. Every currency must be exactly three upper-case letters:

```bash
kpsql -c "SELECT currency, count(*) FROM securities GROUP BY 1 ORDER BY 1;" \
      -c "SELECT currency, count(*) FROM accounts GROUP BY 1 ORDER BY 1;"
```

(`kpsql` is defined in 2.0; on Supabase you can instead run the two `SELECT`s in the SQL Editor.)

Expect only codes such as `MYR` and `USD`. Anything else (`usd`, `MYR `, `US`, `USDT`, a longer string) must be fixed
before you deploy, for example:

```sql
UPDATE securities SET currency = 'MYR' WHERE currency = '<bad value>';
```

Use the code the security is really priced in (a crypto asset priced in MYR is `MYR`). For an account with a bad code,
`UPDATE accounts SET currency = '<code>' WHERE currency = '<bad value>';`. Re-run the query to confirm.

### 2.3 Back up the database

The deploy script takes a `predeploy` backup only if the `backup` profile is in `COMPOSE_PROFILES` in `.env.prod`. Do
not rely on it: take an explicit one first and confirm it exists. This works for self-hosted PostgreSQL and for Supabase
(the backup service dumps over port 5432, verifies the dump with `pg_restore --list`, and pushes it off-host if
you configured that):

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps \
  -e BACKUP_LABEL=manual-multicurrency -e BACKUP_SKIP_RECEIPTS=true --entrypoint /backup.sh backup
```

```bash
ls -lt backups/ | head -5
```

Expect a new `kuberan_manual-multicurrency_<timestamp>.dump` at the top (about the size of the previous dumps). Always use
`ls -lt` (newest first): `ls | tail` sorts by **name**, and `pre-datasteps` sorts before `predeploy`, so the last name is not
the newest file. If `ls` says permission denied, the files belong to the backup container's user: use `sudo ls -lt backups/`.

If the `backup` profile is not enabled at all, use your provider's backup (for example Supabase point-in-time recovery)
and note the time.

This dump is your **old-schema restore point** (the "pre-deploy" state): see 3.2 for how it differs from the others.

### 2.4 Pause the oracle schedule

The oracle runs from cron: `setup-vps.sh` installs `*/30 * * * * ... run --rm oracle` in **root's** crontab, but on a
machine you set up by hand (a Mac, a home server) it is usually in your **own** user crontab. Find the one that holds
the oracle line:

```bash
crontab -l | grep oracle
sudo crontab -l | grep oracle
```

Define a helper for whichever printed the line (shell function, so it works in bash and zsh):

```bash
oracle_crontab() { crontab "$@"; }        # the oracle line is in your user crontab
oracle_crontab() { sudo crontab "$@"; }   # the oracle line is in root's crontab (setup-vps.sh)
```

Save the crontab, then comment out the oracle line (the `sed` form below works with both GNU and BSD/macOS `sed`):

```bash
oracle_crontab -l > ~/crontab.before-multicurrency
```

```bash
oracle_crontab -l | sed -E '/docker compose.*run --rm oracle/ s/^/#PAUSED /' | oracle_crontab -
```

```bash
oracle_crontab -l | grep -n oracle
```

Expect the oracle line to start with `#PAUSED `. If it does not (your line is written differently, for example
`docker-compose`), edit it with `crontab -e` (or `sudo crontab -e`) and comment it out by hand. Then make sure no run is
in flight:

```bash
docker ps --filter name=oracle
```

Expect no rows. (If you run the schedule some other way, pause it that way.)

## 3. Deploy

From your install directory (for example `/opt/kuberan` or `~/kuberan`):

```bash
git pull && ./deploy/deploy.sh
```

Optionally validate the compose files first (the same check CI runs; it starts nothing):

```bash
./scripts/check-compose.sh
```

Expect the stages build, database start, pre-migration backup, migrations, start services, tag images, and no
`ERROR`. Check the migration version:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps -e DB_PORT=5432 --entrypoint /migrate api version
```

Expect `47`. If the deploy fails, it prints the failing stage and some rollback commands; first read 3.1, because the
right response depends on the stage. If you still have `TARGET_CURRENCY` in `.env.prod`, delete it (it is ignored).

### 3.1 If the deploy fails

Look at the stage in `!!! Deploy of <sha> FAILED (exit N) during: <stage>`.

- **`during: build`** (for example `wget: server returned error: HTTP/1.1 504` while building the backup image, from the
  download of supercronic from GitHub): the build stage changes nothing (no backup, no migration, no running service
  touched). **Do not run the printed rollback commands**; just fix the cause and run `./deploy/deploy.sh` again. For a
  transient GitHub error, check the download works and retry (use `arm64` instead of `amd64` on Apple Silicon or an ARM
  server):

  ```bash
  curl -sIL https://github.com/aptible/supercronic/releases/download/v0.2.33/supercronic-linux-amd64 | grep -i '^HTTP'
  ```

  The last status line should be `200`.
- **`yaml: ... mapping key "..." already defined`** while parsing `docker-compose.prod.yml`: a duplicate key in the compose
  file (this happened with `FX_MAX_STALENESS`, fixed in #59). It also fails before anything runs; `git pull` to get the
  fix. CI now catches this (`scripts/check-compose.sh`).
- **Any later stage** (`pre-migration backup`, `migrations`, `start services`): something may already have changed. Read the
  logs it suggests, and use [Rollback](#7-rollback) rather than guessing.

### 3.2 Check the restore points

The runbook leaves you three different restore points. Know which is which before you need one:

| Dump (label) | Taken | Schema | Data | Restore it to |
|---|---|---|---|---|
| `predeploy` (by `deploy.sh`) and/or `manual-multicurrency` (2.3) | before the migrations | **old** (before migration 38) | untouched | undo the whole deploy (needs the previous images) |
| `pre-datasteps` (this step) | after the deploy, before RBOT/reconcile/backfill/import | **new** (migrations applied) | still the old MYR labels | undo the data steps and keep the deploy |
| `pre-ibkr-import` (5.1) | after reconcile and backfill | new | securities relabelled, history backfilled | undo only the import |

An **unlabelled** `kuberan_<timestamp>.dump` is not a restore point. The backup container takes a dump every time it
starts (its `CMD` runs `/backup.sh` first), and `deploy.sh` starts it *after* the migrations. So after a deploy, the
newest unlabelled dump is **post-migration** state.

Check what exists (newest first):

```bash
ls -lt backups/ | grep -E 'predeploy|manual-multicurrency|pre-datasteps'
```

Expect a `kuberan_predeploy_<ts>.dump` from the deploy. If there is none (the `backup` profile was not enabled in
`COMPOSE_PROFILES` at deploy time), your old-schema restore point is the `manual-multicurrency` dump from 2.3; you still
have only one. Either way, take the new-schema one **before the first data step**:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps \
  -e BACKUP_LABEL=pre-datasteps -e BACKUP_SKIP_RECEIPTS=true --entrypoint /backup.sh backup
```

```bash
ls -lt backups/ | head -3
```

Expect `kuberan_pre-datasteps_<timestamp>.dump` first. (Sort by time with `ls -lt`, never `ls | tail`: names sort
alphabetically.)

### 3.3 Set up shell variables and log in

The API listens on the host loopback (`PUBLISH_ADDRESS`, default `127.0.0.1:8080`), so run the commands below on the
server. They need `curl` and `jq`.

```bash
export API_URL=http://127.0.0.1:8080
```

The pipeline key (needed only for step 4.1; the oracle subcommands read it from `.env.prod` themselves):

```bash
export PIPELINE_API_KEY=$(grep -E '^PIPELINE_API_KEY=' .env.prod | tail -n 1 | cut -d= -f2- | tr -d "'\"")
```

**User token** (needed for the account, import and check calls in sections 5 and 6, nowhere else). Read the credentials
without echoing the password or putting it in shell history. Use the form for your shell:

```bash
# zsh (macOS default)
read "KUBERAN_EMAIL?Kuberan login email: "
read -s "KUBERAN_PASSWORD?Password: "; echo
```

```bash
# bash
read -rp 'Kuberan login email: ' KUBERAN_EMAIL
read -rsp 'Password: ' KUBERAN_PASSWORD; echo
```

The login email is the one **you registered with in Kuberan**, which is not necessarily the one shown in other docs. Then
define the login function and run it. The password is handed to `curl` on stdin (never as a command-line argument), and
the output shows only whether login worked, never the token:

```bash
export KUBERAN_EMAIL KUBERAN_PASSWORD
kuberan_login() {
  local resp
  resp=$(jq -n '{email: env.KUBERAN_EMAIL, password: env.KUBERAN_PASSWORD}' \
    | curl -sS -X POST "$API_URL/api/v1/auth/login" -H 'Content-Type: application/json' -d @-)
  TOKEN=$(printf '%s' "$resp" | jq -r '.access_token // empty'); export TOKEN
  printf '%s' "$resp" | jq '{error, has_token: (.access_token != null)}'
}
kuberan_login
```

Expect `{"error": null, "has_token": true}`. If `has_token` is `false` (the response has `access_token: null` and an
`INVALID_CREDENTIALS` error), the email or password is wrong. **Stop and fix them before retrying**: every failed login
counts toward the lockout (5 failed attempts, then 15 minutes locked; a locked account returns the same 401 as a wrong
password). Re-run the two `read` commands and `kuberan_login`.

The token lasts **15 minutes**. When a call returns 401, refresh it with one command: `kuberan_login`.

When you are done with the runbook, wipe the credentials from the shell: `unset KUBERAN_PASSWORD KUBERAN_EMAIL TOKEN`.

Sanity check that migration 39 gave you the right reporting currency:

```bash
curl -sS "$API_URL/api/v1/profile" -H "Authorization: Bearer $TOKEN" | jq .
```

Expect `base_currency` to be `MYR`.

## 4. Fix the securities and backfill history

### 4.1 Fix RBOT (it is mapped to the wrong listing)

RBOT is the iShares Automation & Robotics USD line on the London exchange, but it is stored as the Toronto listing
`RBOT.TO`. Look up its id (this step and the PATCH below need only `PIPELINE_API_KEY`, not the user token):

```bash
curl -sS "$API_URL/api/v1/pipeline/securities" -H "X-API-Key: $PIPELINE_API_KEY" \
  | jq -r '.securities[] | select(.symbol=="RBOT") | [.id,.symbol,.exchange,.provider_symbol,.currency] | @tsv'
```

Expect one row (`RBOT`, exchange `TSX`, provider symbol `RBOT.TO`). If you prefer SQL (`kpsql` is defined in 2.0; on
Supabase use the SQL Editor or the 5432 `psql` helper):

```bash
kpsql -c "SELECT id, symbol, exchange, provider_symbol, currency FROM securities WHERE symbol='RBOT' AND deleted_at IS NULL;"
```

Then correct it. `reset_prices` deletes the prices stored under the wrong listing:

```bash
export RBOT_SECURITY_ID=<RBOT_SECURITY_ID>
```

```bash
curl -sS -X PATCH "$API_URL/api/v1/pipeline/securities/$RBOT_SECURITY_ID" \
  -H "X-API-Key: $PIPELINE_API_KEY" -H 'Content-Type: application/json' \
  -d '{"exchange":"LSE","provider_symbol":"RBOT.L","currency":"USD","reset_prices":true}' | jq .
```

Expect HTTP 200 with the security now `exchange: LSE`, `currency: USD`, and `prices_deleted: <n>`. A 409
`SECURITY_CURRENCY_LOCKED` means you left out `reset_prices`; a 409 on the exchange means another RBOT row already exists
on LSE (look at the list above).

### 4.2 Reconcile currencies (dry run first)

`oracle reconcile` compares each Yahoo-priced security's stored currency with the currency Yahoo reports. Without
`--apply` it changes nothing.

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle reconcile
```

Expect a table with `SYMBOL EXCHANGE PROVIDER_SYMBOL STORED PROVIDER ACTION`. The rows to look for are the ones marked
`mismatch (dry run, use --apply)` with `STORED MYR` and `PROVIDER USD`: the US and USD-line securities (in the owner's
migration these were CCJ, CHA, IBKR, NVDA, NVO, SOUN, SPCX, VUAA and VWRA, plus a tenth, GOLD (`GC=F`), that nobody had
listed). **The set is not fixed**: it is whatever you hold, so compare it with your own list instead of counting to nine.
Rows that say `ok` are the Bursa securities (MYR) and RBOT (already USD after step 4.1). Crypto is not listed (it is
not priced through Yahoo; BTC, ETH and SUI stay MYR). The last line reads
`reconcile (dry run): mismatches=<n> applied=0 failed=<f>`.

Rows that say `fetch failed: ...` are Yahoo errors for that symbol. A persistent 404 (in the owner's migration VERSA,
`0807EA.KL`) is **harmless**: the security is left unchanged, and the command exits non-zero (`failed=1`) only because a
fetch failed. Re-run once to rule out a hiccup; if the same symbol keeps failing with a 404, carry on.

#### Check each unexpected mismatch against your holdings

`--apply` changes **every** mismatch and cannot skip one. For each mismatched symbol you did not expect, look at what you
hold before applying (replace the symbol list):

```bash
kpsql <<'SQL'
SELECT s.symbol, s.provider_symbol, s.currency AS stored_currency,
  (SELECT count(*) FROM investments i
     WHERE i.security_id = s.id AND i.deleted_at IS NULL) AS holdings,
  (SELECT coalesce(sum(i.quantity), 0) FROM investments i
     WHERE i.security_id = s.id AND i.deleted_at IS NULL) AS quantity,
  (SELECT count(*) FROM investment_transactions t
     JOIN investments i ON i.id = t.investment_id
     WHERE i.security_id = s.id AND i.deleted_at IS NULL AND t.deleted_at IS NULL) AS trades
FROM securities s
WHERE s.symbol IN ('GOLD', '<OTHER_SYMBOL>') AND s.deleted_at IS NULL
ORDER BY s.symbol;
SQL
```

- **No holdings (`holdings` 0, `trades` 0):** safe to relabel. Reconcile just changes its currency and deletes its prices.
- **Held only in the IBKR account and covered by the statements:** safe. The import replaces those trades with native
  ones (step 5).
- **Held with legacy trades that the import does not replace:** the existing amounts were stored in MYR, and once the
  security is relabelled USD they are read as USD, so its native cost basis comes out wrong. Reconcile cannot fix trades;
  only the IBKR import replaces them. Decide first: accept it and correct that holding by hand afterwards, or resolve the
  mismatch another way (for example the listing is wrong, so fix its `provider_symbol` / `exchange` with the PATCH in 4.1
  and re-run the dry run). Do not apply blindly.

Other things to look at before applying:

- **`RBOT` still shows a mismatch:** step 4.1 was not applied.
- **A symbol that should match but does not, or the reverse:** it is mapped to the wrong listing or Yahoo reports a
  different currency. Fix its `provider_symbol` / `exchange` with the PATCH above, then repeat the dry run.

Then apply. Each mismatch is PATCHed with its provider currency and `reset_prices: true`, which deletes that security's
(wrongly converted) prices in the same transaction:

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle reconcile --apply
```

Expect `updated, prices_deleted=<n>` for every mismatch and `reconcile (apply): mismatches=<n> applied=<n> failed=<f>`, with
`failed` covering only the harmless fetch failures you saw in the dry run. Re-running the dry run now shows no mismatches.

### 4.3 Backfill FX and price history

This fetches daily USD/MYR rates and the native price history from Yahoo and posts them. FX is always done before prices.
Optionally rehearse first with `--dry-run` (it fetches and counts but posts nothing):

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle backfill --since 2025-01-01 --dry-run
```

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle backfill --since 2025-01-01
```

It looks stuck: it prints **nothing** while it downloads every history first, then the FX table, then the prices table. In
the owner's migration this took a few minutes. To check progress from a second terminal:

```bash
docker ps --filter name=oracle
```

(a row means it is still running), and, once it has started posting (FX goes first, prices after all downloads
finished), the row counts should grow between runs of:

```bash
kpsql -c "SELECT count(*) AS fx_rates FROM fx_rates;" \
      -c "SELECT count(*) AS usd_prices FROM security_prices WHERE currency = 'USD';"
```

Expect an `FX PAIR` table (at least `USD/MYR`, status `ok`, with `FETCHED` and `RECORDED` roughly 500 or more daily
bars), then a `SYMBOL CURRENCY FETCHED RECORDED REJECTED STATUS` table with one row per Yahoo-priced security (your USD securities, RBOT and the Bursa ones), then totals
lines. `REJECTED` must be 0 for every row. Exit code 0.

- **`rejected` greater than 0 or `currency_mismatch`**: the security's stored currency still differs from Yahoo's; re-run
  `reconcile`.
- **`fetch failed` rows**: Yahoo hiccups; re-run just those with `--symbols CCJ,NVDA` (add `--prices`).
- **Bursa rows with `RECORDED` lower than `FETCHED` and `REJECTED` 0 are normal**: timestamps that already exist are skipped.
- Re-running is safe. Rates are upserted (a re-run overwrites the stored rate for the same day). Prices are inserted
  with `ON CONFLICT DO NOTHING`, so a re-run records 0 for days that already have a price (`RECORDED` is lower than
  `FETCHED`, `REJECTED` stays 0) and never overwrites an existing price.
- Options: `--fx` or `--prices` alone, `--until YYYY-MM-DD` (default today UTC), `--symbols A,B`.

Crypto (priced by CoinGecko, currency unchanged) is skipped (the log says "skipping security not priced by Yahoo"). Bursa
securities are included but already MYR, so their existing days are simply skipped.

## 5. Import the IBKR statements

The import is explained in full in [ibkr-import.md](ibkr-import.md); this section is the order of operations for your
data.

### 5.1 Back up again, find the ids

The account and import calls below need the user token (3.3; refresh with `kuberan_login`).

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps \
  -e BACKUP_LABEL=pre-ibkr-import -e BACKUP_SKIP_RECEIPTS=true --entrypoint /backup.sh backup
```

List your accounts and note two ids: the IBKR **investment** account, and the old **IBKR Cash** account (currently
labelled MYR):

```bash
curl -sS "$API_URL/api/v1/accounts?page_size=100" -H "Authorization: Bearer $TOKEN" \
  | jq -r '.data[] | [.id,.type,.currency,.name,.is_active] | @tsv'
```

```bash
export IBKR_INVESTMENT_ACCOUNT_ID=<id of the IBKR investment account>
export OLD_IBKR_CASH_ACCOUNT_ID=<id of the old IBKR Cash account>
```

(SQL alternative: `kpsql -c "SELECT id, type, currency, name FROM accounts WHERE deleted_at IS NULL ORDER BY name;"`.)

### 5.2 Create the `IBKR Cash (USD)` account

A cash account in USD with a zero opening balance. The import writes the real opening balance itself (the 2025 statement's
Ending Cash on 2026-01-01). You can also create it in the web app (Accounts > New account > Cash, currency USD); the import itself has no web UI.

```bash
curl -sS -X POST "$API_URL/api/v1/accounts/cash" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"IBKR Cash (USD)","currency":"USD","initial_balance":0}' | jq .
```

Expect HTTP 201 and `account.currency: "USD"`.

```bash
export IBKR_USD_CASH_ACCOUNT_ID=<account.id from the response>
```

### 5.3 Dry run with all statements

Keep the files and the reports in `$STMT_DIR` (2.1), outside the checkout. One `-F files=@...` per CSV (1 to 10, in any
order; each at most `MAX_UPLOAD_BYTES`, default 10 MiB). Without `apply=true` nothing is written:

```bash
export REPORT="$STMT_DIR/import-report.json"
```

```bash
curl -sS -X POST "$API_URL/api/v1/imports/ibkr" \
  -H "Authorization: Bearer $TOKEN" \
  -F files=@"$STMT_DIR/<STATEMENT_2025.csv>" \
  -F files=@"$STMT_DIR/<STATEMENT_2026.csv>" \
  -F investment_account_id=$IBKR_INVESTMENT_ACCOUNT_ID \
  -F cash_account_id=$IBKR_USD_CASH_ACCOUNT_ID \
  -F cash_start_date=2026-01-01 \
  -F legacy_cash_account_id=$OLD_IBKR_CASH_ACCOUNT_ID \
  > "$REPORT"
```

(Add a third `-F files=@...` line only if your second statement ends before your latest trade.) Read the report with these
summaries; the same three commands work on the applied report in 5.4:

```bash
jq '{applied, periods, errors, warnings}' "$REPORT"
```

```bash
jq '.securities[] | {symbol, existing_transactions, new_transactions,
      final_quantity: .final.quantity,
      all_quantity_ok: ([.checkpoints[].quantity_ok] | all),
      last_cost_basis_diff: .checkpoints[-1].cost_basis_diff}' "$REPORT"
```

```bash
jq '.cash | {opening_balance, legs, dividends, fees, checkpoints,
      deposits_matched: (.deposits_matched | length),
      unmatched_deposits,
      legacy_account_balance_after}' "$REPORT"
```

(For the full per-checkpoint detail of one security: `jq '.securities[] | select(.symbol=="<SYMBOL>") | .checkpoints' "$REPORT"`.)

What you should see:

- `applied: false`, `periods` contiguous from 2025-01-01 to the end of your last statement, `errors: []`.
- Every security `all_quantity_ok: true`. `last_cost_basis_diff` may be non-zero, and `warnings` will contain
  cost-basis notes: Kuberan uses average cost, IBKR uses lots, so this is only a warning (expect it for partly sold
  positions such as NVO, and for IBKR itself).
- `cash.opening_balance` equal to the 2025 statement's Ending Cash (in cents).
- `deposits_matched` (use the full list, `jq '.cash.deposits_matched' "$REPORT"`) lists your bank-to-IBKR MYR transfers that
  will be re-pointed at the new account, with `source_amount` (MYR) and `amount` (USD). A transfer is matched only within
  5 days of the deposit and when its implied MYR/USD rate is within 3% of the market rate for that day.
- `cash.checkpoints`: `ledger` equals `ibkr` at each statement end, within one cent per row. The final one is the cash
  balance you should see after the import (IBKR's Ending Cash on the last statement).
- `cash.legacy_account_balance_after`: what is left on the old account once the matched transfers are re-pointed. It is
  normally **negative**, and that is expected, not an error: the old MYR account keeps the legacy trade legs (the MYR
  cash it paid out for buys and received for sells), while the incoming transfers that used to fund them now point at the
  new USD account. See 5.5 before deactivating it.

To see everything that touches the old account (including incoming transfers, which `GET /accounts/:id/transactions`
does **not** list, because it only returns rows whose source account is that account):

```bash
kpsql -c "SELECT t.date, t.type, t.amount, t.to_amount, (t.account_id = '<OLD_IBKR_CASH_ACCOUNT_ID>') AS outgoing, t.description
          FROM transactions t
          WHERE (t.account_id = '<OLD_IBKR_CASH_ACCOUNT_ID>' OR t.to_account_id = '<OLD_IBKR_CASH_ACCOUNT_ID>')
            AND t.deleted_at IS NULL ORDER BY t.date;"
```

Before the apply this still shows the incoming bank transfers (`outgoing` = f, type `transfer`); they are the ones that
move.

If the report is not clean:

| You see | Meaning and what to do |
|---|---|
| `trades_after_statement` | Kuberan already has a trade dated after the last statement's end. Those trades are never deleted. Export a statement that runs through today and add it. |
| `fx_rate_missing` | No USD/MYR rate on a trade date. Run `oracle backfill --fx --since 2025-01-01` (step 4.3) and dry-run again. |
| `unknown_security` | The statement has a symbol Kuberan does not have. The import never creates securities; add it (web app > Securities, or `POST /api/v1/pipeline/securities`) with the right currency, then retry. |
| `currency_mismatch` | A security's stored currency differs from the statement's. Re-run `oracle reconcile --apply` (or PATCH the security). |
| `insufficient_shares` | The replay sold more than it held. A statement in the middle is missing (periods not contiguous) or a trade is on the wrong security. Check `periods`. |
| `opening_balance_missing` | No statement ends the day before `cash_start_date`. Include the 2025 statement (ending 2025-12-31) or change `cash_start_date`. |
| a `quantity_ok: false` checkpoint | The replayed quantity differs from IBKR's Open Positions (tolerance 1e-4). Usually a missing statement period or a trade booked on the wrong security. Do not apply. |
| `unmatched_deposits` | Deposits or withdrawals with no matching Kuberan transfer (each lists rejected candidates and their rate deviation). Withdrawals are never matched, so they always appear here. Applying is allowed; record them by hand afterwards (see 5.5). The cash ledger leaves them out, so the final cash checkpoint shows them as `diff`. |
| non-empty `errors` of any kind | The apply is refused with HTTP 400 `IMPORT_INVALID` (report in `error.details.report`). Fix and re-run the dry run. |

### 5.4 Apply

> **Before you apply with `deactivate_legacy_cash_account=true`:** deactivating the legacy account takes everything
> still in it out of your net worth, because account lists and snapshots only count active accounts. That includes
> unmatched top-ups, the old trade cash legs and any manual "FX Spread Management" rows. Review the remaining
> transactions and `legacy_account_balance_after` from 5.3 first. If anything there still matters, apply without the
> flag and deactivate later (5.5) once it is dealt with.

Repeat exactly the same command with `-F apply=true`. Deactivate the old account in a separate step afterwards (5.5), once
you have looked at what stays behind; `-F deactivate_legacy_cash_account=true` does it in the same call. It runs in one database transaction; any error rolls everything back.

```bash
curl -sS -X POST "$API_URL/api/v1/imports/ibkr" \
  -H "Authorization: Bearer $TOKEN" \
  -F files=@"$STMT_DIR/<STATEMENT_2025.csv>" \
  -F files=@"$STMT_DIR/<STATEMENT_2026.csv>" \
  -F investment_account_id=$IBKR_INVESTMENT_ACCOUNT_ID \
  -F cash_account_id=$IBKR_USD_CASH_ACCOUNT_ID \
  -F cash_start_date=2026-01-01 \
  -F legacy_cash_account_id=$OLD_IBKR_CASH_ACCOUNT_ID \
  -F apply=true \
  > "$STMT_DIR/import-applied.json"
```

```bash
export REPORT="$STMT_DIR/import-applied.json"
jq '{applied, errors, legacy_balance_after: .cash.legacy_account_balance_after}' "$REPORT"
```

Expect `applied: true`, `errors: []`, and `legacy_balance_after` equal to what is left on the old account (negative
in the usual case, see 5.3). Use the three summary commands from 5.3 on this report to confirm the same numbers as in
the dry run. A `409 IMPORT_IN_PROGRESS` means another apply is running; wait and retry. Re-running the same
apply is safe: it replaces its own rows (found by `import_ref`) and changes nothing.

### 5.5 Review and deactivate the legacy cash account

A **negative** `legacy_account_balance_after` is expected. The old MYR `IBKR Cash` account keeps the legacy trade legs
(the MYR cash for buys and sells), but the bank-to-IBKR transfers that funded them were re-pointed at the new USD account,
so the old account is left with the outflows. List what is left (the SQL from 5.3, now after the apply):

```bash
kpsql -c "SELECT t.date, t.type, t.amount, (t.account_id = '<OLD_IBKR_CASH_ACCOUNT_ID>') AS outgoing, t.description
          FROM transactions t
          WHERE (t.account_id = '<OLD_IBKR_CASH_ACCOUNT_ID>' OR t.to_account_id = '<OLD_IBKR_CASH_ACCOUNT_ID>')
            AND t.deleted_at IS NULL ORDER BY t.date;"
```

(Or, without SQL, outgoing rows only: `curl -sS "$API_URL/api/v1/accounts/$OLD_IBKR_CASH_ACCOUNT_ID/transactions?page_size=100" -H "Authorization: Bearer $TOKEN" | jq -r '.data[] | [.id,.date,.type,.amount,.description] | @tsv'`.)

The remaining rows should be only the old investment legs (type `investment`) and any manual rows you made yourself
(for example "FX Spread Management"), and **no transfers** (type `transfer`). A transfer still listed is a deposit that was
not matched (see `unmatched_deposits` below); do not deactivate until you have dealt with it.

Then deactivate the old account so it stops showing up in totals. This removes whatever is left in it from net worth, so
check the list above first:

```bash
curl -sS -X PUT "$API_URL/api/v1/accounts/$OLD_IBKR_CASH_ACCOUNT_ID" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"is_active":false}' | jq '.account | {name, is_active, balance}'
```

(Skip this if you applied with `deactivate_legacy_cash_account=true`.) Expect `is_active: false`.

Record any `unmatched_deposits` by hand (web app: Transactions > New transfer, which asks for the second amount):

- **The report lists `candidates`** (an existing transfer within 5 days whose implied rate was rejected). The bank
  was already debited by that transfer, so do not record a second one. Transfers can't be edited, only deleted:
  delete the existing wrong transfer, then recreate it as a cross-currency transfer from the same MYR bank account into
  `IBKR Cash (USD)` with the MYR `amount` and the USD `to_amount`.
- **No candidate listed.** Create a new transfer from the MYR bank account to `IBKR Cash (USD)` with the MYR `amount`
  and the USD `to_amount`.
- **Withdrawals** are never matched. Record each as a transfer out of `IBKR Cash (USD)` into the receiving account
  (USD `amount`, and `to_amount` in the receiving currency if it differs).

## 6. Resume the oracle and check the result

### 6.1 Take one manual oracle run, then resume the schedule

Run the oracle once by hand and watch it. It posts native prices and FX rates, computes snapshots and processes fixed
deposits:

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle
```

Expect the summary with zero fetch errors and no `currency_mismatch` rejections (a rejection means a security is still
labelled with the wrong currency). Then restore the cron schedule (with the `oracle_crontab` helper from 2.4):

```bash
oracle_crontab ~/crontab.before-multicurrency
```

```bash
oracle_crontab -l | grep -n oracle
```

Expect the oracle line back without the `#PAUSED` prefix.

### 6.2 Post-deploy checks

These use the user token (refresh with `kuberan_login`).

1. **Holdings vs IBKR.** Compare each IBKR holding with the Open Positions of the latest statement:

   ```bash
   curl -sS "$API_URL/api/v1/accounts/$IBKR_INVESTMENT_ACCOUNT_ID/investments?page_size=100" -H "Authorization: Bearer $TOKEN" \
     | jq -r '.data[] | [.security.symbol, .quantity, .price_currency, .current_price_native, .cost_basis, .cost_basis_base, .fx_missing] | @tsv'
   ```

   Columns: symbol, quantity, price currency, latest price (native, cents), cost basis (native, cents), cost basis (base,
   cents), `fx_missing`. Quantities must equal IBKR's, `price_currency` is the security's own currency, and `fx_missing`
   is `false` everywhere.
2. **Portfolio totals.**

   ```bash
   curl -sS "$API_URL/api/v1/investments/portfolio" -H "Authorization: Bearer $TOKEN" \
     | jq '.portfolio | {currency, total_value, total_cost_basis, total_gain_loss, total_price_return, total_fx_return, fx_stale, unconverted}'
   ```

   `currency` is `MYR`, `unconverted` is empty, and `total_price_return + total_fx_return` equals `total_gain_loss`
   within a cent per holding. `fx_stale: true` only means the newest quote is older than `FX_MAX_STALENESS`.
3. **USD cash vs IBKR cash.** The `IBKR Cash (USD)` balance must equal the Ending Cash of the last statement,
   adjusted for the `unmatched_deposits` you have not recorded yet: add unrecorded deposits to the balance and subtract
   unrecorded withdrawals (the ledger leaves them out, so each one is the `diff` on the final cash checkpoint).
   Withdrawals are never matched, so expect them here until you record them (5.5):

   ```bash
   curl -sS "$API_URL/api/v1/accounts/$IBKR_USD_CASH_ACCOUNT_ID" -H "Authorization: Bearer $TOKEN" \
     | jq '.account | {name, currency, balance, balance_base, fx_missing}'
   ```
4. **Net worth before and after.** The snapshots endpoint returns rows **newest first**, and `to_date` is an inclusive
   bound at the **start** of that day, so `to_date=<today>` leaves out today's snapshots. Use tomorrow as `to_date`,
   sort by time, and look at the last few rows. Set the dates (GNU/Linux first, macOS/BSD second; or just type the
   `YYYY-MM-DD` values):

   ```bash
   TOMORROW=$(date -u -d tomorrow +%F)         # Linux (GNU date)
   TOMORROW=$(date -u -v+1d +%F)               # macOS (BSD date)
   FROM_DATE=<YYYY-MM-DD, a few days before the deploy>
   ```

   ```bash
   curl -sS "$API_URL/api/v1/investments/snapshots?from_date=$FROM_DATE&to_date=$TOMORROW&page_size=200" \
     -H "Authorization: Bearer $TOKEN" \
     | jq '[.data[] | {recorded_at, total_net_worth, currency, fx_missing}] | sort_by(.recorded_at) | .[-8:]'
   ```

   `from_date` and `to_date` are both required (`YYYY-MM-DD` or RFC3339); `page_size` is optional (default 20, max 3650).
   `total_net_worth` is in cents. Compare the last rows from before the deploy with the newest ones (`currency: "MYR"`,
   `fx_missing: false`); old snapshots stay as recorded. **A small change is expected**, because holdings are now revalued
   at today's rate and the import corrected the 2026 cash figures. In the owner's migration net worth moved by -0.04%
   (RM 331,055.68 to RM 330,938.79). A large jump means look for an `fx_missing` snapshot, a holding without a price, or
   an `unconverted` entry.
5. **`fx_missing` flags.** None of these should be true: the holdings and portfolio above, the accounts list
   (`jq '.data[] | select(.fx_missing==true) | .name'` on `/accounts?page_size=100`), and the latest snapshot. If one
   is, look up the pair and backfill it:

   ```bash
   curl -sS "$API_URL/api/v1/fx-rates/latest?base=USD&quote=MYR" -H "Authorization: Bearer $TOKEN" | jq .
   ```

   A 503 `FX_RATE_UNAVAILABLE` means there is no rate: run `oracle backfill --fx --since 2025-01-01`, then one normal
   oracle run. `fx_stale: true` only means the newest quote is older than `FX_MAX_STALENESS` (default 168h); the next
   oracle run refreshes it.
6. **Web app.** Open the dashboard, Investments and an IBKR holding: USD holdings show the native amount with `≈ RM`
   beside it, and the price/FX return split appears on the holding.

### 6.3 Clean up

The statements and the saved reports contain the account holder's name and IBKR account number. Delete them, or move
them somewhere private (an encrypted volume, not the server):

```bash
ls -l "$STMT_DIR"
rm -f "$STMT_DIR"/*.csv "$STMT_DIR"/import-*.json
```

Then remove the saved crontab copy if you no longer need it (`rm ~/crontab.before-multicurrency`) and clear the shell
(`unset KUBERAN_PASSWORD KUBERAN_EMAIL TOKEN`). Expect `ls -l "$STMT_DIR"` to show nothing left to keep.

## 7. Rollback

Migration 47 gives `security_prices.currency` and `budgets.currency` a default (`'MYR'`), so the previous API images
can insert into the new schema (they do not know those columns). That covers the window between the migration and the
new image, and `deploy.sh`'s automatic image rollback. The other columns added by 38 to 46 are nullable or have defaults.

But old code only works on old data. Once native-currency data exists (securities relabelled to USD in step 4, USD prices
and rates, the import in step 5), the previous images would misread native amounts as MYR. So choose by how far you got,
and use the restore point from the table in [3.2](#32-check-the-restore-points):

1. **Before step 4 (nothing but the deploy ran).** Rolling the images back is safe: roll them back as described in
   [backup-and-dr.md](backup-and-dr.md#rollback), or `git checkout <previous-sha> && ./deploy/deploy.sh`. The new
   columns stay and are harmless. To drop them as well, step the migrations back (ten of them, 47 down to 38):

   ```bash
   docker compose -f docker-compose.prod.yml --env-file .env.prod \
     run --rm --no-deps -e DB_PORT=5432 --entrypoint /migrate api down 10
   ```

   Use `down 1` to undo only the newest one. Down files exist for all ten, but they drop the columns and any data in
   them. (A build-stage failure needs none of this: see 3.1.)
2. **After the data steps (4.1 onward).** Do not roll the images back on their own: the relabelling and price deletion,
   the backfilled rates and prices, and the import (it replaces the IBKR investment transactions, rewrites the cash
   ledger and re-points transfers) are not reversible by a down migration, and old code would show native amounts as if
   they were MYR. Restore a dump instead:
   - **To undo the whole deploy:** restore the old-schema dump (`predeploy`, or `manual-multicurrency` from 2.3) **and**
     roll the images back. Old images against an old-schema dump need no migration step.
   - **To undo only the data steps and keep the deploy:** restore `pre-datasteps` (new schema, securities still
     labelled MYR) and keep the new images. To keep the reconcile and the backfill as well, restore `pre-ibkr-import`.
   - **Self-hosted:** stop `api`, `mcp` and `bot`, restore into a fresh database as in
     [backup-and-dr.md section 4](backup-and-dr.md#4--restore-drill-do-this-once-before-relying-on-it), point `DB_NAME` at it,
     and start the services.
   - **Supabase:** stop `api`, `mcp` and `bot`, then restore in place over the direct connection (port 5432) with
     `pg_restore --clean`, exactly as in [database-setup.md](database-setup.md#restoring-from-a-backup) ("Into Supabase").
     `--clean` drops and recreates what is in the dump, so Hydra's schema (inside the same database) goes back to the
     dump's state too; MCP clients may need to re-authorize. Or restore Supabase's own point-in-time backup.

   Anything you recorded after the backup is lost.
3. **Only the import went wrong.** Restore the `pre-ibkr-import` backup from 5.1, fix the cause from the table in 5.3 and
   run the import again. The import is idempotent, so you can also simply fix the input and re-run it with `apply=true`.

Keep the oracle paused during any rollback, and resume it (6.1) once the system is in the state you want.

## 8. Reference

| Item | Value |
|---|---|
| Oracle subcommands | `oracle reconcile [--apply]`, `oracle backfill --since YYYY-MM-DD [--until D] [--prices] [--fx] [--symbols A,B] [--dry-run]`, `oracle help` (run through `docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle <subcommand>`) |
| Security correction | `PATCH /api/v1/pipeline/securities/:id` with `X-API-Key`; body fields `currency`, `exchange`, `provider_symbol`, `name`, `reset_prices` |
| Pipeline FX | `GET /api/v1/pipeline/fx-pairs`, `POST /api/v1/pipeline/fx-rates` |
| FX read | `GET /api/v1/fx-rates?base=&quote=&from=&to=`, `GET /api/v1/fx-rates/latest?base=&quote=` |
| Import | `POST /api/v1/imports/ibkr` (multipart: `files` x1-10, `investment_account_id`, `cash_account_id`, `cash_start_date`, `legacy_cash_account_id`, `deactivate_legacy_cash_account`, `apply`) |
| Reporting currency | `GET/PATCH /api/v1/profile` (`base_currency`; a different value is refused with 409 `BASE_CURRENCY_LOCKED` once you have investment transactions) |
| Env vars | `FX_MAX_STALENESS` (api, default 168h), `PUBLISH_FX_RATES` (oracle), `MAX_UPLOAD_BYTES` (api, default 10 MiB), `MCP_MAX_WRITE_AMOUNT` (now compared in the base currency); `TARGET_CURRENCY` removed |
| Error codes | `FX_RATE_UNAVAILABLE` (503), `FX_AMOUNT_REQUIRED` (400, with a suggested amount), `SECURITY_CURRENCY_LOCKED` (409), `BASE_CURRENCY_LOCKED` (409), `IMPORT_INVALID` (400), `IMPORT_IN_PROGRESS` (409) |
