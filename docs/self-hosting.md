# Self-hosting Kuberan

From a fresh server to a signed-in dashboard. The stack is a Go API, a Next.js
web app, Postgres, Ory Hydra (sign-in for AI assistants), MinIO (receipt
storage) and optional extras (Telegram bot, price worker, backups), all run by
Docker Compose. This page is the short path; the linked docs go deep.

## Prerequisites

- A Linux machine (a small VPS is fine: allow about 2 GB of RAM) with **Docker
  and Docker Compose v2**, plus `git` and `openssl`. `deploy/setup-vps.sh`
  installs Docker and clones the repo on Ubuntu 22.04+ / Debian 12+.
- A **domain** and a way to serve it over **HTTPS**. Kuberan's published ports
  bind to `127.0.0.1` only; a reverse proxy or tunnel on the same host exposes
  them (see [Reverse proxy and TLS](#reverse-proxy-and-tls)).
- A Postgres database: the bundled container (recommended) or a managed one
  such as Supabase ([database-setup.md](database-setup.md)).

## 1. Get the code and configure

```bash
git clone https://github.com/zihangg/kuberan /opt/kuberan
cd /opt/kuberan
cp .env.prod.example .env.prod
chmod 600 .env.prod
```

Edit `.env.prod`. Compose refuses to start unless these are set (generate
secrets with `openssl rand -hex 32`):

| Variable | What to put |
|---|---|
| `JWT_SECRET` | At least 32 bytes. Signs access tokens; changing it signs everyone out. |
| `CORS_ORIGIN` | The exact web origin, e.g. `https://app.example.com` (no `*`). |
| `NEXT_PUBLIC_API_URL` | The public URL of the api, e.g. `https://api.example.com`. Baked into the web build, so a change needs a redeploy. |
| `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`, `DB_PORT` | See below. |
| `HYDRA_DSN` | Connection string for Hydra's own database (`hydra`). |
| `HYDRA_SECRETS_SYSTEM` | Hydra's system secret; keep it stable. |
| `HYDRA_ISSUER_URL`, `HYDRA_LOGIN_URL`, `HYDRA_CONSENT_URL`, `MCP_RESOURCE_URL` | Public URLs for the AI-assistant sign-in (`auth.`, `app.` and `mcp.` hostnames). Required by compose even if you never connect an assistant. |
| `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY` | Receipt storage credentials. The access key is 3-20 characters and the secret 8-40 (`openssl rand -hex 8` and `-hex 16`). |

Also set `PIPELINE_API_KEY` (shared with the price worker) and
`BOT_INTERNAL_SECRET` (shared with the bot) if you use those. Everything else is
optional and documented inline in `.env.prod.example`, including
`DEFAULT_PALETTE`, rate limits, MCP write limits and `NEXT_PUBLIC_MCP_URL`.
`DEMO_ENABLED` must stay `false` on a real instance.

**Database, bundled Postgres (Option A).** In `.env.prod`:

```bash
COMPOSE_PROFILES=postgres,backup
DB_HOST=postgres
DB_PORT=5432
DB_USER=kuberan
DB_PASSWORD=<openssl rand -hex 32>
DB_NAME=kuberan
DB_SSLMODE=disable
HYDRA_DSN=postgres://kuberan:<same password>@postgres:5432/hydra?sslmode=disable
```

Then create Hydra's database once, before the first deploy:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d postgres
docker compose -f docker-compose.prod.yml --env-file .env.prod exec postgres \
  psql -U kuberan -c "CREATE DATABASE hydra OWNER kuberan;"
```

For Supabase, follow Option B in [database-setup.md](database-setup.md).

## 2. Deploy

```bash
./deploy/deploy.sh
```

It checks `JWT_SECRET` and `CORS_ORIGIN`, builds images tagged with the git SHA,
takes a pre-migration backup (when the `backup` profile is on), runs the
migrations, starts every service and waits for health. It prints rollback
instructions if anything fails. The first build takes several minutes.

Check it from the host:

```bash
curl http://127.0.0.1:8080/api/health      # {"database":"connected","status":"ok"}
docker compose -f docker-compose.prod.yml --env-file .env.prod ps
```

## 3. First login

Open your web URL, choose **Create an account** and register, then add your
first account from the dashboard. Registration stays open (rate limited by
`RATE_LIMIT_REGISTER_PER_HOUR`); if the instance is public and only for you,
restrict access at the proxy after you have registered.

Reporting currency, theme and palette are under Settings; the sidebar shows a
"Self-hosted" status. To explore with sample data first, the demo household can
only be loaded when `DEMO_ENABLED=true`, which belongs on a throwaway instance
([public-demo.md](public-demo.md)).

## 4. Optional pieces

- **Prices and net-worth history.** Run the price worker (`oracle`) on a
  schedule: `docker compose -f docker-compose.prod.yml run --rm oracle`.
  `deploy/setup-vps.sh` installs a root cron entry for it every 30 minutes.
  Without it, investments have no prices and the net-worth chart stays empty.
  See [multi-currency.md](multi-currency.md) for FX and backfills.
- **Telegram bot:** [telegram-bot.md](telegram-bot.md).
- **Claude and other AI assistants:** [connect-ai-assistant.md](connect-ai-assistant.md)
  (set `NEXT_PUBLIC_MCP_URL` and route the `mcp.` and `auth.` hostnames).
- **Receipts:** [receipts.md](receipts.md). **Imports:** [csv-import.md](csv-import.md),
  [ibkr-import.md](ibkr-import.md).

## Reverse proxy and TLS

The ports are loopback-only on purpose (`PUBLISH_ADDRESS=127.0.0.1`): do not
set `0.0.0.0` on a machine with a public IP. Terminate TLS in front and forward
by hostname:

| Hostname | Forward to | Needed for |
|---|---|---|
| `app.<domain>` | `127.0.0.1:3000` | The web app and the assistant sign-in pages |
| `api.<domain>` | `127.0.0.1:8080` | The REST API (the origin in `NEXT_PUBLIC_API_URL`) |
| `auth.<domain>` path `/oauth2/register` | `127.0.0.1:8080` | Assistant client registration (must come before the next row) |
| `auth.<domain>` all other paths | `127.0.0.1:4444` | Hydra (authorize, token, keys) |
| `mcp.<domain>` | `127.0.0.1:8081` | The MCP server |

Only the first two are needed without AI assistants. The reference setup is a
Cloudflare tunnel running on the host (see the comments in
`docker-compose.prod.yml` and [mcp-oauth.md](mcp-oauth.md)); Caddy or nginx work
the same way. Never route Hydra's admin port (4445): it is not published.

If the proxy reaches the containers from a Docker network rather than the host,
set `API_TRUSTED_PROXY_CIDRS`, `MCP_TRUSTED_PROXY_CIDRS` and
`HYDRA_TLS_TERMINATION_CIDR` to that network (default `172.30.10.0/24`, the
bundled "edge" network), so rate limits and audit logs see real client IPs. The
refresh cookie is `Secure`, so use HTTPS from the start.

## Upgrades and migrations

```bash
cd /opt/kuberan
git pull
./deploy/deploy.sh
```

Migrations run as part of the deploy, after the backup. Read the
[upgrade notes](backup-and-dr.md#upgrade-notes) when a release changes
`.env.prod`, and [backup-and-dr.md](backup-and-dr.md#6--deploys-and-rollback)
for rollback. New environment variables are listed in `.env.prod.example`;
copy any you want into `.env.prod`.

## Backups

With `COMPOSE_PROFILES` including `backup`, a daily `pg_dump` (plus Hydra's
database) lands in `./backups` and is verified. Off-site encrypted copies to
Cloudflare R2, the receipts mirror, restore drills and failure response are in
[backup-and-dr.md](backup-and-dr.md). Settings > Data also exports everything as
CSV.

## Troubleshooting

- `required variable ... is missing`: a variable from the table above is empty.
- `hydra-migrate` fails: the `hydra` database does not exist yet (step 1).
- Login works but the page cannot reach the api: `NEXT_PUBLIC_API_URL` or
  `CORS_ORIGIN` does not match the real origins; fix and redeploy.
- Logs: `docker compose -f docker-compose.prod.yml --env-file .env.prod logs --tail=200 api`.
