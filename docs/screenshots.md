# Screenshots

`apps/web/scripts/screenshots.mjs` captures every route in light and dark at
1440x900 and 390x844 (viewport shots, animations disabled). It logs in to a
running instance; it starts nothing. Used for the landing page (`apps/site`)
and for visual checks of the web app.

## Seed a demo household

The sample household is built by the api itself (`POST /demo/load`), so there
is no separate seed script. Run an api with `DEMO_ENABLED=true` and either:

- pass `--seed` to the script, which registers the account if the login fails
  and loads the household, or
- run `deploy/demo/reset.sh` (see [public-demo.md](public-demo.md)), which also
  records a net-worth history so the dashboard chart is filled.

## Capture

Playwright is a dev dependency of `apps/web`. If its browser is not installed,
`pnpm exec playwright install chromium`, or point `--chromium` at an existing
executable (the script also tries `/opt/pw-browsers/chromium`).

```bash
cd apps/web
pnpm screenshots --email demo@kuberan.example.com --password '...' \
  --web http://localhost:3000 --api http://localhost:8080 --out ./shots --seed
```

Output is `<route>-<width>-<theme>.png` (for example `dashboard-1440-dark.png`,
`transactions-390-light.png`), 17 routes x 2 themes x 2 viewports (the two
sign-in pages are captured signed out). Useful flags:

| Flag | Default | Meaning |
|---|---|---|
| `--routes a,b` | all | Route slugs: `login`, `register`, `dashboard`, `accounts`, `account-detail`, `transactions`, `categories`, `investments`, `investment-detail`, `securities`, `security-detail`, `analytics`, `budgets`, `receivables`, `rules`, `settings`, `more` |
| `--themes` | `light,dark` | |
| `--viewports` | `desktop,mobile` | 1440x900 and 390x844 |
| `--scale N` | `1` | Device scale factor; use `2` for sharp phone shots |
| `--settle MS` | `1500` | Wait for charts to finish animating |
| `--chromium PATH` | | Browser executable |

The flags also read `KUBERAN_WEB_URL`, `KUBERAN_API_URL`, `KUBERAN_EMAIL`,
`KUBERAN_PASSWORD` and `KUBERAN_SHOTS_DIR`. Run against a production build
(`pnpm build && pnpm start`) so the shots match what users see; the api must
allow the web origin in `CORS_ORIGIN`.

## Landing page images

Convert the shots you want to WebP (quality 80) into `apps/site/img/` using the
names `index.html` expects (see `apps/site/README.md`).
