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
- **Dark Mode** -- Light, dark, and system theme support
- **Audit Logging** -- All sensitive operations are logged for accountability

## Tech Stack

| Layer        | Technology                                        |
|--------------|---------------------------------------------------|
| Backend      | Go 1.24, Gin, GORM, PostgreSQL 16                |
| Frontend     | Next.js 15 (App Router), React 19, Tailwind CSS v4, ShadCN UI, react-query, Recharts |
| Auth         | JWT (access + refresh tokens), bcrypt             |
| Logging      | Zap (structured)                                  |
| Migrations   | golang-migrate (SQL-based)                        |
| API Docs     | Swagger/OpenAPI via swaggo                        |
| Dev Env      | Docker Compose, Air (hot reload), Turbopack       |

## Project Structure

```
/
├── apps/
│   ├── api/              # Go backend (Gin + GORM + PostgreSQL)
│   └── web/              # Next.js frontend (React 19 + Tailwind CSS v4)
├── packages/             # Shared code between apps
├── plans/                # Architecture and upgrade plans
├── scripts/              # Utility scripts
├── tools/                # Development tools
└── docker-compose.yml    # Development environment setup
```

## Getting Started

### Prerequisites

- Docker and Docker Compose
- Go 1.24+ (for local backend development)
- Node.js 20+ (for local frontend development)

### Development (Docker)

```bash
# Start all services (API + frontend + PostgreSQL)
npm run dev
```

This starts:
- **API** at http://localhost:8080
- **Frontend** at http://localhost:3000
- **PostgreSQL** on port 5433 (host) / 5432 (container)
- **Swagger UI** at http://localhost:8080/swagger/index.html

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
cd apps/api && ./scripts/check.sh

# Quick check (build + vet + lint, no tests)
cd apps/api && make check-fast

# Run migrations
cd apps/api && go run cmd/migrate/main.go up
cd apps/api && go run cmd/migrate/main.go down 1

# Build backend
cd apps/api && go build -o bin/api ./cmd/api

# Generate Swagger docs
cd apps/api && swag init -g cmd/api/main.go -d . --output internal/docs

# Lint
cd apps/api && golangci-lint run ./...
```

## API Overview

All endpoints are prefixed with `/api/v1/`. Authentication uses Bearer JWT tokens.

**Public:** Register, login, token refresh, health check, Swagger docs.

**Protected:** CRUD for accounts (cash, investment, credit card), transactions, categories, budgets, and investments. Includes transfer support, budget progress tracking, portfolio summary, spending analytics (by category, monthly summary, daily trends), securities browsing, and portfolio snapshots.

**Pipeline (API key auth):** Security creation, price recording, and portfolio snapshot computation for automated data ingestion.

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

See `apps/api/.env` for backend configuration. Key variables:

| Variable      | Description                          | Default       |
|---------------|--------------------------------------|---------------|
| `ENV`         | Environment (development/production) | `development` |
| `PORT`        | Server port                          | `8080`        |
| `DB_HOST`     | PostgreSQL host                      | `localhost`   |
| `DB_PORT`     | PostgreSQL port                      | `5433`        |
| `JWT_SECRET`  | JWT signing key (required in prod)   | dev default   |

## License

Private project. All rights reserved.
