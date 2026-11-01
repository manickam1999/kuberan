package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// fakeValidator is a TokenValidator stub for exercising makeAuthFromRequest.
type fakeValidator struct {
	claims *AccessClaims
	err    error
}

func (f fakeValidator) Validate(_ context.Context, _ string) (*AccessClaims, error) {
	return f.claims, f.err
}

func TestRequireScope(t *testing.T) {
	tests := []struct {
		name     string
		granted  []string
		accepted []string
		wantOK   bool
	}{
		{"exact match", []string{"read:accounts"}, []string{"read:accounts"}, true},
		{"one of many granted", []string{"read:budgets", "read:accounts"}, []string{"read:accounts"}, true},
		{"any-of accepts alt", []string{"read:portfolio"}, []string{"read:investments", "read:portfolio"}, true},
		{"missing scope", []string{"read:budgets"}, []string{"read:accounts"}, false},
		{"no scopes granted", nil, []string{"read:accounts"}, false},
		{"related-but-different", []string{"read:transactions"}, []string{"read:accounts"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), scopesKey{}, tt.granted)
			denied := requireScope(ctx, tt.accepted...)
			if tt.wantOK && denied != nil {
				t.Fatalf("expected authorized, got denial result")
			}
			if !tt.wantOK {
				if denied == nil {
					t.Fatalf("expected denial result, got nil")
				}
				if !denied.IsError {
					t.Fatalf("denial result should be an MCP error")
				}
			}
		})
	}
}

// TestHandlerEnforcesScope proves the enforcement is wired into a real tool
// handler: with a user but no matching scope, the handler returns the
// insufficient-scope error before ever touching the (nil) service layer.
func TestHandlerEnforcesScope(t *testing.T) {
	s := &Server{} // nil services: the scope check must short-circuit first.

	ctx := context.WithValue(context.Background(), userIDKey{}, "user-1")
	ctx = context.WithValue(ctx, scopesKey{}, []string{"read:budgets"})

	res, err := s.handleListAccounts(ctx, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected an MCP error result for missing scope")
	}
}

// TestHandlerRejectsMissingUser confirms getUserID still guards ahead of scope.
func TestHandlerRejectsMissingUser(t *testing.T) {
	s := &Server{}
	res, err := s.handleListAccounts(context.Background(), mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected an MCP error result for missing user")
	}
}

// TestRequireBearer proves the HTTP-level auth gate: requests without a valid
// Bearer token get a 401 before reaching the MCP handler (the 401 that
// triggers a client's OAuth discovery), and valid tokens pass through.
func TestRequireBearer(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		authHeader string
		validator  fakeValidator
		wantStatus int
	}{
		{"missing header", "", fakeValidator{}, http.StatusUnauthorized},
		{"non-bearer scheme", "Basic abc", fakeValidator{}, http.StatusUnauthorized},
		{"empty bearer token", "Bearer ", fakeValidator{}, http.StatusUnauthorized},
		{"rejected token", "Bearer bad", fakeValidator{err: errors.New("bad token")}, http.StatusUnauthorized},
		{"valid token", "Bearer good", fakeValidator{claims: &AccessClaims{Subject: "user-1"}}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled = false
			h := withWWWAuthenticate(requireBearer(next, tt.validator, nil), testOAuthConfig)

			req := httptest.NewRequest("POST", "/mcp", http.NoBody)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				if nextCalled {
					t.Fatalf("MCP handler must not run for unauthenticated requests")
				}
				challenge := rec.Header().Get("WWW-Authenticate")
				if !strings.Contains(challenge, "resource_metadata=") {
					t.Fatalf("401 must carry the RFC 9728 challenge, got %q", challenge)
				}
			} else if !nextCalled {
				t.Fatalf("MCP handler should run for a valid token")
			}
		})
	}
}

// withClaims returns req carrying claims as requireBearer attaches them.
func withClaims(req *http.Request, claims *AccessClaims) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), claimsKey{}, claims))
}

// TestRequireBearerValidatesOnce proves the token is validated once per
// request and its claims reach the MCP context through makeAuthFromRequest.
func TestRequireBearerValidatesOnce(t *testing.T) {
	v := &countingValidator{claims: &AccessClaims{Subject: "user-42", ClientID: "c1", Scopes: []string{"read:accounts"}}}
	fn := makeAuthFromRequest(nil)
	var got context.Context
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = fn(r.Context(), r)
	})
	req := httptest.NewRequest("POST", "/mcp", http.NoBody)
	req.Header.Set("Authorization", "Bearer tok")
	requireBearer(next, v, nil).ServeHTTP(httptest.NewRecorder(), req)

	if v.calls != 1 {
		t.Fatalf("token validated %d times, want 1", v.calls)
	}
	if user, err := getUserID(got); err != nil || user != "user-42" {
		t.Fatalf("expected user-42 in the MCP context, got %q err=%v", user, err)
	}
	if c, _ := got.Value(clientIDKey{}).(string); c != "c1" {
		t.Fatalf("client ID = %q, want c1", c)
	}
}

type countingValidator struct {
	claims *AccessClaims
	calls  int
}

func (c *countingValidator) Validate(context.Context, string) (*AccessClaims, error) {
	c.calls++
	return c.claims, nil
}

// fakeUsers is a userLookup stub for the active-user check.
type fakeUsers struct {
	users map[string]*models.User
	err   error
	calls int
}

func (f *fakeUsers) GetUserByID(id string) (*models.User, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	return nil, apperrors.ErrUserNotFound
}

// TestRequireBearerRejectsInactiveUser proves a deactivated user is cut off
// from reads too (a refresh token would otherwise keep minting valid access
// tokens), and that the check is cached.
func TestRequireBearerRejectsInactiveUser(t *testing.T) {
	users := &fakeUsers{users: map[string]*models.User{
		"active":   {IsActive: true},
		"inactive": {IsActive: false},
	}}
	now := time.Unix(1_700_000_000, 0)
	active := newActiveUsers(users, time.Minute, func() time.Time { return now })
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	serve := func(sub string) int {
		h := withWWWAuthenticate(requireBearer(next, fakeValidator{claims: &AccessClaims{Subject: sub}}, active), testOAuthConfig)
		req := httptest.NewRequest("POST", "/mcp", http.NoBody)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := serve("active"); got != http.StatusOK {
		t.Fatalf("active user: status %d, want 200", got)
	}
	if got := serve("inactive"); got != http.StatusUnauthorized {
		t.Fatalf("inactive user: status %d, want 401", got)
	}
	if got := serve("deleted"); got != http.StatusUnauthorized {
		t.Fatalf("unknown user: status %d, want 401", got)
	}
	calls := users.calls
	_ = serve("active")
	if users.calls != calls {
		t.Fatalf("the active check should be cached within the TTL")
	}

	// Deactivation takes effect once the cached entry expires.
	users.users["active"].IsActive = false
	now = now.Add(2 * time.Minute)
	if got := serve("active"); got != http.StatusUnauthorized {
		t.Fatalf("deactivated user after TTL: status %d, want 401", got)
	}

	// A database failure is not treated as "inactive" and not cached.
	users.err = errors.New("db down")
	now = now.Add(2 * time.Minute)
	if got := serve("active"); got != http.StatusServiceUnavailable {
		t.Fatalf("lookup failure: status %d, want 503", got)
	}
}

func TestMakeAuthFromRequest(t *testing.T) {
	t.Run("validated claims set subject and scopes", func(t *testing.T) {
		fn := makeAuthFromRequest(nil)

		req := withClaims(httptest.NewRequest("POST", "/mcp", http.NoBody),
			&AccessClaims{Subject: "user-42", Scopes: []string{"read:accounts"}})
		ctx := fn(req.Context(), req)

		gotUser, err := getUserID(ctx)
		if err != nil || gotUser != "user-42" {
			t.Fatalf("expected user-42, got %q err=%v", gotUser, err)
		}
		if got := scopesFromContext(ctx); len(got) != 1 || got[0] != "read:accounts" {
			t.Fatalf("expected scopes [read:accounts], got %v", got)
		}
	})

	t.Run("validated claims set client ID and caller IP", func(t *testing.T) {
		fn := makeAuthFromRequest(nil)

		req := httptest.NewRequest("POST", "/mcp", http.NoBody)
		req.RemoteAddr = "192.0.2.10:54321"
		req = withClaims(req, &AccessClaims{Subject: "user-42", ClientID: "claude-client"})
		ctx := fn(req.Context(), req)

		if got, _ := ctx.Value(clientIDKey{}).(string); got != "claude-client" {
			t.Errorf("client ID = %q, want claude-client", got)
		}
		if got, _ := ctx.Value(clientIPKey{}).(string); got != "192.0.2.10" {
			t.Errorf("IP = %q, want the peer address", got)
		}
	})

	t.Run("CF-Connecting-IP is believed only from a trusted proxy", func(t *testing.T) {
		trusted, err := ParseTrustedProxies([]string{"172.16.0.0/12"})
		if err != nil {
			t.Fatal(err)
		}
		cases := []struct{ peer, header, want string }{
			{"172.18.0.5:443", "198.51.100.4", "198.51.100.4"}, // via the tunnel
			{"203.0.113.9:443", "198.51.100.4", "203.0.113.9"}, // forged by a direct caller
			{"172.18.0.5:443", "not-an-ip", "172.18.0.5"},      // garbage header
			{"172.18.0.5:443", "", "172.18.0.5"},
		}
		for _, tc := range cases {
			req := httptest.NewRequest("POST", "/mcp", http.NoBody)
			req.RemoteAddr = tc.peer
			if tc.header != "" {
				req.Header.Set("CF-Connecting-IP", tc.header)
			}
			if got := requestIP(req, trusted); got != tc.want {
				t.Errorf("peer %s header %q: IP = %q, want %q", tc.peer, tc.header, got, tc.want)
			}
		}
		if _, err := ParseTrustedProxies([]string{"nope"}); err == nil {
			t.Error("an invalid CIDR must be rejected")
		}
	})

	t.Run("no validated claims yields no user", func(t *testing.T) {
		fn := makeAuthFromRequest(nil)
		req := httptest.NewRequest("POST", "/mcp", http.NoBody)
		// A header alone is not trusted: only claims attached by requireBearer.
		req.Header.Set("Authorization", "Bearer sometoken")
		ctx := fn(req.Context(), req)
		if _, err := getUserID(ctx); err == nil {
			t.Fatalf("expected unauthorized context")
		}
		if got := scopesFromContext(ctx); got != nil {
			t.Fatalf("expected no scopes, got %v", got)
		}
	})
}
