package middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiter_Allow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewRateLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, retry := l.Allow("k")
	if ok {
		t.Fatal("third request should be limited")
	}
	if retry != time.Minute {
		t.Errorf("retryAfter = %v, want 1m", retry)
	}
	if ok, _ := l.Allow("other"); !ok {
		t.Error("keys are independent")
	}

	now = now.Add(time.Minute)
	if ok, _ := l.Allow("k"); !ok {
		t.Error("window should reset after it elapses")
	}
}

func TestRateLimiter_DisabledAndBounded(t *testing.T) {
	if ok, _ := NewRateLimiter(0, time.Minute).Allow("k"); !ok {
		t.Error("limit 0 disables the limiter")
	}
	var nilLimiter *RateLimiter
	if ok, _ := nilLimiter.Allow("k"); !ok {
		t.Error("nil limiter allows")
	}

	now := time.Unix(1_700_000_000, 0)
	l := NewRateLimiter(1, time.Minute)
	l.now = func() time.Time { return now }
	l.maxKeys = 2
	l.Allow("a")
	l.Allow("b")
	if ok, _ := l.Allow("c"); !ok {
		t.Error("an untracked key past the cap is allowed")
	}
	if len(l.windows) != 2 {
		t.Errorf("tracked keys = %d, want capped at 2", len(l.windows))
	}

	// Expired windows are swept.
	now = now.Add(2 * time.Minute)
	l.Allow("d")
	if len(l.windows) != 1 {
		t.Errorf("expired keys not swept: %d tracked", len(l.windows))
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	limiter := NewRateLimiter(1, time.Hour)
	r.POST("/login", RateLimit(limiter, ClientIPKey), func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
		req.RemoteAddr = "198.51.100.7:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(); rec.Code != http.StatusOK {
		t.Fatalf("first request: %d", rec.Code)
	}
	rec := do()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "3600" {
		t.Errorf("Retry-After = %q, want 3600", rec.Header().Get("Retry-After"))
	}
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code != "RATE_LIMITED" {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestEmailBodyKey_PreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	limiter := NewRateLimiter(1, time.Hour)
	var seen string
	r.POST("/login", RateLimit(limiter, EmailBodyKey), func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		seen = string(b)
		c.Status(http.StatusOK)
	})

	do := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	body := `{"email":"Victim@Example.com","password":"x"}`
	if code := do(body); code != http.StatusOK {
		t.Fatalf("first: %d", code)
	}
	if seen != body {
		t.Errorf("handler saw %q, want the original body", seen)
	}
	if code := do(`{"email":" victim@example.com ","password":"y"}`); code != http.StatusTooManyRequests {
		t.Errorf("same email (normalised) should be limited, got %d", code)
	}
	if code := do(`not json`); code != http.StatusOK {
		t.Errorf("unparseable body gets no key, got %d", code)
	}
}
