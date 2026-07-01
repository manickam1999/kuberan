package hydra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	return c.redirect(ctx, http.MethodPut,
		"/admin/oauth2/auth/requests/login/accept", url.Values{"login_challenge": {challenge}}, body)
}

// RejectLogin denies a login challenge.
func (c *client) RejectLogin(ctx context.Context, challenge, reason string) (string, error) {
	if challenge == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "login_challenge is required")
	}
	body := map[string]any{"error": "access_denied", "error_description": reason}
	return c.redirect(ctx, http.MethodPut,
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
	return c.redirect(ctx, http.MethodPut,
		"/admin/oauth2/auth/requests/consent/accept", url.Values{"consent_challenge": {challenge}}, body)
}

// RejectConsent denies a consent challenge.
func (c *client) RejectConsent(ctx context.Context, challenge, reason string) (string, error) {
	if challenge == "" {
		return "", apperrors.WithMessage(apperrors.ErrInvalidInput, "consent_challenge is required")
	}
	body := map[string]any{"error": "access_denied", "error_description": reason}
	return c.redirect(ctx, http.MethodPut,
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

// redirect performs a request whose response is a Hydra redirect envelope.
func (c *client) redirect(ctx context.Context, method, path string, query url.Values, body any) (string, error) {
	var out redirectResponse
	if err := c.do(ctx, method, path, query, body, &out); err != nil {
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
