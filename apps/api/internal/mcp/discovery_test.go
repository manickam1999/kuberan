package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

var testOAuthConfig = OAuthConfig{
	Issuer:      "https://auth.example.com",
	ResourceURL: "https://mcp.example.com",
	Scopes:      "read:accounts read:transactions read:budgets",
}

func TestDiscoveryHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, ProtectedResourceMetadataPath, nil)

	discoveryHandler(testOAuthConfig).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var doc ProtectedResourceMetadata
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if doc.Resource != testOAuthConfig.ResourceURL {
		t.Errorf("resource = %q, want %q", doc.Resource, testOAuthConfig.ResourceURL)
	}
	if want := []string{"https://auth.example.com"}; !reflect.DeepEqual(doc.AuthorizationServers, want) {
		t.Errorf("authorization_servers = %v, want %v", doc.AuthorizationServers, want)
	}
	wantScopes := []string{"read:accounts", "read:transactions", "read:budgets"}
	if !reflect.DeepEqual(doc.ScopesSupported, wantScopes) {
		t.Errorf("scopes_supported = %v, want %v", doc.ScopesSupported, wantScopes)
	}
	if want := []string{"header"}; !reflect.DeepEqual(doc.BearerMethodsSupported, want) {
		t.Errorf("bearer_methods_supported = %v, want %v", doc.BearerMethodsSupported, want)
	}
}

func TestResourceMetadataURL(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		want     string
	}{
		{"no trailing slash", "https://mcp.example.com", "https://mcp.example.com/.well-known/oauth-protected-resource"},
		{"trailing slash", "https://mcp.example.com/", "https://mcp.example.com/.well-known/oauth-protected-resource"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceMetadataURL(tt.resource); got != tt.want {
				t.Errorf("resourceMetadataURL(%q) = %q, want %q", tt.resource, got, tt.want)
			}
		})
	}
}

func TestWithWWWAuthenticate(t *testing.T) {
	wantChallenge := `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource"`

	t.Run("adds challenge on 401", func(t *testing.T) {
		h := withWWWAuthenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}), testOAuthConfig)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != wantChallenge {
			t.Errorf("WWW-Authenticate = %q, want %q", got, wantChallenge)
		}
	})

	t.Run("no challenge on 200", func(t *testing.T) {
		h := withWWWAuthenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}), testOAuthConfig)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != "" {
			t.Errorf("WWW-Authenticate = %q, want empty", got)
		}
	})

	t.Run("preserves handler-set challenge", func(t *testing.T) {
		custom := `Bearer error="insufficient_scope"`
		h := withWWWAuthenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", custom)
			w.WriteHeader(http.StatusUnauthorized)
		}), testOAuthConfig)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))

		if got := rec.Header().Get("WWW-Authenticate"); got != custom {
			t.Errorf("WWW-Authenticate = %q, want %q", got, custom)
		}
	})
}
