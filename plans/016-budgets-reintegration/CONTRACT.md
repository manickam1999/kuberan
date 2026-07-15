# Budgets API contract (frozen 2026-07-15)

This is the single source of truth all three streams (backend, web, bot) build against.
Decisions D1 (recurring, no dates), D2 (`is_active` writable), D3 (batch progress), D4 (unique
active per category+period) are locked. Do not deviate without lead sign-off.

## Model

```
Budget {
  id           string   // UUIDv7
  user_id      string
  category_id  string
  name         string   // 1..100
  amount       int64    // cents, > 0
  period       "monthly" | "yearly"
  is_active    bool
  created_at   string   // ISO 8601
  updated_at   string
  category?    Category // preloaded
}
```

**No `start_date` / `end_date`.** Budgets are recurring calendar-period caps; progress is always
computed against the current month/year (per `time.Now()`), which is now the correct and only behavior.

```
BudgetProgress {
  budget_id   string
  budgeted    int64   // cents (the budget amount)
  spent       int64   // cents (sum of expense txns in category for current period)
  remaining   int64   // cents, may be negative when over budget
  percentage  float64 // spent/budgeted*100, may exceed 100
}
```

## Endpoints (all under `/api/v1`, Bearer auth)

| Method | Path | Body | Response |
|--------|------|------|----------|
| POST | `/budgets` | `{category_id, name, amount, period}` | `{ "budget": Budget }` |
| GET | `/budgets?is_active&period&page&page_size` | – | `PageResponse<Budget>` |
| GET | `/budgets/:id` | – | `{ "budget": Budget }` |
| PUT | `/budgets/:id` | `{name?, amount?, period?, is_active?}` | `{ "budget": Budget }` |
| DELETE | `/budgets/:id` | – | `{ "message": string }` |
| GET | `/budgets/:id/progress` | – | `{ "progress": BudgetProgress }` |
| **GET** | **`/budgets/progress`** | – | `{ "progress": BudgetProgress[] }` **(NEW, active budgets only)** |

Register the static `/budgets/progress` route **before** the param route `/budgets/:id` (and its
`/progress` child) so Gin resolves it correctly; if Gin panics on the static/param mix, fall back to
`/budgets/overview` and tell the lead so the frontend/bot can match.

## Rules baked into the contract

- **D4 uniqueness:** creating a second *active* budget for the same `(category_id, period)` must fail
  with a 409-style `AppError` (e.g. `BUDGET_ALREADY_EXISTS`). Inactive duplicates are allowed.
- **D2 `is_active`:** writable via PUT. An inactive budget is excluded from the batch
  `/budgets/progress` response.
- **Expense categories only (D6):** enforced in the web picker; backend still validates the category
  belongs to the user. (Progress only sums `type='expense'` txns, so income budgets read 0 anyway.)
- Money is always int64 cents. Percentage/remaining are computed, not stored.

## Hard safety rules (every stream)

- NO destructive DB actions on the local database (no DROP/TRUNCATE/DELETE-all, no `migrate down`
  that loses data). Create/update only.
- NEVER touch production. NEVER touch Supabase.
- Go tests use in-memory SQLite (already the default) - no external DB needed.
