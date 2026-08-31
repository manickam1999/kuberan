# Transaction Rules API contract (frozen 2026-08-31)

Single source of truth for backend and web. Decisions locked: D1 multi-condition, D3 create-time +
backfill only, D5 child actions table, D6 terminal first-match-wins, D7 category-type safety, D8
balance-neutral backfill. Do not deviate without lead sign-off.

## Semantics

- A rule = **conditions (AND) → actions**. Separate rules act as **OR**.
- Auto-categorization fires on **create only**, when the client sends no `category_id` and the
  transaction `type` is `income` or `expense`. An explicit `category_id` is never overridden. Editing a
  transaction never re-runs rules.
- Excluded by construction (never auto-categorized): transfers, investment cash legs, account
  initial-balance transactions.
- Evaluation: active rules ordered by `priority ASC, created_at ASC`; the **first** rule whose
  conditions all match and whose `set_category` target is valid (exists, not soft-deleted, and its
  `type` equals the transaction's `type`) assigns the category, then evaluation stops.
- Money is always **int64 cents**.

## Models

```
TransactionRule {
  id          string    // UUIDv7
  user_id     string
  name        string    // 1..120
  priority    int       // lower = evaluated first
  is_active   bool
  conditions  RuleCondition[]
  actions     RuleAction[]
  created_at  string    // ISO 8601
  updated_at  string
}

RuleCondition {
  field       "description" | "amount" | "account_id" | "type"
  operator    "contains" | "not_contains" | "equals" | "starts_with" | "ends_with"  // description
            | "gt" | "lt" | "between"                                                // amount
            | "equals"                                                               // account_id, type
  value_text  string | null   // description text / account UUID / type value ("income"|"expense")
  amount_min  int64  | null    // cents; gt, between
  amount_max  int64  | null    // cents; lt, between
}

RuleAction {
  action_type  "set_category"           // v1 (reserved: "add_tag", "rename", "hide")
  category_id  string | null            // for set_category
  category?    Category                 // preloaded on read
}
```

Validation (server, at write time → 400/409-style AppError):
- operator must be allowed for the field (matrix above); `amount_min <= amount_max` for `between`.
- `type` condition value ∈ {income, expense}; `account_id` value must be the user's account.
- `set_category` target must exist, belong to the user, not be soft-deleted, and (checked again at
  match time) match the transaction type.

## Endpoints (all under `/api/v1`, Bearer auth)

| Method | Path | Body | Response |
|--------|------|------|----------|
| POST | `/rules` | `{name, priority?, is_active?, conditions[], actions[]}` | `{ "rule": TransactionRule }` |
| GET | `/rules` | – | `{ "rules": TransactionRule[] }` (priority order) |
| GET | `/rules/:id` | – | `{ "rule": TransactionRule }` |
| PUT | `/rules/:id` | `{name?, is_active?, conditions?, actions?}` | `{ "rule": TransactionRule }` |
| DELETE | `/rules/:id` | – | `{ "message": string }` |
| POST | `/rules/reorder` | `{ rule_ids: string[] }` | `{ "rules": TransactionRule[] }` |
| POST | `/rules/preview` | `{ conditions: RuleCondition[] }` | `{ "count": int, "sample": Transaction[] }` |
| POST | `/rules/:id/apply` | `{ scope, overwrite, dry_run }` | `{ "count": int, "sample": Transaction[], "applied": int }` |

`apply` request:
```
{
  scope:     "uncategorized" | "all"   // default "uncategorized"
  overwrite: bool                       // default false (skip already-categorized)
  dry_run:   bool                       // default true (return count+sample, write nothing)
}
```
`applied` is 0 when `dry_run` is true. Backfill is **balance-neutral** - it updates `category_id`
only and never touches account balances.

Register static `/rules/reorder` and `/rules/preview` **before** the param route `/rules/:id`. If Gin
panics on the static/param mix, fall back to `/rules/-/reorder` and `/rules/-/preview` and tell the
lead so the frontend matches.

## Rules baked into the contract

- Conditions within a rule are AND-ed; rules are OR-ed. Terminal first-match-wins by priority.
- Auto-categorization is create-time only and silent (no audit entry per assignment). Rule CRUD and
  `apply` are audit-logged handler-side.
- Deleting a category deactivates (`is_active=false`) any rule whose action targets it.
- Reorder rewrites `priority` to the given order in one transaction; ties broken by `created_at`.

## Hard safety rules (every stream)

- NO destructive DB actions on the local database (no DROP/TRUNCATE/DELETE-all, no `migrate down`
  that loses data). Create/update only.
- NEVER touch production. NEVER touch Supabase.
- Go tests use in-memory SQLite (default) - no external DB needed.
