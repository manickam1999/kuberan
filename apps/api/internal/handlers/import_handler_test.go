package handlers

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

type mockImportService struct {
	fn    func(userID string, p services.IBKRImportParams, files []io.Reader) (*services.IBKRImportReport, error)
	calls int
	got   services.IBKRImportParams
	read  []string
}

func (m *mockImportService) ImportIBKR(userID string, p services.IBKRImportParams, files []io.Reader) (*services.IBKRImportReport, error) {
	m.calls++
	m.got = p
	for _, f := range files {
		b, _ := io.ReadAll(f)
		m.read = append(m.read, string(b))
	}
	if m.fn != nil {
		return m.fn(userID, p, files)
	}
	return &services.IBKRImportReport{Applied: p.Apply, Account: "U1", Securities: []services.IBKRImportSecurity{{Symbol: "NVO", NewTransactions: 3}}}, nil
}

const (
	impInvID  = "018f0000-0000-7000-8000-0000000000a1"
	impCashID = "018f0000-0000-7000-8000-0000000000a2"
	impLegID  = "018f0000-0000-7000-8000-0000000000a3"
)

func importRouter(h *ImportHandler, withUser bool) *gin.Engine {
	r := gin.New()
	if withUser {
		r.POST("/imports/ibkr", injectUserID("test-user-1"), h.ImportIBKR)
	} else {
		r.POST("/imports/ibkr", h.ImportIBKR)
	}
	return r
}

type importForm struct {
	files  []string
	fields map[string]string
}

func (f importForm) request(t *testing.T) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for i, content := range f.files {
		part, err := w.CreateFormFile("files", "statement"+string(rune('a'+i))+".csv")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(content))
	}
	for k, v := range f.fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/imports/ibkr", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func validFields() map[string]string {
	return map[string]string{"investment_account_id": impInvID, "cash_account_id": impCashID, "cash_start_date": "2026-01-01"}
}

func TestImportHandler_Parsing(t *testing.T) {
	svc := &mockImportService{}
	h := NewImportHandler(svc, &mockAuditService{}, 1024)
	r := importRouter(h, true)

	t.Run("dry run by default", func(t *testing.T) {
		svc.calls, svc.read = 0, nil
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{files: []string{"a,b\n", "c,d\n"}, fields: validFields()}.request(t))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if svc.got.Apply || svc.got.DeactivateLegacyCashAccount || svc.got.LegacyCashAccountID != "" {
			t.Errorf("unexpected defaults: %+v", svc.got)
		}
		if svc.got.InvestmentAccountID != impInvID || svc.got.CashAccountID != impCashID || svc.got.CashStartDate.Format("2006-01-02") != "2026-01-01" {
			t.Errorf("params: %+v", svc.got)
		}
		if len(svc.read) != 2 || svc.read[0] != "a,b\n" || svc.read[1] != "c,d\n" {
			t.Errorf("files: %q", svc.read)
		}
		if !strings.Contains(rec.Body.String(), `"applied":false`) {
			t.Errorf("body: %s", rec.Body)
		}
	})

	t.Run("apply and legacy fields", func(t *testing.T) {
		f := validFields()
		f["apply"], f["legacy_cash_account_id"], f["deactivate_legacy_cash_account"] = "true", impLegID, "true"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{files: []string{"x\n"}, fields: f}.request(t))
		if rec.Code != http.StatusOK || !svc.got.Apply || !svc.got.DeactivateLegacyCashAccount || svc.got.LegacyCashAccountID != impLegID {
			t.Fatalf("status %d params %+v", rec.Code, svc.got)
		}
	})

	bad := map[string]func(f map[string]string){
		"missing investment account": func(f map[string]string) { delete(f, "investment_account_id") },
		"bad cash account":           func(f map[string]string) { f["cash_account_id"] = "nope" },
		"bad legacy account":         func(f map[string]string) { f["legacy_cash_account_id"] = "nope" },
		"missing start date":         func(f map[string]string) { delete(f, "cash_start_date") },
		"bad start date":             func(f map[string]string) { f["cash_start_date"] = "01/01/2026" },
		"bad apply":                  func(f map[string]string) { f["apply"] = "maybe" },
		"bad deactivate":             func(f map[string]string) { f["deactivate_legacy_cash_account"] = "x" },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			svc.calls = 0
			f := validFields()
			mutate(f)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, importForm{files: []string{"x\n"}, fields: f}.request(t))
			if rec.Code != http.StatusBadRequest || svc.calls != 0 {
				t.Errorf("status %d, service calls %d, body %s", rec.Code, svc.calls, rec.Body)
			}
		})
	}
}

func TestImportHandler_Limits(t *testing.T) {
	svc := &mockImportService{}
	h := NewImportHandler(svc, &mockAuditService{}, 100)
	r := importRouter(h, true)

	t.Run("no files", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{fields: validFields()}.request(t))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("more than ten files", func(t *testing.T) {
		files := make([]string, 11)
		for i := range files {
			files[i] = "x\n"
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{files: files, fields: validFields()}.request(t))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("ten files are fine", func(t *testing.T) {
		files := make([]string, 10)
		for i := range files {
			files[i] = "x\n"
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{files: files, fields: validFields()}.request(t))
		if rec.Code != http.StatusOK {
			t.Errorf("status %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("file over the size limit", func(t *testing.T) {
		svc.calls = 0
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{files: []string{strings.Repeat("a", 101)}, fields: validFields()}.request(t))
		if rec.Code != http.StatusRequestEntityTooLarge || svc.calls != 0 {
			t.Errorf("status %d, calls %d", rec.Code, svc.calls)
		}
	})
	t.Run("file at the size limit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, importForm{files: []string{strings.Repeat("a", 100)}, fields: validFields()}.request(t))
		if rec.Code != http.StatusOK {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("not multipart", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/imports/ibkr", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %d", rec.Code)
		}
	})
}

func TestImportHandler_Errors(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		h := NewImportHandler(&mockImportService{}, &mockAuditService{}, 1024)
		rec := httptest.NewRecorder()
		importRouter(h, false).ServeHTTP(rec, importForm{files: []string{"x"}, fields: validFields()}.request(t))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("ownership error is passed through", func(t *testing.T) {
		svc := &mockImportService{fn: func(string, services.IBKRImportParams, []io.Reader) (*services.IBKRImportReport, error) {
			return nil, apperrors.ErrAccountNotFound
		}}
		rec := httptest.NewRecorder()
		importRouter(NewImportHandler(svc, &mockAuditService{}, 1024), true).ServeHTTP(rec, importForm{files: []string{"x"}, fields: validFields()}.request(t))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "ACCOUNT_NOT_FOUND") {
			t.Errorf("status %d body %s", rec.Code, rec.Body)
		}
	})
	t.Run("IMPORT_INVALID carries the report", func(t *testing.T) {
		svc := &mockImportService{fn: func(string, services.IBKRImportParams, []io.Reader) (*services.IBKRImportReport, error) {
			return nil, apperrors.WithDetails(apperrors.ErrImportInvalid, map[string]any{"report": map[string]any{"errors": []string{"x"}}})
		}}
		rec := httptest.NewRecorder()
		importRouter(NewImportHandler(svc, &mockAuditService{}, 1024), true).ServeHTTP(rec, importForm{files: []string{"x"}, fields: validFields()}.request(t))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "IMPORT_INVALID") || !strings.Contains(rec.Body.String(), `"report"`) {
			t.Errorf("status %d body %s", rec.Code, rec.Body)
		}
	})
}

type importAuditRecorder struct {
	actions []string
	changes []map[string]interface{}
}

func (a *importAuditRecorder) Log(_, action, _, _, _ string, changes map[string]interface{}, _ ...services.LogOpts) {
	a.actions = append(a.actions, action)
	a.changes = append(a.changes, changes)
}

func TestImportHandler_AuditsApplyOnly(t *testing.T) {
	audit := &importAuditRecorder{}
	h := NewImportHandler(&mockImportService{}, audit, 1024)
	r := importRouter(h, true)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, importForm{files: []string{"x"}, fields: validFields()}.request(t))
	if len(audit.actions) != 0 {
		t.Errorf("dry run audited: %v", audit.actions)
	}

	f := validFields()
	f["apply"] = "true"
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, importForm{files: []string{"x"}, fields: f}.request(t))
	if len(audit.actions) != 1 || audit.actions[0] != "IMPORT_IBKR" {
		t.Fatalf("audit: %v", audit.actions)
	}
	for k, v := range audit.changes[0] {
		if _, ok := v.(int); !ok {
			t.Errorf("audit field %s is not a count: %v", k, v)
		}
	}
}
