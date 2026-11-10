package handlers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

type mockHistoryService struct {
	historyFn   func(userID, accountID string, from, to time.Time) (*services.AccountBalanceHistory, error)
	sparklineFn func(userID string, days int) ([]services.AccountSparkline, error)
}

var _ services.AccountBalanceHistoryServicer = (*mockHistoryService)(nil)

func (m *mockHistoryService) GetBalanceHistory(userID, accountID string, from, to time.Time) (*services.AccountBalanceHistory, error) {
	return m.historyFn(userID, accountID, from, to)
}

func (m *mockHistoryService) GetSparklines(userID string, days int) ([]services.AccountSparkline, error) {
	return m.sparklineFn(userID, days)
}

type mockTypicalService struct {
	fn func(userID string, months int) (*services.Typical, error)
}

var _ services.TypicalServicer = (*mockTypicalService)(nil)

func (m *mockTypicalService) GetTypical(userID string, months int) (*services.Typical, error) {
	return m.fn(userID, months)
}

func setupHistoryRouter(h *AccountHistoryHandler, a *AnalyticsHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.GET("/accounts/sparklines", h.GetSparklines)
	auth.GET("/accounts/:id/balance-history", h.GetBalanceHistory)
	auth.GET("/analytics/typical", a.GetTypical)
	return r
}

const historyAccountID = "0192f3c0-0000-7000-8000-000000000001"

func TestAccountHistoryHandler(t *testing.T) {
	t.Run("balance history passes parsed dates and wraps data", func(t *testing.T) {
		var gotFrom, gotTo time.Time
		svc := &mockHistoryService{historyFn: func(userID, accountID string, from, to time.Time) (*services.AccountBalanceHistory, error) {
			gotFrom, gotTo = from, to
			if userID != "test-user-1" || accountID != historyAccountID {
				t.Errorf("ids = %s %s", userID, accountID)
			}
			return &services.AccountBalanceHistory{AccountID: accountID, Points: []services.BalancePoint{{Day: "2026-11-01", Balance: 5}}}, nil
		}}
		r := setupHistoryRouter(NewAccountHistoryHandler(svc), NewAnalyticsHandler(nil))
		rec := doRequest(r, "GET", "/accounts/"+historyAccountID+"/balance-history?from=2026-11-01&to=2026-11-05", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"data"`) || !strings.Contains(rec.Body.String(), `"balance":5`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
		if gotFrom.Format("2006-01-02") != "2026-11-01" || gotTo.Format("2006-01-02") != "2026-11-05" {
			t.Errorf("dates = %v %v", gotFrom, gotTo)
		}
	})

	t.Run("balance history rejects bad id and bad date", func(t *testing.T) {
		r := setupHistoryRouter(NewAccountHistoryHandler(&mockHistoryService{}), NewAnalyticsHandler(nil))
		if rec := doRequest(r, "GET", "/accounts/not-a-uuid/balance-history", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("bad id status = %d", rec.Code)
		}
		if rec := doRequest(r, "GET", "/accounts/"+historyAccountID+"/balance-history?from=yesterday", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("bad date status = %d", rec.Code)
		}
		if rec := doRequest(r, "GET", "/accounts/"+historyAccountID+"/balance-history?to=yesterday", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("bad to status = %d", rec.Code)
		}
	})

	t.Run("balance history maps service errors", func(t *testing.T) {
		svc := &mockHistoryService{historyFn: func(string, string, time.Time, time.Time) (*services.AccountBalanceHistory, error) {
			return nil, apperrors.ErrAccountNotFound
		}}
		r := setupHistoryRouter(NewAccountHistoryHandler(svc), NewAnalyticsHandler(nil))
		if rec := doRequest(r, "GET", "/accounts/"+historyAccountID+"/balance-history", ""); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d", rec.Code)
		}
	})

	t.Run("sparklines default and explicit days", func(t *testing.T) {
		var gotDays []int
		svc := &mockHistoryService{sparklineFn: func(_ string, days int) ([]services.AccountSparkline, error) {
			gotDays = append(gotDays, days)
			return []services.AccountSparkline{}, nil
		}}
		r := setupHistoryRouter(NewAccountHistoryHandler(svc), NewAnalyticsHandler(nil))
		if rec := doRequest(r, "GET", "/accounts/sparklines", ""); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if rec := doRequest(r, "GET", "/accounts/sparklines?days=7", ""); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if len(gotDays) != 2 || gotDays[0] != 0 || gotDays[1] != 7 {
			t.Errorf("days = %v", gotDays)
		}
		if rec := doRequest(r, "GET", "/accounts/sparklines?days=abc", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("bad days status = %d", rec.Code)
		}
	})
}

func TestAnalyticsHandler_Typical(t *testing.T) {
	var gotMonths int
	svc := &mockTypicalService{fn: func(_ string, months int) (*services.Typical, error) {
		gotMonths = months
		if months > 24 {
			return nil, apperrors.ErrInvalidInput
		}
		return &services.Typical{Months: months, Currency: "MYR"}, nil
	}}
	r := setupHistoryRouter(NewAccountHistoryHandler(&mockHistoryService{}), NewAnalyticsHandler(svc))

	rec := doRequest(r, "GET", "/analytics/typical", "")
	if rec.Code != http.StatusOK || gotMonths != 6 || !strings.Contains(rec.Body.String(), `"months":6`) {
		t.Errorf("default: status %d months %d body %s", rec.Code, gotMonths, rec.Body.String())
	}
	if rec := doRequest(r, "GET", "/analytics/typical?months=3", ""); rec.Code != http.StatusOK || gotMonths != 3 {
		t.Errorf("explicit: status %d months %d", rec.Code, gotMonths)
	}
	if rec := doRequest(r, "GET", "/analytics/typical?months=x", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric status = %d", rec.Code)
	}
	if rec := doRequest(r, "GET", "/analytics/typical?months=99", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("out of range status = %d", rec.Code)
	}
}
