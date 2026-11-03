# Plan 024 — Multi-Currency (native amounts, convert on read)

**Status: proposed.** Nothing implemented yet. Decisions **Q1–Q8** were all settled on the
recommended option (see the table at the end). The production data inventory that shapes the
migration is in **D10.0**.

## Context

Kuberan pretends to be multi-currency, but MYR is baked in everywhere money is stored:

| Layer | What happens today | Where |
|---|---|---|
| Oracle prices | Every Yahoo price is converted to `TARGET_CURRENCY` (MYR) at fetch-time FX before it's posted. CoinGecko is queried with `vs_currencies=myr`. | `apps/oracle/internal/oracle/oracle.go` (step 5b), `provider/forex.go`, `provider/coingecko.go` |
| `security_prices` | No currency column. Values are implicitly MYR while `securities.currency` says `USD`, so the schema contradicts itself. | `internal/models/security_price.go`, `internal/models/security.go` |
| Investment transactions | `price_per_unit`, `total_amount`, `fee`, `realized_gain_loss` and `investments.cost_basis` have no currency. The web dialogs are entered in MYR. | `internal/services/investment_service.go` (`RecordBuy` / `RecordSell` / `RecordDividend`) |
| Buy funding / sell proceeds | `RecordBuy` debits a cash funding account and `RecordSell` credits a deposit account for `total_amount`, with no currency check. | `investment_service.go` (`fundingAccountID`, `depositAccountID`) |
| Accounts | `accounts.currency` exists (default `'USD'`), but nothing converts. Cross-currency transfers are rejected. | `models/account.go`, `transaction_service.go` (`ErrTransferCurrencyMismatch`) |
| Aggregates | Snapshots, `GetPortfolio`, monthly summary, daily spending, spending-by-category and budget progress add up raw cents across currencies. | `portfolio_snapshot_service.go`, `investment_service.go`, `transaction_service.go` (`periodTotals`, `totalsByCategory`) |
| Web | `formatCurrency` defaults to `"MYR"`. 127 call sites in 37 files, most of them without a currency. | `apps/web/src/lib/format.ts` |
| Bot / MCP | The bot has a per-link `default_currency` (MYR). MCP prints `account.currency` next to raw balances. | `models/telegram_link.go`, `internal/mcp/format.go`, `tools_*.go` |

### Why this is wrong
A USD holding is really *N shares × a USD price*, bought with *USD cash* at a *USD cost*.
Kuberan stores MYR equivalents frozen at whatever rate applied when each row was written, so:

- Cost basis, realized P&L and dividends never match the broker's statements.
- Gain/loss mixes **price return** with **FX return**, and the two can't be separated.
- USD cash at the broker can't be represented without being silently re-valued.
- Every aggregate is correct only while all accounts happen to be MYR.

## Goal and principles

1. **Store every amount in the currency it happened in.** A row's currency comes from its
   owner (account → currency, security → currency); amounts stay `int64` minor units.
2. **Convert only on read**, into the user's **reporting currency** (`users.base_currency`,
   default MYR), using a stored, dated FX rate.
3. **Valuations use the rate at valuation time; history uses the rate at event time.** A
   snapshot recorded on 2026-03-01 uses the 2026-03-01 rate and is never re-valued.
4. **A conversion that actually happened is recorded, not recomputed.** An FX transfer or a
   buy funded from MYR cash stores both legs (what left, what arrived).
5. **No silent mixing.** Every aggregate either converts explicitly or groups by currency.
   A missing rate is an error you can see, never a 1:1 fallback.

### Non-goals
- Multiple currencies inside one account (a "multi-currency wallet"). Model it as one account
  per currency (e.g. "IBKR USD", "IBKR MYR").
- Tax-lot / FIFO accounting. The average-cost model stays.
- Intraday FX. One rate per pair per oracle run is enough.
- Currencies with 0 or 3 minor digits (JPY, KWD): see **Q8**.

## Design

### D1. Reporting currency
- Migration: `users.base_currency VARCHAR(3) NOT NULL DEFAULT 'MYR'`.
- `PATCH /api/v1/profile` accepts `base_currency` (`iso4217`, restricted to currencies with
  an FX source, see D2).
- Changing it re-converts every *live* view immediately (they all convert on read). Stored
  snapshots carry their own currency (D6). See **Q4** for how past snapshots are shown.
- `telegram_links.default_currency` stays as the bot's default for *new accounts*, but the
  bot shows totals in `users.base_currency`.

### D2. FX rates (new table + pipeline)
```sql
CREATE TABLE fx_rates (
  id          UUID PRIMARY KEY,
  base        VARCHAR(3)  NOT NULL,          -- 1 unit of base ...
  quote       VARCHAR(3)  NOT NULL,          -- ... = rate units of quote
  rate        NUMERIC(20,10) NOT NULL CHECK (rate > 0),
  recorded_at TIMESTAMPTZ NOT NULL,
  source      VARCHAR(32) NOT NULL DEFAULT 'yahoo',
  CONSTRAINT uq_fx_rates_pair_recorded UNIQUE (base, quote, recorded_at)
);
CREATE INDEX idx_fx_rates_pair_recorded_desc ON fx_rates (base, quote, recorded_at DESC);
```
- **Rate representation in Go:** the rate is not money, so it isn't cents. Keep it exact in
  storage (`NUMERIC`) and in Go use a fixed-point `int64` scaled by 1e10, or `shopspring/decimal`
  (**Q1**). Conversion = `round_half_even(amount_minor × rate)` in one place, `fx.Convert`.
  Never `float64` money math. (SQLite tests: NUMERIC is stored as REAL. Acceptable for tests,
  and the scaled-int path avoids depending on it.)
- **Storage direction:** store pairs as fetched (`USD→MYR`). Lookups try the direct pair, then
  the inverse (`1/rate`), then a pivot through USD (`EUR→USD→MYR`). Identity (`X→X`) is 1
  with no row.
- **Lookup:** `FXService.Rate(from, to, at)` returns the latest rate with `recorded_at <= at`.
  Batch variant `Rates(pairs, at)` for aggregates (one query, mirrors `getLatestPrices`).
  `RatesSeries(from, to, start, end, bucket)` for snapshot backfill and charts.
- **Staleness:** a rate older than `FX_MAX_STALENESS` (default 7 days, covers weekends and
  holidays) for "now" lookups is still used, but the response is flagged `fx_stale: true` and the
  UI shows a hint. No rate at all → `FX_RATE_UNAVAILABLE` (new AppError, 503 for reads in a
  pipeline, otherwise the item is listed under `unconverted` (see D7) rather than failing the
  whole page).
- **Pipeline endpoints** (API-key auth, same as prices):
  - `POST /api/v1/pipeline/fx-rates`: `{rates: [{base, quote, rate: "4.4712", recorded_at}]}`.
    `rate` is a **string** so JSON floats don't lose precision. Upsert on the unique key.
  - `GET  /api/v1/pipeline/fx-pairs`: the pairs the oracle should fetch = every distinct
    `currency` across active accounts and securities × every distinct `users.base_currency`,
    minus identities. This keeps the oracle config-free.
- **Read endpoint:** `GET /api/v1/fx-rates?base=USD&quote=MYR&from&to` for charts and the UI's
  "rate used" tooltips.

### D3. Oracle changes (`apps/oracle`)
- **Stop converting prices.** Delete step 5b. `RecordPriceEntry` gains `currency` (the
  provider-reported one). `ForexConverter` becomes an **FX fetcher**: `FetchRates(ctx, pairs)`
  using the existing Yahoo `XXXYYY=X` logic and its cache.
- **CoinGecko:** request `vs_currencies=<security.currency lowercased>` per security (group by
  currency, one request per group) instead of the global target. Crypto securities default to
  `USD`.
- **Run order per tick:** fetch pairs → fetch and post FX rates → fetch and post prices →
  compute snapshots → process FDs. Rates go first so the snapshot always has a same-tick rate.
- **Currency mismatch guard:** if Yahoo reports a currency different from `securities.currency`
  (e.g. a security created with the wrong currency, or LSE pence `GBp`), the API rejects that
  price entry (see D4) and the oracle logs it as a `FetchError`. Normalise `GBp` → `GBP` ÷ 100
  and `ZAc`/`ILA` similarly in the Yahoo provider.
- **Config:** `TARGET_CURRENCY` is removed (startup warning if it's still set: "ignored since
  plan 024"). New `FX_BACKFILL_DAYS` for the one-off history backfill (D10).
- **Backfill command:** `oracle backfill --fx --prices --since 2020-01-01` pulls Yahoo daily
  history (`range=max&interval=1d`) for the pairs and for non-MYR securities, and posts them in
  batches of ≤5000 (the existing `RecordPrices` cap).

### D4. Securities and prices in native currency
- Migration: `security_prices.currency VARCHAR(3) NULL`, backfilled (D10), then `NOT NULL`.
- `RecordPrices` validates `entry.currency == security.currency` (case-insensitive). On a
  mismatch, that entry is rejected and the response lists it in `rejected[]`. Other entries
  still record.
- `securities.currency` becomes **immutable after the first price or holding exists**
  (`SECURITY_CURRENCY_LOCKED`). Changing it would re-interpret history.
- `Investment.CurrentPrice` is native. The JSON adds `currency` (from the security) and
  `current_price_base` / `value_base` (converted, see D7).

### D5. Accounts, investment transactions, funding
**Accounts**
- `accounts.currency` is authoritative and **immutable once the account has any
  transaction** (`ACCOUNT_CURRENCY_LOCKED`). The default changes from `'USD'` to the user's
  `base_currency` at creation (handler fills it in; the DB default stays for safety).
- Investment accounts get a currency too (the broker's settlement currency), used for
  `realized_gain_loss` roll-ups and as the default funding currency.

**Holdings rule (Q2):** an investment's amounts (`cost_basis`, `realized_gain_loss`, and on its
transactions `price_per_unit`, `total_amount`, `fee`) are in the **security's currency**.
That is what broker statements show per position, and it keeps per-holding P&L FX-free.
- An account may hold securities in any currency (an IBKR account holding US and HK stocks);
  aggregates convert.

**Trade FX (optional, recorded):** `investment_transactions` gains
- `fx_rate_to_base NUMERIC(20,10) NULL` (security ccy → the user's base, at trade date), and
- `base_amount BIGINT NULL` (`total_amount` in base at that rate, frozen).
Filled automatically from `fx_rates` at the trade `date` if the user didn't give one. These make
**FX-aware cost basis** possible (D7): "what this position actually cost me in MYR".
`investments.cost_basis_base BIGINT` is maintained alongside `cost_basis` (same average-cost
reduction on sell).

**Funding / proceeds across currencies**
- `RecordBuy` with a funding account in a different currency requires the request to carry
  `funding_amount` (what actually left the cash account, in its currency, i.e. what the bank or
  broker charged). If it's omitted, the server proposes one from `fx_rates` and the UI shows it
  for confirmation (the API rejects with `FX_AMOUNT_REQUIRED` + the suggested value). Same for
  `RecordSell` → `deposit_amount`, and dividends credited to a cash account.
- The linked cash `transactions` row stores the funding-side amount in the funding account's
  currency, so cash balances stay exact.
- Same-currency funding keeps working unchanged.

**Dividends:** `total_amount` is in the security's currency. Optional `deposit_account_id` +
`deposit_amount` as above (today dividends don't credit any account; keep that optional).

### D6. Cross-currency transfers
- `POST /api/v1/transactions/transfer` accepts an optional `to_amount`. Same currency:
  `to_amount` must be empty or equal to `amount`. Different currencies: `to_amount` is required
  (it's what actually arrived). The response includes the implied rate.
- Storage: the transfer pair already has two `transactions` rows (out/in). Each row's `amount`
  is in its own account's currency. Add `transactions.counter_amount BIGINT NULL` +
  `counter_currency VARCHAR(3) NULL` on both legs so either side can show "RM 4,470 → US$1,000".
- Editing a transfer edits both amounts explicitly. Deleting removes both legs (as today).
- Fees: out of scope (record a separate expense), but `to_amount` already captures the spread.

### D7. Conversion on read (all aggregates)
One helper used everywhere: `fx.Converter` built per request with `(base, at)` and a batch of
pre-fetched rates. `Convert(amount, from) (int64, ok bool)`.

| Endpoint / service | Change |
|---|---|
| `GET /accounts` | Each account adds `balance_base` (+ `fx_rate`, `fx_stale`). The list adds `totals_base` by type. |
| `GET /investments/portfolio` (`GetPortfolio`) | Value, cost basis and unrealized P&L in base. New fields: `price_return_base` (Σ (value_native − cost_native) × rate_now) and `fx_return_base` (Σ cost_native × rate_now − cost_basis_base). Their sum equals `value_base − cost_basis_base`. `HoldingsByType` values in base. Also `by_currency: {USD: {value, value_base}, ...}` for exposure. |
| `GET /investments`, `/:id`, `/accounts/:id/investments` | Native fields unchanged + `currency`, `value_base`, `cost_basis_base`, `unrealized_base`. |
| Snapshots (`computeAllSnapshots`) | Convert each account balance and each holding value into the user's base at `recordedAt` before adding up. The balance query groups by `(user_id, type, currency)`. New columns: `portfolio_snapshots.currency VARCHAR(3)` (the base used) and `fx_missing BOOLEAN` (any item skipped for lack of a rate, so the chart can mark the point). |
| `GetMonthlySummary`, `GetDailySpending`, `GetSpendingByCategory` (`periodTotals`, `totalsByCategory`) | Group by `accounts.currency` in SQL, then convert each bucket at the **bucket's date** (daily: that day; monthly: month-end or average, see **Q5**) in Go. Single-currency users take a fast path that skips conversion. |
| Budgets (`/budgets/:id/progress`) | `budgets.currency VARCHAR(3)` (default the user's base). Spending in other currencies is converted at transaction date. |
| Receivables (plan 021) | Inherit the currency of the expense's account. Settlements into a different-currency account need `settled_amount` in that account's currency (same rule as D5). The outstanding summary converts to base. |
| Fixed deposits (plan 020) | Already enforce a single currency across source/linked/FD. No change beyond snapshot conversion. |
| Dashboard "net worth", "composition", "cash flow" cards | Consume the `_base` fields. |

Items whose rate is missing are **left out of the base total and listed** in
`unconverted: [{currency, amount}]`. The UI shows "excludes US$X (no rate)". Never 1:1.

### D8. Frontend (`apps/web`)
- `types/models.ts`: add `currency` where it's missing (`Investment`, `InvestmentTransaction`,
  `Budget`, `PortfolioSnapshot`), plus the `_base` fields and `fx_stale`. Add `base_currency` to `User`.
- `formatCurrency(cents, currency)`: **make `currency` required** (drop the `"MYR"` default) so
  the compiler finds all 127 call sites. Add `useBaseCurrency()` (from the profile query) for totals.
- A `<Money amount currency baseAmount? />` component renders native first and the base second
  (`US$1,234.00 · ≈ RM 5,512`), with a tooltip showing the rate and date. It hides the second line
  when the currency already is base. It respects `hide_balances`.
- Dialogs:
  - Create account: currency select (defaults to base, searchable ISO list).
  - Add investment / buy / sell / dividend: amounts in the security's currency (symbol in
    `CurrencyInput`). When the funding/deposit account differs, a second field "Amount charged
    to <account> (MYR)" is pre-filled from the suggested rate and editable.
  - Transfer: when the currencies differ, show "Amount received" + the implied rate.
- Portfolio page: split "Unrealized P&L" into **Price** and **FX** contributions, and add a
  "Currency exposure" donut (by_currency).
- Settings → Profile: "Reporting currency" select. Changing it invalidates all queries.
- Charts: the net worth chart reads `snapshot.currency`. If snapshots in different currencies are
  in the range (after a base change), see **Q4**.

### D9. MCP and bot
- **MCP read tools:** every amount is printed with its currency, and totals with the base
  (`formatMoney(cents, ccy)` replaces `formatCents`). `get_portfolio` includes the price/FX split and
  exposure. `get_snapshots` prints the snapshot currency.
- **MCP write tools (plan 022):** `create_transaction` etc. are unchanged (amounts are in the target
  account's currency, so say so in the tool description). The `MCP_MAX_WRITE_AMOUNT` ceiling is
  evaluated **in base currency** (convert; if no rate, reject). A transfer write tool, if one is
  added later, must require `to_amount` across currencies.
- **Bot:** balances in account currency (it already does this), totals and the "owed" summary in
  base. `/accounts` total line uses `balance_base`.

### D10. Data migration (existing single-user MYR data)
Order matters. Run it in a maintenance window with the oracle stopped.

#### D10.0 Production inventory (queried 2026-11-03)
- **Accounts:** all 19 are labelled `MYR`, and all except possibly **IBKR Cash** really are MYR.
  The `'USD'` column default never caught anything, so the mislabelled-account risk below is
  limited to IBKR Cash. IBKR Cash has 11 transactions since 2026-03-27 and a balance of RM 37.47.
  Investment accounts carry no balance and no cash transactions (funding goes through the cash
  accounts).
- **Securities: all 25 are labelled `MYR`, including foreign listings.** That label is what the
  oracle's converted prices made true. Correct currencies (Yahoo's reported currency):

  | Currency | Securities | Open positions | Trades |
  |---|---|---|---|
  | MYR (Bursa) | CARLSBG, CIMB, GAMUDA, MAYBANK, MBMR, RHBBANK, SENDAI, SIME, SUNREIT, UTDPLT, VERSA (0807EA.KL), YTLPOWR | 7 | 26 |
  | USD (NYSE/NASDAQ) | CCJ, CHA, IBKR, NVDA, NVO, SOUN, SPCX | 5 | 18 |
  | USD (LSE, USD lines) | VUAA.L, VWRA.L | 2 | 10 |
  | CAD (TSX) | RBOT.TO | 0 (closed) | 5 |
  | MYR (crypto, bought on Luno in MYR) | BTC, ETH, SUI | 2 | 22 |

  So **33 trades across 10 securities** (all in the Interactive Brokers account) need
  converting, the earliest on **2025-01-28**. Crypto stays MYR: it was bought with MYR on Luno and
  Luno quotes MYR, so MYR is its native currency.
- **Investment transactions:** 81 in total (45 buy, 25 sell, 7 dividend, 4 split), earliest 2023-08-21.
- **IBKR holding:** 1.9949 shares with `cost_basis = 0` (probably promo or transferred shares).
  The replay has to keep zero-cost lots working.
- **Snapshots:** 11,586 rows since 2026-02-13, all MYR. Keep as recorded (Q6).
- **Receivables:** none.

Consequences for the steps below:
- **New step 2a, relabel securities:** a one-off migration sets `securities.currency` to the
  table above, **before** the price currency guard (D4) and the currency lock are switched on.
  Otherwise every native price for those 10 securities would be rejected as a mismatch.
- **FX backfill** needs USD→MYR and CAD→MYR from **2025-01-01**.
- **Step 4:** with 33 trades, all from one broker, use **path (a)** (IBKR statement export) for
  all of them. Keep path (b) as a fallback for any trade missing from the export.
- **Step 5** covers only IBKR Cash (see open item below).

**IBKR Cash is USD** (confirmed; the IBKR base currency is USD, and the 2026 statement has no
forex trades). It becomes a USD account, and its 11 transactions are re-entered from the statement.
Deposits arrive as USD wires (e.g. 2026-06-04 USD 7,461.52), so each one becomes an FX transfer
(D6) from the MYR bank account: `amount` is the MYR that left the bank (from the existing Kuberan
transaction), `to_amount` is the USD from the statement. IBKR reports USD 120.55 of cash at
2026-11-02, while Kuberan shows RM 37.47.

#### D10.0a IBKR Activity Statement import
Format: IBKR's default **Activity Statement CSV** (multi-section: column 1 is the section,
column 2 is `Header`/`Data`/`SubTotal`/`Total`, and each section has its own header row). Parse by
section name and header, never by column position. Only `Data` rows are used.

| Section | Becomes | Notes |
|---|---|---|
| `Trades` (DataDiscriminator `Order`) | buy (qty > 0) / sell (qty < 0) | `T. Price` → price_per_unit, `abs(Comm/Fee)` → fee, date from `Date/Time` (ET, date part). Codes `O`/`C`/`P`/`FPA`/`IM` are informational. Code `R` (dividend reinvestment) is a normal buy. Funded from / proceeds to IBKR Cash. |
| `Dividends` | dividend | One per (symbol, date). "Payment in Lieu of Dividend" (shares lent out) counts as a dividend. Reversals net out against their original. Credited to IBKR Cash. |
| `Withholding Tax`, `Fees` (lines that name a dividend / ADR fee) | dividend `fee` | Matched to the dividend by symbol + date, so `total_amount` = gross and `fee` = tax + fees. An ADR fee with no dividend that day (CHA 2026-04-14) becomes a plain expense on IBKR Cash. |
| `Grant Activity` | buy with no funding account | One per vesting at the vesting `Price`, for the **net** quantity (vesting − withholding). Cost basis then matches IBKR's (USD 139) instead of today's 0. |
| `Deposits & Withdrawals` | FX transfer leg into IBKR Cash | Matched with the MYR bank transaction by date ± 5 days. Unmatched deposits are listed for manual pairing. |
| `Open Positions` | **reconciliation only** | After import, each holding's quantity must equal IBKR's exactly. Cost basis is compared with a tolerance, since IBKR uses FIFO lots while Kuberan uses average cost (see the note below). |
| `Cash Report` → Ending Cash | reconciliation only | IBKR Cash balance must equal it. |
| Others (NAV, MTM, Realized/Unrealized, Accruals, Instrument Info, Codes, Notes) | ignored | `Financial Instrument Information` maps Symbol → ISIN and listing exchange for matching securities. |

- **Coverage:** one statement covers at most a calendar year. Positions held at 2026-01-01
  (CCJ, CHA, IBKR, NVO, VUAA, VWRA) and the 2025-only symbols (RBOT, SOUN) need the **2025
  statement**, plus any earlier year since account opening. The import takes several files, and
  replays every trade per holding in date order.
- **Splits:** Kuberan has 4 `split` rows since 2026-03-27 that don't appear in the statement
  (there's no corporate-actions section), so they look like manual quantity fixes. The import
  replaces each holding's transactions wholesale, so they go away.
- **Known mismatches today** (Kuberan vs IBKR at 2026-11-02): IBKR 1.9949 vs 1.6706, NVO 30 vs
  30.2548, NVDA 10.2 vs 10.2137. CCJ, VUAA, SPCX and CHA match. VWRA (48.2 vs 37) is a buy of
  11.2 made after the statement period, so reconciliation must only count trades up to the
  statement's end date.
- **Average cost vs FIFO:** for holdings that were partly sold (NVO), Kuberan's remaining cost
  basis and realized P&L will differ from IBKR's FIFO figures. Quantities and cash still match
  exactly. FIFO lots remain a non-goal of this plan.
- The statement contains the account holder's name and account number. Test fixtures must be
  anonymised, and statements are never committed.

1. **Schema migrations** (D1, D2, D4–D7 columns), all nullable or defaulted, reversible.
2. **FX history backfill:** `oracle backfill --fx --since <first transaction date>` for each needed
   pair (USD→MYR, plus any others in use).
3. **Prices:**
   - MYR securities: `UPDATE security_prices SET currency='MYR'` (they're correct already).
   - Non-MYR securities: **delete their price rows and refetch native history**
     (`oracle backfill --prices`). This is more accurate than dividing old rows by a rate, because
     the oracle's conversion rate at fetch time wasn't stored.
   - Then `ALTER ... SET NOT NULL`.
4. **Investment transactions for non-MYR securities.** Two supported paths, chosen per holding (**Q3**):
   - **(a) Re-enter from broker statements (exact).** A CSV import:
     `POST /api/v1/investments/:id/transactions/import` (date, type, qty, price, fee, total,
     optional fx_rate) replaces the holding's transactions and recomputes `quantity`,
     `cost_basis`, `cost_basis_base` and `realized_gain_loss` by replaying them.
   - **(b) Back-convert (approximate).** Script `cmd/migrate-fx`: for each transaction,
     `native = round(myr / rate(USD→MYR, date))`, set `fx_rate_to_base = rate` and
     `base_amount = old MYR amount` (so MYR cost basis is preserved exactly), then replay to
     rebuild the holding. Dry-run by default, prints a per-holding diff, `--apply` to write.
   - Either way the **replay** function (`rebuildInvestment(tx, investmentID)`) is the single source
     of truth and also serves future edits/deletes of investment transactions.
5. **Accounts:** set `currency` correctly on each account (today many are probably the `'USD'`
   column default but actually hold MYR). The script lists accounts whose currency ≠ base and asks
   for confirmation. If any account really is USD (e.g. broker cash), its balance and transactions
   need path (a) or (b) as above.
6. **Snapshots:** `UPDATE portfolio_snapshots SET currency='MYR'`. They're historically correct MYR
   values, so leave them alone. Optionally recompute from the replayed data (**Q6**).
7. **Users:** `base_currency='MYR'`.
8. Start the new oracle. Verify (D12).

### D11. Errors (new `AppError`s)
`FX_RATE_UNAVAILABLE`, `FX_AMOUNT_REQUIRED` (400, with `suggested_amount` + `rate` in details),
`ACCOUNT_CURRENCY_LOCKED`, `SECURITY_CURRENCY_LOCKED`, `PRICE_CURRENCY_MISMATCH` (per entry,
pipeline), `UNSUPPORTED_CURRENCY`. `ErrTransferCurrencyMismatch` is replaced by
"`to_amount` required" (`FX_AMOUNT_REQUIRED`).

### D12. Testing
- **`fx` package (95% target, it's balance logic):** table tests for direct, inverse, pivot and
  identity lookups, `<= at` selection, staleness, missing rates, half-even rounding, huge amounts
  (overflow guard like `lineAmount`).
- **Services:** buy/sell/dividend with same and cross-currency funding (cash balances exact,
  `cost_basis_base` reduction on partial sells), FX transfer legs, snapshots with mixed currencies
  and a missing rate, analytics bucketing per date, budget conversion.
- **Replay:** `rebuildInvestment` gives the same result as the incremental path for random
  sequences (property-style test).
- **Pipeline handlers:** fx-rates upsert, string rate parsing, price currency mismatch rejection,
  fx-pairs listing.
- **Oracle:** no conversion on the price path, CoinGecko grouped by currency, `GBp` normalisation,
  run order, backfill batching.
- **Migration script:** fixture of today's MYR-only data → dry-run diff → apply → portfolio MYR
  cost basis unchanged; native USD figures within 1 cent per transaction of the expected values.
- **Integration:** "MYR cash → buy USD stock → price moves → FX moves → sell to USD cash →
  transfer back to MYR", asserting price return + FX return = total return at each step.
- **Web:** type-check catches every `formatCurrency` call. Snapshot tests for `<Money>`.

## Phases

Each phase ends green on `./scripts/check-go.sh apps/api` and `apps/oracle`, and is deployable on
its own.

| # | Phase | Contents | User-visible? |
|---|---|---|---|
| 1 | **FX foundation** | `fx_rates` migration, `FXService` + `fx.Converter`, pipeline `fx-rates`/`fx-pairs`, read endpoint. Oracle posts rates (still converting prices too). `users.base_currency`. | No |
| 2 | **Native prices** | `security_prices.currency`, mismatch guard, oracle stops converting, CoinGecko per-currency, `GBp` handling, backfill command, **data steps D10.2–3**. `GetPortfolio`, investments list and snapshots convert holdings on read. | Values unchanged (still MYR on screen) but now derived correctly |
| 3 | **Native trades and funding** | Investment transaction FX columns, `cost_basis_base`, `rebuildInvestment`, cross-currency buy/sell/dividend funding, account/security currency locks, **data steps D10.4–7** (import + `migrate-fx`). | Holdings show USD natively |
| 4 | **Cross-currency cash** | Transfer `to_amount`, counter-amount columns, analytics, budgets and receivables conversion, account `balance_base`. | Multi-currency cash accounts usable |
| 5 | **Frontend** | `formatCurrency` required arg, `<Money>`, dialogs, price/FX P&L split, exposure donut, reporting-currency setting, staleness hints. (Pieces can ship with phases 2–4. Listed separately for scope.) | Yes |
| 6 | **MCP, bot, docs** | MCP formatting, base-currency write ceiling, bot totals, `CLAUDE.md` / `.env.prod.example` / `docs/` updates (remove `TARGET_CURRENCY`, document FX). | Yes |

Rough size: phases 1–2 ≈ small/medium, phase 3 is the largest (replay + migration), 4 medium,
5 medium/large (mostly mechanical), 6 small.

## Risks
- **Migration accuracy.** Back-converted USD figures (path b) won't exactly match statements. This is
  why path (a) exists and the dry-run diff is mandatory. MYR cost basis is preserved exactly either way.
- **Yahoo FX gaps** (weekends, outages). Handled by `<= at` lookup + staleness flag + `unconverted`.
- **Performance.** Analytics now group by currency and convert per bucket. Rates for a range are one
  query. Single-currency users take the fast path. The existing aggregate indexes (migration 35)
  still apply, with `accounts.currency` added to the grouping.
- **Wrong `accounts.currency` defaults today** (`'USD'` default while the data is MYR). Step D10.5 is
  required before phase 4's conversions go live, otherwise MYR balances would be "converted" as USD.
  Phase 2 must not convert *account balances* yet (holdings only) for this reason.
- **Editing old investment transactions.** With replay in place this becomes safe. Without it, keep
  them append-only as today.

## Decisions

All accepted as recommended (2026-11-03).

| # | Question | Decision |
|---|---|---|
| Q1 | FX rate type in Go: scaled `int64` (1e10) vs `shopspring/decimal` | Scaled `int64` with `math/big` for the multiply. No new dependency, deterministic. |
| Q2 | Holding amounts in **security** currency vs **account** currency | Security currency (matches per-position broker statements, keeps P&L FX-free). |
| Q3 | Existing USD trades: re-enter from statements (a) or back-convert (b) | Build both. Use (a) for positions you have statements for, (b) for the rest. |
| Q4 | After changing base currency, how to show older snapshots | Convert old snapshots on read at their own `recorded_at` rate (needs FX history, which exists after backfill), so charts stay continuous. |
| Q5 | Monthly analytics FX: month-end rate vs per-transaction-date rate | Per-transaction-date (group by currency + day in SQL, convert, then roll up to month). It's exact and still cheap. |
| Q6 | Recompute historical snapshots after migration? | No. Keep the MYR values as recorded. Offer `POST /pipeline/snapshots/recompute?since=` for later. |
| Q7 | Supported currency list | Any ISO 4217 code that has a Yahoo `XXXMYR=X` (or USD pivot) rate. Validate on account/security create via `fx-pairs` availability. |
| Q8 | Non-2-decimal currencies (JPY, KWD) | Out of scope for now: reject at creation with `UNSUPPORTED_CURRENCY`. A later plan can add a per-currency minor-unit exponent. |
