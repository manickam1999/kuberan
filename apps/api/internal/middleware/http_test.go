package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodySizeLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodySizeLimit(16, "POST /upload/:id"))
	read := func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusOK)
	}
	r.POST("/json", read)
	r.POST("/upload/:id", read)

	do := func(path string, body string, chunked bool) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if chunked {
			req.ContentLength = -1
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	big := strings.Repeat("x", 32)
	if code := do("/json", "small", false); code != http.StatusOK {
		t.Errorf("small body: %d", code)
	}
	if code := do("/json", big, false); code != http.StatusRequestEntityTooLarge {
		t.Errorf("declared oversize body: expected 413, got %d", code)
	}
	if code := do("/json", big, true); code != http.StatusBadRequest {
		t.Errorf("undeclared oversize body: expected read failure, got %d", code)
	}
	if code := do("/upload/1", big, false); code != http.StatusOK {
		t.Errorf("exempt route: expected 200, got %d", code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, hsts := range []bool{false, true} {
		r := gin.New()
		r.Use(SecurityHeaders(hsts))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
		r.GET("/download", func(c *gin.Context) {
			c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
			c.Status(http.StatusOK)
		})

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" ||
			h.Get("X-Frame-Options") != "DENY" || h.Get("Content-Security-Policy") != "frame-ancestors 'none'" {
			t.Errorf("missing headers: %v", h)
		}
		if (h.Get("Strict-Transport-Security") != "") != hsts {
			t.Errorf("hsts=%v but Strict-Transport-Security=%q", hsts, h.Get("Strict-Transport-Security"))
		}

		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download", http.NoBody))
		if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
			t.Errorf("handler CSP was clobbered: %q", got)
		}
	}
}

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS([]string{"https://app.example", "http://localhost:3000"}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func(method, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/x", http.NoBody)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodGet, "https://app.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed origin: %v", rec.Header())
	}
	if !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Origin") {
		t.Error("expected Vary: Origin")
	}

	rec = do(http.MethodGet, "https://evil.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("disallowed origin got CORS headers: %v", rec.Header())
	}

	if rec = do(http.MethodOptions, "http://localhost:3000"); rec.Code != http.StatusNoContent ||
		rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("preflight: %d %v", rec.Code, rec.Header())
	}
}

func TestConfigureTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clientIP := func(cidrs []string, peer, cf string) string {
		r := gin.New()
		if err := ConfigureTrustedProxies(r, cidrs); err != nil {
			t.Fatalf("ConfigureTrustedProxies: %v", err)
		}
		var ip string
		r.GET("/", func(c *gin.Context) { ip = c.ClientIP() })
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = peer
		req.Header.Set("CF-Connecting-IP", cf)
		req.Header.Set("X-Forwarded-For", "203.0.113.99")
		r.ServeHTTP(httptest.NewRecorder(), req)
		return ip
	}

	if got := clientIP(nil, "198.51.100.1:1", "203.0.113.5"); got != "198.51.100.1" {
		t.Errorf("no trusted proxies: ClientIP = %s, want peer", got)
	}
	if got := clientIP([]string{"172.16.0.0/12"}, "198.51.100.1:1", "203.0.113.5"); got != "198.51.100.1" {
		t.Errorf("untrusted peer: ClientIP = %s, want peer", got)
	}
	if got := clientIP([]string{"172.16.0.0/12"}, "172.18.0.4:1", "203.0.113.5"); got != "203.0.113.5" {
		t.Errorf("trusted peer: ClientIP = %s, want CF-Connecting-IP", got)
	}
	if err := ConfigureTrustedProxies(gin.New(), []string{"not-a-cidr"}); err == nil {
		t.Error("expected an error for an invalid CIDR")
	}
}

func TestGzip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Gzip())
	large := strings.Repeat(`{"k":"value"},`, 200)
	r.GET("/json", func(c *gin.Context) { c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(large)) })
	r.GET("/small", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/image", func(c *gin.Context) { c.Data(http.StatusOK, "image/png", []byte(large)) })

	do := func(path string, accept bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		if accept {
			req.Header.Set("Accept-Encoding", "gzip, deflate")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	rec := do("/json", true)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip, headers %v", rec.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if string(plain) != large {
		t.Error("decompressed body differs")
	}

	for _, tc := range []struct {
		path   string
		accept bool
	}{{"/json", false}, {"/small", true}, {"/image", true}} {
		if rec := do(tc.path, tc.accept); rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s (accept=%v) should not be compressed", tc.path, tc.accept)
		}
	}
}
