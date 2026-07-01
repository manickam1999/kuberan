package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"kuberan/internal/config"
	"kuberan/internal/handlers"
	"kuberan/internal/hydra"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// fakeHydraAdmin is an httptest-backed stand-in for Hydra's private admin API.
// It serves only the endpoints the login/consent bridge and DCR proxy exercise,
// recording the request bodies so tests can assert what the bridge granted. This
// lets the real internal/hydra net/http client run end-to-end without a live
// Hydra container (see plan 15 testing strategy).
type fakeHydraAdmin struct {
	server *httptest.Server

	mu sync.Mutex
	// consents maps a consent challenge to the request the bridge will fetch.
	consents map[string]hydra.ConsentRequest
	// lastConsentAccept records the body of the most recent consent/accept call.
	lastConsentAccept map[string]any
	// lastLoginAccept records the body of the most recent login/accept call.
	lastLoginAccept map[string]any
	// lastClientCreate records the body of the most recent admin client create.
	lastClientCreate map[string]any
	// clientSeq generates deterministic client IDs for DCR registrations.
	clientSeq int
}

func newFakeHydraAdmin() *fakeHydraAdmin {
	f := &fakeHydraAdmin{consents: map[string]hydra.ConsentRequest{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeHydraAdmin) close() { f.server.Close() }

// seedConsent registers a pending consent challenge the bridge can fetch.
func (f *fakeHydraAdmin) seedConsent(challenge string, req hydra.ConsentRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	req.Challenge = challenge
	f.consents[challenge] = req
}

func (f *fakeHydraAdmin) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodPut && r.URL.Path == "/admin/oauth2/auth/requests/login/accept":
		f.lastLoginAccept = decodeBody(r)
		writeJSON(w, map[string]string{"redirect_to": "https://auth.example/continue?login=1"})

	case r.Method == http.MethodGet && r.URL.Path == "/admin/oauth2/auth/requests/consent":
		challenge := r.URL.Query().Get("consent_challenge")
		consent, ok := f.consents[challenge]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, consent)

	case r.Method == http.MethodPut && r.URL.Path == "/admin/oauth2/auth/requests/consent/accept":
		f.lastConsentAccept = decodeBody(r)
		writeJSON(w, map[string]string{"redirect_to": "https://auth.example/continue?consent=1"})

	case r.Method == http.MethodPost && r.URL.Path == "/admin/clients":
		f.lastClientCreate = decodeBody(r)
		f.clientSeq++
		created := f.lastClientCreate
		created["client_id"] = fmt.Sprintf("dcr-client-%d", f.clientSeq)
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, created)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func decodeBody(r *http.Request) map[string]any {
	var out map[string]any
	_ = json.NewDecoder(r.Body).Decode(&out)
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// oauthApp is a test stack for the OAuth login/consent bridge: the real handlers
// and services backed by SQLite, with the real hydra admin client pointed at a
// fake Hydra admin server.
type oauthApp struct {
	*testApp
	hydra       *fakeHydraAdmin
	resourceURL string
}

// setupOAuthApp wires the OAuth bridge (login/consent + DCR proxy) onto a fresh
// isolated database and a fake Hydra admin.
func setupOAuthApp(t *testing.T) *oauthApp {
	t.Helper()

	db := setupIsolatedDB(t)
	fake := newFakeHydraAdmin()
	t.Cleanup(fake.close)

	adminClient := hydra.NewAdminClient(fake.server.URL)
	userService := services.NewUserService(db)
	trustedClient := services.NewTrustedClientService(db)
	auditService := services.NewAuditService(db)

	resourceURL := "https://mcp.example"
	oauthHandler := handlers.NewOAuthHandler(
		adminClient, userService, trustedClient, auditService,
		config.DefaultOAuthScopes, resourceURL,
	)
	registrationHandler := handlers.NewRegistrationHandler(adminClient, auditService, config.DefaultOAuthScopes)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.ErrorHandler())

	v1 := router.Group("/api/v1")
	auth := v1.Group("/auth")
	auth.POST("/register", handlers.NewAuthHandler(userService, auditService).Register)

	oauth := v1.Group("/oauth")
	oauth.POST("/login", oauthHandler.Login)
	oauth.GET("/consent", oauthHandler.GetConsent)
	oauth.POST("/consent/accept", oauthHandler.AcceptConsent)
	oauth.POST("/register", registrationHandler.Register)

	return &oauthApp{
		testApp:     &testApp{DB: db, Router: router},
		hydra:       fake,
		resourceURL: resourceURL,
	}
}

func TestOAuthLoginBridge(t *testing.T) {
	app := setupOAuthApp(t)
	_, _, userID := app.registerUser(t, "oauth@example.com", "SuperSecret123!")

	t.Run("valid credentials accept login with subject=user ID", func(t *testing.T) {
		body := `{"login_challenge":"login-abc","email":"oauth@example.com","password":"SuperSecret123!"}`
		rec := app.request("POST", "/api/v1/oauth/login", body, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["redirect_to"] == "" || result["redirect_to"] == nil {
			t.Fatalf("expected redirect_to, got %v", result)
		}
		if got := app.hydra.lastLoginAccept["subject"]; got != userID {
			t.Fatalf("expected AcceptLogin subject=%q, got %v", userID, got)
		}
	})

	t.Run("invalid credentials are rejected", func(t *testing.T) {
		body := `{"login_challenge":"login-abc","email":"oauth@example.com","password":"wrong-password"}`
		rec := app.request("POST", "/api/v1/oauth/login", body, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestOAuthConsentTOFU(t *testing.T) {
	app := setupOAuthApp(t)

	const clientID = "claude-connector"
	// The client requests one grantable scope, one disallowed scope, and no
	// resource audience (as real MCP connectors do).
	seed := func(challenge string) {
		app.hydra.seedConsent(challenge, hydra.ConsentRequest{
			Subject: "user-1",
			Client: hydra.OAuth2Client{
				ClientID:     clientID,
				ClientName:   "Claude",
				RedirectURIs: []string{"https://claude.ai/callback"},
			},
			RequestedScope:               []string{"read:accounts", "admin:everything"},
			RequestedAccessTokenAudience: nil,
		})
	}

	t.Run("unknown client returns consent details to render", func(t *testing.T) {
		seed("consent-1")
		rec := app.request("GET", "/api/v1/oauth/consent?consent_challenge=consent-1", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if _, ok := result["redirect_to"]; ok {
			t.Fatalf("unknown client must not auto-redirect: %v", result)
		}
		scopes := toStringSlice(result["requested_scopes"])
		if len(scopes) != 1 || scopes[0] != "read:accounts" {
			t.Fatalf("expected capped scopes [read:accounts], got %v", scopes)
		}
	})

	t.Run("accept with remember records trust and caps grant", func(t *testing.T) {
		seed("consent-1")
		body := `{"consent_challenge":"consent-1","remember_client":true}`
		rec := app.request("POST", "/api/v1/oauth/consent/accept", body, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Granted scope is capped to the registered read:* set.
		grantScope := toStringSlice(app.hydra.lastConsentAccept["grant_scope"])
		if len(grantScope) != 1 || grantScope[0] != "read:accounts" {
			t.Fatalf("expected grant_scope [read:accounts], got %v", grantScope)
		}
		// The MCP resource URL is force-granted into the audience even though the
		// client requested none, so the token validates against /mcp.
		grantAud := toStringSlice(app.hydra.lastConsentAccept["grant_access_token_audience"])
		if len(grantAud) != 1 || grantAud[0] != app.resourceURL {
			t.Fatalf("expected audience [%s], got %v", app.resourceURL, grantAud)
		}

		// The client is now recorded as trusted.
		var count int64
		app.DB.Model(&models.TrustedOAuthClient{}).Where("client_id = ?", clientID).Count(&count)
		if count != 1 {
			t.Fatalf("expected client to be trusted, got %d rows", count)
		}
		// A trust audit entry was written.
		var audits int64
		app.DB.Model(&models.AuditLog{}).Where("action = ?", "OAUTH_CLIENT_TRUSTED").Count(&audits)
		if audits != 1 {
			t.Fatalf("expected 1 OAUTH_CLIENT_TRUSTED audit, got %d", audits)
		}
	})

	t.Run("trusted client auto-accepts on next consent", func(t *testing.T) {
		seed("consent-2")
		rec := app.request("GET", "/api/v1/oauth/consent?consent_challenge=consent-2", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["redirect_to"] == "" || result["redirect_to"] == nil {
			t.Fatalf("trusted client must auto-redirect, got %v", result)
		}
	})
}

func TestOAuthDCRProxyHardensClient(t *testing.T) {
	app := setupOAuthApp(t)

	// A client tries to register with a confidential auth method, an extra grant
	// type, an implicit response type, and an over-broad scope.
	body := `{
		"client_name":"Rogue",
		"redirect_uris":["https://claude.ai/callback"],
		"grant_types":["authorization_code","client_credentials"],
		"response_types":["code","token"],
		"scope":"read:accounts admin:everything"
	}`
	rec := app.request("POST", "/api/v1/oauth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	create := app.hydra.lastClientCreate
	if got := create["token_endpoint_auth_method"]; got != "none" {
		t.Fatalf("expected public client (auth method none), got %v", got)
	}
	if grants := toStringSlice(create["grant_types"]); strings.Join(grants, ",") != "authorization_code,refresh_token" {
		t.Fatalf("expected restricted grants, got %v", grants)
	}
	if resp := toStringSlice(create["response_types"]); strings.Join(resp, ",") != "code" {
		t.Fatalf("expected response_types [code], got %v", resp)
	}
	if scope, _ := create["scope"].(string); scope != "read:accounts" {
		t.Fatalf("expected capped scope 'read:accounts', got %q", scope)
	}

	// The registration is audited (single-user intrusion signal).
	var audits int64
	app.DB.Model(&models.AuditLog{}).Where("action = ?", "OAUTH_CLIENT_REGISTERED").Count(&audits)
	if audits != 1 {
		t.Fatalf("expected 1 OAUTH_CLIENT_REGISTERED audit, got %d", audits)
	}
}

// toStringSlice coerces a JSON-decoded []any into []string for assertions.
func toStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
