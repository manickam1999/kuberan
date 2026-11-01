package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/hydra"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// protocolScopes are the OIDC / refresh-token scopes that always pass through
// consent in addition to the registered API scopes. Dropping
// offline_access here would silently break silent refresh (Phase 3).
// Keep in sync with PROTOCOL_SCOPES in apps/web/src/lib/oauth-scopes.ts.
var protocolScopes = map[string]bool{
	"openid":         true,
	"offline":        true,
	"offline_access": true,
}

// OAuthHandler bridges the browser-facing login/consent pages (apps/web) to
// Hydra's admin API, keeping all admin traffic server-side. It resolves Hydra's
// login and consent challenges against the existing user store and applies
// per-user, per-scope consent grants. See plans/015-mcp-oauth-hydra Phase 1 and
// plans/022-mcp-write-access.
type OAuthHandler struct {
	hydra         hydra.AdminClient
	userService   services.UserServicer
	grants        services.OAuthGrantServicer
	auditService  services.AuditServicer
	allowedScopes []string // registered API scopes; grants are capped to this set.
	resourceURL   string   // MCP resource identifier always stamped into the granted audience.
	// pinnedClientID is the pre-registered OAuth client (HYDRA_PINNED_CLIENT_ID),
	// shown as verified on the consent screen. Every other client registered
	// itself through DCR, so its name is whatever it chose to call itself.
	pinnedClientID string
}

// OAuthHandlerOption configures optional OAuthHandler settings.
type OAuthHandlerOption func(*OAuthHandler)

// WithPinnedClientID marks clientID (config.HydraPinnedClientID) as the
// operator's pre-registered client: the consent screen shows it as verified
// rather than as a self-registered (unverified) client. Empty means none.
func WithPinnedClientID(clientID string) OAuthHandlerOption {
	return func(h *OAuthHandler) { h.pinnedClientID = clientID }
}

// NewOAuthHandler creates a new OAuthHandler. scopes is the space-delimited set
// of registered API scopes (config.OAuthScopes) that grants are capped to.
// resourceURL is the MCP resource identifier (config.MCPResourceURL); it is
// always added to the granted token audience so every token issued through this
// flow validates against the MCP Resource Server, whose validator requires
// aud contains resourceURL.
func NewOAuthHandler(
	adminClient hydra.AdminClient,
	userService services.UserServicer,
	grants services.OAuthGrantServicer,
	auditService services.AuditServicer,
	scopes string,
	resourceURL string,
	opts ...OAuthHandlerOption,
) *OAuthHandler {
	h := &OAuthHandler{
		hydra:         adminClient,
		userService:   userService,
		grants:        grants,
		auditService:  auditService,
		allowedScopes: strings.Fields(scopes),
		resourceURL:   resourceURL,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// invalidBody is the error for a request body that fails to bind. It does not
// echo the binding error, which names internal struct fields and validators.
func invalidBody(required string) error {
	return apperrors.WithMessage(apperrors.ErrInvalidInput, "invalid request body: "+required)
}

// oauthLoginRequest is the payload the apps/web login page submits.
type oauthLoginRequest struct {
	LoginChallenge string `json:"login_challenge" binding:"required"`
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required"`
}

// oauthLoginRejectRequest is the payload the apps/web login page submits when
// the user cancels a login they did not initiate.
type oauthLoginRejectRequest struct {
	LoginChallenge string `json:"login_challenge" binding:"required"`
}

// oauthConsentAcceptRequest is the payload the apps/web consent page submits.
// GrantedScopes lists the API scopes the user ticked; protocol scopes (openid,
// offline_access) are not user-selectable. When it is omitted, only the
// requested read:* scopes are granted, so an outdated page can never grant
// write access.
type oauthConsentAcceptRequest struct {
	ConsentChallenge string   `json:"consent_challenge" binding:"required"`
	RememberClient   bool     `json:"remember_client"`
	GrantedScopes    []string `json:"granted_scopes"`
}

// oauthConsentRejectRequest is the payload the apps/web consent page submits when
// the user denies an unknown client on the anti-phishing screen.
type oauthConsentRejectRequest struct {
	ConsentChallenge string `json:"consent_challenge" binding:"required"`
}

// Login resolves a Hydra login challenge. It first confirms with Hydra that
// the challenge is a live login request, so the endpoint cannot be used to
// test credentials outside a real authorization flow; then it verifies the
// credentials through the existing login/lockout path and, on success,
// accepts the login with subject = user ID, returning Hydra's redirect_to.
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
// @Failure     404 {object} ErrorResponse "Unknown or expired challenge"
// @Router      /oauth/login [post]
func (h *OAuthHandler) Login(c *gin.Context) {
	var req oauthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, invalidBody("login_challenge, a valid email and password are required"))
		return
	}

	// Validate the challenge before touching credentials: a bad or spent
	// challenge must not reveal whether the password was right, nor count
	// toward (or reset) the account lockout.
	if _, err := h.hydra.GetLoginRequest(c.Request.Context(), req.LoginChallenge); err != nil {
		respondWithError(c, err)
		return
	}

	user, err := h.userService.AttemptLogin(req.Email, req.Password)
	if err != nil {
		h.auditService.Log("", "OAUTH_LOGIN_FAILED", "user", "", c.ClientIP(),
			map[string]interface{}{"email": truncateRunes(req.Email, maxAuditEmailLen)})
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

// RejectLogin cancels a pending login challenge without authenticating. This is
// the login-side escape hatch: a user who lands on the sign-in page for a
// connection they did not start (a phishing signal, since the OAuth login page is
// only reachable mid-flow) can decline rather than abandon the tab. It rejects
// the challenge via Hydra with access_denied and audits the refusal, returning
// Hydra's redirect_to so the client sees the denial. No credentials are checked
// and no subject is bound.
//
// @Summary     Reject an OAuth login challenge
// @Description Cancel an unauthenticated login the user did not initiate
// @Tags        oauth
// @Accept      json
// @Produce     json
// @Param       request body oauthLoginRejectRequest true "Login challenge"
// @Success     200 {object} map[string]string "redirect_to"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     404 {object} ErrorResponse "Unknown challenge"
// @Router      /oauth/login/reject [post]
func (h *OAuthHandler) RejectLogin(c *gin.Context) {
	var req oauthLoginRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, invalidBody("login_challenge is required"))
		return
	}

	redirectTo, err := h.hydra.RejectLogin(c.Request.Context(), req.LoginChallenge, "user cancelled login")
	if err != nil {
		respondWithError(c, err)
		return
	}

	// The user is unauthenticated here, so there is no subject to attribute the
	// refusal to; the client IP is the only available signal.
	h.auditService.Log("", "OAUTH_LOGIN_DENIED", "user", "", c.ClientIP(), nil)
	c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
}

// GetConsent fetches a pending consent challenge. A client the user asked to
// remember is accepted server-side immediately (returning redirect_to), but
// only while every requested scope is one the user has already reviewed, and
// only with the scopes they approved. Otherwise it returns the details the
// consent page needs to render, including the previously approved scopes.
//
// @Summary     Fetch an OAuth consent challenge
// @Description Auto-accept remembered clients for reviewed scopes, or return details for the consent page
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

	grant, err := h.activeGrant(consent)
	if err != nil {
		respondWithError(c, err)
		return
	}

	requested := h.grantableScopes(consent.RequestedScope)
	if grant != nil && grant.AutoAccept && containsAll(grant.Reviewed(), apiScopes(requested)) {
		granted := intersect(requested, append(grant.Approved(), protocolScopeList(requested)...))
		redirectTo, err := h.acceptConsent(c, consent, granted, false)
		if err != nil {
			respondWithError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
		return
	}

	previouslyApproved := []string{}
	if grant != nil {
		previouslyApproved = grant.Approved()
	}

	// Hand the consent page what it needs to render the anti-phishing screen
	// (client name and whether it is verified, where the authorization code
	// will be sent, grantable scopes) and pre-tick what the user approved last
	// time.
	target := consentRedirectTarget(consent)
	c.JSON(http.StatusOK, gin.H{
		"client": gin.H{
			"client_id":   consent.Client.ClientID,
			"client_name": consent.Client.ClientName,
		},
		"client_verified":     h.pinnedClientID != "" && consent.Client.ClientID == h.pinnedClientID,
		"redirect_scheme":     target.Scheme,
		"redirect_host":       target.Host,
		"requested_scopes":    requested,
		"previously_approved": previouslyApproved,
	})
}

// redirectTarget is where an authorization code will be delivered: the scheme
// and, for http/https, the host (with port). A native app's private-use
// scheme (com.example.app:/cb) has no host.
type redirectTarget struct {
	Scheme string
	Host   string
}

// consentRedirectTarget returns where this authorization will send its code.
// That is the redirect_uri of the authorization request itself (Hydra's
// request_url), which Hydra has already matched against the client's
// registration; a client may register several, so the first registered URI is
// not necessarily the one in use. When the request names none, Hydra uses the
// client's only registered URI. Anything unparseable yields an empty target,
// which the consent page shows as unknown.
func consentRedirectTarget(consent *hydra.ConsentRequest) redirectTarget {
	raw := ""
	if u, err := url.Parse(consent.RequestURL); err == nil && consent.RequestURL != "" {
		raw = u.Query().Get("redirect_uri")
	}
	if raw == "" && len(consent.Client.RedirectURIs) == 1 {
		raw = consent.Client.RedirectURIs[0]
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return redirectTarget{}
	}
	t := redirectTarget{Scheme: strings.ToLower(u.Scheme)}
	if t.Scheme == "http" || t.Scheme == "https" {
		t.Host = strings.ToLower(u.Host)
	}
	return t
}

// mergedDecision combines this consent screen's decision with the client's
// existing grant. Scopes shown on this screen take the user's new choice;
// scopes approved earlier but not requested this time keep their approval, so
// a client that asks for fewer scopes in one flow doesn't silently lose
// permissions the user gave it before. A revoked grant is not merged.
func (h *OAuthHandler) mergedDecision(consent *hydra.ConsentRequest, ticked, shown []string) (approved, reviewed []string, err error) {
	prev, err := h.activeGrant(consent)
	if err != nil {
		return nil, nil, err
	}
	if prev == nil {
		return ticked, shown, nil
	}
	approved = mergeScopes(slices.Clone(ticked), withoutScopes(prev.Approved(), shown))
	reviewed = mergeScopes(slices.Clone(shown), prev.Reviewed())
	return approved, reviewed, nil
}

// withoutScopes returns scopes minus the members of remove.
func withoutScopes(scopes, remove []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if !slices.Contains(remove, s) {
			out = append(out, s)
		}
	}
	return out
}

// activeGrant returns the subject's non-revoked grant for the consent's client,
// or nil when there is none.
func (h *OAuthHandler) activeGrant(consent *hydra.ConsentRequest) (*models.OAuthClientGrant, error) {
	if consent.Subject == "" {
		return nil, nil
	}
	grant, err := h.grants.Get(consent.Subject, consent.Client.ClientID)
	if errors.Is(err, apperrors.ErrOAuthClientNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if grant.IsRevoked() {
		return nil, nil
	}
	return grant, nil
}

// AcceptConsent grants a consent challenge with the scopes the user ticked,
// capped to what the client requested and the registered set, and records the
// decision as the user's grant for the client. When remember_client is set,
// future consents for already-reviewed scopes auto-accept.
//
// @Summary     Accept an OAuth consent challenge
// @Description Grant the selected scopes and optionally remember the client
// @Tags        oauth
// @Accept      json
// @Produce     json
// @Param       request body oauthConsentAcceptRequest true "Consent challenge, selected scopes and remember flag"
// @Success     200 {object} map[string]string "redirect_to"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     404 {object} ErrorResponse "Unknown challenge"
// @Router      /oauth/consent/accept [post]
func (h *OAuthHandler) AcceptConsent(c *gin.Context) {
	var req oauthConsentAcceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, invalidBody("consent_challenge is required"))
		return
	}

	consent, err := h.hydra.GetConsentRequest(c.Request.Context(), req.ConsentChallenge)
	if err != nil {
		respondWithError(c, err)
		return
	}

	requested := h.grantableScopes(consent.RequestedScope)
	var granted []string
	if req.GrantedScopes == nil {
		granted = readOnly(requested)
	} else {
		granted = intersect(requested, append(req.GrantedScopes, protocolScopeList(requested)...))
	}

	approved, reviewed, err := h.mergedDecision(consent, apiScopes(granted), apiScopes(requested))
	if err != nil {
		respondWithError(c, err)
		return
	}

	// Record the grant before Hydra issues anything: if this fails, no token
	// exists without a matching grant row for the MCP write guard to check.
	if _, err := h.grants.RecordConsent(services.RecordConsentInput{
		UserID:     consent.Subject,
		ClientID:   consent.Client.ClientID,
		ClientName: consent.Client.ClientName,
		Approved:   approved,
		Reviewed:   reviewed,
		AutoAccept: req.RememberClient,
	}); err != nil {
		respondWithError(c, err)
		return
	}

	redirectTo, err := h.acceptConsent(c, consent, granted, req.RememberClient)
	if err != nil {
		respondWithError(c, err)
		return
	}

	// One event per decision; auto_accept records the "remember this
	// application" choice (formerly a separate OAUTH_CLIENT_TRUSTED event).
	h.auditService.Log(consent.Subject, "OAUTH_CONSENT_GRANTED", "oauth_client", consent.Client.ClientID,
		c.ClientIP(), map[string]interface{}{
			"client_name": consent.Client.ClientName,
			"scopes":      apiScopes(granted),
			"approved":    approved,
			"auto_accept": req.RememberClient,
		})

	c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
}

// RejectConsent denies a consent challenge. This is the anti-phishing tripwire's
// active escape hatch (Q4 TOFU / Phase 5 step 3): when a user is shown the consent
// screen for an unknown client and does not trust it (e.g. a suspicious
// redirect_uri host), Deny rejects the challenge via Hydra with access_denied and
// audits the refusal, returning Hydra's redirect_to so the client sees the denial.
//
// @Summary     Reject an OAuth consent challenge
// @Description Deny the requested authorization for an unknown client
// @Tags        oauth
// @Accept      json
// @Produce     json
// @Param       request body oauthConsentRejectRequest true "Consent challenge"
// @Success     200 {object} map[string]string "redirect_to"
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     404 {object} ErrorResponse "Unknown challenge"
// @Router      /oauth/consent/reject [post]
func (h *OAuthHandler) RejectConsent(c *gin.Context) {
	var req oauthConsentRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, invalidBody("consent_challenge is required"))
		return
	}

	consent, err := h.hydra.GetConsentRequest(c.Request.Context(), req.ConsentChallenge)
	if err != nil {
		respondWithError(c, err)
		return
	}

	redirectTo, err := h.hydra.RejectConsent(c.Request.Context(), consent.Challenge, "user denied consent")
	if err != nil {
		respondWithError(c, err)
		return
	}

	h.auditService.Log(consent.Subject, "OAUTH_CONSENT_DENIED", "oauth_client", consent.Client.ClientID,
		c.ClientIP(), map[string]interface{}{"client_name": consent.Client.ClientName})
	c.JSON(http.StatusOK, gin.H{"redirect_to": redirectTo})
}

// acceptConsent grants scopes (already capped by the caller) and the granted
// audience for a consent challenge.
func (h *OAuthHandler) acceptConsent(c *gin.Context, consent *hydra.ConsentRequest, scopes []string, remember bool) (string, error) {
	return h.hydra.AcceptConsent(c.Request.Context(), consent.Challenge, hydra.AcceptConsentParams{
		GrantScope:    withOfflineAccess(scopes, consent.Client),
		GrantAudience: h.grantableAudience(consent.RequestedAccessTokenAudience),
		Remember:      remember,
	})
}

// withOfflineAccess adds offline_access to granted when the client is
// registered for it, so Hydra issues a refresh token even if the client did not
// ask for one. Many MCP clients request only the RS's scopes_supported and would
// otherwise be forced to re-authorize every 15 minutes when the access token
// expires. It is only added for clients registered with offline_access: Hydra
// rejects refreshing a grant whose scopes exceed the client's registered set.
func withOfflineAccess(granted []string, client hydra.OAuth2Client) []string {
	if slices.Contains(granted, "offline_access") || slices.Contains(granted, "offline") {
		return granted
	}
	if !slices.Contains(strings.Fields(client.Scope), "offline_access") {
		return granted
	}
	return append(granted, "offline_access")
}

// grantableAudience returns the requested audiences with the MCP resource URL
// always included. MCP clients (e.g. Claude connectors) do not request a
// resource audience by default, but the RS validator requires the access token's
// aud to contain resourceURL; force-granting it here guarantees every token
// issued through consent is accepted by /mcp.
func (h *OAuthHandler) grantableAudience(requested []string) []string {
	out := make([]string, 0, len(requested)+1)
	seen := false
	for _, a := range requested {
		out = append(out, a)
		if a == h.resourceURL {
			seen = true
		}
	}
	if h.resourceURL != "" && !seen {
		out = append(out, h.resourceURL)
	}
	return out
}

// grantableScopes returns the requested scopes intersected with the registered
// API scope set (plus the OIDC/refresh protocol scopes). This is the defense-in-depth
// cap: a client can never be granted a scope the RS does not recognise.
func (h *OAuthHandler) grantableScopes(requested []string) []string {
	return capScopes(requested, h.allowedScopes)
}

// capScopes returns requested intersected with the allowed API scope set plus the
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

// writeScopePrefix marks scopes that let a client change the user's data
// (plan 022). They are never granted without an explicit opt-in. Keep in sync
// with isWriteScope in apps/web/src/lib/oauth-scopes.ts.
const writeScopePrefix = "write:"

// apiScopes returns scopes without the OIDC/refresh protocol scopes.
func apiScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if !protocolScopes[s] {
			out = append(out, s)
		}
	}
	return out
}

// protocolScopeList returns the OIDC/refresh protocol scopes present in scopes.
func protocolScopeList(scopes []string) []string {
	out := make([]string, 0, len(protocolScopes))
	for _, s := range scopes {
		if protocolScopes[s] {
			out = append(out, s)
		}
	}
	return out
}

// readOnly returns scopes without any write:* scope.
func readOnly(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if !strings.HasPrefix(s, writeScopePrefix) {
			out = append(out, s)
		}
	}
	return out
}

// intersect returns the members of scopes that are also in allowed, keeping
// the order of scopes.
func intersect(scopes, allowed []string) []string {
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if slices.Contains(allowed, s) {
			out = append(out, s)
		}
	}
	return out
}

// containsAll reports whether every member of want is in have.
func containsAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}
