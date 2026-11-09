package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/middleware"
	"kuberan/internal/models"
	"kuberan/internal/pagination"
	"kuberan/internal/services"
)

func TestTransactionHandler_NotesAndSource(t *testing.T) {
	const body = `{"account_id":"00000000-0000-0000-0000-000000000001","type":"expense","amount":100,"notes":"hello"}`

	t.Run("REST writes source web with the notes", func(t *testing.T) {
		svc := &mockTransactionService{}
		r := setupTransactionRouter(NewTransactionHandler(svc, &mockAuditService{}))
		rec := doRequest(r, "POST", "/transactions", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if svc.lastMeta.Source != models.TransactionSourceWeb || svc.lastMeta.Notes != "hello" {
			t.Errorf("meta = %+v", svc.lastMeta)
		}
	})

	t.Run("a src telegram claim writes source telegram", func(t *testing.T) {
		svc := &mockTransactionService{}
		h := NewTransactionHandler(svc, &mockAuditService{})
		r := gin.New()
		r.Use(injectUserID("u1"), func(c *gin.Context) {
			c.Set(middleware.ContextKeySource, middleware.SourceTelegram)
			c.Next()
		})
		r.POST("/transactions", h.CreateTransaction)
		r.POST("/transactions/transfer", h.CreateTransfer)

		if rec := doRequest(r, "POST", "/transactions", body); rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if svc.lastMeta.Source != models.TransactionSourceTelegram {
			t.Errorf("create meta = %+v", svc.lastMeta)
		}

		svc.lastMeta = services.TransactionMeta{}
		rec := doRequest(r, "POST", "/transactions/transfer",
			`{"from_account_id":"a","to_account_id":"b","amount":100,"notes":"n"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if svc.lastMeta.Source != models.TransactionSourceTelegram || svc.lastMeta.Notes != "n" {
			t.Errorf("transfer meta = %+v", svc.lastMeta)
		}
	})

	t.Run("notes over 2000 characters are rejected", func(t *testing.T) {
		r := setupTransactionRouter(NewTransactionHandler(&mockTransactionService{}, &mockAuditService{}))
		long := strings.Repeat("é", 2001)
		rec := doRequest(r, "POST", "/transactions",
			`{"account_id":"00000000-0000-0000-0000-000000000001","type":"expense","amount":100,"notes":"`+long+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestTransactionHandler_SearchFilters(t *testing.T) {
	var got services.TransactionFilter
	svc := &mockTransactionService{
		getUserTransactionsFn: func(_ string, _ pagination.PageRequest, f services.TransactionFilter) (*pagination.PageResponse[models.Transaction], error) {
			got = f
			resp := pagination.NewPageResponse([]models.Transaction{}, 1, 5, 0)
			return &resp, nil
		},
	}
	r := setupTransactionRouter(NewTransactionHandler(svc, &mockAuditService{}))

	rec := doRequest(r, "GET", "/transactions?q=%20coffee%20&shared=true&has_attachment=false&page_size=5", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got.Search == nil || *got.Search != "coffee" {
		t.Errorf("search = %v", got.Search)
	}
	if got.Shared == nil || !*got.Shared || got.HasAttachment == nil || *got.HasAttachment {
		t.Errorf("shared=%v has_attachment=%v", got.Shared, got.HasAttachment)
	}

	rec = doRequest(r, "GET", "/transactions?q="+strings.Repeat("x", 300), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len([]rune(*got.Search)) != services.MaxSearchLen {
		t.Errorf("search not capped: %d", len(*got.Search))
	}

	rec = doRequest(r, "GET", "/transactions?shared=maybe", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for a bad boolean, got %d", rec.Code)
	}
}

func TestTransactionHandler_Bulk(t *testing.T) {
	t.Run("passes the request to the service", func(t *testing.T) {
		var got services.BulkTransactionInput
		svc := &mockTransactionService{
			bulkUpdateFn: func(_ string, in services.BulkTransactionInput) (*services.BulkTransactionResult, error) {
				got = in
				return &services.BulkTransactionResult{Updated: 2}, nil
			},
		}
		r := setupTransactionRouter(NewTransactionHandler(svc, &mockAuditService{}))
		rec := doRequest(r, "POST", "/transactions/bulk",
			`{"ids":["a","b"],"set":{"category_id":"c","notes_append":"x","shared":false}}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if len(got.IDs) != 2 || got.Set.CategoryID == nil || *got.Set.CategoryID != "c" ||
			got.Set.NotesAppend == nil || got.Set.Shared == nil || *got.Set.Shared || got.Delete {
			t.Errorf("input = %+v", got)
		}
		if parseJSON(t, rec)["updated"].(float64) != 2 {
			t.Errorf("body = %s", rec.Body.String())
		}
	})

	t.Run("validates the id count", func(t *testing.T) {
		r := setupTransactionRouter(NewTransactionHandler(&mockTransactionService{}, &mockAuditService{}))
		if rec := doRequest(r, "POST", "/transactions/bulk", `{"ids":[],"delete":true}`); rec.Code != http.StatusBadRequest {
			t.Errorf("empty ids: expected 400, got %d", rec.Code)
		}
		ids := `"` + strings.Repeat(`x","`, 200) + `x"`
		if rec := doRequest(r, "POST", "/transactions/bulk", `{"ids":[`+ids+`],"delete":true}`); rec.Code != http.StatusBadRequest {
			t.Errorf("201 ids: expected 400, got %d", rec.Code)
		}
	})

	t.Run("maps service errors", func(t *testing.T) {
		svc := &mockTransactionService{
			bulkUpdateFn: func(string, services.BulkTransactionInput) (*services.BulkTransactionResult, error) {
				return nil, apperrors.ErrCategoryTypeMismatch
			},
		}
		r := setupTransactionRouter(NewTransactionHandler(svc, &mockAuditService{}))
		rec := doRequest(r, "POST", "/transactions/bulk", `{"ids":["a"],"set":{"category_id":"c"}}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "CATEGORY_TYPE_MISMATCH") {
			t.Errorf("body = %s", rec.Body.String())
		}
		svc.bulkUpdateFn = func(string, services.BulkTransactionInput) (*services.BulkTransactionResult, error) {
			return nil, apperrors.ErrTransactionNotFound
		}
		if rec := doRequest(r, "POST", "/transactions/bulk", `{"ids":["a"],"delete":true}`); rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})
}
