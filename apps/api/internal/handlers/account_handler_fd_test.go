package handlers

// Tests for the fixed-deposit handler endpoints (plan 020, B10).
//
// The setupAccountRouter function in account_handler_test.go does not include
// the FD routes; this file uses a dedicated fdRouter helper that adds them.

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// fdRouter creates a gin.Engine with all account routes including FD endpoints.
func fdRouter(handler *AccountHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.POST("/accounts/cash", handler.CreateCashAccount)
	auth.POST("/accounts/investment", handler.CreateInvestmentAccount)
	auth.POST("/accounts/credit-card", handler.CreateCreditCardAccount)
	auth.POST("/accounts/fixed-deposit", handler.CreateFixedDepositAccount)
	auth.POST("/accounts/:id/fixed-deposit/close", handler.CloseFixedDeposit)
	auth.GET("/accounts", handler.GetUserAccounts)
	auth.GET("/accounts/:id", handler.GetAccountByID)
	auth.PUT("/accounts/:id", handler.UpdateAccount)
	return r
}

func TestAccountHandler_CreateFixedDepositAccount(t *testing.T) {
	t.Run("returns_201_on_success", func(t *testing.T) {
		acctSvc := &mockAccountService{
			createFixedDepositAccountFn: func(userID string, params services.CreateFixedDepositParams) (*models.Account, error) {
				freq := models.PayoutFrequencyMonthly
				return &models.Account{
					Base:         models.Base{ID: "fd-test-id-1"},
					UserID:       userID,
					Name:         params.Name,
					Type:         models.AccountTypeFixedDeposit,
					Balance:      params.Principal,
					Currency:     "USD",
					IsActive:     true,
					InterestRate: params.InterestRate,
					FixedDeposit: &models.FixedDepositDetails{
						Principal:       params.Principal,
						StartDate:       params.StartDate,
						MaturityDate:    params.MaturityDate,
						PayoutType:      params.PayoutType,
						PayoutFrequency: &freq,
						Compounding:     params.Compounding,
						MaturityValue:   params.Principal + 5000,
						Status:          models.FixedDepositStatusActive,
					},
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "My FD",
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 100000,
			"interest_rate": 5.0,
			"start_date": "2025-01-01",
			"maturity_date": "2026-01-01",
			"payout_type": "periodic",
			"payout_frequency": "monthly",
			"compounding": "monthly"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		acct := result["account"].(map[string]interface{})
		if acct["name"] != "My FD" {
			t.Errorf("expected name 'My FD', got %v", acct["name"])
		}
		if acct["type"] != string(models.AccountTypeFixedDeposit) {
			t.Errorf("expected type fixed_deposit, got %v", acct["type"])
		}
	})

	t.Run("returns_400_on_missing_name", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 100000,
			"start_date": "2025-01-01",
			"maturity_date": "2026-01-01",
			"payout_type": "cumulative",
			"compounding": "annual"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_on_missing_source_account", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "FD",
			"principal": 100000,
			"start_date": "2025-01-01",
			"maturity_date": "2026-01-01",
			"payout_type": "cumulative",
			"compounding": "annual"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_on_zero_principal", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "FD",
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 0,
			"start_date": "2025-01-01",
			"maturity_date": "2026-01-01",
			"payout_type": "cumulative",
			"compounding": "annual"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_on_invalid_payout_type", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "FD",
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 100000,
			"start_date": "2025-01-01",
			"maturity_date": "2026-01-01",
			"payout_type": "invalid_type",
			"compounding": "annual"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid payout_type, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_when_maturity_before_start", func(t *testing.T) {
		acctSvc := &mockAccountService{}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "FD",
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 100000,
			"start_date": "2026-01-01",
			"maturity_date": "2025-01-01",
			"payout_type": "cumulative",
			"compounding": "annual"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 when maturity before start, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns_400_on_service_insufficient_balance", func(t *testing.T) {
		acctSvc := &mockAccountService{
			createFixedDepositAccountFn: func(_ string, _ services.CreateFixedDepositParams) (*models.Account, error) {
				return nil, apperrors.ErrSourceAccountInsufficient
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "FD",
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 100000,
			"start_date": "2025-01-01",
			"maturity_date": "2026-01-01",
			"payout_type": "cumulative",
			"compounding": "annual"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for insufficient balance, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "SOURCE_ACCOUNT_INSUFFICIENT")
	})

	t.Run("returns_401_without_auth", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/accounts/fixed-deposit", handler.CreateFixedDepositAccount)

		body := `{"name":"FD","source_account_id":"x","principal":100000,"start_date":"2025-01-01","maturity_date":"2026-01-01","payout_type":"cumulative","compounding":"annual"}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("params_forwarded_to_service", func(t *testing.T) {
		var captured services.CreateFixedDepositParams
		acctSvc := &mockAccountService{
			createFixedDepositAccountFn: func(userID string, params services.CreateFixedDepositParams) (*models.Account, error) {
				captured = params
				return &models.Account{
					Base: models.Base{ID: "fd-1"},
					Type: models.AccountTypeFixedDeposit,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"name": "Term Deposit",
			"description": "My savings",
			"currency": "USD",
			"source_account_id": "00000000-0000-0000-0000-000000000001",
			"principal": 500000,
			"interest_rate": 4.5,
			"start_date": "2025-03-01",
			"maturity_date": "2026-03-01",
			"payout_type": "cumulative",
			"compounding": "quarterly"
		}`
		rec := doRequest(r, "POST", "/accounts/fixed-deposit", body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if captured.Name != "Term Deposit" {
			t.Errorf("expected name 'Term Deposit', got %q", captured.Name)
		}
		if captured.Principal != 500000 {
			t.Errorf("expected principal 500000, got %d", captured.Principal)
		}
		if captured.InterestRate != 4.5 {
			t.Errorf("expected interest_rate 4.5, got %f", captured.InterestRate)
		}
		if captured.Compounding != models.CompoundingQuarterly {
			t.Errorf("expected compounding quarterly, got %s", captured.Compounding)
		}
		if captured.PayoutType != models.PayoutTypeCumulative {
			t.Errorf("expected payout_type cumulative, got %s", captured.PayoutType)
		}
	})
}

func TestAccountHandler_CloseFixedDeposit(t *testing.T) {
	t.Run("returns_200_on_success", func(t *testing.T) {
		acctSvc := &mockAccountService{
			closeFixedDepositFn: func(userID, accountID string, params services.CloseFixedDepositParams) (*models.Account, error) {
				return &models.Account{
					Base:     models.Base{ID: accountID},
					Type:     models.AccountTypeFixedDeposit,
					IsActive: false,
					Balance:  0,
					FixedDeposit: &models.FixedDepositDetails{
						Status: models.FixedDepositStatusClosed,
					},
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		body := `{"destination_account_id": "00000000-0000-0000-0000-000000000002"}`
		rec := doRequest(r, "POST", "/accounts/00000000-0000-0000-0000-000000000001/fixed-deposit/close", body)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		result := parseJSON(t, rec)
		acct := result["account"].(map[string]interface{})
		if acct["is_active"] != false {
			t.Errorf("expected is_active=false on closed FD, got %v", acct["is_active"])
		}
	})

	t.Run("returns_404_when_not_found", func(t *testing.T) {
		acctSvc := &mockAccountService{
			closeFixedDepositFn: func(_, _ string, _ services.CloseFixedDepositParams) (*models.Account, error) {
				return nil, apperrors.ErrAccountNotFound
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		rec := doRequest(r, "POST", "/accounts/00000000-0000-0000-0000-000000000999/fixed-deposit/close",
			`{"destination_account_id": "00000000-0000-0000-0000-000000000002"}`)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "ACCOUNT_NOT_FOUND")
	})

	t.Run("returns_409_on_already_closed", func(t *testing.T) {
		acctSvc := &mockAccountService{
			closeFixedDepositFn: func(_, _ string, _ services.CloseFixedDepositParams) (*models.Account, error) {
				return nil, apperrors.ErrFixedDepositAlreadyClosed
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		rec := doRequest(r, "POST", "/accounts/00000000-0000-0000-0000-000000000001/fixed-deposit/close",
			`{"destination_account_id": "00000000-0000-0000-0000-000000000002"}`)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "FIXED_DEPOSIT_ALREADY_CLOSED")
	})

	t.Run("returns_400_on_invalid_account_id", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := fdRouter(handler)

		rec := doRequest(r, "POST", "/accounts/not-a-uuid/fixed-deposit/close",
			`{"destination_account_id": "00000000-0000-0000-0000-000000000002"}`)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("params_forwarded_to_service", func(t *testing.T) {
		var capturedParams services.CloseFixedDepositParams
		destID := "00000000-0000-0000-0000-000000000002"
		interestOverride := int64(3000)
		acctSvc := &mockAccountService{
			closeFixedDepositFn: func(_, _ string, params services.CloseFixedDepositParams) (*models.Account, error) {
				capturedParams = params
				return &models.Account{
					Base: models.Base{ID: "fd-1"},
					Type: models.AccountTypeFixedDeposit,
				}, nil
			},
		}
		handler := NewAccountHandler(acctSvc, &mockAuditService{})
		r := fdRouter(handler)

		body := `{
			"destination_account_id": "00000000-0000-0000-0000-000000000002",
			"interest_override": 3000,
			"penalty": 500
		}`
		rec := doRequest(r, "POST", "/accounts/00000000-0000-0000-0000-000000000001/fixed-deposit/close", body)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedParams.DestinationAccountID == nil || *capturedParams.DestinationAccountID != destID {
			t.Errorf("expected destination_account_id %s, got %v", destID, capturedParams.DestinationAccountID)
		}
		if capturedParams.InterestOverride == nil || *capturedParams.InterestOverride != interestOverride {
			t.Errorf("expected interest_override %d, got %v", interestOverride, capturedParams.InterestOverride)
		}
		if capturedParams.Penalty != 500 {
			t.Errorf("expected penalty 500, got %d", capturedParams.Penalty)
		}
	})

	t.Run("returns_401_without_auth", func(t *testing.T) {
		handler := NewAccountHandler(&mockAccountService{}, &mockAuditService{})
		r := gin.New()
		r.POST("/accounts/:id/fixed-deposit/close", handler.CloseFixedDeposit)

		rec := doRequest(r, "POST", "/accounts/00000000-0000-0000-0000-000000000001/fixed-deposit/close", `{}`)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}
