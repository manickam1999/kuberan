package integration

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kuberan/internal/middleware"
	"kuberan/internal/models"
)

// tokenSID returns the sid claim of an access token ("" when absent).
func tokenSID(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT: %q", token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("parse JWT payload: %v", err)
	}
	return claims.SID
}

// listSessions returns GET /auth/sessions for token.
func (app *testApp) listSessions(t *testing.T, token string) []map[string]interface{} {
	t.Helper()
	rec := app.request("GET", "/api/v1/auth/sessions", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions: %d %s", rec.Code, rec.Body.String())
	}
	raw := parseJSON(t, rec)["sessions"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]interface{}))
	}
	return out
}

func currentSession(t *testing.T, sessions []map[string]interface{}) map[string]interface{} {
	t.Helper()
	var found map[string]interface{}
	for _, s := range sessions {
		if s["current"] == true {
			if found != nil {
				t.Fatal("more than one current session")
			}
			found = s
		}
	}
	if found == nil {
		t.Fatal("no current session")
	}
	return found
}

func (app *testApp) auditCount(t *testing.T, action string) int64 {
	t.Helper()
	var n int64
	if err := app.DB.Model(&models.AuditLog{}).Where("action = ?", action).Count(&n).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	return n
}

func TestAccountSecurity_SessionIDClaim(t *testing.T) {
	app := setupApp(t)
	regAccess, regRefresh, _ := app.registerUser(t, "sid@test.com", "password123")
	loginAccess, loginRefresh := app.loginUser(t, "sid@test.com", "password123")

	regSID, loginSID := tokenSID(t, regAccess), tokenSID(t, loginAccess)
	if regSID == "" || loginSID == "" || regSID == loginSID {
		t.Fatalf("expected distinct non-empty sid claims, got %q and %q", regSID, loginSID)
	}

	// Refresh keeps the session, so the sid is stable across rotations.
	rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", regRefresh)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	refreshed := parseJSON(t, rec)["access_token"].(string)
	if got := tokenSID(t, refreshed); got != regSID {
		t.Errorf("sid after refresh = %q, want %q", got, regSID)
	}

	// current follows the presented token's sid.
	sessions := app.listSessions(t, refreshed)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if id := currentSession(t, sessions)["id"]; id != regSID {
		t.Errorf("current session = %v, want %s", id, regSID)
	}
	if id := currentSession(t, app.listSessions(t, loginAccess))["id"]; id != loginSID {
		t.Errorf("current session = %v, want %s", id, loginSID)
	}
	_ = loginRefresh

	cur := currentSession(t, sessions)
	for _, f := range []string{"id", "created_at", "last_seen_at", "user_agent", "ip_address", "current"} {
		if _, ok := cur[f]; !ok {
			t.Errorf("session missing field %q", f)
		}
	}
	// The rotated session's last_seen_at is its rotation time, not its creation.
	if cur["last_seen_at"] == "" {
		t.Error("last_seen_at is empty")
	}

	// A token without sid (bot/legacy) lists sessions but marks none current.
	rec = app.request("GET", "/api/v1/auth/sessions", "", mustLegacyToken(t, app, "sid@test.com"))
	if rec.Code != http.StatusOK {
		t.Fatalf("list with sid-less token: %d %s", rec.Code, rec.Body.String())
	}
	for _, s := range parseJSON(t, rec)["sessions"].([]interface{}) {
		if s.(map[string]interface{})["current"] == true {
			t.Error("no session should be current for a token without sid")
		}
	}
}

// mustLegacyToken signs an access token without a sid claim.
func mustLegacyToken(t *testing.T, app *testApp, email string) string {
	t.Helper()
	var user models.User
	if err := app.DB.Where("email = ?", email).First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	token, err := middleware.GenerateAccessToken(&user)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	return token
}

func TestAccountSecurity_SessionsRequireAuth(t *testing.T) {
	app := setupApp(t)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/sessions"},
		{"DELETE", "/api/v1/auth/sessions/00000000-0000-7000-8000-000000000000"},
		{"POST", "/api/v1/auth/sessions/revoke-others"},
		{"POST", "/api/v1/auth/password"},
	} {
		if rec := app.request(tc.method, tc.path, `{}`, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

func TestAccountSecurity_RevokeOneSession(t *testing.T) {
	app := setupApp(t)
	access, refresh, _ := app.registerUser(t, "revoke1@test.com", "password123")
	otherAccess, otherRefresh := app.loginUser(t, "revoke1@test.com", "password123")
	otherSID := tokenSID(t, otherAccess)
	mySID := tokenSID(t, access)

	// The current session cannot be revoked this way.
	rec := app.request("DELETE", "/api/v1/auth/sessions/"+mySID, "", access)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoking current: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec, "CURRENT_SESSION")

	rec = app.request("DELETE", "/api/v1/auth/sessions/not-a-uuid", "", access)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: expected 400, got %d", rec.Code)
	}
	rec = app.request("DELETE", "/api/v1/auth/sessions/00000000-0000-7000-8000-000000000000", "", access)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown session: expected 404, got %d", rec.Code)
	}

	// Another user's session is indistinguishable from a missing one.
	strangerAccess, _, _ := app.registerUser(t, "stranger@test.com", "password123")
	rec = app.request("DELETE", "/api/v1/auth/sessions/"+otherSID, "", strangerAccess)
	if rec.Code != http.StatusNotFound {
		t.Errorf("foreign session: expected 404, got %d", rec.Code)
	}
	if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", otherRefresh); rec.Code != http.StatusOK {
		t.Fatalf("foreign delete must not end the session, refresh got %d", rec.Code)
	}

	rec = app.request("DELETE", "/api/v1/auth/sessions/"+otherSID, "", access)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke other: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", otherRefresh); rec.Code != http.StatusUnauthorized {
		t.Errorf("revoked device refresh: expected 401, got %d", rec.Code)
	}
	if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", refresh); rec.Code != http.StatusOK {
		t.Errorf("current device refresh: expected 200, got %d", rec.Code)
	}
	if n := len(app.listSessions(t, access)); n != 1 {
		t.Errorf("expected 1 live session left, got %d", n)
	}
	if n := app.auditCount(t, "REVOKE_SESSION"); n != 1 {
		t.Errorf("expected 1 REVOKE_SESSION audit entry, got %d", n)
	}
}

func TestAccountSecurity_RevokeOtherSessions(t *testing.T) {
	app := setupApp(t)
	access, refresh, _ := app.registerUser(t, "revokeall@test.com", "password123")
	_, r2 := app.loginUser(t, "revokeall@test.com", "password123")
	_, r3 := app.loginUser(t, "revokeall@test.com", "password123")

	// A token without sid cannot say which session to keep.
	rec := app.request("POST", "/api/v1/auth/sessions/revoke-others", "", mustLegacyToken(t, app, "revokeall@test.com"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke-others without sid: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := len(app.listSessions(t, access)); n != 3 {
		t.Fatalf("refused revoke-others must keep all sessions, got %d", n)
	}

	rec = app.request("POST", "/api/v1/auth/sessions/revoke-others", "", access)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke-others: %d %s", rec.Code, rec.Body.String())
	}
	if got := parseJSON(t, rec)["revoked_sessions"]; got != float64(2) {
		t.Errorf("revoked_sessions = %v, want 2", got)
	}
	for _, r := range []string{r2, r3} {
		if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", r); rec.Code != http.StatusUnauthorized {
			t.Errorf("revoked device refresh: expected 401, got %d", rec.Code)
		}
	}
	if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", refresh); rec.Code != http.StatusOK {
		t.Errorf("current device refresh: expected 200, got %d", rec.Code)
	}
	if n := app.auditCount(t, "REVOKE_OTHER_SESSIONS"); n != 1 {
		t.Errorf("expected 1 REVOKE_OTHER_SESSIONS audit entry, got %d", n)
	}
}

func TestAccountSecurity_ChangePassword(t *testing.T) {
	app := setupApp(t)
	access, refresh, _ := app.registerUser(t, "pw@test.com", "password123")
	_, otherRefresh := app.loginUser(t, "pw@test.com", "password123")

	change := func(cur, next string) *httptest.ResponseRecorder {
		return app.request("POST", "/api/v1/auth/password",
			fmt.Sprintf(`{"current_password":%q,"new_password":%q}`, cur, next), access)
	}

	rec := change("password123", "short")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("too short: expected 400, got %d", rec.Code)
	}
	rec = change("password123", strings.Repeat("a", 73))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("too long: expected 400, got %d", rec.Code)
	}

	rec = change("wrong-password", "newpassword456")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong current: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec, "INVALID_CURRENT_PASSWORD")
	var user models.User
	if err := app.DB.Where("email = ?", "pw@test.com").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.FailedLoginAttempts != 1 {
		t.Errorf("wrong current password must count toward lockout, attempts = %d", user.FailedLoginAttempts)
	}
	if n := len(app.listSessions(t, access)); n != 2 {
		t.Errorf("a failed change must not revoke sessions, got %d", n)
	}

	rec = change("password123", "newpassword456")
	if rec.Code != http.StatusOK {
		t.Fatalf("change: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := parseJSON(t, rec)["revoked_sessions"]; got != float64(1) {
		t.Errorf("revoked_sessions = %v, want 1", got)
	}
	if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", otherRefresh); rec.Code != http.StatusUnauthorized {
		t.Errorf("other device after password change: expected 401, got %d", rec.Code)
	}
	if rec := app.requestWithRefreshCookie("POST", "/api/v1/auth/refresh", refresh); rec.Code != http.StatusOK {
		t.Errorf("current device after password change: expected 200, got %d", rec.Code)
	}

	if rec := app.request("POST", "/api/v1/auth/login", `{"email":"pw@test.com","password":"password123"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("old password login: expected 401, got %d", rec.Code)
	}
	app.loginUser(t, "pw@test.com", "newpassword456")

	if n := app.auditCount(t, "CHANGE_PASSWORD"); n != 1 {
		t.Errorf("expected 1 CHANGE_PASSWORD audit entry, got %d", n)
	}
	if n := app.auditCount(t, "CHANGE_PASSWORD_FAILED"); n != 1 {
		t.Errorf("expected 1 CHANGE_PASSWORD_FAILED audit entry, got %d", n)
	}
}

func TestAccountSecurity_ChangePasswordLockout(t *testing.T) {
	app := setupApp(t)
	access, _, _ := app.registerUser(t, "pwlock@test.com", "password123")

	var last *httptest.ResponseRecorder
	for i := 0; i < 5; i++ {
		last = app.request("POST", "/api/v1/auth/password", `{"current_password":"nope-nope","new_password":"newpassword456"}`, access)
	}
	if last.Code != http.StatusLocked {
		t.Fatalf("5th wrong attempt: expected 423, got %d: %s", last.Code, last.Body.String())
	}
	// Locked: even the right password is refused, on login and on change.
	if rec := app.request("POST", "/api/v1/auth/password", `{"current_password":"password123","new_password":"newpassword456"}`, access); rec.Code != http.StatusLocked {
		t.Errorf("locked change: expected 423, got %d", rec.Code)
	}
	if rec := app.request("POST", "/api/v1/auth/login", `{"email":"pwlock@test.com","password":"password123"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("locked login: expected 401, got %d", rec.Code)
	}
}

func TestAccountSecurity_UpdateProfileNameAndEmail(t *testing.T) {
	app := setupApp(t)
	access, _, _ := app.registerUser(t, "prof@test.com", "password123")
	app.registerUser(t, "taken@test.com", "password123")

	patch := func(body string) *httptest.ResponseRecorder {
		return app.request("PATCH", "/api/v1/profile", body, access)
	}

	// Names need no password; existing fields keep working.
	rec := patch(`{"first_name":"  Ada ","last_name":"Lovelace","hide_balances":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("names: %d %s", rec.Code, rec.Body.String())
	}
	u := parseJSON(t, rec)["user"].(map[string]interface{})
	if u["first_name"] != "Ada" || u["last_name"] != "Lovelace" || u["hide_balances"] != true {
		t.Errorf("unexpected user after name update: %v", u)
	}

	// An email change needs the current password.
	rec = patch(`{"email":"new@test.com"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("email without password: expected 400, got %d", rec.Code)
	}
	rec = patch(`{"email":"new@test.com","current_password":"wrong-password"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("email with wrong password: expected 400, got %d", rec.Code)
	}
	assertErrorCode(t, rec, "INVALID_CURRENT_PASSWORD")

	// Uniqueness is checked after trimming and lower-casing.
	rec = patch(`{"email":"TAKEN@Test.com","current_password":"password123"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate email: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec, "DUPLICATE_EMAIL")

	rec = patch(`{"email":"not-an-email","current_password":"password123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed email: expected 400, got %d", rec.Code)
	}

	rec = patch(`{"email":"New@Test.com","current_password":"password123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("email change: %d %s", rec.Code, rec.Body.String())
	}
	if got := parseJSON(t, rec)["user"].(map[string]interface{})["email"]; got != "new@test.com" {
		t.Errorf("email = %v, want new@test.com", got)
	}
	app.loginUser(t, "new@test.com", "password123")
	if rec := app.request("POST", "/api/v1/auth/login", `{"email":"prof@test.com","password":"password123"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("login with old email: expected 401, got %d", rec.Code)
	}

	// Re-submitting the current address needs no password and is a no-op.
	rec = patch(`{"email":"new@test.com"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("unchanged email: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if n := app.auditCount(t, "UPDATE_EMAIL"); n != 1 {
		t.Errorf("expected 1 UPDATE_EMAIL audit entry, got %d", n)
	}
	if n := app.auditCount(t, "UPDATE_PROFILE"); n != 1 {
		t.Errorf("expected 1 UPDATE_PROFILE audit entry, got %d", n)
	}
}

func TestAccountSecurity_LastLoginInResponses(t *testing.T) {
	app := setupApp(t)
	app.registerUser(t, "last@test.com", "password123")

	rec := app.request("POST", "/api/v1/auth/login", `{"email":"last@test.com","password":"password123"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	user := parseJSON(t, rec)["user"].(map[string]interface{})
	if s, _ := user["last_login_at"].(string); s == "" {
		t.Errorf("login response last_login_at = %v, want a timestamp", user["last_login_at"])
	}

	access := parseJSON(t, rec)["access_token"].(string)
	rec = app.request("GET", "/api/v1/profile", "", access)
	user = parseJSON(t, rec)["user"].(map[string]interface{})
	if s, _ := user["last_login_at"].(string); s == "" {
		t.Errorf("profile last_login_at = %v, want a timestamp", user["last_login_at"])
	}
}

// assertErrorCode checks the {"error":{"code":...}} body of rec.
func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse error body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != code {
		t.Errorf("error code = %q, want %q (body %s)", body.Error.Code, code, rec.Body.String())
	}
}
