package handlers

import (
	"context"
	"net/http"
	"strings"
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
		// read:secrets is not registered and must be dropped; offline_access
		// survives and the remaining protocol scopes are always registered.
		if got.Scope != "read:accounts offline_access openid offline" {
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

	t.Run("defaults scope to the full read:* + protocol set when none requested", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
		r := setupRegistrationRouter(handler)

		// MCP connectors commonly register without a scope; the client must still
		// be able to request read:* + offline_access at authorize time.
		rec := doRequest(r, "POST", "/oauth/register",
			`{"client_name":"Claude","redirect_uris":["https://claude.ai/cb"]}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		want := "read:accounts read:transactions read:budgets openid offline offline_access"
		if admin.lastCreateClient.Scope != want {
			t.Errorf("expected default registrable scope %q, got %q", want, admin.lastCreateClient.Scope)
		}
	})

	t.Run("defaults scope when all requested scopes are disallowed", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
		r := setupRegistrationRouter(handler)

		// Every requested scope is dropped, which would otherwise leave an empty
		// (unusable) registered scope; fall back to the full default set instead.
		rec := doRequest(r, "POST", "/oauth/register",
			`{"redirect_uris":["https://claude.ai/cb"],"scope":"read:secrets write:everything"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		want := "read:accounts read:transactions read:budgets openid offline offline_access"
		if admin.lastCreateClient.Scope != want {
			t.Errorf("expected default registrable scope %q, got %q", want, admin.lastCreateClient.Scope)
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

func TestRegistrationHandler_RegistersOfflineAccess(t *testing.T) {
	admin := &mockHydraAdmin{}
	handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
	r := setupRegistrationRouter(handler)

	// A client that copies scopes_supported (read:* only) must still be able
	// to receive and use refresh tokens.
	rec := doRequest(r, "POST", "/oauth/register",
		`{"client_name":"Hermes","redirect_uris":["https://hermes.example/cb"],"scope":"read:accounts read:transactions"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	want := "read:accounts read:transactions openid offline offline_access"
	if got := admin.lastCreateClient.Scope; got != want {
		t.Errorf("scope = %q, want %q", got, want)
	}
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

func TestRegistrationHandler_WriteScopes(t *testing.T) {
	t.Run("registers write scopes when they are enabled", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopesWithWrite)

		rec := doRequest(setupRegistrationRouter(handler), "POST", "/oauth/register",
			`{"redirect_uris":["https://claude.ai/cb"],"scope":"read:accounts write:transactions"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		want := "read:accounts write:transactions openid offline offline_access"
		if admin.lastCreateClient.Scope != want {
			t.Errorf("scope = %q, want %q", admin.lastCreateClient.Scope, want)
		}
	})

	t.Run("drops write scopes when they are disabled", func(t *testing.T) {
		admin := &mockHydraAdmin{}
		handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)

		doRequest(setupRegistrationRouter(handler), "POST", "/oauth/register",
			`{"redirect_uris":["https://claude.ai/cb"],"scope":"read:accounts write:transactions"}`)

		if strings.Contains(admin.lastCreateClient.Scope, "write:") {
			t.Errorf("write scopes must not be registrable when disabled, got %q", admin.lastCreateClient.Scope)
		}
	})
}

func TestRegistrationHandler_TruncatesClientName(t *testing.T) {
	admin := &mockHydraAdmin{}
	handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)

	rec := doRequest(setupRegistrationRouter(handler), "POST", "/oauth/register",
		`{"redirect_uris":["https://claude.ai/cb"],"client_name":"`+strings.Repeat("ü", 400)+`"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := len([]rune(admin.lastCreateClient.ClientName)); n != maxClientNameLen {
		t.Errorf("client name has %d characters, want %d", n, maxClientNameLen)
	}
}

func TestSanitizeClientName(t *testing.T) {
	tests := map[string]string{
		"Claude":                         "Claude",
		"Claude\n\nVerified by Kuberan":  "Claude Verified by Kuberan",
		"  Claude\t ":                    "Claude",
		"Cla\u202Eude\u200B":             "Cla ude",
		"bad\xffutf8":                    "badutf8",
		"\x1b[1mBold\x1b[0m":             "[1mBold [0m",
		strings.Repeat("a", 99) + " bcd": strings.Repeat("a", 99),
	}
	for in, want := range tests {
		if got := sanitizeClientName(in); got != want {
			t.Errorf("sanitizeClientName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidRedirectURIPolicy(t *testing.T) {
	valid := []string{
		"https://claude.ai/api/mcp/auth_callback",
		"https://ok/cb",
		"https://example.com:8443/cb?x=1",
		"http://127.0.0.1:33418/callback",
		"http://localhost/cb",
		"http://[::1]:8080/cb",
		"com.example.app:/oauth2redirect",
		"com.example.app://callback",
		"cursor://anysphere.cursor-retrieval/oauth/callback",
		"vscode://callback",
	}
	invalid := []string{
		"",
		"not a url",
		"/relative/cb",
		"https:///no-host",
		"http://example.com/cb",       // plain http off loopback
		"http://127.0.0.1.evil.com/x", // not actually loopback
		"https://claude.ai/cb#frag",
		"https://user:pw@claude.ai/cb",
		"javascript:alert(1)",
		"data:text/html,hi",
		"file:///etc/passwd",
		"vbscript:x",
		"blob:https://claude.ai/x",
		"about:blank",
		"https://claude.ai/" + strings.Repeat("a", 2048),
		"https://claude.ai/cb\nx",
	}
	for _, u := range valid {
		if !isValidRedirectURI(u) {
			t.Errorf("%q should be accepted", u)
		}
	}
	for _, u := range invalid {
		if isValidRedirectURI(u) {
			t.Errorf("%q should be rejected", u)
		}
	}
}

func TestRegistrationHandler_Limits(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"too many redirect URIs", `{"redirect_uris":["https://a.example/1","https://a.example/2","https://a.example/3","https://a.example/4"]}`},
		{"oversized body", `{"redirect_uris":["https://claude.ai/cb"],"client_name":"` + strings.Repeat("a", 17<<10) + `"}`},
		{"javascript redirect", `{"redirect_uris":["javascript:alert(1)"]}`},
		{"not JSON", `not-json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admin := &mockHydraAdmin{}
			handler := NewRegistrationHandler(admin, &recordingAuditService{}, testScopes)
			rec := doRequest(setupRegistrationRouter(handler), "POST", "/oauth/register", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorCode(t, parseJSON(t, rec), apperrors.ErrInvalidInput.Code)
			if admin.createClientCalled {
				t.Error("CreateClient must not be called")
			}
		})
	}
}
