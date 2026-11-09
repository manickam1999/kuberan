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

type mockCSVImportService struct {
	fn    func(userID string, p services.CSVImportParams, file io.Reader) (*services.CSVImportReport, error)
	calls int
	got   services.CSVImportParams
	read  string
}

func (m *mockCSVImportService) ImportCSV(userID string, p services.CSVImportParams, file io.Reader) (*services.CSVImportReport, error) {
	m.calls++
	m.got = p
	b, _ := io.ReadAll(file)
	m.read = string(b)
	if m.fn != nil {
		return m.fn(userID, p, file)
	}
	return &services.CSVImportReport{Applied: p.Apply, Created: 2, Summary: &services.CSVImportSummary{Rows: 3}}, nil
}

func csvRouter(h *CSVImportHandler, withUser bool) *gin.Engine {
	r := gin.New()
	if withUser {
		r.POST("/imports/csv", injectUserID("test-user-1"), h.ImportCSV)
	} else {
		r.POST("/imports/csv", h.ImportCSV)
	}
	return r
}

func csvRequest(t *testing.T, files []string, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, content := range files {
		part, err := w.CreateFormFile("file", "statement.csv")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(content))
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/imports/csv", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestCSVImportHandler_Parsing(t *testing.T) {
	svc := &mockCSVImportService{}
	r := csvRouter(NewCSVImportHandler(svc, &mockAuditService{}, 1024), true)

	t.Run("inspect only without mapping fields", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, csvRequest(t, []string{"a,b\n1,2\n"}, map[string]string{"account_id": impCashID}))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if svc.got.Mapping != nil || svc.got.Apply || svc.got.AccountID != impCashID || svc.read != "a,b\n1,2\n" {
			t.Errorf("params %+v read %q", svc.got, svc.read)
		}
	})
	t.Run("mapping and apply", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, csvRequest(t, []string{"x\n"}, map[string]string{
			"account_id": impCashID, "apply": "true", "date_column": "Date", "description_column": "Memo",
			"debit_column": "Out", "credit_column": "In", "date_format": "DD/MM/YYYY", "decimal_separator": ",",
		}))
		m := svc.got.Mapping
		if rec.Code != http.StatusOK || m == nil || !svc.got.Apply {
			t.Fatalf("status %d params %+v", rec.Code, svc.got)
		}
		if m.DateColumn != "Date" || m.DescriptionColumn != "Memo" || m.DebitColumn != "Out" || m.CreditColumn != "In" ||
			m.DateFormat != "DD/MM/YYYY" || m.DecimalSeparator != "," {
			t.Errorf("mapping %+v", m)
		}
	})

	bad := map[string]map[string]string{
		"missing account": {},
		"bad account":     {"account_id": "nope"},
		"bad apply":       {"account_id": impCashID, "apply": "maybe"},
	}
	for name, fields := range bad {
		t.Run(name, func(t *testing.T) {
			svc.calls = 0
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, csvRequest(t, []string{"x\n"}, fields))
			if rec.Code != http.StatusBadRequest || svc.calls != 0 {
				t.Errorf("status %d calls %d", rec.Code, svc.calls)
			}
		})
	}
}

func TestCSVImportHandler_Limits(t *testing.T) {
	svc := &mockCSVImportService{}
	r := csvRouter(NewCSVImportHandler(svc, &mockAuditService{}, 100), true)
	fields := map[string]string{"account_id": impCashID}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, csvRequest(t, nil, fields))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no file: status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, csvRequest(t, []string{"a\n", "b\n"}, fields))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("two files: status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, csvRequest(t, []string{strings.Repeat("a", 101)}, fields))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, csvRequest(t, []string{strings.Repeat("a", 100)}, fields))
	if rec.Code != http.StatusOK {
		t.Errorf("at the limit: status %d", rec.Code)
	}
	req := httptest.NewRequest("POST", "/imports/csv", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("not multipart: status %d", rec.Code)
	}
}

func TestCSVImportHandler_ErrorsAndAudit(t *testing.T) {
	fields := map[string]string{"account_id": impCashID}
	rec := httptest.NewRecorder()
	csvRouter(NewCSVImportHandler(&mockCSVImportService{}, &mockAuditService{}, 1024), false).
		ServeHTTP(rec, csvRequest(t, []string{"x"}, fields))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated: status %d", rec.Code)
	}

	svc := &mockCSVImportService{fn: func(string, services.CSVImportParams, io.Reader) (*services.CSVImportReport, error) {
		return nil, apperrors.ErrCSVTooManyRows
	}}
	rec = httptest.NewRecorder()
	csvRouter(NewCSVImportHandler(svc, &mockAuditService{}, 1024), true).ServeHTTP(rec, csvRequest(t, []string{"x"}, fields))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "CSV_TOO_MANY_ROWS") {
		t.Errorf("status %d body %s", rec.Code, rec.Body)
	}

	audit := &importAuditRecorder{}
	r := csvRouter(NewCSVImportHandler(&mockCSVImportService{}, audit, 1024), true)
	r.ServeHTTP(httptest.NewRecorder(), csvRequest(t, []string{"x"}, fields))
	if len(audit.actions) != 0 {
		t.Errorf("dry run audited: %v", audit.actions)
	}
	fields["apply"] = "true"
	fields["date_column"] = "Date"
	r.ServeHTTP(httptest.NewRecorder(), csvRequest(t, []string{"x"}, fields))
	if len(audit.actions) != 1 || audit.actions[0] != "IMPORT_CSV" {
		t.Fatalf("audit: %v", audit.actions)
	}
	for k, v := range audit.changes[0] {
		if _, ok := v.(int); !ok {
			t.Errorf("audit field %s is not a count: %v", k, v)
		}
	}
}
