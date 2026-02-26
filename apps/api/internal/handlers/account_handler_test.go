package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// --- mock account service ---

type mockAccountService struct {
	createCashAccountFn       func(userID string, name, description, currency string, initialBalance int64) (*models.Account, error)
	createInvestmentAccountFn func(userID string, name, description, currency, broker, accountNumber string) (*models.Account, error)
	createCreditCardAccountFn func(userID string, name, description, currency string, creditLimit int64, interestRate float64, dueDate *time.Time) (*models.Account, error)
	getUserAccountsFn         func(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Account], error)
	getAccountByIDFn          func(userID, accountID string) (*models.Account, error)
	updateAccountFn           func(userID, accountID string, updates services.AccountUpdateFields) (*models.Account, error)
	updateAccountBalanceFn    func(tx *gorm.DB, account *models.Account, transactionType models.TransactionType, amount int64) error
}

func (m *mockAccountService) CreateCashAccount(userID string, name, description, currency string, initialBalance int64) (*models.Account, error) {
	if m.createCashAccountFn != nil {
		return m.createCashAccountFn(userID, name, description, currency, initialBalance)
	}
	return &models.Account{}, nil
}

func (m *mockAccountService) CreateInvestmentAccount(userID string, name, description, currency, broker, accountNumber string) (*models.Account, error) {
	if m.createInvestmentAccountFn != nil {
		return m.createInvestmentAccountFn(userID, name, description, currency, broker, accountNumber)
	}
	return &models.Account{}, nil
}

func (m *mockAccountService) CreateCreditCardAccount(userID string, name, description, currency string, creditLimit int64, interestRate float64, dueDate *time.Time) (*models.Account, error) {
	if m.createCreditCardAccountFn != nil {
		return m.createCreditCardAccountFn(userID, name, description, currency, creditLimit, interestRate, dueDate)
	}
	return &models.Account{}, nil
}

func (m *mockAccountService) GetUserAccounts(userID string, page pagination.PageRequest) (*pagination.PageResponse[models.Account], error) {
	if m.getUserAccountsFn != nil {
		return m.getUserAccountsFn(userID, page)
	}
	resp := pagination.NewPageResponse([]models.Account{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockAccountService) GetAccountByID(userID, accountID string) (*models.Account, error) {
	if m.getAccountByIDFn != nil {
		return m.getAccountByIDFn(userID, accountID)
	}
	return &models.Account{}, nil
}

func (m *mockAccountService) UpdateAccount(userID, accountID string, updates services.AccountUpdateFields) (*models.Account, error) {
	if m.updateAccountFn != nil {
		return m.updateAccountFn(userID, accountID, updates)
	}
	return &models.Account{}, nil
}

func (m *mockAccountService) UpdateAccountBalance(tx *gorm.DB, account *models.Account, transactionType models.TransactionType, amount int64) error {
	if m.updateAccountBalanceFn != nil {
		return m.updateAccountBalanceFn(tx, account, transactionType, amount)
	}
	return nil
}

// verify interface compliance
var _ services.AccountServicer = (*mockAccountService)(nil)

func setupAccountRouter(handler *AccountHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.POST("/accounts/cash", handler.CreateCashAccount)
	auth.POST("/accounts/investment", handler.CreateInvestmentAccount)
	auth.POST("/accounts/credit-card", handler.CreateCreditCardAccount)
	auth.GET("/accounts", handler.GetUserAccounts)
	auth.GET("/accounts/:id", handler.GetAccountByID)
	auth.PUT("/accounts/:id", handler.UpdateAccount)
	return r
}

func TestAccountHandler_CreateCashAccount(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		acctSvc := &mockAccountService{
			createCashAccountFn: func(userID string, name, desc, currency string, balance int64) (*models.Account, error) {
				return &models.Account{
					Base:     models.Base{ID: "test-id-1"},
					UserID:   userID,
					Name:     name,
					Type:     models.AccountTypeCash,
					Balance:  balance,
					Currency: currency,
					IsActive: true,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/cash",
			`{"name":"Savings","currency":"USD","initial_balance":5000}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		acct := result["account"].(map[string]interface{})
		if acct["name"] != "Savings" {
			t.Errorf("expected Savings, got %v", acct["name"])
		}
	})

	t.Run("returns 400 on missing name", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/cash", `{"currency":"USD"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns 400 on invalid currency", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/cash", `{"name":"Test","currency":"INVALID"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on negative initial balance", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/cash", `{"name":"Test","initial_balance":-100}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/accounts/cash", handler.CreateCashAccount)

		rec := doRequest(r, "POST", "/accounts/cash", `{"name":"Test"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestAccountHandler_CreateInvestmentAccount(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		acctSvc := &mockAccountService{
			createInvestmentAccountFn: func(userID string, name, desc, currency, broker, acctNum string) (*models.Account, error) {
				return &models.Account{
					Base:     models.Base{ID: "test-id-2"},
					UserID:   userID,
					Name:     name,
					Type:     models.AccountTypeInvestment,
					Currency: currency,
					Broker:   broker,
					IsActive: true,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/investment",
			`{"name":"Brokerage","broker":"Fidelity","account_number":"123"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestAccountHandler_GetUserAccounts(t *testing.T) {
	t.Run("returns 200 with paginated accounts", func(t *testing.T) {
		acctSvc := &mockAccountService{
			getUserAccountsFn: func(_ string, _ pagination.PageRequest) (*pagination.PageResponse[models.Account], error) {
				resp := pagination.NewPageResponse([]models.Account{
					{Base: models.Base{ID: "1"}, Name: "Cash"},
					{Base: models.Base{ID: "2"}, Name: "Investment"},
				}, 1, 20, 2)
				return &resp, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "GET", "/accounts", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 accounts, got %d", len(data))
		}
		if result["total_items"].(float64) != 2 {
			t.Errorf("expected total_items=2, got %v", result["total_items"])
		}
	})

	t.Run("passes pagination params to service", func(t *testing.T) {
		var capturedPage pagination.PageRequest
		acctSvc := &mockAccountService{
			getUserAccountsFn: func(_ string, page pagination.PageRequest) (*pagination.PageResponse[models.Account], error) {
				capturedPage = page
				resp := pagination.NewPageResponse([]models.Account{}, 2, 5, 0)
				return &resp, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		doRequest(r, "GET", "/accounts?page=2&page_size=5", "")

		if capturedPage.Page != 2 {
			t.Errorf("expected page=2, got %d", capturedPage.Page)
		}
		if capturedPage.PageSize != 5 {
			t.Errorf("expected page_size=5, got %d", capturedPage.PageSize)
		}
	})
}

func TestAccountHandler_GetAccountByID(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		acctSvc := &mockAccountService{
			getAccountByIDFn: func(_, accountID string) (*models.Account, error) {
				return &models.Account{
					Base: models.Base{ID: accountID},
					Name: "Savings",
					Type: models.AccountTypeCash,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "GET", "/accounts/1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		acct := result["account"].(map[string]interface{})
		if acct["name"] != "Savings" {
			t.Errorf("expected Savings, got %v", acct["name"])
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		acctSvc := &mockAccountService{
			getAccountByIDFn: func(_, _ string) (*models.Account, error) {
				return nil, apperrors.ErrAccountNotFound
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "GET", "/accounts/999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "ACCOUNT_NOT_FOUND")
	})

	t.Run("returns 400 on invalid ID", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "GET", "/accounts/abc", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestAccountHandler_UpdateAccount(t *testing.T) {
	t.Run("returns_200_with_name_update", func(t *testing.T) {
		acctSvc := &mockAccountService{
			updateAccountFn: func(_, accountID uint, updates services.AccountUpdateFields) (*models.Account, error) {
				name := ""
				if updates.Name != nil {
					name = *updates.Name
				}
				desc := ""
				if updates.Description != nil {
					desc = *updates.Description
				}
				return &models.Account{
					Base:        models.Base{ID: accountID},
					Name:        name,
					Description: desc,
					Type:        models.AccountTypeCash,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "PUT", "/accounts/1", `{"name":"Updated","description":"New desc"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		acct := result["account"].(map[string]interface{})
		if acct["name"] != "Updated" {
			t.Errorf("expected Updated, got %v", acct["name"])
		}
	})

	t.Run("returns_200_with_investment_fields", func(t *testing.T) {
		var captured services.AccountUpdateFields
		acctSvc := &mockAccountService{
			updateAccountFn: func(_, accountID uint, updates services.AccountUpdateFields) (*models.Account, error) {
				captured = updates
				return &models.Account{
					Base: models.Base{ID: accountID},
					Type: models.AccountTypeInvestment,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "PUT", "/accounts/1", `{"broker":"Schwab","account_number":"XYZ"}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if captured.Broker == nil || *captured.Broker != "Schwab" {
			t.Errorf("expected broker Schwab, got %v", captured.Broker)
		}
		if captured.AccountNumber == nil || *captured.AccountNumber != "XYZ" {
			t.Errorf("expected account_number XYZ, got %v", captured.AccountNumber)
		}
	})

	t.Run("returns_200_with_credit_card_fields", func(t *testing.T) {
		var captured services.AccountUpdateFields
		acctSvc := &mockAccountService{
			updateAccountFn: func(_, accountID uint, updates services.AccountUpdateFields) (*models.Account, error) {
				captured = updates
				return &models.Account{
					Base: models.Base{ID: accountID},
					Type: models.AccountTypeCreditCard,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "PUT", "/accounts/1", `{"interest_rate":22.5,"credit_limit":1000000}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if captured.InterestRate == nil || *captured.InterestRate != 22.5 {
			t.Errorf("expected interest_rate 22.5, got %v", captured.InterestRate)
		}
		if captured.CreditLimit == nil || *captured.CreditLimit != 1000000 {
			t.Errorf("expected credit_limit 1000000, got %v", captured.CreditLimit)
		}
	})

	t.Run("returns_404_when_not_found", func(t *testing.T) {
		acctSvc := &mockAccountService{
			updateAccountFn: func(_, _ uint, _ services.AccountUpdateFields) (*models.Account, error) {
				return nil, apperrors.ErrAccountNotFound
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "PUT", "/accounts/999", `{"name":"Updated"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "ACCOUNT_NOT_FOUND")
	})
}

func TestAccountHandler_CreateCreditCardAccount(t *testing.T) {
	t.Run("returns 201 with valid request", func(t *testing.T) {
		acctSvc := &mockAccountService{
			createCreditCardAccountFn: func(userID string, name, desc, currency string, creditLimit int64, interestRate float64, dueDate *time.Time) (*models.Account, error) {
				return &models.Account{
					Base:         models.Base{ID: "test-id-3"},
					UserID:       userID,
					Name:         name,
					Type:         models.AccountTypeCreditCard,
					Currency:     "USD",
					CreditLimit:  creditLimit,
					InterestRate: interestRate,
					IsActive:     true,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/credit-card",
			`{"name":"Visa","credit_limit":500000,"interest_rate":19.99}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		acct := result["account"].(map[string]interface{})
		if acct["name"] != "Visa" {
			t.Errorf("expected Visa, got %v", acct["name"])
		}
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/credit-card", `{"credit_limit":500000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns 400 for negative credit limit", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := setupAccountRouter(handler)

		rec := doRequest(r, "POST", "/accounts/credit-card", `{"name":"Visa","credit_limit":-1}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/accounts/credit-card", handler.CreateCreditCardAccount)

		rec := doRequest(r, "POST", "/accounts/credit-card", `{"name":"Visa"}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}
