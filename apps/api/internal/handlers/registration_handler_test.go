package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/hydra"
)

// recordingAuditService captures the last audit call so tests can assert that a
// DCR registration is logged.
type recordingAuditService struct {
	called bool
	action string
	resID  string
}

func (m *recordingAuditService) Log(_, action, _, resourceID, _ string, _ map[string]interface{}) {
	m.called = true
	m.action = action
	m.resID = resourceID
}

func setupRegistrationRouter(handler *RegistrationHandler) *gin.Engine {
	r := gin.New()
	r.POST("/oauth/register", handler.Register)
	return r
}

func TestRegistrationHandler_Register(t *testing.T) {
	t.Run("forces public-client policy and caps scopes", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		audit := &recordingAuditService{}
		handler := NewRegistrationHandler(admin, audit, testScopes)
		r := setupRegistrationRouter(handler)

		body := `{
			"client_name":"Claude",
			"redirect_uris":["https://claude.ai/cb"],
			"grant_types":["authorization_code","client_credentials"],
			"response_types":["code","token"],
			"token_endpoint_auth_method":"client_secret_basic",
			"scope":"read:accounts read:secrets offline_access"
		}`
		rec := doRequest(r, "POST", "/oauth/register", body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if !admin.createClientCalled {
			t.Fatal("expected CreateClient to be called")
		}
		got := admin.lastCreateClient
		if got.TokenEndpointAuthMethod != "none" {
			t.Errorf("expected public client (none), got %q", got.TokenEndpointAuthMethod)
		}
		if len(got.GrantTypes) != 2 || got.GrantTypes[0] != "authorization_code" || got.GrantTypes[1] != "refresh_token" {
			t.Errorf("grant types not restricted: %v", got.GrantTypes)
		}
		if len(got.ResponseTypes) != 1 || got.ResponseTypes[0] != "code" {
			t.Errorf("response types not forced to code: %v", got.ResponseTypes)
		}
		// read:secrets is not registered and must be dropped; offline_access survives.
		if got.Scope != "read:accounts offline_access" {
			t.Errorf("scope not capped correctly: %q", got.Scope)
		}
	})

	t.Run("audits the registration", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		audit := &recordingAuditService{}
		handler := NewRegistrationHandler(admin, audit, testScopes)
		r := setupRegistrationRouter(handler)

		rec := doRequest(r, "POST", "/oauth/register",
			`{"client_name":"Claude","redirect_uris":["https://claude.ai/cb"],"scope":"read:accounts"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if !audit.called || audit.action != "OAUTH_CLIENT_REGISTERED" {
			t.Errorf("expected OAUTH_CLIENT_REGISTERED audit, got called=%v action=%q", audit.called, audit.action)
		}
		if audit.resID != "generated-id" {
			t.Errorf("expected audit to record the new client id, got %q", audit.resID)
		}
	})

	t.Run("defaults grants when none requested", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
		r := setupRegistrationRouter(handler)

		rec := doRequest(r, "POST", "/oauth/register",
			`{"redirect_uris":["https://claude.ai/cb"]}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(admin.lastCreateClient.GrantTypes) != 2 {
			t.Errorf("expected default grant types, got %v", admin.lastCreateClient.GrantTypes)
		}
	})

	t.Run("rejects missing redirect_uris", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
		r := setupRegistrationRouter(handler)

		rec := doRequest(r, "POST", "/oauth/register", `{"client_name":"Claude"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
		if admin.createClientCalled {
			t.Error("CreateClient must not be called without redirect_uris")
		}
	})

	t.Run("rejects an invalid redirect_uri", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
		r := setupRegistrationRouter(handler)

		rec := doRequest(r, "POST", "/oauth/register",
			`{"redirect_uris":["https://ok/cb","not a url"]}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if admin.createClientCalled {
			t.Error("CreateClient must not be called with an invalid redirect_uri")
		}
	})

	t.Run("propagates a Hydra create error", func(t *testing.T) {
		admin := &mockHydraAdmin{
			createClientFn: func(context.Context, hydra.OAuth2ClientCreate) (*hydra.OAuth2Client, error) {
				return nil, apperrors.ErrInternalServer
			},
		}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
		r := setupRegistrationRouter(handler)

		rec := doRequest(r, "POST", "/oauth/register",
			`{"redirect_uris":["https://claude.ai/cb"],"scope":"read:accounts"}`)

		if rec.Code != apperrors.ErrInternalServer.StatusCode {
			t.Fatalf("expected %d, got %d", apperrors.ErrInternalServer.StatusCode, rec.Code)
		}
	})
}

func TestIsValidRedirectURI(t *testing.T) {
	cases := []struct {
		uri  string
		want bool
	}{
		{"https://claude.ai/callback", true},
		{"http://localhost:8080/cb", true},
		{"com.example.app:/oauth/cb", true}, // native custom scheme
		{"", false},
		{"not a url", false},
		{"https://", false}, // scheme but no host
		{"/relative/path", false},
	}
	for _, tc := range cases {
		if got := isValidRedirectURI(tc.uri); got != tc.want {
			t.Errorf("isValidRedirectURI(%q) = %v, want %v", tc.uri, got, tc.want)
		}
	}
}
