# Kuberan MCP Server

A standalone [Model Context Protocol](https://modelcontextprotocol.io) server that
exposes a user's Kuberan financial data to AI agents over Streamable HTTP. It reuses
the API's service layer directly against the database — it does **not** call the REST API.

All tools are **read-only**: accounts, categories, transactions (and analytics),
budgets, the investment portfolio, and net-worth history.

## Running

```bash
cd apps/api
make build-mcp        # builds bin/mcp
MCP_PORT=8081 ./bin/mcp
```

`MCP_PORT` accepts either a bare port (`8081`) or a full listen address (`:8081`,
`0.0.0.0:8081`). It defaults to `8081`. Database configuration uses the same
`DB_*` environment variables as the API.

### Endpoints

| Path      | Auth        | Purpose                                  |
| --------- | ----------- | ---------------------------------------- |
| `/mcp`    | Bearer (OAuth access token) | Streamable HTTP MCP transport |
| `/.well-known/oauth-protected-resource` | none | RFC 9728 Protected Resource Metadata |
| `/health` | none        | Liveness probe, returns `{"status":"ok"}` |

## Authentication

The MCP server is an OAuth 2.1 **Resource Server**. [Ory Hydra](https://www.ory.sh/hydra)
is the Authorization Server; see `plans/015-mcp-oauth-hydra`.

1. An MCP client (e.g. a Claude connector) discovers the server via
   `/.well-known/oauth-protected-resource`, which points at Hydra as the
   `authorization_servers` entry.
2. The client registers (Dynamic Client Registration), then drives the OAuth
   authorization-code + PKCE flow. Login and consent resolve against Kuberan's
   existing user store via the `apps/web` login/consent pages and the
   `apps/api` admin bridge (`/api/v1/oauth/*`).
3. Hydra issues a short-lived (15m) JWT **access token** scoped to granular
   `read:*` scopes, plus a rotating refresh token for silent renewal.
4. The client sends the access token as `Authorization: Bearer <token>` to `/mcp`.
   On each request the server validates the JWT offline against Hydra's JWKS
   (signature, `exp`/`nbf`, issuer, audience) and enforces the required scope for
   the tool being called.

A `/mcp` request without a valid token receives `401` with a
`WWW-Authenticate: Bearer resource_metadata="…"` challenge pointing at the
discovery document.

## Deployment requirements

- **Hydra reachability.** The server validates tokens against Hydra's JWKS at
  `HYDRA_ISSUER_URL`, and advertises itself under `MCP_RESOURCE_URL` (the token
  audience). Both must be set to the public hostnames clients reach. The MCP
  server no longer needs `JWT_SECRET` — that only signs the API's own
  access/refresh/bot tokens.
- **Terminate TLS in front of the server.** Tokens are sent as plaintext bearer
  credentials and the server speaks plain HTTP. Run it behind a reverse proxy /
  load balancer that terminates TLS; do not expose `:8081` directly to clients.
  Keep Hydra's admin API (`:4445`) on the private network — never expose it.
