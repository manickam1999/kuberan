package main

import (
	"context"
	"fmt"
	"kuberan/internal/config"
	"kuberan/internal/database"
	"kuberan/internal/handlers"
	"kuberan/internal/hydra"
	"kuberan/internal/logger"
	"kuberan/internal/middleware"
	"kuberan/internal/services"
	"kuberan/internal/storage"
	"kuberan/internal/validator"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "kuberan/internal/docs" // Import swagger docs
)

// @title           Kuberan API
// @version         1.0
// @description     Kuberan is a personal finance application that allows users to efficiently manage their finances, make budgets, and track investments.
// @termsOfService  http://swagger.io/terms/

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description Pipeline API key for service-to-service authentication.

func main() {
	// Initialize logger (use ENV var if available, default to development)
	logger.Init(os.Getenv("ENV"))
	defer logger.Sync()

	if err := run(); err != nil {
		logger.Get().Fatalf("Fatal error: %v", err)
	}
}

func run() error {
	log := logger.Get()

	// Load configuration
	appConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize database configuration
	dbConfig, err := database.NewConfig()
	if err != nil {
		return fmt.Errorf("failed to load database configuration: %w", err)
	}

	// Create database manager
	dbManager, err := database.NewManager(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to create database manager: %w", err)
	}

	// Run migrations automatically in development; in production, run manually via cmd/migrate
	if appConfig.Env != config.Production {
		if err := dbManager.RunMigrations(); err != nil {
			return fmt.Errorf("failed to run database migrations: %w", err)
		}
	}

	// Initialize services
	db := dbManager.DB()
	userService := services.NewUserService(db)
	accountService := services.NewAccountService(db)
	categoryService := services.NewCategoryService(db)
	ruleService := services.NewRuleService(db, categoryService)
	transactionService := services.NewTransactionService(db, accountService, ruleService)
	budgetService := services.NewBudgetService(db)
	investmentService := services.NewInvestmentService(db, accountService)
	securityService := services.NewSecurityService(db)
	snapshotService := services.NewPortfolioSnapshotService(db)
	auditService := services.NewAuditService(db)
	telegramService := services.NewTelegramService(db)
	trustedClientService := services.NewTrustedClientService(db)

	// Receipt attachments (plan 017): blob store + service. The store is only
	// reachable through the ownership-checked API; MinIO stays private. When no
	// bucket is configured (e.g. a dev instance without MinIO) the API still
	// boots with a disabled store: attachment requests fail cleanly rather than
	// taking the whole server down.
	var blobStore storage.BlobStore
	if appConfig.StorageBucket == "" {
		log.Warn("STORAGE_BUCKET not set; receipt attachments are disabled")
		blobStore = storage.NewDisabledBlobStore()
	} else {
		s3Store, err := storage.NewS3BlobStore(appConfig.StorageConfig())
		if err != nil {
			return fmt.Errorf("failed to initialize blob store: %w", err)
		}
		blobStore = s3Store
	}
	attachmentService := services.NewAttachmentService(db, blobStore, services.AttachmentLimits{
		MaxUploadBytes:      appConfig.MaxUploadBytes,
		MaxAttachmentsPerTx: appConfig.MaxAttachmentsPerTx,
	})

	// Hydra admin client for the OAuth login/consent bridge (private network only).
	hydraAdmin := hydra.NewAdminClient(appConfig.HydraAdminURL)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(userService, auditService)
	oauthHandler := handlers.NewOAuthHandler(hydraAdmin, userService, trustedClientService, auditService, appConfig.OAuthScopes, appConfig.MCPResourceURL)
	registrationHandler := handlers.NewRegistrationHandler(hydraAdmin, auditService, appConfig.OAuthScopes)
	accountHandler := handlers.NewAccountHandler(accountService, auditService)
	categoryHandler := handlers.NewCategoryHandler(categoryService, auditService)
	transactionHandler := handlers.NewTransactionHandler(transactionService, auditService)
	attachmentHandler := handlers.NewAttachmentHandler(attachmentService, auditService, appConfig.MaxUploadBytes)
	budgetHandler := handlers.NewBudgetHandler(budgetService, auditService)
	ruleHandler := handlers.NewRuleHandler(ruleService, transactionService, auditService)
	investmentHandler := handlers.NewInvestmentHandler(investmentService, auditService)
	securityHandler := handlers.NewSecurityHandler(securityService, auditService)
	snapshotHandler := handlers.NewPortfolioSnapshotHandler(snapshotService, auditService)
	telegramHandler := handlers.NewTelegramHandler(telegramService, auditService)

	// Register custom validators before routes
	validator.Register()

	// Set Gin mode based on environment
	if appConfig.Env == config.Production {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogging())
	router.Use(middleware.ErrorHandler())

	// CORS middleware — CORS_ORIGIN env var controls allowed origins (default: *)
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", appConfig.CORSOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health check endpoint
	router.GET("/api/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "database": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "connected"})
	})

	// API v1 group
	v1 := router.Group("/api/v1")

	// Public routes
	auth := v1.Group("/auth")
	auth.POST("/register", authHandler.Register)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.RefreshToken)

	// OAuth login/consent bridge (public: driven by the apps/web pages during the
	// Hydra authorization flow, before a Kuberan session exists). See plan 15.
	oauth := v1.Group("/oauth")
	oauth.POST("/login", oauthHandler.Login)
	oauth.POST("/login/reject", oauthHandler.RejectLogin)
	oauth.GET("/consent", oauthHandler.GetConsent)
	oauth.POST("/consent/accept", oauthHandler.AcceptConsent)
	oauth.POST("/consent/reject", oauthHandler.RejectConsent)
	// Hardened DCR proxy: public clients only, restricted grants, capped scopes,
	// audited + alerted (Phase 5). Served at the RFC-standard /oauth2/register path
	// that Hydra advertises as its registration_endpoint ({issuer}/oauth2/register),
	// so cloudflared can route DCR through this proxy via simple path matching
	// (no path rewriting) instead of hitting Hydra's public endpoint directly.
	// The /api/v1/oauth/register alias is retained for internal/test callers.
	router.POST("/oauth2/register", registrationHandler.Register)
	oauth.POST("/register", registrationHandler.Register)

	// Protected routes
	protected := v1.Group("/")
	protected.Use(middleware.AuthMiddleware())

	// User profile
	protected.GET("/profile", authHandler.GetProfile)
	protected.PATCH("/profile", authHandler.UpdateProfileSettings)

	// Account routes
	accounts := protected.Group("/accounts")
	accounts.POST("/cash", accountHandler.CreateCashAccount)
	accounts.POST("/investment", accountHandler.CreateInvestmentAccount)
	accounts.POST("/credit-card", accountHandler.CreateCreditCardAccount)
	accounts.GET("", accountHandler.GetUserAccounts)
	accounts.GET("/:id", accountHandler.GetAccountByID)
	accounts.PUT("/:id", accountHandler.UpdateAccount)
	accounts.GET("/:id/transactions", transactionHandler.GetAccountTransactions)
	accounts.GET("/:id/investments", investmentHandler.GetAccountInvestments)

	// Transaction routes
	transactions := protected.Group("/transactions")
	transactions.GET("", transactionHandler.GetUserTransactions)
	transactions.POST("", transactionHandler.CreateTransaction)
	transactions.POST("/transfer", transactionHandler.CreateTransfer)
	transactions.GET("/spending-by-category", transactionHandler.GetSpendingByCategory)
	transactions.GET("/cashflow", transactionHandler.GetCashflow)
	transactions.GET("/monthly-summary", transactionHandler.GetMonthlySummary)
	transactions.GET("/daily-spending", transactionHandler.GetDailySpending)
	transactions.GET("/daily-summary", transactionHandler.GetDailySummary)
	transactions.GET("/top-expenses", transactionHandler.GetTopExpenses)
	transactions.GET("/:id", transactionHandler.GetTransactionByID)
	transactions.PUT("/:id", transactionHandler.UpdateTransaction)
	transactions.DELETE("/:id", transactionHandler.DeleteTransaction)
	transactions.POST("/:id/attachments", attachmentHandler.Upload)
	transactions.GET("/:id/attachments", attachmentHandler.List)
	transactions.GET("/:id/attachments/:aid", attachmentHandler.Download)
	transactions.DELETE("/:id/attachments/:aid", attachmentHandler.Delete)

	// Budget routes
	budgets := protected.Group("/budgets")
	budgets.POST("", budgetHandler.CreateBudget)
	budgets.GET("", budgetHandler.GetBudgets)
	// Static route registered before the param routes so Gin resolves it correctly.
	budgets.GET("/progress", budgetHandler.GetBudgetsProgress)
	budgets.GET("/:id", budgetHandler.GetBudget)
	budgets.PUT("/:id", budgetHandler.UpdateBudget)
	budgets.DELETE("/:id", budgetHandler.DeleteBudget)
	budgets.GET("/:id/progress", budgetHandler.GetBudgetProgress)

	// Transaction rule routes (plan 018). Static routes are registered before the
	// param route so Gin resolves /rules/reorder and /rules/preview correctly.
	rules := protected.Group("/rules")
	rules.POST("", ruleHandler.CreateRule)
	rules.GET("", ruleHandler.GetRules)
	rules.POST("/reorder", ruleHandler.ReorderRules)
	rules.POST("/preview", ruleHandler.PreviewRule)
	rules.GET("/:id", ruleHandler.GetRule)
	rules.PUT("/:id", ruleHandler.UpdateRule)
	rules.DELETE("/:id", ruleHandler.DeleteRule)
	rules.POST("/:id/apply", ruleHandler.ApplyRule)

	// Investment routes
	investments := protected.Group("/investments")
	investments.POST("", investmentHandler.AddInvestment)
	investments.GET("", investmentHandler.GetAllInvestments)
	investments.GET("/portfolio", investmentHandler.GetPortfolio)
	investments.GET("/snapshots", snapshotHandler.GetSnapshots)
	investments.GET("/:id", investmentHandler.GetInvestment)
	investments.POST("/:id/buy", investmentHandler.RecordBuy)
	investments.POST("/:id/sell", investmentHandler.RecordSell)
	investments.POST("/:id/dividend", investmentHandler.RecordDividend)
	investments.POST("/:id/split", investmentHandler.RecordSplit)
	investments.GET("/:id/transactions", investmentHandler.GetInvestmentTransactions)

	// Security routes (authenticated)
	securities := protected.Group("/securities")
	securities.GET("", securityHandler.ListSecurities)
	securities.GET("/:id", securityHandler.GetSecurity)
	securities.GET("/:id/prices", securityHandler.GetPriceHistory)

	// Category routes
	categories := protected.Group("/categories")
	categories.POST("", categoryHandler.CreateCategory)
	categories.GET("", categoryHandler.GetUserCategories)
	categories.GET("/:id", categoryHandler.GetCategoryByID)
	categories.PUT("/:id", categoryHandler.UpdateCategory)
	categories.DELETE("/:id", categoryHandler.DeleteCategory)

	// Telegram routes
	telegram := protected.Group("/telegram")
	telegram.GET("/link", telegramHandler.GetLink)
	telegram.POST("/generate-code", telegramHandler.GenerateCode)
	telegram.DELETE("/unlink", telegramHandler.Unlink)

	// Internal routes (for bot service communication)
	internal := v1.Group("/internal")
	internal.Use(middleware.InternalAuthMiddleware(appConfig.BotInternalSecret))
	internal.POST("/telegram/complete-link", telegramHandler.CompleteLink)
	internal.GET("/telegram/resolve/:telegram_user_id", telegramHandler.ResolveUser)
	internal.POST("/telegram/activity/:telegram_user_id", telegramHandler.RecordActivity)

	// Pipeline routes (API key auth, no JWT)
	pipeline := v1.Group("/pipeline")
	pipeline.Use(middleware.PipelineAuthMiddleware(appConfig.PipelineAPIKey))
	pipeline.GET("/securities", securityHandler.ListAllSecurities)
	pipeline.POST("/securities", securityHandler.CreateSecurity)
	pipeline.POST("/securities/prices", securityHandler.RecordPrices)
	pipeline.POST("/snapshots", snapshotHandler.ComputeSnapshots)

	// Create HTTP server
	srv := &http.Server{
		Addr:    ":" + appConfig.Port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Infof("Starting Kuberan backend server on port %s", appConfig.Port)
		log.Infof("Swagger documentation available at http://localhost:%s/swagger/index.html", appConfig.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	// Give outstanding requests 5 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("Server forced to shutdown: %v", err)
	}

	// Close database connections
	sqlDB, dbErr := db.DB()
	if dbErr == nil {
		if err := sqlDB.Close(); err != nil {
			log.Errorf("Error closing database: %v", err)
		}
	}

	log.Info("Server exited cleanly")
	return nil
}
