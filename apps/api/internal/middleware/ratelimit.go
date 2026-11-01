package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "kuberan/internal/errors"
)

// defaultMaxRateLimitKeys bounds the number of keys a RateLimiter tracks, so a
// flood of distinct keys (spoofed emails, many IPs) cannot grow memory without
// limit. Past the cap, untracked keys are allowed: the limiter degrades to
// "no limit" for them rather than to "deny everyone".
const defaultMaxRateLimitKeys = 100_000

// rateWindow is one key's fixed-window counter.
type rateWindow struct {
	start time.Time
	count int
}

// RateLimiter is an in-memory fixed-window rate limiter keyed by an arbitrary
// string (client IP, email, ...). It allows at most limit events per key per
// window. State is per process: with several API replicas each enforces its own
// budget. A nil RateLimiter, or one with limit <= 0, allows everything.
type RateLimiter struct {
	limit   int
	window  time.Duration
	maxKeys int
	now     func() time.Time

	mu        sync.Mutex
	windows   map[string]*rateWindow
	lastSweep time.Time
}

// NewRateLimiter returns a limiter allowing limit events per key per window.
// limit <= 0 disables it.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		maxKeys: defaultMaxRateLimitKeys,
		now:     time.Now,
		windows: make(map[string]*rateWindow),
	}
}

// Allow records one event for key and reports whether it is within the limit.
// When it is not, retryAfter is how long until the key's window resets.
func (l *RateLimiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return true, 0
	}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) >= l.window {
		l.sweep(now)
	}

	w, ok := l.windows[key]
	if !ok || now.Sub(w.start) >= l.window {
		if !ok && len(l.windows) >= l.maxKeys {
			l.sweep(now)
			if len(l.windows) >= l.maxKeys {
				return true, 0
			}
		}
		l.windows[key] = &rateWindow{start: now, count: 1}
		return true, 0
	}

	if w.count >= l.limit {
		return false, w.start.Add(l.window).Sub(now)
	}
	w.count++
	return true, 0
}

// Exhausted reports, without recording an event, whether key has used up its
// budget for the current window. Callers that only count failures use it to
// refuse an attempt up front and call Allow when one fails.
func (l *RateLimiter) Exhausted(key string) bool {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return false
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.windows[key]
	return ok && now.Sub(w.start) < l.window && w.count >= l.limit
}

// sweep drops expired windows. Callers hold l.mu.
func (l *RateLimiter) sweep(now time.Time) {
	for k, w := range l.windows {
		if now.Sub(w.start) >= l.window {
			delete(l.windows, k)
		}
	}
	l.lastSweep = now
}

// RateLimitKeyFunc derives the rate-limit key for a request. An empty key
// skips the limiter for that request.
type RateLimitKeyFunc func(c *gin.Context) string

// ClientIPKey keys requests by Gin's ClientIP (which honours only the trusted
// proxies configured on the engine).
func ClientIPKey(c *gin.Context) string {
	return "ip:" + c.ClientIP()
}

// maxRateLimitBodyPeek bounds how much of a request body EmailBodyKey reads.
const maxRateLimitBodyPeek = 64 << 10

// EmailBodyKey keys requests by the normalised "email" field of their JSON
// body, leaving the body intact for the handler. Requests without a readable
// email get no key (the per-IP limiter still applies).
func EmailBodyKey(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxRateLimitBodyPeek+1))
	// Put back what was read, followed by anything left unread, so the handler
	// (and any body-size limit) sees the original stream.
	c.Request.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(data), c.Request.Body), c.Request.Body}
	if err != nil || len(data) > maxRateLimitBodyPeek {
		return ""
	}
	var body struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(data, &body) != nil {
		return ""
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" {
		return ""
	}
	if len(email) > 254 {
		email = email[:254]
	}
	return "email:" + email
}

// RateLimit returns middleware that rejects requests over limiter's budget for
// their key with 429 RATE_LIMITED and a Retry-After header (seconds).
func RateLimit(limiter *RateLimiter, key RateLimitKeyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := key(c)
		if k == "" {
			c.Next()
			return
		}
		if ok, retryAfter := limiter.Allow(k); !ok {
			secs := int(math.Ceil(retryAfter.Seconds()))
			if secs < 1 {
				secs = 1
			}
			c.Header("Retry-After", strconv.Itoa(secs))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    apperrors.ErrRateLimited.Code,
					"message": apperrors.ErrRateLimited.Message,
				},
			})
			return
		}
		c.Next()
	}
}
