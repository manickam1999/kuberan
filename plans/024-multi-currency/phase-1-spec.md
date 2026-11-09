# Plan 024 — Phase 1 spec: FX foundation

This is the exact contract for Phase 1 (see `024-multi-currency.md`, D1/D2 and the phase table).
Nothing in Phase 1 changes values shown to users: prices are still converted by the oracle, and
no aggregate converts yet.

## API (`apps/api`)

### Migrations
- `000038_create_fx_rates.{up,down}.sql`: the table from D2, exactly:
  ```sql
  CREATE TABLE IF NOT EXISTS fx_rates (
    id          UUID PRIMARY KEY,
    base        VARCHAR(3)  NOT NULL,
    quote       VARCHAR(3)  NOT NULL,
    rate        NUMERIC(20,10) NOT NULL CHECK (rate > 0),
    recorded_at TIMESTAMPTZ NOT NULL,
    source      VARCHAR(32) NOT NULL DEFAULT 'yahoo',
    CONSTRAINT uq_fx_rates_pair_recorded UNIQUE (base, quote, recorded_at),
    CONSTRAINT fx_rates_distinct_pair CHECK (base <> quote)
  );
  CREATE INDEX IF NOT EXISTS idx_fx_rates_pair_recorded_desc ON fx_rates (base, quote, recorded_at DESC);
  ```
  The down migration drops the table.
- `000039_add_base_currency_to_users.{up,down}.sql`:
  `ALTER TABLE users ADD COLUMN base_currency VARCHAR(3) NOT NULL DEFAULT 'MYR';`
  The down migration drops the column.
- Test DB setup (`internal/testutil`) must create both (follow how other tables are created there).

### `internal/fx` package (pure, no DB): money × rate math
- `type Rate int64`: fixed point, `RateScale = 10_000_000_000` (1e10). `4.4712` = `44712000000`.
- `ParseRate(s string) (Rate, error)`: exact decimal parsing (no float), at most 10 fractional
  digits (reject more), must be > 0, max `99999999.9999999999` (fits NUMERIC(20,10)). Accepts `"4"`,
  `"4.4712"`, `"0.2236"`. Rejects `""`, `"-1"`, `"0"`, `"1e3"`, `"abc"`, `"1.2.3"`.
- `(Rate) String() string`: canonical decimal with trailing zeros trimmed (`"4.4712"`, `"4"`).
- `RateFromFloat(f float64) (Rate, error)`: rounds to 10 dp. Only for provider input (the oracle side
  does its own formatting, so this is for tests/tools). Rejects NaN, Inf and ≤ 0.
- `Convert(amount int64, r Rate) (int64, error)`: `amount × r / 1e10` computed with `math/big`,
  **round half to even**, error on int64 overflow. Negative amounts are allowed (symmetric rounding).
- `Invert(r Rate) (Rate, error)`: `1/r` rounded half-even to 10 dp, error if the result is 0 or overflows.
- `Mul(a, b Rate) (Rate, error)`: `a×b` rounded half-even, for pivots.
- `Rate` implements `sql.Scanner` / `driver.Valuer`: Value writes the canonical decimal string. Scan accepts
  `string`, `[]byte` (Postgres NUMERIC), `float64` (SQLite REAL, via `RateFromFloat`) and `int64`.
- Unit tests: table-driven, 95%+ coverage, including half-even ties (e.g. 1 × 0.5 → 0, 3 × 0.5 → 2,
  -3 × 0.5 → -2), huge amounts, and Scan/Value round-trips.

### Model `internal/models/fx_rate.go`
`FXRate{ID, Base, Quote, Rate fx.Rate, RecordedAt, Source}`. Immutable time series: no `Base` embed,
no soft delete, UUIDv7 `BeforeCreate` like `SecurityPrice`. `TableName() = "fx_rates"`.
`models.User` gains `BaseCurrency string` with gorm `size:3;not null;default:'MYR'` and json `base_currency`.

### Errors (`internal/errors`)
- `ErrFXRateUnavailable`: code `FX_RATE_UNAVAILABLE`, 503, "Exchange rate unavailable".
- `ErrUnsupportedCurrency`: code `UNSUPPORTED_CURRENCY`, 400, "Currency is not supported".

### Config
`FX_MAX_STALENESS` (Go duration, default `168h`), parsed in `internal/config` like the other durations.

### `services.FXServicer` (interface in `interfaces.go`, impl `fx_service.go`)
```go
type CurrencyPair struct{ Base, Quote string }   // upper-case ISO 4217

type FXQuote struct {
    Base, Quote string
    Rate        fx.Rate
    RecordedAt  time.Time   // for a pivot: the OLDER of the two legs
    Stale       bool        // at - RecordedAt > maxStaleness
    Path        string      // "identity" | "direct" | "inverse" | "pivot:USD"
}

type FXRateInput struct {
    Base, Quote string
    Rate        fx.Rate
    RecordedAt  time.Time
    Source      string     // default "yahoo"
}

type FXServicer interface {
    Rate(from, to string, at time.Time) (*FXQuote, error)
    Rates(pairs []CurrencyPair, at time.Time) (map[CurrencyPair]*FXQuote, error) // pairs with no rate are absent from the map (no error)
    RecordRates(rates []FXRateInput) (int, error)                                 // upsert on (base, quote, recorded_at), DO UPDATE rate, source
    ListRates(base, quote string, from, to time.Time) ([]models.FXRate, error)    // direct pair only, ascending recorded_at
    NeededPairs() (pairs []CurrencyPair, baseCurrencies []string, err error)
    IsSupportedCurrency(code string, at time.Time) bool
}
func NewFXService(db *gorm.DB, maxStaleness time.Duration) FXServicer
```
- Lookup order for `Rate(from, to, at)`: normalise to upper case. Identity → rate 1e10, `RecordedAt = at`,
  never stale. Then the **direct** row with the max `recorded_at <= at`. Then the **inverse** row (`to→from`,
  inverted). Then a **pivot through USD** (`from→USD` × `USD→to`, each leg direct or inverse, only when
  neither side is USD). If both a direct and an inverse row exist, the direct one wins even if it's older.
  Nothing found → `ErrFXRateUnavailable`.
- `Rates`: batch it, so the cost doesn't grow one query per pair. Fetch the latest row `<= at` for every needed
  (base, quote) and its inverse, plus the `X↔USD` legs, in a bounded number of queries (a
  `DISTINCT ON`/correlated-subquery approach like `getLatestPrices` is fine; it must also work on SQLite).
  Then resolve each pair in Go with the same rules as `Rate`.
- `RecordRates` validates each input (ISO 4217 3 upper-case letters, base ≠ quote, rate > 0, non-zero
  time) and returns `ErrInvalidInput` with the index of the first bad row. Store `recorded_at` in UTC.
- `NeededPairs`: `currencies` = distinct upper(currency) over non-deleted **active** accounts ∪ non-deleted
  securities. `baseCurrencies` = distinct `users.base_currency` over active users. Pairs = every
  (c, b) with c ∈ currencies, b ∈ baseCurrencies, c ≠ b, plus (b, USD) for every b ≠ USD (so pivots
  always work). Sorted and deduplicated.
- `IsSupportedCurrency(code, at)`: true when `code` is a 3-letter code that isn't in the
  non-2-decimal list (`BHD, BIF, CLP, DJF, GNF, IQD, ISK, JOD, JPY, KMF, KRW, KWD, LYD, OMR, PYG,
  RWF, TND, UGX, UYI, VND, VUV, XAF, XOF, XPF`, per Q8), **and** (`code == "USD"` or
  `Rate(code, "USD", at)` succeeds, staleness ignored).

### HTTP
Handler `FXHandler` (`internal/handlers/fx_handler.go`), wired in `routes.go` and `cmd/api/main.go`
like the others, with Swagger annotations and godoc on every exported item.

1. **`POST /api/v1/pipeline/fx-rates`** (pipeline group, API key).
   Request `{"rates":[{"base":"USD","quote":"MYR","rate":"4.4712","recorded_at":"2026-11-03T08:00:00Z","source":"yahoo"}]}`.
   Binding: `rates` required, 1–5000, `dive`. Each: `base`/`quote` required + `iso4217`, `rate` required
   string, `recorded_at` required RFC 3339, `source` optional (≤ 32 chars). Parse `rate` with
   `fx.ParseRate` (a bad rate → 400 naming the index). Response `200 {"recorded": n}`.
2. **`GET /api/v1/pipeline/fx-pairs`** (pipeline group). Response
   `{"pairs":[{"base":"USD","quote":"MYR"}],"base_currencies":["MYR"]}`. `pairs` is never null.
3. **`GET /api/v1/fx-rates?base=USD&quote=MYR&from=...&to=...`** (protected, JWT). `base` and `quote`
   required. `from`/`to` are optional RFC 3339 or `YYYY-MM-DD`; default to the last 30 days; the range is
   capped at 3700 days (400 beyond that). Returns `{"base","quote","rates":[{"rate":"4.4712","recorded_at":...}]}`,
   direct pair only.
4. **`GET /api/v1/fx-rates/latest?base=USD&quote=MYR`** (protected). Returns
   `{"base","quote","rate":"4.4712","recorded_at","stale":false,"path":"direct"}` via `FXService.Rate` at
   now. 503 `FX_RATE_UNAVAILABLE` when there is none.

Rates are **always serialised as decimal strings** in JSON (never JSON numbers).

### Profile
- `UpdateProfileSettingsRequest` gains `BaseCurrency *string \`json:"base_currency" binding:"omitempty,iso4217"\``.
  It counts toward "at least one field is required" (update that message).
- `ProfileSettingsUpdate` gains `BaseCurrency *string`. The service upper-cases it and rejects it with
  `ErrUnsupportedCurrency` unless `fxService.IsSupportedCurrency(code, now)` or it equals the user's current
  base currency. Inject the FX service into the user service via its constructor (update every caller and test).
- `UserResponse` (the GET/PATCH profile and the auth responses that embed the user) includes `base_currency`.
- Audit a change as action `UPDATE_BASE_CURRENCY` with old/new in the details (follow the existing
  audit-log pattern in the auth handler).

### Tests (match existing styles)
- `internal/fx`: as above.
- `fx_service_test.go` (SQLite): identity / direct / inverse / pivot, `<= at` selection (a future row
  is ignored), direct-beats-inverse, staleness flag, unavailable, `Rates` batch results equal per-pair
  `Rate`, `RecordRates` upsert (same key → updated, count), validation errors, `NeededPairs` (inactive or
  deleted accounts excluded, multiple bases, the USD pivot pair), `IsSupportedCurrency`.
- Handler tests with mock services: all 4 endpoints (happy path, validation, string-rate parsing, 503),
  profile `base_currency` (valid, unsupported, iso-invalid, it alone satisfies "at least one").
- Integration (`tests/integration`): pipeline posts USD→MYR, then `GET /fx-rates/latest` for MYR→USD
  returns the inverse.

### Docs
Regenerate Swagger if `swag` is available; otherwise say so (don't hand-edit `internal/docs`).
`CLAUDE.md` (API endpoint list, env var list) and `.env.prod.example` (`FX_MAX_STALENESS`).

## Oracle (`apps/oracle`)

Phase 1 adds rate publishing. **Price conversion stays exactly as it is** (it's removed in Phase 2).

- `client.KuberanClient`:
  - `GetFXPairs(ctx) (FXPairs, error)` → `GET /api/v1/pipeline/fx-pairs`, with
    `FXPairs{Pairs []CurrencyPair; BaseCurrencies []string}`.
  - `RecordFXRates(ctx, []FXRateEntry) (int, error)` → `POST /api/v1/pipeline/fx-rates`, with
    `FXRateEntry{Base, Quote, Rate string, RecordedAt string /*RFC3339*/, Source string}`. Batches of ≤ 5000.
  - Same auth, error handling and retry conventions as the existing methods.
- `provider.ForexConverter`: generalise the fetching to any pair: `FetchRate(ctx, base, quote) (float64, error)`
  using Yahoo `BASEQUOTE=X` (reuse `fetchRate`'s parsing and the cache, keyed by pair). `GetRate`/`Convert`
  keep working for the target currency (implemented on top of it).
- `oracle.Run` order becomes:
  1. fetch securities (unchanged); 2. fetch prices from providers (unchanged, in memory);
  3. **publish FX rates**: pairs = `GetFXPairs().Pairs` ∪ {(c, b) for every distinct provider-reported
     currency c in this run's results × every b in `BaseCurrencies`, c ≠ b} ∪ {(c, TARGET_CURRENCY) for those
     currencies}. Deduplicate. Fetch each rate (reusing the converter cache, so step 4 doesn't refetch), format
     with `strconv.FormatFloat(rate, 'f', 10, 64)`, and use `recorded_at` = this run's start time in UTC,
     truncated to the minute, `source="yahoo"`. Post them. **Any failure here is logged and recorded in
     `result.Errors` but never aborts the run.** Rates that fail to fetch are skipped individually.
     `result.FXRatesRecorded` (new field) holds the count, and the summary log includes it;
  4. convert prices (unchanged step 5b); 5. record prices; 6. snapshots; 7. fixed deposits (unchanged).
- Add a `PublishFX` config flag (`PUBLISH_FX_RATES`, default `true`) so it can be turned off.
- Tests: client methods (httptest server), the pair union and dedupe, the run order (rates are posted before
  prices/snapshots, using a fake client that records call order), failure isolation (an FX post error →
  prices are still recorded), and the cache reuse (one Yahoo fetch per pair per run).
- Docs: the oracle env vars in `CLAUDE.md` / `.env.prod.example` if they're listed there.
