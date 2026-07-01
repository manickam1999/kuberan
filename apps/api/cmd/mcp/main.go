package main

import (
	"fmt"
	"os"
	"strings"

	"kuberan/internal/config"
	"kuberan/internal/database"
	"kuberan/internal/logger"
	mcpserver "kuberan/internal/mcp"
	"kuberan/internal/services"
)

func main() {
	logger.Init(os.Getenv("ENV"))
	defer logger.Sync()

	if err := run(); err != nil {
		logger.Get().Fatalf("Fatal error: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	dbConfig, err := database.NewConfig()
	if err != nil {
		return fmt.Errorf("failed to load database configuration: %w", err)
	}

	dbManager, err := database.NewManager(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to create database manager: %w", err)
	}

	db := dbManager.DB()

	userService := services.NewUserService(db)
	accountService := services.NewAccountService(db)
	categoryService := services.NewCategoryService(db)
	transactionService := services.NewTransactionService(db, accountService)
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService)
	snapshotService := services.NewPortfolioSnapshotService(db)

	// MCP_PORT accepts either a bare port ("8081") or a full listen address
	// (":8081", "0.0.0.0:8081"). Normalize to a valid net.Listen address.
	addr := os.Getenv("MCP_PORT")
	if addr == "" {
		addr = "8081"
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	logger.Get().Infof("Starting MCP server on %s", addr)

	// The Resource Server validates Hydra-issued JWT access tokens offline
	// against the authorization server's published JWKS.
	jwksURL := strings.TrimRight(cfg.HydraIssuerURL, "/") + "/.well-known/jwks.json"
	validator := mcpserver.NewHydraValidator(jwksURL, cfg.HydraIssuerURL, cfg.MCPResourceURL)

	return mcpserver.Run(mcpserver.Services{
		Users:        userService,
		Accounts:     accountService,
		Categories:   categoryService,
		Transactions: transactionService,
		Budgets:      budgetService,
		Investments:  investmentService,
		Snapshots:    snapshotService,
	}, addr, mcpserver.OAuthConfig{
		Issuer:      cfg.HydraIssuerURL,
		ResourceURL: cfg.MCPResourceURL,
		Scopes:      cfg.OAuthScopes,
	}, validator)
}
