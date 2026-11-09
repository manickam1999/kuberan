package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/services"
)

type mockDemoService struct {
	loadErr, removeErr error
	loads, removes     int
}

func (m *mockDemoService) Load(string) (*services.DemoResult, error) {
	m.loads++
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return &services.DemoResult{Rows: map[string]int{"accounts": 5}, Total: 5}, nil
}

func (m *mockDemoService) Remove(string) (*services.DemoResult, error) {
	m.removes++
	if m.removeErr != nil {
		return nil, m.removeErr
	}
	return &services.DemoResult{Rows: map[string]int{"accounts": 5}, Total: 5}, nil
}

func (m *mockDemoService) IsLoaded(string) (bool, error) { return false, nil }

func demoRouter(h *DemoHandler, withUser bool) *gin.Engine {
	r := gin.New()
	handlers := []gin.HandlerFunc{}
	if withUser {
		handlers = append(handlers, injectUserID("test-user-1"))
	}
	r.POST("/demo/load", append(handlers, h.LoadDemo)...)
	r.DELETE("/demo", append(handlers, h.RemoveDemo)...)
	return r
}

func TestDemoHandler_DisabledIs404(t *testing.T) {
	svc := &mockDemoService{}
	r := demoRouter(NewDemoHandler(svc, &mockAuditService{}, false), true)
	for _, req := range []*http.Request{
		httptest.NewRequest("POST", "/demo/load", http.NoBody),
		httptest.NewRequest("DELETE", "/demo", http.NoBody),
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status %d", req.Method, req.URL.Path, rec.Code)
		}
	}
	if svc.loads != 0 || svc.removes != 0 {
		t.Errorf("service called while disabled: %d loads, %d removes", svc.loads, svc.removes)
	}
}

func TestDemoHandler_Enabled(t *testing.T) {
	svc := &mockDemoService{}
	audit := &importAuditRecorder{}
	r := demoRouter(NewDemoHandler(svc, audit, true), true)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/demo/load", http.NoBody))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"total":5`) {
		t.Fatalf("load: status %d body %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("DELETE", "/demo", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: status %d body %s", rec.Code, rec.Body)
	}
	if len(audit.actions) != 2 || audit.actions[0] != "DEMO_LOAD" || audit.actions[1] != "DEMO_REMOVE" {
		t.Errorf("audit: %v", audit.actions)
	}
}

func TestDemoHandler_Errors(t *testing.T) {
	svc := &mockDemoService{loadErr: apperrors.ErrDemoHasData, removeErr: apperrors.ErrDemoInUse}
	r := demoRouter(NewDemoHandler(svc, &mockAuditService{}, true), true)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/demo/load", http.NoBody))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "DEMO_HAS_DATA") {
		t.Errorf("load: status %d body %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("DELETE", "/demo", http.NoBody))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "DEMO_IN_USE") {
		t.Errorf("remove: status %d body %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	demoRouter(NewDemoHandler(&mockDemoService{}, &mockAuditService{}, true), false).
		ServeHTTP(rec, httptest.NewRequest("POST", "/demo/load", http.NoBody))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated: status %d", rec.Code)
	}
}
