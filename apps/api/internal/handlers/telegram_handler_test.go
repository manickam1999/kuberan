package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- mock audit service for telegram tests ---

type mockTelegramAuditService struct{}

func (m *mockTelegramAuditService) Log(_, _, _, _, _ string, _ map[string]interface{}) {
}

var _ services.AuditServicer = (*mockTelegramAuditService)(nil)

// --- test helpers ---

func injectTelegramUserID(uid string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userID", uid)
		c.Next()
	}
}

func doTelegramRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func parseTelegramJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v\nbody: %s", err, rec.Body.String())
	}
	return result
}

// --- mock telegram service ---

type mockTelegramService struct {
	getLinkByUserIDFn      func(userID string) (*models.TelegramLink, error)
	getLinkByTelegramIDFn  func(telegramUserID int64) (*models.TelegramLink, error)
	generateLinkCodeFn     func(userID string) (*models.TelegramLink, error)
	completeLinkFn         func(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error
	unlinkAccountFn        func(userID string) error
	recordActivityFn       func(telegramUserID int64) error
	isLinkedFn             func(userID string) (bool, error)
	getUserWithAuthTokenFn func(telegramUserID int64) (*services.TelegramUserAuth, error)
}

func (m *mockTelegramService) GetLinkByUserID(userID string) (*models.TelegramLink, error) {
	if m.getLinkByUserIDFn != nil {
		return m.getLinkByUserIDFn(userID)
	}
	return &models.TelegramLink{}, nil
}

func (m *mockTelegramService) GetLinkByTelegramID(telegramUserID int64) (*models.TelegramLink, error) {
	if m.getLinkByTelegramIDFn != nil {
		return m.getLinkByTelegramIDFn(telegramUserID)
	}
	return &models.TelegramLink{}, nil
}

func (m *mockTelegramService) GenerateLinkCode(userID string) (*models.TelegramLink, error) {
	if m.generateLinkCodeFn != nil {
		return m.generateLinkCodeFn(userID)
	}
	return &models.TelegramLink{}, nil
}

func (m *mockTelegramService) CompleteLink(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error {
	if m.completeLinkFn != nil {
		return m.completeLinkFn(linkCode, telegramUserID, username, firstName, defaultCurrency)
	}
	return nil
}

func (m *mockTelegramService) UnlinkAccount(userID string) error {
	if m.unlinkAccountFn != nil {
		return m.unlinkAccountFn(userID)
	}
	return nil
}

func (m *mockTelegramService) RecordActivity(telegramUserID int64) error {
	if m.recordActivityFn != nil {
		return m.recordActivityFn(telegramUserID)
	}
	return nil
}

func (m *mockTelegramService) IsLinked(userID string) (bool, error) {
	if m.isLinkedFn != nil {
		return m.isLinkedFn(userID)
	}
	return false, nil
}

func (m *mockTelegramService) GetUserWithAuthToken(telegramUserID int64) (*services.TelegramUserAuth, error) {
	if m.getUserWithAuthTokenFn != nil {
		return m.getUserWithAuthTokenFn(telegramUserID)
	}
	return &services.TelegramUserAuth{}, nil
}

var _ services.TelegramServicer = (*mockTelegramService)(nil)

// --- router setup helpers ---

func setupTelegramRouter(handler *TelegramHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectTelegramUserID("user-123"))
	auth.GET("/telegram/link", handler.GetLink)
	auth.POST("/telegram/generate-code", handler.GenerateCode)
	auth.DELETE("/telegram/unlink", handler.Unlink)
	return r
}

func setupInternalTelegramRouter(handler *TelegramHandler) *gin.Engine {
	r := gin.New()
	r.POST("/internal/telegram/complete-link", handler.CompleteLink)
	r.GET("/internal/telegram/resolve/:telegram_user_id", handler.ResolveUser)
	r.POST("/internal/telegram/activity/:telegram_user_id", handler.RecordActivity)
	return r
}

// --- tests ---

func TestTelegramHandler_GetLink(t *testing.T) {
	t.Run("returns_200_with_link", func(t *testing.T) {
		now := time.Now()
		telegramSvc := &mockTelegramService{
			getLinkByUserIDFn: func(userID string) (*models.TelegramLink, error) {
				return &models.TelegramLink{
					Base:             models.Base{ID: "link-123"},
					UserID:           userID,
					TelegramUserID:   987654321,
					TelegramUsername: "testuser",
					DefaultCurrency:  "MYR",
					IsActive:         true,
					LastMessageAt:    &now,
					MessageCount:     5,
				}, nil
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupTelegramRouter(handler)

		rec := doTelegramRequest(r, "GET", "/telegram/link", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseTelegramJSON(t, rec)
		link := result["link"].(map[string]interface{})
		if link["user_id"] != "user-123" {
			t.Errorf("expected user_id user-123, got %v", link["user_id"])
		}
		if link["telegram_user_id"] != float64(987654321) {
			t.Errorf("expected telegram_user_id 987654321, got %v", link["telegram_user_id"])
		}
	})

	t.Run("returns_404_when_not_found", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			getLinkByUserIDFn: func(userID string) (*models.TelegramLink, error) {
				return nil, apperrors.ErrNotFound
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupTelegramRouter(handler)

		rec := doTelegramRequest(r, "GET", "/telegram/link", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns_401_without_auth", func(t *testing.T) {
		handler := NewTelegramHandler(&mockTelegramService{}, &mockTelegramAuditService{})
		r := gin.New()
		r.GET("/telegram/link", handler.GetLink)

		rec := doTelegramRequest(r, "GET", "/telegram/link", "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestTelegramHandler_GenerateCode(t *testing.T) {
	t.Run("returns_200_with_code", func(t *testing.T) {
		expiresAt := time.Now().Add(15 * time.Minute)
		telegramSvc := &mockTelegramService{
			generateLinkCodeFn: func(userID string) (*models.TelegramLink, error) {
				return &models.TelegramLink{
					Base:              models.Base{ID: "link-123"},
					UserID:            userID,
					LinkCode:          "ABC123",
					LinkCodeExpiresAt: &expiresAt,
				}, nil
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupTelegramRouter(handler)

		rec := doTelegramRequest(r, "POST", "/telegram/generate-code", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseTelegramJSON(t, rec)
		if result["link_code"] != "ABC123" {
			t.Errorf("expected link_code ABC123, got %v", result["link_code"])
		}
		if result["expires_at"] == nil {
			t.Errorf("expected expires_at to be set")
		}
	})

	t.Run("returns_500_on_service_error", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			generateLinkCodeFn: func(userID string) (*models.TelegramLink, error) {
				return nil, apperrors.ErrInternalServer
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupTelegramRouter(handler)

		rec := doTelegramRequest(r, "POST", "/telegram/generate-code", "")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestTelegramHandler_Unlink(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			unlinkAccountFn: func(userID string) error {
				return nil
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupTelegramRouter(handler)

		rec := doTelegramRequest(r, "DELETE", "/telegram/unlink", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseTelegramJSON(t, rec)
		if result["message"] != "Telegram account unlinked successfully" {
			t.Errorf("unexpected message: %v", result["message"])
		}
	})

	t.Run("returns_404_when_not_linked", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			unlinkAccountFn: func(userID string) error {
				return apperrors.ErrNotFound
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupTelegramRouter(handler)

		rec := doTelegramRequest(r, "DELETE", "/telegram/unlink", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestTelegramHandler_CompleteLink(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			completeLinkFn: func(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error {
				if linkCode != "ABC123" {
					t.Errorf("expected linkCode ABC123, got %s", linkCode)
				}
				if telegramUserID != 987654321 {
					t.Errorf("expected telegramUserID 987654321, got %d", telegramUserID)
				}
				return nil
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		body := `{
			"link_code": "ABC123",
			"telegram_user_id": 987654321,
			"telegram_username": "testuser",
			"telegram_first_name": "Test",
			"default_currency": "MYR"
		}`
		rec := doTelegramRequest(r, "POST", "/internal/telegram/complete-link", body)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseTelegramJSON(t, rec)
		if result["message"] != "Telegram account linked successfully" {
			t.Errorf("unexpected message: %v", result["message"])
		}
	})

	t.Run("returns_400_on_invalid_body", func(t *testing.T) {
		handler := NewTelegramHandler(&mockTelegramService{}, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		rec := doTelegramRequest(r, "POST", "/internal/telegram/complete-link", `{"link_code":"ABC"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns_400_on_expired_code", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			completeLinkFn: func(linkCode string, telegramUserID int64, username, firstName, defaultCurrency string) error {
				return apperrors.ErrLinkCodeExpired
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		body := `{
			"link_code": "ABC123",
			"telegram_user_id": 987654321,
			"telegram_username": "testuser",
			"telegram_first_name": "Test"
		}`
		rec := doTelegramRequest(r, "POST", "/internal/telegram/complete-link", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestTelegramHandler_ResolveUser(t *testing.T) {
	t.Run("returns_200_with_user_data", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			getUserWithAuthTokenFn: func(telegramUserID int64) (*services.TelegramUserAuth, error) {
				if telegramUserID != 987654321 {
					t.Errorf("expected telegramUserID 987654321, got %d", telegramUserID)
				}
				return &services.TelegramUserAuth{
					UserID:          "user-123",
					Email:           "test@example.com",
					AuthToken:       "token-abc-123",
					DefaultCurrency: "MYR",
				}, nil
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		rec := doTelegramRequest(r, "GET", "/internal/telegram/resolve/987654321", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseTelegramJSON(t, rec)
		if result["user_id"] != "user-123" {
			t.Errorf("expected user_id user-123, got %v", result["user_id"])
		}
		if result["email"] != "test@example.com" {
			t.Errorf("expected email test@example.com, got %v", result["email"])
		}
		if result["auth_token"] != "token-abc-123" {
			t.Errorf("expected auth_token token-abc-123, got %v", result["auth_token"])
		}
		if result["default_currency"] != "MYR" {
			t.Errorf("expected default_currency MYR, got %v", result["default_currency"])
		}
	})

	t.Run("returns_404_when_not_found", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			getUserWithAuthTokenFn: func(telegramUserID int64) (*services.TelegramUserAuth, error) {
				return nil, apperrors.ErrNotFound
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		rec := doTelegramRequest(r, "GET", "/internal/telegram/resolve/987654321", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns_400_on_invalid_id", func(t *testing.T) {
		handler := NewTelegramHandler(&mockTelegramService{}, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		rec := doTelegramRequest(r, "GET", "/internal/telegram/resolve/invalid", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestTelegramHandler_RecordActivity(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		telegramSvc := &mockTelegramService{
			recordActivityFn: func(telegramUserID int64) error {
				if telegramUserID != 987654321 {
					t.Errorf("expected telegramUserID 987654321, got %d", telegramUserID)
				}
				return nil
			},
		}
		handler := NewTelegramHandler(telegramSvc, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		rec := doTelegramRequest(r, "POST", "/internal/telegram/activity/987654321", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseTelegramJSON(t, rec)
		if result["message"] != "Activity recorded" {
			t.Errorf("unexpected message: %v", result["message"])
		}
	})

	t.Run("returns_400_on_invalid_id", func(t *testing.T) {
		handler := NewTelegramHandler(&mockTelegramService{}, &mockTelegramAuditService{})
		r := setupInternalTelegramRouter(handler)

		rec := doTelegramRequest(r, "POST", "/internal/telegram/activity/invalid", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}
