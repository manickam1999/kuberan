# Receivables API contract (frozen 2026-10-20)

This is the single source of truth for the backend, web, MCP and bot work on plan 021. Read it
together with `021-shared-expenses.md`, whose D-numbers are referenced here. **Do not deviate
without lead sign-off.** If something is ambiguous, stop and report it rather than guessing.

## Semantics (summary)

- A **shared expense** is an `expense` transaction with ≥1 receivable. The full `amount` leaves the
  account. Only the **effective amount** `COALESCE(personal_amount, amount)` counts as spending in
  every report and budget.
- Invariant (D6), maintained only by the server:
  `personal_amount = amount − Σ(receivable.amount − receivable.written_off_amount)` over
  non-deleted receivables, or `NULL` when there are none.
- A **settlement** creates a `reimbursement` transaction (a credit to an account, never income) and
  increases `paid_amount`.
- A **write-off** increases `written_off_amount`; that money becomes the user's spending, dated on
  the original expense.
- `outstanding = amount − paid_amount − written_off_amount`.
- **Net worth is unchanged by this feature**: receivables are excluded (D4), and no snapshot change
  is made.
- Money is always **int64 cents**. Every query is scoped by `user_id`.

## Enums

```
ReceivableKind   = "split" | "on_behalf" | "claim" | "loan" | "deposit" | "refund"
ReceivableStatus = "pending" | "partially_paid" | "settled" | "written_off"
TransactionType  += "reimbursement"   // server-created only; NOT accepted by POST/PUT /transactions
ShareMode        = "equal" | "custom" | "full"
```

Status derivation, recomputed on every change:
- `pending`: paid = 0, written_off = 0, outstanding > 0.
- `partially_paid`: outstanding > 0, and paid > 0 or written_off > 0.
- `settled`: outstanding = 0, written_off = 0.
- `written_off`: outstanding = 0, written_off > 0.

Counterparty name normalization (in Go, used everywhere):
- `name = strings.Join(strings.Fields(raw), " ")`, which must be 1..100 runes after normalization,
  else `INVALID_SHARES`;
- `key = strings.ToLower(name)`.

## Models (JSON)

```
Transaction (existing, additions)
  personal_amount   number | null      // cents; null = not shared
  receivables?      Receivable[]       // preloaded on list AND detail for expenses (no settlements on list)

Receivable
  id                string
  user_id           string
  transaction_id    string             // the expense
  counterparty_name string
  counterparty_key  string
  kind              ReceivableKind
  amount            number             // cents, > 0
  paid_amount       number
  written_off_amount number
  outstanding       number             // derived, not stored (gorm:"-")
  status            ReceivableStatus
  created_at, updated_at  string
  settlements?      ReceivableSettlement[]   // GET /receivables/:id only
  transaction?      ReceivableExpense        // on /receivables list + detail

ReceivableExpense  // trimmed expense view
  id, description, date, amount, personal_amount, account_id, category_id

ReceivableSettlement
  id                string
  receivable_id     string
  transaction_id    string             // the reimbursement transaction
  account_id        string             // account credited (stored, denormalized)
  amount            number
  date              string
  created_at        string
```

Schema note: `receivable_settlements` also has `account_id UUID NOT NULL`.

## Endpoints (all protected; responses wrap as shown)

### POST /api/v1/transactions/shared → 201 `{ "transaction": Transaction }` (with receivables)

```
{
  account_id: string,               // required
  category_id?: string | null,
  amount: number,                   // required, > 0
  description?: string,             // max 500
  date?: string,                    // same parsing as POST /transactions (parseFlexibleTime); default now
  kind: ReceivableKind,             // required
  mode: ShareMode,                  // required
  counterparties: [{ name: string, amount?: number }]   // required, 1..20 entries
}
```

Rules:
- `equal`: amounts must be omitted. With `n` counterparties, `share = amount / (n+1)` (integer
  division) and `personal = amount − n·share`. If `share == 0`, return `INVALID_SHARES`.
- `custom`: every amount is required and > 0, and Σ ≤ `amount` (personal may be 0).
- `full`: exactly 1 counterparty with amount omitted. That counterparty owes `amount`, and personal
  is 0.
- Duplicate counterparty keys within one request: `INVALID_SHARES`.
- Category: if omitted and the mode is not `full`, run rule auto-categorization exactly as
  `CreateTransaction` does. In `full` mode, leave it NULL.
- The type is always `expense`.

### PUT /api/v1/transactions/:id/receivables → 200 `{ "transaction": Transaction }`

```
{ kind?: ReceivableKind, receivables: [{ id?: string, name: string, amount: number }] }
```

This is a diff (D7), not a replacement:
- An entry with `id` updates the name and amount; it requires `amount ≥ paid + written_off`.
- An entry without `id` is inserted.
- An existing receivable that is omitted is soft-deleted, only if `paid_amount = 0`; otherwise 409
  `RECEIVABLE_HAS_PAYMENTS`.
- Σ amounts must be ≤ the expense `amount`, else `INVALID_SHARES`.
- `kind`, if given, applies to all of the expense's receivables.
- An empty list removes all receivables (subject to the rule above), so `personal_amount` becomes
  NULL.
- 400 `INVALID_SHARES` if the transaction is not an expense.

### GET /api/v1/receivables → 200 `PageResponse<Receivable>`

Query: `status` (one of the statuses, or `open` = pending|partially_paid), `kind`, `counterparty`
(a key), `page`, `page_size`. Ordered by expense date desc. Includes `transaction`.

### GET /api/v1/receivables/summary → 200

```
{
  total_outstanding: number,
  by_kind: [{ kind, outstanding, open_count }],
  by_counterparty: [{ counterparty_key, counterparty_name, outstanding, open_count, oldest_date }]
}
```

Open receivables only (outstanding > 0); expense and receivable both non-deleted.
`counterparty_name` is the name on the most recently created receivable for that key.
`by_counterparty` is sorted by outstanding desc.

### GET /api/v1/receivables/counterparties?q= → 200 `{ "counterparties": [{ key, name }] }`

Distinct keys whose key starts with or contains the lowercased `q`; max 20, ordered by most
recent use.

### POST /api/v1/receivables/counterparties/rename → 200 `{ "updated": number }`

`{ from_key: string, to_name: string }`. It renames every one of the user's receivables with
`from_key`, in one DB transaction. Renaming onto an existing key merges them.

### GET /api/v1/receivables/:id → 200 `{ "receivable": Receivable }` (with settlements + transaction)

### POST /api/v1/receivables/:id/settle → 201 `{ "receivable": Receivable, "settlement": ReceivableSettlement }`

```
{ account_id: string, amount: number, date?: string, description?: string, write_off_remainder?: boolean }
```

- `amount` must be > 0 and ≤ outstanding, else `SETTLEMENT_EXCEEDS_OUTSTANDING`.
- The account type must be cash, credit_card or debt, else `INVALID_SETTLEMENT_ACCOUNT`.
- The default description is `"Repayment from {name}: {expense description}"`, truncated to 500
  characters.
- The reimbursement transaction has a NULL category.
- `write_off_remainder` writes off whatever is still outstanding after this payment, in the same DB
  transaction.

### DELETE /api/v1/receivables/settlements/:id → 200 `{ "receivable": Receivable }`

Reverses the reimbursement (soft-deletes that transaction and debits the account by relative
update) and recomputes the receivable.

### POST /api/v1/receivables/:id/write-off → 200 `{ "receivable": Receivable }`

`{ amount?: number }`. The default is the whole outstanding balance. It must be > 0 and
≤ outstanding, else `WRITE_OFF_EXCEEDS_OUTSTANDING`.

### DELETE /api/v1/receivables/:id/write-off → 200 `{ "receivable": Receivable }`

Resets `written_off_amount` to 0. If there is nothing to undo, return 400 `INVALID_INPUT`.

### Changes to existing endpoints

- `GET /transactions` and `GET /accounts/:id/transactions`: accept `type=reimbursement`. Expense
  rows include `personal_amount` and `receivables`.
- `POST /transactions` and `PUT /transactions/:id`: reject `type=reimbursement` (validator).
- `PUT /transactions/:id` on a reimbursement: 400 `TRANSACTION_NOT_EDITABLE`.
- `PUT /transactions/:id` on a shared expense:
  - changing `type` away from expense → 400 `INVALID_TYPE_CHANGE`;
  - a new `amount` < Σ(receivable.amount) → 400 `INVALID_SHARES`;
  - otherwise recompute `personal_amount`.
- `DELETE /transactions/:id`:
  - on a reimbursement → 400 `TRANSACTION_NOT_EDITABLE` (delete it via its settlement);
  - on a shared expense with any settlement → 409 `RECEIVABLE_HAS_SETTLEMENTS`;
  - otherwise it cascades a soft-delete of its receivables.
- Report endpoints (spending-by-category, cashflow, monthly-summary, daily-spending,
  daily-summary, top-expenses, budget progress) use the effective amount. `top-expenses` items
  gain `personal_amount: number | null`; their `amount` stays the full amount, they are ordered
  by the effective amount, and rows with an effective amount of 0 are excluded. The response also gains `total_count` (the number of expenses in range with a non-zero effective amount, ignoring `limit`; added after E2E). `spending-by-category` / cashflow omit categories whose effective total is 0.

## Errors (`{ "error": { "code", "message" } }`)

| Code | HTTP |
|---|---|
| RECEIVABLE_NOT_FOUND | 404 |
| SETTLEMENT_NOT_FOUND | 404 |
| RECEIVABLE_HAS_SETTLEMENTS | 409 |
| RECEIVABLE_HAS_PAYMENTS | 409 |
| SETTLEMENT_EXCEEDS_OUTSTANDING | 400 |
| WRITE_OFF_EXCEEDS_OUTSTANDING | 400 |
| INVALID_SHARES | 400 |
| INVALID_SETTLEMENT_ACCOUNT | 400 |
| INVALID_RECEIVABLE_KIND | 400 |

## Concurrency (D10; no automated race test, by decision)

Every mutating receivable operation runs in one DB transaction:
1. lock the expense row, then its receivables (ordered by id), then the account(s) (ordered by id),
   using `clause.Locking{Strength: "UPDATE"}`;
2. re-read state after locking;
3. change balances only via the relative `ApplyBalanceDelta`
   (`gorm.Expr("balance + ?", delta)`).

## Audit actions (logged in handlers)

`CREATE_SHARED_EXPENSE`, `UPDATE_RECEIVABLES`, `SETTLE_RECEIVABLE`, `DELETE_SETTLEMENT`,
`WRITE_OFF_RECEIVABLE`, `UNDO_WRITE_OFF`, `RENAME_COUNTERPARTY`; resource type `receivable` or
`transaction`.
