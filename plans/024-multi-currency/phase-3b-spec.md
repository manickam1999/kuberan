# Plan 024 — Phase 3b spec: IBKR Activity Statement import

The contract for Phase 3b (see `024-multi-currency.md` D10.0, D10.0a, D10 steps 4–7, and `phase-3-spec.md`).
Phase 3a is merged first: native trade amounts, `_base` fields, `holdingState.apply` / `rebuildInvestment`
(`internal/services/investment_math.go`), transfers with `to_amount`, and `fxAmountRequiredError`.

**Goal:** one endpoint that takes the owner's IBKR Activity Statement CSVs and rebuilds the IBKR holdings,
and the USD cash ledger from a chosen start date, so Kuberan matches the statements. It runs as a **dry run by
default**, and is applied atomically when asked. It's general for any IBKR account, but the acceptance test is the
owner's real data (below).

## Inputs and privacy
- Real statements: the owner's 2025 and 2026 Activity Statements, uploaded to the development session (development only, **never commit, never copy into the repo**; their paths are given to agents in the task brief, not recorded here).
- Test fixtures under `internal/importers/ibkr/testdata/` must be **anonymised** copies: replace the name, account
  number and addresses. Keep every number. Use a fake account like `U00000000` and the name `Test User`. Grep the fixtures
  for the real name and account id before finishing.
- The endpoint processes uploads **in memory only**. Nothing is written to disk or storage, and neither file contents nor
  PII are logged (log counts only).

## Endpoint
`POST /api/v1/imports/ibkr` (protected, JWT). `multipart/form-data`:

| field | required | meaning |
|---|---|---|
| `files` | yes, 1–10 | Activity Statement CSVs (each ≤ `MAX_UPLOAD_BYTES`) |
| `investment_account_id` | yes | the user's investment account the holdings belong to |
| `cash_account_id` | yes | a **cash** account in the statement's base currency (USD) that receives the IBKR cash ledger |
| `cash_start_date` | yes | `YYYY-MM-DD`; cash legs are only created for events on/after this date |
| `legacy_cash_account_id` | no | the old IBKR cash account (MYR) whose incoming transfers get re-pointed (see Deposits) |
| `deactivate_legacy_cash_account` | no, default false | set `is_active=false` on the legacy account after apply |
| `apply` | no, default false | `true` writes; otherwise a dry run (same report, nothing written) |

Response `200` with the **report** (below) for both modes. Validation and ownership errors use the usual AppErrors.
Audit log `IMPORT_IBKR` on apply, with counts only. Rate: one apply at a time per user (a simple in-process mutex keyed
by user is enough).

## Package layout
- `internal/importers/ibkr`: a **pure parser** (no DB). `Parse(r io.Reader) (*Statement, error)`. It reads the
  multi-section CSV (column 1 = section, column 2 = `Header`/`Data`/`SubTotal`/`Total`; each section has its own header row,
  and a section can have **several header rows**: re-key on each `Header`). It maps columns **by header name, never by
  position**. Exact decimals use `math/big.Rat` (never float) for money. Quantities are float64, rounded to 10 dp on parse.
- `internal/services/import_service.go`: `ImportServicer.ImportIBKR(userID string, p IBKRImportParams, files []io.Reader) (*IBKRImportReport, error)`.
  It turns statements into events, validates, and applies. Handler `internal/handlers/import_handler.go`.

## Parsing → events (per D10.0a; both real files must parse with zero unknown-row errors)
Statement metadata: `Statement/Period` (start and end dates), `Account Information/Base Currency`.
Multiple files: **sorted by period**, they must be contiguous (next start = previous end + 1 day), all for the same account
id and base currency. Otherwise error.

| Section / rows | Event |
|---|---|
| `Trades`, DataDiscriminator `Order`, Asset Category `Stocks` | buy (Quantity > 0) / sell (< 0). `price_per_unit` = round(T. Price × 100). `fee` = round(abs(Comm/Fee) × 100). Buy `total_amount` = round((abs(Proceeds) + abs(Comm/Fee)) × 100); sell `total_amount` = round((Proceeds − abs(Comm/Fee)) × 100), each a single rounding of the exact decimal. Time = `Date/Time` in **America/New_York**. Rows of other asset categories (Forex, Options...) → error "unsupported". |
| `Dividends` (Data, not Total) | summed per (symbol, date) into one dividend with gross = Σ amount. Description patterns: `Cash Dividend`, `Payment in Lieu of Dividend`, `- Reversal`. A sum ≤ 0 → no event (fully reversed). Symbol = the text before `(`. |
| `Withholding Tax` | summed per (symbol, date) → added to that dividend's `fee` (abs). No matching dividend → a standalone expense. |
| `Fees` / `Other Fees`, description containing `Cash Dividend` | added to the dividend's `fee` for (symbol, date). |
| `Fees`, other descriptions (e.g. `ADR Fee`) | a standalone **expense** on the cash account (if on/after cash start), description = IBKR's. |
| `Grant Activity`, "Stock Award Grant for Cash Deposit" | buy, no funding, qty, `total_amount` = round(Value × 100), price = round(Price × 100). Date = Report Date. |
| `Grant Activity`, "Stock Award Return for Cash Withdrawal" | sell, no deposit, qty = abs, total = round(abs(Value) × 100). |
| `Grant Activity`, "Stock Award Vesting" | **ignored** (the shares are already counted from the grant). |
| `Grant Activity`, "Stock Award Withholding" | sell, no deposit, qty = abs, total = round(abs(Value) × 100), note "Stock award tax withholding". |
| `Corporate Actions`, description matching `Split (\d+) for (\d+)` | split, ratio N/M, time = `Date/Time`. Any other corporate action → error naming the row. |
| `Deposits & Withdrawals` (Data rows with a currency, not Total) | deposit (> 0) / withdrawal (< 0), cash only. |
| `Open Positions`, DataDiscriminator `Summary` | checkpoint: symbol → quantity, cost basis (native), at the statement end date. `Lot` rows are ignored. |
| `Cash Report`, `Ending Cash`, `Base Currency Summary` | checkpoint: cash at the statement end date. |
| `Financial Instrument Information` | symbol → listing exchange, ISIN, description (for matching securities). |
| `Statement`, `Account Information`, `Net Asset Value`, `Change in NAV`, `Mark-to-Market…`, `Realized & Unrealized…`, `Change in Dividend Accruals`, `Codes`, `Notes/Legal Notes`, `Total P/L…` | ignored; listed in the report's `ignored_sections` |
| any other section with Data rows | error "unsupported section X" (fail loudly rather than silently dropping money) |

Event ordering: by time, where date-only events take 00:00 New York time on that date. Ties are broken by section order
(grant/split before trade before dividend/fee/withholding before deposit), then file order.

## Matching securities and investments
- IBKR listing exchange → Kuberan exchange: `LSEETF`/`LSE` → `LSE`, `NASDAQ` → `NASDAQ`, `NYSE` → `NYSE`; other values are
  passed through as-is.
- Find the security by (symbol, exchange), case-insensitive. If there's no match, use the symbol alone **only if exactly one**
  security has it. Otherwise it's an error in the report (`unknown_security`). Never create securities.
- The security's currency must equal the trade currency (USD). Otherwise `currency_mismatch` (the Phase 2 runbook
  reconcile must run first).
- The investment is the user's investment for that security **in `investment_account_id`**. If none exists, one is created on
  apply (it's possible for symbols only held in the past).

## Apply (all in ONE DB transaction; any error rolls everything back)
For each symbol in the statements:
1. Soft-delete all existing `investment_transactions` of that investment.
2. Insert the new rows (`fx_rate_to_base` and `base_amount` from `FXServicer.Rate(USD, user base, date)`. **A missing rate
   is an error**, telling the operator to run `oracle backfill --fx` first).
3. `rebuildInvestment`.
Then the cash ledger, for events on/after `cash_start_date`, into `cash_account_id` (which must be a cash account whose
currency equals the statement base currency):
- **Opening balance:** a `opening_balance` transaction dated `cash_start_date` for the `Ending Cash` checkpoint of the statement
  ending the day before `cash_start_date` (error if no such statement). If the account already has an opening balance, it is
  replaced (soft-delete the old one and adjust the balance).
- Each trade on/after start: a cash leg (buy = expense, sell = income) like `RecordBuy`/`RecordSell` create. Use the same
  description format, and the same `ApplyBalanceDelta`.
- Each dividend on/after start: income for net (gross − fee).
- Standalone fees/withholding: expense.
- **Deposits:** if `legacy_cash_account_id` is set, look for an existing **transfer** whose `to_account_id` is the legacy
  account, dated within ±5 days of the deposit and not already matched. Matching is one-to-one, nearest date first. If found,
  **re-point it**: `to_account_id = cash_account_id`, `to_amount` = the USD deposit (the source keeps its MYR amount). Reverse
  its effect on the legacy balance and apply it to the new one. If not found, it's an `unmatched_deposit` in the report (no
  transaction is created; the owner records it by hand). Withdrawals are likewise unmatched and reported.
- Running this twice must not duplicate cash rows. Cash legs created by the import carry a marker (e.g. a description prefix
  `IBKR import:` **and** a dedicated nullable column `import_ref VARCHAR(64)` on `transactions`, migration `000044`, holding
  `ibkr:<account>:<event key>`). A re-import first soft-deletes previous rows with that `import_ref` prefix for the account
  (reversing their balance effect).
- If `deactivate_legacy_cash_account` → set it inactive and report its remaining balance.

## Report (JSON; the same for the dry run and apply)
```json
{
  "applied": false,
  "account": "U…", "base_currency": "USD", "periods": [{"start":"2025-01-01","end":"2025-12-31"}, …],
  "ignored_sections": ["Codes", …],
  "securities": [{
    "symbol": "NVO", "security_id": "…", "investment_id": "…|null (will create)",
    "existing_transactions": 6, "new_transactions": 9,
    "final": {"quantity": 30.2548, "cost_basis": 182666, "cost_basis_base": 812345, "realized_gain_loss": …, "realized_gain_loss_base": …},
    "checkpoints": [{"date":"2025-12-31","ibkr_quantity":71.2295,"quantity":71.2295,"quantity_ok":true,"ibkr_cost_basis":396555,"cost_basis":396555,"cost_basis_diff":0}]
  }],
  "cash": {"opening_balance": 465, "legs": 23, "dividends": 9, "fees": 1,
           "deposits_matched": [{"date":"2026-06-04","amount":746152,"transaction_id":"…","source_amount":3300000}],
           "unmatched_deposits": [], "checkpoints": [{"date":"2026-11-02","ibkr":12055,"ledger":12055,"diff":0}],
           "legacy_account_balance_after": 0},
  "errors": [{"code":"unknown_security","detail":"…"}],
  "warnings": ["…"]
}
```
- **Apply is refused** (400 `IMPORT_INVALID`, with the report in `details`) if `errors` is non-empty, or any checkpoint has
  `quantity_ok: false` (quantity tolerance 1e-4, since IBKR prints 4 dp).
- Cost-basis and cash diffs are warnings, never errors. Cost basis differs because of FIFO vs average cost. Cash may be off by
  ≤ 1 cent per row; larger cash diffs are a warning naming the period.
- The checkpoint replay uses `holdingState.apply` up to each statement's end date (no DB writes in the dry run).

## Tests
- **Parser:** section/header re-keying, header-name mapping, Total/SubTotal skipped, exact decimals, every mapping row in the
  table above (small hand-written CSV snippets), unsupported section/asset/corporate action → error, multi-file contiguity
  and account checks.
- **The two anonymised real statements** (fixtures): the parse succeeds with no errors, and running the event build + checkpoint
  replay against an in-memory set of securities gives:
  - 2025-12-31: CCJ 30.0511, CHA 102.9936, IBKR 2.3791, NVO 71.2295, VUAA 28, VWRA 15; RBOT and SOUN 0.
  - 2026-11-02: CCJ 30.0511, CHA 0, IBKR 1.6706, NVDA 10.2137, NVO 30.2548, SPCX 10, VUAA 45.2, VWRA 37.
  - Every quantity is within 1e-4 of IBKR's.
  - The cash ledger from 2026-01-01 with opening 4.65 ends within a few cents of 120.55 (assert the exact value your
    implementation produces and that |diff| ≤ number of rows in cents).
- **Service (SQLite):** a dry run writes nothing (count the rows before and after); apply replaces legacy transactions; the
  re-import is idempotent (same counts, same balances); deposit re-pointing moves balance from the legacy account to the new one
  and sets `to_amount`; an unmatched deposit is reported; a missing FX rate → error; unknown security → apply refused; a
  quantity-checkpoint failure → apply refused.
- **Handler:** multipart parsing, the file-count/size limits, `apply` parsing, ownership errors.

## Docs
- CLAUDE.md: the endpoint.
- A short `docs/ibkr-import.md`: how to download the statements (Activity Statement → Annual/Custom → CSV), the curl
  invocation, dry run first, then apply.
- Add the production steps after the Phase 2 runbook in `phase-2-spec.md`'s runbook section: create the `IBKR Cash (USD)`
  account, dry-run the import with both statements, review, apply.
