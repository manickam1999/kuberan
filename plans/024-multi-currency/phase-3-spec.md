# Plan 024 — Phase 3 spec: native trades, funding and FX transfers

**Re-cut (2026-11-08):** the original Phase 3 is split in two, and cross-currency transfers (D6) move
here from Phase 4, because the IBKR import needs them (IBKR deposits are MYR→USD transfers).
- **Phase 3a (this spec):** native trade amounts, base-currency cost basis, cross-currency funding, FX
  transfers, and the replay function.
- **Phase 3b (separate spec, after 3a):** the IBKR Activity Statement import and the production data
  migration (D10.0a, D10 steps 4–7).
- **Phase 4** becomes analytics, budgets, receivables, account `balance_base`, and snapshots converting
  account balances.

**Deploy note:** nothing is deployed until all phases are done (the owner's decision), so Phase 3a may
change the meaning of existing investment JSON fields (`cost_basis` etc. become native). The web UI is
updated in Phase 5, before deploy. Phase 2's "`current_price` stays in base" rule is unchanged.

Background: `024-multi-currency.md` D5 and D6, plus Q2 (holding amounts are in the **security's**
currency). Phases 1–2 are merged (`internal/fx`, `FXServicer`, `holding_valuation.go`, native prices).

---

## Data model

### Migration `000042_native_investment_amounts`
```sql
ALTER TABLE investment_transactions
  ADD COLUMN fx_rate_to_base NUMERIC(20,10),                         -- security ccy -> user base, at trade date; NULL = legacy row
  ADD COLUMN base_amount BIGINT,                                     -- total_amount in base at that rate (frozen)
  ADD COLUMN realized_gain_loss_base BIGINT NOT NULL DEFAULT 0;
ALTER TABLE investments
  ADD COLUMN cost_basis_base BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN realized_gain_loss_base BIGINT NOT NULL DEFAULT 0;
-- Before this phase every amount was entered in the user's base currency (MYR in practice), so the
-- legacy base figures equal the stored ones.
UPDATE investments SET cost_basis_base = cost_basis, realized_gain_loss_base = realized_gain_loss;
UPDATE investment_transactions SET base_amount = total_amount, realized_gain_loss_base = realized_gain_loss
  WHERE base_amount IS NULL;
```
The down migration drops the five columns. Model fields: `InvestmentTransaction.FXRateToBase *fx.Rate`
(json `fx_rate_to_base`, a decimal string or null), `BaseAmount *int64`, `RealizedGainLossBase int64`.
`Investment.CostBasisBase int64`, `RealizedGainLossBase int64`. Response-only (`gorm:"-"`):
`InvestmentTransaction.Currency` (the security's currency) and `Investment.Currency` (same as `price_currency`).

### Migration `000043_add_to_amount_to_transactions`
`ALTER TABLE transactions ADD COLUMN to_amount BIGINT CHECK (to_amount IS NULL OR to_amount > 0);`
The down migration drops it. `Transaction.ToAmount *int64` (json `to_amount`). Response-only:
`ImpliedRate string` (json `implied_rate,omitempty`, decimal string `to_amount/amount` to 10 dp, half-even)
for cross-currency transfers.

### Errors
- `AppError` gains `Details map[string]any \`json:"details,omitempty"\``, and the error middleware must include
  it in the JSON response (check `internal/middleware` and `respondWithError`). Add `WithDetails(err, map)`,
  which returns a copy (never mutate the shared sentinel).
- `ErrFXAmountRequired`: `FX_AMOUNT_REQUIRED`, 400, "An amount in the other account's currency is required".
  Details: `{"currency": "MYR", "suggested_amount": 447120, "rate": "4.4712", "rate_recorded_at": "...", "field": "funding_amount"}`.
  If no market rate exists, the details carry only `currency` and `field`.
- `ErrTransferCurrencyMismatch` is no longer returned for transfers. Delete it if it's unused afterwards.

---

## Semantics

### Currencies
- An investment's amounts (`price_per_unit`, `total_amount`, `fee`, `cost_basis`, `realized_gain_loss` on both
  the holding and its transactions) are in the **security's currency** (`UPPER(securities.currency)`).
- `_base` amounts are in the user's `base_currency`.
- A cash/credit/debt account's transactions are in the **account's currency**. Account currency already can't be
  changed (no update path). Keep it that way.

### Trade FX rate (`fx_rate_to_base`), precedence
1. An explicit `fx_rate` in the request (decimal string, parsed with `fx.ParseRate`).
2. Else, if a funding/deposit account is used **and its currency is the user's base**, the rate implied by that
   cash leg: `funding_amount / total_amount` (half-even to 10 dp). It's what was actually paid, so it beats a
   market rate.
3. Else, if the security currency equals the base currency: `1`.
4. Else the market rate `FXServicer.Rate(securityCcy, base, date)` (staleness allowed).
5. Else `FX_RATE_UNAVAILABLE`, with a message telling the caller to pass `fx_rate`.

`base_amount = fx.Convert(total_amount, fx_rate_to_base)`.

### Buy (`RecordBuy`, and `AddInvestment`'s initial buy)
- Native, as today: `total_amount = lineAmount(qty, price) + fee`. `cost_basis += total_amount`, `quantity += qty`.
- Base: `cost_basis_base += base_amount`.
- Funding account (optional, cash only, as today), with currency F:
  - F == security currency → debit `total_amount` (as today). A `funding_amount`, if sent, must equal it (else 400).
  - F ≠ security currency → `funding_amount` (> 0, in F) is **required**, else `FX_AMOUNT_REQUIRED` with
    `field: "funding_amount"` and a suggestion from `Rate(securityCcy, F, date)` applied to `total_amount`. The cash
    transaction row and the balance delta use `funding_amount`.

### Sell (`RecordSell`)
- Native, as today: proceeds `total_amount = lineAmount(qty, price) − fee`. Removed cost =
  `round(cost_basis × qty / position)` (keep the existing formula and rounding), `realized_gain_loss = proceeds − removed`.
- Base: `removed_base = round(cost_basis_base × qty / position)` with the **same** rounding helper;
  `proceeds_base = base_amount`; `realized_gain_loss_base = proceeds_base − removed_base`. Selling the whole position
  removes exactly the remaining `cost_basis_base` (no residue), the same as the native side.
- Deposit account (optional): the same rules as funding, with `deposit_amount` / `field: "deposit_amount"`.

### Dividend (`RecordDividend`)
- New optional inputs: `fee` (≥ 0, native; withholding tax plus dividend fees), `deposit_account_id`, `deposit_amount`,
  `fx_rate`. `total_amount` = **gross** (native). `base_amount` = Convert(gross).
- With a deposit account: credit **net** (`total_amount − fee`), or `deposit_amount` when the currencies differ (that is
  the net in the account's currency). Without one: no cash movement (as today).
- Dividends don't touch cost basis or realized P&L. Validation: `fee <= total_amount`.

### Split
Unchanged. The base figures are unaffected.

### Service signatures
These methods now take many optional values, so use param structs (per CLAUDE.md), e.g.
`RecordBuy(userID, investmentID string, p BuyParams)`, `RecordSell(..., p SellParams)`,
`RecordDividend(..., p DividendParams)`, `AddInvestment(userID string, p AddInvestmentParams)`. Update the interface,
handlers, MCP (if it calls them) and every test.

### Handlers
Request bodies gain `fx_rate` (string, optional), `funding_amount` / `deposit_amount` (int64, optional, > 0), and for
dividends `fee`, `deposit_account_id`, `deposit_amount`. Swagger annotations are updated by hand (swag isn't installed).

### Portfolio (`GetPortfolio`)
Per open holding with a rate (identity counts), with `rate_now` = the Phase 2 valuer's quote:
- `value_base` (Phase 2) = Convert(value_native)
- `cost_basis_base` (stored)
- `price_return_base` = Convert(value_native − cost_basis) (i.e. the native gain at today's rate)
- `fx_return_base` = Convert(cost_basis) − cost_basis_base
- Invariant (assert in tests, allowing ±1 cent per holding for rounding): `price_return + fx_return = value_base − cost_basis_base`.

Summary changes:
- `total_cost_basis` = Σ `cost_basis_base` (the unit was already base; it's now correct for foreign holdings).
- New `total_price_return` and `total_fx_return`.
- `total_realized_gain_loss` = Σ `realized_gain_loss_base` over all positions.

Holdings with no rate stay in `unconverted` as in Phase 2. `HoldingsByType` is unchanged.

### FX transfers (`CreateTransfer`, update, delete)
- The request gains optional `to_amount` (int64 > 0, in the destination account's currency).
- Same currency: `to_amount` must be omitted or equal `amount` (else 400). Store NULL.
- Different currency: `to_amount` is required, else `FX_AMOUNT_REQUIRED` (`field: "to_amount"`, suggestion from
  `Rate(fromCcy, toCcy, date)` applied to `amount`).
- **Balances:** the source changes by `amount` as today. The destination changes by `effective_to_amount =
  COALESCE(to_amount, amount)`. Find **every** code path that applies, reverses or recomputes a transfer's effect on
  the destination: create, update, delete, any balance recompute or reconcile helper in `account_service.go` /
  `transaction_service.go`, fixed-deposit or receivable flows that create transfers, and the MCP write tool. Make them
  all use the effective amount. List them in your report.
- Update: if either side's account or `amount` changes on a cross-currency transfer, `to_amount` must be sent again
  (else `FX_AMOUNT_REQUIRED`). Changing a transfer so both accounts share a currency clears `to_amount`.
- Response: `to_amount` and `implied_rate`.
- **MCP `create_transfer`:** if the two accounts' currencies differ, reject with a clear message ("cross-currency
  transfers aren't supported over MCP yet"). Full MCP support is Phase 6.
- Analytics: transfers are already excluded from income/expense (verify; no change expected).

### Replay: `rebuildInvestment(tx *gorm.DB, investmentID string) error`
- Loads the investment's non-deleted transactions ordered by `date, created_at, id` and recomputes, from zero:
  `quantity`, `cost_basis`, `cost_basis_base`, `realized_gain_loss`, `realized_gain_loss_base` on the holding, and
  `realized_gain_loss` / `realized_gain_loss_base` on each sell row. It uses **exactly** the same helpers as the incremental
  path (extract them so both call one implementation). Splits multiply the quantity. Dividends contribute nothing.
- A sell that exceeds the replayed position → returns `ErrInsufficientQuantity` (or the existing equivalent) naming the
  transaction id. The caller's DB transaction rolls back.
- Not exposed over HTTP in 3a. Phase 3b's import uses it.
- **Property test:** for many random sequences of buys, sells (≤ position), splits and dividends with mixed fx rates,
  the incremental path and `rebuildInvestment` give identical holding and row values (use a fixed seed, about 200 sequences).

### Security currency lock (extends Phase 2)
`UpdateSecurity` refuses a currency change (`SECURITY_CURRENCY_LOCKED`, even with `reset_prices`) when any
non-deleted `investment_transactions` row of any investment in that security has `fx_rate_to_base IS NOT NULL`
(native-era trades). Legacy rows (NULL) don't block, so the Phase 2 `reconcile` still works on the existing data.

---

## Tests (matching existing styles)
- Buy/sell/dividend in a MYR security for a MYR user (identity: base == native, rate 1).
- USD security for a MYR user:
  - funded from a USD cash account (no `funding_amount`), with the market rate → `base_amount` from `fx_rates`;
  - funded from a MYR account → `funding_amount` required (assert the error details incl. suggestion), and with it the
    implied rate is used and the MYR account is debited exactly `funding_amount`;
  - explicit `fx_rate` beats both;
  - a partial sell then a full sell: `cost_basis_base` reaches exactly 0 and the realized base figures add up;
  - a dividend with fee into a USD account (net credited) and into a MYR account (`deposit_amount` required).
- `FX_RATE_UNAVAILABLE` when there's no rate and no `fx_rate`.
- The portfolio price/FX split invariant on a mixed portfolio.
- Transfers: same currency (to_amount omitted, equal, unequal → 400); cross currency (missing → error with suggestion,
  given → both balances right, `implied_rate`); update and delete restore both balances exactly; MCP rejection.
- Replay property test, plus a replay over an oversold sequence → error.
- Security currency lock with native-era vs legacy trades.
- Migration-shape test: AutoMigrate creates the new columns (the test DB uses models). The SQL is checked on Postgres
  by the reviewer.
- Integration test: MYR user, MYR cash, USD cash, MYR→USD transfer with `to_amount`, USD security bought from USD cash,
  then sold to the MYR account with `deposit_amount`. Assert every balance, the holding's native and base figures, and
  the portfolio split.

## Docs
CLAUDE.md: the Monetary Values note (trade amounts are in the security currency; `_base` fields; transfers' `to_amount`),
and the request fields in the endpoint list where useful.
