# Plan 024 — Phase 2 spec: native prices

The contract for Phase 2 (see `024-multi-currency.md` D3, D4, D7 holdings rows, D10 steps 2–3, and
D10.0 for the production data). Phase 1 (`phase-1-spec.md`) is merged: `fx_rates`, `internal/fx`,
`services.FXServicer`, `users.base_currency`, and the oracle publishing rates.

**Goal:** prices are stored in each security's own currency, and every place that values holdings
converts at read time. **The screens show the same MYR numbers as before** (derived correctly now), so
no frontend change is needed in this phase. Cost basis, trades and cash balances are untouched (Phase 3/4).

**Phase 2 compatibility rule:** existing JSON fields keep their meaning for current clients.
`Investment.current_price` stays **in the user's base currency**. The native price is added
alongside it. (D4's "`current_price` becomes native" moves to Phase 5, when the frontend switches.)

---

## API (`apps/api`)

### Migrations
- `000040_add_currency_to_security_prices`:
  up: `ALTER TABLE security_prices ADD COLUMN currency VARCHAR(3);`, then
  `UPDATE security_prices sp SET currency = UPPER(s.currency) FROM securities s WHERE s.id = sp.security_id;`, then
  `ALTER TABLE security_prices ALTER COLUMN currency SET NOT NULL;`. (Today every stored price really is in its
  security's labelled currency, because the oracle converted to MYR and every security is labelled MYR.)
  down: drop the column.
- `000041_add_currency_to_portfolio_snapshots`:
  up: `ALTER TABLE portfolio_snapshots ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT 'MYR', ADD COLUMN fx_missing BOOLEAN NOT NULL DEFAULT false;`
  down: drop both.
- Models: `SecurityPrice.Currency string` (`size:3;not null`, json `currency`). `PortfolioSnapshot.Currency`
  (json `currency`) and `FXMissing bool` (json `fx_missing`).

### Recording prices (`POST /api/v1/pipeline/securities/prices`)
- Each entry gains optional `currency` (binding `omitempty,iso4217`).
- The service loads the securities referenced in the batch (one query) and, per entry:
  - unknown security → rejected (`reason: "unknown_security"`);
  - `currency` given and ≠ `UPPER(security.currency)` → **rejected** (`reason: "currency_mismatch"`);
  - `currency` empty → stored as the security's currency (backward compatibility for an old oracle), and a
    single Warn log per request says how many entries had no currency.
  Rejected entries are skipped; the rest are inserted as today (`ON CONFLICT DO NOTHING`).
- Response: `{"recorded": n, "rejected": [{"index": 3, "security_id": "...", "currency": "USD", "expected_currency": "MYR", "reason": "currency_mismatch"}]}`.
  `rejected` is always present (`[]` when none). If **every** entry is rejected → still 200, with `recorded: 0`.
- `SecurityServicer.RecordPrices` signature changes accordingly (return a result struct). Update the handler,
  tests, and any other callers.

### Correcting a security (`PATCH /api/v1/pipeline/securities/:id`, pipeline API key)
Used by the oracle's `reconcile` command and by hand for the RBOT fix.
- Body (all optional, at least one required): `currency` (iso4217), `exchange`, `provider_symbol`, `name`,
  `reset_prices` (bool, default false).
- **Currency lock:** changing `currency` (case-insensitive compare) when the security has any `security_prices`
  rows requires `reset_prices: true`. Otherwise 409 with new error `SECURITY_CURRENCY_LOCKED` ("Security currency
  cannot change while it has prices; pass reset_prices to delete them"). With `reset_prices: true`, the update and
  `DELETE FROM security_prices WHERE security_id = ?` run in **one DB transaction**. `reset_prices` without a currency
  change is a 400 (it would only destroy data).
- Changing `exchange` must respect the unique `(symbol, exchange)` index: a conflict → 409 `SECURITY_ALREADY_EXISTS`
  (reuse the existing error if there is one for create).
- Response `200 {"security": {...}, "prices_deleted": n}`. Log at Info with Zap: security id, symbol, the changed
  fields old→new, prices_deleted.
- Add `UpdateSecurity` to `SecurityServicer`, a handler method, the route, Swagger annotations and tests (lock without
  reset, lock with reset deleting only that security's prices, reset without a currency change, exchange conflict, 404).

### Valuing holdings: one shared helper
Create `internal/services/holding_valuation.go`:
```go
// nativePrice is a security's latest price in its own currency.
type nativePrice struct { Price int64; Currency string; RecordedAt time.Time }

// latestNativePrices replaces getLatestPrices: the latest price row per security (same query
// shape and index as today) with the security's currency (UPPER(securities.currency)).
func latestNativePrices(db *gorm.DB, securityIDs []string) (map[string]nativePrice, error)

// valuer converts native amounts into one target currency at one instant, with all the FX
// rates it needs fetched in ONE FXServicer.Rates call (identity needs no query).
type valuer struct { target string; at time.Time; quotes map[CurrencyPair]*FXQuote }
func newValuer(fxs FXServicer, target string, at time.Time, currencies []string) (*valuer, error)
// Convert returns amount (in currency) in the target currency. ok=false when no rate exists.
func (v *valuer) Convert(amount int64, currency string) (converted int64, quote *FXQuote, ok bool)
```
- `fx.Convert` does the multiplication (never floats). `valueCents(qty, nativePrice)` is computed **first, in the
  native currency**, then converted once (convert the line value, not the unit price, so rounding matches the broker).
- `getLatestPrices` is removed. Every caller uses the helper.
- `FXServicer` is injected into `InvestmentService`, `AccountService` and `PortfolioSnapshotService` via their
  constructors (`AccountService` already takes variadic options; use a functional option such as
  `WithFXService(fxs)` there, per CLAUDE.md). Production wiring (`cmd/api`, `cmd/mcp`) always passes the single FX
  service built in Phase 1. A nil FX service is allowed **in tests only**: then only identity conversion succeeds.

### Where conversion applies (all at `time.Now()` unless stated)

| Place | Target currency | Behaviour |
|---|---|---|
| `Investment` responses (`AddInvestment`, `GetAccountInvestments`, `GetAllInvestments`, `GetInvestmentByID`) | the **user's** `base_currency` | `current_price` = native latest price converted (unit price conversion is fine **here**, since it's display only). New response-only fields (`gorm:"-"`): `current_price_native` (int64), `price_currency` (string), `fx_rate` (decimal string; `""` when the price currency is already base), `fx_stale` (bool), `fx_missing` (bool). No rate → `current_price = 0`, `fx_missing = true`. |
| `GetPortfolio` (`PortfolioSummary`) | user's base | Value per holding = `Convert(valueCents(qty, native), price_currency)`. Holdings with no rate are left out of `total_value` and the gain/loss, and listed in the new `unconverted: [{"currency":"USD","value":12345}]` (native value summed per currency). New fields: `currency` (base), `fx_stale` (true if any quote used was stale). Cost basis and realized P&L unchanged (still as stored). |
| Investment account balance (`account_service.go`, which sets `Balance` = market value of holdings) | the **account's** currency | Same per-holding conversion. A missing rate → that holding contributes 0 and a Warn log names the account and currency. |
| Snapshots (`computeAllSnapshots`) | each **user's** `base_currency` | Holdings converted at **`recordedAt`** (the FX lookup's `at`). One `Rates` call per distinct base currency for all currencies present. A holding with no rate is skipped and that user's snapshot gets `fx_missing = true`. Store `currency = base`. The upsert's `DoUpdates` must include `currency` and `fx_missing`. Account balances are **not** converted in Phase 2 (all accounts are MYR; Phase 4 does this). |
| MCP `tools_investments.go` | unchanged | It reads `inv.CurrentPrice`, which is still base currency, so it keeps working. No change beyond compiling. |

Snapshot API responses include `currency` and `fx_missing` automatically via the model.

### Tests
- `holding_valuation_test.go`: identity (no FX query, which can be asserted with a nil FX service), USD→MYR direct,
  inverse, missing rate, line value rounded **once** (e.g. qty 3 × 3333 native cents at 4.4712 vs converting the unit
  price first must differ), stale propagation.
- Update existing investment/account/snapshot tests for the constructor change, and add cross-currency cases: a USD
  security with USD prices and a USD→MYR rate, a MYR user → MYR values; the missing-rate path for each table row above;
  a snapshot at a past `recordedAt` uses the rate in effect then, not today's.
- The price recording and security PATCH tests as listed above.
- Integration test: create a MYR user, a MYR investment account, a USD security; post a USD→MYR rate and a USD price
  through the pipeline; `GET /investments/portfolio` shows the MYR value; a price posted with `currency: "MYR"` for that
  security is rejected as a mismatch.

### Docs
CLAUDE.md endpoint list (the new PATCH route; mention rejected entries on prices). Don't hand-edit Swagger (swag isn't installed).

---

## Oracle (`apps/oracle`)

### Part A: run path (normal `oracle` invocation)
1. **Stop converting prices.** Remove step 5b. Every `RecordPriceEntry` carries `currency` = the provider-reported
   currency (normalised, below). `ForexConverter` stays only as the FX-rate fetcher (`FetchRate`). Drop `Convert`,
   `NeedsConversion` and `GetRate` if nothing else uses them.
2. **Yahoo minor-unit currencies:** if `meta.currency` is `GBp` (or `GBX`), `ZAc` or `ILA`, report `GBP`/`ZAR`/`ILS`
   and divide the price by 100 (round half away from zero, in integer cents). Case-sensitive: `GBP` ≠ `GBp`.
   Table-driven test.
3. **CoinGecko:** group securities by `UPPER(security.currency)`. One request per group with
   `vs_currencies=<lower(currency)>`. The reported currency is that group's currency. (Crypto securities are MYR in
   production today, so they keep requesting MYR.)
4. **TARGET_CURRENCY is removed.** If the env var is set, log one Warn ("TARGET_CURRENCY is ignored since plan 024
   phase 2") and ignore it. `NewForexConverter` no longer needs a target. The FX pair union becomes the API's pairs ∪
   (each provider-reported currency × each base currency), without the target.
5. **Rejected prices:** the client decodes the new `rejected` list. Each rejected entry becomes a `FetchError` (symbol
   from the security, error text includes reason and both currencies), so the run summary counts them.
6. Tests: update the existing ones (no conversion anymore: a USD Yahoo price is posted as USD cents unchanged), and
   cover GBp, CoinGecko grouping, TARGET_CURRENCY ignored, and rejected entries turned into FetchErrors.

### Part B: subcommands (for the one-off migration; useful later too)
`main.go` dispatches on `os.Args[1]`: no argument → the normal run (**the Docker `ENTRYPOINT` and cron usage must not
change**). Use the standard `flag` package with a `flag.FlagSet` per subcommand. Unknown subcommand → usage on stderr,
exit 2.

1. **`oracle reconcile [--apply]`.** For every security that a Yahoo-type provider supports (same `Supports`/asset-type
   logic as the run), fetch Yahoo's `meta.currency` (normalised as above) for its `provider_symbol` (or the symbol the
   provider would build). Print a table: symbol, exchange, provider_symbol, stored currency, provider currency, action.
   Without `--apply` it changes nothing (dry run is the default). With `--apply`, each mismatch calls
   `PATCH /pipeline/securities/:id` with `{"currency": <provider>, "reset_prices": true}` and prints `prices_deleted`.
   Fetch failures are listed and don't stop the others. Exit 0 if all mismatches are applied (or none exist), 1 if any
   failed.
2. **`oracle backfill --since YYYY-MM-DD [--until YYYY-MM-DD] [--prices] [--fx] [--symbols A,B] [--dry-run]`.** If
   neither `--prices` nor `--fx` is given, do both. `--until` defaults to today (UTC).
   - **Prices:** for each Yahoo-supported security (filtered by `--symbols` against `symbol`, case-insensitive), fetch
     daily history from the same Yahoo chart endpoint with `period1`/`period2` (unix seconds) and `interval=1d`. Use each
     bar's `timestamp` as `recorded_at` and its `close` as the price (skip null closes). Normalise the currency and minor
     units as in Part A. Post with that currency in batches of ≤ 5000. Crypto/CoinGecko securities are skipped with an
     Info log (their native currency didn't change).
   - **FX:** pairs = `GET /pipeline/fx-pairs` pairs ∪ (currencies seen in the price backfill × base currencies).
     Fetch daily history for `BASEQUOTE=X` the same way, and post via `POST /pipeline/fx-rates` (rate as
     `FormatFloat(close,'f',10,64)`, `recorded_at` = bar timestamp, source `yahoo`), batched ≤ 5000.
   - Run FX **before** prices. Print per-security / per-pair counts (fetched, recorded, rejected) and totals.
     `--dry-run` fetches and prints counts but posts nothing. Exit 1 if anything failed.
   - Reuse the Yahoo HTTP client, retry helper and parsing. Factor the chart-response parsing so the live fetch and the
     history fetch share it.
3. Tests with httptest servers / fake transports: arg parsing (defaults, both flags off means both on, bad dates), the
   reconcile dry run vs apply (PATCH bodies), backfill batching, FX-before-prices order, null closes skipped, GBp history,
   `--symbols` filter, `--dry-run` posts nothing, and no-args still runs the normal path.

### Docs
`.env.prod.example`: mark `TARGET_CURRENCY` as removed. Add a short "Oracle subcommands" note to CLAUDE.md (under
Common Commands) showing how to run them in production:
`docker compose -f docker-compose.prod.yml --profile oracle run --rm oracle reconcile`.

---

## Production runbook (done by the operator after deploying Phase 2; recorded here, not automated)
0. Back up the database. Disable the oracle schedule.
1. Deploy (migrations 40 and 41 run).
2. Fix RBOT by hand (it's mapped to the wrong listing): `PATCH /pipeline/securities/<RBOT id>` with
   `{"exchange":"LSE","provider_symbol":"RBOT.L","currency":"USD","reset_prices":true}`.
3. `oracle reconcile` (dry run). Expect exactly CCJ, CHA, IBKR, NVDA, NVO, SOUN, SPCX, VUAA, VWRA → USD, and nothing
   for the Bursa or crypto securities. Then `oracle reconcile --apply`.
4. `oracle backfill --since 2025-01-01`.
5. Re-enable the schedule. Check the portfolio total against the previous day's snapshot (it should move only by the
   difference between the old fetch-time rates and the backfilled rates).
