package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/hydra"
	"kuberan/internal/services"
)

// protocolScopes are the OIDC / refresh-token scopes that always pass through
// consent in addition to the registered read:* API scopes. Dropping
// offline_access here would silently break silent refresh (Phase 3).
var protocolScopes = map[string]bool{
	"openid":         true,
	"offline":        true,
	"offline_access": true,
}

// OAuthHandler bridges the browser-facing login/consent pages (apps/web) to
// Hydra's admin API, keeping all admin traffic server-side. It resolves Hydra's
// login and consent challenges against the existing user store and applies
// trust-on-first-use consent. See plans/015-mcp-oauth-hydra Phase 1.
type OAuthHandler struct {
	hydra         hydra.AdminClient
	userService   services.UserServicer
	trustedClient services.TrustedClientServicer
	auditService  services.AuditServicer
	allowedScopes []string // registered read:* scopes; grants are capped to this set.
}

// NewOAuthHandler creates a new OAuthHandler. scopes is the space-delimited set
// of registered read:* scopes (config.OAuthScopes) that grants are capped to.
func NewOAuthHandler(
	adminClient hydra.AdminClient,
	userService services.UserServicer,
	trustedClient services.TrustedClientServicer,
	auditService services.AuditServicer,
	scopes string,
) *OAuthHandler {
	return &OAuthHandler{
		hydra:         adminClient,
		userService:   userService,
		trustedClient: trustedClient,
		auditService:  auditService,
		allowedScopes: strings.Fields(scopes),
	}
}

// oauthLoginRequest is the payload the apps/web login page submits.
type oauthLoginRequest struct {
	LoginChallenge string `json:"login_challenge" binding:"required"`
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required"`
}

// oauthConsentAcceptRequest is the payload the apps/web consent page submits.
type oauthConsentAcceptRequest struct {
	ConsentChallenge string `json:"consent_challenge" binding:"required"`
	RememberClient   bool   `json:"remember_client"`
}

// Login resolves a Hydra login challenge. It verifies the submitted credentials
// through the existing login/lockout path and, on success, accepts the login
// with subject = user ID, returning Hydra's redirect_to.
//
// @Summary     Resolve an OAuth login challenge
// @Description Verify credentials and accept Hydra's login challenge
// @Tags        oauth
// @Accept      json
// @Produce     json
// @Param       request body oauthLoginRequest true "Login challenge and credentials"
// @Success     200 {object} map[string]string "redirect_to"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Invalid credentials"
// @Router      /oauth/login [post]
func (h *OAuthHandler) Login(c *gin.Context) {
	var req oauthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	user, err := h.userService.AttemptLogin(req.Email, req.Password)
	if err != nil {
		h.auditService.Log("", "OAUTH_LOGIN_FAILED", "user", "", c.ClientIP(),
			map[string]interface{}{"email": req.Email})
		respondWithError(c, err)
		return
	}

	redirectTo, err := h.hydra.AcceptLogin(c.Request.Context(), req.LoginChallenge, hydra.AcceptLoginParams{
		Subject:  user.ID,
		Remember: true,
	})
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(user.ID, "OAUTH_LOGIN", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
}

// GetConsent fetches a pending consent challenge. Trusted clients are accepted
// server-side immediately (returning redirect_to); unknown clients return the
// details the consent page needs to render.
//
// @Summary     Fetch an OAuth consent challenge
// @Description Auto-accept trusted clients, or return details for unknown clients
// @Tags        oauth
// @Produce     json
// @Param       consent_challenge query string true "Consent challenge"
// @Success     200 {object} map[string]interface{} "redirect_to or consent details"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     404 {object} ErrorResponse "Unknown challenge"
// @Router      /oauth/consent [get]
func (h *OAuthHandler) GetConsent(c *gin.Context) {
	challenge := c.Query("consent_challenge")
	if challenge == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "consent_challenge is required"))
		return
	}

	consent, err := h.hydra.GetConsentRequest(c.Request.Context(), challenge)
	if err != nil {
		respondWithError(c, err)
		return
	}

	trusted, err := h.trustedClient.IsTrusted(consent.Client.ClientID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if trusted {
		redirectTo, err := h.acceptConsent(c, consent, false)
		if err != nil {
			respondWithError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
		return
	}

	// Unknown client: hand the consent page what it needs to render the
	// anti-phishing screen (client name, redirect host, grantable scopes).
	c.JSON(http.StatusOK, gin.H{
		"client": gin.H{
			"client_id":   consent.Client.ClientID,
			"client_name": consent.Client.ClientName,
		},
		"requested_scopes": h.grantableScopes(consent.RequestedScope),
		"redirect_uris":    consent.Client.RedirectURIs,
	})
}

// AcceptConsent grants a consent challenge. When remember_client is set the
// client is recorded in trusted_oauth_clients (with an audit entry) so future
// consents auto-accept.
//
// @Summary     Accept an OAuth consent challenge
// @Description Grant the requested read:* scopes and optionally remember the client
// @Tags        oauth
// @Accept      json
// @Produce     json
// @Param       request body oauthConsentAcceptRequest true "Consent challenge and remember flag"
// @Success     200 {object} map[string]string "redirect_to"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     404 {object} ErrorResponse "Unknown challenge"
// @Router      /oauth/consent/accept [post]
func (h *OAuthHandler) AcceptConsent(c *gin.Context) {
	var req oauthConsentAcceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}

	consent, err := h.hydra.GetConsentRequest(c.Request.Context(), req.ConsentChallenge)
	if err != nil {
		respondWithError(c, err)
		return
	}

	redirectTo, err := h.acceptConsent(c, consent, req.RememberClient)
	if err != nil {
		respondWithError(c, err)
		return
	}

	if req.RememberClient {
		if _, err := h.trustedClient.Trust(consent.Client.ClientID, consent.Client.ClientName); err != nil {
			respondWithError(c, err)
			return
		}
		h.auditService.Log(consent.Subject, "OAUTH_CLIENT_TRUSTED", "oauth_client", consent.Client.ClientID,
			c.ClientIP(), map[string]interface{}{"client_name": consent.Client.ClientName})
	}

	c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
}

// acceptConsent grants the requested scopes (capped to the allowed set) and the
// requested audience for a consent challenge.
func (h *OAuthHandler) acceptConsent(c *gin.Context, consent *hydra.ConsentRequest, remember bool) (string, error) {
	return h.hydra.AcceptConsent(c.Request.Context(), consent.Challenge, hydra.AcceptConsentParams{
		GrantScope:    h.grantableScopes(consent.RequestedScope),
		GrantAudience: consent.RequestedAccessTokenAudience,
		Remember:      remember,
	})
}

// grantableScopes returns the requested scopes intersected with the registered
// read:* set (plus the OIDC/refresh protocol scopes). This is the defense-in-depth
// cap: a client can never be granted a scope the RS does not recognise.
func (h *OAuthHandler) grantableScopes(requested []string) []string {
	return capScopes(requested, h.allowedScopes)
}

// capScopes returns requested intersected with the allowed read:* set plus the
// OIDC/refresh protocol scopes. Shared by consent granting and DCR registration
// so both apply the same defense-in-depth scope ceiling.
func capScopes(requested, allowed []string) []string {
	set := make(map[string]bool, len(allowed))
	for _, s := range allowed {
		set[s] = true
	}
	out := make([]string, 0, len(requested))
	for _, s := range requested {
		if set[s] || protocolScopes[s] {
			out = append(out, s)
		}
	}
	return out
}
