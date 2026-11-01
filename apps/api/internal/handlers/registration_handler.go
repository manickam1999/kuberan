package handlers

import (
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/hydra"
	"kuberan/internal/logger"
	"kuberan/internal/services"
)

// allowedGrantTypes are the only OAuth grant types a DCR-registered client may
// hold: the authorization-code flow plus refresh-token rotation. Anything else
// (client_credentials, implicit, password) is dropped by the proxy.
var allowedGrantTypes = []string{"authorization_code", "refresh_token"}

// RegistrationHandler is a hardened RFC 7591 Dynamic Client Registration proxy in
// front of Hydra's admin API. It forces every self-registered client to be a
// public client (token_endpoint_auth_method=none; Hydra requires PKCE S256 for
// public clients via oauth2.pkce.enforced_for_public_clients in
// ory/hydra/hydra.yml), bounds the submitted metadata, restricts grant types to authorization_code + refresh_token, forces the
// authorization-code response type, and caps requested scopes to the registered
// read:* set. Every registration is audited and alerted on: on a single-user
// instance a new OAuth client is a near-perfect intrusion signal.
// See plans/015-mcp-oauth-hydra Phase 5.
type RegistrationHandler struct {
	hydra         hydra.AdminClient
	auditService  services.AuditServicer
	allowedScopes []string
}

// NewRegistrationHandler creates a RegistrationHandler. scopes is the space-
// delimited registered read:* set (config.OAuthScopes) that requested scopes are
// capped to.
func NewRegistrationHandler(adminClient hydra.AdminClient, auditService services.AuditServicer, scopes string) *RegistrationHandler {
	return &RegistrationHandler{
		hydra:         adminClient,
		auditService:  auditService,
		allowedScopes: strings.Fields(scopes),
	}
}

// dcrRequest is the RFC 7591 client-metadata payload a client submits to register.
// Only the fields the proxy inspects are bound; the rest of the policy is forced.
type dcrRequest struct {
	ClientName    string   `json:"client_name"`
	RedirectURIs  []string `json:"redirect_uris"`
	GrantTypes    []string `json:"grant_types"`
	ResponseTypes []string `json:"response_types"`
	Scope         string   `json:"scope"`
}

// Register handles a Dynamic Client Registration request. It validates and
// constrains the requested metadata, creates the client via Hydra admin, records
// an audit entry, and returns the created client (RFC 7591 registration response).
//
// @Summary     Dynamically register an OAuth client (DCR proxy)
// @Description Register a public OAuth client with forced PKCE, restricted grants, and capped scopes
// @Tags        oauth
// @Accept      json
// @Produce     json
// @Param       request body dcrRequest true "Client metadata (RFC 7591)"
// @Success     201 {object} hydra.OAuth2Client "Registered client"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Router      /oauth/register [post]
func (h *RegistrationHandler) Register(c *gin.Context) {
	// The endpoint is public: bound the body before decoding it.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDCRBodyBytes)
	var req dcrRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput,
			"invalid client metadata: expected a JSON object of at most 16 KiB"))
		return
	}

	if len(req.RedirectURIs) == 0 {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "redirect_uris is required"))
		return
	}
	if len(req.RedirectURIs) > maxRedirectURIs {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "too many redirect_uris (max 3)"))
		return
	}
	for _, u := range req.RedirectURIs {
		if !isValidRedirectURI(u) {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput,
				"invalid redirect_uri: use https, http on a loopback address, or a native-app scheme (com.example.app:/callback)"))
			return
		}
	}

	// Cap requested scopes to the read:* set (plus OIDC/refresh protocol scopes),
	// dropping anything disallowed. MCP connectors commonly register without a
	// scope, declaring the scopes they want only at authorize time; Hydra checks
	// authorize-time scopes against the client's registered set, so a client
	// registered with an empty scope could never obtain a usable token. In that
	// case default the registrable scope to the full read:* set plus the
	// protocol scopes so authorize and silent refresh can succeed. The consent
	// bridge still caps what is actually granted per request (defense in depth).
	scope := capScopes(strings.Fields(req.Scope), h.allowedScopes)
	if len(scope) == 0 {
		scope = h.defaultRegistrableScopes()
	} else {
		// Clients that register with an explicit scope usually copy the RS's
		// scopes_supported (read:* only) and omit offline_access. Without it
		// registered, Hydra can never issue them a refresh token (and refuses to
		// refresh one granted at consent), so the connection dies with the
		// 15-minute access token. Always register the protocol scopes.
		scope = appendMissing(scope, refreshProtocolScopes...)
	}

	// Force the hardened policy regardless of what the client asked for.
	create := hydra.OAuth2ClientCreate{
		ClientName:              sanitizeClientName(req.ClientName),
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              allowedGrantTypes,
		ResponseTypes:           []string{"code"},
		Scope:                   strings.Join(scope, " "),
		TokenEndpointAuthMethod: "none",
	}

	client, err := h.hydra.CreateClient(c.Request.Context(), create)
	if err != nil {
		respondWithError(c, err)
		return
	}

	// Audit + alert: a new OAuth client on a single-user instance is a strong
	// intrusion signal (Phase 5 step 4).
	h.auditService.Log("", "OAUTH_CLIENT_REGISTERED", "oauth_client", client.ClientID, c.ClientIP(),
		map[string]interface{}{
			"client_name":   client.ClientName,
			"redirect_uris": client.RedirectURIs,
			"scope":         client.Scope,
		})
	logger.Get().Warnw("new OAuth client registered via DCR",
		"client_id", client.ClientID,
		"client_name", client.ClientName,
		"redirect_uris", client.RedirectURIs,
		"ip", c.ClientIP(),
	)

	c.JSON(http.StatusCreated, client)
}

// Limits on self-registered client metadata. Registration is public, so
// every field is bounded.
const (
	// maxDCRBodyBytes caps the registration request body.
	maxDCRBodyBytes = 16 << 10
	// maxRedirectURIs caps how many redirect URIs one client may register.
	maxRedirectURIs = 3
	// maxRedirectURILen caps each redirect URI.
	maxRedirectURILen = 2048
	// maxClientNameLen caps a self-registered client's display name. It is
	// shown on the consent screen and in Settings > Connected apps.
	maxClientNameLen = 100
)

// sanitizeClientName returns name with control and format characters (newlines,
// bidi overrides, zero-width characters) replaced by spaces, whitespace
// collapsed, and cut to maxClientNameLen characters. The name is chosen by
// whoever registers the client and is displayed to the user on the consent
// screen, so it must not be able to fake extra lines or reorder text.
func sanitizeClientName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(name, ""))
	clean = strings.Join(strings.Fields(clean), " ")
	if runes := []rune(clean); len(runes) > maxClientNameLen {
		clean = strings.TrimSpace(string(runes[:maxClientNameLen]))
	}
	return clean
}

// defaultRegistrableScopes is the scope set assigned to a DCR client that
// registers without requesting any scope: the full read:* set plus the
// OIDC/refresh protocol scopes (openid, offline, offline_access), so the client
// can select them at authorize time and obtain refresh tokens. Consent still
// gates what is actually granted per request.
func (h *RegistrationHandler) defaultRegistrableScopes() []string {
	out := make([]string, 0, len(h.allowedScopes)+3)
	out = append(out, h.allowedScopes...)
	out = append(out, refreshProtocolScopes...)
	return out
}

// refreshProtocolScopes are the OIDC/refresh protocol scopes every DCR client
// is registered with, in registration order.
var refreshProtocolScopes = []string{"openid", "offline", "offline_access"}

// appendMissing appends each of add that is not already in scopes.
func appendMissing(scopes []string, add ...string) []string {
	for _, a := range add {
		if !slices.Contains(scopes, a) {
			scopes = append(scopes, a)
		}
	}
	return scopes
}

// forbiddenSchemes are URI schemes that can never be an OAuth redirect: they
// run script, read local files or render attacker-chosen content.
var forbiddenSchemes = map[string]bool{
	"javascript": true, "data": true, "file": true, "vbscript": true,
	"blob": true, "about": true, "ftp": true, "ws": true, "wss": true,
}

// isValidRedirectURI reports whether s is an acceptable OAuth redirect URI for
// a self-registered client (RFC 6749 3.1.2, RFC 8252 7, OAuth 2.1):
//
//   - an absolute URI of at most maxRedirectURILen characters, with no
//     fragment and no userinfo;
//   - https with a host; or
//   - http only on a loopback host (127.0.0.1, [::1] or localhost), for
//     native apps that listen locally; or
//   - a private-use scheme for native apps. RFC 8252 recommends reverse-DNS
//     form (com.example.app:/callback), but common MCP clients register
//     single-word schemes (cursor://, vscode://), so any scheme is accepted
//     except one that runs script or reads local content. The consent screen
//     warns the user when the target is a custom scheme.
func isValidRedirectURI(s string) bool {
	if s == "" || len(s) > maxRedirectURILen || strings.ContainsAny(s, " \t\r\n\\") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Fragment != "" || u.User != nil || strings.Contains(s, "#") {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return u.Hostname() != ""
	case "http":
		return isLoopbackHost(u.Hostname())
	}
	return !forbiddenSchemes[scheme]
}

// isLoopbackHost reports whether host is a loopback address or localhost.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
