# Multi-currency: operator guide and deploy-day runbook

Plan 024 makes Kuberan store every amount in the currency it happened in and convert only when it reads. This page is
what you follow on deploy day. It covers what changed, the ordered runbook (backup to checks), and how to roll back.

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
- **IBKR import.** `POST /api/v1/imports/ibkr` rebuilds your Interactive Brokers holdings and the USD cash ledger from
  Activity Statement CSVs so they match the statements exactly. See [ibkr-import.md](ibkr-import.md) for the details.
- **Web, MCP and Telegram bot** show amounts with their currency (`RM 1,234.50`, `US$1,234.50`) and total in your
  reporting currency.

Database migrations 38 to 46 are applied by the deploy:

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

All of them are additive. None of them changes a stored amount.

> **Why the order below matters.** After the deploy, every security is still labelled `MYR` and the API rejects a USD price
> for a `MYR` security (`currency_mismatch`). Until the securities are corrected, the oracle must stay off, and the import
> refuses a statement for a security whose currency disagrees with the statement. So: deploy, correct the securities,
> backfill, import, and only then resume the oracle.

## 2. Before you deploy

### 2.1 Export the IBKR Activity Statements

In Client Portal: **Performance & Reports > Statements > Activity**, format **CSV**, **Custom Date Range**. Download:

1. the 2025 statement (1 January to 31 December 2025),
2. the 2026 statement from 1 January 2026 to the date you last exported,
3. one more from the day after that to **today** (the day you run the import).

The files must be contiguous (each starts the day after the previous one ends) and from the same account. The 2025
statement must end on 31 December 2025, because the import uses its Ending Cash as the opening balance on 1 January 2026
(`cash_start_date=2026-01-01`). The last statement must run through today, otherwise recent trades (for example a buy made
after your last export) are reported as `trades_after_statement` and the apply is refused.

Statements contain your name and account number. Keep them out of the repository and delete them when you are done. The
server handles them in memory only.

### 2.2 Back up the database

The deploy script takes a `predeploy` backup if the `backup` compose profile is enabled, but take an explicit one first
and confirm it exists:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps \
  -e BACKUP_LABEL=manual-multicurrency -e BACKUP_SKIP_RECEIPTS=true --entrypoint /backup.sh backup
```

```bash
ls -l backups/ | tail -5
```

Expect a new `kuberan_manual-multicurrency_<timestamp>.dump` (about the size of the previous dumps). If the `backup`
profile is not enabled, use your provider's backup (for example Supabase point-in-time recovery) and note the time.

### 2.3 Pause the oracle schedule

The oracle runs from the root crontab (`setup-vps.sh` installs `*/30 * * * * ... run --rm oracle`). Save the crontab, then
comment out the oracle line:

```bash
sudo crontab -l > ~/crontab.before-multicurrency
```

```bash
sudo crontab -l | sed -E '/docker compose.*run --rm oracle/ s/^/#PAUSED /' | sudo crontab -
```

```bash
sudo crontab -l | grep -n oracle
```

Expect the oracle line to start with `#PAUSED `. Then make sure no run is in flight:

```bash
docker ps --filter name=oracle
```

Expect no rows. (If you run the schedule some other way, pause it that way.)

## 3. Deploy

From the install directory (`/opt/kuberan`):

```bash
git pull && ./deploy/deploy.sh
```

Expect the stages build, database start, pre-migration backup, migrations, start services, tag images, and no
`ERROR`. Check the migration version:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps -e DB_PORT=5432 --entrypoint /migrate api version
```

Expect `46`. If the deploy fails, it prints the failing stage and the rollback commands; see [Rollback](#7-rollback).
If you still have `TARGET_CURRENCY` in `.env.prod`, delete it (it is ignored).

### 3.1 Set up shell variables for the rest of the runbook

The API listens on the host loopback (`PUBLISH_ADDRESS`, default `127.0.0.1:8080`), so run the commands below on the
server. They need `curl` and `jq`.

```bash
export API_URL=http://127.0.0.1:8080
```

```bash
export PIPELINE_API_KEY=$(grep -E '^PIPELINE_API_KEY=' .env.prod | tail -n 1 | cut -d= -f2- | tr -d "'\"")
```

Log in to get a user token (used for the account and import calls; it lasts 15 minutes, so repeat this when you get a
401):

```bash
export TOKEN=$(curl -sS -X POST "$API_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"<YOUR_EMAIL>","password":"<YOUR_PASSWORD>"}' | jq -r .access_token)
```

```bash
echo "${TOKEN:0:12}..."
```

Expect a token prefix, not `null`.

Sanity check that migration 39 gave you the right reporting currency:

```bash
curl -sS "$API_URL/api/v1/profile" -H "Authorization: Bearer $TOKEN" | jq .
```

Expect `base_currency` to be `MYR`.

## 4. Fix the securities and backfill history

### 4.1 Fix RBOT (it is mapped to the wrong listing)

RBOT is the iShares Automation & Robotics USD line on the London exchange, but it is stored as the Toronto listing
`RBOT.TO`. Look up its id (the pipeline list is authenticated with the pipeline key):

```bash
curl -sS "$API_URL/api/v1/pipeline/securities" -H "X-API-Key: $PIPELINE_API_KEY" \
  | jq -r '.securities[] | select(.symbol=="RBOT") | [.id,.symbol,.exchange,.provider_symbol,.currency] | @tsv'
```

Expect one row (`RBOT`, exchange `TSX`, provider symbol `RBOT.TO`). If you prefer SQL:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod exec postgres \
  psql -U "$DB_USER" -d "$DB_NAME" -c "SELECT id, symbol, exchange, provider_symbol, currency FROM securities WHERE symbol='RBOT';"
```

(`DB_USER` / `DB_NAME` are the values from `.env.prod`. On a hosted database, run the same query through your own
`psql` connection.) Then correct it. `reset_prices` deletes the prices stored under the wrong listing:

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

Expect a table with `SYMBOL EXCHANGE PROVIDER_SYMBOL STORED PROVIDER ACTION`, and exactly these nine rows marked
`mismatch (dry run, use --apply)` with `STORED MYR` and `PROVIDER USD`:

```
CCJ  CHA  IBKR  NVDA  NVO  SOUN  SPCX  VUAA  VWRA
```

Every other row should say `ok`: the Bursa securities (MYR) and RBOT (already USD after step 4.1). Crypto is not listed
(it is not priced through Yahoo; BTC, ETH and SUI stay MYR). The last line reads `reconcile (dry run): mismatches=9
applied=0 failed=0`.

If something differs, stop and look before applying:

- **A different set of symbols** (extra mismatches, or one of the nine missing): the security is mapped to the wrong
  listing or Yahoo reports a different currency. Fix its `provider_symbol` / `exchange` with the PATCH above, then repeat
  the dry run.
- **`fetch failed`**: a Yahoo error for that symbol. Re-run; failures never stop the other rows, and the exit code is 1.
- **RBOT still shows a mismatch**: step 4.1 was not applied.

Then apply. Each mismatch is PATCHed with its provider currency and `reset_prices: true`, which deletes that security's
(wrongly converted) prices in the same transaction:

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle reconcile --apply
```

Expect nine `updated, prices_deleted=<n>` rows and `reconcile (apply): mismatches=9 applied=9 failed=0`. Re-running the
dry run now shows no mismatches.

### 4.3 Backfill FX and price history

This fetches daily USD/MYR rates and the native price history from Yahoo and posts them. FX is always done before prices.
Optionally rehearse first with `--dry-run` (it fetches and counts but posts nothing):

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle backfill --since 2025-01-01 --dry-run
```

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle backfill --since 2025-01-01
```

Expect an `FX PAIR` table (at least `USDMYR`, status `ok`, with `FETCHED` and `RECORDED` roughly 500 or more daily
bars), then a `SYMBOL CURRENCY FETCHED RECORDED REJECTED STATUS` table with the nine USD securities and RBOT, then totals
lines. `REJECTED` must be 0 for every row. Exit code 0.

- **`rejected` greater than 0 or `currency_mismatch`**: the security's stored currency still differs from Yahoo's; re-run
  `reconcile`.
- **`fetch failed` rows**: Yahoo hiccups; re-run just those with `--symbols CCJ,NVDA` (add `--prices`).
- Re-running is safe: rates and prices are upserted.
- Options: `--fx` or `--prices` alone, `--until YYYY-MM-DD` (default today UTC), `--symbols A,B`.

Crypto and Bursa securities are not touched: their stored prices are already in MYR.

## 5. Import the IBKR statements

The import is explained in full in [ibkr-import.md](ibkr-import.md); this section is the order of operations for your
data.

### 5.1 Back up again, find the ids

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod run --rm --no-deps \
  -e BACKUP_LABEL=pre-ibkr-import -e BACKUP_SKIP_RECEIPTS=true --entrypoint /backup.sh backup
```

List your accounts and note three ids: the IBKR **investment** account, and the old **IBKR Cash** account (currently
labelled MYR):

```bash
curl -sS "$API_URL/api/v1/accounts?page_size=100" -H "Authorization: Bearer $TOKEN" \
  | jq -r '.data[] | [.id,.type,.currency,.name,.is_active] | @tsv'
```

```bash
export IBKR_INVESTMENT_ACCOUNT_ID=<id of the IBKR investment account>
export OLD_IBKR_CASH_ACCOUNT_ID=<id of the old IBKR Cash account>
```

(SQL alternative: `SELECT id, type, currency, name FROM accounts WHERE deleted_at IS NULL ORDER BY name;`.)

### 5.2 Create the `IBKR Cash (USD)` account

A cash account in USD with a zero opening balance. The import writes the real opening balance itself (the 2025 statement's
Ending Cash on 2026-01-01). You can also create it in the web app (Accounts > New account > Cash, currency USD).

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

```bash
curl -sS -X POST "$API_URL/api/v1/imports/ibkr" \
  -H "Authorization: Bearer $TOKEN" \
  -F files=@<STATEMENT_2025.csv> \
  -F files=@<STATEMENT_2026.csv> \
  -F files=@<STATEMENT_2026_TO_TODAY.csv> \
  -F investment_account_id=$IBKR_INVESTMENT_ACCOUNT_ID \
  -F cash_account_id=$IBKR_USD_CASH_ACCOUNT_ID \
  -F cash_start_date=2026-01-01 \
  -F legacy_cash_account_id=$OLD_IBKR_CASH_ACCOUNT_ID \
  > import-report.json
```

Use one `-F files=@...` per CSV (1 to 10, in any order; each at most `MAX_UPLOAD_BYTES`, default 10 MiB). Without
`apply=true` nothing is written. Read the report:

```bash
jq '{applied, base_currency, periods, errors, warnings}' import-report.json
```

```bash
jq -r '.securities[] | .symbol as $s | .checkpoints[] | [$s, .date, .ibkr_quantity, .quantity, .quantity_ok, .cost_basis_diff] | @tsv' import-report.json
```

```bash
jq '.cash | {opening_balance, deposits_matched, unmatched_deposits, checkpoints}' import-report.json
```

What you should see:

- `applied: false`, `base_currency: "USD"`, `periods` contiguous from 2025-01-01 to today, `errors: []`.
- Every security checkpoint has `quantity_ok: true`. `cost_basis_diff` may be non-zero: Kuberan uses average cost, IBKR
  uses lots, so this is only a warning (expect it for partly sold positions such as NVO).
- `cash.opening_balance` equal to the 2025 statement's Ending Cash (in cents).
- `deposits_matched` lists your bank-to-IBKR MYR transfers (for example the 2026-06-04 USD 7,461.52 wire) that will be
  re-pointed at the new account, with `source_amount` (MYR) and `amount` (USD). A transfer is matched only within 5 days
  of the deposit and when its implied MYR/USD rate is within 3% of the market rate for that day.
- `cash.checkpoints`: `ledger` equals `ibkr` at each statement end, within one cent per row. The final one is the cash
  balance you should see after the import (IBKR's Ending Cash on the last statement).

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
| `unmatched_deposits` | Deposits or withdrawals with no matching Kuberan transfer (each lists rejected candidates and their rate deviation). Applying is allowed; record them by hand afterwards. The cash ledger leaves them out, so the final cash checkpoint shows them as `diff`. |
| non-empty `errors` of any kind | The apply is refused with HTTP 400 `IMPORT_INVALID` (report in `error.details.report`). Fix and re-run the dry run. |

### 5.4 Apply

Repeat exactly the same command with `-F apply=true` (and `-F deactivate_legacy_cash_account=true` to retire the old
MYR IBKR Cash account in the same step). It runs in one database transaction; any error rolls everything back.

```bash
curl -sS -X POST "$API_URL/api/v1/imports/ibkr" \
  -H "Authorization: Bearer $TOKEN" \
  -F files=@<STATEMENT_2025.csv> \
  -F files=@<STATEMENT_2026.csv> \
  -F files=@<STATEMENT_2026_TO_TODAY.csv> \
  -F investment_account_id=$IBKR_INVESTMENT_ACCOUNT_ID \
  -F cash_account_id=$IBKR_USD_CASH_ACCOUNT_ID \
  -F cash_start_date=2026-01-01 \
  -F legacy_cash_account_id=$OLD_IBKR_CASH_ACCOUNT_ID \
  -F deactivate_legacy_cash_account=true \
  -F apply=true \
  > import-applied.json
```

```bash
jq '{applied, errors, legacy_balance_after: .cash.legacy_account_balance_after}' import-applied.json
```

Expect `applied: true`, `errors: []`, and `legacy_balance_after` equal to what is left on the old account (0 if it only
held the IBKR deposits). A `409 IMPORT_IN_PROGRESS` means another apply is running; wait and retry. Re-running the same
apply is safe: it replaces its own rows (found by `import_ref`) and changes nothing.

### 5.5 Deactivate the legacy cash account

If you did not pass `deactivate_legacy_cash_account=true`, deactivate the old MYR `IBKR Cash` account now so it stops
showing up in totals (check first that its balance is 0 or only contains what you want to keep):

```bash
curl -sS -X PUT "$API_URL/api/v1/accounts/$OLD_IBKR_CASH_ACCOUNT_ID" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"is_active":false}' | jq '.account | {name, is_active, balance}'
```

Record any `unmatched_deposits` by hand as transfers from the right MYR bank account to `IBKR Cash (USD)` with the USD
amount in `to_amount` (web app: Transactions > New transfer, which asks for the second amount).

## 6. Resume the oracle and check the result

### 6.1 Take one manual oracle run, then resume the schedule

Run the oracle once by hand and watch it. It posts native prices and FX rates, computes snapshots and processes fixed
deposits:

```bash
docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle
```

Expect the summary with zero fetch errors and no `currency_mismatch` rejections (a rejection means a security is still
labelled with the wrong currency). Then restore the cron schedule:

```bash
sudo crontab ~/crontab.before-multicurrency
```

```bash
sudo crontab -l | grep -n oracle
```

Expect the oracle line back without the `#PAUSED` prefix.

### 6.2 Post-deploy checks

1. **Portfolio vs IBKR.** Compare each holding's quantity with the Open Positions of the latest statement and look at the
   portfolio totals:

   ```bash
   curl -sS "$API_URL/api/v1/investments?page_size=100" -H "Authorization: Bearer $TOKEN" \
     | jq -r '.data[] | [.security.symbol, .quantity, .cost_basis, .fx_missing] | @tsv'
   ```

   ```bash
   curl -sS "$API_URL/api/v1/investments/portfolio" -H "Authorization: Bearer $TOKEN" \
     | jq '.portfolio | {currency, total_value, total_cost_basis, total_gain_loss, total_price_return, total_fx_return, fx_stale, unconverted}'
   ```

   Quantities must equal IBKR's. `currency` is `MYR`, `unconverted` is empty, and `total_price_return + total_fx_return`
   equals `total_gain_loss` within a cent per holding.
2. **USD cash vs IBKR cash.** The `IBKR Cash (USD)` balance must equal the Ending Cash of the last statement, less any
   `unmatched_deposits` you have not recorded yet:

   ```bash
   curl -sS "$API_URL/api/v1/accounts/$IBKR_USD_CASH_ACCOUNT_ID" -H "Authorization: Bearer $TOKEN" \
     | jq '.account | {name, currency, balance, balance_base, fx_missing}'
   ```
3. **Net worth vs the previous snapshot.** The new total should move only by the difference between the old fetch-time
   rates and the backfilled daily rates, plus the 2026 cash figures the import corrected:

   ```bash
   curl -sS "$API_URL/api/v1/investments/snapshots?from_date=$(date -u -d '3 days ago' +%F)&page_size=5" \
     -H "Authorization: Bearer $TOKEN" | jq '.data[] | {recorded_at, total_net_worth, currency, fx_missing}'
   ```

   Snapshots come newest first. Compare the newest ones (`currency: "MYR"`, `fx_missing: false`) with the last pre-deploy snapshot (widen `from_date` to reach it). Old snapshots
   stay as recorded.
4. **`fx_missing` flags.** None of these should be true: the portfolio above, the accounts list
   (`jq '.data[] | select(.fx_missing==true) | .name'` on `/accounts?page_size=100`), and the latest snapshot. If one
   is, look up the pair and backfill it:

   ```bash
   curl -sS "$API_URL/api/v1/fx-rates/latest?base=USD&quote=MYR" -H "Authorization: Bearer $TOKEN" | jq .
   ```

   A 503 `FX_RATE_UNAVAILABLE` means there is no rate: run `oracle backfill --fx --since 2025-01-01`, then one normal
   oracle run. `fx_stale: true` only means the newest quote is older than `FX_MAX_STALENESS` (default 168h); the next
   oracle run refreshes it.
5. **Web app.** Open the dashboard, Investments and an IBKR holding: USD holdings show the native amount with `≈ RM`
   beside it, and the price/FX return split appears on the holding.

Delete the statement files and `import-*.json` reports from the server when you are done.

## 7. Rollback

The migrations only add columns and tables, so the previous images still work against the new schema until you run the
data steps. Choose the smallest rollback that fits:

1. **Before step 4 (nothing but the deploy ran).** Roll the images back as described in
   [backup-and-dr.md](backup-and-dr.md#rollback), or `git checkout <previous-sha> && ./deploy/deploy.sh`. The new
   columns stay and are harmless. To drop them as well, step the migrations back (nine of them, 46 down to 38):

   ```bash
   docker compose -f docker-compose.prod.yml --env-file .env.prod \
     run --rm --no-deps -e DB_PORT=5432 --entrypoint /migrate api down 9
   ```

   Use `down 1` to undo only the newest one. Down files exist for all nine, but they drop the columns and any data in
   them.
2. **After the data steps (4.1 onward).** These are not reversible by a down migration: the security relabelling and price
   deletion, the backfilled rates and prices, and the import (it replaces the IBKR investment transactions and rewrites the
   cash ledger and re-points transfers). Restore the backup you took in 2.2 (or 5.1 to keep the migration and the
   backfill): stop `api`, `mcp` and `bot`, restore into a fresh database as in
   [backup-and-dr.md section 4](backup-and-dr.md#4--restore-drill-do-this-once-before-relying-on-it), point `DB_NAME` at it,
   and start the services. Old images against a restored pre-migration dump need no migration step.
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
| Reporting currency | `GET/PATCH /api/v1/profile` (`base_currency`) |
| Env vars | `FX_MAX_STALENESS` (api, default 168h), `PUBLISH_FX_RATES` (oracle), `MAX_UPLOAD_BYTES` (api, default 10 MiB), `MCP_MAX_WRITE_AMOUNT` (now compared in the base currency); `TARGET_CURRENCY` removed |
| Error codes | `FX_RATE_UNAVAILABLE` (503), `FX_AMOUNT_REQUIRED` (400, with a suggested amount), `SECURITY_CURRENCY_LOCKED` (409), `IMPORT_INVALID` (400), `IMPORT_IN_PROGRESS` (409) |
