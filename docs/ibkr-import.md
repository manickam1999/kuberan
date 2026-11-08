# IBKR Activity Statement import

`POST /api/v1/imports/ibkr` rebuilds your Interactive Brokers holdings and the USD cash ledger from Activity Statement
CSVs, so Kuberan matches the statements: native USD trade amounts, base-currency figures frozen at each trade date's
exchange rate, dividends, fees and the cash balance. It is a dry run unless you pass `apply=true`.

Everything happens in memory: the files are never written to disk or storage, and the server logs counts only, never
file contents or the account holder's name.

## 1. Download the statements

In Client Portal: **Performance & Reports > Statements > Activity**. Choose **Annual** (or **Custom Date Range**), the
period, format **CSV**, and download. Use one file per period. Several files must be for the same account and base
currency and be contiguous (the next one starts the day after the previous one ends). Up to 10 files per import.

## 2. Prepare Kuberan

- The securities must already exist, with the right currency (run the Phase 2 `oracle reconcile` first). The import
  never creates securities; it matches by symbol and listing exchange (`LSEETF` is `LSE`).
- FX history must cover the trade dates. A missing rate is reported as `fx_rate_missing`; run
  `oracle backfill --fx --since <first statement date>` and try again.
- Create a **cash account in the statement's base currency** (USD) to receive the cash ledger, and know the id of the
  investment account that holds the IBKR positions. If you previously tracked IBKR cash in another currency, pass that
  account as `legacy_cash_account_id`.

## 3. Dry run first

```bash
curl -sS -X POST "$API/api/v1/imports/ibkr" \
  -H "Authorization: Bearer $TOKEN" \
  -F files=@U1234567_2025.csv \
  -F files=@U1234567_2026.csv \
  -F investment_account_id=$IBKR_INVESTMENT_ACCOUNT_ID \
  -F cash_account_id=$IBKR_USD_CASH_ACCOUNT_ID \
  -F cash_start_date=2026-01-01 \
  -F legacy_cash_account_id=$OLD_IBKR_CASH_ACCOUNT_ID \
  | jq
```

| field | required | meaning |
|---|---|---|
| `files` | yes (1-10) | Activity Statement CSVs, each at most `MAX_UPLOAD_BYTES` |
| `investment_account_id` | yes | investment account the holdings belong to |
| `cash_account_id` | yes | cash account in the statements' base currency |
| `cash_start_date` | yes | `YYYY-MM-DD`; cash rows are written from this date. A statement must end the day before: its Ending Cash becomes the opening balance |
| `legacy_cash_account_id` | no | old IBKR cash account whose incoming transfers are re-pointed at `cash_account_id` |
| `deactivate_legacy_cash_account` | no | `true` deactivates the legacy account after apply |
| `apply` | no | `true` writes; the default is a dry run |

Read the report:

- `trades_after_statement` means an investment already has trades dated after the last statement ends. They are never
  deleted; export a statement that runs through today and include it.

- `errors` must be empty (`unknown_security`, `currency_mismatch`, `fx_rate_missing`, `insufficient_shares`,
  `opening_balance_missing`, ...). Any error, or any `securities[].checkpoints[].quantity_ok: false`, makes an apply fail
  with `400 IMPORT_INVALID` and the report under `error.details.report`.
- `securities[].checkpoints` compare the replay with IBKR's Open Positions at each statement end. Quantities must agree
  within 1e-4. `cost_basis_diff` is only a warning: Kuberan uses average cost, IBKR tracks lots.
- `cash.checkpoints` compare the ledger with IBKR's Ending Cash; a difference of at most one cent per row is rounding.
- `cash.deposits_matched` lists the legacy transfers that will be re-pointed (within 5 days of the deposit, one to one,
  in deposit date order; the transfer's implied rate must be within 3% of the market rate, the closest wins, then the
  nearest date). `cash.unmatched_deposits` lists deposits and withdrawals that have no transfer; record those by hand (each lists the rejected `candidates` with their rate deviation).
  The cash ledger excludes them, so the final cash checkpoint shows them as `diff`.

## 4. Apply

Repeat the same request with `-F apply=true` (and `-F deactivate_legacy_cash_account=true` if you want to retire the old
account). All changes happen in one database transaction; any error rolls everything back.

What apply does:

1. For each symbol, soft-deletes the investment's existing transactions, inserts the statement's buys, sells, splits and
   dividends (USD amounts, `fx_rate_to_base` and `base_amount` from the trade date's rate) and rebuilds the holding.
   Missing investments are created.
2. In the cash account: replaces its opening balance, then writes a cash leg per trade, an income per dividend (net of
   withholding and fees) and an expense per standalone fee, all from `cash_start_date`. Descriptions start with
   `IBKR import:` and each row carries an `import_ref` (`ibkr:<account>:<event key>:dr|cr`, the balance direction the re-import reverses by).
3. Re-points each matched legacy transfer: its destination becomes the new cash account, `to_amount` the USD deposit, the
   source keeps its amount; the legacy account's balance is reduced and the new account's increased.

Running the same import again is safe: the previous import's rows are found by `import_ref`, reversed and rewritten, and
the already re-pointed transfers are recognised, so counts and balances do not change. Only one apply per user runs at a
time (`409 IMPORT_IN_PROGRESS` otherwise).

An audit log entry `IMPORT_IBKR` (counts only) is written on apply.
