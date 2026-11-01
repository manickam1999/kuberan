package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kuberan/internal/models"
)

func TestAuthFlow_RegisterLoginProfileRefresh(t *testing.T) {
	app := setupApp(t)

	// Step 1: Register
	accessToken, refreshToken, userID := app.registerUser(t, "auth@test.com", "password123")
	if accessToken == "" || refreshToken == "" {
		t.Fatal("expected non-empty tokens from registration")
	}
	if userID == "" {
		t.Fatal("expected non-empty user ID")
	}

	// Step 2: Login with same credentials
	loginAccess, loginRefresh := app.loginUser(t, "auth@test.com", "password123")
	if loginAccess == "" || loginRefresh == "" {
		t.Fatal("expected non-empty tokens from login")
	}

	// Step 3: Access profile with login access token
	rec := app.request("GET", "/api/v1/profile", "", loginAccess)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	user := result["user"].(map[string]interface{})
	if user["email"] != "auth@test.com" {
		t.Errorf("expected email auth@test.com, got %v", user["email"])
	}

	// Step 4: Refresh token (from the HttpOnly cookie)
	rec = app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", loginRefresh)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
	refreshResult := parseJSON(t, rec)
	newAccess := refreshResult["access_token"].(string)
	if newAccess == "" {
		t.Fatal("expected non-empty new access token after refresh")
	}
	if _, ok := refreshResult["refresh_token"]; ok {
		t.Error("refresh_token must not be in the response body")
	}
	if u, ok := refreshResult["user"].(map[string]interface{}); !ok || u["email"] != "auth@test.com" {
		t.Errorf("expected the user in the refresh response, got %v", refreshResult["user"])
	}
	rotated := refreshCookieValue(t, rec)

	// Legacy migration path: a body token is still accepted when no cookie is sent.
	rec = app.request("POST", "/api/v1/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, rotated), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy body refresh failed: %d %s", rec.Code, rec.Body.String())
	}

	// Step 5: Access profile with new access token
	rec = app.request("GET", "/api/v1/profile", "", newAccess)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with new token, got %d: %s", rec.Code, rec.Body.String())
	}

	// Note: Token rotation (old refresh token invalidated after use) is not tested here
	// because JWTs generated within the same second for the same user are identical,
	// making the hash comparison pass even after rotation. This is a known limitation
	// of the current token generation (no random jti claim).
}

func TestAuthFlow_RegisterDuplicateEmail(t *testing.T) {
	app := setupApp(t)

	app.registerUser(t, "dup@test.com", "password123")

	// Try to register again with same email
	rec := app.request("POST", "/api/v1/auth/register",
		`{"email":"dup@test.com","password":"password123"}`, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate email, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	errObj := result["error"].(map[string]interface{})
	if errObj["code"] != "DUPLICATE_EMAIL" {
		t.Errorf("expected DUPLICATE_EMAIL, got %v", errObj["code"])
	}
}

func TestAuthFlow_LoginWrongPassword(t *testing.T) {
	app := setupApp(t)

	app.registerUser(t, "wrong@test.com", "password123")

	rec := app.request("POST", "/api/v1/auth/login",
		`{"email":"wrong@test.com","password":"wrongpassword"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	errObj := result["error"].(map[string]interface{})
	if errObj["code"] != "INVALID_CREDENTIALS" {
		t.Errorf("expected INVALID_CREDENTIALS, got %v", errObj["code"])
	}
}

func TestAuthFlow_AccountLockout(t *testing.T) {
	app := setupApp(t)

	app.registerUser(t, "lockout@test.com", "password123")

	// Fail 5 times
	for i := 0; i < 5; i++ {
		rec := app.request("POST", "/api/v1/auth/login",
			`{"email":"lockout@test.com","password":"wrong"}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	// While locked, responses are indistinguishable from bad credentials
	// (no account enumeration), even with the correct password.
	for _, password := range []string{"wrong", "password123"} {
		rec := app.request("POST", "/api/v1/auth/login",
			`{"email":"lockout@test.com","password":"`+password+`"}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 while locked, got %d: %s", rec.Code, rec.Body.String())
		}
		errObj := parseJSON(t, rec)["error"].(map[string]interface{})
		if errObj["code"] != "INVALID_CREDENTIALS" {
			t.Errorf("expected INVALID_CREDENTIALS, got %v", errObj["code"])
		}
	}

	// The lockout and the refused attempts are audited distinctly.
	var locked int64
	app.DB.Model(&models.AuditLog{}).Where("action = ?", "ACCOUNT_LOCKED").Count(&locked)
	if locked != 1 {
		t.Errorf("expected one ACCOUNT_LOCKED audit entry, got %d", locked)
	}
}

func TestAuthFlow_EmailCaseInsensitive(t *testing.T) {
	app := setupApp(t)

	app.registerUser(t, "alice@test.com", "password123")

	rec := app.request("POST", "/api/v1/auth/register",
		`{"email":"Alice@Test.com","password":"password123"}`, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a case-variant duplicate, got %d: %s", rec.Code, rec.Body.String())
	}

	app.loginUser(t, "ALICE@test.com", "password123")
}

func TestAuthFlow_LogoutRevokesCookieSession(t *testing.T) {
	app := setupApp(t)

	_, refresh, _ := app.registerUser(t, "logout@test.com", "password123")

	rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/logout", refresh)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	rec = app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", refresh)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", rec.Code)
	}
}

func TestSecurityHeadersAndCORS(t *testing.T) {
	app := setupApp(t)

	req := httptest.NewRequest("OPTIONS", "/api/v1/auth/login", http.NoBody)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent ||
		rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed origin preflight: %d %v", rec.Code, rec.Header())
	}

	req = httptest.NewRequest("GET", "/api/v1/profile", http.NoBody)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	app.Router.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin got ACAO %q", got)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("missing security headers: %v", rec.Header())
	}
}

func TestBodySizeLimit(t *testing.T) {
	app := setupApp(t)

	big := `{"email":"a@test.com","password":"` + strings.Repeat("a", 2<<20) + `"}`
	rec := app.request("POST", "/api/v1/auth/login", big, "")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for a 2 MiB JSON body, got %d", rec.Code)
	}
}

func TestAuthFlow_ProfileWithoutAuth(t *testing.T) {
	app := setupApp(t)

	rec := app.request("GET", "/api/v1/profile", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthFlow_ProfileWithInvalidToken(t *testing.T) {
	app := setupApp(t)

	rec := app.request("GET", "/api/v1/profile", "", "invalid-token")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
