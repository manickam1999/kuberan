# End-to-end tests

`apps/web/e2e/` is a Playwright suite that drives the web app like a user
(real browser, real API, real Postgres) and checks the result twice: what the
page shows, and what the API stored (balances in cents). It starts nothing; you
point it at a running instance.

## What it needs

- A Postgres database of its own (never your real one: the suite registers a
  fresh user per run, but it also writes FX rates and securities, which are
  global).
- The API with migrations applied, running with the rate limits off (the suite
  logs in once per test):

  ```bash
  cd apps/api
  DB_HOST=localhost DB_PORT=5432 DB_USER=kuberan DB_PASSWORD=kuberan \
  DB_NAME=kuberan_e2e DB_SSLMODE=disable ENV=development PORT=8195 \
  COOKIE_SECURE=false CORS_ORIGIN=http://localhost:3315 \
  PIPELINE_API_KEY=<32+ chars> \
  RATE_LIMIT_LOGIN_PER_MINUTE=0 RATE_LIMIT_LOGIN_EMAIL_PER_HOUR=0 \
  RATE_LIMIT_REGISTER_PER_HOUR=0 RATE_LIMIT_REFRESH_PER_MINUTE=0 \
  RATE_LIMIT_DCR_PER_HOUR=0 RATE_LIMIT_PASSWORD_PER_HOUR=0 \
  go run ./cmd/api
  ```

- A production build of the web app compiled against that API (not `pnpm dev`):

  ```bash
  cd apps/web
  NEXT_PUBLIC_API_URL=http://localhost:8195 pnpm build
  PORT=3315 NEXT_PUBLIC_API_URL=http://localhost:8195 pnpm start
  ```

- A Chromium. Playwright's own is used when installed; otherwise the config
  falls back to `/opt/pw-browsers/chromium`, or set `E2E_CHROMIUM=/path/to/chrome`.

## Run

```bash
cd apps/web
E2E_WEB_URL=http://localhost:3315 \
E2E_API_URL=http://localhost:8195 \
E2E_PIPELINE_KEY=<the api's PIPELINE_API_KEY> \
pnpm e2e                      # everything
pnpm e2e investments          # one spec file (substring match)
pnpm e2e -g "bulk"            # by test title
```

`pnpm e2e` runs `playwright test -c e2e/playwright.config.ts --trace off`. Two
projects exist: `desktop` (1440x900, all specs) and `mobile` (390x844, touch;
only `*.mobile.spec.ts` and `*.both.spec.ts`). Failure screenshots land in
`apps/web/e2e/.run/artifacts` (git-ignored).

## How it is organised

- `global-setup.ts` seeds once per run: ~200 days of USD->MYR and SGD->MYR
  rates (today: 4.5000 and 3.3500), a USD and a MYR security with 30 days of
  prices, then registers a user with base currency MYR, with MYR, USD and SGD
  cash accounts, a USD investment account, categories and a few transactions.
  Ids go to `e2e/.run/state.json`.
- `lib/fixtures.ts` signs in through the real login form per test (`app`),
  gives a typed API client (`api`) and `mkCash` to create an isolated account.
  **Any `console.error` or uncaught page error fails the test**, except the
  expected 401 of the session bootstrap on `/auth/refresh`.
- Each test creates the accounts it mutates, so balance assertions are exact;
  aggregate pages (insights, portfolio) are compared with the API and with an
  independent conversion computed in the test.
- Tests run serially (one worker) against one shared user.

## Not covered

- Receipt upload needs the S3/MinIO blob store (`STORAGE_BUCKET`); the receipt
  test skips itself when the API reports the feature disabled.
- Telegram linking itself (needs a bot), OAuth/MCP consent, IBKR and CSV
  import file flows.
