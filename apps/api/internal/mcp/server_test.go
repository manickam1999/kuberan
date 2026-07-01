package mcp

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
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

func TestMakeAuthFromRequest(t *testing.T) {
	t.Run("valid token sets subject and scopes", func(t *testing.T) {
		v := fakeValidator{claims: &AccessClaims{Subject: "user-42", Scopes: []string{"read:accounts"}}}
		fn := makeAuthFromRequest(v)

		req := httptest.NewRequest("POST", "/mcp", nil)
		req.Header.Set("Authorization", "Bearer sometoken")
		ctx := fn(context.Background(), req)

		gotUser, err := getUserID(ctx)
		if err != nil || gotUser != "user-42" {
			t.Fatalf("expected user-42, got %q err=%v", gotUser, err)
		}
		if got := scopesFromContext(ctx); len(got) != 1 || got[0] != "read:accounts" {
			t.Fatalf("expected scopes [read:accounts], got %v", got)
		}
	})

	t.Run("missing header yields no user", func(t *testing.T) {
		fn := makeAuthFromRequest(fakeValidator{})
		req := httptest.NewRequest("POST", "/mcp", nil)
		ctx := fn(context.Background(), req)
		if _, err := getUserID(ctx); err == nil {
			t.Fatalf("expected unauthorized context")
		}
	})

	t.Run("malformed header yields no user", func(t *testing.T) {
		fn := makeAuthFromRequest(fakeValidator{claims: &AccessClaims{Subject: "x"}})
		req := httptest.NewRequest("POST", "/mcp", nil)
		req.Header.Set("Authorization", "Basic abc")
		ctx := fn(context.Background(), req)
		if _, err := getUserID(ctx); err == nil {
			t.Fatalf("expected unauthorized context for non-Bearer scheme")
		}
	})

	t.Run("rejected token yields no user", func(t *testing.T) {
		fn := makeAuthFromRequest(fakeValidator{err: errors.New("bad token")})
		req := httptest.NewRequest("POST", "/mcp", nil)
		req.Header.Set("Authorization", "Bearer sometoken")
		ctx := fn(context.Background(), req)
		if _, err := getUserID(ctx); err == nil {
			t.Fatalf("expected unauthorized context for rejected token")
		}
		if got := scopesFromContext(ctx); got != nil {
			t.Fatalf("expected no scopes for rejected token, got %v", got)
		}
	})
}
