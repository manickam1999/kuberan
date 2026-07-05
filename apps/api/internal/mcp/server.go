package mcp

import (
	"context"
	"errors"
	"fmt"
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

// Services bundles all service interfaces the MCP server needs.
type Services struct {
	Users        services.UserServicer
	Accounts     services.AccountServicer
	Categories   services.CategoryServicer
	Transactions services.TransactionServicer
	Budgets      services.BudgetServicer
	Investments  services.InvestmentServicer
	Snapshots    services.PortfolioSnapshotServicer
}

type userIDKey struct{}

type scopesKey struct{}

// Server wraps the MCP server with Kuberan services.
type Server struct {
	services Services
	mcp      *server.MCPServer
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

// makeAuthFromRequest returns an HTTP context function that extracts the Bearer
// access token from the Authorization header, validates it against Hydra's
// JWKS (signature, exp/nbf, issuer, audience), and stores the resulting subject
// (Kuberan user ID) and granted scope set in the context. On any failure it
// returns the bare context; tools then reject the call via getUserID.
func makeAuthFromRequest(v TokenValidator) server.HTTPContextFunc {
	return func(ctx context.Context, r *http.Request) context.Context {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.Get().Warn("MCP auth: missing Authorization header")
			return ctx
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Get().Warn("MCP auth: malformed Authorization header")
			return ctx
		}

		claims, err := v.Validate(ctx, parts[1])
		if err != nil {
			logger.Get().Warnf("MCP auth: token rejected: %v", err)
			return ctx
		}

		ctx = context.WithValue(ctx, userIDKey{}, claims.Subject)
		return context.WithValue(ctx, scopesKey{}, claims.Scopes)
	}
}

// errUnauthorized returns an MCP tool error for unauthenticated requests.
func errUnauthorized() *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError("unauthorized: valid Bearer token required")
}

// Run creates the MCP server, registers all tools, and starts the HTTP transport.
// oauth carries the Resource Server discovery settings advertised at the
// RFC 9728 well-known endpoint and in WWW-Authenticate challenges. validator
// verifies incoming Bearer access tokens against Hydra's JWKS.
func Run(svc Services, addr string, oauth OAuthConfig, validator TokenValidator) error {
	s := &Server{
		services: svc,
		mcp: server.NewMCPServer(
			"kuberan",
			"1.0.0",
			server.WithToolCapabilities(true),
		),
	}

	s.registerAccountTools()
	s.registerCategoryTools()
	s.registerTransactionTools()
	s.registerBudgetTools()
	s.registerInvestmentTools()
	s.registerSnapshotTools()

	mcpHandler := server.NewStreamableHTTPServer(s.mcp,
		server.WithHTTPContextFunc(makeAuthFromRequest(validator)),
	)

	// Wrap the MCP handler in our own mux so we can expose an unauthenticated
	// /health endpoint for container/orchestrator health checks alongside the
	// authenticated /mcp transport endpoint.
	mux := http.NewServeMux()
	// Wrap /mcp so 401 responses carry a WWW-Authenticate challenge pointing at
	// the Protected Resource Metadata document (RFC 9728), and serve that
	// document so MCP clients can discover the Hydra authorization server.
	mux.Handle("/mcp", withWWWAuthenticate(mcpHandler, oauth))
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
