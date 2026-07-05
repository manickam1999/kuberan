// Package hydra provides a thin, typed client over the Ory Hydra admin REST API.
//
// It exposes only the surface Kuberan needs to resolve Hydra's login and consent
// challenges against the existing user store (see plans/015-mcp-oauth-hydra). The
// admin API is reachable only on the private Docker network; browser traffic must
// go through apps/api handlers, never directly to Hydra's admin port.
package hydra

// OAuth2Client is the subset of a Hydra OAuth 2.0 client used for consent display,
// trust checks, and DCR registration responses.
type OAuth2Client struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   string   `json:"scope"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

// OAuth2ClientCreate is the client metadata posted to Hydra's admin API to
// register a new OAuth 2.0 client. The DCR proxy fully controls these fields
// (public client, capped scopes, restricted grants) before creation.
type OAuth2ClientCreate struct {
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

// LoginRequest describes a pending Hydra login challenge.
type LoginRequest struct {
	Challenge      string       `json:"challenge"`
	Subject        string       `json:"subject"`
	Skip           bool         `json:"skip"`
	Client         OAuth2Client `json:"client"`
	RequestedScope []string     `json:"requested_scope"`
	RequestURL     string       `json:"request_url"`
}

// ConsentRequest describes a pending Hydra consent challenge.
type ConsentRequest struct {
	Challenge                    string       `json:"challenge"`
	Subject                      string       `json:"subject"`
	Skip                         bool         `json:"skip"`
	Client                       OAuth2Client `json:"client"`
	RequestedScope               []string     `json:"requested_scope"`
	RequestedAccessTokenAudience []string     `json:"requested_access_token_audience"`
}

// AcceptLoginParams are the inputs for accepting a login challenge.
type AcceptLoginParams struct {
	Subject     string // Kuberan user ID; becomes the token `sub`.
	Remember    bool   // remember this login for the browser session.
	RememberFor int    // seconds to remember (0 = remember forever).
}

// AcceptConsentParams are the inputs for accepting a consent challenge.
type AcceptConsentParams struct {
	GrantScope    []string // scopes to grant (subset of requested ∩ allowed).
	GrantAudience []string // access-token audience to grant.
	Remember      bool     // remember this consent (trust-on-first-use).
	RememberFor   int      // seconds to remember (0 = remember forever).
}

// redirectResponse is Hydra's reply to accept/reject calls.
type redirectResponse struct {
	RedirectTo string `json:"redirect_to"`
}
