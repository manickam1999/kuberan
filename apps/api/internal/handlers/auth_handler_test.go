package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/services"
	"kuberan/internal/validator"
)

// --- mock services ---

type mockUserService struct {
	createUserFn            func(email, password, firstName, lastName string) (*models.User, error)
	getUserByEmailFn        func(email string) (*models.User, error)
	getUserByIDFn           func(id string) (*models.User, error)
	verifyPasswordFn        func(user *models.User, password string) bool
	attemptLoginFn          func(email, password string) (*models.User, error)
	createRefreshSessionFn  func(userID, tokenHash, userAgent, ipAddress string, expiresAt time.Time) error
	rotateRefreshSessionFn  func(userID, oldHash, newHash string, expiresAt time.Time) error
	revokeRefreshSessionFn  func(userID, tokenHash string) error
	updateProfileSettingsFn func(userID string, update services.ProfileSettingsUpdate) error
}

func (m *mockUserService) CreateUser(email, password, firstName, lastName string) (*models.User, error) {
	if m.createUserFn != nil {
		return m.createUserFn(email, password, firstName, lastName)
	}
	return &models.User{}, nil
}

func (m *mockUserService) GetUserByEmail(email string) (*models.User, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(email)
	}
	return &models.User{}, nil
}

func (m *mockUserService) GetUserByID(id string) (*models.User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(id)
	}
	return &models.User{}, nil
}

func (m *mockUserService) VerifyPassword(user *models.User, password string) bool {
	if m.verifyPasswordFn != nil {
		return m.verifyPasswordFn(user, password)
	}
	return true
}

func (m *mockUserService) AttemptLogin(email, password string) (*models.User, error) {
	if m.attemptLoginFn != nil {
		return m.attemptLoginFn(email, password)
	}
	return &models.User{}, nil
}

func (m *mockUserService) CreateRefreshSession(userID, tokenHash, userAgent, ipAddress string, expiresAt time.Time) error {
	if m.createRefreshSessionFn != nil {
		return m.createRefreshSessionFn(userID, tokenHash, userAgent, ipAddress, expiresAt)
	}
	return nil
}

func (m *mockUserService) RotateRefreshSession(userID, oldHash, newHash string, expiresAt time.Time) error {
	if m.rotateRefreshSessionFn != nil {
		return m.rotateRefreshSessionFn(userID, oldHash, newHash, expiresAt)
	}
	return nil
}

func (m *mockUserService) RevokeRefreshSession(userID, tokenHash string) error {
	if m.revokeRefreshSessionFn != nil {
		return m.revokeRefreshSessionFn(userID, tokenHash)
	}
	return nil
}

func (m *mockUserService) UpdateProfileSettings(userID string, update services.ProfileSettingsUpdate) error {
	if m.updateProfileSettingsFn != nil {
		return m.updateProfileSettingsFn(userID, update)
	}
	return nil
}

type mockAuditService struct{}

func (m *mockAuditService) Log(_, _, _, _, _ string, _ map[string]interface{}) {}

// --- test helpers ---

func init() {
	gin.SetMode(gin.TestMode)
	validator.Register()
}

func setupAuthRouter(handler *AuthHandler) *gin.Engine {
	r := gin.New()
	r.POST("/auth/register", handler.Register)
	r.POST("/auth/login", handler.Login)
	r.POST("/auth/refresh", handler.RefreshToken)
	r.POST("/auth/logout", handler.Logout)
	r.GET("/profile", injectUserID("test-user-1"), handler.GetProfile)
	r.PATCH("/profile", injectUserID("test-user-1"), handler.UpdateProfileSettings)
	return r
}

func injectUserID(uid string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userID", uid)
		c.Next()
	}
}

func doRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// doRequestWithCookie is doRequest with the refresh cookie attached.
func doRequestWithCookie(r *gin.Engine, method, path, body, refreshToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: refreshToken})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// refreshCookie returns the kuberan_refresh cookie set on the response, failing
// the test when absent.
func refreshCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie set; headers: %v", RefreshCookieName, rec.Header())
	return nil
}

// assertRefreshCookieIssued checks the cookie attributes from the shared
// contract and that the body no longer carries the refresh token.
func assertRefreshCookieIssued(t *testing.T, rec *httptest.ResponseRecorder, secure bool) *http.Cookie {
	t.Helper()
	c := refreshCookie(t, rec)
	if c.Value == "" {
		t.Error("expected a refresh token in the cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/v1/auth" || c.Domain != "" {
		t.Errorf("unexpected cookie attributes: %+v", c)
	}
	if c.Secure != secure {
		t.Errorf("expected Secure=%v, got %v", secure, c.Secure)
	}
	if c.MaxAge != int(middleware.RefreshTokenExpiry/time.Second) {
		t.Errorf("expected Max-Age %d, got %d", int(middleware.RefreshTokenExpiry/time.Second), c.MaxAge)
	}
	if _, ok := parseJSON(t, rec)["refresh_token"]; ok {
		t.Error("refresh_token must not be returned in the body")
	}
	return c
}

func parseJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v\nbody: %s", err, rec.Body.String())
	}
	return result
}

func assertErrorCode(t *testing.T, result map[string]interface{}, code string) {
	t.Helper()
	errObj, ok := result["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object in response, got: %v", result)
	}
	if errObj["code"] != code {
		t.Errorf("expected error code %q, got %q", code, errObj["code"])
	}
}

// --- tests ---

func TestAuthHandler_Register(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		userSvc := &mockUserService{
			createUserFn: func(email, _, firstName, lastName string) (*models.User, error) {
				return &models.User{
					Base:      models.Base{ID: "1"},
					Email:     email,
					FirstName: firstName,
					LastName:  lastName,
				}, nil
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register",
			`{"email":"test@example.com","password":"password123","first_name":"John","last_name":"Doe"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["access_token"] == nil || result["access_token"] == "" {
			t.Error("expected non-empty access_token")
		}
		assertRefreshCookieIssued(t, rec, true)
		user := result["user"].(map[string]interface{})
		if user["email"] != "test@example.com" {
			t.Errorf("expected email test@example.com, got %v", user["email"])
		}
	})

	t.Run("returns 400 on a password over 72 characters", func(t *testing.T) {
		handler := NewAuthHandler(&mockUserService{}, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register",
			`{"email":"test@example.com","password":"`+strings.Repeat("a", 73)+`"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on missing email", func(t *testing.T) {
		handler := NewAuthHandler(&mockUserService{}, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register", `{"password":"password123"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns 400 on short password", func(t *testing.T) {
		handler := NewAuthHandler(&mockUserService{}, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register", `{"email":"test@example.com","password":"short"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid email format", func(t *testing.T) {
		handler := NewAuthHandler(&mockUserService{}, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register", `{"email":"not-an-email","password":"password123"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 409 on duplicate email", func(t *testing.T) {
		userSvc := &mockUserService{
			createUserFn: func(_, _, _, _ string) (*models.User, error) {
				return nil, apperrors.ErrDuplicateEmail
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register", `{"email":"dup@example.com","password":"password123"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "DUPLICATE_EMAIL")
	})

	t.Run("creates a refresh session", func(t *testing.T) {
		var storedHash string
		userSvc := &mockUserService{
			createUserFn: func(email, _, _, _ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "42"}, Email: email}, nil
			},
			createRefreshSessionFn: func(_, hash, _, _ string, _ time.Time) error {
				storedHash = hash
				return nil
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register", `{"email":"test@example.com","password":"password123"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rec.Code)
		}
		if storedHash == "" {
			t.Error("refresh token hash was not stored")
		}
		if len(storedHash) != 64 {
			t.Errorf("expected SHA-256 hex digest (64 chars), got %d chars", len(storedHash))
		}
	})

	t.Run("returns 500 when token storage fails", func(t *testing.T) {
		userSvc := &mockUserService{
			createUserFn: func(email, _, _, _ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "1"}, Email: email}, nil
			},
			createRefreshSessionFn: func(_, _, _, _ string, _ time.Time) error {
				return fmt.Errorf("db connection lost")
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/register", `{"email":"test@example.com","password":"password123"}`)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestAuthHandler_Login(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		userSvc := &mockUserService{
			attemptLoginFn: func(email, _ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "1"}, Email: email, FirstName: "Test"}, nil
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{}, WithSecureCookie(false))
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/login", `{"email":"test@example.com","password":"password123"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["access_token"] == nil || result["access_token"] == "" {
			t.Error("expected non-empty access_token")
		}
		assertRefreshCookieIssued(t, rec, false)
	})

	t.Run("audits failures with a truncated email", func(t *testing.T) {
		audit := &recordingAudit{}
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return nil, apperrors.ErrInvalidCredentials
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, audit))
		long := strings.Repeat("a", 240) + "@example.com"

		rec := doRequest(r, "POST", "/auth/login", `{"email":"`+long+`","password":"x"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
		e := audit.only(t)
		if e.action != "LOGIN_FAILED" || e.changes["reason"] != "invalid_credentials" {
			t.Errorf("unexpected audit entry %+v", e)
		}
		if got, _ := e.changes["email"].(string); len(got) > maxAuditEmailLen {
			t.Errorf("audit email not truncated: %d chars", len(got))
		}
	})

	t.Run("returns 401 on invalid credentials", func(t *testing.T) {
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return nil, apperrors.ErrInvalidCredentials
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/login", `{"email":"test@example.com","password":"wrong"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_CREDENTIALS")
	})

	t.Run("locked account looks like invalid credentials but is audited distinctly", func(t *testing.T) {
		audit := &recordingAudit{}
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return nil, services.ErrLoginLocked
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, audit))

		rec := doRequest(r, "POST", "/auth/login", `{"email":"locked@example.com","password":"password123"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_CREDENTIALS")
		if e := audit.only(t); e.changes["reason"] != "account_locked" {
			t.Errorf("expected reason account_locked, got %+v", e)
		}
	})

	t.Run("lockout trigger is audited as ACCOUNT_LOCKED", func(t *testing.T) {
		audit := &recordingAudit{}
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return nil, services.ErrLoginLockoutTriggered
			},
			getUserByEmailFn: func(_ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "u9"}}, nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, audit))

		rec := doRequest(r, "POST", "/auth/login", `{"email":"x@example.com","password":"bad"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		if len(audit.entries) != 2 || audit.entries[1].action != "ACCOUNT_LOCKED" || audit.entries[1].userID != "u9" {
			t.Errorf("expected LOGIN_FAILED then ACCOUNT_LOCKED for u9, got %+v", audit.entries)
		}
	})

	t.Run("returns 400 on missing fields", func(t *testing.T) {
		handler := NewAuthHandler(&mockUserService{}, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/login", `{}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestAuthHandler_RefreshToken(t *testing.T) {
	user := &models.User{Base: models.Base{ID: "u1"}, Email: "a@example.com", IsActive: true}
	refreshToken, err := middleware.GenerateRefreshToken(user)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	body := fmt.Sprintf(`{"refresh_token":%q}`, refreshToken)

	t.Run("rotates the presented token within its session", func(t *testing.T) {
		var gotOld, gotNew string
		userSvc := &mockUserService{
			getUserByIDFn: func(_ string) (*models.User, error) { return user, nil },
			rotateRefreshSessionFn: func(userID, oldHash, newHash string, _ time.Time) error {
				if userID != "u1" {
					t.Errorf("rotated session for user %q, want u1", userID)
				}
				gotOld, gotNew = oldHash, newHash
				return nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequestWithCookie(r, "POST", "/auth/refresh", "", refreshToken)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if u, ok := result["user"].(map[string]interface{}); !ok || u["id"] != "u1" {
			t.Errorf("expected the user in the refresh response, got %v", result["user"])
		}
		newToken := assertRefreshCookieIssued(t, rec, true).Value
		if gotOld != middleware.HashToken(refreshToken) {
			t.Error("old hash does not match the presented refresh token")
		}
		if newToken == "" || gotNew != middleware.HashToken(newToken) {
			t.Error("new hash does not match the cookie's refresh token")
		}
	})

	t.Run("falls back to a legacy body token when there is no cookie", func(t *testing.T) {
		var gotOld string
		userSvc := &mockUserService{
			getUserByIDFn: func(_ string) (*models.User, error) { return user, nil },
			rotateRefreshSessionFn: func(_, oldHash, _ string, _ time.Time) error {
				gotOld = oldHash
				return nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/refresh", body)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotOld != middleware.HashToken(refreshToken) {
			t.Error("legacy body token was not used")
		}
		assertRefreshCookieIssued(t, rec, true)
	})

	t.Run("returns 401 with neither cookie nor body", func(t *testing.T) {
		r := setupAuthRouter(NewAuthHandler(&mockUserService{}, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/refresh", "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("returns 401, clears the cookie and audits when no session matches", func(t *testing.T) {
		audit := &recordingAudit{}
		userSvc := &mockUserService{
			getUserByIDFn: func(_ string) (*models.User, error) { return user, nil },
			rotateRefreshSessionFn: func(_, _, _ string, _ time.Time) error {
				return apperrors.ErrUnauthorized
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, audit))

		rec := doRequestWithCookie(r, "POST", "/auth/refresh", "", refreshToken)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		if c := refreshCookie(t, rec); c.MaxAge >= 0 {
			t.Errorf("expected the cookie to be cleared, got %+v", c)
		}
		if e := audit.only(t); e.action != "REFRESH_REJECTED" || e.userID != "u1" {
			t.Errorf("unexpected audit entry %+v", e)
		}
	})

	t.Run("returns 401 for an access token", func(t *testing.T) {
		accessToken, _ := middleware.GenerateAccessToken(user)
		r := setupAuthRouter(NewAuthHandler(&mockUserService{}, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, accessToken))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("returns 401 for an inactive user", func(t *testing.T) {
		userSvc := &mockUserService{
			getUserByIDFn: func(_ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "u1"}, IsActive: false}, nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/refresh", body)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	user := &models.User{Base: models.Base{ID: "u1"}, Email: "a@example.com"}
	refreshToken, err := middleware.GenerateRefreshToken(user)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	t.Run("revokes the cookie's session and clears the cookie", func(t *testing.T) {
		var revoked string
		userSvc := &mockUserService{
			revokeRefreshSessionFn: func(_, hash string) error {
				revoked = hash
				return nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequestWithCookie(r, "POST", "/auth/logout", "", refreshToken)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if revoked != middleware.HashToken(refreshToken) {
			t.Error("revoked hash does not match the cookie's refresh token")
		}
		if c := refreshCookie(t, rec); c.MaxAge >= 0 || c.Value != "" {
			t.Errorf("expected the cookie to be cleared, got %+v", c)
		}
	})

	t.Run("succeeds with no token at all", func(t *testing.T) {
		r := setupAuthRouter(NewAuthHandler(&mockUserService{}, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/logout", "")

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
	})

	t.Run("revokes the session for a legacy body token", func(t *testing.T) {
		var revoked string
		userSvc := &mockUserService{
			revokeRefreshSessionFn: func(_, hash string) error {
				revoked = hash
				return nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/logout", fmt.Sprintf(`{"refresh_token":%q}`, refreshToken))

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if revoked != middleware.HashToken(refreshToken) {
			t.Error("revoked hash does not match the presented refresh token")
		}
	})

	t.Run("succeeds without revoking for an invalid token", func(t *testing.T) {
		userSvc := &mockUserService{
			revokeRefreshSessionFn: func(_, _ string) error {
				t.Error("revoke should not be called for an invalid token")
				return nil
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequest(r, "POST", "/auth/logout", `{"refresh_token":"garbage"}`)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
	})
}

func TestAuthHandler_GetProfile(t *testing.T) {
	t.Run("returns 200 with user profile", func(t *testing.T) {
		now := time.Now()
		userSvc := &mockUserService{
			getUserByIDFn: func(id string) (*models.User, error) {
				return &models.User{
					Base:        models.Base{ID: id},
					Email:       "test@example.com",
					FirstName:   "John",
					LastName:    "Doe",
					LastLoginAt: &now,
				}, nil
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "GET", "/profile", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		user := result["user"].(map[string]interface{})
		if user["email"] != "test@example.com" {
			t.Errorf("expected test@example.com, got %v", user["email"])
		}
		if user["first_name"] != "John" {
			t.Errorf("expected John, got %v", user["first_name"])
		}
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewAuthHandler(&mockUserService{}, &mockAuditService{})
		r := gin.New()
		r.GET("/profile", handler.GetProfile)

		rec := doRequest(r, "GET", "/profile", "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("returns 404 when user not found", func(t *testing.T) {
		userSvc := &mockUserService{
			getUserByIDFn: func(_ string) (*models.User, error) {
				return nil, apperrors.ErrUserNotFound
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "GET", "/profile", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestAuthHandler_UpdateProfileSettings(t *testing.T) {
	const accountID = "01a15c66-01e4-7f03-a5a1-e3e86a73a7f5"

	// recordUpdates returns a mock that captures every UpdateProfileSettings call.
	recordUpdates := func(calls *[]services.ProfileSettingsUpdate) *mockUserService {
		return &mockUserService{
			updateProfileSettingsFn: func(_ string, update services.ProfileSettingsUpdate) error {
				*calls = append(*calls, update)
				return nil
			},
		}
	}

	t.Run("sets the default account and returns it", func(t *testing.T) {
		var calls []services.ProfileSettingsUpdate
		userSvc := recordUpdates(&calls)
		userSvc.getUserByIDFn = func(id string) (*models.User, error) {
			accID := accountID
			return &models.User{Base: models.Base{ID: id}, DefaultAccountID: &accID}, nil
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequest(r, "PATCH", "/profile", `{"default_account_id":"`+accountID+`"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(calls) != 1 {
			t.Fatalf("expected 1 service call, got %d", len(calls))
		}
		got := calls[0]
		if !got.SetDefaultAccount || got.DefaultAccountID == nil || *got.DefaultAccountID != accountID {
			t.Errorf("expected default account %s, got %+v", accountID, got)
		}
		if got.HideBalances != nil {
			t.Error("hide_balances was absent and must not be updated")
		}
		user := parseJSON(t, rec)["user"].(map[string]interface{})
		if user["default_account_id"] != accountID {
			t.Errorf("expected default_account_id %s, got %v", accountID, user["default_account_id"])
		}
	})

	for _, body := range []string{`{"default_account_id":null}`, `{"default_account_id":""}`} {
		t.Run("clears the default account with "+body, func(t *testing.T) {
			var calls []services.ProfileSettingsUpdate
			r := setupAuthRouter(NewAuthHandler(recordUpdates(&calls), &mockAuditService{}))

			rec := doRequest(r, "PATCH", "/profile", body)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if len(calls) != 1 || !calls[0].SetDefaultAccount || calls[0].DefaultAccountID != nil {
				t.Errorf("expected a single clearing update, got %+v", calls)
			}
		})
	}

	t.Run("hide_balances alone leaves the default account untouched", func(t *testing.T) {
		var calls []services.ProfileSettingsUpdate
		audit := &recordingAudit{}
		r := setupAuthRouter(NewAuthHandler(recordUpdates(&calls), audit))

		rec := doRequest(r, "PATCH", "/profile", `{"hide_balances":true}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if e := audit.only(t); e.action != "UPDATE_PROFILE" || e.changes["hide_balances"] != true {
			t.Errorf("unexpected audit entry %+v", e)
		}
		if len(calls) != 1 || calls[0].SetDefaultAccount ||
			calls[0].HideBalances == nil || !*calls[0].HideBalances {
			t.Errorf("expected only hide_balances=true, got %+v", calls)
		}
	})

	t.Run("sends both settings in a single update", func(t *testing.T) {
		var calls []services.ProfileSettingsUpdate
		r := setupAuthRouter(NewAuthHandler(recordUpdates(&calls), &mockAuditService{}))

		rec := doRequest(r, "PATCH", "/profile", `{"hide_balances":true,"default_account_id":"`+accountID+`"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(calls) != 1 {
			t.Fatalf("expected 1 service call, got %d", len(calls))
		}
		got := calls[0]
		if got.HideBalances == nil || !*got.HideBalances || !got.SetDefaultAccount ||
			got.DefaultAccountID == nil || *got.DefaultAccountID != accountID {
			t.Errorf("expected both settings, got %+v", got)
		}
	})

	for name, body := range map[string]string{
		"no setting":          `{}`,
		"non-UUID account id": `{"default_account_id":"abc"}`,
		"non-string value":    `{"default_account_id":123}`,
	} {
		t.Run("returns 400 for "+name, func(t *testing.T) {
			var calls []services.ProfileSettingsUpdate
			r := setupAuthRouter(NewAuthHandler(recordUpdates(&calls), &mockAuditService{}))

			rec := doRequest(r, "PATCH", "/profile", body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if len(calls) != 0 {
				t.Errorf("expected no service call, got %+v", calls)
			}
		})
	}

	t.Run("propagates service errors", func(t *testing.T) {
		userSvc := &mockUserService{
			updateProfileSettingsFn: func(_ string, _ services.ProfileSettingsUpdate) error {
				return apperrors.ErrAccountNotFound
			},
		}
		r := setupAuthRouter(NewAuthHandler(userSvc, &mockAuditService{}))

		rec := doRequest(r, "PATCH", "/profile", `{"default_account_id":"`+accountID+`"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
