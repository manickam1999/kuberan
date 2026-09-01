package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

// --- mock transaction service ---

type mockTransactionService struct {
	createTransactionFn      func(userID, accountID string, categoryID *string, transactionType models.TransactionType, amount int64, description string, date time.Time) (*models.Transaction, error)
	createTransferFn         func(userID, fromAccountID, toAccountID string, amount int64, description string, date time.Time) (*models.Transaction, error)
	getAccountTransactionsFn func(userID, accountID string, page pagination.PageRequest, filter services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	getUserTransactionsFn    func(userID string, page pagination.PageRequest, filter services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error)
	getTransactionByIDFn     func(userID, transactionID string) (*models.Transaction, error)
	updateTransactionFn      func(userID, transactionID string, updates services.TransactionUpdateFields) (*models.Transaction, error)
	deleteTransactionFn      func(userID, transactionID string) error
	getSpendingByCategoryFn  func(userID string, from, to time.Time) (*services.SpendingByCategory, error)
	getCashflowFn            func(userID string, from, to time.Time) (*services.Cashflow, error)
	getMonthlySummaryFn      func(userID string, months int) ([]services.MonthlySummaryItem, error)
	getDailySpendingFn       func(userID string, from, to time.Time) ([]services.DailySpendingItem, error)
	previewRuleMatchesFn     func(userID string, conditions []services.RuleConditionInput) (*services.RuleMatchPreview, error)
	applyRuleFn              func(userID, ruleID string, opts services.ApplyRuleOptions) (*services.ApplyRuleResult, error)
	getDailySummaryFn        func(userID string, from, to time.Time) ([]services.DailySummaryItem, error)
	getTopExpensesFn         func(userID string, from, to time.Time, limit int, categoryID *string) (*services.TopExpenses, error)
}

func (m *mockTransactionService) CreateTransaction(userID, accountID string, categoryID *string, transactionType models.TransactionType, amount int64, description string, date time.Time) (*models.Transaction, error) {
	if m.createTransactionFn != nil {
		return m.createTransactionFn(userID, accountID, categoryID, transactionType, amount, description, date)
	}
	return &models.Transaction{}, nil
}

func (m *mockTransactionService) CreateTransfer(userID, fromAccountID, toAccountID string, amount int64, description string, date time.Time) (*models.Transaction, error) {
	if m.createTransferFn != nil {
		return m.createTransferFn(userID, fromAccountID, toAccountID, amount, description, date)
	}
	return &models.Transaction{}, nil
}

func (m *mockTransactionService) GetAccountTransactions(userID, accountID string, page pagination.PageRequest, filter services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
	if m.getAccountTransactionsFn != nil {
		return m.getAccountTransactionsFn(userID, accountID, page, filter)
	}
	resp := pagination.NewPageResponse([]models.Transaction{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockTransactionService) GetUserTransactions(userID string, page pagination.PageRequest, filter services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
	if m.getUserTransactionsFn != nil {
		return m.getUserTransactionsFn(userID, page, filter)
	}
	resp := pagination.NewPageResponse([]models.Transaction{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockTransactionService) GetTransactionByID(userID, transactionID string) (*models.Transaction, error) {
	if m.getTransactionByIDFn != nil {
		return m.getTransactionByIDFn(userID, transactionID)
	}
	return &models.Transaction{}, nil
}

func (m *mockTransactionService) UpdateTransaction(userID, transactionID string, updates services.TransactionUpdateFields) (*models.Transaction, error) {
	if m.updateTransactionFn != nil {
		return m.updateTransactionFn(userID, transactionID, updates)
	}
	return &models.Transaction{}, nil
}

func (m *mockTransactionService) DeleteTransaction(userID, transactionID string) error {
	if m.deleteTransactionFn != nil {
		return m.deleteTransactionFn(userID, transactionID)
	}
	return nil
}

func (m *mockTransactionService) GetSpendingByCategory(userID string, from, to time.Time) (*services.SpendingByCategory, error) {
	if m.getSpendingByCategoryFn != nil {
		return m.getSpendingByCategoryFn(userID, from, to)
	}
	return &services.SpendingByCategory{Items: []services.SpendingByCategoryItem{}}, nil
}

func (m *mockTransactionService) GetCashflow(userID string, from, to time.Time) (*services.Cashflow, error) {
	if m.getCashflowFn != nil {
		return m.getCashflowFn(userID, from, to)
	}
	return &services.Cashflow{
		Income:   []services.SpendingByCategoryItem{},
		Expenses: []services.SpendingByCategoryItem{},
	}, nil
}

func (m *mockTransactionService) GetMonthlySummary(userID string, months int) ([]services.MonthlySummaryItem, error) {
	if m.getMonthlySummaryFn != nil {
		return m.getMonthlySummaryFn(userID, months)
	}
	return []services.MonthlySummaryItem{}, nil
}

func (m *mockTransactionService) GetDailySpending(userID string, from, to time.Time) ([]services.DailySpendingItem, error) {
	if m.getDailySpendingFn != nil {
		return m.getDailySpendingFn(userID, from, to)
	}
	return []services.DailySpendingItem{}, nil
}

func (m *mockTransactionService) PreviewRuleMatches(userID string, conditions []services.RuleConditionInput) (*services.RuleMatchPreview, error) {
	if m.previewRuleMatchesFn != nil {
		return m.previewRuleMatchesFn(userID, conditions)
	}
	return &services.RuleMatchPreview{Sample: []models.Transaction{}}, nil
}

func (m *mockTransactionService) ApplyRule(userID, ruleID string, opts services.ApplyRuleOptions) (*services.ApplyRuleResult, error) {
	if m.applyRuleFn != nil {
		return m.applyRuleFn(userID, ruleID, opts)
	}
	return &services.ApplyRuleResult{Sample: []models.Transaction{}}, nil
}

func (m *mockTransactionService) GetDailySummary(userID string, from, to time.Time) ([]services.DailySummaryItem, error) {
	if m.getDailySummaryFn != nil {
		return m.getDailySummaryFn(userID, from, to)
	}
	return []services.DailySummaryItem{}, nil
}

func (m *mockTransactionService) GetTopExpenses(userID string, from, to time.Time, limit int, categoryID *string) (*services.TopExpenses, error) {
	if m.getTopExpensesFn != nil {
		return m.getTopExpensesFn(userID, from, to, limit, categoryID)
	}
	return &services.TopExpenses{Items: []services.TopExpenseItem{}}, nil
}

var _ services.TransactionServicer = (*mockTransactionService)(nil)

func setupTransactionRouter(handler *TransactionHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.GET("/transactions", handler.GetUserTransactions)
	auth.POST("/transactions", handler.CreateTransaction)
	auth.POST("/transactions/transfer", handler.CreateTransfer)
	auth.GET("/transactions/spending-by-category", handler.GetSpendingByCategory)
	auth.GET("/transactions/monthly-summary", handler.GetMonthlySummary)
	auth.GET("/transactions/daily-spending", handler.GetDailySpending)
	auth.GET("/transactions/daily-summary", handler.GetDailySummary)
	auth.GET("/transactions/top-expenses", handler.GetTopExpenses)
	auth.GET("/accounts/:id/transactions", handler.GetAccountTransactions)
	auth.GET("/transactions/:id", handler.GetTransactionByID)
	auth.PUT("/transactions/:id", handler.UpdateTransaction)
	auth.DELETE("/transactions/:id", handler.DeleteTransaction)
	return r
}

func TestTransactionHandler_CreateTransaction(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		txSvc := &mockTransactionService{
			createTransactionFn: func(userID, accountID string, _ *string, txType models.TransactionType, amount int64, desc string, _ time.Time) (*models.Transaction, error) {
				return &models.Transaction{
					Base:      models.Base{ID: "1"},
					UserID:    userID,
					AccountID: accountID,
					Type:      txType,
					Amount:    amount,
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions",
			`{"account_id":"00000000-0000-0000-0000-000000000001","type":"income","amount":5000,"description":"Salary"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		tx := result["transaction"].(map[string]interface{})
		if tx["amount"].(float64) != 5000 {
			t.Errorf("expected amount 5000, got %v", tx["amount"])
		}
	})

	t.Run("returns 400 on missing account_id", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions",
			`{"type":"income","amount":5000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on zero amount", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions",
			`{"account_id":"00000000-0000-0000-0000-000000000001","type":"expense","amount":0}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid type", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions",
			`{"account_id":"00000000-0000-0000-0000-000000000001","type":"invalid","amount":1000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 404 when account not found", func(t *testing.T) {
		txSvc := &mockTransactionService{
			createTransactionFn: func(_, _ string, _ *string, _ models.TransactionType, _ int64, _ string, _ time.Time) (*models.Transaction, error) {
				return nil, apperrors.ErrAccountNotFound
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions",
			`{"account_id":"00000000-0000-0000-0000-000000000999","type":"income","amount":1000}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns 401 without auth", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/transactions", handler.CreateTransaction)

		rec := doRequest(r, "POST", "/transactions",
			`{"account_id":"00000000-0000-0000-0000-000000000001","type":"income","amount":1000}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_CreateTransfer(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		txSvc := &mockTransactionService{
			createTransferFn: func(userID, from, to string, amount int64, _ string, _ time.Time) (*models.Transaction, error) {
				toAcct := to
				return &models.Transaction{
					Base:        models.Base{ID: "1"},
					UserID:      userID,
					AccountID:   from,
					ToAccountID: &toAcct,
					Type:        models.TransactionTypeTransfer,
					Amount:      amount,
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions/transfer",
			`{"from_account_id":"00000000-0000-0000-0000-000000000001","to_account_id":"00000000-0000-0000-0000-000000000002","amount":1000,"description":"Transfer"}`)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 400 on same account", func(t *testing.T) {
		txSvc := &mockTransactionService{
			createTransferFn: func(_, _, _ string, _ int64, _ string, _ time.Time) (*models.Transaction, error) {
				return nil, apperrors.ErrSameAccountTransfer
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions/transfer",
			`{"from_account_id":"00000000-0000-0000-0000-000000000001","to_account_id":"00000000-0000-0000-0000-000000000001","amount":1000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "SAME_ACCOUNT_TRANSFER")
	})

	t.Run("returns 400 on insufficient balance", func(t *testing.T) {
		txSvc := &mockTransactionService{
			createTransferFn: func(_, _, _ string, _ int64, _ string, _ time.Time) (*models.Transaction, error) {
				return nil, apperrors.ErrInsufficientBalance
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions/transfer",
			`{"from_account_id":"00000000-0000-0000-0000-000000000001","to_account_id":"00000000-0000-0000-0000-000000000002","amount":999999}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INSUFFICIENT_BALANCE")
	})

	t.Run("returns 400 on missing required fields", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "POST", "/transactions/transfer", `{"amount":1000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_GetAccountTransactions(t *testing.T) {
	t.Run("returns 200 with paginated transactions", func(t *testing.T) {
		now := time.Now()
		txSvc := &mockTransactionService{
			getAccountTransactionsFn: func(_, _ string, _ pagination.PageRequest, _ services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
				resp := pagination.NewPageResponse([]models.Transaction{
					{Base: models.Base{ID: "1"}, Amount: 5000, Type: "income", Date: now},
				}, 1, 20, 1)
				return &resp, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/accounts/00000000-0000-0000-0000-000000000001/transactions", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 transaction, got %d", len(data))
		}
	})

	t.Run("passes filter params to service", func(t *testing.T) {
		var capturedFilter services.TransactionFilter
		txSvc := &mockTransactionService{
			getAccountTransactionsFn: func(_, _ string, _ pagination.PageRequest, filter services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
				capturedFilter = filter
				resp := pagination.NewPageResponse([]models.Transaction{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		doRequest(r, "GET", "/accounts/00000000-0000-0000-0000-000000000001/transactions?type=income&min_amount=100&max_amount=5000", "")

		if capturedFilter.Type == nil || *capturedFilter.Type != models.TransactionTypeIncome {
			t.Errorf("expected type=income filter, got %v", capturedFilter.Type)
		}
		if capturedFilter.MinAmount == nil || *capturedFilter.MinAmount != 100 {
			t.Errorf("expected min_amount=100, got %v", capturedFilter.MinAmount)
		}
		if capturedFilter.MaxAmount == nil || *capturedFilter.MaxAmount != 5000 {
			t.Errorf("expected max_amount=5000, got %v", capturedFilter.MaxAmount)
		}
	})

	t.Run("returns 400 on invalid type filter", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/accounts/00000000-0000-0000-0000-000000000001/transactions?type=invalid", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid date format", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/accounts/00000000-0000-0000-0000-000000000001/transactions?from_date=not-a-date", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid min_amount", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/accounts/00000000-0000-0000-0000-000000000001/transactions?min_amount=abc", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid account ID", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/accounts/abc/transactions", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_GetUserTransactions(t *testing.T) {
	t.Run("returns_200_with_transactions", func(t *testing.T) {
		now := time.Now()
		txSvc := &mockTransactionService{
			getUserTransactionsFn: func(_ string, _ pagination.PageRequest, _ services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
				resp := pagination.NewPageResponse([]models.Transaction{
					{Base: models.Base{ID: "1"}, Amount: 5000, Type: "income", Date: now},
					{Base: models.Base{ID: "2"}, Amount: 3000, Type: "expense", Date: now},
				}, 1, 20, 2)
				return &resp, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 transactions, got %d", len(data))
		}
	})

	t.Run("returns_200_empty_when_no_transactions", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getUserTransactionsFn: func(_ string, _ pagination.PageRequest, _ services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
				resp := pagination.NewPageResponse([]models.Transaction{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("expected 0 transactions, got %d", len(data))
		}
	})

	t.Run("passes_filters_to_service", func(t *testing.T) {
		var capturedFilter services.TransactionFilter
		txSvc := &mockTransactionService{
			getUserTransactionsFn: func(_ string, _ pagination.PageRequest, filter services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
				capturedFilter = filter
				resp := pagination.NewPageResponse([]models.Transaction{}, 1, 20, 0)
				return &resp, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		doRequest(r, "GET", "/transactions?type=income&account_id=5&min_amount=100", "")

		if capturedFilter.Type == nil || *capturedFilter.Type != models.TransactionTypeIncome {
			t.Errorf("expected type=income filter, got %v", capturedFilter.Type)
		}
		if capturedFilter.AccountID == nil || *capturedFilter.AccountID != "5" {
			t.Errorf("expected account_id=5, got %v", capturedFilter.AccountID)
		}
		if capturedFilter.MinAmount == nil || *capturedFilter.MinAmount != 100 {
			t.Errorf("expected min_amount=100, got %v", capturedFilter.MinAmount)
		}
	})

	t.Run("returns_400_for_invalid_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions?from_date=not-a-date", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_400_for_invalid_type", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions?type=invalid", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
	})

	t.Run("returns_401_without_auth", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := gin.New()
		r.GET("/transactions", handler.GetUserTransactions)

		rec := doRequest(r, "GET", "/transactions", "")

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_GetTransactionByID(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getTransactionByIDFn: func(_, txID string) (*models.Transaction, error) {
				return &models.Transaction{
					Base:   models.Base{ID: txID},
					Amount: 5000,
					Type:   "income",
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/00000000-0000-0000-0000-000000000001", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getTransactionByIDFn: func(_, _ string) (*models.Transaction, error) {
				return nil, apperrors.ErrTransactionNotFound
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/00000000-0000-0000-0000-000000000999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_DeleteTransaction(t *testing.T) {
	t.Run("returns 200 on success", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "DELETE", "/transactions/00000000-0000-0000-0000-000000000001", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		result := parseJSON(t, rec)
		if result["message"] != "Transaction deleted successfully" {
			t.Errorf("unexpected message: %v", result["message"])
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		txSvc := &mockTransactionService{
			deleteTransactionFn: func(_, _ string) error {
				return apperrors.ErrTransactionNotFound
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "DELETE", "/transactions/00000000-0000-0000-0000-000000000999", "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on invalid ID", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "DELETE", "/transactions/abc", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_UpdateTransaction(t *testing.T) {
	t.Run("returns_200_with_updated_transaction", func(t *testing.T) {
		txSvc := &mockTransactionService{
			updateTransactionFn: func(_, txID string, _ services.TransactionUpdateFields) (*models.Transaction, error) {
				return &models.Transaction{
					Base:      models.Base{ID: txID},
					UserID:    "1",
					AccountID: "1",
					Type:      models.TransactionTypeExpense,
					Amount:    3000,
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "PUT", "/transactions/00000000-0000-0000-0000-000000000001", `{"amount":3000}`)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		tx := result["transaction"].(map[string]interface{})
		if tx["amount"].(float64) != 3000 {
			t.Errorf("expected amount 3000, got %v", tx["amount"])
		}
	})

	t.Run("returns_400_for_invalid_amount", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "PUT", "/transactions/00000000-0000-0000-0000-000000000001", `{"amount":-1}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_for_invalid_type", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "PUT", "/transactions/00000000-0000-0000-0000-000000000001", `{"type":"invalid"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_404_for_nonexistent_transaction", func(t *testing.T) {
		txSvc := &mockTransactionService{
			updateTransactionFn: func(_, _ string, _ services.TransactionUpdateFields) (*models.Transaction, error) {
				return nil, apperrors.ErrTransactionNotFound
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "PUT", "/transactions/00000000-0000-0000-0000-000000000999", `{"amount":1000}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns_400_for_non_editable_type", func(t *testing.T) {
		txSvc := &mockTransactionService{
			updateTransactionFn: func(_, _ string, _ services.TransactionUpdateFields) (*models.Transaction, error) {
				return nil, apperrors.ErrTransactionNotEditable
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "PUT", "/transactions/00000000-0000-0000-0000-000000000001", `{"amount":1000}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "TRANSACTION_NOT_EDITABLE")
	})

	t.Run("passes_update_fields_to_service", func(t *testing.T) {
		var captured services.TransactionUpdateFields
		txSvc := &mockTransactionService{
			updateTransactionFn: func(_, _ string, updates services.TransactionUpdateFields) (*models.Transaction, error) {
				captured = updates
				return &models.Transaction{Base: models.Base{ID: "1"}}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		doRequest(r, "PUT", "/transactions/00000000-0000-0000-0000-000000000001", `{"amount":5000,"type":"income","description":"Updated"}`)

		if captured.Amount == nil || *captured.Amount != 5000 {
			t.Errorf("expected amount=5000, got %v", captured.Amount)
		}
		if captured.Type == nil || *captured.Type != models.TransactionTypeIncome {
			t.Errorf("expected type=income, got %v", captured.Type)
		}
		if captured.Description == nil || *captured.Description != "Updated" {
			t.Errorf("expected description=Updated, got %v", captured.Description)
		}
	})
}

func TestTransactionHandler_GetSpendingByCategory(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		catID := "3"
		txSvc := &mockTransactionService{
			getSpendingByCategoryFn: func(_ string, _, _ time.Time) (*services.SpendingByCategory, error) {
				return &services.SpendingByCategory{
					Items: []services.SpendingByCategoryItem{
						{CategoryID: &catID, CategoryName: "Groceries", CategoryColor: "#22C55E", Total: 5000},
						{CategoryID: nil, CategoryName: "Uncategorized", CategoryColor: "#9CA3AF", Total: 1500},
					},
					TotalSpent: 6500,
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/spending-by-category?from_date=2026-01-01&to_date=2026-01-31", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		items := result["items"].([]interface{})
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
		if result["total_spent"].(float64) != 6500 {
			t.Errorf("expected total_spent 6500, got %v", result["total_spent"])
		}
	})

	t.Run("returns_400_missing_from_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/spending-by-category?to_date=2026-01-31", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_missing_to_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/spending-by-category?from_date=2026-01-01", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_200_empty_items", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getSpendingByCategoryFn: func(_ string, _, _ time.Time) (*services.SpendingByCategory, error) {
				return &services.SpendingByCategory{
					Items:      []services.SpendingByCategoryItem{},
					TotalSpent: 0,
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/spending-by-category?from_date=2026-01-01&to_date=2026-01-31", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		items := result["items"].([]interface{})
		if len(items) != 0 {
			t.Errorf("expected 0 items, got %d", len(items))
		}
	})
}

func TestTransactionHandler_GetMonthlySummary(t *testing.T) {
	t.Run("returns_200_with_default_months", func(t *testing.T) {
		var capturedMonths int
		txSvc := &mockTransactionService{
			getMonthlySummaryFn: func(_ string, months int) ([]services.MonthlySummaryItem, error) {
				capturedMonths = months
				return []services.MonthlySummaryItem{
					{Month: "2025-09", Income: 500000, Expenses: 320000},
					{Month: "2025-10", Income: 480000, Expenses: 350000},
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/monthly-summary", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedMonths != 6 {
			t.Errorf("expected default months=6, got %d", capturedMonths)
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 items, got %d", len(data))
		}
	})

	t.Run("returns_200_with_custom_months", func(t *testing.T) {
		var capturedMonths int
		txSvc := &mockTransactionService{
			getMonthlySummaryFn: func(_ string, months int) ([]services.MonthlySummaryItem, error) {
				capturedMonths = months
				return []services.MonthlySummaryItem{}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/monthly-summary?months=3", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedMonths != 3 {
			t.Errorf("expected months=3, got %d", capturedMonths)
		}
	})

	t.Run("returns_200_empty_data", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getMonthlySummaryFn: func(_ string, _ int) ([]services.MonthlySummaryItem, error) {
				return []services.MonthlySummaryItem{}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/monthly-summary", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("expected 0 items, got %d", len(data))
		}
	})
}

func TestTransactionHandler_GetDailySpending(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getDailySpendingFn: func(_ string, _, _ time.Time) ([]services.DailySpendingItem, error) {
				return []services.DailySpendingItem{
					{Date: "2026-02-01", Total: 5000},
					{Date: "2026-02-02", Total: 0},
					{Date: "2026-02-03", Total: 1500},
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-spending?from_date=2026-02-01&to_date=2026-02-03", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 3 {
			t.Errorf("expected 3 items, got %d", len(data))
		}
	})

	t.Run("returns_400_missing_from_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-spending?to_date=2026-02-03", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_missing_to_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-spending?from_date=2026-02-01", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_for_excessive_range", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-spending?from_date=2024-01-01&to_date=2026-02-03", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestTransactionHandler_GetDailySummary(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getDailySummaryFn: func(_ string, _, _ time.Time) ([]services.DailySummaryItem, error) {
				return []services.DailySummaryItem{
					{Date: "2026-02-01", Income: 10000, Expenses: 5000},
					{Date: "2026-02-02", Income: 0, Expenses: 0},
					{Date: "2026-02-03", Income: 0, Expenses: 1500},
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-summary?from_date=2026-02-01&to_date=2026-02-03", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		data := result["data"].([]interface{})
		if len(data) != 3 {
			t.Errorf("expected 3 items, got %d", len(data))
		}
	})

	t.Run("returns_400_missing_from_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-summary?to_date=2026-02-03", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_missing_to_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-summary?from_date=2026-02-01", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_for_excessive_range", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/daily-summary?from_date=2024-01-01&to_date=2026-02-03", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestTransactionHandler_GetTopExpenses(t *testing.T) {
	t.Run("returns_200_with_data", func(t *testing.T) {
		txSvc := &mockTransactionService{
			getTopExpensesFn: func(_ string, _, _ time.Time, _ int, _ *string) (*services.TopExpenses, error) {
				return &services.TopExpenses{
					Items: []services.TopExpenseItem{
						{ID: "tx-1", Description: "big", Amount: 5000, AccountName: "Checking", CategoryName: "Uncategorized"},
						{ID: "tx-2", Description: "medium", Amount: 3000, AccountName: "Checking", CategoryName: "Uncategorized"},
					},
				}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=2026-02-01&to_date=2026-02-28", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		items := result["items"].([]interface{})
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
	})

	t.Run("returns_400_missing_from_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?to_date=2026-02-28", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_missing_to_date", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=2026-02-01", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_invalid_date_format", func(t *testing.T) {
		handler := NewTransactionHandler(&mockTransactionService{}, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=not-a-date&to_date=2026-02-28", "")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("limit_clamped_to_max", func(t *testing.T) {
		var capturedLimit int
		txSvc := &mockTransactionService{
			getTopExpensesFn: func(_ string, _, _ time.Time, limit int, _ *string) (*services.TopExpenses, error) {
				capturedLimit = limit
				return &services.TopExpenses{Items: []services.TopExpenseItem{}}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=2026-02-01&to_date=2026-02-28&limit=500", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedLimit != 100 {
			t.Errorf("expected limit clamped to 100, got %d", capturedLimit)
		}
	})

	t.Run("default_limit_when_omitted", func(t *testing.T) {
		var capturedLimit int
		txSvc := &mockTransactionService{
			getTopExpensesFn: func(_ string, _, _ time.Time, limit int, _ *string) (*services.TopExpenses, error) {
				capturedLimit = limit
				return &services.TopExpenses{Items: []services.TopExpenseItem{}}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=2026-02-01&to_date=2026-02-28", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedLimit != 10 {
			t.Errorf("expected default limit 10, got %d", capturedLimit)
		}
	})

	t.Run("passes_through_category_id_when_present", func(t *testing.T) {
		var capturedCategoryID *string
		txSvc := &mockTransactionService{
			getTopExpensesFn: func(_ string, _, _ time.Time, _ int, categoryID *string) (*services.TopExpenses, error) {
				capturedCategoryID = categoryID
				return &services.TopExpenses{Items: []services.TopExpenseItem{}}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=2026-02-01&to_date=2026-02-28&category_id=cat-1", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedCategoryID == nil || *capturedCategoryID != "cat-1" {
			t.Errorf("expected category_id 'cat-1', got %v", capturedCategoryID)
		}
	})

	t.Run("category_id_nil_when_omitted", func(t *testing.T) {
		var capturedCategoryID *string
		called := false
		txSvc := &mockTransactionService{
			getTopExpensesFn: func(_ string, _, _ time.Time, _ int, categoryID *string) (*services.TopExpenses, error) {
				called = true
				capturedCategoryID = categoryID
				return &services.TopExpenses{Items: []services.TopExpenseItem{}}, nil
			},
		}
		handler := NewTransactionHandler(txSvc, &mockAuditService{})
		r := setupTransactionRouter(handler)

		rec := doRequest(r, "GET", "/transactions/top-expenses?from_date=2026-02-01&to_date=2026-02-28", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !called || capturedCategoryID != nil {
			t.Errorf("expected nil category_id when omitted, got %v", capturedCategoryID)
		}
	})
}
