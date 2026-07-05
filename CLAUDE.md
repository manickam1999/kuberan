# CLAUDE.md

## Project Overview

Kuberan is a personal finance application that helps users manage their finances, create budgets, track investments, and make better financial decisions. Named after the Hindu god of wealth (Kubera), it is a self-hosted, privacy-first finance tracker.

## Repository Structure

Monorepo with two main applications:

```
/
├── apps/
│   ├── api/          # Go backend (Gin + GORM + PostgreSQL)
│   └── web/          # Next.js frontend (React 19 + Tailwind CSS v4)
├── packages/         # Shared code between apps
├── plans/            # Architecture and upgrade plans
├── scripts/          # Shared scripts (check-go.sh, etc.)
├── tools/            # Development tools
└── docker-compose.yml
```

## Technology Stack

### Backend (`apps/api/`)
- **Language**: Go 1.24
- **Framework**: Gin (HTTP router)
- **ORM**: GORM with PostgreSQL 16
- **Auth**: JWT (golang-jwt/jwt/v5) with bcrypt password hashing
- **Logging**: Zap (structured logging)
- **Migrations**: golang-migrate (SQL-based, version-controlled)
- **API Docs**: Swagger/OpenAPI via swaggo
- **Testing**: Go standard `testing` package with SQLite in-memory for tests

### Frontend (`apps/web/`)
- **Framework**: Next.js 15 (App Router)
- **Language**: TypeScript (strict mode, never use `any`)
- **React**: 19
- **Styling**: Tailwind CSS v4
- **Components**: ShadCN UI (`new-york` style, built on Radix UI + Lucide icons)
- **Data Fetching**: @tanstack/react-query v5 (query key factories, smart cache invalidation)
- **Forms**: react-hook-form + zod schema validation
- **Charts**: Recharts (pie, bar, area charts on dashboard)
- **Theming**: next-themes (light/dark/system)
- **Notifications**: Sonner (toast notifications)
- **Auth**: JWT token management with auto-refresh, cookie-based route protection via Next.js middleware
- **API Layer**: Typed HTTP client (`lib/api-client.ts`) consumed by React Query hooks in `src/hooks/`
- **Package Manager**: pnpm

### Infrastructure
- **Database**: PostgreSQL 16 (Alpine)
- **Dev Environment**: Docker Compose
- **Hot Reload**: Air (Go), Turbopack (Next.js)

## Backend Architecture (`apps/api/`)

```
apps/api/
├── cmd/
│   ├── api/main.go           # Application entrypoint
│   └── migrate/main.go       # Migration CLI tool
├── migrations/               # SQL migration files (golang-migrate)
├── Makefile                  # Dev targets (build, test, lint, migrate, etc.)
├── internal/
│   ├── config/               # Environment-based configuration
│   ├── database/             # DB connection, pooling, config
│   ├── errors/               # Custom AppError types with codes
│   ├── handlers/             # HTTP handlers (thin, delegate to services)
│   ├── logger/               # Zap logger setup
│   ├── middleware/            # Auth, error handling, request logging
│   ├── models/               # GORM models (single source of truth)
│   ├── pagination/           # Pagination utilities
│   ├── services/             # Business logic layer (interface-based)
│   ├── validator/            # Custom Gin validators
│   ├── testutil/             # Test helpers (DB setup, fixtures)
│   └── docs/                 # Generated Swagger docs
└── tests/
    └── integration/          # End-to-end workflow tests
```

### Architecture Patterns
- **3-layer architecture**: Handlers -> Services -> Models
- **Interface-based services**: All services define interfaces for testability
- **Custom error types**: `AppError` with error codes, HTTP status, and internal error wrapping
- **Dependency injection**: Services injected into handlers via constructors

## Frontend Architecture (`apps/web/`)

```
apps/web/src/
├── app/
│   ├── (auth)/                   # Auth route group (login, register)
│   │   └── layout.tsx            # Centered card layout, redirects authenticated users
│   ├── (dashboard)/              # Dashboard route group (all protected pages)
│   │   ├── layout.tsx            # Sidebar + header layout, auth guard
│   │   ├── page.tsx              # Dashboard home
│   │   ├── accounts/             # Accounts list + [id] detail
│   │   ├── transactions/         # Cross-account transactions
│   │   ├── categories/           # Category management
│   │   ├── budgets/              # Budget cards with progress
│   │   ├── investments/          # Portfolio overview + [id] detail
│   │   └── securities/           # Securities browse + [id] detail
│   └── layout.tsx                # Root layout (providers chain)
├── components/
│   ├── ui/                       # ShadCN UI primitives (24 components)
│   ├── layout/                   # App sidebar, header
│   ├── accounts/                 # Account dialogs (create, edit)
│   ├── transactions/             # Transaction dialogs (create, edit)
│   ├── categories/               # Category dialogs (create, edit, delete)
│   ├── budgets/                  # Budget dialogs (create, edit, delete)
│   ├── investments/              # Investment action dialogs (add, buy, sell, dividend, split)
│   └── dashboard/                # Dashboard charts (expenditure, income/expenses, spending trend)
├── hooks/                        # React Query hooks (one per domain, with query key factories)
├── providers/                    # ThemeProvider, QueryProvider, AuthProvider
├── lib/
│   ├── api-client.ts             # HTTP client with auto token refresh
│   ├── auth.ts                   # JWT parsing, token storage, auth cookies
│   ├── format.ts                 # Currency (cents->display), date, percentage formatters
│   └── utils.ts                  # cn() Tailwind utility
├── types/
│   ├── models.ts                 # Domain model types matching backend
│   └── api.ts                    # API request/response DTOs
└── middleware.ts                  # Next.js middleware for cookie-based route protection
```

### Frontend Patterns
- **Route groups**: `(auth)` for public pages, `(dashboard)` for protected pages with sidebar layout
- **Provider chain**: Root layout wraps `ThemeProvider` > `QueryProvider` > `AuthProvider` > `Toaster`
- **Data fetching**: All API calls go through `lib/api-client.ts`, consumed by React Query hooks in `src/hooks/`. Each hook file exports a query key factory for structured cache management
- **Auth flow**: JWT access tokens (localStorage) + auth flag cookie (for middleware). Auto-refresh on 401 with concurrent request deduplication
- **Component organization**: ShadCN primitives in `ui/`, feature-specific dialogs in domain folders (`accounts/`, `transactions/`, etc.)
- **Forms**: react-hook-form with zod schemas for validation, ShadCN Form components for UI

## Data Model & Conventions

### Account Types
Four account types are supported: `cash`, `investment`, `credit_card`, and `debt`. Each type has specific fields:
- **Cash**: Basic accounts with balance tracking
- **Investment**: Adds `Broker`, `AccountNumber`, and related `Investments` (holdings)
- **Credit Card / Debt**: Adds `InterestRate`, `DueDate`, `CreditLimit`. Balance semantics are inverted — expenses increase the balance (debt goes up), payments decrease it. Credit card balances count toward `DebtBalance` in portfolio snapshots and are subtracted from net worth.

### Monetary Values
All monetary values are stored and transmitted as **int64 cents** (not float64). `$10.50` = `1050`. This eliminates floating-point rounding errors. The frontend is responsible for display formatting.

Non-monetary floats that remain as float64: `Investment.Quantity`, `SplitRatio`, `InterestRate`, `YieldToMaturity`, `CouponRate`. `CreditLimit` is int64 cents (not a float).

### Database Migrations
- Managed by golang-migrate, NOT GORM AutoMigrate
- Files in `apps/api/migrations/` as numbered SQL pairs (`NNNNNN_description.up.sql` / `.down.sql`)
- Run via CLI: `go run cmd/migrate/main.go up|down|version`
- In development, migrations run automatically on server start

### Error Handling
- Services return `*AppError` (defined in `internal/errors/`)
- Each error has a code (e.g., `ACCOUNT_NOT_FOUND`), message, and HTTP status
- Error middleware converts AppErrors to consistent JSON responses
- Internal/unexpected errors are logged but never exposed to clients

### Authentication
- JWT access tokens (short-lived, 15min) + refresh tokens (7d)
- Refresh token hash stored in user record
- Account lockout after 5 failed login attempts (15min cooldown)
- `LastLoginAt` tracking

### Logging
- Zap structured logger (JSON in production, console in development)
- Request logging middleware with request IDs
- All log calls use Zap, never `log.Println` or `fmt.Printf`

## Key Design Decisions

1. **Cents not floats**: All money as int64 cents for precision
2. **Soft deletes**: All models use GORM soft deletes. Deleted categories remain as references for existing transactions
3. **User-scoped queries**: Every data query includes `user_id` check for data isolation
4. **Atomic operations**: All balance-affecting operations wrapped in DB transactions
5. **Audit logging**: Sensitive operations logged to `audit_logs` table
6. **SQL migrations over AutoMigrate**: Version-controlled, reversible schema changes
7. **MCP auth via OAuth 2.1 (Ory Hydra)**: The MCP server is an OAuth Resource Server; Hydra owns DCR, authorize/token, and refresh rotation, reusing `UserService` for login. Flow, rationale, and interop gotchas: `docs/mcp-oauth.md`

## Common Commands

```bash
# Development (all services via Docker)
npm run dev

# Backend only
cd apps/api && air

# Frontend only
cd apps/web && pnpm dev

# Run backend tests
cd apps/api && go test ./... -v

# Run tests with coverage
cd apps/api && go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out

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

## Verification & Feedback Loop

**Primary check command** (run after any code change):
```bash
./scripts/check-go.sh apps/api
./scripts/check-go.sh apps/oracle
```

This runs, in order: `go build` -> `go vet` -> `golangci-lint` -> `go test` -> `go test -race`. All must pass.

**Quick check** (compile + lint only, no tests):
```bash
cd apps/api && make check-fast
```

### Agentic Development Protocol

When using AI coding agents (Claude Code, Cursor, Copilot, etc.) to modify this codebase:

1. **After each file change**: Run `go build ./...` from `apps/api/` to catch compile errors immediately.
2. **After completing a logical unit of work**: Run `make check-fast` (build + vet + lint).
3. **After completing a feature or phase**: Run `./scripts/check-go.sh apps/<app>` (full verification).
4. **Never skip tests**: If tests exist, they must pass before moving on.
5. **Never suppress lint errors**: Fix them properly. No `//nolint` without justification comments.
6. **Fix errors in order**: Compilation first, then vet, then lint, then tests. If step 1 fails, do not proceed to step 2.

## API Endpoints

### Public
```
POST /api/v1/auth/register     # Register new user
POST /api/v1/auth/login        # Login, returns access + refresh tokens
POST /api/v1/auth/refresh      # Refresh access token
GET  /api/health               # Health check (includes DB ping)
GET  /swagger/*                # Swagger UI
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
```

### Pipeline (require API key via X-API-Key header)
```
POST   /api/v1/pipeline/securities          # Create security
POST   /api/v1/pipeline/securities/prices   # Record security prices
POST   /api/v1/pipeline/snapshots           # Compute portfolio snapshots for all users
```

## Testing Strategy

- **Service tests**: Table-driven Go tests with in-memory SQLite
- **Handler tests**: BDD-style tests using `httptest` with mock services (via interfaces)
- **Integration tests**: Full workflow tests with real SQLite DB in `tests/integration/`
- **Coverage target**: 80% overall, 95% on auth/transactions/balance logic

## Code Style

### Go Backend
- Group related functionality into focused packages
- Use interfaces for service contracts
- Functional options pattern for functions with many optional parameters
- Always validate and sanitize user input
- Wrap errors with context using `fmt.Errorf("...: %w", err)` or `AppError.Wrap()`
- All exported functions must have godoc comments

### Frontend
- Strict TypeScript, never `any`
- Server components by default, client components only when interactivity is needed
- Container/presentational component separation
- Minimal components, prefer ShadCN when available
- Atomic design: atoms, molecules, organisms, templates, pages

## Environment Variables

### Backend (`apps/api/.env`)
```
ENV=development|staging|production
PORT=8080
DB_HOST=localhost
DB_PORT=5433
DB_USER=kuberan
DB_PASSWORD=kuberan
DB_NAME=kuberan
DB_SSLMODE=disable
JWT_SECRET=<required in production>
JWT_EXPIRES_IN=15m
```

In production, `JWT_SECRET` must be explicitly set (not the default) and `DB_PASSWORD` must not be the development default.
