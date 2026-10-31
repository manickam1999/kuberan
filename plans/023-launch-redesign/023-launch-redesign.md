# Plan 023 — Launch Redesign

**Status: Phase 0 implemented.** Later phases are proposed. Where Phase 0's code differs from the plan below:

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
- **0.4 (opening balance)** shipped as migration `000035_add_opening_balance_type`, not `000036`.
  - The down migration turns every `opening_balance` row back into `income`, which is exactly the old scheme, so
    no marker column is needed.
  - The monthly and daily summaries no longer filter on `description != 'Initial balance'`. That filter also hid
    real income with that name.
  - An opening balance moves the balance in the account's own sign convention (never inverted), so it is already
    correct for the credit-card `current_balance` in Phase 1.2. That field itself is still to do.
- **0.7 also covers** the account page's own type map, the cash-flow card's expense figures and bar, the sankey
  "Out" total, top-expense amounts, calendar cells and negative day totals. Months and savings with a negative net
  stay red.
- **No frontend unit tests.** `apps/web` has no test runner. Adding one (Vitest is the natural fit) is a separate
  decision. Phase 0 was checked with `tsc`, `next lint`, `next build` and Playwright against a seeded local stack.
- **Known, unrelated:** `TestSharedExpenseFlow_FullModeLoanAndWriteOffOnSettle` fails on `main` as well. The
  spending query ends at midnight today, so it misses a transaction created earlier today. The installed
  golangci-lint 2.5.0 reports 66 issues on `main`; Phase 0 adds none (`--new-from-rev`).

Design references:

- Review and proposal: https://claude.ai/artifact/XBWoNQHMj4zwZv7i7tDFgg
- Clickable prototype, 37 screens on desktop and mobile web: https://claude.ai/artifact/WFQR9ag4P6MaMBSLdxZAdf

Both artifacts are private to the repo owner. The prototype is the visual spec for this plan. Where this plan
and the prototype disagree, this plan wins; the prototype will be updated to match.

## Context

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
| Rules | CRUD, reorder, preview, apply, test | Match counts, suggestions |
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
| D1 | Visual direction | **Gold Ledger** as prototyped: warm charcoal and paper themes, one gold accent, Instrument Serif for titles, Source Serif 4 for large figures, Geist for UI, Geist Mono for labels. The dither chart theme stays as an opt-in in Appearance. | Rework the direction before building: the owner flagged that it still reads as AI-made. This should be settled before Phase 2. Phases 0–1 don't depend on it. |
| D2 | Currency at launch | **One base currency per user.** New accounts default to it and the account form hides the currency picker. Existing accounts in another currency are flagged, not converted. | Exchange-rate conversion now: oracle-fed rates and conversion in snapshots, budgets and analytics. That's large, and goes in its own plan. |
| D3 | Opening balances | **New transaction type `opening_balance`.** It affects balance like income but is excluded from all income, spending, budget and cash-flow figures. Existing rows are backfilled. | Flag column `is_opening_balance`. Rejected: every query would need two conditions. |
| D4 | Navigation | **Six main destinations:** Home, Transactions, Accounts, Investments, Budgets, Insights. Owed to me sits under "People"; Categories and Rules under "Organize". Securities becomes a tab in Investments. Routes stay where they are; only the nav changes. Mobile tabs: Home, Activity, ＋, Wealth, More. | Keep the current flat nav. |
| D5 | Rename Analytics to Insights | **Rename the label only.** Keep `/analytics` and add a redirect from `/insights`. | Move the route. |
| D6 | Mobile app | **PWA** (manifest, icons, `display: standalone`, safe-area handling) in Phase 3. No service worker or offline support at launch. | Capacitor wrapper later. Out of scope. |
| D7 | Where the MCP change log comes from | **Add `source` and `client_id` columns to `audit_logs`**, backfilled from the `changes` JSON (`via`, `client_id`, which `mcp/write.go:169-170` already writes). | Query `changes LIKE '%"via":"mcp"%'`. Rejected: slow and fragile. |
| D8 | Transaction search | **Postgres `ILIKE` on description and notes**, with a `pg_trgm` GIN index. SQLite tests use plain `LIKE`. | Full-text `tsvector`. Overkill for short descriptions. |
| D9 | Bulk edit | **One endpoint, `POST /transactions/bulk`**, capped at 200 IDs and run in a single database transaction. | Many parallel `PUT`s from the client. Rejected: not atomic, and noisy in the audit log. |
| D10 | Data export | **Per-table CSV as a zip, streamed.** A Postgres dump stays an operator task (the backup service). The UI shows backup status and links to docs, but doesn't download dumps. | Downloading `pg_dump` from the UI. Rejected: large, and a security risk. |
| D11 | Receipt OCR in the bot | **Out of scope.** Photos attach to the transaction just entered, but nothing reads them. | OCR. That needs a model or provider, which conflicts with privacy-first. Revisit separately. |

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

Migrations continue from `000034`. Every item updates Swagger, the TypeScript types (`types/models.ts`,
`types/api.ts`) and the hooks.

### 1.1 Base currency

- **Migration `000035_add_base_currency_to_users`:** `users.base_currency VARCHAR(3) NOT NULL DEFAULT 'USD'`.
  Backfill each user's most common account currency, falling back to `telegram_links.default_currency`, then `USD`.
- **Register:** `RegisterRequest.base_currency` (`omitempty,iso4217`). The server default comes from the new env
  var `DEFAULT_CURRENCY` (default `USD`).
- **Profile:** `PATCH /profile` accepts `base_currency`. Changing it doesn't touch existing accounts.
- **Accounts:** all `Create*Account` services take the user's base currency when the request leaves currency
  empty, instead of hard-coding `"USD"`.
- **Responses:** every aggregate response gains a `currency` field: portfolio summary, snapshots list, budget
  progress, monthly summary, spending by category, cash flow, daily spending, top expenses and the receivables
  summary. This lets the client stop guessing.
- **Mismatched accounts:** `GET /accounts` adds `excluded_from_totals: true` for accounts whose currency isn't the
  base currency.
  - Snapshots and analytics keep summing all accounts at launch, which is what happens today.
  - The UI shows a one-time banner: "2 accounts are in USD and are added without conversion".
  - Changing this is the FX follow-up (D2).
- **Bot:** read the user's `base_currency`. `telegram_links.default_currency` is deprecated but kept for one
  release.
- **MCP:** `format.go` uses the base currency for totals.
- **Web:**
  - `formatCurrency` loses its `"MYR"` default and takes the currency explicitly.
  - Add `useBaseCurrency()` from the profile.
  - About 37 files call `formatCurrency`. Do it as one mechanical change with `tsc` as the check.
- **Tests:** backfill migration on fixture data, register default, account create default, and the `currency`
  field on each aggregate.

### 1.2 Opening balance type

- **Model:** add `TransactionTypeOpeningBalance = "opening_balance"` and teach the `transaction_type` validator
  about it.
- **Rules for the new type:**
  - It can't be created through `POST /transactions` or MCP create tools.
  - It is written by account creation only.
  - It can be edited (amount and date) through `PUT /transactions/:id`, with the balance recalculated, and deleted.
  - Credit-card accounts get an optional `current_balance` on create. That writes an opening balance with the
    card's inverted sign.
- **Balance math:** opening balance behaves like income for cash and investment accounts, and like an expense
  (debt goes up) for credit cards. All of this happens inside the existing atomic balance update.
- **Excluded from:**
  - income and expense totals, monthly summary, daily spending/summary, spending by category, top expenses, cash
    flow (sankey sources), budget progress;
  - rule matching;
  - MCP summaries.
- **Still included in** account balance and snapshots.
- **Migration `000036_backfill_opening_balance`:** set `type='opening_balance'` where `type='income'` and
  `description='Initial balance'` and `category_id IS NULL`, the transaction was created within 5 seconds of its
  account, and it's the earliest transaction on that account. The down migration reverses exactly those rows
  (tracked through a temporary marker column dropped at the end of the up migration, or recorded in a side table;
  pick one in review).
- **Web:** Recent activity and lists show "Opening balance" with a neutral badge, and it can't be recategorised.
- **Tests:** a table test that each analytics query excludes the type, balance math for each account type, and
  the backfill on fixtures, including a real income named "Initial balance" that must not be touched.

### 1.3 Transaction search, notes and source

- **Migration `000037_add_notes_source_to_transactions`:**
  - `notes TEXT NOT NULL DEFAULT ''`;
  - `source VARCHAR(20) NOT NULL DEFAULT 'web'`, one of `web | telegram | mcp | rule | import | system`;
  - Postgres only: `CREATE EXTENSION IF NOT EXISTS pg_trgm` and a GIN trigram index on `description`.
    The migration is guarded so SQLite tests skip it.
- **Notes:** both create and update requests take `notes` (max 2,000 characters, counted as characters like the
  other text limits).
- **Source:**
  - REST handlers set `web`.
  - The bot's tokens come from `/internal/telegram/resolve`. Add a `src: "telegram"` claim to those tokens, and
    have `AuthMiddleware` expose it so handlers write `telegram`.
  - MCP write tools set `mcp`.
  - Account creation sets `system`, CSV import sets `import`, and rule-created rows set `rule` (none exist yet).
- **Search:**
  - Add `Search *string` to `TransactionFilter`.
  - `GET /transactions?q=` matches `description ILIKE %q%` OR `notes ILIKE %q%`. Trim `q` and cap it at 100
    characters.
  - A numeric `q` (for example `64.73`) also matches `amount = cents`.
  - Also add `shared=true` (rows with receivables) and `has_attachment=true` filters; the filter chips need both.
- **⌘K:** the command palette calls the same endpoint with `page_size=5`.
- **Tests:** search across description, notes and amount, user scoping, `src` claim propagation, and source
  values for each path.

### 1.4 Bulk transaction edit

- **`POST /transactions/bulk`:**
  - Body: `{ ids: string[] (1–200), set: { category_id?, notes_append?, shared?: false }, delete?: bool }`.
  - All IDs must belong to the user, otherwise the request fails with `404`.
  - Runs in one database transaction and writes one audit row per change.
  - Category type must match the transaction type, reusing `ensureCategoryOwned` and its type check.
  - Opening-balance rows are rejected.
- **MCP:** unchanged. `categorize_transactions` already covers the AI case.
- **Tests:** atomicity (one bad ID rolls everything back), ownership, and type mismatch.

### 1.5 Profile, password and devices

- **`PATCH /profile`** also accepts `first_name`, `last_name` and `email`.
  - Email must be unique. An email change requires `current_password`.
  - All changes are audited.
- **`POST /auth/password`:** `{ current_password, new_password }` (min 8, max 128).
  - Revokes every other refresh session and keeps the current one.
  - Audited.
  - Wrong current passwords count toward the lockout counter.
- **`GET /auth/sessions`:** lists the user's live `refresh_sessions` (not soft-deleted and not expired):
  `{ id, created_at, last_seen_at, user_agent, ip_address, current }`.
  - `current` is computed from the presented refresh session ID. The access token needs a `sid` claim; add it at
    issue.
  - `last_seen_at` is `rotated_at` falling back to `created_at`. Parse the user agent into a device label on the
    client.
- **`DELETE /auth/sessions/:id`:** revokes one session (it can't be the current one; use logout for that).
  **`POST /auth/sessions/revoke-others`** revokes all but the current one.
- **Last login:** fix "Last login: —" right after logging in by setting `LastLoginAt` before building the login
  response. Check this first; it may be a frontend read issue.
- **Tests:** the `sid` claim, revoke-one, revoke-others, password change revoking others, and email uniqueness.

### 1.6 MCP change log

- **Migration `000038_add_source_to_audit_logs`:**
  - `source VARCHAR(20) NOT NULL DEFAULT 'web'` and `client_id VARCHAR(255) NOT NULL DEFAULT ''`;
  - an index on `(user_id, source, created_at DESC)`;
  - backfill: `source='mcp'`, with `client_id` taken from the JSON, where `changes::jsonb->>'via' = 'mcp'`
    (Postgres only).
- **Writers:** `AuditService.Log` gains an options struct, `LogOpts{Source, ClientID}`. `mcp/write.go:172` passes
  both. REST callers default to `web`, and the bot path to `telegram`.
- **`GET /oauth/clients/:client_id/activity?page=&page_size=`:** returns
  `{ id, created_at, action, resource_type, resource_id, summary }`.
  - The server builds `summary` from a fixed template per action, for example "Added transaction · Blue Bottle ·
    RM 18.71".
  - User-written text in it is truncated to 80 characters. The UI treats it as plain text, never HTML.
- **`GET /oauth/clients`** adds `write_limits: { max_amount, per_minute, per_day }` from config when the grant has
  any `write:*` scope.
- **Tests:** the backfill, a scoping test (user A can't read user B's activity), and that the summary template
  escapes and truncates.

### 1.7 Data export

- **`GET /export.zip`:**
  - Streams one CSV per table: accounts, transactions, categories, budgets, investments, investment transactions,
    securities held, receivables, settlements and rules.
  - User-scoped, with money in cents plus a formatted column. Soft-deleted rows are included with `deleted_at`.
  - Rate-limited to 1 per minute per user, and audited.
- **`GET /system/status`:**
  - `latest_price_at` (max `security_prices.recorded_at`);
  - `latest_snapshot_at`;
  - `last_backup_at`, if the backup service writes a heartbeat. Add a small `backup_runs` table, or a file the API
    can read; decide in review. Return `null` if it isn't available.
  - Read-only, for the sidebar host chip and Settings → Data.
- **Tests:** zip contents against fixtures, user scoping, and the rate limit.

**Phase 1 exit:** `./scripts/check-go.sh apps/api` is green, Swagger is regenerated, and the new endpoints are
covered in `tests/integration/`.

## Phase 2: Design system re-skin (tokens only, no layout changes)

D1 must be signed off first.

- **2.1 Tokens.** Replace the `:root` and `.dark` values in `apps/web/src/app/globals.css` with the prototype's
  values. Keep the existing token names (`--background`, `--card`, `--primary`, `--positive`…) so shadcn primitives
  keep working.
  - Map `ink-0..3` to `background`, `card`, `elevated` and `muted`; `gold` to `primary`; `pos` and `neg` to
    `positive` and `negative`.
  - Add `--k1..--k8` as the category palette, with one shared hue order for light and dark.
  - Replace `--chart-1..6`.
- **2.2 Type.** Load Instrument Serif and Source Serif 4 (opsz) through `next/font/google`. Add `--font-display`
  and `--font-figures`. Body stays Geist.
  - Utilities: `.font-display` for page titles and brand; `.font-figures` with
    `font-variant-numeric: lining-nums tabular-nums` for hero figures and KPI values.
- **2.3 `<Money>`.** Add `components/ui/money.tsx`: `value` (cents), `currency`, `sign` (`auto | always | never`),
  `size` (`hero | lg | md | sm`), `dimCents`, `compact`, `masked`.
  - Uses a true minus (U+2212) after the currency code ("RM −64.73").
  - Respects `hideBalances`.
  - Replace direct `formatCurrency` output in JSX with `<Money>`, screen by screen during Phase 4. Keep
    `formatCurrency` for strings (toasts, aria labels).
- **2.4 Primitives.**
  - Radius 14px on cards and 9px on controls; borders at 8% (dark) and 9% (light); 34px default control height.
  - Restyle `button`, `input`, `select`, `tabs` (into the segmented style), `badge` (into `chip`, with tones
    `pos | neg | warn | gold | neutral`), `dialog`, `sheet` and `card`.
  - Add `components/ui/tile.tsx` (the tinted icon square), `pill.tsx` (category pill), `segmented.tsx`,
    `stat-strip.tsx` (replaces the four `StatCard`s with one joined strip) and `toggle-chip.tsx` (filter chips).
- **2.5 Charts.**
  - `ThemedAreaChart` gets an emphasised endpoint dot, a 5-tick grid, a gradient fill from the series colour, and
    optional `baseline` (cost-basis dashed line) and `markers` (trades) props.
  - Charts read colours from CSS variables, so both themes work.
  - The dither theme is untouched and keeps working.
- **2.6 Brand.**
  - Replace the gradient "K" tile with the ring mark (an SVG component).
  - Update `favicon.ico` and add `icon.svg`, `apple-icon.png` and a 512px maskable icon.
  - Ideally a designer supplies the final mark. The ring is a placeholder until then.

**Exit:** every existing page renders in the new theme with no layout regressions (screenshots of all routes in
light and dark at 1440 and 390, compared side by side), `pnpm build` is clean, and no `any` is introduced.

## Phase 3: App shell, navigation and mobile web

- **3.1 Sidebar** (`components/layout/app-sidebar.tsx`):
  - D4 groups, plus a search entry (opens ⌘K) at the top.
  - A host chip in the footer, fed by `GET /system/status`: "Self-hosted · prices 06:00".
  - A user row in the footer that links to Settings.
  - The "Owed to me" badge comes from the receivables summary.
- **3.2 Header** (`app-header.tsx`):
  - Desktop: an optional month switcher slot, search, hide balances, "＋ New" (`N`), avatar.
  - Mobile: a mark, the page title, search and hide balances.
  - The theme switch moves to Settings → Appearance; the header keeps only hide balances.
- **3.3 Mobile tab bar:**
  - Add `components/layout/mobile-tab-bar.tsx`, shown below `md`. Tabs: Home, Activity, ＋ (opens add-transaction),
    Wealth (`/accounts`, with an Accounts/Portfolio segmented switch), More (`/more`, a new route listing the
    remaining destinations).
  - Sticky at the bottom, with `padding-bottom: env(safe-area-inset-bottom)`. Content gets matching bottom padding.
  - The shadcn mobile sidebar sheet stays reachable from More, then "All pages".
- **3.4 Responsive dialogs:**
  - Add `ResponsiveDialog`: a `Dialog` at `md` and up, a bottom `Sheet` (with grab handle, 92% max height and
    full-height variant) below.
  - Every create and edit dialog moves onto it.
  - Inputs keep at least 16px font on mobile so iOS doesn't zoom on focus.
  - Amount inputs use `inputmode="decimal"`.
- **3.5 PWA:**
  - `app/manifest.ts` (`name`, `short_name`, `display: "standalone"`, `theme_color` per scheme, icons);
  - `viewport` with `viewportFit: "cover"` and the `apple-mobile-web-app-*` metas;
  - "Add to Home Screen" instructions on `/more` (iOS has no install event; Android uses `beforeinstallprompt`).
- **3.6 Viewport height:** use `dvh` units for full-height layouts and sheets, so the collapsing mobile URL bar
  doesn't hide them.
- **3.7 Keyboard:**
  - `N` opens new transaction, `/` focuses search, and `G` then `H/T/A/I` navigates.
  - Guard against typing in inputs. Show the shortcuts in ⌘K.

**Exit:** every route is usable at 360–430px with no horizontal scroll, the tab bar never covers content, and
Lighthouse PWA "installable" passes.

## Phase 4: Screen-by-screen rebuild

Each screen is its own PR, matching the prototype screen of the same name. The order follows daily use. The
"API" column lists the phase that must have landed first.

| Screen (prototype id) | Route | Key changes | API |
|---|---|---|---|
| Home (`home`) | `/` | Hero net worth (6M default, delta, balance sheet panel, owed-to-you line); "this month" strip of spend vs pace, budgets at risk and cash flow; Recent; Accounts with sparklines; "Ask your assistant" card shown only if a client is connected | Today, plus 5.1 (sparklines) and 5.4 (pace); render without them until they land |
| Onboarding (`welcome`) | `/` (empty state) | Four-step checklist computed client-side from accounts, transactions, budgets, Telegram link and OAuth clients; "Load demo data" | 5.7 for the demo button; hide it until then |
| Search (`search`) | overlay | One palette for transactions, actions and navigation | 1.3 |
| Transactions (`transactions`, `tx-bulk`) | `/transactions` | Desktop table with 40px rows, sticky day totals and an inline category picker; mobile grouped list; search box; filter chips (period, account, category, type, amount, shared); multi-select with a bulk bar; j/k/x/c/e keys | 1.3, 1.4 |
| Add transaction (`tx-new`) | overlay | Amount first; recent-category chips; account and date; split, receipt and note as optional extras; ⌘↵ saves, ⇧⌘↵ saves and adds another | 1.3 (notes) |
| Transaction detail (`tx-detail`) | overlay | Editable fields, a shared-expense panel, receipt, and a "Create rule" suggestion | Today (receivables, attachments), plus 1.3 |
| Accounts (`accounts`) | `/accounts` | Balance-sheet groups (cash and savings, fixed deposits, investments, credit cards and loans) with group totals; a stat strip; archived toggle | Today, plus 5.1 (sparklines) |
| New account (`account-new`) | overlay | Type picker first, then only that type's fields; credit card gets "current balance owed" | 1.2, plus 5.3 (loan) |
| Cash account (`account`) | `/accounts/[id]` | Balance hero with history chart, in/out this month, activity | 5.1 for the chart; hide it until then |
| Credit card (`account-card`) | `/accounts/[id]` | Amount owed, utilisation bar, due-date block, interest estimate (client-side), "Pay card" (prefilled transfer) | Today; "Schedule a reminder" needs 5.6 |
| Fixed deposit (`account-fd`) | `/accounts/[id]` | Principal, accrued and projected interest (client-side from rate, compounding and dates), timeline, terms, close early | Today |
| Investments (`investments`) | `/investments` | Portfolio hero with a cost-basis dashed line; allocation donut (class only; Region needs 5.5); holdings table and mobile list; tabs Current, Closed, Securities | Today |
| Holding (`holding`) | `/investments/[id]` | Stat strip; price chart with trade markers; actions; history | Today |
| Record trade (`holding-trade`) | overlay | Buy, Sell, Dividend and Split in one sheet; realised-gain preview | Today |
| Securities (`securities`) | `/securities` (tab) | Search, type chips, "Held" flag, sparkline, change percentage | Today (prices) |
| Security (`security`) | `/securities/[id]` | Price hero, range, details, your position | Today |
| Budgets (`budgets`) | `/budgets` (restored to nav) | Summary bar; cards with On track / Close / Over status and a pace marker; paused section | Today |
| Edit budget (`budget-edit`) | overlay | Six-month average hint (client-side from monthly summary) | Today |
| Insights (`insights`) | `/analytics` | KPI strip with "vs typical"; sankey (desktop) or ranked list (mobile); calendar heatmap; by category; largest; six-month trend; clicking any mark opens filtered Transactions | Today plus 1.2; "vs typical" needs 5.4 |
| Owed to me (`owed`) | `/receivables` | A card per person with progress and "Record repayment"; stats; recently settled | Today |
| Record repayment, lend (`owed-settle`, `lend`) | overlays | Quick full/half/other amounts; existing people as chips | Today |
| Categories (`organize-categories`) | `/categories` | Grid with this month's count and total; subcategories | Today |
| Edit category (`category-edit`) | overlay | Curated icon picker (Phase 0.2 set), eight-colour swatches, parent | Today |
| Rules (`organize-rules`) | `/rules` | Sentence-style rows, drag reorder, match counts, suggestion card | Today; counts and suggestions need 5.2 |
| Edit rule (`rule-edit`) | overlay | Conditions builder; live "matches N existing" (existing `preview`); "Recategorise them" | Today |
| Settings: Profile and security (`prefs-profile`) | `/settings` | Editable profile, base currency, default account, devices list, change password, log out | 1.1, 1.5 |
| Settings: Appearance (`prefs-appearance`) | `/settings?tab=appearance` | Theme cards, Clean/Dither, hide-on-open (exists), compact rows and show cents (localStorage) | Today |
| Settings: Telegram (`prefs-telegram`) | `/settings?tab=telegram` | Linked state with username, last message and message count (all exist on `telegram_links`); a sample chat showing only commands that really exist | Today; the quick-add sample needs Phase 6 |
| Settings: Connected apps (`prefs-apps`) | `/settings?tab=apps` | Can see / can change per app, write limits, change log, connector URL with copy | 1.6 |
| Settings: Data and export (`prefs-data`) | `/settings?tab=data` | Export, backup status, demo data | 1.7, plus 5.7 and 5.8 (import) |
| More (`more`) | `/more` (new) | Mobile hub and Add to Home Screen | Today |
| Sign in, create account (`signin`, `signup`) | `/login`, `/register` | Split layout, lockout copy, base currency at signup, `autocomplete` attributes | 1.1 |
| OAuth sign-in and consent (`connect-signin`, `connect-consent`) | `/oauth/login`, `/oauth/consent` | Client banner; read scopes; separate "Also allow changes" with limits; remember toggle | Today (plus `write_limits` from 1.6) |

Every screen PR includes:

- loading skeletons that match the final layout;
- an empty state with one sentence and one action;
- an error state;
- light and dark screenshots at 1440 and 390 in the PR description.

## Phase 5: Second-tier backend

These items aren't needed to launch. Each one unhides a piece of UI that was built in Phase 4 but kept hidden.

- **5.1 Per-account balance history.** `GET /accounts/:id/balance-history?from=&to=&interval=day|week` replays
  transactions backwards from the current balance, in one query with a window function. It's cheap enough to
  compute on demand for these data sizes. Add a cache later if needed.
  `GET /accounts/sparklines?days=30` returns every account in one call for the Home and Accounts lists.
- **5.2 Rule stats and suggestions.**
  - Add `transaction_rules.match_count` and `last_matched_at`, incremented when a rule assigns a category.
  - `GET /rules/suggestions` looks at the last 90 days for descriptions recategorised by the user at least 3 times
    to the same category with no matching rule, and returns `{ description_pattern, category_id, count }`.
- **5.3 Loan accounts.**
  - `POST /accounts/debt`: name, principal owed, interest rate, monthly payment and due day.
  - Uses the same inverted balance semantics as credit cards, and counts toward `debt_balance` in snapshots (it
    already does by type).
- **5.4 Insights summary.** `GET /analytics/typical?months=6` returns:
  - average monthly income, spending and saved percentage;
  - the average cumulative daily spending curve (31 points) for the pace chart;
  - average spending by category for the "vs typical" toggle.

  All of it excludes opening balances.
- **5.5 Security region.** Add `securities.region` (nullable, set through the pipeline API and the oracle's
  provider metadata) for "Allocation by Region". If the oracle can't provide it, drop the Region toggle.
- **5.6 Reminders.**
  - Add a `reminders` table (`user_id`, `kind` = `card_due | fd_maturity | custom`, `ref_id`, `remind_at`,
    `channel` = `telegram`, `sent_at`).
  - The oracle worker's daily run sends due ones through a new internal bot endpoint.
  - Card-due and fixed-deposit maturity reminders are created automatically, and each can be switched off.
- **5.7 Demo data.**
  - `POST /demo/load` creates a tagged sample household; `DELETE /demo` removes it.
  - Every demo row is linked through a `demo_rows(user_id, table, id)` registry, so deleting it never touches real
    data.
  - Allowed only when the user has no transactions of their own (load), or always (delete). Disabled when
    `ENV=production` unless `DEMO_ENABLED=true` (for the public demo instance, Phase 7).
- **5.8 CSV import.**
  - `POST /import/csv/preview` (multipart) returns detected columns, a sample and suggested mapping.
  - `POST /import/csv/commit` takes `{ upload_id, account_id, mapping, date_format, invert_sign }`.
  - Dedupes on (date, amount, normalised description) against existing rows in that account, applies rules, and
    sets `source='import'`.
  - Capped at 5,000 rows. Runs in one database transaction.

## Phase 6: Telegram bot

- **6.1 One-line quick add.**
  - A `MessageHandler(filters.TEXT & ~filters.COMMAND)` outside conversations parses "`<text> <amount>`" or
    "`<amount> <text>`".
  - The category is guessed by a new `POST /transactions/suggest-category`
    (`{ description, type }` → `{ category_id, reason: "rule" | "history" }`). It uses the rule engine's matcher
    first, then the user's most recent category for that description. `POST /rules/preview` matches existing
    rows for a draft rule, so it doesn't fit here. The account is the user's default account.
  - Replies "Added RM 18.40 Dining to DBS Everyday", with inline buttons to change the category or account or to
    undo.
  - Ambiguous input falls through to the existing guided flow.
- **6.2 `/undo`.** Deletes the last transaction the bot created in the last 10 minutes (`source='telegram'`, 1.3).
- **6.3 Receipt photos.** A photo sent within 10 minutes of an entry attaches to that transaction through the
  attachments endpoint. Otherwise the bot asks which transaction it belongs to. No OCR (D11).
- **6.4 `/spent`.** This month by category. It's an alias of `/summary` with a category breakdown if `/summary`
  doesn't already cover it; check during implementation.
- The Settings → Telegram sample chat (Phase 4) shows only what exists at the time it ships.

## Phase 7: Launch surfaces

- **7.1 Landing page.** Either a separate static site (`apps/site`, so a self-hoster's instance never serves
  marketing) or `/welcome` on a hosted demo. Recommendation: a separate static site.
  - Content follows the proposal's landing mockup: headline, one-command install, demo link, trust strip (your
    database, Claude through MCP, Telegram, investing), real screenshots in both themes.
- **7.2 Public demo instance.**
  - `DEMO_ENABLED=true`, a nightly reset job, and writes allowed but wiped nightly.
  - MCP is disabled on the demo.
- **7.3 Docs.** Self-hosting quickstart, upgrading, backups, connecting Claude (`docs/mcp-oauth.md` exists), and
  the Telegram bot.
- **7.4 Screenshots.** A Playwright script (`scripts/screenshots.ts`) that seeds demo data and captures every
  route in light and dark at 1440 and 390. It's used for marketing and for the Phase 2–4 visual checks.

## Out of scope (follow-ups)

- Exchange-rate conversion and multi-currency totals (D2). This needs its own plan: rates from the oracle,
  conversion in snapshots, budgets and analytics, and a per-account display currency.
- Native mobile app or Capacitor wrapper (D6). Offline support and a service worker.
- Receipt OCR (D11).
- Bank connections or aggregators. These conflict with the no-bank-logins positioning.
- Reorderable or hideable dashboard cards (Copilot-style). Revisit after launch feedback.
- Shared households or multiple users per budget.

## Risks

- **Direction sign-off (D1).** The owner said the prototype still feels AI-generated. Phases 0–1 are independent
  of the look; Phase 2 waits for a decision. Most of the visual change lives in tokens and type, so changing
  direction later mostly touches `globals.css`, the fonts and a few primitives.
- **Opening-balance backfill misclassifies.** The heuristic is narrow (exact description, no category, earliest
  row, created with the account). Run a dry-run count query on production before applying, the same way plan 022
  handled its post-deploy check.
- **The `formatCurrency` sweep (37 files) touches every screen.** Do it in one mechanical PR with no visual
  changes, before Phase 2.
- **`pg_trgm` availability.** It ships with Postgres 16 contrib (the Alpine image includes it). The migration
  must not fail on SQLite in tests.
- **Mobile Safari quirks** (safe areas, `dvh`, input zoom, sticky inside overflow). Test on a real iPhone in
  both the browser and Home Screen modes before calling Phase 3 done.

## Verification

- **Backend:** `./scripts/check-go.sh apps/api` (build, vet, lint, test, race) after each phase. Integration tests
  under `tests/integration/` for search, bulk edit, sessions, export and the opening-balance flows. The
  migrations are checked up, down and up again on Postgres 16.
- **Frontend:** `pnpm build`, `pnpm lint`, strict TypeScript, and the Phase 7.4 screenshot script compared
  against the prototype for each screen. Manual pass on iPhone Safari and Android Chrome at the end of Phases 3
  and 4.
- **Bot:** existing `apps/bot/tests` plus parser tests for quick add (amount first or last, decimals, commas,
  currency prefixes, emoji).

## Files touched (summary)

| Area | Files |
|---|---|
| Migrations | `000035_add_base_currency_to_users`, `000036_backfill_opening_balance`, `000037_add_notes_source_to_transactions`, `000038_add_source_to_audit_logs`; Phase 5 adds rule stats, `reminders`, `demo_rows`, `securities.region` |
| API models | `user.go`, `transaction.go`, `audit_log.go`, `refresh_session.go` (read only), `transaction_rule.go` |
| API services | `account_service.go`, `transaction_service.go` (filter, bulk, opening-balance exclusions), analytics and budget services, `audit_service.go`, auth/user service (password, sessions), new `export_service.go` and `status_service.go` |
| API handlers | `auth_handler.go` (profile, password, sessions), `transaction_handler.go` (search, bulk), `oauth_clients_handler.go` (activity, limits), new `export_handler.go` and `status_handler.go`, `routes.go` |
| Middleware | `auth.go` (`sid` and `src` claims) |
| MCP | `write.go` (audit `LogOpts`), `format.go` (base currency) |
| Web | `globals.css`, `app/layout.tsx` (fonts, viewport), `app/manifest.ts`, `lib/format.ts`, `lib/category-icons.ts`, `lib/domain-visuals.ts`, `components/ui/*` (money, tile, pill, segmented, stat-strip, toggle-chip, responsive-dialog), `components/layout/*` (sidebar, header, mobile tab bar, command palette), every page under `app/(dashboard)`, `(auth)` and `(oauth)`, the new `app/(dashboard)/more`, `hooks/*` (new: sessions, export, status, app activity) |
| Bot | `handlers/transaction_flow.py` (quick add, undo, photo), `api_client.py` |
| Docs | `CLAUDE.md` (endpoints, env vars `DEFAULT_CURRENCY` and `DEMO_ENABLED`), `docs/self-hosting.md`, `docs/telegram.md` |
