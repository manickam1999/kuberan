# Kuberan

A self-hosted, privacy-first personal finance application. Named after the Hindu god of wealth (Kubera).

Built with a Go backend and Next.js frontend, organized as a monorepo.

## Features

- **Account Management** -- Cash, investment, credit card, and debt accounts with multi-currency support
- **Transaction Tracking** -- Income, expenses, and account-to-account transfers with editing support
- **Analytics & Charts** -- Spending by category, monthly income/expenses summary, daily spending trends
- **Budgets** -- Monthly/yearly budgets with spending progress tracking
- **Investment Portfolio** -- Track stocks, ETFs, bonds, crypto, and REITs with buy/sell/dividend/split transactions and realized gain/loss tracking
- **Securities & Pricing** -- Browse securities, view price history, pipeline API for automated price ingestion
- **Portfolio Snapshots** -- Historical net worth tracking (cash + investments - debt) with time-series charts
- **Categories** -- Hierarchical income/expense categories with icons and colors
- **Receipt Attachments** -- Attach receipt images/PDFs to transactions, stored in a private S3-compatible object store (MinIO) with server-side sanitization; see [docs/receipts.md](docs/receipts.md)
- **Telegram Bot** -- Link a Telegram account and interact with your finances via chat (`apps/bot`)
- **Price Oracle** -- Standalone worker that ingests security prices and triggers portfolio snapshots via the pipeline API (`apps/oracle`)
- **MCP Server** -- Read-only finance tools for MCP clients (e.g. Claude connectors), secured with OAuth 2.1 via Ory Hydra (`apps/api/cmd/mcp`)
- **Automated Backups** -- Cron-based pg_dump backup service with retention pruning, plus off-host client-side-encrypted replication of the DB dumps and receipts bucket to Cloudflare R2 (`apps/backup`); see [docs/backup-and-dr.md](docs/backup-and-dr.md)
- **Dark Mode** -- Dark-first UI with light and system theme support
- **Audit Logging** -- All sensitive operations are logged for accountability

## Tech Stack

| Layer        | Technology                                        |
|--------------|---------------------------------------------------|
| Backend      | Go 1.24, Gin, GORM, PostgreSQL 16                |
| Frontend     | Next.js 15 (App Router), React 19, Tailwind CSS v4, ShadCN UI, react-query, Recharts |
| Auth         | JWT (access + refresh tokens), bcrypt; OAuth 2.1 via Ory Hydra for MCP clients |
| Logging      | Zap (structured)                                  |
| Migrations   | golang-migrate (SQL-based)                        |
| API Docs     | Swagger/OpenAPI via swaggo                        |
| Dev Env      | Docker Compose, Air (hot reload), Turbopack       |

## Project Structure

```
/
├── apps/
│   ├── api/                  # Go backend (Gin + GORM + PostgreSQL); also hosts the MCP server (cmd/mcp)
│   ├── web/                  # Next.js frontend (React 19 + Tailwind CSS v4)
│   ├── bot/                  # Python Telegram bot
│   ├── oracle/               # Go price-ingestion + snapshot worker
│   └── backup/               # Cron-based pg_dump backup service
├── deploy/                   # Deployment scripts
├── docs/                     # Documentation (MCP OAuth, database setup, receipts, backup & DR)
├── ory/                      # Ory Hydra (OAuth authorization server) config
├── plans/                    # Architecture and upgrade plans
├── scripts/                  # Utility scripts (check-go.sh, etc.)
├── docker-compose.yml        # Development environment
└── docker-compose.prod.yml   # Production environment
```

## Getting Started

### Prerequisites

- Docker and Docker Compose
- Go 1.24+ (for local backend development)
- Node.js 20+ (for local frontend development)
- Python 3 (for local Telegram bot development)

### Development (Docker)

```bash
# Start the default services (API + frontend + MCP + Hydra + PostgreSQL)
npm run dev
```

This starts:
- **API** at http://localhost:8080
- **Frontend** at http://localhost:3000
- **MCP server** at http://localhost:8081
- **Ory Hydra** (OAuth authorization server) at http://localhost:4444 (public), plus a one-shot `hydra-migrate` job
- **PostgreSQL** on port 5433 (host) / 5432 (container)
- **Swagger UI** at http://localhost:8080/swagger/index.html

The `bot`, `oracle`, and `backup` services sit behind Docker Compose profiles and are not started by default (`npm run dev:bot` starts the bot profile).

### Development (Local)

```bash
# Backend only (requires PostgreSQL running)
cd apps/api && air

# Frontend only
cd apps/web && pnpm dev
```

### Common Commands

```bash
# Run backend tests
cd apps/api && go test ./... -v

# Run tests with coverage
cd apps/api && go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out

# Run full verification (build + vet + lint + test + race detection)
./scripts/check-go.sh apps/api
./scripts/check-go.sh apps/oracle

# Quick check (build + vet + lint, no tests)
cd apps/api && make check-fast

# Run migrations
cd apps/api && go run cmd/migrate/main.go up
cd apps/api && go run cmd/migrate/main.go down 1

# Build backend
cd apps/api && go build -o bin/api ./cmd/api

# Generate Swagger docs
cd apps/api && swag init -g cmd/api/main.go -d . --output internal/docs --parseDependency

# Lint
cd apps/api && golangci-lint run ./...
```

## API Overview

All endpoints are prefixed with `/api/v1/`. Authentication uses Bearer JWT tokens.

**Public:** Register, login, token refresh, health check (`/api/health`), Swagger docs, and the OAuth endpoints for the MCP flow (login/consent bridge under `/api/v1/oauth/*` and the RFC 7591 dynamic client registration proxy at `/oauth2/register`).

**Protected:** CRUD for accounts (cash, investment, credit card), transactions, categories, budgets, and investments. Includes transfer support, budget progress tracking, portfolio summary, spending analytics (by category, monthly summary, daily trends), securities browsing, portfolio snapshots, user profile, and Telegram account linking.

**Internal (bot secret auth):** Telegram link completion, user resolution, and activity tracking for the bot service.

**Pipeline (API key auth):** Security creation and listing, price recording, and portfolio snapshot computation for automated data ingestion.

See the [Swagger UI](http://localhost:8080/swagger/index.html) for full API documentation or refer to `CLAUDE.md` for the complete endpoint listing.

## Architecture

The backend follows a 3-layer architecture:

```
Handlers (HTTP) → Services (Business Logic) → Models (Database)
```

Key patterns:
- Interface-based services for testability
- All monetary values stored as **int64 cents** (not floats)
- Custom `AppError` types with error codes and HTTP status mapping
- Soft deletes on all models
- User-scoped queries for data isolation
- Audit logging on sensitive operations

The MCP server (`apps/api/cmd/mcp`) exposes read-only finance tools to MCP
clients (e.g. Claude connectors) and authenticates them with OAuth 2.1, using
Ory Hydra as the authorization server. See
[docs/mcp-oauth.md](docs/mcp-oauth.md) for the flow, the rationale, and
deployment topology.

## Environment Variables

See `.env.prod.example` for the full documented production template and `apps/api/.env` for local development values. Key variables:

| Variable              | Description                                         | Default            |
|-----------------------|-----------------------------------------------------|--------------------|
| `ENV`                 | Environment (development/production)                | `development`      |
| `PORT`                | API server port                                     | `8080`             |
| `DB_HOST` / `DB_PORT` | PostgreSQL host/port (`5433` on host in local dev)  | `localhost`/`5432` |
| `JWT_SECRET`          | JWT signing key (required in prod)                  | dev default        |
| `PIPELINE_API_KEY`    | API key for pipeline endpoints (used by the oracle) | --                 |
| `BOT_INTERNAL_SECRET` | Shared secret between the API and the Telegram bot  | --                 |
| `CORS_ORIGIN`         | Allowed CORS origin                                 | `*`                |
| `HYDRA_*`             | Ory Hydra wiring (DSN, issuer/admin URLs, login/consent URLs) | see `.env.prod.example` |
| `MCP_RESOURCE_URL` / `MCP_PORT` | MCP resource server URL and port          | `http://localhost:8081` / `8081` |
| `TELEGRAM_BOT_TOKEN`  | Telegram bot token (bot service)                    | --                 |

## License

Private project. All rights reserved.
