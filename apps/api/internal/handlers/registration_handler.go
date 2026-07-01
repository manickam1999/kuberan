package handlers

import (
	"net/http"
	"net/url"
	"strings"

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
// public client (token_endpoint_auth_method=none, so Hydra mandates PKCE S256),
// restricts grant types to authorization_code + refresh_token, forces the
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
	var req dcrRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	if len(req.RedirectURIs) == 0 {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "redirect_uris is required"))
		return
	}
	for _, u := range req.RedirectURIs {
		if !isValidRedirectURI(u) {
			respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid redirect_uri: "+u))
			return
		}
	}

	// Force the hardened policy regardless of what the client asked for.
	create := hydra.OAuth2ClientCreate{
		ClientName:              req.ClientName,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              allowedGrantTypes,
		ResponseTypes:           []string{"code"},
		Scope:                   strings.Join(capScopes(strings.Fields(req.Scope), h.allowedScopes), " "),
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

// isValidRedirectURI reports whether s is an acceptable OAuth redirect URI: an
// absolute URL with a scheme and either a host (https/http, incl. loopback) or a
// custom native-app scheme with an opaque/path body.
func isValidRedirectURI(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return false
	}
	// http/https must carry a host; custom schemes (native apps) need not.
	if (u.Scheme == "http" || u.Scheme == "https") && u.Host == "" {
		return false
	}
	return true
}
