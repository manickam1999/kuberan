# Kuberan API

The backend service for Kuberan, built with Go 1.24, Gin, GORM, and PostgreSQL 16. This module also hosts the MCP server (`cmd/mcp`), a standalone binary that exposes read-only finance tools to MCP clients and validates OAuth 2.1 tokens issued by Ory Hydra (see `cmd/mcp/README.md` and `docs/mcp-oauth.md`).

## Architecture

3-layer architecture with interface-based services:

```
Handlers (HTTP) → Services (Business Logic) → Models (Database/GORM)
```

```
apps/api/
├── cmd/
│   ├── api/main.go           # API server entrypoint
│   ├── mcp/main.go           # MCP server entrypoint (OAuth Resource Server)
│   └── migrate/main.go       # Migration CLI tool
├── migrations/               # SQL migration files (golang-migrate)
├── Makefile                  # Dev targets
├── internal/
│   ├── config/               # Environment-based configuration
│   ├── database/             # DB connection, pooling, health check
│   ├── docs/                 # Generated Swagger docs
│   ├── errors/               # Custom AppError types with codes
│   ├── handlers/             # HTTP handlers (thin, delegate to services)
│   ├── hydra/                # Ory Hydra admin API client
│   ├── logger/               # Zap structured logger
│   ├── mcp/                  # MCP server, tools, JWKS validator, discovery
│   ├── middleware/            # Auth, error handling, request logging
│   ├── models/               # GORM models (single source of truth)
│   ├── pagination/           # Generic PageRequest/PageResponse[T]
│   ├── services/             # Business logic layer (interface-based)
│   ├── testutil/             # Test helpers (DB setup, fixtures, assertions)
│   ├── uuid/                 # UUID helpers
│   └── validator/            # Custom Gin validators
└── tests/
    └── integration/          # End-to-end workflow tests
```

## Development

### Prerequisites

- Go 1.24+
- PostgreSQL 16 (or use Docker Compose from the repo root)
- [Air](https://github.com/air-verse/air) for hot reload
- [golangci-lint](https://golangci-lint.run/) for linting
- [swag](https://github.com/swaggo/swag) for Swagger doc generation

### Running

```bash
# Via Docker Compose (from repo root, starts API + web + MCP + Hydra + PostgreSQL)
npm run dev

# Locally with Air hot reload (requires PostgreSQL running)
make dev

# Or directly
go run cmd/api/main.go
```

The server runs at http://localhost:8080. Swagger UI is available at http://localhost:8080/swagger/index.html.

### Makefile Targets

| Target            | Description                                      |
|-------------------|--------------------------------------------------|
| `make dev`        | Run with Air hot reload                          |
| `make build`      | Build API binary to `bin/api`                    |
| `make build-mcp`  | Build MCP server binary to `bin/mcp`             |
| `make test`       | Run all tests                                    |
| `make test-cover` | Run tests with HTML coverage report              |
| `make test-race`  | Run tests with race detector                     |
| `make lint`       | Run golangci-lint                                |
| `make fmt`        | Format code (gofmt + goimports)                  |
| `make check-fast` | Quick check: build + vet + lint (no tests)       |
| `make check`      | Full verification: build + vet + lint + test + race |
| `make migrate-up`   | Run all pending migrations                     |
| `make migrate-down` | Roll back the last migration                   |
| `make migrate-version` | Show current migration version              |
| `make swagger`    | Regenerate Swagger docs                          |
| `make clean`      | Remove build artifacts and coverage files        |

### Verification

After any code change, run:

```bash
# Quick feedback (compile + lint)
make check-fast

# Full verification (must pass before merging)
make check
```

## API Endpoints

### Public

```
POST /api/v1/auth/register     # Register new user
POST /api/v1/auth/login        # Login, returns access + refresh tokens
POST /api/v1/auth/refresh      # Refresh access token
GET  /api/health               # Health check (includes DB ping)
GET  /swagger/*                # Swagger UI

# OAuth login/consent bridge (drives the Hydra authorization flow for MCP clients)
POST /api/v1/oauth/login
POST /api/v1/oauth/login/reject
GET  /api/v1/oauth/consent
POST /api/v1/oauth/consent/accept
POST /api/v1/oauth/consent/reject
POST /oauth2/register          # Hardened RFC 7591 DCR proxy (alias: POST /api/v1/oauth/register)
```

### Protected (require Bearer token)

```
# User
GET    /api/v1/profile

# Accounts
POST   /api/v1/accounts/cash
POST   /api/v1/accounts/investment
POST   /api/v1/accounts/credit-card
GET    /api/v1/accounts
GET    /api/v1/accounts/:id
PUT    /api/v1/accounts/:id
GET    /api/v1/accounts/:id/transactions
GET    /api/v1/accounts/:id/investments

# Transactions
GET    /api/v1/transactions
POST   /api/v1/transactions
POST   /api/v1/transactions/transfer
GET    /api/v1/transactions/spending-by-category
GET    /api/v1/transactions/monthly-summary
GET    /api/v1/transactions/daily-spending
GET    /api/v1/transactions/:id
PUT    /api/v1/transactions/:id
DELETE /api/v1/transactions/:id

# Categories
POST   /api/v1/categories
GET    /api/v1/categories
GET    /api/v1/categories/:id
PUT    /api/v1/categories/:id
DELETE /api/v1/categories/:id

# Budgets
POST   /api/v1/budgets
GET    /api/v1/budgets
GET    /api/v1/budgets/:id
PUT    /api/v1/budgets/:id
DELETE /api/v1/budgets/:id
GET    /api/v1/budgets/:id/progress

# Investments
POST   /api/v1/investments
GET    /api/v1/investments
GET    /api/v1/investments/portfolio
GET    /api/v1/investments/snapshots
GET    /api/v1/investments/:id
POST   /api/v1/investments/:id/buy
POST   /api/v1/investments/:id/sell
POST   /api/v1/investments/:id/dividend
POST   /api/v1/investments/:id/split
GET    /api/v1/investments/:id/transactions

# Securities
GET    /api/v1/securities
GET    /api/v1/securities/:id
GET    /api/v1/securities/:id/prices

# Telegram
GET    /api/v1/telegram/link
POST   /api/v1/telegram/generate-code
DELETE /api/v1/telegram/unlink
```

### Internal (require bot secret via `InternalAuthMiddleware`)

```
POST   /api/v1/internal/telegram/complete-link
GET    /api/v1/internal/telegram/resolve/:telegram_user_id
POST   /api/v1/internal/telegram/activity/:telegram_user_id
```

### Pipeline (require API key via X-API-Key header)

```
GET    /api/v1/pipeline/securities          # List all securities
POST   /api/v1/pipeline/securities          # Create security
POST   /api/v1/pipeline/securities/prices   # Record security prices
POST   /api/v1/pipeline/snapshots           # Compute portfolio snapshots for all users
```

## Key Design Decisions

- **Monetary values as int64 cents** -- `$10.50` = `1050`. No floating-point rounding errors.
- **SQL migrations** via golang-migrate, not GORM AutoMigrate. Version-controlled and reversible.
- **Soft deletes** on all models. Deleted categories remain as references for existing transactions.
- **User-scoped queries** -- every data query includes `user_id` for data isolation.
- **Atomic operations** -- all balance-affecting operations wrapped in DB transactions.
- **Audit logging** -- sensitive operations logged to `audit_logs` table.
- **JWT auth** -- short-lived access tokens (15min) + refresh tokens (7d) with rotation.
- **Account lockout** -- 5 failed login attempts triggers a 15-minute lockout.
- **MCP auth via OAuth 2.1 (Ory Hydra)** -- the MCP server is an OAuth Resource Server; Hydra owns DCR, authorize/token, and refresh rotation. The API hosts the login/consent bridge and a hardened DCR proxy. See `docs/mcp-oauth.md`.

## Testing

```bash
make test          # Run all tests
make test-cover    # Tests with HTML coverage report
make test-race     # Tests with race detector
```

Three levels of tests:

- **Service tests** -- table-driven unit tests with in-memory SQLite (`internal/services/*_test.go`)
- **Handler tests** -- HTTP tests with mock services via interfaces (`internal/handlers/*_test.go`)
- **Integration tests** -- full workflow tests with real SQLite DB (`tests/integration/`)

## Database Migrations

Migrations live in `migrations/` as numbered SQL pairs:

```
000001_create_users.up.sql / .down.sql
000002_create_accounts.up.sql / .down.sql
...
000018_create_telegram_links.up.sql / .down.sql
...
000022_create_trusted_oauth_clients.up.sql / .down.sql
000023_drop_mcp_token_hash_from_users.up.sql / .down.sql
```

```bash
make migrate-up        # Apply all pending migrations
make migrate-down      # Roll back last migration
make migrate-version   # Check current version
```

In development, migrations run automatically on server start.

## Environment Variables

Configured via `apps/api/.env` (full production template: `.env.prod.example` at the repo root):

| Variable       | Description                          | Default       |
|----------------|--------------------------------------|---------------|
| `ENV`          | Environment                          | `development` |
| `PORT`         | Server port                          | `8080`        |
| `DB_HOST`      | PostgreSQL host                      | `localhost`   |
| `DB_PORT`      | PostgreSQL port (`5433` on host in local dev) | `5432` |
| `DB_USER`      | Database user                        | `kuberan`     |
| `DB_PASSWORD`  | Database password                    | `kuberan`     |
| `DB_NAME`      | Database name                        | `kuberan`     |
| `DB_SSLMODE`   | SSL mode                             | `disable`     |
| `JWT_SECRET`   | JWT signing key (required in prod)   | dev default   |
| `JWT_EXPIRES_IN` | Config token expiration (note: access-token lifetime is hardcoded to 15m in `internal/middleware/auth.go`) | `24h` |
| `PIPELINE_API_KEY` | API key for pipeline endpoints   | --            |
| `BOT_INTERNAL_SECRET` | Shared secret for internal telegram routes | -- |
| `CORS_ORIGIN`  | Allowed CORS origin                  | `*`           |
| `HYDRA_ISSUER_URL` | Hydra public URL (OAuth issuer)  | `http://localhost:4444` |
| `HYDRA_ADMIN_URL` | Hydra admin API URL               | `http://localhost:4445` |
| `MCP_RESOURCE_URL` | MCP resource server URL (token audience) | `http://localhost:8081` |
| `OAUTH_SCOPES` | Override of the default `read:*` OAuth scope set | built-in defaults |
| `HYDRA_PINNED_CLIENT_ID` | Optional pinned OAuth client   | --            |

The MCP server additionally reads `MCP_PORT` (default `8081`).

In production, `JWT_SECRET` must be explicitly set and `DB_PASSWORD` must not be the development default.
