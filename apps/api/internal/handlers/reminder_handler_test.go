package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/services"
)

type mockReminderService struct {
	upcomingFn   func(userID, refID string) ([]models.Reminder, error)
	setEnabledFn func(userID, id string, enabled bool) (*models.Reminder, error)
	dueFn        func() ([]services.DueReminder, error)
	markSentFn   func(id string) (*models.Reminder, error)
}

func (m *mockReminderService) ListUpcoming(userID, refID string, _ time.Time) ([]models.Reminder, error) {
	if m.upcomingFn != nil {
		return m.upcomingFn(userID, refID)
	}
	return []models.Reminder{}, nil
}

func (m *mockReminderService) SetEnabled(userID, id string, enabled bool) (*models.Reminder, error) {
	if m.setEnabledFn != nil {
		return m.setEnabledFn(userID, id, enabled)
	}
	return &models.Reminder{}, nil
}

func (m *mockReminderService) SyncAccount(string, time.Time) error { return nil }

func (m *mockReminderService) Due(time.Time) ([]services.DueReminder, error) {
	if m.dueFn != nil {
		return m.dueFn()
	}
	return []services.DueReminder{}, nil
}

func (m *mockReminderService) MarkSent(id string, _ time.Time) (*models.Reminder, error) {
	if m.markSentFn != nil {
		return m.markSentFn(id)
	}
	return &models.Reminder{}, nil
}

var _ services.ReminderServicer = (*mockReminderService)(nil)

func setupReminderRouter(h *ReminderHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("u1"))
	auth.GET("/reminders", h.List)
	auth.PATCH("/reminders/:id", h.SetEnabled)
	r.GET("/internal/telegram/reminders/due", h.Due)
	r.POST("/internal/telegram/reminders/:id/sent", h.MarkSent)
	return r
}

func TestReminderHandler_List(t *testing.T) {
	var gotUser, gotRef string
	h := NewReminderHandler(&mockReminderService{upcomingFn: func(u, ref string) ([]models.Reminder, error) {
		gotUser, gotRef = u, ref
		return []models.Reminder{{Base: models.Base{ID: "r1"}, Kind: models.ReminderKindCardDue, Enabled: true}}, nil
	}}, &mockAuditService{})
	rec := doRequest(setupReminderRouter(h), "GET", "/reminders?ref_id=acct-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotUser != "u1" || gotRef != "acct-1" {
		t.Fatalf("service got user=%q ref=%q", gotUser, gotRef)
	}
	list := parseJSON(t, rec)["reminders"].([]interface{})
	if len(list) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(list))
	}
}

func TestReminderHandler_SetEnabled(t *testing.T) {
	t.Run("switches the reminder off", func(t *testing.T) {
		got := true
		h := NewReminderHandler(&mockReminderService{setEnabledFn: func(_, id string, enabled bool) (*models.Reminder, error) {
			got = enabled
			return &models.Reminder{Base: models.Base{ID: id}, Enabled: enabled}, nil
		}}, &mockAuditService{})
		rec := doRequest(setupReminderRouter(h), "PATCH", "/reminders/r1", `{"enabled":false}`)
		if rec.Code != http.StatusOK || got {
			t.Fatalf("expected 200 and enabled=false, got %d enabled=%v: %s", rec.Code, got, rec.Body.String())
		}
	})
	t.Run("enabled is required", func(t *testing.T) {
		h := NewReminderHandler(&mockReminderService{}, &mockAuditService{})
		rec := doRequest(setupReminderRouter(h), "PATCH", "/reminders/r1", `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
	t.Run("unknown reminder is 404", func(t *testing.T) {
		h := NewReminderHandler(&mockReminderService{setEnabledFn: func(string, string, bool) (*models.Reminder, error) {
			return nil, apperrors.ErrNotFound
		}}, &mockAuditService{})
		rec := doRequest(setupReminderRouter(h), "PATCH", "/reminders/nope", `{"enabled":true}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestReminderHandler_InternalEndpoints(t *testing.T) {
	h := NewReminderHandler(&mockReminderService{
		dueFn: func() ([]services.DueReminder, error) {
			return []services.DueReminder{{ID: "r1", AccountName: "Amex Gold", Amount: 123450, Currency: "MYR", DueOn: "2026-12-01"}}, nil
		},
		markSentFn: func(id string) (*models.Reminder, error) {
			if id == "gone" {
				return nil, apperrors.ErrNotFound
			}
			now := time.Now()
			return &models.Reminder{Base: models.Base{ID: id}, SentAt: &now}, nil
		},
	}, &mockAuditService{})
	r := setupReminderRouter(h)

	rec := doRequest(r, "GET", "/internal/telegram/reminders/due", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("due: expected 200, got %d", rec.Code)
	}
	due := parseJSON(t, rec)["reminders"].([]interface{})
	if len(due) != 1 || due[0].(map[string]interface{})["currency"] != "MYR" {
		t.Fatalf("unexpected due payload: %v", due)
	}

	if rec := doRequest(r, "POST", "/internal/telegram/reminders/r1/sent", ""); rec.Code != http.StatusOK {
		t.Fatalf("sent: expected 200, got %d", rec.Code)
	}
	if rec := doRequest(r, "POST", "/internal/telegram/reminders/gone/sent", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("sent unknown: expected 404, got %d", rec.Code)
	}
}

func TestAccountHandler_CreateDebtAccount(t *testing.T) {
	router := func(svc *mockAccountService) *gin.Engine {
		r := gin.New()
		auth := r.Group("", injectUserID("u1"))
		auth.POST("/accounts/debt", NewAccountHandler(svc, &mockAuditService{}).CreateDebtAccount)
		return r
	}

	t.Run("passes the loan fields to the service", func(t *testing.T) {
		var got services.CreateDebtParams
		svc := &mockAccountService{createDebtAccountFn: func(_ string, p services.CreateDebtParams) (*models.Account, error) {
			got = p
			return &models.Account{Base: models.Base{ID: "d1"}, Name: p.Name, Type: models.AccountTypeDebt, Balance: p.AmountOwed}, nil
		}}
		rec := doRequest(router(svc), "POST", "/accounts/debt",
			`{"name":"Car loan","currency":"MYR","amount_owed":5000000,"interest_rate":3.5,"monthly_payment":120000,"due_day":15}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if got.AmountOwed != 5000000 || got.MonthlyPayment != 120000 || got.DueDay != 15 || got.Currency != "MYR" || got.InterestRate != 3.5 {
			t.Fatalf("unexpected params: %+v", got)
		}
	})

	for name, body := range map[string]string{
		"missing name":      `{"amount_owed":1}`,
		"negative owed":     `{"name":"x","amount_owed":-1}`,
		"due day too big":   `{"name":"x","due_day":32}`,
		"negative payment":  `{"name":"x","monthly_payment":-5}`,
		"rate over 100":     `{"name":"x","interest_rate":101}`,
		"bad currency code": `{"name":"x","currency":"ZZZZ"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := doRequest(router(&mockAccountService{}), "POST", "/accounts/debt", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAccountHandler_CreditCardDueDay(t *testing.T) {
	var got services.CreateCreditCardParams
	svc := &mockAccountService{createCreditCardParamsFn: func(_ string, p services.CreateCreditCardParams) (*models.Account, error) {
		got = p
		return &models.Account{Base: models.Base{ID: "c1"}}, nil
	}}
	r := gin.New()
	auth := r.Group("", injectUserID("u1"))
	auth.POST("/accounts/credit-card", NewAccountHandler(svc, &mockAuditService{}).CreateCreditCardAccount)

	if rec := doRequest(r, "POST", "/accounts/credit-card", `{"name":"Amex","due_day":1}`); rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if got.DueDay != 1 {
		t.Fatalf("due_day not passed: %+v", got)
	}
	if rec := doRequest(r, "POST", "/accounts/credit-card", `{"name":"Amex","due_day":40}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for due_day 40, got %d", rec.Code)
	}
}
