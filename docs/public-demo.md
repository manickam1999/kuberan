# Running a public demo

A try-it-yourself instance for the landing page (`apps/site`): visitors sign in
with a shared login, poke around, and may change anything. Every night the
whole database is dropped and the sample household is loaded again.

This is a separate stack from production. Never run it against a database you
care about.

## What the demo is

`docker-compose.demo.yml` runs four services:

| Service | Role |
|---|---|
| `postgres` | Throwaway Postgres 16 in its own volume (`kuberan-demo_demo_postgres`) |
| `api` | The normal api image with `DEMO_ENABLED=true` |
| `web` | The normal web image |
| `demo-reset` | `deploy/demo/reset.sh` once at start, then nightly at 03:00 UTC (supercronic, same pattern as `apps/backup`) |

What is deliberately not there:

- **MCP is off.** No `mcp` or `hydra` service runs, `MCP_WRITE_ENABLED` is
  `false` and `NEXT_PUBLIC_MCP_URL` is empty, so nobody can connect an AI
  assistant to the demo and Settings > Connected apps just links to the docs.
- No MinIO (receipt uploads are unavailable), no Telegram bot, no oracle (the
  sample holdings carry their own prices; the foreign-currency sample account is
  only added when an FX rate exists, so the demo stays single-currency), no
  backups.

Writes are allowed. Visitors can add transactions, delete accounts or register
their own account (rate limited by `RATE_LIMIT_REGISTER_PER_HOUR`, default 10
per hour per IP in this stack). All of it disappears at the next reset.

## Setup

```bash
cp .env.demo.example .env.demo
# edit: JWT_SECRET, DB_PASSWORD, the two public URLs, DEMO_EMAIL / DEMO_PASSWORD
docker compose -f docker-compose.demo.yml --env-file .env.demo up -d --build
```

Publish the two loopback ports (`3000` web, `8080` api; override with
`DEMO_WEB_PORT` / `DEMO_API_PORT`) through your reverse proxy or tunnel, e.g.
`demo.<domain>` to the web port and `demo-api.<domain>` to the api port, and set
`NEXT_PUBLIC_API_URL` / `CORS_ORIGIN` in `.env.demo` to match. If the proxy
connects from a Docker network, set `API_TRUSTED_PROXY_CIDRS` to it so rate
limits see real client IPs. See [self-hosting.md](self-hosting.md) for TLS notes.

The demo login is `DEMO_EMAIL` / `DEMO_PASSWORD` from `.env.demo`. It is public
on purpose: put it next to the demo link on the landing page.

## How the reset works

`deploy/demo/reset.sh`, in order:

1. Wait for Postgres.
2. `DROP DATABASE ... WITH (FORCE)` and `CREATE DATABASE` (the api's pooled
   connections are cut and reconnect on their own).
3. `migrate up` (the same migrations as production).
4. Wait for `GET /api/health`.
5. `POST /auth/register` for the demo login, `POST /auth/login`.
6. `POST /demo/load` to create the sample household (accounts, about three
   months of transactions, a budget, holdings with prices, receivables).

It uses only existing endpoints; nothing in the api is demo-specific besides
`DEMO_ENABLED`. Dates in the sample data are relative to the day it is loaded,
so the demo always looks current. The script refuses to run unless
`DEMO_RESET_CONFIRM=drop-database`, which only the demo compose file sets.

Signed-in demo sessions end at the reset (the user no longer exists); visitors
land on the login page again.

## Operating it

```bash
# Run a reset now
docker compose -f docker-compose.demo.yml --env-file .env.demo exec demo-reset /reset.sh
# Watch the reset logs
docker compose -f docker-compose.demo.yml --env-file .env.demo logs -f demo-reset
# Upgrade
git pull && docker compose -f docker-compose.demo.yml --env-file .env.demo up -d --build
```

To change the schedule, edit `deploy/demo/crontab` and rebuild. If a reset
fails, the previous (or an empty) database stays up; the log line says which
step failed.
