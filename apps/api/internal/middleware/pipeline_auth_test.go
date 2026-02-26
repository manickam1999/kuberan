package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupPipelineRouter(apiKey string) *gin.Engine {
	r := gin.New()
	r.Use(PipelineAuthMiddleware(apiKey))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r
}

func doRequest(r *gin.Engine, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func parseBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	return result
}

func TestPipelineAuthMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		configuredKey string
		requestKey    string
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:          "valid_api_key",
			configuredKey: "secret-pipeline-key",
			requestKey:    "secret-pipeline-key",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "invalid_api_key",
			configuredKey: "secret-pipeline-key",
			requestKey:    "wrong-key",
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "INVALID_API_KEY",
		},
		{
			name:          "missing_api_key",
			configuredKey: "secret-pipeline-key",
			requestKey:    "",
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "INVALID_API_KEY",
		},
		{
			name:          "empty_configured_key",
			configuredKey: "",
			requestKey:    "any-key",
			wantStatus:    http.StatusServiceUnavailable,
			wantErrorCode: "PIPELINE_NOT_CONFIGURED",
		},
		{
			name:          "both_empty",
			configuredKey: "",
			requestKey:    "",
			wantStatus:    http.StatusServiceUnavailable,
			wantErrorCode: "PIPELINE_NOT_CONFIGURED",
		},
		{
			name:          "partial_match_rejected",
			configuredKey: "secret-pipeline-key",
			requestKey:    "secret-pipeline",
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "INVALID_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupPipelineRouter(tt.configuredKey)
			rec := doRequest(router, tt.requestKey)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				body := parseBody(t, rec)
				errObj, ok := body["error"].(map[string]interface{})
				if !ok {
					t.Fatal("expected error object in response")
				}
				if code, _ := errObj["code"].(string); code != tt.wantErrorCode {
					t.Errorf("error code = %q, want %q", code, tt.wantErrorCode)
				}
			}

			if tt.wantStatus == http.StatusOK {
				body := parseBody(t, rec)
				if status, _ := body["status"].(string); status != "ok" {
					t.Errorf("expected handler to be reached, got status = %q", status)
				}
			}
		})
	}
}

func setupInternalRouter(secret string) *gin.Engine {
	r := gin.New()
	r.Use(InternalAuthMiddleware(secret))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r
}

func doInternalRequest(r *gin.Engine, secret string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	if secret != "" {
		req.Header.Set("X-Internal-Secret", secret)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestInternalAuthMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		configuredKey string
		requestKey    string
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:          "valid_secret",
			configuredKey: "test-secret",
			requestKey:    "test-secret",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "invalid_secret",
			configuredKey: "test-secret",
			requestKey:    "wrong-secret",
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "INVALID_INTERNAL_SECRET",
		},
		{
			name:          "missing_secret",
			configuredKey: "test-secret",
			requestKey:    "",
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "INVALID_INTERNAL_SECRET",
		},
		{
			name:          "empty_configured_secret",
			configuredKey: "",
			requestKey:    "any",
			wantStatus:    http.StatusServiceUnavailable,
			wantErrorCode: "INTERNAL_AUTH_NOT_CONFIGURED",
		},
		{
			name:          "both_empty",
			configuredKey: "",
			requestKey:    "",
			wantStatus:    http.StatusServiceUnavailable,
			wantErrorCode: "INTERNAL_AUTH_NOT_CONFIGURED",
		},
		{
			name:          "partial_match_rejected",
			configuredKey: "test-secret",
			requestKey:    "test-secre",
			wantStatus:    http.StatusUnauthorized,
			wantErrorCode: "INVALID_INTERNAL_SECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupInternalRouter(tt.configuredKey)
			rec := doInternalRequest(router, tt.requestKey)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantErrorCode != "" {
				body := parseBody(t, rec)
				errObj, ok := body["error"].(map[string]interface{})
				if !ok {
					t.Fatal("expected error object in response")
				}
				if code, _ := errObj["code"].(string); code != tt.wantErrorCode {
					t.Errorf("error code = %q, want %q", code, tt.wantErrorCode)
				}
			}

			if tt.wantStatus == http.StatusOK {
				body := parseBody(t, rec)
				if status, _ := body["status"].(string); status != "ok" {
					t.Errorf("expected handler to be reached, got status = %q", status)
				}
			}
		})
	}
}
