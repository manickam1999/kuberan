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

	fxService := services.NewFXService(db, cfg.FXMaxStaleness)
	userService := services.NewUserService(db, fxService)
	accountService := services.NewAccountService(db, services.WithFXService(fxService))
	categoryService := services.NewCategoryService(db)
	ruleService := services.NewRuleService(db, categoryService)
	transactionService := services.NewTransactionService(db, accountService, ruleService, services.WithTransactionFXService(fxService))
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService, fxService)
	snapshotService := services.NewPortfolioSnapshotService(db, fxService)
	receivableService := services.NewReceivableService(db, accountService, ruleService)
	auditService := services.NewAuditService(db)
	grantService := services.NewOAuthGrantService(db)

	// MCP_PORT accepts either a bare port ("8081") or a full listen address
	// (":8081", "0.0.0.0:8081"). Normalize to a valid net.Listen address.
	addr := os.Getenv("MCP_PORT")
	if addr == "" {
		addr = "8081"
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	logger.Get().Infof("Starting MCP server on %s (writes enabled: %t)", addr, cfg.MCPWriteEnabled)
	if cfg.MCPWriteEnabled {
		// The api service registers and grants the write:* scopes; if it does
		// not also have the flag, the consent screen never offers them and
		// every write is refused.
		logger.Get().Info("MCP writes enabled: the api service must also run with MCP_WRITE_ENABLED=true")
	}

	trustedProxies, err := mcpserver.ParseTrustedProxies(cfg.MCPTrustedProxyCIDRs)
	if err != nil {
		return fmt.Errorf("MCP_TRUSTED_PROXY_CIDRS: %w", err)
	}

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
		Receivables:  receivableService,
		Audit:        auditService,
		Grants:       grantService,
	}, addr, mcpserver.OAuthConfig{
		Issuer:      cfg.HydraIssuerURL,
		ResourceURL: cfg.MCPResourceURL,
		Scopes:      cfg.OAuthScopes,
	}, validator, mcpserver.WriteConfig{
		Enabled:       cfg.MCPWriteEnabled,
		MaxAmount:     cfg.MCPMaxWriteAmount,
		RatePerMinute: cfg.MCPWriteRatePerMinute,
		RatePerDay:    cfg.MCPWriteRatePerDay,
	}, trustedProxies)
}
