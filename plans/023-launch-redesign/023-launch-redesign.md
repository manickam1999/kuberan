# Plan 023 — Launch Redesign

**Status: all phases (0–7) implemented** (migrations `000048`–`000057`: notes/source, backup runs, audit source, grant
client host, appearance, account balance snapshots, rule stats, loan columns, reminders, demo rows). Phase 6 added
`POST /transactions/suggest-category` (bot quick add); Phase 7 added `apps/site`, `docker-compose.demo.yml` with a
nightly reset (`deploy/demo/`), the self-hosting, Telegram and AI-assistant guides, and `pnpm screenshots`. Additions beyond the plan text: `GET /accounts?include_inactive=true`; `NEXT_PUBLIC_MCP_URL` (optional) for
the "Connect an assistant" steps; `GET /system/status` also reports `demo_enabled` / `demo_loaded`; CSV-imported rows
stay editable (only `ibkr:` rows are locked); `DELETE /demo` also removes what the user added inside sample accounts
and categories, and refuses with `DEMO_IN_USE` where removal would damage real data; reminders are one row per
occurrence (`due_on`), 3 days before a card or loan due date and 7 before a deposit matures; investment accounts
record balance history going forward only (portfolio history is per user, not per account).

Phase 1 notes:

- Wrong current passwords return 400 `INVALID_CURRENT_PASSWORD`, not 401, so the web client doesn't mistake them
  for an expired token. The current device is found from the refresh cookie; no token claim was added.
- MCP activity summaries tag amounts with the ISO code (`MYR 18.71`), the MCP convention.
- The first paint of a first-ever visit uses Catppuccin and switches to the server's `DEFAULT_PALETTE` once
  `GET /appearance` loads; the cookie mirror prevents the flash after that.
- `density` and `show_cents` are stored and validated but have no UI yet (Phase 4). Where Phase 0's code differs from the plan below:

- **0.3 steps down, not up.** A new account's few days of snapshots look the same at 6M and 1M (both grouped
  by day), so a longer range never helps. The chart defaults to 6M and, until the user picks a range, falls back
  to 1W (hourly), then 1D, while it has fewer than 8 points.
- **0.5 wasn't a bug.** The empty investment and net-worth charts in the review came from full-page Playwright
  screenshots, which resize the viewport and restart Recharts' entry animation. The live DOM has the full path at
  2, 5 and 9 seconds. Nothing was changed, and bug 6 in the Context section is withdrawn.
- **0.2 includes the picker.** The API now rejects free-text icons, so the category dialogs got a curated icon
  grid plus a one-emoji field now instead of in Phase 4. Icons are validated on create, and on update only when
  they change, so categories with legacy free-text icons can still be edited. The bot shows only emoji icons and
  asks again when a new category's icon isn't a single emoji; it used to cut any text to 2 characters.
- **0.4 (opening balance)** shipped as migration `000037_add_opening_balance_type`: PR #38 (security and
  performance hardening) took `000035` and `000036`, and this branch is stacked on it.
  - The down migration turns every `opening_balance` row back into `income`, which is exactly the old scheme, so
    no marker column is needed.
  - The monthly and daily summaries no longer filter on `description != 'Initial balance'`. That filter also hid
    real income with that name.
  - An opening balance moves the balance in the account's own sign convention (never inverted), so it is already
    correct for the credit-card `current_balance` in item 1.9. That field itself is still to do.
- **0.6 shipped as `formatCompactAmount(cents)`** (`apps/web/src/lib/format.ts`), not `formatCompactCurrency`. It
  prints no currency symbol; the calendar adds the sign (U+2212).
- **0.7 also covers** the account page's own type map, the cash-flow card's expense figures and bar, the sankey
  "Out" total, top-expense amounts, calendar cells and negative day totals. Months and savings with a negative net
  stay red.
- **Frontend unit tests.** Phase 0 shipped without them; it was checked with `tsc`, `next lint`, `next build` and
  Playwright against a seeded local stack. PR #61 has since added Vitest (`pnpm test`, `vitest.config.mts`,
  `src/lib/money-expr.test.ts`), but CI doesn't run it yet. Follow-up: run `pnpm test` in CI and add the Phase 0
  tests (`CategoryIcon` fallback, `formatCompactAmount`, net-worth auto-period).
- **Stacked on PR #38.** #38 moved the daily and monthly summaries into one grouped query
  (`services/period_totals.go`), carrying the `description != 'Initial balance'` filter with it; Phase 0 replaces
  that filter with a type check there, and turns #38's "Initial balance" income fixture into a real opening
  balance. On top of #38, `./scripts/check-go.sh apps/api` passes in full with CI's golangci-lint v2.15.0.

Design references:

- Review and proposal: https://claude.ai/artifact/XBWoNQHMj4zwZv7i7tDFgg
- Clickable prototype, 37 screens on desktop and mobile web: https://claude.ai/artifact/WFQR9ag4P6MaMBSLdxZAdf

Both artifacts are private to the repo owner. The prototype is the visual spec for this plan. Where this plan
and the prototype disagree, this plan wins; the prototype will be updated to match.

## Backend re-audit (9 Nov 2026)

Re-checked against `main` at `3b3bea4`, after Phase 0 (#39), the security hardening (#38) and multi-currency
(plan 024, #49 and later) landed. A second pass on the same day checked every claim in this plan against the code;
the corrections are folded into the sections below.

**Migration numbers.** `main` runs to `000047`. Numbers in this plan (`000048`–`000051`) are placeholders in item
order. Assign the real number when a PR merges, in merge order: golang-migrate only applies versions above the
current one, so a lower number merged after a higher one is silently skipped in production.

**Done since the plan was written:**

| Plan item | Where |
|---|---|
| 1.1 Base currency (mostly) | Plan 024. `users.base_currency` (migration 39, default `'MYR'`), `PATCH /profile` accepts it (locked once there is investment history), FX rates and conversion on read for every aggregate, `formatMoney(cents, currency)` with a required currency (`en-MY`), and a `<Money>` component with FX tooltips. Two gaps remain: see 1.1 below. |
| 1.2 Opening balance type | Phase 0 (#39), migration `000037`. |
| Category icon rules | Phase 0 (#39). |
| Part of 5.8 (import) | Plan 024 added `POST /imports/ibkr` (IBKR Activity Statements). A generic bank CSV import is still missing. |
| Frontend groundwork | `formatCurrency` is gone (plan 024's `formatMoney`, 43 files); `components/ui/money.tsx` exists; the dialog is already a bottom sheet on phones (`max-h-[92dvh]`, grab handle, safe-area padding); inputs are 16px on mobile; money fields use `inputMode="decimal"` and support arithmetic (#61). |

**Still missing, needed before launch (Phase 1):**

| # | Need | Prototype screens that use it | Today |
|---|---|---|---|
| 1.1 | Base currency at sign-up; new accounts default to it | Create account, New account | `RegisterRequest` has no `base_currency`; `Create*Account` hard-codes `"USD"` when the currency is empty |
| 1.3 | Transaction text search, `notes`, `source`, `shared` / `has_attachment` filters | Transactions, ⌘K search, transaction detail | Filters are account, category, type, dates and amount only; no notes or source columns |
| 1.4 | Bulk edit (`POST /transactions/bulk`) | Transactions (bulk bar) | None |
| 1.5 | Edit name and email, change password, list and revoke signed-in devices | Settings → Profile & security | `PATCH /profile` takes only `hide_balances`, `default_account_id`, `base_currency`; `refresh_sessions` exist but aren't exposed |
| 1.6 | MCP change log, write limits, accurate "last used" (reads too), disconnect that stops reads within a minute, and the app's host | Settings → Connected apps, "via Claude" on transactions | MCP writes are audited with `via`/`client_id` inside the `changes` JSON, but `audit_logs` has no columns for them and nothing reads them back; limits live only in config |
| 1.7 | Data export and server status | Settings → Data, sidebar host chip | None |
| 1.8 | Appearance preferences on the profile, plus a server default palette (`DEFAULT_PALETTE`, Catppuccin when unset) | Settings → Appearance, theme picker, sign-in pages | Theme and chart style live in each browser's `localStorage` only |
| 1.9 | Credit-card "current balance owed" on create (moved from Phase 0) | New account | `CreateCreditCardAccountRequest` has no opening amount |

**Still missing, can follow launch (Phase 5 and 6):**

| # | Need | Today |
|---|---|---|
| 5.1 | Per-account balance history and sparklines | Snapshots hold totals only |
| 5.2 | Rule match counts and suggestions | Preview and apply exist; no counts |
| 5.3 | Loan (`debt`) account creation | The type exists, but there's no `POST /accounts/debt` |
| 5.4 | "Typical" averages for pace and "vs typical" | Derivable from many summary calls; no endpoint |
| 5.5 | Security region (allocation by region) | No field |
| 5.6 | Reminders (card due, deposit maturity) | None |
| 5.7 | Demo data load and delete | None |
| 5.8 | Generic bank CSV import | Only the IBKR importer |
| 6.x | Bot one-line quick add, `/undo`, receipt photos, `suggest-category` endpoint | The bot has guided flows and `/balance`, `/summary`, `/budgets`, `/owed`, `/accounts`, `/categories` |

**Frontend-only gaps the prototype needs after plan 024.** The prototype assumed one currency (RM everywhere).
Before Phase 4 it should show:

- each amount in its own currency, with the converted total;
- FX transfers (`to_amount` and the implied rate);
- the IBKR import screen;
- the reporting-currency setting and its lock;
- the price-versus-FX split of investment returns.

None of these need new API.

## Context

This section describes the original review (31 Oct, `main` at `8213076`). Later work has changed some of it; the
re-audit above is current.

Before launch, the web app was reviewed against a seeded demo: four accounts, about 140 transactions over
six months, five holdings, and weekly snapshots. The engineering is solid: OKLCH tokens, tabular figures,
React Query key factories, ⌘K, and the dither chart theme. But the UI reads as a stock shadcn template,
and several bugs are visible to a first-time user.

The prototype redesigns every screen as **one responsive layout**. Mobile is the same Next.js app at phone
width, not a separate design. That's deliberate: a native app is a long way off, so the mobile web app (and
a Home Screen PWA) is what mobile users will get for the foreseeable future.

Checking the prototype against the API showed that most screens are already backed. Some aren't:

| Area | Backed today | Missing |
|---|---|---|
| Accounts | Create cash, card, investment and fixed deposit; update; pin and archive; close a fixed deposit | Loan (`debt`) creation, opening balance for credit cards, per-account balance history |
| Transactions | CRUD, transfer, attachments, shared expenses, filters (account, category, type, dates, amount) | Text search, notes separate from description, source ("via Telegram/Claude"), bulk edit |
| Analytics | Cash flow (sankey), daily spending, top expenses, monthly summary, spending by category | Opening balance excluded from income, "vs typical" and spending-pace data |
| Rules | CRUD, reorder, preview, apply | Match counts, suggestions |
| Profile and security | `PATCH /profile` (`hide_balances`, `default_account_id`), logout | Name and email edit, password change, base currency, device list and revocation |
| AI and MCP | OAuth consent with scopes and write opt-in, connected apps list (`scopes`, `last_used_at`), disconnect | Reading the MCP change log; MCP writes are audited to `audit_logs` but nothing reads them back |
| Data | Nothing | CSV export and import, backup status, demo data |
| Bot | Guided `/expense`-style flows; `/balance`, `/summary`, `/budgets`, `/owed`, `/accounts`, `/categories` | One-line quick add ("coffee 18.40"), receipt photos, `/undo` |

### Visible bugs found in the review

The file and line references below were checked against `main` at `8213076`.

1. **Mixed currency on one screen.** `formatCurrency(cents, currency = "MYR")` in `apps/web/src/lib/format.ts:12`
   formats every aggregate as MYR. Account cards pass the account's own currency, and
   `accountService.CreateCashAccount` defaults to `"USD"` (`services/account_service.go:42`).
2. **"View all" links stack under card titles.** `SectionCardHeader` in `app/(dashboard)/page.tsx:46` adds
   `flex-row justify-between`, but `CardHeader` (`components/ui/card.tsx:18`) uses `display:grid`.
3. **Category icons render as raw text.** The icon field is free text (`create-category-dialog.tsx:194`), and
   names like `shopping-cart` overflow their tile on budgets and top expenses.
4. **Opening balances counted as income.** `CreateCashAccount` writes the opening balance as
   `TransactionTypeIncome` with description `"Initial balance"` (`account_service.go:61-68`). It shows up in Recent
   activity, the sankey ("Uncategorized RM 32,920"), and income totals.
5. **The net-worth chart defaults to 1W** (`net-worth-chart.tsx:137`). With weekly snapshots that draws one dot.
6. ~~**The investment value chart rendered no area** despite 26 snapshots.~~ Withdrawn: a screenshot artifact
   (see Status).
7. **Unformatted numbers.** The calendar shows `-157.7`, and budgets show `386.09%`.

## Goal

Ship the redesign in phases that each leave `main` releasable:

1. Fix the visible bugs (Phase 0).
2. Add the backend features launch depends on (Phase 1).
3. Re-skin through tokens, then rebuild the shell and screens (Phases 2–4).
4. Add the second-tier backend and bot features (Phases 5–6).
5. Ship the launch surfaces (Phase 7).

At the end, every screen in the prototype either works against the real API or has been cut from it.

## Decisions (for review)

| # | Decision | Recommendation | Alternative |
|---|---|---|---|
| D1 | Visual direction | **Decided (9 Nov):** the layout, type and components as prototyped, with colour as a user-selectable palette. Eight palettes (Catppuccin, Gold Ledger, Cobalt, Saffron, Graphite, Lagoon, Nord, Gruvbox), each in light and dark. Users can keep the server default, which the server owner sets with `DEFAULT_PALETTE`; Catppuccin when unset (item 1.8). The dither chart theme stays an Appearance option. | Owner earlier felt Gold Ledger read as AI-made; the palette picker replaces a single fixed look. |
| D2 | Currency at launch | **Superseded by plan 024:** amounts are stored in their own currency and converted to the user's reporting currency on read, with FX rates from the oracle. | — |
| D3 | Opening balances | **New transaction type `opening_balance`.** It affects balance like income but is excluded from all income, spending, budget and cash-flow figures. Existing rows are backfilled. | Flag column `is_opening_balance`. Rejected: every query would need two conditions. |
| D4 | Navigation | **Six main destinations:** Home, Transactions, Accounts, Investments, Budgets, Insights. Owed to me sits under "People"; Categories and Rules under "Organize". Securities leaves the sidebar and is reached from a "Securities" tab on Investments; `/securities` and `/securities/[id]` stay as routes. Budgets is already a page and in the nav. Mobile tabs: Home, Activity, ＋, Wealth, More. | Keep the current flat nav. |
| D5 | Rename Analytics to Insights | **Rename the label only.** Keep `/analytics` and add a redirect from `/insights`. | Move the route. |
| D6 | Mobile app | **PWA** (manifest, icons, `display: standalone`, safe-area handling) in Phase 3. No service worker or offline support at launch. | Capacitor wrapper later. Out of scope. |
| D7 | Where the MCP change log comes from | **Add `source` and `client_id` columns to `audit_logs`**, backfilled from the `changes` JSON (`via`, `client_id` and `tool`, which `internal/mcp/write.go:171-173` already writes). | Query `changes LIKE '%"via":"mcp"%'`. Rejected: slow and fragile. |
| D8 | Transaction search | **`LOWER(col) LIKE ? ESCAPE '\'` on description and notes**, the pattern `security_service.go` and `receivable_service.go` already use (it works on Postgres and on the SQLite test database), with `%` and `_` in the query escaped. Postgres gets GIN trigram indexes on `lower(description)` and `lower(notes)`. | Full-text `tsvector`. Overkill for short descriptions. |
| D9 | Bulk edit | **One endpoint, `POST /transactions/bulk`**, capped at 200 IDs and run in a single database transaction. | Many parallel `PUT`s from the client. Rejected: not atomic, and noisy in the audit log. |
| D10 | Data export | **Per-table CSV as a zip, streamed.** The web app fetches it with the Bearer token and saves the blob (a plain link can't carry the token). A Postgres dump stays an operator task (the backup service, `docs/backup-and-dr.md`). The UI shows backup status and links to docs, but doesn't download dumps. | Downloading `pg_dump` from the UI. Rejected: large, and a security risk. |
| D11 | Receipt OCR in the bot | **Out of scope.** Photos attach to the transaction just entered, but nothing reads them. | OCR. That needs a model or provider, which conflicts with privacy-first. Revisit separately. |
| D12 | How AI fits in | **MCP only (decided 9 Nov).** Kuberan has no built-in assistant and makes no AI calls of its own. People use the assistant they already have (Claude, Claude Code or any MCP app) through the MCP server. The app's job is connecting, controlling and reviewing that access: Settings → Connected apps (setup steps, per-app access and limits, a log of changes) and "via Claude" on rows an app changed. Copy must not claim data never leaves the server: what a user allows goes to their assistant's provider. | An in-app assistant through the Anthropic API (rejected: it puts AI calls, keys and data egress inside Kuberan). |

## Phase 0: Visible bug fixes (no redesign)

These ship on the current UI, and none needs a migration except 0.4.

- **0.1 Card headers.** Make `SectionCardHeader` use `CardAction` (the shadcn slot that the grid already supports),
  or override the grid with `flex`. Check the dashboard, budgets card and composition card.
- **0.2 Category icons.**
  - Add `lib/category-icons.ts`, a curated Lucide set of about 40 icons keyed by name, plus emoji passthrough.
  - Add `<CategoryIcon icon color />`. Unknown or long values fall back to the first letter of the category name.
  - Use it everywhere a category icon renders (budgets card, top expenses, the transaction rows that show the
    category icon, and the category list).
  - The picker itself comes in Phase 4.
  - Backend: validate `icon` as either a known key or a single emoji grapheme. Existing data is left alone and
    rendered with the fallback.
- **0.3 Net-worth chart default.** Default to `6M`. If that range has fewer than 8 points, step up to the next
  range that has enough.
- **0.4 Opening balance type.** This is Phase 1.2, pulled forward: it's small, and it fixes the most misleading
  numbers.
- **0.5 Investment value chart.** Reproduce against seeded data, fix it, and add a real empty state.
- **0.6 Number formatting.** Add `formatCompactCurrency` (for example `1.2k`, `−158`) for the calendar cells, and
  whole-number percentages in budgets (`386%`, or "Over by RM 1,144" when over 100%).
- **0.7 Expense colour.** In `domain-visuals.ts`, show expense amounts in `text-foreground`, not `negative`. Red is
  only for over-budget, overdue and error states.

**Tests:** unit-test `CategoryIcon` fallback, `formatCompactCurrency`, and range auto-step. Phase 1.2 lists its own
backend tests.

## Phase 1: Launch-critical backend

Migration numbers below are placeholders in item order; assign the real number at merge time, in merge order (see
the re-audit). SQL migrations run only on Postgres. The test suite builds its schema with GORM `AutoMigrate`
(`internal/testutil/database.go`), so every new column also goes on the model struct, and every query has to work
on both. Every item updates Swagger, the TypeScript types (`types/models.ts`, `types/api.ts`) and the hooks.

### 1.1 Base currency: the two remaining gaps

Plan 024 did the rest (see the re-audit). The original 1.1 design (a `DEFAULT_CURRENCY` env var, an
`excluded_from_totals` flag, a `formatCurrency` sweep) is superseded and has been removed from this plan.

- **Sign-up:** `RegisterRequest` gains an optional `base_currency`, validated against the same reporting-currency
  list `PATCH /profile` uses (2-decimal currencies only). Leaving it out keeps the column default (`'MYR'`). The
  register page gets a currency select.
- **New accounts default to the user's currency:** `CreateCashAccount`, `CreateInvestmentAccount` and
  `CreateCreditCardAccount` hard-code `"USD"` when the request leaves currency empty (`services/account_service.go`,
  three places). Use the user's `base_currency` instead. Fixed deposits already take the source account's currency.
- **Tests:** register with and without a currency, and the account default for each account type.

### 1.2 Opening balance type

**Done in Phase 0** (#39, migration `000037_add_opening_balance_type`). See the Status notes at the top.

### 1.3 Transaction search, notes and source

- **Migration `000048_add_notes_source_to_transactions`:**
  - `notes TEXT NOT NULL DEFAULT ''`;
  - `source VARCHAR(20) NOT NULL DEFAULT 'web'`, one of `web | telegram | mcp | import | system`;
  - `source_client_id VARCHAR(255) NULL`: the OAuth client when `source = 'mcp'`, so a row can say which app
    changed it (see 1.6);
  - a backfill that is safe to infer: `import` where `import_ref IS NOT NULL` (IBKR rows), `system` for
    `opening_balance`, `reimbursement` and `investment` rows. Everything else stays `web`, because older rows
    can't be told apart;
  - `CREATE EXTENSION IF NOT EXISTS pg_trgm` and GIN trigram indexes on `lower(description)` and `lower(notes)`.
- **Notes:** create and update take `notes` (max 2,000 characters, counted as characters like the other text
  limits in `services/text_limits.go`).
- **Passing source through:** `CreateTransaction` and `CreateTransfer` take positional arguments today
  (`services/interfaces.go`). Add one `TransactionMeta{Notes, Source, SourceClientID}` argument rather than three
  more positional ones, and update REST, MCP, the receivable and investment services, and the tests.
  - REST sets `web`, or `telegram` when the request's token is a bot token. Bot tokens already carry
    `token_type: "bot"` (`middleware/auth.go`, minted by `/internal/telegram/resolve`) and last a year, so don't
    add a new claim: have `AuthMiddleware` put the token type in the request context.
  - MCP write tools set `mcp` and the token's `client_id`.
  - Server-created rows (opening balances, fixed-deposit payouts, investment cash legs, repayments) set `system`;
    the IBKR import sets `import`.
- **Search:**
  - Add `Search *string` to `TransactionFilter` (`services/interfaces.go`) and apply it in `applyTransactionFilters`.
  - `GET /transactions?q=` matches `LOWER(description) LIKE ? ESCAPE '\'` OR the same on `notes`, the pattern
    `security_service.go` and `receivable_service.go` already use. Escape `%` and `_` in `q`, trim it, and cap it at
    100 characters.
  - A numeric `q` (for example `64.73`) also matches `amount`, `to_amount` (transfers) and `personal_amount` in
    cents. Every supported currency has 2 decimals, so the conversion is exact.
  - Add `shared=true` (rows with receivables) and `has_attachment=true`; the filter chips need both.
- **⌘K:** the command palette calls the same endpoint with `page_size=5`.
- **Tests:** search across description, notes and amounts, wildcard escaping, user scoping, `telegram` from a bot
  token, and source values for each path.

### 1.4 Bulk transaction edit

- **`POST /transactions/bulk`:** `{ ids: string[] (1–200), set?: { category_id?, notes_append? }, delete?: bool }`.
  - All IDs must belong to the user, otherwise the request fails with `404`.
  - Runs in one database transaction and writes one audit row per change.
- **Recategorising:** only income and expense rows can be recategorised (`isCategorizableType`). Ownership comes
  from `ensureCategoryOwned` (`services/category_helpers.go`), which only checks ownership. The type match comes
  from `categoryTypeMatchesTxType` (`services/rule_matcher.go`), which REST `UpdateTransaction` doesn't call today.
  Fix that too.
- **Deleting:** reuse `DeleteTransaction`'s rules: repayments can't be deleted directly, transfers delete as one
  row, and an expense's receivables go with it. `DeleteTransaction` opens its own database transaction, so split
  out a variant that takes the caller's `tx`.
- **Changing shared status** stays per transaction (`PUT /transactions/:id/receivables`), because settlements make
  it more than a flag.
- **MCP:** unchanged. `categorize_transactions` already covers the AI case.
- **Tests:** atomicity (one bad ID rolls everything back), ownership, type mismatch, and the delete rules.

### 1.5 Profile, password and devices

- **`PATCH /profile`** also accepts `first_name`, `last_name` and `email`.
  - Email must be unique. The `users.email` unique index ignores soft deletes, so a deleted account's email stays
    taken; keep that.
  - An email change requires `current_password`. All changes are audited.
- **`POST /auth/password`:** `{ current_password, new_password }` with the same rule as register (`min=8,max=72`
  plus the 72-byte bcrypt check).
  - It revokes every other refresh session and keeps the current one. Other devices keep a valid access token for
    up to 15 minutes; the Telegram bot's year-long token and connected apps are unaffected. Say so in the UI, and
    offer "Disconnect all apps" (`Grants.RevokeAll` exists) separately.
  - Audited. Wrong current passwords count toward the lockout counter.
- **`GET /auth/sessions`:** the user's live `refresh_sessions` (`expires_at > now`; revoked sessions are hard
  deleted): `{ id, created_at, last_seen_at, user_agent, ip_address, current }`.
  - `current` needs no new token claim: the refresh cookie is scoped to `Path=/api/v1/auth`, so these routes
    receive it. Hash it and match `token_hash` or `previous_token_hash`.
  - `last_seen_at` is `rotated_at`, falling back to `created_at`. The client turns the user agent into a device
    label.
- **`DELETE /auth/sessions/:id`** revokes one session (not the current one; that's logout).
  **`POST /auth/sessions/revoke-others`** revokes all but the current one.
- **Routing:** the `/auth` group is public today, so these routes need `AuthMiddleware` per route, plus a rate
  limiter in the style #38 added.
- **"Last login: —":** `AttemptLogin` already sets `LastLoginAt`, but `userPayload` (`handlers/auth_handler.go`)
  never returns it. Add `last_login_at` to the payload. Showing the previous login rather than this one is a small
  follow-up.
- **Tests:** current-session detection from the cookie, revoke-one, revoke-others, the password change revoking
  others, email uniqueness, and `last_login_at` in the profile.

### 1.6 MCP change log and connected-app tracking

The Connected apps screen is Kuberan's only AI surface (D12), so it has to be accurate about which apps are
connected, when they were last used, what they changed, and whether a disconnect has taken effect.

How it works today (checked against `main` at `3b3bea4`):

- An app registers through the DCR proxy (`POST /oauth2/register`). Its display name is the `client_name` it
  declares, sanitised and truncated (`sanitizeClientName`), and the consent screen marks such apps as unverified.
- Accepting consent (`AcceptConsent`) upserts a row in `oauth_client_grants`: user, `client_id`, `client_name`,
  approved scopes, `auto_accept`, `last_used_at`, `revoked_at`. Hydra keeps its own consent session. A remembered
  app skips that handler: `GetConsent` accepts it directly without touching the grant.
- `GET /oauth/clients` lists the grants, plus "legacy" connections that have a Hydra consent but no grant row.
  Each surface of an assistant (claude.ai, Claude Code) registers separately and gets its own row.
- Disconnect revokes the Hydra consent sessions (refresh tokens die) and then marks the grant revoked.
- Writes call `Grants.Authorize`, which re-checks the grant on every write and updates `last_used_at` at most once
  a minute (`lastUsedInterval`). Reads are validated offline from the JWT plus a cached active-user check; they
  never consult the grant.

**Change log**

- **Migration `000049_add_source_to_audit_logs`:**
  - `source VARCHAR(20) NOT NULL DEFAULT 'web'` and `client_id VARCHAR(255) NOT NULL DEFAULT ''`;
  - an index on `(user_id, source, created_at DESC)`;
  - a backfill. `changes` is `TEXT` and is `''` when a caller passes no changes (logout does), so guard the cast:
    `... WHERE CASE WHEN changes LIKE '{%' THEN changes::jsonb->>'via' END = 'mcp'`.
- **Writers:** `AuditServicer.Log` has about 60 callers. Add `LogOpts{Source, ClientID}` as a trailing variadic
  argument so none of them change; the MCP write wrapper (`internal/mcp/write.go`) passes both.
- **Richer MCP audit rows:** MCP audit `changes` hold only `type`, `amount` and `account_id` today
  (`tools_transactions_write.go`), so a summary like "Added transaction · Blue Bottle · RM 18.71" can't be built
  from them. Record `description` and `currency` from now on. For older rows, join the transaction (unscoped, since
  it may be deleted), and fall back to the action alone.
- **`GET /oauth/clients/:client_id/activity?page=&page_size=`:** `{ id, created_at, action, resource_type,
  resource_id, summary }`. The server builds `summary` from a fixed template per action and truncates user-written
  text to 80 characters. The UI treats it as plain text, never HTML.
- **`GET /oauth/clients/activity?page=&page_size=`:** the same rows for all of the user's apps, newest first, with
  `client_id` and the grant's `client_name`. The "Recent changes by apps" card uses this.
- **"Via Claude" on transactions:** item 1.3's `source_client_id`. The API returns `source_client_name` from the
  grant, falling back to "an app" if the grant is gone.
- **Write limits:** `GET /oauth/clients` adds `write_limits: { max_amount, currency, per_minute, per_day }` when the
  grant has any `write:*` scope. Since plan 024 the ceiling is in the user's base currency. The api reads these
  settings already (`internal/config/config.go`), but `docker-compose.prod.yml` passes `MCP_MAX_WRITE_AMOUNT` and
  the rate limits only to the `mcp` service. Add them to `api`, or the screen shows defaults.

**Last used, including reads**

- Today `last_used_at` moves only on writes, so a read-only app shows no use at all.
- Add `Touch(userID, clientID)` to the narrow grant interface the MCP server holds (`GrantAuthorizer` in
  `internal/mcp/server.go`). Call it once per authenticated request from `requireBearer`, which will need the grant
  dependency passed in.
  - It runs in the background, and a failure is logged without failing the request.
  - Use the same 1-minute interval as `Authorize`: skip touches within it in-process, and guard the
    `UPDATE … WHERE last_used_at IS NULL OR last_used_at < ?` with a cutoff computed in Go (not `now() - interval`,
    which SQLite lacks), so several MCP replicas stay cheap.

**Disconnect takes effect for reads too**

- Today writes stop at once, but reads keep working until the app's access token expires (Hydra
  `access_token: 15m`, `ory/hydra/hydra.yml`).
- In `requireBearer`, check that the app's grant exists and isn't revoked. Cache the result per user and app for
  1 minute, the same TTL `activeUsers` uses (`internal/mcp/active.go`). Per-tool scopes stay with `requireScope`,
  which reads them from the token.
- A disconnect then stops reads within a minute. Writes keep the uncached check in `Authorize`. The Connected apps
  screen says "Disconnected. Requests already under way may finish within a minute."
- **Legacy connections** (a Hydra consent but no grant row) are **refused** (decided 9 Nov). Those apps must
  reconnect once after the deploy; the screen already labels them "reconnect to choose permissions", and the
  upgrade notes say so.

**Where the app connects from**

- Add `client_host` to the grant (migration `000050_add_client_host_to_oauth_grants`, `VARCHAR(255) NOT NULL
  DEFAULT ''`). Set it from `consentRedirectTarget(consent).Host` (`handlers/oauth_handler.go`), the host #38
  already shows on the consent screen, in both `AcceptConsent` and `GetConsent`'s auto-accept branch. Otherwise
  remembered apps would never get one.
- `GET /oauth/clients` returns it, and the screen shows it under the name ("claude.ai"). That tells apart two apps
  that both call themselves "Claude". Until a grant has a host, the screen shows the name only.

**Tests**

- The audit backfill, including empty `changes`.
- Scoping: user A can't read user B's activity, per app or across apps.
- The summary template escapes and truncates, and falls back when the transaction is gone.
- `source_client_name` resolves, including after a grant is deleted.
- `Touch` throttling in-process and in SQL, and that a touch failure doesn't fail the request.
- A revoked grant is refused on reads once the cache expires.
- `client_host` is recorded on both consent paths.

### 1.7 Data export and server status

- **`GET /api/v1/export`:** a zip streamed as it's built, one CSV per table. The web app fetches it with the Bearer
  token and saves the blob.
  - Tables: accounts (with fixed-deposit details), transactions, categories, budgets, rules with their conditions
    and actions, investments and investment transactions, the securities the user holds, receivables, settlements,
    and attachment metadata (the files themselves stay with the backups).
  - Every money column comes with its currency, plus the frozen base-currency figures plan 024 stores (`_base`
    columns, `to_amount`, `fx_rate_to_base`).
  - User-scoped. Soft-deleted rows are included with `deleted_at`.
  - Rate-limited to 1 per minute per user. That needs a user-ID key function; the limiter has only `ClientIPKey`
    and `EmailBodyKey` today. Audited.
- **`GET /api/v1/system/status`:**
  - `latest_price_at` (max `security_prices.recorded_at`), `latest_fx_rate_at`, and `latest_snapshot_at`;
  - `last_backup_at`, if the backup service records one. Add a small `backup_runs` table it writes to, or a file the
    API can read; decide in review. Return `null` when it isn't available.
  - Read-only, for the sidebar status chip and Settings → Data.
- **Tests:** zip contents against fixtures, user scoping, and the rate limit.

### 1.8 Appearance preferences (theme picker)

Users pick a palette, or keep the server default. Whoever runs the server picks that default. With neither set,
Kuberan uses **Catppuccin**. Settings follow the user across devices instead of resetting per browser.

- **Server default:** new env var `DEFAULT_PALETTE` on the api.
  - Validated at startup against the palette list. Unset falls back to `catppuccin`. An unknown value logs a
    warning and falls back too, so a typo never stops the server.
- **Migration `000051_add_appearance_to_users`:** `users.appearance JSONB NOT NULL DEFAULT '{}'`. On the model, use
  a JSON-serialised type that also works on SQLite.
- **Shape:** `{ "palette": "default" | "<palette id>", "mode": "system" | "light" | "dark", "chart_style":
  "clean" | "dither", "density": "comfortable" | "compact", "show_cents": true }`.
  - A missing key means "default". `hide_balances` stays its own column.
  - The server validates `palette` against the same list as `DEFAULT_PALETTE`, so an unknown palette can't be
    stored.
- **Palette list:** one Go list shared by the config check and the validator, mirrored by the web app's palette
  registry. Add a Go test that reads the web registry file and compares the ids. The category icon list has no
  such test yet (its two copies are kept in sync by a comment only); add one there too.
- **API:**
  - `GET /profile` returns `appearance` plus `resolved_palette`, so the client never repeats the fallback logic.
  - `PATCH /profile` accepts `appearance` as a partial update. Merge in Go (read, merge, write in one transaction),
    not with Postgres `||`, so it also works on SQLite.
  - The new public `GET /api/v1/appearance` returns `{ "default_palette": "catppuccin" }`. The sign-in, register
    and OAuth consent pages render before anyone is signed in, so they need it.
- **Resolution order:** the user's choice, then `DEFAULT_PALETTE`, then `catppuccin`.
- **Web:**
  - next-themes sets the `dark` class on `<html>` (`attribute="class"`), so palette CSS is
    `[data-palette="…"]` for light values and `.dark[data-palette="…"]` for dark.
  - Applying the palette before first paint is new work. The chart-theme init script reads `localStorage` today;
    add a cookie mirror, read by an inline script that carries the CSP nonce like the chart-theme script does
    (`app/layout.tsx`). `localStorage` stays as the fallback when signed out.
  - Settings → Appearance shows "Server default (<name>)" first, then every palette as a tile previewing its own
    colours. Today that tab holds only the chart style; mode moves here from the header menu.
- **Tests:** config fallback (unset and unknown values), validation of unknown keys and values, merge semantics,
  `resolved_palette` for each layer, the registry cross-check, and that the public endpoint needs no auth and
  exposes nothing else.

### 1.9 Credit-card opening balance

- `CreateCreditCardAccountRequest.current_balance` (`gte=0`, optional) writes an `opening_balance` row.
- `CreateCreditCardAccount` (`services/account_service.go`) doesn't use a database transaction today. Wrap it in
  one, as `CreateCashAccount` does, so the account and its opening balance are created together.
- The Phase 0 type already moves liability balances the right way (never inverted).
- Web: the "Current balance owed" field in New account.

**Phase 1 exit:** `./scripts/check-go.sh apps/api` is green, `pnpm lint`, `tsc` and `pnpm test` pass in
`apps/web`, Swagger is regenerated, and the new endpoints are covered in `tests/integration/`.

## Phase 2: Design system re-skin (tokens only, no layout changes)

The direction is decided (D1). Phase 2 changes how things look without moving anything.

- **2.1 Tokens and palettes.**
  - Replace the `:root` and `.dark` values in `apps/web/src/app/globals.css` with the prototype's values. Keep the
    existing token names (`--background`, `--card`, `--elevated`, `--muted`, `--primary`, `--positive`,
    `--negative`, `--chart-1..6`…) so shadcn primitives keep working. Map `ink-0..3` to `background`, `card`,
    `elevated` and `muted`; the accent to `primary`; `pos` and `neg` to `positive` and `negative`.
  - Ship each palette as a token set. next-themes puts the `dark` class on `<html>` (`attribute="class"`), so the
    selectors are `[data-palette="…"]` for light values and `.dark[data-palette="…"]` for dark (see 1.8). Ship the
    eight prototype palettes, Catppuccin as the built-in default. Each must pass the contrast checks the prototype's
    palettes were held to (text 7:1, secondary text and money colours 4.5:1).
  - **Make the rest follow the tokens:**
    - 38 hard-coded Tailwind colour classes in five files move to tokens: `components/layout/app-sidebar.tsx`,
      `app/(auth)/layout.tsx`, `components/settings/telegram-settings.tsx`, `app/(dashboard)/accounts/[id]/page.tsx`
      and `app/(dashboard)/investments/[id]/page.tsx`.
    - The dither charts use named colours (`DitherColor`, `components/dither-kit/palette.ts`). Either map each
      palette to dither colours, or leave dither as a fixed look that ignores the palette. Decide in Phase 2.
  - **Category colours are user data.** `categories.color` stores any hex the user picked from 16 presets
    (`components/ui/color-palette.tsx`), so they can't simply become `--k1..--k8`. Keep the stored hex as the
    source of truth, and replace the 16 presets with a smaller set chosen to read well across all eight palettes.
    Existing colours are left as they are. `--k1..--k8` are for UI-owned series only (allocation, exposure,
    account types).
- **2.2 Type.** Load Instrument Serif and Source Serif 4 (opsz) through `next/font/google`. Add `--font-display`
  and `--font-figures`. Body stays Geist. `next/font` self-hosts at build time, so the CSP's `font-src 'self'` is
  fine, but the build needs network access to Google Fonts.
  - Utilities: `.font-display` for page titles and brand; `.font-figures` with
    `font-variant-numeric: lining-nums tabular-nums` for hero figures and KPI values.
- **2.3 Money display.** Extend the existing `components/ui/money.tsx` (plan 024). Today it takes `amount`,
  `currency`, the FX props (`baseAmount`, `baseCurrency`, `fxRate`, `fxDate`, `fxStale`, `fxMissing`), `masked` and
  `layout`, and shows "US$1,200.00 ≈ RM 5,512.00" with an FX tooltip.
  - Add `sign` (`auto | always | never`), `size` (`hero | lg | md | sm`), `dimCents` and `compact`. Keep the FX
    behaviour and the `amount` name.
  - The true minus after the currency ("RM −64.73") is new formatting work. `Intl` with `en-MY` gives
    `-RM 64.73`. Do it in `formatMoney` with `formatToParts`, so strings (toasts, aria labels, which keep using
    `formatMoney`) and JSX match.
  - Remove the hand-added "−" in `composition-card.tsx` and `cashflow-card.tsx`, or they'll show two minus signs.
  - Move remaining raw `formatMoney` output in JSX onto `<Money>` screen by screen during Phase 4.
- **2.4 Primitives.**
  - Radius 14px on cards and 9px on controls; borders at 8% (dark) and 9% (light).
  - Control height: 34px at desktop widths only. Inputs are 40px today, and stay at least 40px on phones for touch.
  - Restyle `button`, `input`, `select`, `tabs` (into the segmented style), `badge` (into `chip`, with tones
    `pos | neg | warn | accent | neutral`), `dialog`, `sheet` and `card`.
  - Add `components/ui/tile.tsx` (the tinted icon square), `pill.tsx` (category pill), `segmented.tsx`,
    `stat-strip.tsx` and `toggle-chip.tsx` (filter chips). `stat-strip` replaces each row of `StatCard`s (2 to 6 per
    page today) with one joined strip.
- **2.5 Charts.** `ThemedAreaChart` already has a gradient fill from the series colour and horizontal grid lines.
  - Add an emphasised endpoint dot, and optional `baseline` (cost-basis dashed line) and `markers` (trades) props.
  - Forward the new props through the lazy wrapper `components/charts/lazy.tsx`, which `net-worth-chart.tsx` and
    `investment-value-chart.tsx` use.
  - Under the dither theme, `baseline` and `markers` are ignored unless dither-kit gets equivalents. Say so in the
    component docs.
- **2.6 Brand.**
  - Replace the gradient "K" tile, in `app-sidebar.tsx` and `app/(auth)/layout.tsx`, with the ring mark (an SVG
    component).
  - Update `favicon.ico` and add `icon.svg`, `apple-icon.png` and a 512px maskable icon. Delete the create-next-app
    placeholder SVGs in `public/`.
  - Ideally a designer supplies the final mark. The ring is a placeholder until then.

**Exit:** every existing page renders in the new theme with no layout regressions (screenshots of all routes in
light and dark at 1440 and 390, compared side by side), `pnpm build` and `pnpm test` are clean, and no `any` is
introduced.

## Phase 3: App shell, navigation and mobile web

- **3.1 Sidebar** (`components/layout/app-sidebar.tsx`). Today it lists every page in a flat nav, with a Settings
  link in the footer.
  - D4 groups, plus a search entry (opens ⌘K) at the top.
  - A status chip in the footer, fed by `GET /system/status` (1.7): "Self-hosted · prices 06:00".
  - A user row in the footer that links to Settings.
  - The "Owed to me" badge comes from the receivables summary hook (`hooks/use-receivables.ts`).
- **3.2 Header** (`components/layout/app-header.tsx`). Today: ⌘K search button, hide balances, a theme menu and the
  avatar.
  - New on desktop: an optional month switcher slot and "＋ New" (`N`).
  - New on mobile: the mark and page title, with search and hide balances.
  - The theme menu moves to Settings → Appearance (1.8).
- **3.3 Mobile tab bar:**
  - Add `components/layout/mobile-tab-bar.tsx`, shown below `md`. Tabs: Home, Activity, ＋ (opens add-transaction),
    Wealth (`/accounts`, with an Accounts/Portfolio segmented switch), More (`/more`, a new route listing the
    remaining destinations).
  - Sticky at the bottom with `padding-bottom: env(safe-area-inset-bottom)`. Define the bottom inset once (a CSS
    variable on the layout), because the dialog already adds its own safe-area padding and the two mustn't double up.
  - The shadcn mobile sidebar sheet stays reachable from More, then "All pages".
- **3.4 Dialogs on phones.** Mostly done already. `components/ui/dialog.tsx` becomes a bottom sheet below `sm`, with
  full width, `max-h-[92dvh]`, a grab handle and safe-area padding. Inputs are 16px on mobile (`input.tsx`), and
  money fields use `inputMode="decimal"`. Remaining:
  - a full-height variant for add-transaction;
  - optionally move the breakpoint from `sm` to `md`, so small tablets also get the sheet.
- **3.5 PWA.** None of this exists today.
  - Add `app/manifest.ts` (`name`, `short_name`, `display: "standalone"`, `theme_color` per scheme, icons).
  - Add a `viewport` export to `app/layout.tsx` (only `metadata` is exported today) with `viewportFit: "cover"`, plus
    the `apple-mobile-web-app-*` metas.
  - Check the CSP in `middleware.ts` allows the manifest (it has no `manifest-src`, so `default-src` applies).
  - Put "Add to Home Screen" instructions on `/more`. iOS has no install event; Android uses
    `beforeinstallprompt`.
- **3.6 Viewport height:** use `dvh` for full-height layouts as the dialog already does, so the collapsing mobile
  URL bar doesn't hide content.
- **3.7 Keyboard.** Today there's ⌘K (`components/layout/command-palette.tsx`) and the sidebar's ⌘B.
  - `N` opens new transaction. `/` opens ⌘K; there's no inline search box to focus.
  - Go-to: `G` then `H` Home, `T` Transactions, `A` Accounts, `I` Investments, `B` Budgets, `N` Insights.
  - Guard against typing in inputs. List the shortcuts in ⌘K, and add the missing Analytics/Insights entry to its
    nav list (`command-palette-menu.tsx`).

**Exit:** every route is usable at 360–430px with no horizontal scroll, the tab bar never covers content, and
Lighthouse PWA "installable" passes.

## Phase 4: Screen-by-screen rebuild

Each screen is its own PR, matching the prototype screen of the same name. The order follows daily use. The "API"
column lists what must have landed first.

Before Phase 4, the prototype needs plan 024's screens (the re-audit lists them). Every screen keeps what plan 024
added:

- amounts in their own currency through `<Money>`, with the converted total and FX tooltip;
- the cross-currency amount field on transfers (`cross-currency-amount.tsx`);
- the funding and deposit amounts on trades (`cross-currency-field.tsx`);
- the currency exposure chart on Investments;
- FX notices (`fx-notice.tsx`);
- the reporting-currency setting.

| Screen (prototype id) | Route | Key changes | API |
|---|---|---|---|
| Home (`home`) | `/` | Hero net worth (6M default, delta, balance sheet panel, owed-to-you line); "this month" strip of spend vs pace, budgets at risk and cash flow; Recent; Accounts with sparklines | Today, plus 5.1 (sparklines) and 5.4 (pace); render without them until they land |
| Onboarding (`welcome`) | `/` (empty state) | Four-step checklist computed client-side from accounts, transactions, budgets and connected apps; "Load demo data" | 5.7 for the demo button; hide it until then |
| Search (`search`) | overlay (⌘K) | One palette for transactions, actions and navigation | 1.3 |
| Transactions (`transactions`, `tx-bulk`) | `/transactions` | Desktop table with 40px rows, sticky day totals and an inline category picker; mobile grouped list; search box; filter chips (period, account, category, type, amount, shared); multi-select with a bulk bar; j/k/x/c/e keys | 1.3, 1.4 |
| Add transaction (`tx-new`) | overlay | Amount first; recent-category chips; account and date; split, receipt and note as optional extras; cross-currency transfers keep the received amount; ⌘↵ saves, ⇧⌘↵ saves and adds another | 1.3 (notes) |
| Transaction detail (`tx-detail`) | overlay | Editable fields, a shared-expense panel, receipt, "via Claude" when an app made it, and a "Create rule" suggestion | Today (receivables, attachments), plus 1.3 and 1.6 |
| Accounts (`accounts`) | `/accounts` | Balance-sheet groups (cash and savings, fixed deposits, investments, credit cards and loans) with group totals, each account in its own currency and totals in the base currency; a stat strip; an "inactive accounts" toggle (`is_active`) | Today, plus 5.1 (sparklines) |
| New account (`account-new`) | overlay | Type picker first, then only that type's fields; credit card gets "current balance owed"; currency defaults to the base currency | 1.1, 1.9, plus 5.3 (loan) |
| Cash account (`account`) | `/accounts/[id]` | Balance hero with history chart, in/out this month, activity | 5.1 for the chart; hide it until then |
| Credit card (`account-card`) | `/accounts/[id]` | Amount owed, utilisation bar, due-date block, interest estimate (client-side), "Pay card" (prefilled transfer) | Today; "Schedule a reminder" needs 5.6 |
| Fixed deposit (`account-fd`) | `/accounts/[id]` | Principal, accrued and projected interest (client-side from rate, compounding and dates), timeline, terms, close early (exists: `close-fixed-deposit-dialog.tsx`) | Today |
| Investments (`investments`) | `/investments` | Portfolio hero with a cost-basis dashed line; allocation donut (class only; Region needs 5.5); currency exposure; price vs FX return; holdings table and mobile list; tabs Current, Closed (exist) and Securities (new, links to `/securities`) | Today |
| Holding (`holding`) | `/investments/[id]` | Stat strip; price chart with trade markers; actions; history | Today |
| Record trade (`holding-trade`) | overlay | Buy, Sell, Dividend and Split in one sheet, keeping the cross-currency funding/deposit fields; realised-gain preview | Today |
| Securities (`securities`) | `/securities` (reached from the Investments tab, not the sidebar) | Search, type chips, "Held" flag, sparkline, change percentage | Today (prices) |
| Security (`security`) | `/securities/[id]` | Price hero, range, details, your position | Today |
| Budgets (`budgets`) | `/budgets` (already a page and in the nav) | Summary bar; cards with On track / Close / Over status and a pace marker; paused section (`is_active`) | Today |
| Edit budget (`budget-edit`) | overlay | Six-month average hint (client-side from monthly summary) | Today |
| Insights (`insights`) | `/analytics` | KPI strip with "vs typical"; sankey (desktop) or ranked list (mobile); calendar heatmap; by category; largest; six-month trend; clicking any mark opens filtered Transactions | Today; "vs typical" needs 5.4 |
| Owed to me (`owed`) | `/receivables` | A card per person with progress and "Record repayment"; stats; recently settled | Today |
| Record repayment, lend (`owed-settle`, `lend`) | overlays | Quick full/half/other amounts; existing people as chips | Today |
| Categories (`organize-categories`) | `/categories` | Grid with this month's count and total; subcategories | Today |
| Edit category (`category-edit`) | overlay | Curated icon picker (exists since Phase 0), the reduced colour presets from 2.1, parent | Today |
| Rules (`organize-rules`) | `/rules` | Sentence-style rows, drag reorder (exists), match counts, suggestion card | Today; counts and suggestions need 5.2 |
| Edit rule (`rule-edit`) | overlay | Conditions builder; live "matches N existing" (existing `preview`); "Recategorise them" (existing `apply`) | Today |
| Settings tabs | `/settings?tab=…` | Tabs aren't linkable today (`<Tabs defaultValue="profile">`). Drive them from `searchParams`, keeping the existing values `profile`, `appearance`, `telegram` and `connected-apps`, and adding `data` | — |
| Settings: Profile and security (`prefs-profile`) | `/settings?tab=profile` | Editable profile, devices list, change password, log out; the reporting-currency and default-account settings already live here | 1.5 |
| Settings: Appearance (`prefs-appearance`) | `/settings?tab=appearance` | Palette tiles with "Server default", theme mode (moved from the header), Clean/Dither (exists here), hide balances on open (`hide_balances` exists, toggled from the header today), compact rows, show cents | 1.8 |
| Settings: Telegram (`prefs-telegram`) | `/settings?tab=telegram` | Linked state with username, last message and message count (all exist on `telegram_links`); a sample chat showing only commands that really exist | Today; the quick-add sample needs Phase 6 |
| Settings: Connected apps (`prefs-apps`) | `/settings?tab=connected-apps` | Per app: name and host, access level, can see / can change, write limits, last used, disconnect. Recent changes across apps. "Connect an assistant" steps with the connector URL and copy | 1.6 (and 1.3 for "via Claude") |
| Settings: Data and export (`prefs-data`) | `/settings?tab=data` (new tab) | Export, backup status, the IBKR import (API exists, no UI yet), demo data | 1.7; IBKR import today; 5.7 and 5.8 |
| More (`more`) | `/more` (new) | Mobile hub and Add to Home Screen | Today |
| Sign in, create account (`signin`, `signup`) | `/login`, `/register` | Split layout, lockout copy and `autocomplete` (both exist), base currency at sign-up | 1.1 for the sign-up currency |
| OAuth sign-in and consent (`connect-signin`, `connect-consent`) | `/oauth/login`, `/oauth/consent` | Client banner; read scopes; separate "Also allow changes" with limits; remember toggle (exists) | Today (plus `write_limits` from 1.6) |

Every screen PR includes:

- loading skeletons that match the final layout;
- an empty state with one sentence and one action;
- an error state;
- light and dark screenshots at 1440 and 390 in the PR description.

## Phase 5: Second-tier backend

These items aren't needed to launch. Each one unhides a piece of UI that was built in Phase 4 but kept hidden.

- **5.1 Per-account balance history.** Replaying transactions doesn't work for every account:
  - The `type` of an investment cash leg doesn't say which way the money moved. Buys and sells are both stored as
    `investment` rows, with the direction applied at write time (`services/investment_service.go`), and IBKR import
    rows carry it only in the `import_ref` suffix (`:dr` / `:cr`).
  - An investment account's balance is market value, worked out when read.
  - Transfers need both legs (`to_account_id` rows add `COALESCE(to_amount, amount)`), and liabilities invert,
    except opening balances.

  So **record balances going forward**: the snapshot pipeline (`POST /pipeline/snapshots`, run by the oracle every
  30 minutes) also writes one row per account per day to a new `account_balance_snapshots(account_id, day,
  balance, currency)` table.
  - A one-off backfill replays history only for cash, credit-card, debt and fixed-deposit accounts with no
    `investment` rows, where replay is exact.
  - Investment accounts use the portfolio history that already exists.
  - Endpoints: `GET /accounts/:id/balance-history?from=&to=` and `GET /accounts/sparklines?days=30`. Register the
    sparklines route before `/:id` in `routes.go`.
  - Values are in each account's own currency. Home compares accounts in the base currency, converting with the
    stored FX rates.
- **5.2 Rule stats and suggestions.**
  - Add `transaction_rules.match_count` and `last_matched_at`. Today `resolveRuleCategory`
    (`services/receivable_helpers.go`) throws away the matched rule's ID and runs outside the write transaction.
    Return the ID and increment inside the create transaction. `ApplyRule` adds the number of rows it changed.
  - Suggestions: nothing records past recategorisations (the `UPDATE_TRANSACTION` audit row has no changes), so
    define a suggestion on data that exists. That means at least 3 income or expense rows in the last 90 days with
    the same normalised description and the same category, which no rule matches.
    `GET /rules/suggestions` returns `{ description_pattern, category_id, count }`.
  - Separately, record old and new values in the `UPDATE_TRANSACTION` audit row from now on; it's useful well beyond
    this.
- **5.3 Loan accounts.**
  - The `debt` type exists (models, validator, snapshots count it as debt) but has no create route, and the model
    has no monthly payment or recurring due day: `DueDate` is a single date that the service treats as
    credit-card only.
  - Add a migration for `monthly_payment BIGINT NULL` and `due_day SMALLINT NULL` on accounts. Credit cards can
    use `due_day` too, which 5.6 needs.
  - `POST /accounts/debt`: name, currency, amount owed (written as an `opening_balance` row, never inverted),
    interest rate, monthly payment, due day. One database transaction.
- **5.4 Insights summary.** `GET /analytics/typical?months=6` returns, in the base currency:
  - average monthly income, spending and saved percentage;
  - the average cumulative daily spending curve (31 points) for the pace chart;
  - average spending by category for the "vs typical" toggle.

  It builds on `services/period_totals.go`, which already excludes opening balances and converts currencies.
- **5.5 Security region: dropped for launch.** The price providers return only price, currency and time, and a
  listing exchange (`securities.exchange`) is not a fund's exposure: an ETF listed in London can hold the world.
  The Investments screen keeps "by class", and plan 024's currency exposure covers the "where is my money" question.
  Revisit with a manually set region if users ask.
- **5.6 Reminders.**
  - Add a `reminders` table: `user_id`, `kind` = `card_due | loan_due | fd_maturity`, `ref_id`, `remind_at`,
    `sent_at`, `enabled`.
  - The API creates and updates them from `due_day` (5.3) and `fixed_deposit_details.maturity_date`.
  - **The bot pulls them.** It polls and has no HTTP server, and the API never calls it. Its existing `JobQueue`
    polls `GET /internal/telegram/reminders/due` and confirms each with `POST /internal/telegram/reminders/:id/sent`,
    using the existing bot secret. No new inbound endpoint or network path is needed. The oracle isn't involved: it
    runs every 30 minutes with only the pipeline key.
  - Each reminder kind can be switched off in Settings → Telegram.
- **5.7 Demo data.**
  - `POST /demo/load` creates a sample household; `DELETE /demo` removes it for good (a hard delete, not a soft
    one).
  - Every demo row is listed in `demo_rows(user_id, table_name, row_id)`, so deleting never touches real data.
  - Load only when the user has no transactions of their own, ignoring opening balances and demo rows.
  - Block transfers between demo and real accounts. Deleting the demo would otherwise leave a real balance wrong.
  - New config: `DEMO_ENABLED` (off by default; on for the public demo instance, 7.2).
- **5.8 CSV import.** Follow the IBKR importer (`POST /imports/ibkr`, `docs/ibkr-import.md`).
  - One endpoint: `POST /imports/csv` (multipart), a dry run unless `apply=true`. The file is held in memory only,
    never stored, so there's no `upload_id`. The dry run returns detected columns, a sample, the suggested mapping
    and what would be created or skipped.
  - Exempt the route from the 1 MiB `BodySizeLimit`, as the IBKR and attachment routes are (`cmd/api/main.go`).
  - Set `import_ref = "csv:<account>:<row hash>"` (column and index from migration `000044`), so a re-import is
    idempotent the way IBKR's is. Also fuzzy-match (date, amount, normalised description) against rows entered by
    hand.
  - Load the user's rules once and call the pure matcher (`Match`, `services/rule_matcher.go`) per row. Rows get
    `source = 'import'` (1.3). Amounts are in the account's currency.
  - Capped at 5,000 rows. Applies in one database transaction.

## Phase 6: Telegram bot

The bot authenticates as the user: the bot secret resolves to a per-user token (`token_type: "bot"`), and it then
calls the normal protected routes. Its API client is JSON-only today, with just `_get` and `_post`
(`apps/bot/api_client.py`).

- **6.1 One-line quick add.**
  - Today the guided flow starts only from `/expense` or `/income`, and `/expense 50 coffee` is already a quick path
    (`handlers/transaction_flow.py`).
  - Add a plain-text entry point to that `ConversationHandler`, not a separate global handler. A global one would
    catch text while a conversation waits for a button tap and create a second transaction.
  - Extend `_parse_amount_description`, which accepts amount-first only, to accept "coffee 18.40" too. Reuse
    `_extract_account_and_category`. The account is the user's default account.
  - `POST /transactions` already applies the user's rules when no category is sent. Add
    `POST /transactions/suggest-category` (`{ description, type }` → `{ category_id, reason: "rule" | "history" }`)
    only for the reply and for the history fallback: the most recent category used for that description. It wraps
    `ResolveForUser` (`services/rule_service.go`) and accepts income and expense only.
  - Reply "Added RM 18.40 Dining to DBS Everyday", with buttons to change the category or account or to undo.
    Input it can't parse falls through to the guided flow.
- **6.2 `/undo`.** Deletes the last transaction the bot created in the last 10 minutes, found by `source='telegram'`
  (1.3). Needs a `_delete` method on the API client.
- **6.3 Receipt photos.**
  - A photo sent within 10 minutes of an entry attaches to that transaction through
    `POST /transactions/:id/attachments` (multipart; JPEG, PNG, WebP or PDF, per `docs/receipts.md`). Otherwise the
    bot asks which transaction it belongs to.
  - Needs a multipart upload method on the API client.
  - No OCR (D11, D12).
- **6.4 `/spent`.** A new command: this month by category, from `GET /transactions/spending-by-category`, in the base
  currency. `/summary` shows only income, spending and net.
- The Settings → Telegram sample chat (Phase 4) shows only what exists at the time it ships.

## Phase 7: Launch surfaces

- **7.1 Landing page.** Either a separate static site (`apps/site`, so a self-hoster's instance never serves
  marketing) or `/welcome` on a hosted demo. Recommendation: a separate static site.
  - Content follows the proposal's landing mockup: headline, one-command install, demo link, trust strip (your
    database, Claude through MCP, Telegram, investing, multi-currency), real screenshots in both themes.
- **7.2 Public demo instance.** `DEMO_ENABLED=true` (5.7), a nightly reset job, writes allowed but wiped nightly,
  and MCP disabled.
- **7.3 Docs.** Already in `docs/`: `mcp-oauth.md`, `backup-and-dr.md` (backups, deploys, rollback, upgrade notes),
  `database-setup.md`, `receipts.md`, `multi-currency.md` and `ibkr-import.md`.
  - New: a self-hosting quickstart, a Telegram bot guide (today there's only `apps/bot/.env.example`), and
    "Connect your AI assistant" (the user-facing side of `mcp-oauth.md`).
  - Update: `CLAUDE.md`'s endpoint and env var lists. Its "budgets have no page" note is also stale.
- **7.4 Screenshots.** Nothing exists to reuse: plan 024's screenshots were taken by hand, and no seed script is
  committed. Add:
  - a seed script that creates a demo household through the API (or calls 5.7);
  - a Playwright script that captures every route in light and dark at 1440 and 390.

  Playwright isn't a dependency anywhere yet, so add it as a dev dependency of `apps/web` and run the script from
  there. It's used for marketing and for the Phase 2–4 visual checks.

## Out of scope (follow-ups)

- Native mobile app or Capacitor wrapper (D6). Offline support and a service worker.
- Receipt OCR (D11) and any in-app assistant (D12).
- Bank connections or aggregators. These conflict with the no-bank-logins positioning.
- Reorderable or hideable dashboard cards (Copilot-style). Revisit after launch feedback.
- Shared households or multiple users per budget.
- Security region (5.5), unless users ask.

## Risks

- **Migration numbering.** Several Phase 1 items add migrations, and they may merge in any order. A lower number
  merged after a higher one is skipped silently in production. Assign numbers at merge time.
- **Legacy MCP connections** (a Hydra consent but no grant row) lose read access once 1.6 checks grants on reads,
  unless 1.6 decides otherwise. Say so in the upgrade notes.
- **Password change doesn't log out the bot or apps.** Bot tokens last a year and are stateless, and app tokens
  come from Hydra. 1.5 tells the user and offers "Disconnect all apps".
- **`pg_trgm`** ships with Postgres 16 contrib, and the Alpine image includes it. Migrations don't run on SQLite,
  so the tests aren't affected; the search queries themselves must work on both (1.3).
- **Mobile Safari quirks** (safe areas, `dvh`, input zoom, sticky inside overflow). Test on a real iPhone in both
  the browser and Home Screen modes before calling Phase 3 done.
- **Opening-balance data.** Migration `000037` has shipped. After the next production deploy, check that
  every account created with a starting balance has exactly one `opening_balance` row and no "Initial balance"
  income.

## Verification

- **Backend:** `./scripts/check-go.sh apps/api` (build, vet, golangci-lint v2.15.0 as pinned in CI, test, race)
  after each phase. Integration tests under `tests/integration/` for search, bulk edit, sessions, export and
  import. Migrations are checked up, down and up again on Postgres 16.
- **Frontend:** `pnpm build`, `pnpm lint`, `tsc --noEmit` and `pnpm test` (Vitest). CI doesn't run `pnpm test` yet;
  add it in Phase 2. The Phase 7.4 screenshot script is compared against the prototype for each screen. Manual pass
  on iPhone Safari and Android Chrome at the end of Phases 3 and 4.
- **Bot:** `apps/bot/tests` (pytest) plus parser tests for quick add (amount first or last, decimals, commas,
  currency prefixes, emoji).

## Files touched (summary)

| Area | Files |
|---|---|
| Migrations | Done: `000037_add_opening_balance_type` (Phase 0); multi-currency `000038`–`000047` (plan 024). Planned (numbers assigned at merge): notes/source on transactions, source/client on audit logs, `client_host` on grants, `appearance` on users; Phase 5 adds `account_balance_snapshots`, rule match stats, `monthly_payment`/`due_day` on accounts, `reminders`, `demo_rows` |
| API models | `user.go`, `transaction.go`, `audit_log.go`, `oauth_client_grant.go`, `account.go`, `transaction_rule.go`; `refresh_session.go` read only |
| API services | `account_service.go` (currency default, card opening balance, debt), `transaction_service.go` and `period_totals.go` (search, bulk, typical), `category_helpers.go` / `rule_matcher.go` (type checks), `receivable_helpers.go` (rule match count), `audit_service.go` (`LogOpts`), `user_service.go` (profile, password, sessions), `oauth_grant_service.go` (`Touch`, host), new export, status, import and reminder services |
| API handlers | `auth_handler.go` (register currency, profile, password, sessions, `last_login_at`), `transaction_handler.go` (search, notes, bulk, update audit), `oauth_handler.go` (`client_host` on both consent paths), `oauth_clients_handler.go` (activity, limits), new export, status, CSV import and reminder handlers, `routes.go` |
| Middleware | `auth.go` (expose `token_type` in the request context; no new claims) |
| MCP | `server.go` (grant check and `Touch` in `requireBearer`), `write.go` (audit `LogOpts`), `tools_transactions_write.go` (description and currency in audit rows, source) |
| Config and deploy | `config.go` (`DEFAULT_PALETTE`, `DEMO_ENABLED`), `docker-compose.prod.yml` (MCP limit vars for `api`) |
| Web | `globals.css`, `app/layout.tsx` (fonts, viewport, palette init script with nonce), new `app/manifest.ts`, `lib/format.ts` (`formatMoney` minus sign), `lib/domain-visuals.ts`, `components/ui/*` (extend `money`; new tile, pill, segmented, stat-strip, toggle-chip), `components/ui/color-palette.tsx`, `components/layout/*` (sidebar, header, new mobile tab bar, command palette), `components/charts/*` (`ThemedAreaChart`, `lazy.tsx`), settings tabs from `searchParams`, every page under `app/(dashboard)`, `(auth)` and `(oauth)`, new `app/(dashboard)/more`, `hooks/*` (new: sessions, export, status, app activity) |
| Bot | `handlers/transaction_flow.py` (quick add, undo, photo), new `/spent` handler, `api_client.py` (`_delete`, multipart upload), reminders job |
| Docs | `CLAUDE.md` (endpoints, env vars `DEFAULT_PALETTE` and `DEMO_ENABLED`, the stale budgets note), new self-hosting quickstart, Telegram guide and "connect your assistant" guide |
