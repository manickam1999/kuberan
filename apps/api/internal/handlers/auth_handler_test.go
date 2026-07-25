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
	"kuberan/internal/models"
	"kuberan/internal/validator"
)

// --- mock services ---

type mockUserService struct {
	createUserFn            func(email, password, firstName, lastName string) (*models.User, error)
	getUserByEmailFn        func(email string) (*models.User, error)
	getUserByIDFn           func(id string) (*models.User, error)
	verifyPasswordFn        func(user *models.User, password string) bool
	attemptLoginFn          func(email, password string) (*models.User, error)
	storeRefreshTokenHashFn func(userID string, tokenHash string) error
	getRefreshTokenHashFn   func(userID string) (string, error)
	updateHideBalancesFn    func(userID string, hide bool) error
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

func (m *mockUserService) StoreRefreshTokenHash(userID, tokenHash string) error {
	if m.storeRefreshTokenHashFn != nil {
		return m.storeRefreshTokenHashFn(userID, tokenHash)
	}
	return nil
}

func (m *mockUserService) GetRefreshTokenHash(userID string) (string, error) {
	if m.getRefreshTokenHashFn != nil {
		return m.getRefreshTokenHashFn(userID)
	}
	return "", nil
}

func (m *mockUserService) UpdateHideBalances(userID string, hide bool) error {
	if m.updateHideBalancesFn != nil {
		return m.updateHideBalancesFn(userID, hide)
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
	r.GET("/profile", injectUserID("test-user-1"), handler.GetProfile)
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
		if result["refresh_token"] == nil || result["refresh_token"] == "" {
			t.Error("expected non-empty refresh_token")
		}
		user := result["user"].(map[string]interface{})
		if user["email"] != "test@example.com" {
			t.Errorf("expected email test@example.com, got %v", user["email"])
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

	t.Run("stores refresh token hash", func(t *testing.T) {
		var storedHash string
		userSvc := &mockUserService{
			createUserFn: func(email, _, _, _ string) (*models.User, error) {
				return &models.User{Base: models.Base{ID: "42"}, Email: email}, nil
			},
			storeRefreshTokenHashFn: func(_ string, hash string) error {
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
			storeRefreshTokenHashFn: func(_ string, _ string) error {
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
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/login", `{"email":"test@example.com","password":"password123"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		if result["access_token"] == nil || result["access_token"] == "" {
			t.Error("expected non-empty access_token")
		}
		if result["refresh_token"] == nil || result["refresh_token"] == "" {
			t.Error("expected non-empty refresh_token")
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

	t.Run("returns 423 on locked account", func(t *testing.T) {
		userSvc := &mockUserService{
			attemptLoginFn: func(_, _ string) (*models.User, error) {
				return nil, apperrors.ErrAccountLocked
			},
		}
		handler := NewAuthHandler(userSvc, &mockAuditService{})
		r := setupAuthRouter(handler)

		rec := doRequest(r, "POST", "/auth/login", `{"email":"locked@example.com","password":"password123"}`)

		if rec.Code != http.StatusLocked {
			t.Fatalf("expected 423, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "ACCOUNT_LOCKED")
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
