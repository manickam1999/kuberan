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
	"kuberan/internal/middleware"
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

// makeAuthFromRequest returns an HTTP context function that extracts and
// validates the MCP JWT from the Authorization header, verifies the token
// hash against the database, checks the user is active, and stores the
// user ID in the context.
func makeAuthFromRequest(userService services.UserServicer) server.HTTPContextFunc {
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

		tokenString := parts[1]

		claims, err := middleware.ValidateMCPToken(tokenString)
		if err != nil {
			logger.Get().Warnf("MCP auth: invalid token: %v", err)
			return ctx
		}

		// Verify the token hash matches the stored hash (supports revocation).
		storedHash, err := userService.GetMCPTokenHash(claims.UserID)
		if err != nil {
			logger.Get().Warnf("MCP auth: failed to retrieve token hash for user %s: %v", claims.UserID, err)
			return ctx
		}
		if storedHash == "" || storedHash != middleware.HashToken(tokenString) {
			logger.Get().Warnf("MCP auth: token hash mismatch for user %s (token may be revoked)", claims.UserID)
			return ctx
		}

		// Verify the user still exists and is active.
		user, err := userService.GetUserByID(claims.UserID)
		if err != nil {
			logger.Get().Warnf("MCP auth: user %s not found: %v", claims.UserID, err)
			return ctx
		}
		if !user.IsActive {
			logger.Get().Warnf("MCP auth: user %s is inactive", claims.UserID)
			return ctx
		}

		return context.WithValue(ctx, userIDKey{}, claims.UserID)
	}
}

// errUnauthorized returns an MCP tool error for unauthenticated requests.
func errUnauthorized() *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError("unauthorized: valid Bearer token required")
}

// Run creates the MCP server, registers all tools, and starts the HTTP transport.
// oauth carries the Resource Server discovery settings advertised at the
// RFC 9728 well-known endpoint and in WWW-Authenticate challenges.
func Run(svc Services, addr string, oauth OAuthConfig) error {
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
		server.WithHTTPContextFunc(makeAuthFromRequest(svc.Users)),
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
