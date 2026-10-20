# Plan 021 end-to-end run (Playwright)

The scenario behind the PR screenshots. It runs against a real stack:
Postgres 16, the API on :8080 and the production web build on :3000.

1. Start Postgres and run the API: `ENV=development JWT_SECRET=… DB_*=… CORS_ORIGIN=http://localhost:3000`.
   Migrations run on start.
2. Build and start the web app: `pnpm build && pnpm start -p 3000`.
3. Seed a demo user with base data: `./seed.sh`.
4. Run the scenario: `npm i playwright@1.56.1`, then `node run.mjs <out-dir> all`.
   `run.mjs` imports `steps-all.mjs` and writes screenshots to `<out-dir>/shots/`.

The scenario covers:
- a 3-way split;
- a work claim;
- a deposit;
- recording a repayment;
- lending money;
- a partial deposit return with write-off;
- the Owed to me filters;
- budgets, analytics and the dashboard;
- the read-only repayment view;
- mobile.

Final API state is checked separately; see the PR description.
