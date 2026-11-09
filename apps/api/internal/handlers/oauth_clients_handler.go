package handlers

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// ConnectedAppResponse is one OAuth client the user has granted access to.
// Hydra-only entries (Legacy) are clients whose access predates per-scope
// grants (plan 022): Hydra still holds their consent, but Kuberan has no
// grant row, so they can read but never write. CreatedAt and LastUsedAt are
// unknown for them.
type ConnectedAppResponse struct {
	ClientID   string `json:"client_id"`
	ClientName string `json:"client_name"`
	// ClientHost is where the app connects from (claude.ai); "" until the
	// grant's next consent.
	ClientHost string   `json:"client_host"`
	Scopes     []string `json:"scopes"`
	AutoAccept bool     `json:"auto_accept"`
	Legacy     bool     `json:"legacy,omitempty"`
	// WriteLimits is set when the grant has any write:* scope.
	WriteLimits *WriteLimitsResponse `json:"write_limits,omitempty"`
	CreatedAt   *time.Time           `json:"created_at,omitempty"`
	LastUsedAt  *time.Time           `json:"last_used_at,omitempty"`
}

// WriteLimitsResponse is what an app may change: each change is capped at
// MaxAmount cents of Currency (the user's reporting currency), and the app
// gets PerMinute and PerDay writes (0 means no limit).
type WriteLimitsResponse struct {
	MaxAmount int64  `json:"max_amount"`
	Currency  string `json:"currency"`
	PerMinute int    `json:"per_minute"`
	PerDay    int    `json:"per_day"`
}

// ConnectedAppsResponse wraps the user's connected apps.
type ConnectedAppsResponse struct {
	Clients []ConnectedAppResponse `json:"clients"`
}

// ListConnectedApps returns the OAuth clients (e.g. Claude connectors) the
// authenticated user has granted access to: their grants, plus any client
// Hydra still holds a consent session for without an active grant (one
// connected before grants existed), so every connection can be disconnected.
// If Hydra cannot be reached the grants alone are returned.
//
// @Summary     List connected apps
// @Description List the OAuth clients the user has granted access to, with their scopes
// @Tags        oauth
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} ConnectedAppsResponse
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /oauth/clients [get]
func (h *OAuthHandler) ListConnectedApps(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}

	grants, err := h.grants.List(userID)
	if err != nil {
		respondWithError(c, err)
		return
	}

	limits := h.writeLimitsFor(userID, grants)

	out := make([]ConnectedAppResponse, 0, len(grants))
	listed := make(map[string]int, len(grants))
	for i := range grants {
		g := &grants[i]
		listed[g.ClientID] = len(out)
		app := ConnectedAppResponse{
			ClientID:   g.ClientID,
			ClientName: g.ClientName,
			ClientHost: g.ClientHost,
			Scopes:     g.Approved(),
			AutoAccept: g.AutoAccept,
			CreatedAt:  &g.CreatedAt,
			LastUsedAt: g.LastUsedAt,
		}
		if hasWriteScope(app.Scopes) {
			app.WriteLimits = limits
		}
		out = append(out, app)
	}

	sessions, err := h.hydra.ListConsentSessions(c.Request.Context(), userID)
	if err != nil {
		logger.Get().Warnw("connected apps: could not list Hydra consent sessions", "error", err)
	}
	for i := range sessions {
		s := &sessions[i]
		client := s.ConsentRequest.Client
		if client.ClientID == "" {
			continue
		}
		if i, ok := listed[client.ClientID]; ok {
			if out[i].Legacy {
				out[i].Scopes = mergeScopes(out[i].Scopes, apiScopes(s.GrantScope))
			}
			continue
		}
		listed[client.ClientID] = len(out)
		out = append(out, ConnectedAppResponse{
			ClientID:   client.ClientID,
			ClientName: client.ClientName,
			Scopes:     apiScopes(s.GrantScope),
			Legacy:     true,
		})
	}

	c.JSON(http.StatusOK, ConnectedAppsResponse{Clients: out})
}

// mergeScopes returns a with the members of b it lacks appended.
func mergeScopes(a, b []string) []string {
	for _, s := range b {
		if !slices.Contains(a, s) {
			a = append(a, s)
		}
	}
	return a
}

// DisconnectApp revokes the authenticated user's access for an OAuth client.
// Hydra revokes the client's consent sessions first, which invalidates its
// refresh tokens; the local grant (if any) is then marked revoked, which
// blocks MCP writes at once and reads within a minute (the MCP server caches
// the grant check that long). Requests already under way may finish. If Hydra fails nothing is
// changed, so the user can simply retry.
//
// Hydra is called even without a grant row: a client connected before grants
// existed (migration 000033 dropped the old trust table) has a live Hydra
// session and nothing else. Revoking only ever affects the caller's own
// sessions, so the call is idempotent and safe for any client_id.
//
// @Summary     Disconnect an app
// @Description Revoke an OAuth client's access and stop it from auto-connecting
// @Tags        oauth
// @Security    BearerAuth
// @Param       client_id path string true "OAuth client ID"
// @Success     204 "Disconnected"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /oauth/clients/{client_id} [delete]
func (h *OAuthHandler) DisconnectApp(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	clientID := c.Param("client_id")
	if clientID == "" {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, "client_id is required"))
		return
	}

	grant, err := h.grants.Get(userID, clientID)
	if errors.Is(err, apperrors.ErrOAuthClientNotFound) {
		grant, err = nil, nil
	}
	if err != nil {
		respondWithError(c, err)
		return
	}

	if err := h.hydra.RevokeConsentSessions(c.Request.Context(), userID, clientID); err != nil {
		respondWithError(c, err)
		return
	}
	// The handler checks IsRevoked itself (Revoke would also refuse) so an
	// already-revoked grant still gets the Hydra revoke above, as a retry.
	if grant != nil && !grant.IsRevoked() {
		if err := h.grants.Revoke(userID, clientID); err != nil {
			respondWithError(c, err)
			return
		}
	}

	h.auditService.Log(userID, "OAUTH_CLIENT_REVOKED", "oauth_client", clientID, c.ClientIP(),
		map[string]interface{}{"client_name": grantClientName(grant), "had_grant": grant != nil})
	c.Status(http.StatusNoContent)
}

// hasWriteScope reports whether any scope is a write:* scope.
func hasWriteScope(scopes []string) bool {
	for _, s := range scopes {
		if strings.HasPrefix(s, "write:") {
			return true
		}
	}
	return false
}

// writeLimitsFor returns the MCP write limits to show on grants that have a
// write:* scope, or nil when no grant has one, the limits are not configured
// or the user's reporting currency can't be read (the screen then shows none
// rather than a wrong currency).
func (h *OAuthHandler) writeLimitsFor(userID string, grants []models.OAuthClientGrant) *WriteLimitsResponse {
	if h.writeLimits == nil {
		return nil
	}
	hasWrite := false
	for i := range grants {
		if hasWriteScope(grants[i].Approved()) {
			hasWrite = true
			break
		}
	}
	if !hasWrite {
		return nil
	}
	user, err := h.userService.GetUserByID(userID)
	if err != nil {
		logger.Get().Warnw("connected apps: could not read the user's currency for write limits", "error", err)
		return nil
	}
	return &WriteLimitsResponse{
		MaxAmount: h.writeLimits.MaxAmount,
		Currency:  user.BaseCurrency,
		PerMinute: h.writeLimits.PerMinute,
		PerDay:    h.writeLimits.PerDay,
	}
}

// GetAppActivity lists the changes one connected app made to the user's data,
// newest first. The summary of each row is plain text built by the server;
// clients must not render it as HTML.
//
// @Summary     Changes made by one app
// @Description Page through the changes a connected app made, newest first
// @Tags        oauth
// @Produce     json
// @Security    BearerAuth
// @Param       client_id path string true "OAuth client ID"
// @Param       page query int false "Page number" default(1)
// @Param       page_size query int false "Items per page (max 100)" default(20)
// @Success     200 {object} pagination.PageResponse[services.AppActivityItem]
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /oauth/clients/{client_id}/activity [get]
func (h *OAuthHandler) GetAppActivity(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	if h.activity == nil {
		respondWithError(c, apperrors.ErrInternalServer)
		return
	}
	var result *pagination.PageResponse[services.AppActivityItem]
	result, err = h.activity.ListByClient(userID, c.Param("client_id"), page)
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetAllAppActivity lists the changes made by all of the user's connected
// apps, newest first, each tagged with the app's id and name.
//
// @Summary     Changes made by all apps
// @Description Page through the changes all connected apps made, newest first
// @Tags        oauth
// @Produce     json
// @Security    BearerAuth
// @Param       page query int false "Page number" default(1)
// @Param       page_size query int false "Items per page (max 100)" default(20)
// @Success     200 {object} pagination.PageResponse[services.AllAppActivityItem]
// @Failure     400 {object} ErrorResponse "Invalid input"
// @Failure     401 {object} ErrorResponse "Unauthorized"
// @Router      /oauth/clients/activity [get]
func (h *OAuthHandler) GetAllAppActivity(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		respondWithError(c, err)
		return
	}
	var page pagination.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		respondWithError(c, apperrors.WithMessage(apperrors.ErrInvalidInput, err.Error()))
		return
	}
	if h.activity == nil {
		respondWithError(c, apperrors.ErrInternalServer)
		return
	}
	var result *pagination.PageResponse[services.AllAppActivityItem]
	result, err = h.activity.ListAll(userID, page)
	if err != nil {
		respondWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// grantClientName returns the grant's client name, or "" without a grant.
func grantClientName(g *models.OAuthClientGrant) string {
	if g == nil {
		return ""
	}
	return g.ClientName
}
