package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
)

func TestErrorHandler_RendersDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/with", func(c *gin.Context) {
		_ = c.Error(apperrors.WithDetails(apperrors.ErrFXAmountRequired, map[string]any{"currency": "MYR", "field": "to_amount", "suggested_amount": int64(447120)}))
	})
	r.GET("/without", func(c *gin.Context) { _ = c.Error(apperrors.ErrNotFound) })

	type body struct {
		Error map[string]any `json:"error"`
	}
	get := func(path string) (int, body) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		var b body
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		return rec.Code, b
	}

	code, b := get("/with")
	d, _ := b.Error["details"].(map[string]any)
	if code != http.StatusBadRequest || b.Error["code"] != "FX_AMOUNT_REQUIRED" || d["currency"] != "MYR" || d["field"] != "to_amount" || d["suggested_amount"] != float64(447120) {
		t.Errorf("code=%d body=%v", code, b)
	}
	code, b = get("/without")
	if _, has := b.Error["details"]; code != http.StatusNotFound || has {
		t.Errorf("an error without details must not render the key: %d %v", code, b)
	}
}
