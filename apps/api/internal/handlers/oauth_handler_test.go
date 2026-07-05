package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/hydra"
	"kuberan/internal/models"
)

// --- mock Hydra admin client ---

type mockHydraAdmin struct {
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
}

func (m *mockHydraAdmin) GetLoginRequest(context.Context, string) (*hydra.LoginRequest, error) {
	return &hydra.LoginRequest{}, nil
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

// --- mock trusted client service ---

type mockTrustedClientService struct {
	isTrustedFn func(clientID string) (bool, error)
	trustFn     func(clientID, name string) (*models.TrustedOAuthClient, error)
	trustCalled bool
	trustedID   string
}

func (m *mockTrustedClientService) IsTrusted(clientID string) (bool, error) {
	if m.isTrustedFn != nil {
		return m.isTrustedFn(clientID)
	}
	return false, nil
}

func (m *mockTrustedClientService) Trust(clientID, name string) (*models.TrustedOAuthClient, error) {
	m.trustCalled = true
	m.trustedID = clientID
	if m.trustFn != nil {
		return m.trustFn(clientID, name)
	}
	return &models.TrustedOAuthClient{ClientID: clientID, Name: name}, nil
}

func (m *mockTrustedClientService) ListTrusted() ([]models.TrustedOAuthClient, error) {
	return nil, nil
}

// --- test helpers ---

const testScopes = "read:accounts read:transactions read:budgets"

const testResourceURL = "https://mcp.example"

func setupOAuthRouter(handler *OAuthHandler) *gin.Engine {
	r := gin.New()
	r.POST("/oauth/login", handler.Login)
	r.POST("/oauth/login/reject", handler.RejectLogin)
	r.GET("/oauth/consent", handler.GetConsent)
	r.POST("/oauth/consent/accept", handler.AcceptConsent)
	r.POST("/oauth/consent/reject", handler.RejectConsent)
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
		handler := NewOAuthHandler(admin, userSvc, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
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
		handler := NewOAuthHandler(admin, userSvc, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
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
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login", `{"email":"a@b.com","password":"secret123"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

// --- RejectLogin ---

func TestOAuthHandler_RejectLogin(t *testing.T) {
	t.Run("rejects the challenge via Hydra without accepting any login", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewOAuthHandler(admin, &mockUserService{}, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
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
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/login/reject", `{}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

// --- GetConsent ---

func TestOAuthHandler_GetConsent(t *testing.T) {
	t.Run("auto-accepts a trusted client and returns redirect_to", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:      challenge,
					Client:         hydra.OAuth2Client{ClientID: "known", ClientName: "Claude"},
					RequestedScope: []string{"read:accounts", "offline_access"},
				}, nil
			},
		}
		trusted := &mockTrustedClientService{isTrustedFn: func(string) (bool, error) { return true, nil }}
		handler := NewOAuthHandler(admin, &mockUserService{}, trusted, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "GET", "/oauth/consent?consent_challenge=cc1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["redirect_to"] != "https://hydra/redirect/consent" {
			t.Errorf("expected redirect_to for trusted client, got: %v", result)
		}
		if !admin.acceptConsentCalled {
			t.Error("expected AcceptConsent to be called for a trusted client")
		}
		// Only registered + protocol scopes survive the cap.
		got := admin.lastAcceptConsent.GrantScope
		if len(got) != 2 || got[0] != "read:accounts" || got[1] != "offline_access" {
			t.Errorf("unexpected granted scopes: %v", got)
		}
		// The MCP resource URL must always be stamped into the audience so the
		// RS validator accepts the token, even though the client requested none.
		aud := admin.lastAcceptConsent.GrantAudience
		if len(aud) != 1 || aud[0] != testResourceURL {
			t.Errorf("expected granted audience [%s], got: %v", testResourceURL, aud)
		}
	})

	t.Run("preserves a requested audience while keeping the resource URL unique", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:                    challenge,
					Client:                       hydra.OAuth2Client{ClientID: "known", ClientName: "Claude"},
					RequestedScope:               []string{"read:accounts"},
					RequestedAccessTokenAudience: []string{"https://other.example", testResourceURL},
				}, nil
			},
		}
		trusted := &mockTrustedClientService{isTrustedFn: func(string) (bool, error) { return true, nil }}
		handler := NewOAuthHandler(admin, &mockUserService{}, trusted, &mockAuditService{}, testScopes, testResourceURL)
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
					Client: hydra.OAuth2Client{
						ClientID:     "unknown",
						ClientName:   "Mystery",
						RedirectURIs: []string{"https://evil.example/cb"},
					},
					RequestedScope: []string{"read:accounts", "read:secrets"},
				}, nil
			},
		}
		trusted := &mockTrustedClientService{isTrustedFn: func(string) (bool, error) { return false, nil }}
		handler := NewOAuthHandler(admin, &mockUserService{}, trusted, &mockAuditService{}, testScopes, testResourceURL)
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
		scopes, ok := result["requested_scopes"].([]interface{})
		if !ok || len(scopes) != 1 || scopes[0] != "read:accounts" {
			t.Errorf("expected only grantable scopes in payload, got: %v", result["requested_scopes"])
		}
	})

	t.Run("rejects missing consent_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "GET", "/oauth/consent", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}

// --- AcceptConsent ---

func TestOAuthHandler_AcceptConsent(t *testing.T) {
	t.Run("grants consent and remembers the client when asked", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:      challenge,
					Subject:        "user-7",
					Client:         hydra.OAuth2Client{ClientID: "cid-9", ClientName: "Claude"},
					RequestedScope: []string{"read:accounts", "read:transactions"},
				}, nil
			},
		}
		trusted := &mockTrustedClientService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, trusted, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/accept",
			`{"consent_challenge":"cc1","remember_client":true}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !trusted.trustCalled || trusted.trustedID != "cid-9" {
			t.Errorf("expected client cid-9 to be trusted, got called=%v id=%q", trusted.trustCalled, trusted.trustedID)
		}
		if !admin.lastAcceptConsent.Remember {
			t.Error("expected Hydra consent remember flag to be set")
		}
	})

	t.Run("grants consent without remembering when flag is false", func(t *testing.T) {
		admin := &mockHydraAdmin{
			getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
				return &hydra.ConsentRequest{
					Challenge:      challenge,
					Client:         hydra.OAuth2Client{ClientID: "cid-9"},
					RequestedScope: []string{"read:accounts"},
				}, nil
			},
		}
		trusted := &mockTrustedClientService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, trusted, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/accept",
			`{"consent_challenge":"cc1","remember_client":false}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if trusted.trustCalled {
			t.Error("client must not be trusted when remember_client is false")
		}
	})

	t.Run("rejects missing consent_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/accept", `{"remember_client":true}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
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
		trusted := &mockTrustedClientService{}
		handler := NewOAuthHandler(admin, &mockUserService{}, trusted, &mockAuditService{}, testScopes, testResourceURL)
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
		if trusted.trustCalled {
			t.Error("client must never be trusted when denying consent")
		}
	})

	t.Run("rejects missing consent_challenge", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, &mockTrustedClientService{}, &mockAuditService{}, testScopes, testResourceURL)
		r := setupOAuthRouter(handler)

		rec := doRequest(r, "POST", "/oauth/consent/reject", `{}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
	})
}
