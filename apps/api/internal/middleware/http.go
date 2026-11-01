package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
)

// DefaultMaxBodyBytes is the default request-body cap for API routes.
const DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

// BodySizeLimit caps request bodies at maxBytes. Requests that declare a
// larger Content-Length are rejected up front with 413; others get a
// http.MaxBytesReader so binding fails once the cap is crossed. Routes listed
// in exempt (as "METHOD /full/path" using Gin's route pattern, e.g.
// "POST /api/v1/transactions/:id/attachments") enforce their own, larger cap.
func BodySizeLimit(maxBytes int64, exempt ...string) gin.HandlerFunc {
	skip := make(map[string]bool, len(exempt))
	for _, e := range exempt {
		skip[e] = true
	}
	return func(c *gin.Context) {
		if maxBytes <= 0 || c.Request.Body == nil || skip[c.Request.Method+" "+c.FullPath()] {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": gin.H{
					"code":    apperrors.ErrBodyTooLarge.Code,
					"message": apperrors.ErrBodyTooLarge.Message,
				},
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// SecurityHeaders sets defensive response headers on every API response.
// HSTS is only sent when hsts is true (i.e. outside development, where the API
// is served over HTTPS). Handlers may override Content-Security-Policy (the
// attachment download sets its own sandboxing policy); the frame-ancestors
// default here only applies when they do not.
func SecurityHeaders(hsts bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		if hsts {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		c.Next()
	}
}

// CORS returns middleware that allows cross-origin, credentialed requests only
// from the exact origins in allowed. A matching Origin is echoed back (never
// "*", which browsers refuse with credentials); other origins get no CORS
// headers, so the browser blocks them. Preflight requests are answered with
// 204 either way.
func CORS(allowed []string) gin.HandlerFunc {
	origins := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		origins[strings.TrimRight(o, "/")] = true
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Add("Vary", "Origin")
		if origin := c.GetHeader("Origin"); origin != "" && origins[origin] {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			h.Set("Access-Control-Max-Age", "600")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ConfigureTrustedProxies makes c.ClientIP() trustworthy: forwarding headers
// (CF-Connecting-IP first, then X-Forwarded-For / X-Real-IP) are believed only
// when the TCP peer is inside one of cidrs. With no CIDRs no proxy is trusted
// and ClientIP is the peer address, so a client cannot spoof its IP for rate
// limiting or audit logs. This mirrors the MCP server's requestIP.
func ConfigureTrustedProxies(router *gin.Engine, cidrs []string) error {
	for _, c := range cidrs {
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q: %w", c, err)
		}
	}
	router.ForwardedByClientIP = true
	router.RemoteIPHeaders = []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"}
	router.TrustedPlatform = ""
	if len(cidrs) == 0 {
		return router.SetTrustedProxies(nil)
	}
	return router.SetTrustedProxies(cidrs)
}
