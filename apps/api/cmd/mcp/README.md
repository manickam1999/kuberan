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
| `/mcp`    | Bearer (MCP token) | Streamable HTTP MCP transport     |
| `/health` | none        | Liveness probe, returns `{"status":"ok"}` |

## Authentication

1. A logged-in user calls `POST /api/v1/auth/mcp-token` on the **API** to mint a
   long-lived (1 year) MCP token. The token's SHA-256 hash is stored on the user
   record so it can be validated and revoked.
2. The MCP client sends that token as `Authorization: Bearer <token>` to `/mcp`.
3. On each request the MCP server validates the JWT, confirms its type is `mcp`,
   checks the stored hash matches (supports revocation), and that the user is active.
4. `DELETE /api/v1/auth/mcp-token` revokes the token by clearing the stored hash.

MCP tokens are rejected by the main REST API (`AuthMiddleware`), and access/bot
tokens are rejected by the MCP server — token types cannot be used cross-purpose.

> **One token per user.** Only the most recently issued MCP token is valid;
> generating a new one invalidates the previous token.

## Deployment requirements

- **Shared `JWT_SECRET`.** The MCP server validates tokens with the same
  `JWT_SECRET` the API signs them with. The API and MCP processes **must** be
  configured with an identical `JWT_SECRET`, or every token will be rejected.
  In production this must be a strong, explicitly-set value on both services
  (the dev fallback is refused in production by config validation).
- **Terminate TLS in front of the server.** Tokens are sent as plaintext bearer
  credentials and the server speaks plain HTTP. Run it behind a reverse proxy /
  load balancer that terminates TLS; do not expose `:8081` directly to clients.
