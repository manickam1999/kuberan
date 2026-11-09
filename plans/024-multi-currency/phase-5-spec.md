# Plan 024 — Phase 5 spec: frontend

The web app (`apps/web`) catches up with Phases 1–4. Every amount is shown in **its own currency**, with the
base-currency (MYR) equivalent next to it where they differ. Dialogs ask for the second amount when money crosses
currencies. The portfolio shows the price/FX split and currency exposure. Settings gets the reporting currency.

**Decision:** `Investment.current_price` stays in the base currency (the Phase 2 compatibility rule is kept for good;
D4's flip is dropped). The UI reads `current_price_native` / `price_currency` for native figures. The API is not
changed in this phase, except for fixing a real bug if one is found (report it, don't work around it).

The API contract is in `CLAUDE.md` (Monetary Values, plus the endpoint list with the new fields). Read the Go models and
handlers when a field's exact JSON is unclear: `apps/api/internal/models/*.go`, `internal/services/interfaces.go`,
`internal/handlers/*_handler.go`.

## Verification (every agent)
There's no frontend test runner. Every agent must leave **`pnpm lint` and `pnpm build` passing** (run in `apps/web`). Plain
`npx tsc --noEmit` has one pre-existing error (`./globals.css` side-effect import in `src/app/layout.tsx`); ignore only that
one. Strict TypeScript, never `any`, ShadCN primitives, react-hook-form + zod, as CLAUDE.md says.

## F1 — Foundation (must land first)
1. **Types** (`src/types/models.ts`, `src/types/api.ts`): add every field the API now returns:
   - `User.base_currency`
   - `Account.balance_base?` (absent when there's no rate), `fx_rate?`, `fx_stale?`, `fx_missing?`
   - `Investment.current_price_native`, `price_currency`, `fx_rate`, `fx_stale`, `fx_missing`, `cost_basis_base`,
     `realized_gain_loss_base`, `currency`
   - `InvestmentTransaction.currency`, `fx_rate_to_base` (string | null), `base_amount` (number | null), `realized_gain_loss_base`
   - `Transaction.to_amount?`, `implied_rate?`
   - `PortfolioSummary.currency`, `fx_stale`, `unconverted`, `total_price_return`, `total_fx_return`
   - `PortfolioSnapshot.currency`, `fx_missing`
   - analytics types (monthly/daily items, spending by category, cashflow, top expenses + item `amount_base`/`currency`):
     `currency`, `fx_missing`, `unconverted`
   - `Budget.currency`, `BudgetProgress.currency`/`fx_missing`
   - `Receivable.currency`, `ReceivableSettlement.currency`/`account_amount`, `ReceivableSummary.currency`/`fx_missing`/`by_currency`
   - request DTOs: `fx_rate`, `funding_amount`, `deposit_amount`, dividend `fee`/`deposit_account_id`, transfer `to_amount`,
     settle `account_amount`, budget `currency`, profile `base_currency`
2. **Errors:** `ApiClientError` gains `details?: Record<string, unknown>` parsed from `error.details`. Add a typed helper
   `getFxAmountRequired(err): { field: string; currency: string; suggestedAmount?: number; rate?: string; rateRecordedAt?: string } | null`.
3. **Formatting:**
   - Add `formatMoney(cents: number, currency: string, masked?: boolean)`, with currency **required**. Mark
     `formatCurrency` `@deprecated` (still exported; F2–F4 migrate their call sites; F5 deletes it).
   - Currencies with symbols should render the standard `Intl` way (`US$1,234.00`, `MYR 1,234.00` or `RM` per the existing locale
     choice; keep whatever MYR rendering the app uses today).
4. **`useBaseCurrency()`** hook (from the profile query; falls back to `"MYR"` while loading).
5. **`<Money>`** component (`src/components/ui/money.tsx`): props `amount`, `currency`, optional `baseAmount`,
   `baseCurrency`, `fxRate`, `fxStale`, `fxMissing`, `masked`, `className`. It renders the native amount, and when
   `currency ≠ baseCurrency` a muted second line or suffix `≈ RM 5,512.00`. A tooltip shows the rate and its date (or
   "Exchange rate is stale" / "No exchange rate"). With `fxMissing` it shows the native amount plus a small warning icon,
   and never shows an `≈` figure. It respects `hide_balances` masking.
6. **Settings → Reporting currency:** a select in the profile/appearance area (wherever profile settings live) listing a sensible
   set (MYR, USD, SGD, GBP, EUR, AUD, HKD, CNY…). It calls `PATCH /profile {base_currency}`, shows the API's
   `UNSUPPORTED_CURRENCY` error inline, and on success invalidates **all** queries.
7. **`<FxNotice>`**: a small reusable banner/inline note for responses with `fx_missing` or `unconverted` ("Excludes US$120.00 —
   no exchange rate").

## F2 — Investments (after F1)
`app/(dashboard)/investments/page.tsx`, `investments/[id]/page.tsx`, `securities/**`, `components/investments/**`,
and the investments section of `accounts/[id]/page.tsx`.
- **Holdings table:**
  - price = native (`current_price_native` + `price_currency`);
  - market value = native `quantity × current_price_native`, with `≈` base;
  - cost basis = native `cost_basis` with `≈` base `cost_basis_base`;
  - unrealized P&L in **base** = `quantity × current_price` − `cost_basis_base` (both base). Show the native P&L in the tooltip;
  - sorting uses the base value.
  - **Never mix** a native figure with a base figure in one calculation.
- **Detail page:** the same rules; transactions list in the security currency, with `≈ base_amount`.
- **Portfolio summary cards:** total value, cost basis, unrealized P&L (base), plus **Price return** and **FX return**
  (`total_price_return`, `total_fx_return`), realized P&L (base). `<FxNotice>` for `unconverted` / `fx_stale`.
- **Currency exposure:** a donut (Recharts, like the asset-allocation chart) of base value by `price_currency`, using the
  investments list.
- **Dialogs:**
  - **Add/buy/sell:** price and fee inputs show the security's currency symbol. When the chosen funding/deposit account's currency
    differs, show "Amount charged to <account> (<CCY>)" / "Amount received in <account>". Pre-fill it with a suggestion: call
    `GET /fx-rates/latest` for the pair and multiply. It stays editable and is required. Optional "Exchange rate" (advanced) maps to
    `fx_rate`. If the API still returns `FX_AMOUNT_REQUIRED`, use `getFxAmountRequired` to reveal the field, pre-filled with
    `suggested_amount`.
  - **Dividend:** add **withholding/fees** (`fee`) and an optional **deposit account** with the same cross-currency field
    (`deposit_amount` = the net in that account's currency).
- **Securities pages:** prices in `security.currency` (already partly done). Check the charts' axis/tooltip formatting.
- **Charts** (`investment-value-chart`, `asset-allocation-chart`): base currency.

## F3 — Accounts, transactions, receivables (after F1)
`app/(dashboard)/accounts/**` (except the investments section), `transactions/**`, `receivables/**`,
`components/accounts/**`, `components/transactions/**`, `components/receivables/**`, `lib/receivable-format.ts`, `lib/shared-expense.ts`.
- **Accounts list:** each balance in its account currency, with `≈ balance_base` when it differs (or a missing-rate warning). Any
  group or overall totals must use `balance_base` (skip accounts without it, and show `<FxNotice>`). Never sum raw balances
  across currencies.
- **Create account:** a currency select defaulting to `useBaseCurrency()` (the dialog already has currency state; verify every
  account type).
- **Transactions:** amounts in the transaction's account currency (look up the account from the accounts query by
  `account_id`). For transfers show `RM 4,460.00 → US$1,100.00` and the implied rate when `to_amount` is set.
- **Create transfer:** when the two accounts' currencies differ, show "Amount received (<CCY>)" (required). Pre-fill it from
  `/fx-rates/latest`, editable, with the implied rate shown live. Handle `FX_AMOUNT_REQUIRED` as above.
- **Receivables:**
  - amounts in `receivable.currency`;
  - settle dialog: when the target account's currency differs, show "Amount received in <account>" (`account_amount`), with the
    same suggestion/error handling;
  - summary totals in base, with `by_currency` shown and `<FxNotice>`;
  - settlement history shows `account_amount` when present.

## F4 — Dashboard, analytics, budgets (after F1)
`app/(dashboard)/page.tsx`, `components/dashboard/**`, `app/(dashboard)/analytics/**`, `components/analytics/**`,
`app/(dashboard)/budgets/**`, `components/dashboard/budgets-card.tsx`, `hooks/use-budgets.ts`.
- Use each response's `currency` for formatting (base). Show `<FxNotice>` where `fx_missing` / `unconverted` is present.
- **Net worth chart:** format with each snapshot's `currency`. If snapshots in range have different currencies (the user changed
  base), show a note, and don't connect the line across the change.
- **Composition card:** use `balance_base` / base values, never raw cross-currency sums.
- **Top expenses:** rank and show `amount_base`, with the native amount + currency underneath when it differs.
- **Budgets:** create/edit gains a currency select (default base). Progress is formatted in `budget.currency`.

## F5 — Cleanup (last)
Delete `formatCurrency` (the build must pass with zero call sites left), update the CLAUDE.md frontend notes (`<Money>`, `formatMoney`,
`useBaseCurrency`, the pages list now includes analytics/budgets/receivables/rules), and run lint + build.

## Visual check (reviewer)
Done by the reviewer, not the agents: Playwright screenshots of the investments, accounts, transactions, dashboard and settings pages
against a seeded API (MYR user, a USD cash account, USD holdings, a cross-currency transfer), at desktop and phone width.
