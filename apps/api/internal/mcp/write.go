package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	apperrors "kuberan/internal/errors"
	"kuberan/internal/logger"
	"kuberan/internal/services"
)

// WriteConfig controls the MCP write tools (plan 022).
type WriteConfig struct {
	// Enabled registers the write tools. When false the server is read-only.
	Enabled bool
	// MaxAmount is the per-change ceiling for any amount, in cents.
	MaxAmount int64
	// RatePerMinute and RatePerDay cap writes per (user, client); 0 disables
	// that window. Each user is also capped across all their clients at
	// userRateMultiplier times these values.
	RatePerMinute int
	RatePerDay    int
}

const (
	// maxFutureDays bounds how far ahead a write may date a record. One extra
	// day of slack is allowed because dates are the user's local calendar date
	// while the server compares in UTC.
	maxFutureDays = 7
	// maxPastYears bounds how far back a write may date a record.
	maxPastYears = 10
)

// writeNotice ends every write tool description. Clients surface tool
// descriptions to the model; this steers it to confirm with the user and to
// check before retrying, which matters because a create is not idempotent.
const writeNotice = " This changes the user's financial records: confirm the details with the user before calling. " +
	"If a call may have failed, check with list_transactions before retrying."

// caller identifies who is making an MCP write, for the grant check, the rate
// limit and the audit log.
type caller struct {
	UserID   string
	ClientID string
	IP       string
}

// auditEntry is one audit_logs row produced by a write.
type auditEntry struct {
	Action       string
	ResourceType string
	ResourceID   string
	Changes      map[string]interface{}
}

// writeOp is a validated write, ready to run.
type writeOp struct {
	// Cost is how many writes this counts against the rate limit (the number of
	// records it will change; 0 when there is nothing to do).
	Cost int
	// Run performs the write. It returns the tool's output text, the audit rows
	// to record, and an error result if the write failed. Audit rows are
	// recorded even alongside an error result, for partially applied batches.
	Run func() (text string, audits []auditEntry, res *mcp.CallToolResult)
}

// prepareFunc validates a write tool's arguments for an authorized caller and
// returns the write to run, or an error result. It may read the database (to
// check ownership or capture before-values) but must not change anything.
type prepareFunc func(c caller, args map[string]interface{}) (*writeOp, *mcp.CallToolResult)

// addWriteTool registers a write tool. Every write tool goes through the same
// sequence, so none can skip a step:
//
//  1. authorizeWrite: the token scope and the user's consent grant;
//  2. prepare: argument validation, with no side effects;
//  3. the rate limit, charged only for a valid write, by its cost;
//  4. the write itself, then its audit rows, tagged with the tool and client.
func (s *Server) addWriteTool(tool mcp.Tool, scope string, prepare prepareFunc) {
	s.writeScopes[tool.Name] = scope
	s.mcp.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, denied := s.authorizeWrite(ctx, scope)
		if denied != nil {
			return denied, nil
		}
		op, res := prepare(c, req.GetArguments())
		if res != nil {
			return res, nil
		}
		if res := s.chargeWrites(c, op.Cost); res != nil {
			return res, nil
		}
		text, audits, res := op.Run()
		for _, a := range audits {
			s.audit(c, tool.Name, a)
		}
		if res != nil {
			return res, nil
		}
		return mcp.NewToolResultText(text), nil
	})
}

// authorizeWrite resolves the caller and enforces the token scope and the
// user's consent grant in the database (not revoked, scope approved, user
// active). It returns the caller, or an MCP error result to return directly.
//
// The grant check is what makes revocation immediate for writes: access
// tokens are validated offline and stay valid for up to 15 minutes after the
// user disconnects the client in Settings.
func (s *Server) authorizeWrite(ctx context.Context, scope string) (caller, *mcp.CallToolResult) {
	userID, err := getUserID(ctx)
	if err != nil {
		return caller{}, errUnauthorized()
	}
	if denied := requireScope(ctx, scope); denied != nil {
		return caller{}, denied
	}

	c := caller{UserID: userID}
	c.ClientID, _ = ctx.Value(clientIDKey{}).(string)
	c.IP, _ = ctx.Value(clientIPKey{}).(string)
	if c.ClientID == "" {
		return caller{}, mcp.NewToolResultError("forbidden: access token has no client_id")
	}

	if err := s.services.Grants.Authorize(userID, c.ClientID, scope); err != nil {
		if errors.Is(err, apperrors.ErrOAuthScopeNotGranted) {
			return caller{}, mcp.NewToolResultError(fmt.Sprintf(
				"forbidden: this connection is not allowed to make changes (%s). "+
					"Reconnect and allow changes, or check Settings > Connected apps in Kuberan.", scope))
		}
		logger.Get().Errorf("MCP write: grant check failed: %v", err)
		return caller{}, mcp.NewToolResultError("failed to verify access")
	}
	return c, nil
}

// chargeWrites counts n writes against the caller's rate limit.
func (s *Server) chargeWrites(c caller, n int) *mcp.CallToolResult {
	if n <= 0 {
		return nil
	}
	if s.write.RatePerMinute > 0 && n > s.write.RatePerMinute {
		return mcp.NewToolResultError(fmt.Sprintf(
			"this change touches %d records, more than the limit of %d per minute; split it into smaller batches",
			n, s.write.RatePerMinute))
	}
	if ok, retry := s.limiter.allowCaller(c.UserID, c.ClientID, n); !ok {
		return mcp.NewToolResultError(fmt.Sprintf(
			"rate limited: too many changes, try again in %ds", int(math.Ceil(retry.Seconds()))))
	}
	return nil
}

// audit records an MCP write in audit_logs, tagged with the OAuth client and
// tool so every change can be traced to the connection that made it.
func (s *Server) audit(c caller, tool string, a auditEntry) {
	changes := make(map[string]interface{}, len(a.Changes)+3)
	for k, v := range a.Changes {
		changes[k] = v
	}
	changes["via"] = "mcp"
	changes["client_id"] = c.ClientID
	changes["tool"] = tool
	s.services.Audit.Log(c.UserID, a.Action, a.ResourceType, a.ResourceID, c.IP, changes)
}

// change records a field's before and after values in an audit entry.
func change(from, to interface{}) map[string]interface{} {
	return map[string]interface{}{"from": from, "to": to}
}

// serviceErrorMessage returns a client-safe description of a service error.
// AppError messages are written for clients (the REST API returns them as-is);
// anything else is logged and reported generically.
func serviceErrorMessage(err error) string {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) && appErr.StatusCode < 500 {
		return appErr.Message
	}
	logger.Get().Errorf("MCP write failed: %v", err)
	return "internal error"
}

// serviceError turns a service error into a tool error for action.
func serviceError(action string, err error) *mcp.CallToolResult {
	return mcp.NewToolResultError("failed to " + action + ": " + serviceErrorMessage(err))
}

// writeTool returns the options shared by every write tool: the description
// with the confirmation notice, and annotations telling clients the tool
// changes data. destructive marks tools that overwrite existing values.
func writeTool(name, description string, destructive bool, opts ...mcp.ToolOption) mcp.Tool {
	all := append([]mcp.ToolOption{
		mcp.WithDescription(description + writeNotice),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(destructive),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	}, opts...)
	return mcp.NewTool(name, all...)
}

// readTool returns a tool annotated as read-only, so clients can tell reads
// from writes and skip confirmation for them.
func readTool(name string, opts ...mcp.ToolOption) mcp.Tool {
	all := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	}, opts...)
	return mcp.NewTool(name, all...)
}

// stringArg returns a trimmed string argument, or "" when absent.
func stringArg(args map[string]interface{}, name string) string {
	v, _ := args[name].(string)
	return strings.TrimSpace(v)
}

// requiredString returns a non-empty string argument or an error result.
func requiredString(args map[string]interface{}, name string) (string, *mcp.CallToolResult) {
	v := stringArg(args, name)
	if v == "" {
		return "", mcp.NewToolResultError(name + " is required")
	}
	return v, nil
}

// optionalString returns a pointer to a string argument when it was supplied
// (even if empty, which callers use to clear a value), or nil.
func optionalString(args map[string]interface{}, name string) *string {
	v, ok := args[name].(string)
	if !ok {
		return nil
	}
	v = strings.TrimSpace(v)
	return &v
}

// checkLength rejects a value longer than limit characters, using the
// services' rule (services.CheckTextLength) so MCP and REST cannot drift. It
// runs before the write so invalid input never counts against the rate limit;
// the service enforces the same limit again.
func checkLength(field, v string, limit int) *mcp.CallToolResult {
	if err := services.CheckTextLength(field, v, limit); err != nil {
		return mcp.NewToolResultError(serviceErrorMessage(err))
	}
	return nil
}

// descriptionArg returns the optional description argument ("" when absent).
func descriptionArg(args map[string]interface{}) (string, *mcp.CallToolResult) {
	v := stringArg(args, "description")
	return v, checkLength("description", v, services.MaxDescriptionLen)
}

// optionalDescriptionArg returns the description argument when supplied (even
// empty, which clears it), or nil.
func optionalDescriptionArg(args map[string]interface{}) (*string, *mcp.CallToolResult) {
	v := optionalString(args, "description")
	if v == nil {
		return nil, nil
	}
	return v, checkLength("description", *v, services.MaxDescriptionLen)
}

// checkCeiling rejects an amount (in cents) above the per-change limit. It
// applies to every amount a write moves, including ones the caller did not
// pass explicitly (a full write-off, a transaction whose type is flipped).
func (s *Server) checkCeiling(what string, cents int64) *mcp.CallToolResult {
	if s.write.MaxAmount > 0 && cents > s.write.MaxAmount {
		return mcp.NewToolResultError(fmt.Sprintf(
			"%s of %s exceeds the per-change limit of %s; make this change in the Kuberan app instead",
			what, formatCents(cents), formatCents(s.write.MaxAmount)))
	}
	return nil
}

// amountArg reads the "amount" argument (dollars) and returns it in cents. It
// must be positive and at most the configured ceiling. present is false when
// the argument was omitted.
func (s *Server) amountArg(args map[string]interface{}) (cents int64, present bool, res *mcp.CallToolResult) {
	raw, ok := args["amount"]
	if !ok || raw == nil {
		return 0, false, nil
	}
	dollars, ok := raw.(float64)
	if !ok || math.IsNaN(dollars) || math.IsInf(dollars, 0) {
		return 0, true, mcp.NewToolResultError("amount must be a number of dollars")
	}
	cents = dollarsToCents(dollars)
	if cents <= 0 {
		return 0, true, mcp.NewToolResultError("amount must be greater than zero")
	}
	return cents, true, s.checkCeiling("amount", cents)
}

// requiredAmount reads the required "amount" argument (dollars), in cents.
func (s *Server) requiredAmount(args map[string]interface{}) (int64, *mcp.CallToolResult) {
	cents, present, res := s.amountArg(args)
	if res != nil {
		return 0, res
	}
	if !present {
		return 0, mcp.NewToolResultError("amount is required")
	}
	return cents, nil
}

// dateArgDescription tells the model which date to send. The server cannot
// know the user's time zone, so "today" must come from the client.
const dateArgDescription = "Date in the user's own time zone (YYYY-MM-DD)"

// dateArg reads the optional "date" argument (YYYY-MM-DD) as UTC midnight, the
// same instant the web app stores for a date. It must be within maxPastYears
// back and maxFutureDays ahead. present is false when the argument was omitted.
func dateArg(args map[string]interface{}, now time.Time) (t time.Time, present bool, res *mcp.CallToolResult) {
	v := stringArg(args, "date")
	if v == "" {
		return time.Time{}, false, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, true, mcp.NewToolResultError("date must be in YYYY-MM-DD format")
	}
	today := now.UTC().Truncate(24 * time.Hour)
	if t.After(today.AddDate(0, 0, maxFutureDays+1)) {
		return time.Time{}, true, mcp.NewToolResultError(fmt.Sprintf("date cannot be more than %d days in the future", maxFutureDays))
	}
	if t.Before(today.AddDate(-maxPastYears, 0, 0)) {
		return time.Time{}, true, mcp.NewToolResultError(fmt.Sprintf("date cannot be more than %d years in the past", maxPastYears))
	}
	return t, true, nil
}

// requiredDate reads the required "date" argument. Creates require a date
// because the server's clock is UTC: defaulting to "now" would put an evening
// expense in a UTC-8 time zone on the next day, possibly in the next month.
func requiredDate(args map[string]interface{}, now time.Time) (time.Time, *mcp.CallToolResult) {
	t, present, res := dateArg(args, now)
	if res != nil {
		return time.Time{}, res
	}
	if !present {
		return time.Time{}, mcp.NewToolResultError("date is required (the user's local date, YYYY-MM-DD)")
	}
	return t, nil
}

// stringListArg reads a non-empty array of distinct, non-empty strings of at
// most maxLen items.
func stringListArg(args map[string]interface{}, name string, maxLen int) ([]string, *mcp.CallToolResult) {
	raw, ok := args[name].([]interface{})
	if !ok || len(raw) == 0 {
		return nil, mcp.NewToolResultError(name + " is required (a non-empty list of IDs)")
	}
	if len(raw) > maxLen {
		return nil, mcp.NewToolResultError(fmt.Sprintf("%s can have at most %d items", name, maxLen))
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		v, ok := item.(string)
		v = strings.TrimSpace(v)
		if !ok || v == "" {
			return nil, mcp.NewToolResultError(name + " must contain only non-empty IDs")
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out, nil
}

// userRateMultiplier sets the per-user write limit, across all of a user's
// clients, as a multiple of the per-(user, client) limit: one connection
// cannot exceed its own limit, and several connections (for example a burst
// of freshly registered clients a phished user approved) together cannot
// exceed this multiple.
const userRateMultiplier = 2

// rateSweepInterval is how often idle buckets are evicted.
const rateSweepInterval = 10 * time.Minute

// rateLimit is a pair of fixed-window limits; 0 disables a window.
type rateLimit struct {
	perMinute int
	perDay    int
}

// rateLimiter is a fixed-window counter per key, with a one-minute and a
// one-day window. State is held in memory, per MCP server instance: the
// server runs as a single instance, and a restart resets the windows. Buckets
// idle for a full day are evicted.
type rateLimiter struct {
	mu        sync.Mutex
	client    rateLimit // per (user, client)
	user      rateLimit // per user, across clients
	now       func() time.Time
	buckets   map[string]*rateBucket
	lastSweep time.Time
}

type rateBucket struct {
	minuteStart time.Time
	minuteCount int
	dayStart    time.Time
	dayCount    int
}

// newRateLimiter returns a limiter allowing perMinute/perDay events per
// (user, client) and userRateMultiplier times that per user.
func newRateLimiter(perMinute, perDay int, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		client:    rateLimit{perMinute: perMinute, perDay: perDay},
		user:      rateLimit{perMinute: perMinute * userRateMultiplier, perDay: perDay * userRateMultiplier},
		now:       now,
		buckets:   map[string]*rateBucket{},
		lastSweep: now(),
	}
}

// allowCaller counts n writes for a user and client against both the
// per-(user, client) and the per-user limits. Either refusing refuses both,
// and nothing is counted.
func (l *rateLimiter) allowCaller(userID, clientID string, n int) (ok bool, retry time.Duration) {
	return l.allowKeys(n,
		limitedKey{key: "c|" + userID + "|" + clientID, limit: l.client},
		limitedKey{key: "u|" + userID, limit: l.user},
	)
}

// allow counts n events for key against the per-(user, client) limit.
func (l *rateLimiter) allow(key string, n int) (ok bool, retry time.Duration) {
	return l.allowKeys(n, limitedKey{key: key, limit: l.client})
}

// limitedKey is a bucket key and the limit that applies to it.
type limitedKey struct {
	key   string
	limit rateLimit
}

// allowKeys counts n events against every key and reports whether they fit in
// all of their windows. When they do not, nothing is counted and retry is how
// long until the blocking window resets.
func (l *rateLimiter) allowKeys(n int, keys ...limitedKey) (ok bool, retry time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	buckets := make([]*rateBucket, len(keys))
	for i, k := range keys {
		b := l.buckets[k.key]
		if b == nil {
			b = &rateBucket{minuteStart: now, dayStart: now}
			l.buckets[k.key] = b
		}
		if now.Sub(b.minuteStart) >= time.Minute {
			b.minuteStart, b.minuteCount = now, 0
		}
		if now.Sub(b.dayStart) >= 24*time.Hour {
			b.dayStart, b.dayCount = now, 0
		}
		if k.limit.perDay > 0 && b.dayCount+n > k.limit.perDay {
			return false, b.dayStart.Add(24 * time.Hour).Sub(now)
		}
		if k.limit.perMinute > 0 && b.minuteCount+n > k.limit.perMinute {
			return false, b.minuteStart.Add(time.Minute).Sub(now)
		}
		buckets[i] = b
	}
	for _, b := range buckets {
		b.minuteCount += n
		b.dayCount += n
	}
	return true, 0
}

// sweep evicts buckets whose day window has expired (both windows are then
// empty, so dropping one is the same as keeping it). It runs at most every
// rateSweepInterval. The caller holds l.mu.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < rateSweepInterval {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.dayStart) >= 24*time.Hour {
			delete(l.buckets, k)
		}
	}
}
