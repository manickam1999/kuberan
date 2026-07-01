# MCP Auth: Static Token → OAuth 2.1 via Ory Hydra

## Context

The Kuberan MCP server (`apps/api/cmd/mcp`, `apps/api/internal/mcp/`) currently authenticates
callers with a hand-issued, long-lived bearer token:

- A per-user HS256 JWT (`token_type: "mcp"`, 1-year expiry) minted by `POST /auth/mcp-token`
  (`internal/handlers/auth_handler.go:260`, `internal/middleware/auth.go:96`).
- Its SHA-256 hash is stored on the user row (`users.mcp_token_hash`, `internal/models/user.go:14`)
  and compared on every request to support revocation.
- Validated per-request in `internal/mcp/server.go:54` (`makeAuthFromRequest`): parse + signature +
  `token_type` check, hash match against DB, `IsActive` check, then `userID` is put in context.
- The MCP server is DB-direct (its own connection, calls the same service layer). There is no
  outbound HTTP hop to the REST API.

This works but is limiting:

1. **Manual issuance & refresh** - the user copies a token string into the client config; rotating
   it is a manual redo. There is no refresh mechanism.
2. **Not the flow connectors expect** - Claude connectors (and MCP clients generally) drive an
   **OAuth 2.1** authorization flow with **Dynamic Client Registration (DCR, RFC 7591)** so a
   connector can be added by URL with no manual token.

## Goal

Replace the static token with a standards-compliant OAuth 2.1 flow, **without authoring any
security-critical token/crypto code ourselves**:

- Add the connector by URL; no copy-pasting a token.
- Access tokens refresh silently (refresh-token rotation). No 1-year manual rotation.
- Standard discovery (RFC 9728 Protected Resource Metadata) and per-client revocation.

## Decision: Ory Hydra as the Authorization Server

The MCP server becomes an OAuth **Resource Server (RS)**. **Ory Hydra** is the **Authorization
Server (AS)** - it owns DCR, the authorize/token endpoints, and refresh rotation. Kuberan's existing
`UserService` answers "who is this user" via Hydra's **login & consent** flow, so Hydra manages no
users and we do not need Ory Kratos.

Why Hydra over the alternatives:

| Criterion | Roll our own AS | Zitadel | **Ory Hydra** |
|---|---|---|---|
| DCR (RFC 7591) - MCP hard requirement | Build it | **Not supported** (open feature request zitadel/zitadel#9810) | **Built-in** (ory/hydra#1616) |
| Security-critical crypto authored by us | All of it | None | **None** |
| User store / login UI | Have it | Included (unused) | **Reuse `UserService`** |
| Net | "Asking for trouble" | Needs a DCR proxy shim anyway | **Does the hard part; BYO-users fits us** |

## Locked decisions (from design review)

| # | Decision | Choice | Notes |
|---|---|---|---|
| Q1 | Access-token format | **JWT + local JWKS** | RS validates offline against Hydra JWKS; revocation bounded by short TTL. |
| Q2 | Database hosting | **Hydra on existing Postgres, own schema** | Dev: the compose Postgres. Prod: Supabase. No data migration; managed backups keep working. |
| Q3 | Login/consent host | **`apps/web` (Next.js) for UI** | Admin-API interaction lives in `apps/api` (keeps Hydra admin private). |
| Q4 | Consent | **Auto-accept, via trust-on-first-use** | Auto-accept trusted clients; show consent for unknown ones (reconciles Q4 with the DCR hardening below). |
| Q5 | Migration | **Hard cutover** | No dual-validate window. Deploy additive phases, verify, then delete the legacy path. |
| Q6 | Scopes | **Granular `read:*`** | Per-domain scopes; RS enforces the required scope per tool. |

> Adding Hydra does **not** justify a database migration. Hydra is just another Postgres client;
> it points at the existing instance with its own schema. "Self-host the DB" is a separate project
> (needs backups + offsite replication + restore drills) and should not be bundled here.

---

## Target Architecture

### Authorization flow (first connect)

```
Claude (MCP client)         Kuberan MCP /mcp (RS)      Ory Hydra (AS)        Kuberan login+consent
      | GET /mcp (no token)        |                        |                        |
      |--------------------------->| 401 + WWW-Authenticate |                        |
      |                            |   resource_metadata    |                        |
      | GET /.well-known/oauth-protected-resource           |                        |
      |--------------------------->| { authorization_servers:[Hydra] }               |
      | POST /oauth2/register (DCR) ------------------------>| client_id (+secret)    |
      | GET /oauth2/auth (code + PKCE) -------------------->| redirect login_challenge|
      |                            |                        |----------------------->|
      |                            |                        |   user signs in        |
      |                            |                        |   (bcrypt/UserService) |
      |                            |                        |<--accept login (sub=uid)|
      |                            |                        |--redirect consent------>|
      |                            |                        |<--accept consent (scope)|
      | <-- authorization code ------------------------------|                        |
      | POST /oauth2/token (code + verifier) --------------->| access + refresh (JWT) |
      | GET/POST /mcp (Bearer access) ------------------------>| validate via JWKS     |
      | <-- tool results ----------|                        |                        |
```

### Deployment topology

- `apps/api` REST (`:8080`), `apps/api` MCP RS (`:8081`), `apps/web` (`:3000`), **Ory Hydra**
  (public `:4444`, admin `:4445`), all in `docker-compose`.
- One Postgres (compose in dev, Supabase in prod). Hydra uses its own schema/DB.
- **Cloudflare Tunnel (cloudflared)** exposes only the public surface over HTTPS:
  Hydra public (`:4444`), `/mcp`, and the login/consent pages. **Hydra admin (`:4445`) is never
  exposed** - it stays on the private Docker network.
- The DB is never public (Supabase or local), independent of the connector story.

### Component inventory

**New:**
- `oryd/hydra` container + Hydra config (`ory/hydra/hydra.yml`) and a migration/init step.
- `apps/api/internal/hydra/` - thin Hydra **admin** client (login/consent challenge accept/reject,
  client CRUD, introspection if needed).
- `apps/api/internal/mcp/discovery.go` - RFC 9728 Protected Resource Metadata + `WWW-Authenticate`.
- `apps/api/internal/mcp/validator.go` - JWKS-based access-token validator (`HydraValidator`).
- `apps/api/internal/handlers/oauth_handler.go` - login/consent **admin bridge** endpoints called by
  the `apps/web` pages (keeps admin API access server-side in Go).
- `apps/web` login + consent pages wired into the Hydra flow.
- `trusted_oauth_clients` table (trust-on-first-use) + migration.
- (Phase 5) DCR registration **proxy/policy** + Cloudflare rate-limit + new-client alerting.

**Changed:**
- `internal/mcp/server.go` - `makeAuthFromRequest` swaps DB hash-check for `HydraValidator`; wire the
  discovery endpoints and per-tool scope enforcement.
- Every tool handler (`tools_*.go`) - enforce a required scope in addition to `getUserID`.
- `docker-compose.yml` - add `hydra` (+ `hydra-migrate`), env for issuer/admin URLs, scopes, pinned
  resource URL; MCP service no longer needs `JWT_SECRET` for MCP validation.
- `internal/config` - add Hydra/OAuth config keys.

**Deleted (Phase 4, hard cutover):**
- `middleware.GenerateMCPToken`, `middleware.ValidateMCPToken` (+ the `"mcp"` branch usage).
- `UserService.StoreMCPTokenHash` / `GetMCPTokenHash`; `users.mcp_token_hash` column (migration).
- Routes `POST/DELETE /auth/mcp-token` (`cmd/api/main.go:165-166`) and their handlers.

---

## Phase 0 - Stand up Hydra (additive, no app code)

**Goal:** Hydra running against the existing Postgres, issuing JWT access tokens, reachable only where
intended.

1. **Database.** Create a dedicated logical DB/schema for Hydra.
   - Dev: extend `apps/api/dev/init.sql` to `CREATE DATABASE hydra;` (compose Postgres).
   - Prod (Supabase): create a `hydra` database (or a dedicated schema + role) and a connection URL.
2. **`docker-compose.yml`** - add:
   - `hydra-migrate` (one-shot: `hydra migrate sql -e --yes`, `DSN` → the Hydra DB).
   - `hydra` (`oryd/hydra:v2.x`) serving `:4444` (public) and `:4445` (admin), `depends_on`
     `hydra-migrate`. **Do not publish `:4445` beyond the Docker network.**
3. **`ory/hydra/hydra.yml`** (or env) - configure:
   - `dsn` → Hydra DB.
   - `urls.self.issuer` → `https://auth.<domain>` (must match the public hostname cloudflared serves).
   - `urls.login`, `urls.consent` → the `apps/web` pages (Phase 1).
   - `strategies.access_token: jwt` (Q1 - RS validates via JWKS).
   - `ttl.access_token: 15m`, `ttl.refresh_token: 720h` (rotation on), `ttl.id_token` as needed.
   - `oidc.subject_identifiers` default; `secrets.system` set from env.
   - `oauth2.expose_internal_errors: false`.
   - Register the API scopes: `read:accounts read:transactions read:budgets read:categories
     read:investments read:portfolio read:snapshots` (Q6).
4. **`internal/config`** - add `HydraIssuerURL`, `HydraAdminURL`, `MCPResourceURL`,
   `OAuthScopes`, `HydraPinnedClientID` (optional), with dev defaults.

**Verify:** `hydra` health is green; `GET https://auth.<domain>/.well-known/openid-configuration`
and `/.well-known/jwks.json` resolve through cloudflared; `:4445` is unreachable publicly.

> **Confirm against the pinned Hydra version's docs**: exact config keys for access-token strategy,
> scope registration, and DCR settings differ slightly across Hydra 2.x minors.

## Phase 1 - Login + consent (UI in `apps/web`, admin bridge in `apps/api`)

**Goal:** Hydra's login/consent challenges resolve against the existing user store, with the Hydra
admin API accessed only server-side.

1. **`internal/hydra` admin client** - typed wrapper over the Hydra admin REST API (or the official
   `github.com/ory/hydra-client-go/v2` SDK):
   - `GetLoginRequest(challenge)`, `AcceptLogin(challenge, subject, remember)`, `RejectLogin`.
   - `GetConsentRequest(challenge)`, `AcceptConsent(challenge, grantScope, grantAudience, remember)`,
     `RejectConsent`.
   - `GetClient(id)` (for consent display / trust checks).
   Base URL = `HydraAdminURL` (private network only).
2. **`oauth_handler.go` (apps/api)** - browser-callable endpoints that drive the admin client:
   - `POST /api/v1/oauth/login` `{login_challenge, email, password}` → verify via `UserService`
     (reuse the existing login/lockout path), on success `AcceptLogin(subject=user.ID, remember)`,
     return `{redirect_to}`. On failure return the same errors the login form already handles.
   - `GET  /api/v1/oauth/consent?consent_challenge=…` → fetch the consent request; look up
     `client_id` in `trusted_oauth_clients`:
       - **trusted** → `AcceptConsent(grantScope=requested ∩ allowed)` server-side, return
         `{redirect_to}` (page redirects immediately, no UI).
       - **unknown** → return `{client, requested_scopes, redirect_uri}` for the page to render.
   - `POST /api/v1/oauth/consent/accept` `{consent_challenge, remember_client}` → `AcceptConsent`;
     if `remember_client`, insert into `trusted_oauth_clients` (+ audit log). Return `{redirect_to}`.
   - Enforce that granted scopes are a subset of the registered `read:*` set (defense in depth).
3. **`apps/web` pages:**
   - `/oauth/login?login_challenge=…` - render the existing sign-in form; submit to
     `POST /oauth/login`; redirect to `redirect_to`.
   - `/oauth/consent?consent_challenge=…` - call `GET /oauth/consent`; if it returns `redirect_to`,
     redirect; else render a consent screen showing **client name + redirect_uri host + scopes** and a
     "remember this client" checkbox; submit to `POST /oauth/consent/accept`.
4. **`trusted_oauth_clients`** migration (`0000NN_create_trusted_oauth_clients`): `id`, `client_id`
   (unique), `name`, `created_at`. Model + minimal service.

**Verify:** using Hydra's example/dev client, complete `authorize → login → consent → token` by hand
(`hydra perform authorization-code` or curl). First run shows consent; "remember" makes the second
run auto-accept. Confirm the browser never talks to `:4445` directly.

## Phase 2 - RS validation (JWKS) + discovery

**Goal:** `/mcp` accepts Hydra-issued JWT access tokens and advertises discovery; per-tool scopes
enforced.

1. **`internal/mcp/validator.go`** - `HydraValidator`:
   - Fetch + cache Hydra JWKS (`github.com/MicahParks/keyfunc/v3`, background refresh on rotation).
   - `Validate(ctx, raw) (*AccessClaims, error)`: verify RS256 signature, `exp`/`nbf`, `iss` ==
     issuer, `aud` contains `MCPResourceURL`, and parse `scope` (space-delimited) + `sub`.
2. **`server.go` - `makeAuthFromRequest`** - replace the mint/hash/user-lookup body with:
   `Validate` → on success put `sub` (user ID) **and the scope set** in context; on failure return the
   bare context (tools still reject via `getUserID`). See the diff in the appendix.
3. **Per-tool scope enforcement** - add `requireScope(ctx, "read:accounts")` used by each handler
   after `getUserID`; map each tool to its scope:
   | Tool | Scope |
   |---|---|
   | `list_accounts` | `read:accounts` |
   | `list_transactions`, `get_spending_by_category`, `get_monthly_summary`, `get_daily_spending` | `read:transactions` |
   | `list_budgets`, `get_budget_progress` | `read:budgets` |
   | `list_categories` | `read:categories` |
   | `get_portfolio` | `read:investments` / `read:portfolio` |
   | `get_net_worth_history` | `read:snapshots` |
4. **Discovery - `internal/mcp/discovery.go`** - register on the MCP mux:
   - `GET /.well-known/oauth-protected-resource` → `{ resource, authorization_servers:[issuer],
     scopes_supported, bearer_methods_supported:["header"] }`.
   - Make 401 responses from `/mcp` include
     `WWW-Authenticate: Bearer resource_metadata="https://mcp.<domain>/.well-known/oauth-protected-resource"`.

**Tradeoff to note (from review):** dropping the per-request user lookup means `IsActive` is no
longer checked on every call. Mitigation: short access-token TTL (Phase 0) + enforce `IsActive` at
`AcceptLogin` time (Phase 1). If instant deactivation is ever required, switch that one check to Hydra
token introspection (Q1 revisited) - out of scope here.

**Verify:** a token minted in Phase 1 calls `/mcp` tools successfully; a token missing the required
scope is rejected by the specific tool; discovery doc + `WWW-Authenticate` header are correct.

## Phase 3 - DCR + connect Claude end-to-end

**Goal:** the real Claude connector completes the whole loop, including silent refresh.

1. **Enable Hydra public DCR** (`/oauth2/register`) so clients self-register (hardened in Phase 5).
2. **Add the connector by URL** in Claude pointing at `https://mcp.<domain>` (through cloudflared).
   Claude discovers PRM → Hydra AS metadata → registers → authorizes → obtains tokens.
3. **First-connect consent** shows the Claude client (Phase 1 TOFU); approve + remember → recorded in
   `trusted_oauth_clients` and treated as the pinned client thereafter.
4. Exercise tool calls; let the access token expire and confirm **silent refresh** works.

**Verify (E2E, per CLAUDE.md):** drive it exactly as an end user would - add the connector in Claude,
authorize, run several tools, wait out the access TTL, confirm continued access via refresh.

## Phase 4 - Hard cutover: delete the legacy token path

**Goal:** remove the static-token machinery once OAuth is proven (Q5).

1. Delete `POST/DELETE /auth/mcp-token` routes (`cmd/api/main.go:165-166`) and
   `AuthHandler.GenerateMCPToken` / `RevokeMCPToken`.
2. Delete `middleware.GenerateMCPToken`, `middleware.ValidateMCPToken`, and the `mcp` handling in
   `validateToken`. Leave `GenerateBotToken`/`bot` untouched (pipeline). `AuthMiddleware` already
   rejects non-`access`/`bot` types, so it needs no change.
3. Delete `UserService.StoreMCPTokenHash` / `GetMCPTokenHash`; remove `MCPTokenHash` from the model.
4. Migration `0000NN_drop_mcp_token_hash_from_users` (down re-adds the column - keeps rollback safe).
5. `docker-compose.yml` - drop `JWT_SECRET` from the `mcp` service (no longer validates our JWT);
   confirm `config.Load` in the MCP binary doesn't hard-require it.
6. Update Swagger + any docs that reference the MCP token endpoints.

**Verify:** `./scripts/check-go.sh apps/api` clean; MCP works only via OAuth; the removed endpoints
404; down-migration restores the column.

## Phase 5 - Harden the open DCR endpoint + ops

**Goal:** make public DCR safe on a self-hosted, Cloudflare-fronted, single-user instance. The real
risk is a rogue client with an attacker-controlled `redirect_uri`; registration alone is harmless
until a user authorizes it.

1. **[MUST] Keep Hydra admin (`:4445`) private** - cloudflared maps only public paths (Hydra `:4444`,
   `/mcp`, `/oauth/*` pages). Re-verify after any tunnel change.
2. **Constrain DCR-registered clients** - via Hydra DCR policy and/or a **registration proxy**
   (recommended): serve `registration_endpoint` through a small `apps/api` handler (or Cloudflare
   Worker) that forces public clients, mandatory PKCE (S256), grant types limited to
   `authorization_code` + `refresh_token`, caps requested scopes to the `read:*` set, then calls
   Hydra admin to create the client. This also gives us the audit hook in step 4.
3. **Consent as the anti-phishing tripwire (Q4 TOFU)** - already built in Phase 1: unknown clients get
   a consent screen showing the `redirect_uri` host; only remembered clients auto-accept.
4. **Alert on every new client registration** - the registration proxy logs each DCR to `audit_logs`
   and fires a notification (reuse Zap + existing audit infra). On a single-user instance, "a new
   OAuth client registered" is a near-perfect intrusion signal. (If not using the proxy, run a
   periodic reconciler that lists Hydra clients via admin and diffs against `trusted_oauth_clients`.)
5. **Cloudflare WAF rate-limit + Bot Fight Mode** on `/oauth2/register` (DoS / mass-registration).
   IP allowlisting is not viable - Anthropic's servers perform DCR for claude.ai connectors and their
   egress isn't a stable published range.
6. **Token audience** - confirm Hydra stamps `aud = MCPResourceURL`; RS rejects other audiences.
7. **[Verify] Can we skip public DCR entirely?** - if any client we actually use accepts a
   pre-registered `client_id`, disable public DCR and the risk class disappears. Likely unavailable
   for claude.ai custom connectors; confirm per client (Claude Desktop/Code may differ).

**Verify:** `:4445` unreachable externally; a DCR with a disallowed scope/grant is rejected or
down-scoped; a new registration produces an audit entry + alert; rate-limit triggers under a burst.

---

## Data model & migrations

- `0000NN_create_trusted_oauth_clients.up.sql` (Phase 1): `trusted_oauth_clients(id, client_id
  UNIQUE, name, created_at)`. Down drops the table.
- `0000NN_drop_mcp_token_hash_from_users.up.sql` (Phase 4): `ALTER TABLE users DROP COLUMN
  mcp_token_hash;`. Down re-adds `mcp_token_hash VARCHAR(64)`.
- Hydra owns its own schema/tables via `hydra migrate sql` - **not** managed by golang-migrate.

## New dependencies

- Container: `oryd/hydra:v2.x` (pin the exact minor).
- Go: `github.com/MicahParks/keyfunc/v3` (JWKS cache). Hydra admin: official
  `github.com/ory/hydra-client-go/v2` **or** a thin REST client (prefer the SDK for the admin surface).
- Reuse `github.com/golang-jwt/jwt/v5` (already present) for access-token parsing.

## Configuration (new env)

`apps/api` (REST + MCP):
- `HYDRA_ISSUER_URL` (public, e.g. `https://auth.<domain>`)
- `HYDRA_ADMIN_URL` (private, e.g. `http://hydra:4445`)
- `MCP_RESOURCE_URL` (e.g. `https://mcp.<domain>`)
- `OAUTH_SCOPES` (space-delimited `read:*` set)
- `HYDRA_PINNED_CLIENT_ID` (optional, if not using TOFU)

Hydra: `DSN`, `URLS_SELF_ISSUER`, `URLS_LOGIN`, `URLS_CONSENT`, `STRATEGIES_ACCESS_TOKEN=jwt`,
`SECRETS_SYSTEM`, TTLs, registered scopes.

## Testing strategy

- **Unit (`apps/api`):**
  - `HydraValidator` - generate an RSA key in-test, serve a JWKS via `httptest`, sign access tokens;
    assert accept/reject on signature, `exp`, `iss`, `aud`, and scope.
  - Per-tool `requireScope` - table-driven: correct scope passes, missing scope returns the MCP error.
  - `oauth_handler` login/consent - mock the Hydra admin client (interface); assert `AcceptLogin`
    subject = user ID, TOFU behavior (unknown → consent payload; trusted → `redirect_to`), scope
    subset enforcement, and audit logging on remember.
  - Registration proxy policy (Phase 5) - reject/down-scope disallowed grants/scopes; audit on create.
- **Integration (`tests/integration`):** the login/consent bridge against a mocked Hydra admin
  (`httptest`); RS validation with a locally-signed JWKS. Full Hydra-in-Docker is optional and heavier
  - keep it manual/E2E.
- **E2E (manual, per CLAUDE.md):** add the connector in Claude through cloudflared and drive the real
  flow, including refresh, exactly as an end user would.
- **Gate:** `./scripts/check-go.sh apps/api` (build → vet → lint → test → test -race) must pass after
  each phase; `make check-fast` after each file change.

## Rollout & rollback

- Phases 0-3 are **strictly additive** - the legacy token path keeps working untouched. Deploy them,
  verify OAuth end-to-end via Claude, and only then do Phase 4.
- **Before Phase 4:** rollback = don't cut over (no data migration to reverse).
- **After Phase 4 (hard cutover):** rollback = redeploy the prior image + run the down-migration to
  re-add `mcp_token_hash`, and re-issue a legacy token. Because it's single-user, the cutover window
  (re-adding the connector in Claude) is small and acceptable.

## Open items to confirm during implementation

- Exact Hydra 2.x config keys for access-token strategy, scope registration, and DCR policy.
- Whether any Claude client we use supports a pre-registered `client_id` (would let us disable public
  DCR - the strongest hardening).
- Whether to implement the DCR **registration proxy** in `apps/api` vs a Cloudflare Worker (both keep
  Hydra admin private and give the audit/alert hook; pick per ops preference).
- Supabase specifics for the Hydra DB (separate database vs schema + role) and its connection string
  from the compose network in production.

## Appendix - core RS diff

`internal/mcp/server.go` (`makeAuthFromRequest`), before → after:

```go
// BEFORE: mint our own HS256 token, compare SHA-256 hash in DB, load user.
claims, err := middleware.ValidateMCPToken(tokenString)
if err != nil { return ctx }
storedHash, err := userService.GetMCPTokenHash(claims.UserID)
if err != nil || storedHash == "" || storedHash != middleware.HashToken(tokenString) { return ctx }
user, err := userService.GetUserByID(claims.UserID)
if err != nil || !user.IsActive { return ctx }
return context.WithValue(ctx, userIDKey{}, claims.UserID)

// AFTER: validate the Hydra-issued JWT access token against cached JWKS.
claims, err := v.Validate(ctx, tokenString) // sig, exp, iss, aud==MCP resource, scopes
if err != nil {
    logger.Get().Warnf("MCP auth: token rejected: %v", err)
    return ctx
}
ctx = context.WithValue(ctx, userIDKey{}, claims.Subject) // sub == Kuberan user ID
return context.WithValue(ctx, scopesKey{}, claims.Scopes)
```

`internal/mcp/discovery.go` (new): RFC 9728 Protected Resource Metadata handler advertising the Hydra
issuer as the `authorization_servers` entry, plus the `WWW-Authenticate` header on 401s.
