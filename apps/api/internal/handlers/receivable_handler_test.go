package handlers

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

const (
	rcvUUID  = "00000000-0000-0000-0000-0000000000a1"
	rcvUUID2 = "00000000-0000-0000-0000-0000000000a2"
	rcvAcct  = "00000000-0000-0000-0000-000000000001"
)

// --- recording audit service ---

type auditEntry struct {
	userID, action, resourceType, resourceID string
	changes                                  map[string]interface{}
}

type recordingAudit struct {
	mu      sync.Mutex
	entries []auditEntry
}

func (a *recordingAudit) Log(userID, action, resourceType, resourceID, _ string, changes map[string]interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, auditEntry{userID, action, resourceType, resourceID, changes})
}

func (a *recordingAudit) only(t *testing.T) auditEntry {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.entries) != 1 {
		t.Fatalf("expected exactly one audit entry, got %d: %+v", len(a.entries), a.entries)
	}
	return a.entries[0]
}

func (a *recordingAudit) assertNone(t *testing.T) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.entries) != 0 {
		t.Errorf("expected no audit entries, got %+v", a.entries)
	}
}

// --- mock receivable service ---

type mockReceivableService struct {
	createSharedFn      func(userID string, in services.CreateSharedExpenseInput) (*models.Transaction, error)
	updateReceivablesFn func(userID, txnID string, in services.UpdateReceivablesInput) (*models.Transaction, error)
	settleFn            func(userID, id string, in services.SettleInput) (*models.Receivable, *models.ReceivableSettlement, error)
	deleteSettlementFn  func(userID, id string) (*models.Receivable, error)
	writeOffFn          func(userID, id string, amount *int64) (*models.Receivable, error)
	undoWriteOffFn      func(userID, id string) (*models.Receivable, error)
	listFn              func(userID string, page pagination.PageRequest, f services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error)
	getFn               func(userID, id string) (*models.Receivable, error)
	summaryFn           func(userID string) (*services.ReceivableSummary, error)
	counterpartiesFn    func(userID, q string) ([]services.CounterpartySuggestion, error)
	renameFn            func(userID, fromKey, toName string) (int64, error)
}

func (m *mockReceivableService) CreateSharedExpense(userID string, in services.CreateSharedExpenseInput) (*models.Transaction, error) {
	if m.createSharedFn != nil {
		return m.createSharedFn(userID, in)
	}
	return &models.Transaction{Base: models.Base{ID: "txn-1"}}, nil
}

func (m *mockReceivableService) UpdateReceivables(userID, txnID string, in services.UpdateReceivablesInput) (*models.Transaction, error) {
	if m.updateReceivablesFn != nil {
		return m.updateReceivablesFn(userID, txnID, in)
	}
	return &models.Transaction{Base: models.Base{ID: txnID}}, nil
}

func (m *mockReceivableService) Settle(userID, id string, in services.SettleInput) (*models.Receivable, *models.ReceivableSettlement, error) {
	if m.settleFn != nil {
		return m.settleFn(userID, id, in)
	}
	return &models.Receivable{Base: models.Base{ID: id}}, &models.ReceivableSettlement{Base: models.Base{ID: "set-1"}}, nil
}

func (m *mockReceivableService) DeleteSettlement(userID, id string) (*models.Receivable, error) {
	if m.deleteSettlementFn != nil {
		return m.deleteSettlementFn(userID, id)
	}
	return &models.Receivable{Base: models.Base{ID: "rec-1"}}, nil
}

func (m *mockReceivableService) WriteOff(userID, id string, amount *int64) (*models.Receivable, error) {
	if m.writeOffFn != nil {
		return m.writeOffFn(userID, id, amount)
	}
	return &models.Receivable{Base: models.Base{ID: id}}, nil
}

func (m *mockReceivableService) UndoWriteOff(userID, id string) (*models.Receivable, error) {
	if m.undoWriteOffFn != nil {
		return m.undoWriteOffFn(userID, id)
	}
	return &models.Receivable{Base: models.Base{ID: id}}, nil
}

func (m *mockReceivableService) ListReceivables(userID string, page pagination.PageRequest, f services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
	if m.listFn != nil {
		return m.listFn(userID, page, f)
	}
	resp := pagination.NewPageResponse([]models.Receivable{}, 1, 20, 0)
	return &resp, nil
}

func (m *mockReceivableService) GetReceivable(userID, id string) (*models.Receivable, error) {
	if m.getFn != nil {
		return m.getFn(userID, id)
	}
	return &models.Receivable{Base: models.Base{ID: id}}, nil
}

func (m *mockReceivableService) Summary(userID string) (*services.ReceivableSummary, error) {
	if m.summaryFn != nil {
		return m.summaryFn(userID)
	}
	return &services.ReceivableSummary{ByKind: []services.ReceivableKindTotal{}, ByCounterparty: []services.ReceivableCounterpartyTotal{}}, nil
}

func (m *mockReceivableService) ListCounterparties(userID, q string) ([]services.CounterpartySuggestion, error) {
	if m.counterpartiesFn != nil {
		return m.counterpartiesFn(userID, q)
	}
	return []services.CounterpartySuggestion{}, nil
}

func (m *mockReceivableService) RenameCounterparty(userID, fromKey, toName string) (int64, error) {
	if m.renameFn != nil {
		return m.renameFn(userID, fromKey, toName)
	}
	return 0, nil
}

var _ services.ReceivableServicer = (*mockReceivableService)(nil)

// setupReceivableRouter registers the routes exactly like cmd/api/main.go so
// route collisions between static segments and /:id would surface here.
func setupReceivableRouter(h *ReceivableHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))

	transactions := auth.Group("/transactions")
	transactions.POST("/shared", h.CreateSharedExpense)
	transactions.PUT("/:id/receivables", h.UpdateReceivables)

	receivables := auth.Group("/receivables")
	receivables.GET("", h.ListReceivables)
	receivables.GET("/summary", h.GetReceivablesSummary)
	receivables.GET("/counterparties", h.ListCounterparties)
	receivables.POST("/counterparties/rename", h.RenameCounterparty)
	receivables.DELETE("/settlements/:id", h.DeleteSettlement)
	receivables.GET("/:id", h.GetReceivable)
	receivables.POST("/:id/settle", h.SettleReceivable)
	receivables.POST("/:id/write-off", h.WriteOffReceivable)
	receivables.DELETE("/:id/write-off", h.UndoWriteOff)
	return r
}

func newReceivableTest(svc *mockReceivableService) (*gin.Engine, *recordingAudit) {
	audit := &recordingAudit{}
	return setupReceivableRouter(NewReceivableHandler(svc, audit)), audit
}

func TestReceivableHandler_CreateSharedExpense(t *testing.T) {
	const valid = `{"account_id":"` + rcvAcct + `","category_id":"` + rcvUUID + `","amount":6000,"description":"Dinner","date":"2026-03-05",` +
		`"kind":"split","mode":"equal","counterparties":[{"name":"Alex"},{"name":"Sam"}]}`

	t.Run("returns 201, passes parsed input to the service and audits", func(t *testing.T) {
		var got services.CreateSharedExpenseInput
		var gotUser string
		svc := &mockReceivableService{createSharedFn: func(userID string, in services.CreateSharedExpenseInput) (*models.Transaction, error) {
			gotUser, got = userID, in
			p := int64(2000)
			return &models.Transaction{Base: models.Base{ID: "txn-9"}, Amount: in.Amount, PersonalAmount: &p,
				Receivables: []models.Receivable{{Base: models.Base{ID: "r1"}, CounterpartyName: "Alex", Amount: 2000, Outstanding: 2000}}}, nil
		}}
		r, audit := newReceivableTest(svc)

		rec := doRequest(r, "POST", "/transactions/shared", valid)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		tx := parseJSON(t, rec)["transaction"].(map[string]interface{})
		if tx["personal_amount"].(float64) != 2000 || len(tx["receivables"].([]interface{})) != 1 {
			t.Errorf("body = %v", tx)
		}
		if gotUser != "test-user-1" || got.AccountID != rcvAcct || got.Amount != 6000 || got.Description != "Dinner" ||
			got.Kind != models.ReceivableKindSplit || got.Mode != services.ShareModeEqual || len(got.Counterparties) != 2 ||
			got.CategoryID == nil || *got.CategoryID != rcvUUID {
			t.Errorf("service input = %+v", got)
		}
		if want := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC); !got.Date.Equal(want) {
			t.Errorf("date = %v, want %v", got.Date, want)
		}
		e := audit.only(t)
		if e.action != "CREATE_SHARED_EXPENSE" || e.resourceType != "transaction" || e.resourceID != "txn-9" || e.userID != "test-user-1" {
			t.Errorf("audit = %+v", e)
		}
	})

	t.Run("custom amounts and RFC3339 date, empty category treated as absent", func(t *testing.T) {
		var got services.CreateSharedExpenseInput
		svc := &mockReceivableService{createSharedFn: func(_ string, in services.CreateSharedExpenseInput) (*models.Transaction, error) {
			got = in
			return &models.Transaction{Base: models.Base{ID: "t"}}, nil
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "POST", "/transactions/shared",
			`{"account_id":"`+rcvAcct+`","category_id":"","amount":100,"date":"2026-03-05T10:00:00Z","kind":"loan","mode":"custom","counterparties":[{"name":"A","amount":60}]}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if got.CategoryID != nil || got.Counterparties[0].Amount == nil || *got.Counterparties[0].Amount != 60 || got.Date.Hour() != 10 {
			t.Errorf("input = %+v", got)
		}
	})

	t.Run("omitted date reaches the service as the zero time", func(t *testing.T) {
		var got services.CreateSharedExpenseInput
		svc := &mockReceivableService{createSharedFn: func(_ string, in services.CreateSharedExpenseInput) (*models.Transaction, error) {
			got = in
			return &models.Transaction{Base: models.Base{ID: "t"}}, nil
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "POST", "/transactions/shared",
			`{"account_id":"`+rcvAcct+`","amount":100,"kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`)
		if rec.Code != http.StatusCreated || !got.Date.IsZero() {
			t.Fatalf("status %d date %v", rec.Code, got.Date)
		}
	})

	invalid := map[string]string{
		"malformed json":       `{`,
		"missing account":      `{"amount":100,"kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"non-uuid account":     `{"account_id":"abc","amount":100,"kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"non-uuid category":    `{"account_id":"` + rcvAcct + `","category_id":"abc","amount":100,"kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"missing amount":       `{"account_id":"` + rcvAcct + `","kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"zero amount":          `{"account_id":"` + rcvAcct + `","amount":0,"kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"negative amount":      `{"account_id":"` + rcvAcct + `","amount":-5,"kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"missing kind":         `{"account_id":"` + rcvAcct + `","amount":100,"mode":"full","counterparties":[{"name":"A"}]}`,
		"missing mode":         `{"account_id":"` + rcvAcct + `","amount":100,"kind":"loan","counterparties":[{"name":"A"}]}`,
		"unknown mode":         `{"account_id":"` + rcvAcct + `","amount":100,"kind":"loan","mode":"half","counterparties":[{"name":"A"}]}`,
		"bad date":             `{"account_id":"` + rcvAcct + `","amount":100,"date":"yesterday","kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
		"description too long": `{"account_id":"` + rcvAcct + `","amount":100,"description":"` + strings.Repeat("x", 501) + `","kind":"loan","mode":"full","counterparties":[{"name":"A"}]}`,
	}
	for name, body := range invalid {
		t.Run("returns 400 on "+name, func(t *testing.T) {
			called := false
			svc := &mockReceivableService{createSharedFn: func(string, services.CreateSharedExpenseInput) (*models.Transaction, error) {
				called = true
				return &models.Transaction{}, nil
			}}
			r, audit := newReceivableTest(svc)
			rec := doRequest(r, "POST", "/transactions/shared", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
			if called {
				t.Error("service must not be called")
			}
			audit.assertNone(t)
		})
	}

	serviceErrors := []struct {
		err    *apperrors.AppError
		status int
	}{
		{apperrors.ErrInvalidShares, http.StatusBadRequest},
		{apperrors.ErrInvalidReceivableKind, http.StatusBadRequest},
		{apperrors.ErrAccountNotFound, http.StatusNotFound},
		{apperrors.ErrCategoryNotFound, http.StatusNotFound},
	}
	for _, tt := range serviceErrors {
		t.Run("maps "+tt.err.Code, func(t *testing.T) {
			svc := &mockReceivableService{createSharedFn: func(string, services.CreateSharedExpenseInput) (*models.Transaction, error) {
				return nil, tt.err
			}}
			r, audit := newReceivableTest(svc)
			rec := doRequest(r, "POST", "/transactions/shared", valid)
			if rec.Code != tt.status {
				t.Fatalf("expected %d, got %d", tt.status, rec.Code)
			}
			assertErrorCode(t, parseJSON(t, rec), tt.err.Code)
			audit.assertNone(t)
		})
	}

	t.Run("returns 401 without auth", func(t *testing.T) {
		h := NewReceivableHandler(&mockReceivableService{}, &recordingAudit{})
		r := gin.New()
		r.POST("/transactions/shared", h.CreateSharedExpense)
		if rec := doRequest(r, "POST", "/transactions/shared", valid); rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestReceivableHandler_UpdateReceivables(t *testing.T) {
	t.Run("returns 200 with the diff passed through and audits", func(t *testing.T) {
		var got services.UpdateReceivablesInput
		svc := &mockReceivableService{updateReceivablesFn: func(_, _ string, in services.UpdateReceivablesInput) (*models.Transaction, error) {
			got = in
			return &models.Transaction{Base: models.Base{ID: rcvUUID}}, nil
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "PUT", "/transactions/"+rcvUUID+"/receivables",
			`{"kind":"claim","receivables":[{"id":"`+rcvUUID2+`","name":"Alex","amount":100},{"name":"New","amount":50}]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got.Kind == nil || *got.Kind != models.ReceivableKindClaim || len(got.Receivables) != 2 ||
			got.Receivables[0].ID == nil || *got.Receivables[0].ID != rcvUUID2 || got.Receivables[1].ID != nil || got.Receivables[1].Amount != 50 {
			t.Errorf("input = %+v", got)
		}
		e := audit.only(t)
		if e.action != "UPDATE_RECEIVABLES" || e.resourceType != "transaction" || e.resourceID != rcvUUID {
			t.Errorf("audit = %+v", e)
		}
	})

	t.Run("an empty list is valid and removes everything", func(t *testing.T) {
		var got services.UpdateReceivablesInput
		svc := &mockReceivableService{updateReceivablesFn: func(_, _ string, in services.UpdateReceivablesInput) (*models.Transaction, error) {
			got = in
			return &models.Transaction{}, nil
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "PUT", "/transactions/"+rcvUUID+"/receivables", `{"receivables":[]}`)
		if rec.Code != http.StatusOK || got.Receivables == nil || len(got.Receivables) != 0 {
			t.Fatalf("status %d input %+v", rec.Code, got)
		}
	})

	invalid := map[string]struct{ path, body string }{
		"missing receivables key": {"/transactions/" + rcvUUID + "/receivables", `{"kind":"split"}`},
		"malformed json":          {"/transactions/" + rcvUUID + "/receivables", `{`},
		"non-uuid transaction id": {"/transactions/abc/receivables", `{"receivables":[]}`},
		"non-uuid receivable id":  {"/transactions/" + rcvUUID + "/receivables", `{"receivables":[{"id":"abc","name":"A","amount":1}]}`},
	}
	for name, tt := range invalid {
		t.Run("returns 400 on "+name, func(t *testing.T) {
			r, audit := newReceivableTest(&mockReceivableService{})
			rec := doRequest(r, "PUT", tt.path, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			audit.assertNone(t)
		})
	}

	t.Run("maps RECEIVABLE_HAS_PAYMENTS to 409", func(t *testing.T) {
		svc := &mockReceivableService{updateReceivablesFn: func(_, _ string, _ services.UpdateReceivablesInput) (*models.Transaction, error) {
			return nil, apperrors.ErrReceivableHasPayments
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "PUT", "/transactions/"+rcvUUID+"/receivables", `{"receivables":[]}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "RECEIVABLE_HAS_PAYMENTS")
		audit.assertNone(t)
	})
}

func TestReceivableHandler_List(t *testing.T) {
	t.Run("passes filters and pagination", func(t *testing.T) {
		var gotFilter services.ReceivableFilter
		var gotPage pagination.PageRequest
		svc := &mockReceivableService{listFn: func(_ string, p pagination.PageRequest, f services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
			gotFilter, gotPage = f, p
			resp := pagination.NewPageResponse([]models.Receivable{{Base: models.Base{ID: "r1"}, Outstanding: 5}}, 2, 5, 11)
			return &resp, nil
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "GET", "/receivables?status=open&kind=loan&counterparty=alex&page=2&page_size=5", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotFilter != (services.ReceivableFilter{Status: "open", Kind: "loan", CounterpartyKey: "alex"}) || gotPage.Page != 2 || gotPage.PageSize != 5 {
			t.Errorf("filter %+v page %+v", gotFilter, gotPage)
		}
		body := parseJSON(t, rec)
		if body["total_items"].(float64) != 11 || body["total_pages"].(float64) != 3 || len(body["data"].([]interface{})) != 1 {
			t.Errorf("body = %v", body)
		}
	})
	t.Run("returns 400 on bad page_size", func(t *testing.T) {
		r, _ := newReceivableTest(&mockReceivableService{})
		if rec := doRequest(r, "GET", "/receivables?page_size=1000", ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
	t.Run("maps service validation errors", func(t *testing.T) {
		svc := &mockReceivableService{listFn: func(string, pagination.PageRequest, services.ReceivableFilter) (*pagination.PageResponse[models.Receivable], error) {
			return nil, apperrors.ErrInvalidReceivableKind
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "GET", "/receivables?kind=gift", "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_RECEIVABLE_KIND")
	})
}

func TestReceivableHandler_StaticRoutesDoNotCollideWithID(t *testing.T) {
	var hit string
	svc := &mockReceivableService{
		summaryFn: func(string) (*services.ReceivableSummary, error) {
			hit = "summary"
			return &services.ReceivableSummary{TotalOutstanding: 700, ByKind: []services.ReceivableKindTotal{}, ByCounterparty: []services.ReceivableCounterpartyTotal{}}, nil
		},
		counterpartiesFn: func(_, q string) ([]services.CounterpartySuggestion, error) {
			hit = "counterparties:" + q
			return []services.CounterpartySuggestion{{Key: "alex", Name: "Alex"}}, nil
		},
		getFn: func(_, id string) (*models.Receivable, error) {
			hit = "get:" + id
			return &models.Receivable{Base: models.Base{ID: id}}, nil
		},
		deleteSettlementFn: func(_, id string) (*models.Receivable, error) {
			hit = "delsettlement:" + id
			return &models.Receivable{Base: models.Base{ID: "rec"}}, nil
		},
		undoWriteOffFn: func(_, id string) (*models.Receivable, error) {
			hit = "undo:" + id
			return &models.Receivable{Base: models.Base{ID: id}}, nil
		},
		renameFn: func(string, string, string) (int64, error) {
			hit = "rename"
			return 3, nil
		},
	}
	r, _ := newReceivableTest(svc)

	tests := []struct {
		method, path, body, want string
	}{
		{"GET", "/receivables/summary", "", "summary"},
		{"GET", "/receivables/counterparties?q=al", "", "counterparties:al"},
		{"GET", "/receivables/" + rcvUUID, "", "get:" + rcvUUID},
		{"DELETE", "/receivables/settlements/" + rcvUUID2, "", "delsettlement:" + rcvUUID2},
		{"DELETE", "/receivables/" + rcvUUID + "/write-off", "", "undo:" + rcvUUID},
		{"POST", "/receivables/counterparties/rename", `{"from_key":"a","to_name":"B"}`, "rename"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			hit = ""
			rec := doRequest(r, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusOK || hit != tt.want {
				t.Fatalf("status %d hit %q, want 200 %q (%s)", rec.Code, hit, tt.want, rec.Body.String())
			}
		})
	}

	t.Run("summary response shape", func(t *testing.T) {
		body := parseJSON(t, doRequest(r, "GET", "/receivables/summary", ""))
		if body["total_outstanding"].(float64) != 700 {
			t.Errorf("body = %v", body)
		}
		if _, ok := body["by_kind"].([]interface{}); !ok {
			t.Errorf("by_kind must be an array: %v", body)
		}
		if _, ok := body["by_counterparty"].([]interface{}); !ok {
			t.Errorf("by_counterparty must be an array: %v", body)
		}
	})
	t.Run("counterparties response shape", func(t *testing.T) {
		body := parseJSON(t, doRequest(r, "GET", "/receivables/counterparties?q=al", ""))
		cps := body["counterparties"].([]interface{})
		first := cps[0].(map[string]interface{})
		if first["key"] != "alex" || first["name"] != "Alex" {
			t.Errorf("body = %v", body)
		}
	})
}

func TestReceivableHandler_RenameCounterparty(t *testing.T) {
	t.Run("returns 200 with the updated count and audits", func(t *testing.T) {
		var from, to string
		svc := &mockReceivableService{renameFn: func(_, f, n string) (int64, error) {
			from, to = f, n
			return 4, nil
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "POST", "/receivables/counterparties/rename", `{"from_key":"alx","to_name":"Alex"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if parseJSON(t, rec)["updated"].(float64) != 4 || from != "alx" || to != "Alex" {
			t.Errorf("body %s from %q to %q", rec.Body.String(), from, to)
		}
		e := audit.only(t)
		if e.action != "RENAME_COUNTERPARTY" || e.resourceType != "receivable" || e.changes["updated"] != int64(4) {
			t.Errorf("audit = %+v", e)
		}
	})
	for name, body := range map[string]string{
		"missing from_key": `{"to_name":"Alex"}`,
		"missing to_name":  `{"from_key":"alx"}`,
		"malformed":        `{`,
	} {
		t.Run("returns 400 on "+name, func(t *testing.T) {
			r, audit := newReceivableTest(&mockReceivableService{})
			if rec := doRequest(r, "POST", "/receivables/counterparties/rename", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
			audit.assertNone(t)
		})
	}
	t.Run("maps INVALID_SHARES for a bad name", func(t *testing.T) {
		svc := &mockReceivableService{renameFn: func(_, _, _ string) (int64, error) { return 0, apperrors.ErrInvalidShares }}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "POST", "/receivables/counterparties/rename", `{"from_key":"a","to_name":"b"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_SHARES")
		audit.assertNone(t)
	})
}

func TestReceivableHandler_Get(t *testing.T) {
	t.Run("returns 200 wrapped in receivable", func(t *testing.T) {
		svc := &mockReceivableService{getFn: func(_, id string) (*models.Receivable, error) {
			return &models.Receivable{Base: models.Base{ID: id}, Outstanding: 42,
				Settlements: []models.ReceivableSettlement{{Base: models.Base{ID: "s1"}, Amount: 5}}}, nil
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "GET", "/receivables/"+rcvUUID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		got := parseJSON(t, rec)["receivable"].(map[string]interface{})
		if got["outstanding"].(float64) != 42 || len(got["settlements"].([]interface{})) != 1 {
			t.Errorf("receivable = %v", got)
		}
	})
	t.Run("returns 404", func(t *testing.T) {
		svc := &mockReceivableService{getFn: func(_, _ string) (*models.Receivable, error) { return nil, apperrors.ErrReceivableNotFound }}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "GET", "/receivables/"+rcvUUID, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "RECEIVABLE_NOT_FOUND")
	})
	t.Run("returns 400 on a non-uuid id", func(t *testing.T) {
		r, _ := newReceivableTest(&mockReceivableService{})
		if rec := doRequest(r, "GET", "/receivables/not-a-uuid", ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestReceivableHandler_Settle(t *testing.T) {
	path := "/receivables/" + rcvUUID + "/settle"

	t.Run("returns 201 with receivable and settlement, audits", func(t *testing.T) {
		var got services.SettleInput
		var gotID string
		svc := &mockReceivableService{settleFn: func(_, id string, in services.SettleInput) (*models.Receivable, *models.ReceivableSettlement, error) {
			gotID, got = id, in
			return &models.Receivable{Base: models.Base{ID: id}, Status: models.ReceivableStatusSettled},
				&models.ReceivableSettlement{Base: models.Base{ID: "set-7"}, Amount: in.Amount, AccountID: in.AccountID}, nil
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "POST", path,
			`{"account_id":"`+rcvAcct+`","amount":2000,"date":"2026-03-06","description":"venmo","write_off_remainder":true}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := parseJSON(t, rec)
		if body["receivable"].(map[string]interface{})["status"] != "settled" || body["settlement"].(map[string]interface{})["amount"].(float64) != 2000 {
			t.Errorf("body = %v", body)
		}
		if gotID != rcvUUID || got.AccountID != rcvAcct || got.Amount != 2000 || got.Description != "venmo" || !got.WriteOffRemainder ||
			!got.Date.Equal(time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("input = %+v id %s", got, gotID)
		}
		e := audit.only(t)
		if e.action != "SETTLE_RECEIVABLE" || e.resourceType != "receivable" || e.resourceID != rcvUUID || e.changes["settlement_id"] != "set-7" {
			t.Errorf("audit = %+v", e)
		}
	})

	invalid := map[string]struct{ path, body string }{
		"missing account":  {path, `{"amount":100}`},
		"non-uuid account": {path, `{"account_id":"abc","amount":100}`},
		"missing amount":   {path, `{"account_id":"` + rcvAcct + `"}`},
		"bad date":         {path, `{"account_id":"` + rcvAcct + `","amount":100,"date":"soon"}`},
		"long description": {path, `{"account_id":"` + rcvAcct + `","amount":100,"description":"` + strings.Repeat("x", 501) + `"}`},
		"malformed":        {path, `{`},
		"non-uuid id":      {"/receivables/abc/settle", `{"account_id":"` + rcvAcct + `","amount":100}`},
	}
	for name, tt := range invalid {
		t.Run("returns 400 on "+name, func(t *testing.T) {
			r, audit := newReceivableTest(&mockReceivableService{})
			rec := doRequest(r, "POST", tt.path, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
			audit.assertNone(t)
		})
	}

	t.Run("a zero amount reaches the service, which owns the contract error", func(t *testing.T) {
		svc := &mockReceivableService{settleFn: func(_, _ string, in services.SettleInput) (*models.Receivable, *models.ReceivableSettlement, error) {
			if in.Amount != 0 {
				t.Errorf("amount = %d", in.Amount)
			}
			return nil, nil, apperrors.ErrSettlementExceedsOutstanding
		}}
		r, _ := newReceivableTest(svc)
		rec := doRequest(r, "POST", path, `{"account_id":"`+rcvAcct+`","amount":0}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		assertErrorCode(t, parseJSON(t, rec), "SETTLEMENT_EXCEEDS_OUTSTANDING")
	})

	serviceErrors := []struct {
		err    *apperrors.AppError
		status int
	}{
		{apperrors.ErrSettlementExceedsOutstanding, http.StatusBadRequest},
		{apperrors.ErrInvalidSettlementAccount, http.StatusBadRequest},
		{apperrors.ErrReceivableNotFound, http.StatusNotFound},
		{apperrors.ErrAccountNotFound, http.StatusNotFound},
	}
	for _, tt := range serviceErrors {
		t.Run("maps "+tt.err.Code, func(t *testing.T) {
			svc := &mockReceivableService{settleFn: func(string, string, services.SettleInput) (*models.Receivable, *models.ReceivableSettlement, error) {
				return nil, nil, tt.err
			}}
			r, audit := newReceivableTest(svc)
			rec := doRequest(r, "POST", path, `{"account_id":"`+rcvAcct+`","amount":100}`)
			if rec.Code != tt.status {
				t.Fatalf("expected %d, got %d", tt.status, rec.Code)
			}
			assertErrorCode(t, parseJSON(t, rec), tt.err.Code)
			audit.assertNone(t)
		})
	}
}

func TestReceivableHandler_DeleteSettlement(t *testing.T) {
	t.Run("returns 200 with the receivable and audits", func(t *testing.T) {
		svc := &mockReceivableService{deleteSettlementFn: func(_, id string) (*models.Receivable, error) {
			return &models.Receivable{Base: models.Base{ID: "rec-5"}, Status: models.ReceivableStatusPending, Outstanding: 2000}, nil
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "DELETE", "/receivables/settlements/"+rcvUUID2, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := parseJSON(t, rec)["receivable"].(map[string]interface{}); got["id"] != "rec-5" || got["status"] != "pending" {
			t.Errorf("receivable = %v", got)
		}
		e := audit.only(t)
		if e.action != "DELETE_SETTLEMENT" || e.resourceType != "receivable" || e.resourceID != "rec-5" || e.changes["settlement_id"] != rcvUUID2 {
			t.Errorf("audit = %+v", e)
		}
	})
	t.Run("returns 404 when the settlement is unknown", func(t *testing.T) {
		svc := &mockReceivableService{deleteSettlementFn: func(_, _ string) (*models.Receivable, error) { return nil, apperrors.ErrSettlementNotFound }}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "DELETE", "/receivables/settlements/"+rcvUUID2, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "SETTLEMENT_NOT_FOUND")
		audit.assertNone(t)
	})
	t.Run("returns 400 on a non-uuid id", func(t *testing.T) {
		r, _ := newReceivableTest(&mockReceivableService{})
		if rec := doRequest(r, "DELETE", "/receivables/settlements/nope", ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestReceivableHandler_WriteOff(t *testing.T) {
	path := "/receivables/" + rcvUUID + "/write-off"

	bodies := map[string]*int64{
		"explicit amount": ptr64(1500),
		"empty object":    nil,
		"no body":         nil,
	}
	payloads := map[string]string{"explicit amount": `{"amount":1500}`, "empty object": `{}`, "no body": ``}
	for name, want := range bodies {
		t.Run("accepts "+name, func(t *testing.T) {
			var got *int64
			called := false
			svc := &mockReceivableService{writeOffFn: func(_, _ string, amount *int64) (*models.Receivable, error) {
				got, called = amount, true
				return &models.Receivable{Base: models.Base{ID: rcvUUID}, WrittenOffAmount: 1500}, nil
			}}
			r, audit := newReceivableTest(svc)
			rec := doRequest(r, "POST", path, payloads[name])
			if rec.Code != http.StatusOK || !called {
				t.Fatalf("status %d called %v: %s", rec.Code, called, rec.Body.String())
			}
			if (want == nil) != (got == nil) || (want != nil && *want != *got) {
				t.Errorf("amount = %v, want %v", got, want)
			}
			e := audit.only(t)
			if e.action != "WRITE_OFF_RECEIVABLE" || e.resourceType != "receivable" || e.resourceID != rcvUUID {
				t.Errorf("audit = %+v", e)
			}
		})
	}

	t.Run("returns 400 on malformed json", func(t *testing.T) {
		r, audit := newReceivableTest(&mockReceivableService{})
		if rec := doRequest(r, "POST", path, `{"amount":`); rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		audit.assertNone(t)
	})
	t.Run("maps WRITE_OFF_EXCEEDS_OUTSTANDING", func(t *testing.T) {
		svc := &mockReceivableService{writeOffFn: func(_, _ string, _ *int64) (*models.Receivable, error) {
			return nil, apperrors.ErrWriteOffExceedsOutstanding
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "POST", path, `{"amount":999999}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "WRITE_OFF_EXCEEDS_OUTSTANDING")
		audit.assertNone(t)
	})
	t.Run("returns 404 for an unknown receivable", func(t *testing.T) {
		svc := &mockReceivableService{writeOffFn: func(_, _ string, _ *int64) (*models.Receivable, error) {
			return nil, apperrors.ErrReceivableNotFound
		}}
		r, _ := newReceivableTest(svc)
		if rec := doRequest(r, "POST", path, `{}`); rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func ptr64(v int64) *int64 { return &v }

func TestReceivableHandler_UndoWriteOff(t *testing.T) {
	path := "/receivables/" + rcvUUID + "/write-off"

	t.Run("returns 200 and audits", func(t *testing.T) {
		r, audit := newReceivableTest(&mockReceivableService{})
		rec := doRequest(r, "DELETE", path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		e := audit.only(t)
		if e.action != "UNDO_WRITE_OFF" || e.resourceType != "receivable" || e.resourceID != rcvUUID {
			t.Errorf("audit = %+v", e)
		}
	})
	t.Run("returns 400 INVALID_INPUT when there is nothing to undo", func(t *testing.T) {
		svc := &mockReceivableService{undoWriteOffFn: func(_, _ string) (*models.Receivable, error) {
			return nil, apperrors.WithMessage(apperrors.ErrInvalidInput, "nothing to undo")
		}}
		r, audit := newReceivableTest(svc)
		rec := doRequest(r, "DELETE", path, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorCode(t, parseJSON(t, rec), "INVALID_INPUT")
		audit.assertNone(t)
	})
	t.Run("returns 404", func(t *testing.T) {
		svc := &mockReceivableService{undoWriteOffFn: func(_, _ string) (*models.Receivable, error) {
			return nil, apperrors.ErrReceivableNotFound
		}}
		r, _ := newReceivableTest(svc)
		if rec := doRequest(r, "DELETE", path, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
