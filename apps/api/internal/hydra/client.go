package hydra

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	apperrors "kuberan/internal/errors"
)

// AdminClient drives Hydra's login/consent flow and reads client metadata via the
// private admin API. All methods are called server-side only.
type AdminClient interface {
	GetLoginRequest(ctx context.Context, challenge string) (*LoginRequest, error)
	AcceptLogin(ctx context.Context, challenge string, p AcceptLoginParams) (redirectTo string, err error)
	RejectLogin(ctx context.Context, challenge, reason string) (redirectTo string, err error)

	GetConsentRequest(ctx context.Context, challenge string) (*ConsentRequest, error)
	AcceptConsent(ctx context.Context, challenge string, p AcceptConsentParams) (redirectTo string, err error)
	RejectConsent(ctx context.Context, challenge, reason string) (redirectTo string, err error)

	GetClient(ctx context.Context, clientID string) (*OAuth2Client, error)
	CreateClient(ctx context.Context, in OAuth2ClientCreate) (*OAuth2Client, error)

	// ListConsentSessions lists the consent sessions subject has granted, across
	// all clients.
	ListConsentSessions(ctx context.Context, subject string) ([]ConsentSession, error)
	// RevokeConsentSessions revokes every consent subject has given clientID and
	// the refresh tokens issued under it. Access tokens already issued (JWTs,
	// validated offline) stay valid until they expire.
	RevokeConsentSessions(ctx context.Context, subject, clientID string) error
	// RevokeAllSessions revokes every consent subject has given any client
	// (and the refresh tokens issued under them) and subject's login sessions,
	// so no client can mint new tokens and the next authorization must sign in
	// again. Use it when a user is deactivated.
	RevokeAllSessions(ctx context.Context, subject string) error
}

// client is the concrete AdminClient backed by net/http.
type client struct {
	baseURL string
	http    *http.Client
}

// NewAdminClient creates an AdminClient targeting the given Hydra admin base URL
// (e.g. http://hydra:4445). The URL must be reachable only on the private network.
func NewAdminClient(adminURL string) AdminClient {
	return &client{
		baseURL: strings.TrimRight(adminURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// GetLoginRequest fetches a pending login challenge.
func (c *client) GetLoginRequest(ctx context.Context, challenge string) (*LoginRequest, error) {
	if challenge == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "login_challenge is required")
	}
	var out LoginRequest
	if err := c.do(ctx, http.MethodGet,
		"/admin/oauth2/auth/requests/login", url.Values{"login_challenge": {challenge}}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AcceptLogin resolves a login challenge with the authenticated subject.
func (c *client) AcceptLogin(ctx context.Context, challenge string, p AcceptLoginParams) (string, error) {
	if challenge == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "login_challenge is required")
	}
	if p.Subject == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "subject is required")
	}
	body := map[string]any{
		"subject":      p.Subject,
		"remember":     p.Remember,
		"remember_for": p.RememberFor,
	}
	return c.redirect(ctx,
		"/admin/oauth2/auth/requests/login/accept", url.Values{"login_challenge": {challenge}}, body)
}

// RejectLogin denies a login challenge.
func (c *client) RejectLogin(ctx context.Context, challenge, reason string) (string, error) {
	if challenge == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "login_challenge is required")
	}
	body := map[string]any{"error": "access_denied", "error_description": reason}
	return c.redirect(ctx,
		"/admin/oauth2/auth/requests/login/reject", url.Values{"login_challenge": {challenge}}, body)
}

// GetConsentRequest fetches a pending consent challenge.
func (c *client) GetConsentRequest(ctx context.Context, challenge string) (*ConsentRequest, error) {
	if challenge == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "consent_challenge is required")
	}
	var out ConsentRequest
	if err := c.do(ctx, http.MethodGet,
		"/admin/oauth2/auth/requests/consent", url.Values{"consent_challenge": {challenge}}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AcceptConsent grants the requested scopes/audience for a consent challenge.
func (c *client) AcceptConsent(ctx context.Context, challenge string, p AcceptConsentParams) (string, error) {
	if challenge == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "consent_challenge is required")
	}
	body := map[string]any{
		"grant_scope":                 p.GrantScope,
		"grant_access_token_audience": p.GrantAudience,
		"remember":                    p.Remember,
		"remember_for":                p.RememberFor,
	}
	return c.redirect(ctx,
		"/admin/oauth2/auth/requests/consent/accept", url.Values{"consent_challenge": {challenge}}, body)
}

// RejectConsent denies a consent challenge.
func (c *client) RejectConsent(ctx context.Context, challenge, reason string) (string, error) {
	if challenge == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "consent_challenge is required")
	}
	body := map[string]any{"error": "access_denied", "error_description": reason}
	return c.redirect(ctx,
		"/admin/oauth2/auth/requests/consent/reject", url.Values{"consent_challenge": {challenge}}, body)
}

// GetClient reads an OAuth 2.0 client's metadata by ID.
func (c *client) GetClient(ctx context.Context, clientID string) (*OAuth2Client, error) {
	if clientID == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "client_id is required")
	}
	var out OAuth2Client
	if err := c.do(ctx, http.MethodGet,
		"/admin/clients/"+url.PathEscape(clientID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateClient registers a new OAuth 2.0 client via the admin API. The DCR proxy
// is responsible for sanitising in (forcing a public client, restricting grants,
// and capping scopes) before calling this.
func (c *client) CreateClient(ctx context.Context, in OAuth2ClientCreate) (*OAuth2Client, error) {
	var out OAuth2Client
	if err := c.do(ctx, http.MethodPost, "/admin/clients", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListConsentSessions lists subject's consent sessions (the first page of up to
// maxConsentSessions, which is far more clients than one user connects).
func (c *client) ListConsentSessions(ctx context.Context, subject string) ([]ConsentSession, error) {
	if subject == "" {
		return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "subject is required")
	}
	var out []ConsentSession
	if err := c.do(ctx, http.MethodGet, "/admin/oauth2/auth/sessions/consent",
		url.Values{"subject": {subject}, "page_size": {strconv.Itoa(maxConsentSessions)}}, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// maxConsentSessions is the page size requested by ListConsentSessions.
const maxConsentSessions = 250

// RevokeConsentSessions revokes subject's consent sessions for clientID. Hydra
// invalidates the refresh tokens issued under them, so the client cannot mint
// new access tokens; access tokens already issued remain valid until they
// expire. It is idempotent: a subject with no session for the client succeeds.
func (c *client) RevokeConsentSessions(ctx context.Context, subject, clientID string) error {
	if subject == "" || clientID == "" {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "subject and client_id are required")
	}
	err := c.do(ctx, http.MethodDelete, "/admin/oauth2/auth/sessions/consent",
		url.Values{"subject": {subject}, "client": {clientID}}, nil, nil)
	if stderrors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}

// RevokeAllSessions revokes all of subject's consent sessions, across every
// client, and its login sessions. Both calls are idempotent; it stops at the
// first failure so the caller can retry.
func (c *client) RevokeAllSessions(ctx context.Context, subject string) error {
	if subject == "" {
		return apperrors.WithMessage(apperrors.ErrInvalidInput, "subject is required")
	}
	err := c.do(ctx, http.MethodDelete, "/admin/oauth2/auth/sessions/consent",
		url.Values{"subject": {subject}, "all": {"true"}}, nil, nil)
	if err != nil && !stderrors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	err = c.do(ctx, http.MethodDelete, "/admin/oauth2/auth/sessions/login",
		url.Values{"subject": {subject}}, nil, nil)
	if err != nil && !stderrors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	return nil
}

// redirect performs a PUT whose response is a Hydra redirect envelope (the
// accept/reject endpoints).
func (c *client) redirect(ctx context.Context, path string, query url.Values, body any) (string, error) {
	var out redirectResponse
	if err := c.do(ctx, http.MethodPut, path, query, body, &out); err != nil {
		return "", err
	}
	return out.RedirectTo, nil
}

// do executes an admin request, decoding a JSON response into out (may be nil) and
// mapping Hydra HTTP errors onto AppError sentinels.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return apperrors.Wrap(apperrors.ErrInternalServer, err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapStatusError(resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apperrors.Wrap(apperrors.ErrInternalServer, err)
	}
	return nil
}

// mapStatusError translates a non-2xx Hydra admin status into an AppError.
func mapStatusError(status int) error {
	switch status {
	case http.StatusNotFound, http.StatusGone:
		return apperrors.WithMessage(apperrors.ErrNotFound, "hydra challenge or client not found")
	case http.StatusUnauthorized, http.StatusForbidden:
		return apperrors.WithMessage(apperrors.ErrInternalServer, "hydra admin request unauthorized")
	default:
		return apperrors.Wrap(apperrors.ErrInternalServer,
			fmt.Errorf("hydra admin returned status %d", status))
	}
}
