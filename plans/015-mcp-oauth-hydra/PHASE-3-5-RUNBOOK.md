# Plan 015 - Phase 3 & Phase 5 Operator Runbook

All of plan 015's **code** is implemented and unit/integration-tested (see
`.gnhf/runs/implement-plan-15-c2c777/notes.md`). The only work that cannot be
performed or verified in an offline CI environment is the live end-to-end
connect (**Phase 3**) and the Cloudflare-dashboard hardening (**Phase 5** steps
1, 5, 7). This runbook is the actionable checklist for an operator with the live
stack (Docker + Hydra + cloudflared + a real Claude connector).

It also documents the two interop failure modes that only surface against a real
client, and exactly which knob to turn for each - so the operator does not have
to re-derive them.

---

## 0. Preconditions

- Prod stack up via `docker compose -f docker-compose.prod.yml up -d`
  (Hydra, hydra-migrate, api, mcp, web all healthy).
- `.env.prod` populated from `.env.prod.example`, in particular:
  `HYDRA_ISSUER_URL`, `HYDRA_LOGIN_URL`, `HYDRA_CONSENT_URL`, `HYDRA_DSN`,
  `HYDRA_SECRETS_SYSTEM`, `MCP_RESOURCE_URL`, `OAUTH_SCOPES`.
- The external Cloudflare tunnel (managed outside this repo) has its public
  hostnames configured against the host-published ports (see §3).
- A Kuberan user account exists (register through the web app first).

Reference public surface (all through the external tunnel, HTTPS only; the
tunnel reaches the compose services via their host-published ports):

| Host / path | Backend (host port) | Purpose |
|---|---|---|
| `auth.<domain>/oauth2/register` | `:8080` (api) | **Hardened DCR proxy** (`POST /oauth2/register`) |
| `auth.<domain>/*` (all other) | `:4444` (hydra) | OAuth AS: authorize, token, JWKS, `.well-known/*` |
| `mcp.<domain>` | `:8081` (mcp) | MCP Resource Server + `/.well-known/oauth-protected-resource` |
| `app.<domain>/oauth/login`, `/oauth/consent` | `:3000` (web) | Login + consent pages |
| `api.<domain>` (optional) | `:8080` (api) | REST API |

Hydra admin (`:4445`) is **never** published by compose nor mapped in the tunnel.

---

## 1. Phase 3 - Connect Claude end-to-end

Drive it exactly as an end user would (per CLAUDE.md's E2E requirement):

1. In Claude, **Add a custom connector** pointing at `https://mcp.<domain>/mcp`.
2. Claude fetches `/mcp` unauthenticated, gets `401` with
   `WWW-Authenticate: Bearer resource_metadata="https://mcp.<domain>/.well-known/oauth-protected-resource"`,
   then fetches that PRM document and reads `authorization_servers: [https://auth.<domain>]`.
3. Claude fetches `https://auth.<domain>/.well-known/oauth-authorization-server`
   (served by Hydra) to learn `authorization_endpoint`, `token_endpoint`,
   `jwks_uri`, and `registration_endpoint`.
4. Claude self-registers via `POST https://auth.<domain>/oauth2/register`
   (routed by cloudflared to the **api proxy**, not Hydra - see §3).
5. Claude opens the authorize URL; Hydra redirects to
   `app.<domain>/oauth/login?login_challenge=...`. Sign in.
6. First connect is an **unknown client** -> the consent screen renders (client
   name, `redirect_uri` host, scopes, "remember this client"). Approve + remember.
7. Claude exchanges the code (+ PKCE verifier) for access + refresh JWTs and
   calls tools. Run several: `list_accounts`, `list_transactions`, `get_portfolio`.
8. **Refresh:** wait past the 15m access-token TTL (`ttl.access_token` in
   `ory/hydra/hydra.yml`) and run another tool. Silent refresh must succeed with
   no re-prompt.

**Pass criteria:** connector authorizes without pasting any token; a
`OAUTH_CLIENT_REGISTERED` audit row + Zap warn appears at registration; a
`trusted_oauth_clients` row appears after "remember"; the second connect (or
re-auth) auto-accepts consent with no screen; tools return data; post-TTL calls
keep working via refresh.

### Fast offline sanity checks (before the Claude test)

```sh
# PRM document (should list authorization_servers = [issuer], scopes, bearer=header)
curl -s https://mcp.<domain>/.well-known/oauth-protected-resource | jq

# 401 challenge carries the resource_metadata hint
curl -si https://mcp.<domain>/mcp | grep -i www-authenticate

# Hydra AS metadata + JWKS resolve, and advertise a registration_endpoint
curl -s https://auth.<domain>/.well-known/oauth-authorization-server | jq '{authorization_endpoint,token_endpoint,jwks_uri,registration_endpoint}'
curl -s https://auth.<domain>/.well-known/jwks.json | jq '.keys[0].kid'

# Admin port MUST be unreachable from outside (expect timeout / connection refused)
curl -si --max-time 5 https://auth.<domain>:4445/admin/clients ; echo "exit=$?"
```

---

## 2. Interop failure modes (diagnose here first)

Both are latent design ambiguities that only a real client exercises. The
current code is internally **consistent** (host-only resource identity is used
by the PRM `resource`, the forced consent audience, and the RS validator's
`aud` check alike), so it should work with a spec-compliant client. If Claude
fails, these are the two knobs.

### 2a. DCR reaches Hydra instead of the proxy

**Symptom:** a new client registers but is a *confidential* client, or holds
`client_credentials`/implicit grants, or uncapped scopes; no
`OAUTH_CLIENT_REGISTERED` audit row appears.

**Cause:** the tunnel routed `auth.<domain>/oauth2/register` straight to
Hydra (`:4444`), bypassing every Phase 5 control.

**Fix:** in the Cloudflare tunnel config, ensure the **more specific**
`/oauth2/register` path rule points at the api service (host port `:8080`) and
is ordered *before* the catch-all `auth.<domain>/* -> :4444` rule. Re-run the §1
sanity checks. (`cmd/api/main.go:178` serves this exact path.)

### 2b. Resource-identity / token-audience mismatch (RFC 9728 + RFC 8707)

**Symptom:** Claude cannot find the PRM document (fetches
`/.well-known/oauth-protected-resource/mcp` and gets 404), **or** rejects the
issued access token because its `aud` does not match the connector URL, **or**
the token request's `resource` indicator is refused.

**Cause:** `MCP_RESOURCE_URL` is host-only (`https://mcp.<domain>`), but the
connector URL has a path (`https://mcp.<domain>/mcp`). RFC 9728 §3.1 builds the
metadata URL by inserting `/.well-known/oauth-protected-resource` *before* the
resource path, and RFC 8707 clients may request `resource=<connector URL incl.
path>`. A strict client may therefore expect the resource identity to include
`/mcp`.

**Fix (only if 2b is observed in the live test):** make the resource identity
path-inclusive and consistent end to end:

1. Set `MCP_RESOURCE_URL=https://mcp.<domain>/mcp` in `.env.prod`. This flows to
   both the RS validator's `aud` check and the consent bridge's forced audience
   grant (`config.MCPResourceURL`), keeping them aligned.
2. Register the discovery handler at the path-suffixed well-known URL too:
   in `internal/mcp/server.go` add a second `mux.Handle` for
   `"/.well-known/oauth-protected-resource/mcp"` alongside the existing
   `ProtectedResourceMetadataPath`, both served by `discoveryHandler(oauth)`.
   The `resource` field in the emitted doc will then correctly be the `/mcp` URL.
3. Rebuild the mcp image and re-run §1.

Do **not** apply this pre-emptively: the host-only identity is coherent and
spec-compliant when the client honors the `resource_metadata` hint from the
`WWW-Authenticate` header (which the MCP auth spec mandates). Flip it only if the
real client demonstrably needs the path-inclusive form.

---

## 3. Phase 5 - Cloudflare & ops hardening

Code-side hardening (proxy policy, capped scopes, audit/alert, consent tripwire,
Deny/Cancel escape hatches, forced token audience) is already shipped. The
following are dashboard/ops actions:

- **[MUST] Admin port stays private (step 1).** Confirm no cloudflared public
  hostname maps to `:4445`. Re-verify after *any* tunnel change with the admin
  `curl` in §1 (must fail).
- **[MUST] `/oauth2/register` routes to the api proxy (step 2).** See §2a.
- **WAF rate-limit + Bot Fight Mode on `/oauth2/register` (step 5).** In the
  Cloudflare dashboard, add a Rate Limiting rule scoped to
  `http.host eq "auth.<domain>" and http.request.uri.path eq "/oauth2/register"`
  (e.g. 5 req / min / IP -> block) and enable Bot Fight Mode. IP allowlisting is
  **not** viable - Anthropic's DCR egress is not a stable published range.
- **New-client alert (step 4).** Already emitted as a Zap warn +
  `OAUTH_CLIENT_REGISTERED` audit row per registration. Optionally forward that
  audit event / log line to a notification channel (email, Telegram bot) so a
  single-user instance gets a real-time intrusion signal.
- **Skip public DCR entirely? (step 7).** If a Claude client variant you use
  accepts a pre-registered `client_id`, set `HYDRA_PINNED_CLIENT_ID`, disable the
  public `/oauth2/register` ingress, and the mass-registration risk class
  disappears. Confirm per client; claude.ai custom connectors likely require DCR.

**Phase 5 pass criteria:** `:4445` unreachable externally; a DCR request with a
disallowed grant/scope is down-scoped by the proxy (public client, `read:*`
only); a new registration produces an audit entry + alert; a burst against
`/oauth2/register` trips the rate-limit.

---

## 4. Rollback (post-cutover)

Phase 4 already deleted the legacy static-token path. If OAuth must be rolled
back: redeploy the pre-Phase-4 api/mcp image, run
`go run cmd/migrate/main.go down 1` to re-add `users.mcp_token_hash` (migration
`000023`), re-issue a legacy token, and re-add the connector in Claude. Single
user -> the cutover window is small and acceptable.
