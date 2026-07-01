package hydra

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "kuberan/internal/errors"
)

// recordingServer captures the last request and serves a canned response.
type recordingServer struct {
	server     *httptest.Server
	lastMethod string
	lastPath   string
	lastQuery  string
	lastBody   map[string]any
}

func newRecordingServer(t *testing.T, status int, respBody any) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.lastMethod = r.Method
		rs.lastPath = r.URL.Path
		rs.lastQuery = r.URL.RawQuery
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			rs.lastBody = map[string]any{}
			_ = json.Unmarshal(raw, &rs.lastBody)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if respBody != nil {
			_ = json.NewEncoder(w).Encode(respBody)
		}
	}))
	t.Cleanup(rs.server.Close)
	return rs
}

func TestGetLoginRequest(t *testing.T) {
	rs := newRecordingServer(t, http.StatusOK, LoginRequest{
		Challenge:      "abc",
		Client:         OAuth2Client{ClientID: "cid", ClientName: "Claude"},
		RequestedScope: []string{"read:accounts"},
	})
	c := NewAdminClient(rs.server.URL)

	got, err := c.GetLoginRequest(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetLoginRequest: %v", err)
	}
	if got.Client.ClientID != "cid" || len(got.RequestedScope) != 1 {
		t.Fatalf("unexpected login request: %+v", got)
	}
	if rs.lastMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", rs.lastMethod)
	}
	if rs.lastPath != "/admin/oauth2/auth/requests/login" {
		t.Errorf("path = %s", rs.lastPath)
	}
	if rs.lastQuery != "login_challenge=abc" {
		t.Errorf("query = %s", rs.lastQuery)
	}
}

func TestGetLoginRequestEmptyChallenge(t *testing.T) {
	c := NewAdminClient("http://unused")
	if _, err := c.GetLoginRequest(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty challenge")
	}
}

func TestAcceptLogin(t *testing.T) {
	rs := newRecordingServer(t, http.StatusOK, redirectResponse{RedirectTo: "https://hydra/next"})
	c := NewAdminClient(rs.server.URL)

	redirect, err := c.AcceptLogin(context.Background(), "chal", AcceptLoginParams{
		Subject: "user-42", Remember: true, RememberFor: 3600,
	})
	if err != nil {
		t.Fatalf("AcceptLogin: %v", err)
	}
	if redirect != "https://hydra/next" {
		t.Errorf("redirect = %s", redirect)
	}
	if rs.lastMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", rs.lastMethod)
	}
	if rs.lastPath != "/admin/oauth2/auth/requests/login/accept" {
		t.Errorf("path = %s", rs.lastPath)
	}
	if rs.lastBody["subject"] != "user-42" || rs.lastBody["remember"] != true {
		t.Errorf("body = %+v", rs.lastBody)
	}
}

func TestAcceptLoginRequiresSubject(t *testing.T) {
	c := NewAdminClient("http://unused")
	if _, err := c.AcceptLogin(context.Background(), "chal", AcceptLoginParams{}); err == nil {
		t.Fatal("expected error for empty subject")
	}
}

func TestAcceptConsent(t *testing.T) {
	rs := newRecordingServer(t, http.StatusOK, redirectResponse{RedirectTo: "https://hydra/done"})
	c := NewAdminClient(rs.server.URL)

	redirect, err := c.AcceptConsent(context.Background(), "chal", AcceptConsentParams{
		GrantScope:    []string{"read:accounts", "read:budgets"},
		GrantAudience: []string{"https://mcp.example"},
		Remember:      true,
	})
	if err != nil {
		t.Fatalf("AcceptConsent: %v", err)
	}
	if redirect != "https://hydra/done" {
		t.Errorf("redirect = %s", redirect)
	}
	if rs.lastPath != "/admin/oauth2/auth/requests/consent/accept" {
		t.Errorf("path = %s", rs.lastPath)
	}
	scopes, ok := rs.lastBody["grant_scope"].([]any)
	if !ok || len(scopes) != 2 {
		t.Errorf("grant_scope = %+v", rs.lastBody["grant_scope"])
	}
}

func TestGetConsentRequest(t *testing.T) {
	rs := newRecordingServer(t, http.StatusOK, ConsentRequest{
		Challenge:      "cc",
		Client:         OAuth2Client{ClientID: "cid"},
		RequestedScope: []string{"read:accounts"},
	})
	c := NewAdminClient(rs.server.URL)

	got, err := c.GetConsentRequest(context.Background(), "cc")
	if err != nil {
		t.Fatalf("GetConsentRequest: %v", err)
	}
	if got.Client.ClientID != "cid" {
		t.Errorf("client = %+v", got.Client)
	}
	if rs.lastQuery != "consent_challenge=cc" {
		t.Errorf("query = %s", rs.lastQuery)
	}
}

func TestGetClient(t *testing.T) {
	rs := newRecordingServer(t, http.StatusOK, OAuth2Client{
		ClientID: "cid", ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/cb"},
	})
	c := NewAdminClient(rs.server.URL)

	got, err := c.GetClient(context.Background(), "cid")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.ClientName != "Claude" || len(got.RedirectURIs) != 1 {
		t.Errorf("client = %+v", got)
	}
	if rs.lastPath != "/admin/clients/cid" {
		t.Errorf("path = %s", rs.lastPath)
	}
}

func TestNotFoundMapsToErrNotFound(t *testing.T) {
	rs := newRecordingServer(t, http.StatusNotFound, nil)
	c := NewAdminClient(rs.server.URL)

	_, err := c.GetConsentRequest(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperrors.AppError
	if !stderrors.As(err, &appErr) || appErr.Code != apperrors.ErrNotFound.Code {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestServerErrorMapsToInternal(t *testing.T) {
	rs := newRecordingServer(t, http.StatusInternalServerError, nil)
	c := NewAdminClient(rs.server.URL)

	_, err := c.GetLoginRequest(context.Background(), "chal")
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperrors.AppError
	if !stderrors.As(err, &appErr) || appErr.Code != apperrors.ErrInternalServer.Code {
		t.Errorf("want ErrInternalServer, got %v", err)
	}
}

func TestRejectConsent(t *testing.T) {
	rs := newRecordingServer(t, http.StatusOK, redirectResponse{RedirectTo: "https://hydra/denied"})
	c := NewAdminClient(rs.server.URL)

	redirect, err := c.RejectConsent(context.Background(), "chal", "user declined")
	if err != nil {
		t.Fatalf("RejectConsent: %v", err)
	}
	if redirect != "https://hydra/denied" {
		t.Errorf("redirect = %s", redirect)
	}
	if rs.lastBody["error"] != "access_denied" {
		t.Errorf("body = %+v", rs.lastBody)
	}
}
