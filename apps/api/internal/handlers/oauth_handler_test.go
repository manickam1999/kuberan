package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/hydra"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// --- mock Hydra admin client ---

type mockHydraAdmin struct {
	getLoginFn          func(ctx context.Context, challenge string) (*hydra.LoginRequest, error)
	acceptLoginFn       func(ctx context.Context, challenge string, p hydra.AcceptLoginParams) (string, error)
	getConsentFn        func(ctx context.Context, challenge string) (*hydra.ConsentRequest, error)
	acceptConsentFn     func(ctx context.Context, challenge string, p hydra.AcceptConsentParams) (string, error)
	rejectConsentFn     func(ctx context.Context, challenge, reason string) (string, error)
	rejectLoginFn       func(ctx context.Context, challenge, reason string) (string, error)
	createClientFn      func(ctx context.Context, in hydra.OAuth2ClientCreate) (*hydra.OAuth2Client, error)
	lastAcceptLogin     hydra.AcceptLoginParams
	lastAcceptConsent   hydra.AcceptConsentParams
	lastCreateClient    hydra.OAuth2ClientCreate
	lastRejectReason    string
	acceptConsentCalled bool
	rejectConsentCalled bool
	rejectLoginCalled   bool
	createClientCalled  bool
	revokeErr           error
	sessions            []hydra.ConsentSession
	sessionsErr         error
	revokeCalled        bool
	lastRevokeSubject   string
	lastRevokeClient    string
}

func (m *mockHydraAdmin) GetLoginRequest(ctx context.Context, challenge string) (*hydra.LoginRequest, error) {
	if m.getLoginFn != nil {
		return m.getLoginFn(ctx, challenge)
	}
	return &hydra.LoginRequest{Challenge: challenge}, nil
}

func (m *mockHydraAdmin) AcceptLogin(ctx context.Context, challenge string, p hydra.AcceptLoginParams) (string, error) {
	m.lastAcceptLogin = p
	if m.acceptLoginFn != nil {
		return m.acceptLoginFn(ctx, challenge, p)
	}
	return "https://hydra/redirect/login", nil
}

func (m *mockHydraAdmin) RejectLogin(ctx context.Context, challenge, reason string) (string, error) {
	m.rejectLoginCalled = true
	m.lastRejectReason = reason
	if m.rejectLoginFn != nil {
		return m.rejectLoginFn(ctx, challenge, reason)
	}
	return "https://hydra/redirect/login-denied", nil
}

func (m *mockHydraAdmin) GetConsentRequest(ctx context.Context, challenge string) (*hydra.ConsentRequest, error) {
	if m.getConsentFn != nil {
		return m.getConsentFn(ctx, challenge)
	}
	return &hydra.ConsentRequest{Challenge: challenge}, nil
}

func (m *mockHydraAdmin) AcceptConsent(ctx context.Context, challenge string, p hydra.AcceptConsentParams) (string, error) {
	m.acceptConsentCalled = true
	m.lastAcceptConsent = p
	if m.acceptConsentFn != nil {
		return m.acceptConsentFn(ctx, challenge, p)
	}
	return "https://hydra/redirect/consent", nil
}

func (m *mockHydraAdmin) RejectConsent(ctx context.Context, challenge, reason string) (string, error) {
	m.rejectConsentCalled = true
	m.lastRejectReason = reason
	if m.rejectConsentFn != nil {
		return m.rejectConsentFn(ctx, challenge, reason)
	}
	return "https://hydra/redirect/consent-denied", nil
}

func (m *mockHydraAdmin) GetClient(context.Context, string) (*hydra.OAuth2Client, error) {
	return &hydra.OAuth2Client{}, nil
}

func (m *mockHydraAdmin) CreateClient(ctx context.Context, in hydra.OAuth2ClientCreate) (*hydra.OAuth2Client, error) {
	m.createClientCalled = true
	m.lastCreateClient = in
	if m.createClientFn != nil {
		return m.createClientFn(ctx, in)
	}
	return &hydra.OAuth2Client{
		ClientID:                "generated-id",
		ClientName:              in.ClientName,
		RedirectURIs:            in.RedirectURIs,
		Scope:                   in.Scope,
		GrantTypes:              in.GrantTypes,
		ResponseTypes:           in.ResponseTypes,
		TokenEndpointAuthMethod: in.TokenEndpointAuthMethod,
	}, nil
}

func (m *mockHydraAdmin) ListConsentSessions(context.Context, string) ([]hydra.ConsentSession, error) {
	return m.sessions, m.sessionsErr
}

func (m *mockHydraAdmin) RevokeAllSessions(context.Context, string) error { return nil }

func (m *mockHydraAdmin) RevokeConsentSessions(_ context.Context, subject, clientID string) error {
	m.revokeCalled = true
	m.lastRevokeSubject = subject
	m.lastRevokeClient = clientID
	return m.revokeErr
}

// --- mock grant service ---

type mockGrantService struct {
	grant      *models.OAuthClientGrant
	getErr     error
	recordErr  error
	revokeErr  error
	recorded   *services.RecordConsentInput
	revokedFor string
}

func (m *mockGrantService) Get(_, _ string) (*models.OAuthClientGrant, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.grant == nil {
		return nil, apperrors.ErrOAuthClientNotFound
	}
	return m.grant, nil
}

func (m *mockGrantService) RecordConsent(in services.RecordConsentInput) (*models.OAuthClientGrant, error) {
	m.recorded = &in
	if m.recordErr != nil {
		return nil, m.recordErr
	}
	return &models.OAuthClientGrant{UserID: in.UserID, ClientID: in.ClientID}, nil
}

func (m *mockGrantService) List(_ string) ([]models.OAuthClientGrant, error) {
	if m.grant == nil || m.grant.IsRevoked() {
		return nil, nil
	}
	return []models.OAuthClientGrant{*m.grant}, nil
}

func (m *mockGrantService) Revoke(_, clientID string) error {
	m.revokedFor = clientID
	return m.revokeErr
}

func (m *mockGrantService) Authorize(_, _, _ string) error { return nil }

func (m *mockGrantService) RevokeAll(_ string) (int64, error) { return 0, nil }

// rememberedGrant returns an auto-accepting grant for client "known".
func rememberedGrant(approved, reviewed string) *models.OAuthClientGrant {
	return &models.OAuthClientGrant{
		UserID: "user-7", ClientID: "known", ClientName: "Claude",
		ApprovedScopes: approved, ReviewedScopes: reviewed, AutoAccept: true,
	}
}

// --- test helpers ---

const testScopes = "read:accounts read:transactions read:budgets"

// testScopesWithWrite is the registered scope set when MCP writes are enabled.
const testScopesWithWrite = testScopes + " write:transactions"

const testResourceURL = "https://mcp.example"

func setupOAuthRouter(handler *OAuthHandler) *gin.Engine {
	r := gin.New()
	r.POST("/oauth/login", handler.Login)
	r.POST("/oauth/login/reject", handler.RejectLogin)
	r.GET("/oauth/consent", handler.GetConsent)
	r.POST("/oauth/consent/accept", handler.AcceptConsent)
	r.POST("/oauth/consent/reject", handler.RejectConsent)
	r.GET("/oauth/clients", injectUserID("user-7"), handler.ListConnectedApps)
	r.DELETE("/oauth/clients/:client_id", injectUserID("user-7"), handler.DisconnectApp)
	return r
}

// --- Login ---

func TestOAuthHandler_Login(t *testing.T) {
	t.Run("accepts login with subject = user ID and returns redirect_to", func(t *testing.T) {
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "user-42"}}, nil
			},
		}
		admin := &mockHydraAdmin{}
		handler := NewOAuthHandler(admin, userSvc, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login",
			`{"login_challenge":"lc1","email":"a@b.com","password":"secret123"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["redirect_to"] != "https://hydra/redirect/login" {
			t.Errorf("unexpected redirect_to: %v", result["redirect_to"])
		}
		if admin.lastAcceptLogin.Subject != "user-42" {
			t.Errorf("expected subject user-42, got %q", admin.lastAcceptLogin.Subject)
		}
	})

	t.Run("returns error when credentials are rejected", func(t *testing.T) {
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return nil, apperrors.ErrInvalidCredentials
			},
		}
		admin := &mockHydraAdmin{}
		handler := NewOAuthHandler(admin, userSvc, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login",
			`{"login_challenge":"lc1","email":"a@b.com","password":"wrong"}`)

		if rec.Code != apperrors.ErrInvalidCredentials.StatusCode {
			t.Fatalf("expected %d, got %d: %s", apperrors.ErrInvalidCredentials.StatusCode, rec.Code, rec.Body.String())
		}
		if admin.lastAcceptLogin.Subject != "" {
			t.Error("AcceptLogin must not be called on failed credentials")
		}
	})

	t.Run("rejects missing login_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login", `{"email":"a@b.com","password":"secret123"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		assertErrorCode(t, result, apperrors.ErrInvalidInput.Code)
		// The binding error (Go struct and validator names) is not echoed.
		if body := rec.Body.String(); strings.Contains(body, "oauthLoginRequest") || strings.Contains(body, "binding") {
			t.Errorf("binding details leaked to the client: %s", body)
		}
	})

	t.Run("checks the challenge with Hydra before the credentials", func(t *testing.T) {
		attempted := false
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				attempted = true
				return &models.User{Base: models.Base{ID: "user-42"}}, nil
			},
		}
		admin := &mockHydraAdmin{getLoginFn: func(context.Context, string) (*hydra.LoginRequest, error) {
			return nil, apperrors.WithMessage(apperrors.ErrNotFound, "hydra challenge or client not found")
		}}
		handler := NewOAuthHandler(admin, userSvc, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login",
			`{"login_challenge":"bogus","email":"a@b.com","password":"secret123"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for an unknown challenge, got %d: %s", rec.Code, rec.Body.String())
		}
		if attempted {
			t.Error("credentials must not be checked for an unknown challenge")
		}
		if admin.lastAcceptLogin.Subject != "" {
			t.Error("AcceptLogin must not be called for an unknown challenge")
		}
	})
}

// --- RejectLogin ---

func TestOAuthHandler_RejectLogin(t *testing.T) {
	t.Run("rejects the challenge via Hydra without accepting any login", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login/reject", `{"login_challenge":"lc1"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if result := parseJSON(t, rec); result["redirect_to"] != "https://hydra/redirect/login-denied" {
			t.Errorf("unexpected redirect_to: %v", result["redirect_to"])
		}
		if !admin.rejectLoginCalled {
			t.Error("expected Hydra RejectLogin to be called")
		}
		if admin.lastAcceptLogin.Subject != "" {
			t.Error("AcceptLogin must not be called when cancelling")
		}
	})

	t.Run("rejects missing login_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login/reject", `{}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

// --- GetConsent ---

// consentFor returns a Hydra mock serving one consent request for user-7 and
// client "known" with the given requested scopes.
func consentFor(requested ...string) *mockHydraAdmin {
	return &mockHydraAdmin{
		getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
			return &hydra.ConsentRequest{
				Challenge:      challenge,
				Subject:        "user-7",
				Client:         hydra.OAuth2Client{ClientID: "known", ClientName: "Claude"},
				RequestedScope: requested,
			}, nil
		},
	}
}

func TestConsentRedirectTarget(t *testing.T) {
	tests := []struct {
		name       string
		requestURL string
		registered []string
		want       redirectTarget
	}{
		{
			"uses the redirect_uri of this request, not the first registered",
			"https://auth.example/oauth2/auth?client_id=c&redirect_uri=https%3A%2F%2Fevil.example%2Fcb&response_type=code",
			[]string{"https://claude.ai/cb", "https://evil.example/cb"},
			redirectTarget{Scheme: "https", Host: "evil.example"},
		},
		{
			"falls back to the only registered URI",
			"https://auth.example/oauth2/auth?client_id=c&response_type=code",
			[]string{"https://Claude.AI/cb"},
			redirectTarget{Scheme: "https", Host: "claude.ai"},
		},
		{
			"unknown when the request names none and several are registered",
			"https://auth.example/oauth2/auth?client_id=c",
			[]string{"https://a.example/cb", "https://b.example/cb"},
			redirectTarget{},
		},
		{
			"loopback keeps the port",
			"https://auth.example/oauth2/auth?redirect_uri=http%3A%2F%2F127.0.0.1%3A33418%2Fcb",
			nil,
			redirectTarget{Scheme: "http", Host: "127.0.0.1:33418"},
		},
		{
			"private-use scheme has no host",
			"https://auth.example/oauth2/auth?redirect_uri=com.example.app%3A%2Fcb",
			nil,
			redirectTarget{Scheme: "com.example.app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consentRedirectTarget(&hydra.ConsentRequest{
				RequestURL: tt.requestURL,
				Client:     hydra.OAuth2Client{RedirectURIs: tt.registered},
			})
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOAuthHandler_GetConsentShowsRedirectAndVerification(t *testing.T) {
	admin := &mockHydraAdmin{getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
		return &hydra.ConsentRequest{
			Challenge: challenge, Subject: "user-7", RequestedScope: []string{"read:accounts"},
			RequestURL: "https://auth.example/oauth2/auth?redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback",
			Client: hydra.OAuth2Client{ClientID: "dcr-1", ClientName: "Claude",
				RedirectURIs: []string{"https://other.example/cb", "https://claude.ai/api/mcp/auth_callback"}},
		}, nil
	}}

	for _, tt := range []struct {
		pinned       string
		wantVerified bool
	}{{"", false}, {"someone-else", false}, {"dcr-1", true}} {
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockGrantService{}, &mockAuditService{},
			testScopes, testResourceURL, WithPinnedClientID(tt.pinned))
		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["redirect_host"] != "claude.ai" || result["redirect_scheme"] != "https" {
			t.Errorf("unexpected redirect target: %v / %v", result["redirect_scheme"], result["redirect_host"])
		}
		if result["client_verified"] != tt.wantVerified {
			t.Errorf("pinned %q: client_verified = %v, want %v", tt.pinned, result["client_verified"], tt.wantVerified)
		}
		if _, ok := result["redirect_uris"]; ok {
			t.Error("the registered redirect URI list is no longer sent")
		}
	}
}

func TestOAuthHandler_GetConsent(t *testing.T) {
	t.Run("auto-accepts a remembered client for reviewed scopes", func(t *testing.T) {
		admin := consentFor("read:accounts", "offline_access")
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "GET", "/oauth/consent?consent_challenge=cc1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["redirect_to"] != "https://hydra/redirect/consent" {
			t.Errorf("expected redirect_to for remembered client, got: %v", result)
		}
		if !admin.acceptConsentCalled {
			t.Error("expected AcceptConsent to be called for a remembered client")
		}
		if got := admin.lastAcceptConsent.GrantScope; !slices.Equal(got, []string{"read:accounts", "offline_access"}) {
			t.Errorf("unexpected granted scopes: %v", got)
		}
		// The MCP resource URL must always be stamped into the audience so the
		// RS validator accepts the token, even though the client requested none.
		aud := admin.lastAcceptConsent.GrantAudience
		if len(aud) != 1 || aud[0] != testResourceURL {
			t.Errorf("expected granted audience [%s], got: %v", testResourceURL, aud)
		}
	})

	t.Run("auto-accept grants only the approved subset of reviewed scopes", func(t *testing.T) {
		admin := consentFor("read:accounts", "write:transactions")
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts write:transactions")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopesWithWrite, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")

		if rec.Code != http.StatusOK || !admin.acceptConsentCalled {
			t.Fatalf("expected auto-accept, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := admin.lastAcceptConsent.GrantScope; !slices.Equal(got, []string{"read:accounts"}) {
			t.Errorf("a declined write scope must not be granted on auto-accept, got %v", got)
		}
	})

	t.Run("shows the consent screen when a remembered client requests a new scope", func(t *testing.T) {
		admin := consentFor("read:accounts", "write:transactions")
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopesWithWrite, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if admin.acceptConsentCalled {
			t.Fatal("a scope the user never reviewed must not be auto-accepted")
		}
		result := parseJSON(t, rec)
		if !slices.Equal(toStrings(result["requested_scopes"]), []string{"read:accounts", "write:transactions"}) {
			t.Errorf("unexpected requested_scopes: %v", result["requested_scopes"])
		}
		if !slices.Equal(toStrings(result["previously_approved"]), []string{"read:accounts"}) {
			t.Errorf("unexpected previously_approved: %v", result["previously_approved"])
		}
	})

	t.Run("does not auto-accept a client the user did not ask to remember", func(t *testing.T) {
		admin := consentFor("read:accounts")
		grant := rememberedGrant("read:accounts", "read:accounts")
		grant.AutoAccept = false
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockGrantService{grant: grant}, &mockAuditService{}, testScopes, testResourceURL)

		doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")

		if admin.acceptConsentCalled {
			t.Error("AcceptConsent must not be called without remember")
		}
	})

	t.Run("does not auto-accept a revoked grant", func(t *testing.T) {
		admin := consentFor("read:accounts")
		grant := rememberedGrant("read:accounts", "read:accounts")
		revokedAt := time.Now()
		grant.RevokedAt = &revokedAt
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockGrantService{grant: grant}, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")

		if admin.acceptConsentCalled {
			t.Error("a revoked grant must not auto-accept")
		}
		if got := toStrings(parseJSON(t, rec)["previously_approved"]); len(got) != 0 {
			t.Errorf("a revoked grant must not pre-tick scopes, got %v", got)
		}
	})

	t.Run("preserves a requested audience while keeping the resource URL unique", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:                    challenge,
					Subject:                      "user-7",
					Client:                       hydra.OAuth2Client{ClientID: "known", ClientName: "Claude"},
					RequestedScope:               []string{"read:accounts"},
					RequestedAccessTokenAudience: []string{"https://other.example", testResourceURL},
				}, nil
			},
		}
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "GET", "/oauth/consent?consent_challenge=cc1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		aud := admin.lastAcceptConsent.GrantAudience
		// Requested audiences pass through and the resource URL is not duplicated.
		if len(aud) != 2 || aud[0] != "https://other.example" || aud[1] != testResourceURL {
			t.Errorf("unexpected granted audience: %v", aud)
		}
	})

	t.Run("returns consent details for an unknown client without accepting", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge: challenge,
					Subject:   "user-7",
					Client: hydra.OAuth2Client{
						ClientID:     "unknown",
						ClientName:   "Mystery",
						RedirectURIs: []string{"https://evil.example/cb"},
					},
					RequestedScope: []string{"read:accounts", "read:secrets"},
				}, nil
			},
		}
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "GET", "/oauth/consent?consent_challenge=cc1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if admin.acceptConsentCalled {
			t.Error("AcceptConsent must not be called for an unknown client")
		}
		result := parseJSON(t, rec)
		if result["redirect_to"] != nil {
			t.Error("unknown client must not return redirect_to")
		}
		if !slices.Equal(toStrings(result["requested_scopes"]), []string{"read:accounts"}) {
			t.Errorf("expected only grantable scopes in payload, got: %v", result["requested_scopes"])
		}
	})

	t.Run("propagates grant lookup failures", func(t *testing.T) {
		grants := &mockGrantService{getErr: apperrors.ErrInternalServer}
		handler := NewOAuthHandler(consentFor("read:accounts"), &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("rejects missing consent_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "GET", "/oauth/consent", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

// toStrings converts a decoded JSON array into a string slice.
func toStrings(v interface{}) []string {
	raw, _ := v.([]interface{})
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// --- AcceptConsent ---

func TestOAuthHandler_AcceptConsent(t *testing.T) {
	accept := func(t *testing.T, scopes string, requested []string, body string) (*mockHydraAdmin, *mockGrantService, *httptest.ResponseRecorder) {
		t.Helper()
		admin := consentFor(requested...)
		grants := &mockGrantService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, scopes, testResourceURL)
		rec := doRequest(setupOAuthRouter(handler), "POST", "/oauth/consent/accept", body)
		return admin, grants, rec
	}

	t.Run("grants consent and records a remembered grant", func(t *testing.T) {
		admin, grants, rec := accept(t, testScopes, []string{"read:accounts", "read:transactions"},
			`{"consent_challenge":"cc1","remember_client":true,"granted_scopes":["read:accounts","read:transactions"]}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if grants.recorded == nil || grants.recorded.UserID != "user-7" || grants.recorded.ClientID != "known" {
			t.Fatalf("expected a grant for user-7/known, got %+v", grants.recorded)
		}
		if !grants.recorded.AutoAccept {
			t.Error("expected remember_client to set auto-accept")
		}
		if !admin.lastAcceptConsent.Remember {
			t.Error("expected Hydra consent remember flag to be set")
		}
	})

	t.Run("records a grant without auto-accept when remember is false", func(t *testing.T) {
		_, grants, rec := accept(t, testScopes, []string{"read:accounts"},
			`{"consent_challenge":"cc1","remember_client":false,"granted_scopes":["read:accounts"]}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if grants.recorded == nil || grants.recorded.AutoAccept {
			t.Errorf("expected a non-auto-accept grant, got %+v", grants.recorded)
		}
	})

	t.Run("grants only the ticked scopes and records what was reviewed", func(t *testing.T) {
		admin, grants, rec := accept(t, testScopesWithWrite,
			[]string{"read:accounts", "read:transactions", "write:transactions", "offline_access"},
			`{"consent_challenge":"cc1","granted_scopes":["read:accounts"]}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := admin.lastAcceptConsent.GrantScope; !slices.Equal(got, []string{"read:accounts", "offline_access"}) {
			t.Errorf("granted %v, want [read:accounts offline_access]", got)
		}
		if !slices.Equal(grants.recorded.Approved, []string{"read:accounts"}) {
			t.Errorf("unexpected approved: %v", grants.recorded.Approved)
		}
		if !slices.Equal(grants.recorded.Reviewed, []string{"read:accounts", "read:transactions", "write:transactions"}) {
			t.Errorf("unexpected reviewed: %v", grants.recorded.Reviewed)
		}
	})

	t.Run("grants a write scope only when ticked", func(t *testing.T) {
		admin, _, rec := accept(t, testScopesWithWrite, []string{"read:accounts", "write:transactions"},
			`{"consent_challenge":"cc1","granted_scopes":["read:accounts","write:transactions"]}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := admin.lastAcceptConsent.GrantScope; !slices.Equal(got, []string{"read:accounts", "write:transactions"}) {
			t.Errorf("granted %v", got)
		}
	})

	t.Run("omitted granted_scopes grants only read scopes", func(t *testing.T) {
		admin, _, rec := accept(t, testScopesWithWrite, []string{"read:accounts", "write:transactions"},
			`{"consent_challenge":"cc1"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := admin.lastAcceptConsent.GrantScope; !slices.Equal(got, []string{"read:accounts"}) {
			t.Errorf("an outdated page must never grant write scopes, got %v", got)
		}
	})

	t.Run("ignores ticked scopes the client did not request or that are not registered", func(t *testing.T) {
		admin, _, _ := accept(t, testScopes, []string{"read:accounts", "write:transactions"},
			`{"consent_challenge":"cc1","granted_scopes":["read:accounts","read:budgets","write:transactions"]}`)

		if got := admin.lastAcceptConsent.GrantScope; !slices.Equal(got, []string{"read:accounts"}) {
			t.Errorf("granted %v, want [read:accounts]", got)
		}
	})

	t.Run("does not accept in Hydra when the grant cannot be recorded", func(t *testing.T) {
		admin := consentFor("read:accounts")
		grants := &mockGrantService{recordErr: apperrors.ErrInternalServer}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "POST", "/oauth/consent/accept",
			`{"consent_challenge":"cc1","granted_scopes":["read:accounts"]}`)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if admin.acceptConsentCalled {
			t.Error("Hydra must not issue tokens without a recorded grant")
		}
	})

	t.Run("rejects missing consent_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/accept", `{"remember_client":true}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

func TestOAuthHandler_ConsentGrantsOfflineAccess(t *testing.T) {
	grantFor := func(t *testing.T, clientScope string, requested []string) []string {
		t.Helper()
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:      challenge,
					Client:         hydra.OAuth2Client{ClientID: "cid", Scope: clientScope},
					RequestedScope: requested,
				}, nil
			},
		}
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		rec := doRequest(setupOAuthRouter(handler), "POST", "/oauth/consent/accept",
			`{"consent_challenge":"cc1","remember_client":false}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		return admin.lastAcceptConsent.GrantScope
	}

	t.Run("adds offline_access for a client registered with it", func(t *testing.T) {
		got := grantFor(t, "read:accounts openid offline offline_access", []string{"read:accounts"})
		if !slices.Equal(got, []string{"read:accounts", "offline_access"}) {
			t.Errorf("granted %v, want [read:accounts offline_access]", got)
		}
	})

	t.Run("does not duplicate a requested offline scope", func(t *testing.T) {
		got := grantFor(t, "read:accounts offline offline_access", []string{"read:accounts", "offline"})
		if !slices.Equal(got, []string{"read:accounts", "offline"}) {
			t.Errorf("granted %v, want [read:accounts offline]", got)
		}
	})

	t.Run("skips clients not registered for offline_access", func(t *testing.T) {
		got := grantFor(t, "read:accounts", []string{"read:accounts"})
		if !slices.Equal(got, []string{"read:accounts"}) {
			t.Errorf("granted %v, want [read:accounts]", got)
		}
	})
}

// --- RejectConsent ---

func TestOAuthHandler_RejectConsent(t *testing.T) {
	t.Run("rejects the challenge via Hydra and never grants scope or trust", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:      challenge,
					Subject:        "user-7",
					Client:         hydra.OAuth2Client{ClientID: "cid-9", ClientName: "Rogue"},
					RequestedScope: []string{"read:accounts"},
				}, nil
			},
		}
		grants := &mockGrantService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/reject", `{"consent_challenge":"cc1"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if result := parseJSON(t, rec); result["redirect_to"] != "https://hydra/redirect/consent-denied" {
			t.Errorf("unexpected redirect_to: %v", result["redirect_to"])
		}
		if !admin.rejectConsentCalled {
			t.Error("expected Hydra RejectConsent to be called")
		}
		if admin.acceptConsentCalled {
			t.Error("AcceptConsent must not be called when denying")
		}
		if grants.recorded != nil {
			t.Error("no grant may be recorded when denying consent")
		}
	})

	t.Run("rejects missing consent_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/reject", `{}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

// --- Connected apps ---

func TestOAuthHandler_ListConnectedApps(t *testing.T) {
	t.Run("lists the user's grants with their approved scopes", func(t *testing.T) {
		grants := &mockGrantService{grant: rememberedGrant("read:accounts write:transactions", "read:accounts write:transactions")}
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/clients", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		clients, _ := parseJSON(t, rec)["clients"].([]interface{})
		if len(clients) != 1 {
			t.Fatalf("expected 1 client, got %v", clients)
		}
		first, _ := clients[0].(map[string]interface{})
		if first["client_id"] != "known" || first["client_name"] != "Claude" {
			t.Errorf("unexpected client: %v", first)
		}
		if !slices.Equal(toStrings(first["scopes"]), []string{"read:accounts", "write:transactions"}) {
			t.Errorf("unexpected scopes: %v", first["scopes"])
		}
	})

	t.Run("adds Hydra-only connections as legacy entries", func(t *testing.T) {
		admin := &mockHydraAdmin{sessions: []hydra.ConsentSession{
			{ConsentRequest: hydra.ConsentSessionRequest{Client: hydra.OAuth2Client{ClientID: "known", ClientName: "Claude"}},
				GrantScope: []string{"read:accounts"}},
			{ConsentRequest: hydra.ConsentSessionRequest{Client: hydra.OAuth2Client{ClientID: "old", ClientName: "Old connector"}},
				GrantScope: []string{"read:accounts", "offline_access"}},
			{ConsentRequest: hydra.ConsentSessionRequest{Client: hydra.OAuth2Client{ClientID: "old", ClientName: "Old connector"}},
				GrantScope: []string{"read:budgets"}},
		}}
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/clients", "")

		clients, _ := parseJSON(t, rec)["clients"].([]interface{})
		if len(clients) != 2 {
			t.Fatalf("expected the grant plus one legacy client, got %v", clients)
		}
		legacy, _ := clients[1].(map[string]interface{})
		if legacy["client_id"] != "old" || legacy["legacy"] != true {
			t.Errorf("unexpected legacy entry: %v", legacy)
		}
		if !slices.Equal(toStrings(legacy["scopes"]), []string{"read:accounts", "read:budgets"}) {
			t.Errorf("legacy scopes must merge sessions and drop protocol scopes, got %v", legacy["scopes"])
		}
		if first, _ := clients[0].(map[string]interface{}); first["legacy"] != nil {
			t.Errorf("a client with a grant must not be marked legacy: %v", first)
		}
	})

	t.Run("still lists grants when Hydra is unreachable", func(t *testing.T) {
		admin := &mockHydraAdmin{sessionsErr: apperrors.ErrInternalServer}
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/clients", "")

		if rec.Code != http.StatusOK || len(parseJSON(t, rec)["clients"].([]interface{})) != 1 {
			t.Fatalf("expected the grant to be listed, got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns an empty list, not null", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockGrantService{}, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/clients", "")

		if body := rec.Body.String(); body != `{"clients":[]}` {
			t.Errorf("unexpected body: %s", body)
		}
	})
}

func TestOAuthHandler_DisconnectApp(t *testing.T) {
	t.Run("revokes in Hydra, then the local grant, and audits", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		audit := &recordingAuditService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, audit, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "DELETE", "/oauth/clients/known", "")

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if !admin.revokeCalled || admin.lastRevokeSubject != "user-7" || admin.lastRevokeClient != "known" {
			t.Errorf("expected Hydra revoke for user-7/known, got %v %q %q", admin.revokeCalled, admin.lastRevokeSubject, admin.lastRevokeClient)
		}
		if grants.revokedFor != "known" {
			t.Errorf("expected the local grant to be revoked, got %q", grants.revokedFor)
		}
		if audit.action != "OAUTH_CLIENT_REVOKED" || audit.resID != "known" {
			t.Errorf("unexpected audit: %+v", audit)
		}
	})

	t.Run("leaves the grant untouched when Hydra fails", func(t *testing.T) {
		admin := &mockHydraAdmin{revokeErr: apperrors.ErrInternalServer}
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "DELETE", "/oauth/clients/known", "")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
		if grants.revokedFor != "" {
			t.Error("the local grant must stay active so the user can retry")
		}
	})

	t.Run("revokes in Hydra even without a grant (a pre-grant connection)", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		grants := &mockGrantService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "DELETE", "/oauth/clients/legacy-client", "")

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if !admin.revokeCalled || admin.lastRevokeClient != "legacy-client" || admin.lastRevokeSubject != "user-7" {
			t.Errorf("expected a Hydra revoke for user-7/legacy-client, got %v %q %q", admin.revokeCalled, admin.lastRevokeSubject, admin.lastRevokeClient)
		}
		if grants.revokedFor != "" {
			t.Error("there is no grant to revoke")
		}
	})

	t.Run("an already revoked grant still retries the Hydra revoke", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		grant := rememberedGrant("read:accounts", "read:accounts")
		revokedAt := time.Now()
		grant.RevokedAt = &revokedAt
		grants := &mockGrantService{grant: grant}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "DELETE", "/oauth/clients/known", "")

		if rec.Code != http.StatusNoContent || !admin.revokeCalled {
			t.Fatalf("expected 204 and a Hydra revoke, got %d revoke=%v", rec.Code, admin.revokeCalled)
		}
		if grants.revokedFor != "" {
			t.Error("an already revoked grant must not be revoked again")
		}
	})

	t.Run("propagates grant lookup failures without calling Hydra", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		grants := &mockGrantService{getErr: apperrors.ErrInternalServer}
		handler := NewOAuthHandler(admin, &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)

		rec := doRequest(setupOAuthRouter(handler), "DELETE", "/oauth/clients/known", "")

		if rec.Code != http.StatusInternalServerError || admin.revokeCalled {
			t.Fatalf("expected 500 and no Hydra call, got %d revoke=%v", rec.Code, admin.revokeCalled)
		}
	})
}

func TestOAuthHandler_AcceptConsentMergesEarlierChoices(t *testing.T) {
	accept := func(t *testing.T, grant *models.OAuthClientGrant, requested []string, ticked string) *mockGrantService {
		t.Helper()
		grants := &mockGrantService{grant: grant}
		handler := NewOAuthHandler(consentFor(requested...), &mockUserService{}, grants, &mockAuditService{}, testScopesWithWrite, testResourceURL)
		rec := doRequest(setupOAuthRouter(handler), "POST", "/oauth/consent/accept",
			`{"consent_challenge":"cc1","granted_scopes":`+ticked+`}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		return grants
	}

	t.Run("keeps approvals for scopes not requested this time", func(t *testing.T) {
		prev := rememberedGrant("read:accounts write:transactions", "read:accounts write:transactions")
		grants := accept(t, prev, []string{"read:budgets"}, `["read:budgets"]`)

		if !slices.Equal(grants.recorded.Approved, []string{"read:budgets", "read:accounts", "write:transactions"}) {
			t.Errorf("approved = %v", grants.recorded.Approved)
		}
		if !slices.Equal(grants.recorded.Reviewed, []string{"read:budgets", "read:accounts", "write:transactions"}) {
			t.Errorf("reviewed = %v", grants.recorded.Reviewed)
		}
	})

	t.Run("unticking a shown scope removes it", func(t *testing.T) {
		prev := rememberedGrant("read:accounts write:transactions", "read:accounts write:transactions")
		grants := accept(t, prev, []string{"read:accounts", "write:transactions"}, `["read:accounts"]`)

		if !slices.Equal(grants.recorded.Approved, []string{"read:accounts"}) {
			t.Errorf("approved = %v, want write:transactions dropped", grants.recorded.Approved)
		}
	})

	t.Run("a revoked grant is not merged", func(t *testing.T) {
		prev := rememberedGrant("read:accounts write:transactions", "read:accounts write:transactions")
		revokedAt := time.Now()
		prev.RevokedAt = &revokedAt
		grants := accept(t, prev, []string{"read:budgets"}, `["read:budgets"]`)

		if !slices.Equal(grants.recorded.Approved, []string{"read:budgets"}) {
			t.Errorf("approved = %v, want only this screen's choice", grants.recorded.Approved)
		}
	})
}

func TestOAuthHandler_AcceptConsentAuditsOneEvent(t *testing.T) {
	audit := &countingAuditService{}
	handler := NewOAuthHandler(consentFor("read:accounts"), &mockUserService{}, &mockGrantService{}, audit, testScopes, testResourceURL)

	rec := doRequest(setupOAuthRouter(handler), "POST", "/oauth/consent/accept",
		`{"consent_challenge":"cc1","remember_client":true,"granted_scopes":["read:accounts"]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(audit.actions) != 1 || audit.actions[0] != "OAUTH_CONSENT_GRANTED" {
		t.Fatalf("expected one OAUTH_CONSENT_GRANTED event, got %v", audit.actions)
	}
	if audit.changes[0]["auto_accept"] != true {
		t.Errorf("the event must record auto_accept, got %v", audit.changes[0])
	}
}

// countingAuditService records every audit call.
type countingAuditService struct {
	actions []string
	changes []map[string]interface{}
}

func (m *countingAuditService) Log(_, action, _, _, _ string, changes map[string]interface{}) {
	m.actions = append(m.actions, action)
	m.changes = append(m.changes, changes)
}
