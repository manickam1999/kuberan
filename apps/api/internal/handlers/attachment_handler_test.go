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
	"kuberan/internal/models"
	"kuberan/internal/services"
)

// --- mock attachment service ---

type mockAttachmentService struct {
	uploadFn func(userID, txID, fileName, declaredType string, size int64, data io.Reader) (*models.TransactionAttachment, error)
	listFn   func(userID, txID string) ([]models.TransactionAttachment, error)
	openFn   func(userID, attachmentID string) (*models.TransactionAttachment, io.ReadCloser, error)
	deleteFn func(userID, attachmentID string) error
}

func (m *mockAttachmentService) Upload(userID, txID, fileName, declaredType string, size int64, data io.Reader) (*models.TransactionAttachment, error) {
	if m.uploadFn != nil {
		return m.uploadFn(userID, txID, fileName, declaredType, size, data)
	}
	return &models.TransactionAttachment{Base: models.Base{ID: "att-1"}}, nil
}

func (m *mockAttachmentService) List(userID, txID string) ([]models.TransactionAttachment, error) {
	if m.listFn != nil {
		return m.listFn(userID, txID)
	}
	return nil, nil
}

func (m *mockAttachmentService) Open(userID, attachmentID string) (*models.TransactionAttachment, io.ReadCloser, error) {
	if m.openFn != nil {
		return m.openFn(userID, attachmentID)
	}
	return &models.TransactionAttachment{Base: models.Base{ID: attachmentID}}, io.NopCloser(strings.NewReader("")), nil
}

func (m *mockAttachmentService) Delete(userID, attachmentID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(userID, attachmentID)
	}
	return nil
}

var _ services.AttachmentServicer = (*mockAttachmentService)(nil)

// A valid UUID is required by parsePathID for :id and :aid path params.
const (
	testTxID  = "018f0000-0000-7000-8000-000000000001"
	testAttID = "018f0000-0000-7000-8000-000000000002"
)

func setupAttachmentRouter(handler *AttachmentHandler) *gin.Engine {
	r := gin.New()
	auth := r.Group("", injectUserID("test-user-1"))
	auth.POST("/transactions/:id/attachments", handler.Upload)
	auth.GET("/transactions/:id/attachments", handler.List)
	auth.GET("/transactions/:id/attachments/:aid", handler.Download)
	auth.DELETE("/transactions/:id/attachments/:aid", handler.Delete)
	return r
}

// multipartRequest builds a multipart/form-data POST with a single "file" field.
func multipartRequest(t *testing.T, path, fieldName, fileName, contentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{`form-data; name="` + fieldName + `"; filename="` + fileName + `"`}
	if contentType != "" {
		hdr["Content-Type"] = []string{contentType}
	}
	part, err := w.CreatePart(hdr)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestAttachmentHandler_Upload(t *testing.T) {
	t.Run("returns 201 on success", func(t *testing.T) {
		var gotFileName, gotDeclaredType string
		svc := &mockAttachmentService{
			uploadFn: func(_, _, fileName, declaredType string, _ int64, _ io.Reader) (*models.TransactionAttachment, error) {
				gotFileName = fileName
				gotDeclaredType = declaredType
				return &models.TransactionAttachment{
					Base:        models.Base{ID: testAttID},
					ContentType: "image/jpeg",
					ByteSize:    42,
				}, nil
			},
		}
		handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		req := multipartRequest(t, "/transactions/"+testTxID+"/attachments", "file", "receipt.jpg", "image/jpeg", []byte("fake-bytes"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotFileName != "receipt.jpg" {
			t.Errorf("expected filename receipt.jpg, got %q", gotFileName)
		}
		if gotDeclaredType != "image/jpeg" {
			t.Errorf("expected declared type image/jpeg, got %q", gotDeclaredType)
		}
		result := parseJSON(t, rec)
		att := result["attachment"].(map[string]interface{})
		if att["id"] != testAttID {
			t.Errorf("expected id %s, got %v", testAttID, att["id"])
		}
	})

	t.Run("returns 400 on invalid transaction ID", func(t *testing.T) {
		handler := NewAttachmentHandler(&mockAttachmentService{}, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		req := multipartRequest(t, "/transactions/not-a-uuid/attachments", "file", "r.jpg", "image/jpeg", []byte("x"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 on missing file field", func(t *testing.T) {
		handler := NewAttachmentHandler(&mockAttachmentService{}, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		req := multipartRequest(t, "/transactions/"+testTxID+"/attachments", "wrongfield", "r.jpg", "image/jpeg", []byte("x"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("propagates service error code (415)", func(t *testing.T) {
		svc := &mockAttachmentService{
			uploadFn: func(_, _, _, _ string, _ int64, _ io.Reader) (*models.TransactionAttachment, error) {
				return nil, apperrors.ErrUnsupportedMediaType
			},
		}
		handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		req := multipartRequest(t, "/transactions/"+testTxID+"/attachments", "file", "r.txt", "text/plain", []byte("x"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", rec.Code)
		}
	})
}

func TestAttachmentHandler_List(t *testing.T) {
	svc := &mockAttachmentService{
		listFn: func(_, _ string) ([]models.TransactionAttachment, error) {
			return []models.TransactionAttachment{
				{Base: models.Base{ID: "a1"}, FileName: "one.jpg"},
				{Base: models.Base{ID: "a2"}, FileName: "two.pdf"},
			}, nil
		},
	}
	handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
	r := setupAttachmentRouter(handler)

	rec := doRequest(r, "GET", "/transactions/"+testTxID+"/attachments", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	result := parseJSON(t, rec)
	atts := result["attachments"].([]interface{})
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(atts))
	}
}

func TestAttachmentHandler_Download(t *testing.T) {
	t.Run("streams bytes with hardened headers", func(t *testing.T) {
		payload := "the-image-bytes"
		svc := &mockAttachmentService{
			openFn: func(_, aid string) (*models.TransactionAttachment, io.ReadCloser, error) {
				return &models.TransactionAttachment{
						Base:        models.Base{ID: aid},
						ContentType: "image/png",
					},
					io.NopCloser(strings.NewReader(payload)), nil
			},
		}
		handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		rec := doRequest(r, "GET", "/transactions/"+testTxID+"/attachments/"+testAttID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if rec.Body.String() != payload {
			t.Errorf("expected body %q, got %q", payload, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "image/png" {
			t.Errorf("expected Content-Type image/png, got %q", got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("expected nosniff, got %q", got)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
			t.Errorf("expected sandbox CSP, got %q", got)
		}
		if got := rec.Header().Get("Content-Disposition"); got != "inline" {
			t.Errorf("expected inline disposition, got %q", got)
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := &mockAttachmentService{
			openFn: func(_, _ string) (*models.TransactionAttachment, io.ReadCloser, error) {
				return nil, nil, apperrors.ErrAttachmentNotFound
			},
		}
		handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		rec := doRequest(r, "GET", "/transactions/"+testTxID+"/attachments/"+testAttID, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestAttachmentHandler_Delete(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		var deletedID string
		svc := &mockAttachmentService{
			deleteFn: func(_, aid string) error {
				deletedID = aid
				return nil
			},
		}
		handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		rec := doRequest(r, "DELETE", "/transactions/"+testTxID+"/attachments/"+testAttID, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if deletedID != testAttID {
			t.Errorf("expected delete of %s, got %s", testAttID, deletedID)
		}
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := &mockAttachmentService{
			deleteFn: func(_, _ string) error {
				return apperrors.ErrAttachmentNotFound
			},
		}
		handler := NewAttachmentHandler(svc, &mockAuditService{}, 10<<20)
		r := setupAttachmentRouter(handler)

		rec := doRequest(r, "DELETE", "/transactions/"+testTxID+"/attachments/"+testAttID, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
