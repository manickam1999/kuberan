package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"kuberan/internal/hydra"
	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/testutil"
)

const claudeRequestURL = "https://auth.example/oauth2/auth?client_id=known&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback"

func consentWithRedirect(requested ...string) *mockHydraAdmin {
	return &mockHydraAdmin{
		getConsentFn: func(_ context.Context, challenge string) (*hydra.ConsentRequest, error) {
			return &hydra.ConsentRequest{
				Challenge:      challenge,
				Subject:        "user-7",
				Client:         hydra.OAuth2Client{ClientID: "known", ClientName: "Claude"},
				RequestedScope: requested,
				RequestURL:     claudeRequestURL,
			}, nil
		},
	}
}

func TestOAuthHandler_ClientHostRecorded(t *testing.T) {
	t.Run("on AcceptConsent", func(t *testing.T) {
		grants := &mockGrantService{}
		handler := NewOAuthHandler(consentWithRedirect("read:accounts"), &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)
		rec := doRequest(setupOAuthRouter(handler), "POST", "/oauth/consent/accept",
			`{"consent_challenge":"cc1","remember_client":true,"granted_scopes":["read:accounts"]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if grants.recorded == nil || grants.recorded.ClientHost != "claude.ai" {
			t.Errorf("expected client host claude.ai, got %+v", grants.recorded)
		}
	})

	t.Run("on the auto-accept path of GetConsent", func(t *testing.T) {
		grants := &mockGrantService{grant: rememberedGrant("read:accounts", "read:accounts")}
		handler := NewOAuthHandler(consentWithRedirect("read:accounts"), &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)
		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if parseJSON(t, rec)["redirect_to"] == nil {
			t.Fatal("expected the consent to be auto-accepted")
		}
		if grants.hostSet != "claude.ai" || grants.hostSetFor != "known" {
			t.Errorf("auto-accept must record the host, got %q for %q", grants.hostSet, grants.hostSetFor)
		}
	})

	t.Run("a screen that is shown records nothing until it is accepted", func(t *testing.T) {
		grants := &mockGrantService{}
		handler := NewOAuthHandler(consentWithRedirect("read:accounts"), &mockUserService{}, grants, &mockAuditService{}, testScopes, testResourceURL)
		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/consent?consent_challenge=cc1", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if grants.hostSet != "" {
			t.Errorf("no host should be recorded before the user accepts, got %q", grants.hostSet)
		}
	})
}

func TestOAuthHandler_ListConnectedAppsHostAndLimits(t *testing.T) {
	grant := rememberedGrant("read:accounts write:transactions", "read:accounts write:transactions")
	grant.ClientHost = "claude.ai"
	readOnly := rememberedGrant("read:accounts", "read:accounts")

	userSvc := &mockUserService{getUserByIDFn: func(string) (*models.User, error) {
		return &models.User{BaseCurrency: "MYR"}, nil
	}}
	opts := []OAuthHandlerOption{WithWriteLimits(WriteLimitsConfig{MaxAmount: 1_000_000, PerMinute: 30, PerDay: 500})}

	t.Run("a grant with a write scope carries the limits and its host", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, userSvc, &mockGrantService{grant: grant}, &mockAuditService{}, testScopesWithWrite, testResourceURL, opts...)
		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/clients", "")
		clients, _ := parseJSON(t, rec)["clients"].([]interface{})
		first, _ := clients[0].(map[string]interface{})
		if first["client_host"] != "claude.ai" {
			t.Errorf("client_host = %v", first["client_host"])
		}
		limits, _ := first["write_limits"].(map[string]interface{})
		if limits["max_amount"] != float64(1_000_000) || limits["currency"] != "MYR" ||
			limits["per_minute"] != float64(30) || limits["per_day"] != float64(500) {
			t.Errorf("unexpected write_limits: %v", limits)
		}
	})

	t.Run("a read-only grant has no limits and an empty host", func(t *testing.T) {
		handler := NewOAuthHandler(&mockHydraAdmin{}, userSvc, &mockGrantService{grant: readOnly}, &mockAuditService{}, testScopes, testResourceURL, opts...)
		rec := doRequest(setupOAuthRouter(handler), "GET", "/oauth/clients", "")
		clients, _ := parseJSON(t, rec)["clients"].([]interface{})
		first, _ := clients[0].(map[string]interface{})
		if _, has := first["write_limits"]; has {
			t.Errorf("a read-only app must not carry write_limits: %v", first)
		}
		if host, has := first["client_host"]; !has || host != "" {
			t.Errorf("client_host should be present and empty, got %v", first["client_host"])
		}
	})
}

func TestOAuthHandler_AppActivityEndpoints(t *testing.T) {
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)
	userA := testutil.CreateTestUser(t, db)
	userB := testutil.CreateTestUser(t, db)
	grantSvc := services.NewOAuthGrantService(db)
	for _, u := range []*models.User{userA, userB} {
		_, err := grantSvc.RecordConsent(services.RecordConsentInput{
			UserID: u.ID, ClientID: "claude", ClientName: "Claude", Approved: []string{"write:budgets"},
		})
		testutil.AssertNoError(t, err)
	}
	audit := services.NewAuditService(db)
	opts := services.LogOpts{Source: models.AuditSourceMCP, ClientID: "claude"}
	audit.Log(userA.ID, "MCP_CREATE_BUDGET", "budget", "", "", map[string]any{"name": "A food"}, opts)
	audit.Log(userB.ID, "MCP_CREATE_BUDGET", "budget", "", "", map[string]any{"name": "B secret"}, opts)

	handler := NewOAuthHandler(&mockHydraAdmin{}, &mockUserService{}, grantSvc, audit, testScopes, testResourceURL,
		WithAppActivity(services.NewAppActivityService(db)))
	routerFor := func(userID string) *gin.Engine {
		r := gin.New()
		r.GET("/oauth/clients/activity", injectUserID(userID), handler.GetAllAppActivity)
		r.GET("/oauth/clients/:client_id/activity", injectUserID(userID), handler.GetAppActivity)
		return r
	}

	t.Run("per app is scoped to the signed-in user", func(t *testing.T) {
		rec := doRequest(routerFor(userA.ID), "GET", "/oauth/clients/claude/activity", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := parseJSON(t, rec)
		data, _ := body["data"].([]interface{})
		if len(data) != 1 {
			t.Fatalf("expected 1 row, got %v", body)
		}
		row, _ := data[0].(map[string]interface{})
		if row["summary"] != "Created budget · A food" || row["action"] != "MCP_CREATE_BUDGET" || row["resource_type"] != "budget" {
			t.Errorf("unexpected row: %v", row)
		}
		if row["id"] == "" || row["created_at"] == nil {
			t.Errorf("row is missing id or created_at: %v", row)
		}
	})

	t.Run("across apps adds the client id and name and stays scoped", func(t *testing.T) {
		rec := doRequest(routerFor(userB.ID), "GET", "/oauth/clients/activity?page=1&page_size=10", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		data, _ := parseJSON(t, rec)["data"].([]interface{})
		if len(data) != 1 {
			t.Fatalf("expected B's single row, got %v", data)
		}
		row, _ := data[0].(map[string]interface{})
		if row["client_id"] != "claude" || row["client_name"] != "Claude" || row["summary"] != "Created budget · B secret" {
			t.Errorf("unexpected row: %v", row)
		}
	})

	t.Run("rejects an invalid page size", func(t *testing.T) {
		rec := doRequest(routerFor(userA.ID), "GET", "/oauth/clients/activity?page_size=1000", "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}
