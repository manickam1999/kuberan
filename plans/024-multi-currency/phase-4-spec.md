# Plan 024 — Phase 4 spec: cross-currency aggregates

Contract for Phase 4 (re-cut in `phase-3-spec.md`): every total that adds money across accounts converts it
to a single currency explicitly. Background: `024-multi-currency.md` D7 (the table) and Q5 (convert at each
transaction's own date). Phases 1–3b are merged: `FXServicer` (`Rate`, `Rates`, `ListRates`),
`holding_valuation.go` (`valuer`, at one instant), native trade amounts, and transfers with `to_amount`.

**Rules for every aggregate below**
- The target currency is the user's `base_currency` unless stated otherwise.
- **Fast path:** if every currency involved equals the target, no FX query is made (most users, and every
  query that only touches MYR accounts). This must be covered by a test with a nil FX service.
- A missing rate never falls back to 1:1. The affected amounts are left out of the converted total, and the response says
  so (`fx_missing: true`, plus `unconverted: [{currency, amount}]` where the response shape allows it).
- Responses gain `currency` (the currency their amounts are in).
- Amounts stay int64 minor units. Conversion goes through `fx.Convert`. **Group first, convert once per group** (never per
  row), so rounding is stable and the queries stay cheap.

## A. Dated conversion helper (owner: agent B)
Analytics need a rate **per day** over a range, not at one instant.
- Add `RatesByDay(pairs []CurrencyPair, start, end time.Time) (map[CurrencyPair]map[string]*FXQuote, error)` to
  `FXServicer`. Days are UTC `YYYY-MM-DD`, start..end inclusive. Each day's quote is the one `Rate(pair, endOfDay)` would
  return (direct, then inverse, then USD pivot; latest ≤ end of that day; carry forward over weekends/holidays; staleness
  measured at end of day). Implementation: one bounded query for all needed legs' rows in [start, end], plus a carry-in per
  leg (the latest row before start, reusing `loadLegs`). Then resolve each day in Go with the existing `resolve`. It must not
  run one query per day.
- `datedValuer` in `internal/services/holding_valuation.go` (or a sibling file): built from `RatesByDay`; `Convert(amount,
  currency, day string) (int64, ok)`.
- Tests: carry-forward over a gap, inverse and pivot per day, a range with no rows before start (days before the first
  rate are missing), the batch result equals per-day `Rate` for a random sample, and a query-count bound (assert ≤ a small
  constant number of queries for a 366-day range).

## B. Analytics (owner: agent B)
`period_totals.go` (`periodTotals`, behind monthly summary, daily spending and daily summary), `totalsByCategory`
(spending/income by category) and `GetTopExpenses` in `transaction_service.go`:
- Join `accounts` and group by **(UTC day, account currency, type[, category])** in SQL. Then convert each group with
  `datedValuer` at its day, and roll the converted values up to the response's buckets (day or month) in Go. Keep the
  existing Postgres/SQLite bucket-expression split, now always at day granularity before rollup.
- Monthly figures are the sum of daily-converted amounts (Q5: per-transaction date).
- `effectiveAmountSQL` (`COALESCE(personal_amount, amount)` for expenses) still applies before conversion.
- `GetTopExpenses`: rank by the **converted** amount. Each item keeps its native `amount` and gains `amount_base` and
  `currency` (the account's currency).
- Each response type gains `currency` (base) and `fx_missing`.
- Transfers stay excluded, as today.

## C. Budgets (owner: agent B)
- Migration `000045_add_currency_to_budgets`: `ADD COLUMN currency VARCHAR(3)`; `UPDATE budgets b SET currency =
  u.base_currency FROM users u WHERE u.id = b.user_id`; `SET NOT NULL`. The down migration drops it. Model `Budget.Currency`
  (`size:3;not null`, json `currency`).
- Create: `currency` is optional (`iso4217`), defaulting to the user's base. Update may change it.
- Progress (`GetBudgetProgress`, `GetActiveBudgetsProgress`): spending in the budget's category/period, converted to the
  **budget's** currency at each transaction's date (via `datedValuer`, grouped by day + account currency). Response gains
  `currency` and `fx_missing`.

## D. Accounts (owner: agent A)
`GetUserAccounts` and `GetAccountByID` responses: each account gains response-only `balance_base` (int64, the balance
converted to the user's base **now**), `fx_rate` (decimal string, `""` when same currency), `fx_stale`, `fx_missing`. Use
the Phase 2 `valuer` with one batched rate lookup per request. Investment-account balances are already in the account's
currency (Phase 2), so they convert the same way. A paginated list adds `totals_base` by account type in its response
**only if** the existing response wrapper allows extra fields without breaking clients (otherwise skip it and say so).

## E. Snapshots (owner: agent A)
`computeAllSnapshots`: the balance query groups by `(user_id, type, currency)`. Each group is converted into that user's
base at **`recordedAt`** (one `Rates` call per distinct base, combined with the Phase 2 holdings conversion). A group with
no rate is skipped and the snapshot gets `fx_missing = true`. Liability types (debt, credit card) convert the same way
before subtraction. Fixed deposits are converted like cash.

## F. Receivables (owner: agent A)
- A receivable's currency is the currency of the account of the expense transaction that created it. Add response-only
  `currency` to receivables and settlements (load it efficiently with a join in list/get; no N+1).
- **Settle** into an account whose currency differs from the receivable's: the request gains optional `account_amount`
  (> 0, in the account's currency). It's required when the currencies differ, otherwise
  `fxAmountRequiredError(..., field: "account_amount")`. The settlement row keeps `amount` in the receivable's currency (it
  reduces the outstanding). The reimbursement transaction in the account uses `account_amount`. Same-currency: `account_amount`
  must be omitted or equal. Store `account_amount` on the settlement (migration `000046_add_account_amount_to_settlements`,
  nullable BIGINT, `CHECK > 0`) so deleting a settlement reverses the account by the right amount. Check every settlement
  reversal path (`DeleteSettlement`, any cascade from deleting the expense) and list them in the report.
- `Summary`: outstanding totals converted to base **now**. Add `by_currency: [{currency, outstanding}]` (native) and
  `fx_missing`. Per-counterparty figures are converted the same way if present.
- The MCP receivable write tools (`record_repayment`, `write_off_receivable`): a cross-currency `record_repayment` is
  rejected with a clear message (as for transfers; full MCP support is Phase 6).

## Tests (both agents, matching existing styles)
Each section needs:
- the single-currency fast path, asserting no FX query (nil FX service);
- a mixed MYR + USD case with hand-computed expected values;
- a missing-rate case, which must not count 1:1.

Plus:
- Analytics: a monthly total equals the sum of daily conversions, not one month-end conversion. Use two USD expenses on days
  with different rates.
- Snapshots: a USD cash account and a USD credit card for a MYR user at a past `recordedAt`.
- Receivables: a cross-currency settle, then deleting the settlement restores the account balance exactly.
- Integration test (written by a separate follow-up task after agents A and B finish): a MYR user with MYR and USD cash, USD expenses on
  two dates, a USD budget and a MYR budget, and a USD receivable settled into the MYR account. Check monthly summary,
  spending by category, budget progress, accounts `balance_base`, a computed snapshot and the receivables summary.

## Docs
CLAUDE.md: one short paragraph on "aggregates convert to base; `fx_missing`/`unconverted`; analytics convert per transaction
day".
