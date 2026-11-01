package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"kuberan/internal/config"
	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
)

// TestRegisterRoutes_RateLimitsPublicAuth checks that the limiters are wired
// to the public endpoints from the shared contract and share budgets where
// intended.
func TestRegisterRoutes_RateLimitsPublicAuth(t *testing.T) {
	userSvc := &mockUserService{
		attemptLoginFn: func(_, _ string) (*models.User, error) { return nil, apperrors.ErrInvalidCredentials },
	}
	r := gin.New()
	RegisterRoutes(r, Routes{
		Auth:         NewAuthHandler(userSvc, &mockAuditService{}),
		OAuth:        &OAuthHandler{},
		Registration: &RegistrationHandler{},
		Limits: NewRouteLimits(config.RateLimits{
			LoginPerMinute:      2,
			RegisterPerHour:     1,
			RefreshPerMinute:    1,
			RegistrationPerHour: 1,
		}),
	}, RouteSecrets{})

	login := `{"email":"a@example.com","password":"x"}`
	for i := 0; i < 2; i++ {
		if rec := doRequest(r, "POST", "/api/v1/auth/login", login); rec.Code != http.StatusUnauthorized {
			t.Fatalf("login %d: expected 401, got %d", i+1, rec.Code)
		}
	}
	// The OAuth login shares the per-IP login budget.
	if rec := doRequest(r, "POST", "/api/v1/oauth/login", login); rec.Code != http.StatusTooManyRequests {
		t.Errorf("oauth login after budget: expected 429, got %d", rec.Code)
	}

	if rec := doRequest(r, "POST", "/api/v1/auth/refresh", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("first refresh: expected 401, got %d", rec.Code)
	}
	if rec := doRequest(r, "POST", "/api/v1/auth/refresh", ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("second refresh: expected 429, got %d", rec.Code)
	}

	if rec := doRequest(r, "POST", "/api/v1/auth/register", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("first register: expected 400, got %d", rec.Code)
	}
	if rec := doRequest(r, "POST", "/api/v1/auth/register", `{}`); rec.Code != http.StatusTooManyRequests {
		t.Errorf("second register: expected 429, got %d", rec.Code)
	}

	// Both DCR paths share one budget; the first call fails on its empty body.
	doRequest(r, "POST", "/oauth2/register", `{}`)
	if rec := doRequest(r, "POST", "/api/v1/oauth/register", `{}`); rec.Code != http.StatusTooManyRequests {
		t.Errorf("DCR alias after budget: expected 429, got %d", rec.Code)
	}
}
