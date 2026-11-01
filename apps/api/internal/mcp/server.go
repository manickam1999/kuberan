package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"kuberan/internal/logger"
	"kuberan/internal/services"
)

// Services bundles all service interfaces the MCP server needs. Audit and
// Grants back the write tools (plan 022).
type Services struct {
	Users        services.UserServicer
	Accounts     services.AccountServicer
	Categories   services.CategoryServicer
	Transactions services.TransactionServicer
	Budgets      services.BudgetServicer
	Investments  services.InvestmentServicer
	Snapshots    services.PortfolioSnapshotServicer
	Receivables  services.ReceivableServicer
	Audit        services.AuditServicer
	Grants       GrantAuthorizer
}

// GrantAuthorizer is the one part of services.OAuthGrantServicer the MCP
// server needs: the per-write check that the user still grants the calling
// client a scope. Authorize also records the grant's last use (shown as
// "Last change" under Settings > Connected apps).
type GrantAuthorizer interface {
	Authorize(userID, clientID, scope string) error
}

type userIDKey struct{}

type scopesKey struct{}

type clientIDKey struct{}

type clientIPKey struct{}

// Server wraps the MCP server with Kuberan services.
type Server struct {
	services Services
	mcp      *server.MCPServer
	write    WriteConfig
	limiter  *rateLimiter
	// writeScopes maps each registered write tool to the scope it requires.
	writeScopes map[string]string
}

// getUserID extracts the authenticated user ID from the context.
func getUserID(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userIDKey{}).(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("unauthorized: missing user context")
	}
	return userID, nil
}

// scopesFromContext returns the OAuth scopes granted to the authenticated
// caller, as extracted from the validated access token.
func scopesFromContext(ctx context.Context) []string {
	scopes, _ := ctx.Value(scopesKey{}).([]string)
	return scopes
}

// requireScope enforces that the caller was granted at least one of the
// accepted scopes (Q6 granular read:* enforcement). It returns nil when
// authorized, or an MCP error result the handler should return directly.
// Handlers call it after getUserID.
func requireScope(ctx context.Context, accepted ...string) *mcpgo.CallToolResult {
	for _, granted := range scopesFromContext(ctx) {
		for _, a := range accepted {
			if granted == a {
				return nil
			}
		}
	}
	return mcpgo.NewToolResultError(
		"forbidden: missing required scope (" + strings.Join(accepted, " or ") + ")")
}

// claimsKey carries the validated access-token claims from requireBearer to
// makeAuthFromRequest on the request context.
type claimsKey struct{}

// makeAuthFromRequest returns an HTTP context function that stores the
// authenticated caller in the MCP request context: the subject (Kuberan user
// ID), granted scope set and OAuth client ID from the access token that
// requireBearer already validated, plus the caller's IP for audit logging.
// The token is validated once per request, in requireBearer; this function
// only reads the claims it attached. Without them it returns the context
// without a user, and tools reject the call via getUserID.
func makeAuthFromRequest(trustedProxies []*net.IPNet) server.HTTPContextFunc {
	return func(ctx context.Context, r *http.Request) context.Context {
		ctx = context.WithValue(ctx, clientIPKey{}, requestIP(r, trustedProxies))

		claims, ok := r.Context().Value(claimsKey{}).(*AccessClaims)
		if !ok || claims == nil {
			return ctx
		}
		ctx = context.WithValue(ctx, userIDKey{}, claims.Subject)
		ctx = context.WithValue(ctx, clientIDKey{}, claims.ClientID)
		return context.WithValue(ctx, scopesKey{}, claims.Scopes)
	}
}

// requestIP returns the caller's IP for audit logging: the TCP peer address,
// or Cloudflare's CF-Connecting-IP when the peer is a trusted proxy (the
// tunnel). Anyone else could set that header to forge the audited IP.
func requestIP(r *http.Request, trustedProxies []*net.IPNet) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	}
	forwarded := strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))
	if forwarded == "" || net.ParseIP(forwarded) == nil {
		return peer
	}
	if ip := net.ParseIP(peer); ip != nil {
		for _, n := range trustedProxies {
			if n.Contains(ip) {
				return forwarded
			}
		}
	}
	return peer
}

// ParseTrustedProxies parses CIDRs (config.MCPTrustedProxyCIDRs) for requestIP.
func ParseTrustedProxies(cidrs []string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", c, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// errUnauthorized returns an MCP tool error for unauthenticated requests.
func errUnauthorized() *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError("unauthorized: valid Bearer token required")
}

// requireBearer gates the MCP transport at the HTTP level: any request without
// a valid Hydra-issued Bearer access token is rejected with 401 (RFC 6750)
// before it reaches the MCP handler. This 401 is what triggers an MCP client's
// OAuth discovery via the WWW-Authenticate challenge added by
// withWWWAuthenticate; without it, clients would connect unauthenticated and
// only fail in-band at tool-call time.
//
// When active is set, the token's user must also still be active (checked
// against the database, cached briefly): access tokens are validated offline
// and a refresh token keeps minting them, so without this a deactivated user
// would keep read access for as long as the client keeps refreshing.
//
// The validated claims are attached to the request context for
// makeAuthFromRequest, so each request validates its token once.
func requireBearer(next http.Handler, v TokenValidator, active *activeUsers) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			writeUnauthorized(w)
			return
		}
		claims, err := v.Validate(r.Context(), parts[1])
		if err != nil {
			logger.Get().Warnf("MCP auth: token rejected: %v", err)
			writeUnauthorized(w)
			return
		}
		if active != nil {
			ok, err := active.isActive(claims.Subject)
			if err != nil {
				logger.Get().Errorf("MCP auth: user check failed: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"temporarily_unavailable"}`))
				return
			}
			if !ok {
				logger.Get().Warnw("MCP auth: token for inactive or deleted user", "user_id", claims.Subject)
				writeUnauthorized(w)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
	})
}

// writeUnauthorized emits the RFC 6750 401 response; the WWW-Authenticate
// challenge is injected by the enclosing withWWWAuthenticate wrapper.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","error_description":"valid Bearer access token required"}`))
}

// newServer creates the MCP server and registers its tools. Write tools are
// registered only when write.Enabled is set (plan 022).
func newServer(svc Services, write WriteConfig) *Server {
	s := &Server{
		services: svc,
		mcp: server.NewMCPServer(
			"kuberan",
			"1.0.0",
			server.WithToolCapabilities(true),
		),
		write:       write,
		limiter:     newRateLimiter(write.RatePerMinute, write.RatePerDay, time.Now),
		writeScopes: map[string]string{},
	}

	s.registerAccountTools()
	s.registerCategoryTools()
	s.registerTransactionTools()
	s.registerBudgetTools()
	s.registerInvestmentTools()
	s.registerSnapshotTools()
	s.registerReceivableTools()

	if write.Enabled {
		s.registerWriteTools()
	}
	return s
}

// Run creates the MCP server, registers all tools, and starts the HTTP transport.
// oauth carries the Resource Server discovery settings advertised at the
// RFC 9728 well-known endpoint and in WWW-Authenticate challenges. validator
// verifies incoming Bearer access tokens against Hydra's JWKS. write controls
// the write tools and their limits. trustedProxies are the networks whose
// CF-Connecting-IP header is believed for audit logging.
func Run(svc Services, addr string, oauth OAuthConfig, validator TokenValidator, write WriteConfig, trustedProxies []*net.IPNet) error {
	s := newServer(svc, write)

	mcpHandler := server.NewStreamableHTTPServer(s.mcp,
		server.WithHTTPContextFunc(makeAuthFromRequest(trustedProxies)),
	)
	var active *activeUsers
	if svc.Users != nil {
		active = newActiveUsers(svc.Users, activeUserTTL, time.Now)
	}

	// Wrap the MCP handler in our own mux so we can expose an unauthenticated
	// /health endpoint for container/orchestrator health checks alongside the
	// authenticated /mcp transport endpoint.
	mux := http.NewServeMux()
	// Gate /mcp behind Bearer validation (401 without a valid token) and wrap it
	// so those 401s carry a WWW-Authenticate challenge pointing at the Protected
	// Resource Metadata document (RFC 9728); serve that document so MCP clients
	// can discover the Hydra authorization server.
	mux.Handle("/mcp", withWWWAuthenticate(requireBearer(mcpHandler, validator, active), oauth))
	mux.Handle(ProtectedResourceMetadataPath, discoveryHandler(oauth))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start server in a goroutine so we can listen for shutdown signals.
	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Get().Infof("Received signal %s, shutting down MCP server", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Cancel the MCP session sweeper, then drain in-flight HTTP requests.
		_ = mcpHandler.Shutdown(ctx)
		return httpServer.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}
