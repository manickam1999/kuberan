# Plan 024 — Phase 6 spec: MCP, Telegram bot, docs

The last phase. Background: `024-multi-currency.md` D9, plus CLAUDE.md "Monetary Values" (the API contract after Phases 1–5).

## M — MCP server (`apps/api/internal/mcp`)

### Formatting
- Replace `formatCents(cents)` (which hard-codes `$`) with `formatMoney(cents int64, currency string) string`. Output an
  unambiguous ISO-code form such as `MYR 1,234.56` / `USD -12.00` (code + space + grouped amount; negatives as `-`). Models
  read ISO codes reliably, and `$` is ambiguous. Delete `formatCents` once nothing uses it.
- **Every** amount a tool prints carries its currency, e.g. `USD 1,866.40 (≈ MYR 7,571.10)`. Never print a bare number for money.
  A total that was converted says so (`total in MYR`), and a response with `fx_missing` / `unconverted` prints a line like
  `Note: excludes USD 120.00 (no exchange rate)`.

### Read tools
| Tool | What it prints |
|---|---|
| `list_accounts` | Each balance in the account currency, with `≈ base` when it differs (`balance_base`; "no rate" when absent). Add a final `Net worth (MYR)` line using base figures only. |
| `list_transactions` | Each amount in its account's currency. Transfers: `MYR 4,460.00 → USD 1,100.00 (rate 0.2466)` when `to_amount` is set. Also fix the blank account name: `GetUserTransactions` doesn't preload `Account`/`ToAccount`. Preload both and print `From → To` for transfers. |
| `get_portfolio` | Totals in base. Add price return, FX return, `unconverted`. Holdings list: native price/value plus base value and base unrealized P&L (same formulas as the web: `quantity × current_price − cost_basis_base`). |
| `get_net_worth_history` (snapshots) | Each point in its snapshot `currency`. Note a currency change and `fx_missing` points. |
| `get_monthly_summary`, `get_daily_spending`, `get_daily_summary`, `get_spending_by_category`, `get_top_expenses` | In the response `currency`. Top expenses show `amount_base` plus the native amount when it differs. FX notes as above. |
| `list_budgets`, `get_budget_progress` | In `budget.currency`. |
| `list_receivables`, `get_receivables_summary` | Per-receivable `currency`. The summary total in base plus `by_currency`. |

### Write tools
- **The amount unit is the target account's currency.** Every write-tool description must say so explicitly (e.g. "amount: in the
  account's own currency, as a decimal like 12.50").
- **The ceiling (`MCP_MAX_WRITE_AMOUNT`, cents) is evaluated in the user's base currency.** Convert each amount being written (at the
  current rate, via FXServicer) before comparing. **No rate → reject** with a clear message. Same-currency amounts are compared
  directly (no FX query). Update `checkAmount`/the guard in `write.go` and its tests.
- **`create_transfer`:** add optional `to_amount` (dollars, destination currency). Same rules as the API: required across currencies
  (if missing, the error text includes the API's suggested amount and rate), omitted or equal otherwise. Both `amount` and
  `to_amount` count against the ceiling (converted). Remove the Phase 3a "not supported over MCP" rejection.
- **`record_repayment`:** add optional `account_amount` (the target account's currency), same pattern. Remove the Phase 4 rejection.
- The rate limiter's "records changed" accounting is unchanged.
- Tests: formatting (MYR, USD, negative, grouping); each read tool prints currencies (at least accounts, transactions incl. a
  cross-currency transfer, portfolio, one analytics tool, receivables); the ceiling in base (USD under / over the ceiling after
  conversion, missing rate rejected, same currency no FX call); cross-currency `create_transfer` and `record_repayment` (missing →
  error with suggestion, given → both balances right).

## B — Telegram bot (`apps/bot`, Python)
- `utils/formatting.format_currency(amount_cents, currency)`: **currency required** (no default), formatted like the web
  (`RM 1,234.50`, `US$1,234.50`; use a small symbol map for common codes, else `CODE 1,234.50`). Update the tests.
- **Base currency:** the bot today uses the link's `default_currency` as the display currency. Totals must use the user's
  **`base_currency`** from `GET /api/v1/profile` (`UserAPIClient.get_profile` exists). Fetch it per command (or cache briefly in
  `user_data` with a short TTL). `default_currency` stays only as the default for **new accounts** created by the bot.
- `/balance` (`handlers/balance.py`): each account in its own currency. The total is the sum of **`balance_base`** in the base currency,
  never the raw balances. List accounts without `balance_base` (no rate) as "not included in total". Remove the "summed naively" comment.
- `/accounts`: each account in its currency (and the credit limit in the account's currency).
- `/budgets`: format in each budget's `currency` (progress responses carry it).
- `/owed`: the summary total in `summary.currency`, per-counterparty in base, plus a `by_currency` line when there's more than one currency.
- `/summary` (monthly summary): in the response `currency`.
- Transaction flow: when the bot creates an expense in an account, confirm the amount in **that account's** currency (it currently
  may format with the default).
- Tests: update the existing ones and add cases for a mixed MYR+USD `/balance` (the total uses balance_base; a missing-rate account is
  excluded and listed), budgets in USD, owed with `by_currency`. Run `pytest` (the venv command is given in the task brief).

## D — Docs (after M and B)
- `docs/multi-currency.md`: the **operator guide and deploy runbook** in one place. It's what the owner follows on deploy day:
  1. what changed (short);
  2. pre-deploy: back up the DB; export IBKR Activity Statements (2025, 2026-to-date, and one from the last statement's end to today);
  3. deploy; migrations 38–46 run;
  4. the Phase 2 runbook (RBOT PATCH, `oracle reconcile` dry run then `--apply`, `oracle backfill --since 2025-01-01`) — copy it from
     `plans/024-multi-currency/phase-2-spec.md` (Production runbook + appended Phase 3b steps) and correct anything that no longer
     matches the code;
  5. the IBKR import (create the `IBKR Cash (USD)` account, dry run with all statements, read the report, apply, deactivate the legacy
     cash account);
  6. post-deploy checks (portfolio vs IBKR positions, USD cash vs IBKR cash, net worth vs the previous snapshot, the `fx_missing` flags);
  7. rollback (restore the backup; the migrations' down files exist, but data steps aren't reversible).
- `CLAUDE.md`: an MCP note (currency-tagged output, the ceiling in base, cross-currency write params) and a bot note; link
  `docs/multi-currency.md` in the docs list.
- `plans/024-multi-currency/024-multi-currency.md`: set **Status: implemented**, with a short "where the code differs from the plan"
  list (phase re-cut 3a/3b/4; `current_price` stays base; transfers are single rows; aggregates per D7; etc.). Follow the style of
  `plans/022-mcp-write-access/022-mcp-write-access.md`'s status header.
